package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
	"github.com/AarnoStormborn/tree-trunk/internal/state"
)

// worktreeActionMsg reports the result of a worktree mutation.
type worktreeActionMsg struct {
	op   string // "add", "remove", "lock", "unlock", "prune", "open"
	repo string
	err  error
}

// worktreeActions bundles the git runner + store for action commands.
type worktreeActions struct {
	runner *git.ExecRunner
	store  *state.Store
}

func newWorktreeActions(gitPath string, store *state.Store) *worktreeActions {
	return &worktreeActions{runner: git.NewExecRunner(gitPath), store: store}
}

// addCmd creates a worktree (git layer decides -b vs existing branch).
func (a *worktreeActions) addCmd(repo *model.Repo, branch, base, path string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := git.AddWorktree(ctx, a.runner, repo.Path, git.AddOptions{
			Branch:      branch,
			Base:        base,
			Path:        path,
			GuessRemote: true,
		})
		return worktreeActionMsg{op: "add", repo: repo.ID, err: err}
	}
}

// removeCmd runs the two-step remove (safe first; force only after the UI
// showed the dirty-worktree confirmation — 03 §3.2).
func (a *worktreeActions) removeCmd(repo *model.Repo, wt model.Worktree, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := git.RemoveWorktree(ctx, a.runner, repo.Path, wt.Path, force)
		return worktreeActionMsg{op: "remove", repo: repo.ID, err: err}
	}
}

func (a *worktreeActions) lockCmd(repo *model.Repo, wt model.Worktree) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var err error
		if wt.Locked {
			err = git.UnlockWorktree(ctx, a.runner, repo.Path, wt.Path)
		} else {
			err = git.LockWorktree(ctx, a.runner, repo.Path, wt.Path, "")
		}
		return worktreeActionMsg{op: "lock", repo: repo.ID, err: err}
	}
}

func (a *worktreeActions) pruneCmd(repo *model.Repo, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		out, err := git.PruneWorktree(ctx, a.runner, repo.Path, dryRun)
		if dryRun && err == nil {
			err = nil
			_ = out // preview shown via toast in M2
		}
		return worktreeActionMsg{op: "prune", repo: repo.ID, err: err}
	}
}

// openWorktree prints the path and copies it to the clipboard (D11), with
// print-only degradation when the clipboard is unavailable (review m8).
func openWorktree(path string) tea.Cmd {
	return func() tea.Msg {
		fmt.Println(path)
		err := clipboard.WriteAll(path)
		return worktreeActionMsg{op: "open", repo: "", err: err}
	}
}

// suggestWorktreePath builds the D8 default path:
// <worktrees.directory>/<repo>/<branch-slug> (00-decisions.md D8/Q3).
func suggestWorktreePath(dir, repoName, branch string) string {
	slug := git.Slug(branch)
	if slug == "" {
		slug = "worktree"
	}
	return filepath.Join(dir, repoName, slug)
}

// uniquePath appends -2, -3, … while the path exists (review m7).
func uniquePath(p string) string {
	if !pathExists(p) {
		return p
	}
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	for i := 2; ; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s-%d", base, i))
		if !pathExists(cand) {
			return cand
		}
	}
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// wtReload reloads worktrees for a repo after a mutation (dirty flags for
// the focused repo only — review M5).
func wtReloadCmd(refresher *state.Refresher, repoID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		refresher.LoadWorktrees(ctx, repoID, true)
		return wtReloadMsg{repo: repoID}
	}
}

type wtReloadMsg struct{ repo string }

func wtErrorText(err error) string {
	if err == nil {
		return ""
	}
	switch e := err.(type) {
	case *git.BranchCheckedOutElsewhereError:
		return fmt.Sprintf("branch %q is checked out in another worktree — use a different branch or detach", e.Branch)
	case *git.BranchExistsError:
		return e.Error()
	case *git.WorktreeDirtyError:
		return "worktree has modified/untracked files — confirm force remove"
	case *git.WorktreeLockedError:
		return "worktree is locked — unlock it first"
	default:
		msg := strings.TrimSpace(err.Error())
		if strings.Contains(msg, ": ") {
			if i := strings.Index(msg, ": "); i > 0 {
				msg = msg[i+2:]
			}
		}
		return msg
	}
}
