package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AarnoStormborn/tree-trunk/internal/config"
	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
	"github.com/AarnoStormborn/tree-trunk/internal/state"
)

// wtCommand implements `tree-trunk wt <op> [flags]` — the mutating half of
// the agent-facing CLI (docs/design/10-agent-cli.md). It reuses the guarded
// internal/git engine: branch-exists, checked-out-elsewhere, dirty, locked.
//
// Every op returns a JSON envelope: { "ok": bool, ... } with structured
// errors { "ok": false, "error": { "code", "message", "repo", "branch" } }.

// wtUsage is printed by `tree-trunk wt --help` and by usage errors.
const wtUsage = `tree-trunk wt <op> [repo] [branch] [flags]

Ops:
  list                     all worktrees across repos (read-only)
  create <repo> <branch>   create a worktree [--from base] [--path dir] [--force]
  delete <repo> <branch>   remove a worktree (refuses dirty unless --force)
  lock   <repo> <branch>   lock a worktree [--reason S]
  unlock <repo> <branch>   unlock a worktree
  prune  [repo]            prune prunable worktrees [--dry-run]

Flags may appear before or after positionals.
Exit codes: 0 ok, 1 usage/IO, 3 not found, 4 dirty-blocked, 5 locked.`

// wtResult is the JSON envelope for a successful op.
type wtResult struct {
	Ok     bool     `json:"ok"`
	Op     string   `json:"op,omitempty"`
	Repo   string   `json:"repo,omitempty"`
	Branch string   `json:"branch,omitempty"`
	Path   string   `json:"path,omitempty"`
	DryRun bool     `json:"dry_run,omitempty"`
	Pruned []string `json:"pruned,omitempty"`
	Error  *wtError `json:"error,omitempty"`
}

