package git

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// WorktreeDirtyError means `git worktree remove` refused because the
// worktree has modified/untracked files (two-step force flow, 03 §3.2).
type WorktreeDirtyError struct{ Path string }

func (e *WorktreeDirtyError) Error() string {
	return "worktree has modified or untracked files: " + e.Path
}

// WorktreeLockedError means the worktree is locked and cannot be removed.
type WorktreeLockedError struct{ Path string }

func (e *WorktreeLockedError) Error() string { return "worktree is locked: " + e.Path }

// BranchCheckedOutElsewhereError means the branch is checked out in another
// worktree (the #1 worktree footgun; guard per 03 §5).
type BranchCheckedOutElsewhereError struct{ Branch string }

func (e *BranchCheckedOutElsewhereError) Error() string {
	return "branch is already checked out in another worktree: " + e.Branch
}

// BranchExistsError means `add -b` was used but the branch already exists.
type BranchExistsError struct{ Branch string }

func (e *BranchExistsError) Error() string {
	return "branch already exists: " + e.Branch + " (use the existing branch or pick a new name)"
}

// ListWorktrees returns all worktrees of the repo containing dir (main
// first), via `git worktree list --porcelain -z` (03 §3.2).
func ListWorktrees(ctx context.Context, r Runner, dir string) ([]model.Worktree, error) {
	out, err := r.RunIn(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	wts, err := ParseWorktrees(out)
	if err != nil {
		return nil, err
	}
	// IsCurrent: the worktree whose path matches the launch cwd
	// (02-data-model §1.2, review m4).
	if cwd, err := os.Getwd(); err == nil {
		for i := range wts {
			if samePath(wts[i].Path, cwd) {
				wts[i].IsCurrent = true
			}
		}
	}
	return wts, nil
}

// AddOptions configures `git worktree add`.
type AddOptions struct {
	Branch      string // new branch name ("" = no -b)
	Base        string // commit-ish to base on ("" = HEAD)
	Path        string // destination directory
	Detach      bool
	GuessRemote bool
	Force       bool // -f, useful when the path is a missing-but-prunable entry
}

// AddWorktree creates a worktree at opts.Path.
//
// Branch semantics (herdr-style, 04 §3.2): an EXISTING local branch is
// checked out with `add <path> <branch>` (git refuses if checked out
// elsewhere — surfaced as BranchCheckedOutElsewhereError); a NEW branch
// uses `-b`; a branch that exists only remotely becomes a local branch
// tracking it (`-b <branch> --track <remote>/<branch>`). The UI only needs
// to pass Branch + Base + Path.
func AddWorktree(ctx context.Context, r Runner, dir string, opts AddOptions) error {
	args := []string{"worktree", "add"}
	branch := opts.Branch

	useNewBranch := branch != "" // default: -b
	trackRef := ""
	if branch != "" {
		switch {
		case branchExists(ctx, r, dir, "refs/heads/"+branch):
			useNewBranch = false // existing local branch
		case opts.GuessRemote:
			if ref := remoteBranchRef(ctx, r, dir, branch); ref != "" {
				useNewBranch = false // remote-only branch → track it
				trackRef = ref
			}
		}
	}
	if useNewBranch {
		args = append(args, "-b", branch)
	}
	if trackRef != "" {
		args = append(args, "--track", trackRef)
	}
	if opts.Detach {
		args = append(args, "--detach")
	}
	if opts.Force {
		args = append(args, "--force")
	}
	args = append(args, opts.Path)
	switch {
	case useNewBranch:
		if opts.Base != "" {
			args = append(args, opts.Base)
		}
	case trackRef == "":
		args = append(args, branch) // existing local branch, no -b
	}

	_, err := r.RunIn(ctx, dir, args...)
	if err == nil {
		return nil
	}
	stderr := err.Error()
	switch {
	case strings.Contains(stderr, "is already checked out at") ||
		strings.Contains(stderr, "already used by worktree") ||
		strings.Contains(stderr, "already checked out"):
		return &BranchCheckedOutElsewhereError{Branch: branch}
	case strings.Contains(stderr, "branch named") && strings.Contains(stderr, "already exists"):
		return &BranchExistsError{Branch: branch}
	}
	return err
}

// branchExists reports whether a ref exists (show-ref --verify).
func branchExists(ctx context.Context, r Runner, dir, ref string) bool {
	_, err := r.RunIn(ctx, dir, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

// remoteBranchRef returns the short remote-tracking ref (e.g. "origin/feat")
// matching name, or "" when no remote branch matches.
func remoteBranchRef(ctx context.Context, r Runner, dir, name string) string {
	out, err := r.RunIn(ctx, dir, "for-each-ref", "--format=%(refname:short)", "refs/remotes")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == name || strings.HasSuffix(line, "/"+name) {
			return line
		}
	}
	return ""
}

// RemoveWorktree removes the worktree at path. Two-step flow (03 §3.2):
// first without force; on refusal due to dirty/locked state, return the
// typed error so the UI offers the explicit --force confirmation. Never
// deletes branches.
func RemoveWorktree(ctx context.Context, r Runner, dir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := r.RunIn(ctx, dir, args...)
	if err == nil {
		return nil
	}
	stderr := err.Error()
	switch {
	case strings.Contains(stderr, "cannot be locked") || strings.Contains(stderr, "is locked"):
		return &WorktreeLockedError{Path: path}
	case strings.Contains(stderr, "contains modified or untracked files"):
		return &WorktreeDirtyError{Path: path}
	}
	return err
}

// LockWorktree locks the worktree at path with an optional reason.
func LockWorktree(ctx context.Context, r Runner, dir, path, reason string) error {
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	args = append(args, path)
	_, err := r.RunIn(ctx, dir, args...)
	return err
}

// UnlockWorktree unlocks the worktree at path.
func UnlockWorktree(ctx context.Context, r Runner, dir, path string) error {
	_, err := r.RunIn(ctx, dir, "worktree", "unlock", path)
	return err
}

// PruneWorktree runs `git worktree prune`. An explicit prune should remove
// immediately-prunable entries regardless of the 3-week gc expiry, so the
// real prune uses --expire now; the dry-run preview matches it
// (04 §3.2.2 / review M6).
func PruneWorktree(ctx context.Context, r Runner, dir string, dryRun bool) (string, error) {
	args := []string{"worktree", "prune"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, "--expire", "now")
	out, err := r.RunIn(ctx, dir, args...)
	return strings.TrimSpace(string(out)), err
}

// WorktreeDirty reports whether the worktree at path has changes
// (review M5: per-worktree dirty state; run with cwd = worktree path).
func WorktreeDirty(ctx context.Context, r Runner, path string) (bool, error) {
	out, err := r.RunIn(ctx, path, "status", "--porcelain=v1", "-z")
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

func isBranchCheckedOutErr(err error) bool {
	return strings.Contains(err.Error(), "is already checked out at") ||
		strings.Contains(err.Error(), "already checked out")
}

func samePath(a, b string) bool {
	absA, err1 := filepath.Abs(a)
	absB, err2 := filepath.Abs(b)
	if err1 == nil && err2 == nil {
		return absA == absB
	}
	return a == b
}
