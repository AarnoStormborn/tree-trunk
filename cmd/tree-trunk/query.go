package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/AarnoStormborn/tree-trunk/internal/config"
	"github.com/AarnoStormborn/tree-trunk/internal/discover"
	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
	"github.com/AarnoStormborn/tree-trunk/internal/state"
)

// queryOutput is the top-level JSON document emitted by `tree-trunk query`.
// It is the machine-readable "what's working where, and how much" contract
// for coding agents and scripts (docs/design/… agent CLI).
type queryOutput struct {
	Version   string    `json:"version"`
	ScannedAt time.Time `json:"scanned_at"`
	Roots     []string  `json:"roots,omitempty"`
	Total     int       `json:"total_repos"`
	Repos     []repoDoc `json:"repos"`
}

// repoDoc is one repository in the query document. It wraps the domain
// model.Repo with the stable JSON tags already on the struct.
type repoDoc struct {
	model.Repo
}

// runQuery implements `tree-trunk query [--json] [--filter k=v] [--fields a,b]`.
// It scans+refreshes like the TUI does (so worktrees/status are populated),
// then emits a single deterministic JSON document (sorted by repo id).
func runQuery(cfg *config.Config, gitPath, version string, filterKV, fields []string, jsonOut bool) error {
	ctx := context.Background()
	store := state.NewStore()
	runner := git.NewExecRunner(gitPath)
	refresher := state.NewRefresher(runner, store, cfg.Workers)

	// Discover + resolve every repo, then refresh each once so worktrees and
	// status are populated (mirrors the TUI's flow).
	collect := func(p string) error {
		repo, err := git.Resolve(ctx, runner, p)
		if err != nil {
			return nil // not a git repo / inaccessible: skip
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
		roots := discover.Roots(cfg.Home, cfg.ScanRoots, cfg.Discover.ScanRoots, cfg.Discover.ScanHome)
		opts := discover.Options{
			Roots:           roots,
			MaxDepth:        cfg.Discover.MaxDepth,
			Ignore:          cfg.Discover.Ignore,
			IncludeBare:     cfg.Discover.IncludeBare,
			FollowSymlinks:  cfg.Discover.FollowSymlinks,
			Hidden:          cfg.Discover.HiddenDirs,
			HiddenPeekDepth: cfg.Discover.HiddenPeekDepth,
		}
		if err := discover.Scanner(ctx, opts, func(hit discover.Hit) error {
			return collect(hit.Path)
		}); err != nil {
			return err
		}
	}

	// Refresh each discovered repo once (status + worktrees).
	ids := store.List()
	for _, id := range ids {
		refresher.RefreshOne(ctx, id.ID)
	}

	// Build output doc.
	out := queryOutput{
		Version:   version,
		ScannedAt: time.Now().UTC(),
		Roots:     rootsPlain(cfg),
	}
	repos := store.List()
	// Apply filters, then fields, then sort.
	repos = filterRepos(repos, filterKV)
	docs := make([]repoDoc, 0, len(repos))
	for _, r := range repos {
		docs = append(docs, repoDoc{Repo: *r})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	if len(fields) > 0 {
		docs = applyFields(docs, fields)
	}
	out.Repos = docs
	out.Total = len(docs)

	var buf []byte
	var err error
	if jsonOut {
		buf, err = json.MarshalIndent(out, "", "  ")
	} else {
		buf, err = json.Marshal(out) // compact
	}
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}

// rootsPlain returns the effective scan roots (home-expanded) as a plain list.
func rootsPlain(cfg *config.Config) []string {
	return discover.Roots(cfg.Home, cfg.ScanRoots, cfg.Discover.ScanRoots, cfg.Discover.ScanHome)
}

// discoverOptions builds discovery options from config.
func discoverOptions(cfg *config.Config) discover.Options {
	return discover.Options{
		Roots:           discover.Roots(cfg.Home, cfg.ScanRoots, cfg.Discover.ScanRoots, cfg.Discover.ScanHome),
		MaxDepth:        cfg.Discover.MaxDepth,
		Ignore:          cfg.Discover.Ignore,
		IncludeBare:     cfg.Discover.IncludeBare,
		FollowSymlinks:  cfg.Discover.FollowSymlinks,
		Hidden:          cfg.Discover.HiddenDirs,
		HiddenPeekDepth: cfg.Discover.HiddenPeekDepth,
	}
}

// collectAll runs discovery + explicit repos, resolving each path into the
// store, then refreshes each repo once (status + worktrees).
func collectAll(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, store *state.Store) error {
	collect := func(p string) error {
		repo, err := git.Resolve(ctx, runner, p)
		if err != nil {
			return nil // not a git repo / inaccessible: skip
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
		rf0 := state.NewRefresher(runner, store, 1)
		_ = rf0
		err := collectScanned(ctx, cfg, runner, store, collect)
		if err != nil {
			return err
		}
	}
	rf := state.NewRefresher(runner, store, cfg.Workers)
	for _, r := range store.List() {
		rf.RefreshOne(ctx, r.ID)
	}
	return nil
}

// collectScanned runs the discovery scanner, calling collect on each hit.
func collectScanned(ctx context.Context, cfg *config.Config, runner *git.ExecRunner, store *state.Store, collect func(string) error) error {
	opts := discoverOptions(cfg)
	return discover.Scanner(ctx, opts, func(hit discover.Hit) error {
		return collect(hit.Path)
	})
}

// filterRepos keeps repos whose fields match all k=v filters.
// Supported keys: id, name, branch, dirty, prunable, locked, bare, worktrees.
func filterRepos(repos []*model.Repo, kvs []string) []*model.Repo {
	if len(kvs) == 0 {
		return repos
	}
	var out []*model.Repo
	for _, r := range repos {
		ok := true
		for _, kv := range kvs {
			k, v, _ := strings.Cut(kv, "=")
			if !matchRepo(r, k, v) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

func matchRepo(r *model.Repo, k, v string) bool {
	switch k {
	case "id":
		return r.ID == v
	case "name":
		return r.Name == v
	case "branch":
		return r.Branch == v
	case "dirty":
		return (r.Status.Dirty() == (v == "true"))
	case "prunable":
		pr := false
		for _, w := range r.Worktrees {
			if w.Prunable {
				pr = true
				break
			}
		}
		return pr == (v == "true")
	case "locked":
		lc := false
		for _, w := range r.Worktrees {
			if w.Locked {
				lc = true
				break
			}
		}
		return lc == (v == "true")
	case "bare":
		return r.Bare == (v == "true")
	case "worktrees":
		n := len(r.Worktrees)
		return fmt.Sprintf("%d", n) == v
	}
	return false
}

// applyFields trims each repo doc to only the requested comma-separated
// top-level fields. Nested fields (e.g. status.*) are passed through whole.
func applyFields(docs []repoDoc, fields []string) []repoDoc {
	set := map[string]bool{}
	for _, f := range fields {
		for _, part := range strings.Split(f, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				set[part] = true
			}
		}
	}
	for i := range docs {
		d := &docs[i]
		d.Repo = trimRepo(d.Repo, set)
	}
	return docs
}

func trimRepo(r model.Repo, keep map[string]bool) model.Repo {
	zero := model.Repo{}
	r0 := zero
	// Clear fields not requested (keep ID always as identity).
	if keep["id"] {
		r0.ID = r.ID
	}
	if keep["name"] {
		r0.Name = r.Name
	}
	if keep["path"] {
		r0.Path = r.Path
	}
	if keep["git_dir"] {
		r0.GitDir = r.GitDir
	}
	if keep["bare"] {
		r0.Bare = r.Bare
	}
	if keep["worktrees"] {
		r0.Worktrees = r.Worktrees
	}
	if keep["branch"] {
		r0.Branch = r.Branch
	}
	if keep["status"] {
		r0.Status = r.Status
	}
	return r0
}

// runQueryCommand parses `tree-trunk query [flags]` and runs it. It reuses
// the common config flags (--repo/--scan-root/--no-scan) via a second pass.
// strList is a local repeated-string flag (config's multiFlag is unexported).
type strList []string

func (s *strList) String() string     { return strings.Join(*s, ",") }
func (s *strList) Set(v string) error { *s = append(*s, v); return nil }

func runQueryCommand(args []string) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit a single pretty JSON document (default)")
	var filterKV strList
	fs.Var(&filterKV, "filter", "filter k=v (repeatable); keys: id,name,branch,dirty,prunable,locked,bare,worktrees")
	var fields strList
	fs.Var(&fields, "fields", "only top-level fields (repeatable/comma); e.g. id,branch,status")
	// Common config flags so agents can scope the query to specific repos/roots.
	var repos, roots strList
	var noScan bool
	fs.Var(&repos, "repo", "explicit repo path (repeatable)")
	fs.Var(&roots, "scan-root", "scan root (repeatable)")
	fs.BoolVar(&noScan, "no-scan", false, "do not scan; use --repo inputs only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Defaults()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home dir: %w", err)
	}
	cfg.Home = home
	cfg.Repos = repos
	cfg.ScanRoots = roots
	cfg.NoScan = noScan

	gitPath, err := git.LookPath()
	if err != nil {
		return err
	}
	return runQuery(&cfg, gitPath, version, filterKV, fields, *jsonOut)
}