// wtError is the structured error an agent can react to programmatically.
type wtError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Repo    string `json:"repo,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Path    string `json:"path,omitempty"`
}

// resolveRepo finds the repo whose name or id matches sel. It scans like the
// TUI/query so worktrees+status are populated, then selects by exact name,
// repo id (git dir), or repo path.
func resolveRepo(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, sel string) (*model.Repo, error) {
	store := state.NewStore()
	collect := func(p string) error {
		repo, err := git.Resolve(ctx, runner, p)
		if err != nil {
			return nil
		}
		store.Upsert(repo)
		return nil
	}
	for _, p := range cfg.Repos {
		if err := collect(p); err != nil {
			return nil, err
		}
	}
	if !cfg.NoScan {
		if err := collectScanned(ctx, cfg, runner, store, collect); err != nil {
			return nil, err
		}
	}
	// Refresh so Worktrees are populated (needed for delete/lock/prune).
	rf := state.NewRefresher(runner, store, cfg.Workers)
	for _, r := range store.List() {
		rf.RefreshOne(ctx, r.ID)
	}
	for _, r := range store.List() {
		if r.Name == sel || r.ID == sel || r.Path == sel {
			return r, nil
		}
	}
	return nil, fmt.Errorf("repo %q not found", sel)
}

// runWTCommand parses `tree-trunk wt <op> [args] [flags]`.
func runWTCommand(args []string, version string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tree-trunk wt <op> [repo] [branch] [flags] (ops: list, create, delete, lock, unlock, prune)")
	}
	op := args[0]
	rest := args[1:]

	// `tree-trunk wt --help` / `wt help` (help is a success, not an error).
	if op == "--help" || op == "-h" || op == "help" {
		fmt.Println(wtUsage)
		return nil
	}

	// Interleaved flag parsing: accept --flags before or after positionals
	// (Go's flag stops at the first positional; agents shouldn't care about
	// ordering). We parse flags manually, leaving positionals untouched.
	jsonOut := true
	force := false
	dryRun := false
	reason := ""
	base := ""
	path := ""
	var repos, roots []string
	noScan := false
	var positionals []string
	{
		i := 0
		for i < len(rest) {
			a := rest[i]
			switch {
			case a == "--json":
				jsonOut = true
				i++
			case a == "--no-json":
				jsonOut = false
				i++
			case a == "--force":
				force = true
				i++
			case a == "--dry-run":
				dryRun = true
				i++
			case a == "--no-scan":
				noScan = true
				i++
			case a == "--help" || a == "-h":
				fmt.Println(wtUsage)
				return nil
			case a == "--repo" && i+1 < len(rest):
				repos = append(repos, rest[i+1])
				i += 2
			case a == "--scan-root" && i+1 < len(rest):
				roots = append(roots, rest[i+1])
				i += 2
			case a == "--reason" && i+1 < len(rest):
				reason = rest[i+1]
				i += 2
			case a == "--from" && i+1 < len(rest):
				base = rest[i+1]
				i += 2
			case a == "--path" && i+1 < len(rest):
				path = rest[i+1]
				i += 2
			case strings.HasPrefix(a, "-"):
				return fmt.Errorf("unknown flag %q", a)
			default:
				positionals = append(positionals, a)
				i++
			}
		}
	}

	cfg := config.Defaults()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg.Home = home
	cfg.Repos = repos
	cfg.ScanRoots = roots
	cfg.NoScan = noScan
	cfg.Workers = 4

	gitPath, err := git.LookPath()
	if err != nil {
		return err
	}
	runner := git.NewExecRunner(gitPath)
	ctx := context.Background()

	switch op {
	case "list":
		return wtList(ctx, &cfg, runner, jsonOut)
	case "create":
		if len(positionals) < 2 {
			return fmt.Errorf("usage: tree-trunk wt create <repo> <branch> [--from base] [--path dir]")
		}
		return wtCreate(ctx, &cfg, runner, positionals[0], positionals[1], base, path, force, jsonOut)
	case "delete":
		if len(positionals) < 2 {
			return fmt.Errorf("usage: tree-trunk wt delete <repo> <branch> [--force]")
		}
		return wtDelete(ctx, &cfg, runner, positionals[0], positionals[1], force, jsonOut)
	case "lock":
		if len(positionals) < 2 {
			return fmt.Errorf("usage: tree-trunk wt lock <repo> <branch> [--reason S]")
		}
		return wtLock(ctx, &cfg, runner, positionals[0], positionals[1], reason, true, jsonOut)
	case "unlock":
		if len(positionals) < 2 {
			return fmt.Errorf("usage: tree-trunk wt unlock <repo> <branch>")
		}
		return wtLock(ctx, &cfg, runner, positionals[0], positionals[1], "", false, jsonOut)
	case "prune":
		return wtPrune(ctx, &cfg, runner, positionals, dryRun, jsonOut)
	default:
		return fmt.Errorf("unknown wt op %q (ops: list, create, delete, lock, unlock, prune)", op)
	}
}

// findWorktree returns the worktree of repo matching a branch or path.
func findWorktree(repo *model.Repo, sel string) *model.Worktree {
	for i := range repo.Worktrees {
		w := &repo.Worktrees[i]
		if w.Branch == sel || filepath.Clean(w.Path) == filepath.Clean(sel) {
			return w
		}
	}
	return nil
}

func wtList(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, jsonOut bool) error {
	store := state.NewStore()
	collect := func(p string) error {
		repo, err := git.Resolve(ctx, runner, p)
		if err != nil {
			return nil
		}
		store.Upsert(repo)
		return nil
	}
	for _, p := range cfg.Repos {
		if err := collect(p); err != nil {
			return err
		}
	}
	if !cfg.NoScan {
		if err := collectScanned(ctx, cfg, runner, store, collect); err != nil {
			return err
		}
	}
	rf := state.NewRefresher(runner, store, cfg.Workers)
	for _, r := range store.List() {
		rf.RefreshOne(ctx, r.ID)
	}
	type wtRow struct {
		Repo     string `json:"repo"`
		Branch   string `json:"branch"`
		Path     string `json:"path"`
		IsMain   bool   `json:"is_main"`
		Locked   bool   `json:"locked"`
		Prunable bool   `json:"prunable"`
		Dirty    bool   `json:"dirty"`
	}
	type wtListOut struct {
		Ok        bool    `json:"ok"`
		Total     int     `json:"total_worktrees"`
		Repos     int     `json:"repos"`
		Worktrees []wtRow `json:"worktrees"`
	}
	var rows []wtRow
	repoCount := 0
	for _, r := range store.List() {
		if len(r.Worktrees) == 0 {
			continue
		}
		repoCount++
		for _, w := range r.Worktrees {
			rows = append(rows, wtRow{
				Repo:     r.Name,
				Branch:   w.Branch,
				Path:     w.Path,
				IsMain:   w.IsMain,
				Locked:   w.Locked,
				Prunable: w.Prunable,
				Dirty:    w.Dirty,
			})
		}
	}
	out := wtListOut{Ok: true, Total: len(rows), Repos: repoCount, Worktrees: rows}
	return emitJSON(out, jsonOut)
}

func wtCreate(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, repoSel, branch, base, path string, force, jsonOut bool) error {
	repo, err := resolveRepo(ctx, cfg, runner, repoSel)
	if err != nil {
		return emitErr(&wtError{Code: "repo_not_found", Message: err.Error(), Repo: repoSel})
	}
	// Default destination: ~/.worktrees/<repo>/<slug>.
	if path == "" {
		home, _ := os.UserHomeDir()
		slug := git.Slug(branch)
		if slug == "" {
			return emitErr(&wtError{Code: "invalid_branch", Message: "cannot slug branch name", Repo: repoSel, Branch: branch})
		}
		path = filepath.Join(home, ".worktrees", repo.Name, slug)
	}
	err = git.AddWorktree(ctx, runner, repo.Path, git.AddOptions{
		Branch:      branch,
		Base:        base,
		Path:        path,
		GuessRemote: true,
		Force:       force,
	})
	if err != nil {
		return emitErr(classifyWTError(err, repo.Name, branch, path))
	}
	return emitJSON(wtResult{Ok: true, Op: "create", Repo: repo.Name, Branch: branch, Path: path}, jsonOut)
}

func wtDelete(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, repoSel, branch string, force, jsonOut bool) error {
	repo, err := resolveRepo(ctx, cfg, runner, repoSel)
	if err != nil {
		return emitErr(&wtError{Code: "repo_not_found", Message: err.Error(), Repo: repoSel})
	}
	wt := findWorktree(repo, branch)
	if wt == nil {
		return emitErr(&wtError{Code: "worktree_not_found", Message: "no worktree matches branch/path", Repo: repo.Name, Branch: branch})
	}
	// Deterministic guard ahead of git: a locked tree cannot be removed even
	// with --force (git needs `remove -f -f`), and the parsed worktree list is
	// authoritative, so don't rely on stderr phrasing alone.
	if wt.Locked {
		return emitErr(&wtError{Code: "worktree_locked", Message: "worktree is locked: " + wt.Path, Repo: repo.Name, Branch: wt.Branch, Path: wt.Path})
	}
	err = git.RemoveWorktree(ctx, runner, repo.Path, wt.Path, force)
	if err != nil {
		return emitErr(classifyWTError(err, repo.Name, wt.Branch, wt.Path))
	}
	return emitJSON(wtResult{Ok: true, Op: "delete", Repo: repo.Name, Branch: wt.Branch, Path: wt.Path}, jsonOut)
}

func wtLock(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, repoSel, branch, reason string, lock bool, jsonOut bool) error {
	repo, err := resolveRepo(ctx, cfg, runner, repoSel)
	if err != nil {
		return emitErr(&wtError{Code: "repo_not_found", Message: err.Error(), Repo: repoSel})
	}
	wt := findWorktree(repo, branch)
	if wt == nil {
		return emitErr(&wtError{Code: "worktree_not_found", Message: "no worktree matches branch/path", Repo: repo.Name, Branch: branch})
	}
	op := "lock"
	var err2 error
	if lock {
		err2 = git.LockWorktree(ctx, runner, repo.Path, wt.Path, reason)
	} else {
		err2 = git.UnlockWorktree(ctx, runner, repo.Path, wt.Path)
		op = "unlock"
	}
	if err2 != nil {
		return emitErr(classifyWTError(err2, repo.Name, wt.Branch, wt.Path))
	}
	return emitJSON(wtResult{Ok: true, Op: op, Repo: repo.Name, Branch: wt.Branch, Path: wt.Path}, jsonOut)
}

func wtPrune(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, positionals []string, dryRun, jsonOut bool) error {
	// Prune can target all repos (no selector) or one repo.
	var repos []*model.Repo
	if len(positionals) > 0 {
		r, err := resolveRepo(ctx, cfg, runner, positionals[0])
		if err != nil {
			return emitErr(&wtError{Code: "repo_not_found", Message: err.Error(), Repo: positionals[0]})
		}
		repos = []*model.Repo{r}
	} else {
		store := state.NewStore()
		collect := func(p string) error {
			repo, err := git.Resolve(ctx, runner, p)
			if err != nil {
				return nil
			}
			store.Upsert(repo)
			return nil
		}
		for _, p := range cfg.Repos {
			if err := collect(p); err != nil {
				return err
			}
		}
		if !cfg.NoScan {
			if err := collectScanned(ctx, cfg, runner, store, collect); err != nil {
				return err
			}
		}
		rf := state.NewRefresher(runner, store, cfg.Workers)
		for _, r := range store.List() {
			rf.RefreshOne(ctx, r.ID)
		}
		repos = store.List()
	}
	var pruned []string
	for _, r := range repos {
		out, err := git.PruneWorktree(ctx, runner, r.Path, dryRun)
		if err != nil {
			return emitErr(classifyWTError(err, r.Name, "", r.Path))
		}
		if out != "" {
			pruned = append(pruned, out)
		}
	}
	return emitJSON(wtResult{Ok: true, Op: "prune", DryRun: dryRun, Pruned: pruned}, jsonOut)
}

// classifyWTError maps the guarded engine errors to stable agent codes.
func classifyWTError(err error, repo, branch, path string) *wtError {
	code := "git_error"
	var msg string
	switch e := err.(type) {
	case *git.WorktreeDirtyError:
		code = "worktree_dirty"
		msg = e.Error()
	case *git.WorktreeLockedError:
		code = "worktree_locked"
		msg = e.Error()
	case *git.BranchCheckedOutElsewhereError:
		code = "branch_checked_out_elsewhere"
		msg = e.Error()
	case *git.BranchExistsError:
		code = "branch_exists"
		msg = e.Error()
	default:
		msg = err.Error()
	}
	return &wtError{Code: code, Message: msg, Repo: repo, Branch: branch, Path: path}
}

// exitCodeFor maps a structured agent error to the documented process exit
// code (docs/design/10-agent-cli.md §Exit codes): 0 ok, 1 usage/IO,
// 3 not-found, 4 dirty-blocked, 5 locked. Codes without a documented slot
// (branch_exists, branch_checked_out_elsewhere, git_error, usage) are 1.
func exitCodeFor(e *wtError) int {
	switch e.Code {
	case "repo_not_found", "worktree_not_found":
		return 3
	case "worktree_dirty":
		return 4
	case "worktree_locked":
		return 5
	default:
		return 1
	}
}

// emitErr prints a structured error JSON envelope and returns an error that
// carries the contract exit code. The envelope is the only output: main
// detects the exitCoder and prints nothing more to stderr.
func emitErr(e *wtError) error {
	out, _ := json.MarshalIndent(wtResult{Ok: false, Error: e}, "", "  ")
	fmt.Println(string(out))
	return &codedError{code: exitCodeFor(e), err: errors.New(e.Code + ": " + e.Message)}
}

// emitJSON prints the envelope if jsonOut, else a compact human line.
func emitJSON(v interface{}, jsonOut bool) error {
	if jsonOut {
		buf, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(buf))
		return nil
	}
	buf, _ := json.Marshal(v)
	fmt.Println(string(buf))
	return nil
}
