package main

import (
	"strings"
	"testing"

	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

func TestClassifyWTError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&git.WorktreeDirtyError{Path: "/x"}, "worktree_dirty"},
		{&git.WorktreeLockedError{Path: "/x"}, "worktree_locked"},
		{&git.BranchCheckedOutElsewhereError{Branch: "b"}, "branch_checked_out_elsewhere"},
		{&git.BranchExistsError{Branch: "b"}, "branch_exists"},
		{git.ErrGitNotFound, "git_error"}, // unknown → git_error
	}
	for _, c := range cases {
		got := classifyWTError(c.err, "repo", "b", "/x")
		if got.Code != c.want {
			t.Errorf("classify(%T) = %q, want %q", c.err, got.Code, c.want)
		}
	}
}

func TestFindWorktree(t *testing.T) {
	repo := &model.Repo{Name: "r", Path: "/main",
		Worktrees: []model.Worktree{
			{Branch: "main", Path: "/main", IsMain: true},
			{Branch: "feat/x", Path: "/wt/feat-x"},
		}}
	if w := findWorktree(repo, "feat/x"); w == nil || w.Path != "/wt/feat-x" {
		t.Fatalf("find by branch failed: %+v", w)
	}
	if w := findWorktree(repo, "/wt/feat-x"); w == nil || w.Branch != "feat/x" {
		t.Fatalf("find by path failed: %+v", w)
	}
	if w := findWorktree(repo, "nope"); w != nil {
		t.Fatalf("find nonexistent: %+v", w)
	}
}

func TestSlugPathDefault(t *testing.T) {
	// The default create path is ~/.worktrees/<repo>/<slug>; verify slug.
	if s := git.Slug("feat/my-branch"); s != "feat-my-branch" {
		t.Fatalf("slug = %q", s)
	}
	if s := git.Slug("agentic/v2/signal-search-filter"); !strings.HasPrefix(s, "agentic-v2-signal-search-filter") {
		t.Fatalf("slug = %q", s)
	}
}

func TestInterleavedFlagParser(t *testing.T) {
	// Flags before, after, and mixed with positionals must all parse.
	rest := []string{"--repo", "/r", "rocinante", "feat/x", "--no-scan", "--force", "--path", "/p"}
	var positionals []string
	var repos []string
	noScan, force := false, false
	path := ""
	i := 0
	for i < len(rest) {
		a := rest[i]
		switch {
		case a == "--force":
			force = true
			i++
		case a == "--no-scan":
			noScan = true
			i++
		case a == "--repo" && i+1 < len(rest):
			repos = append(repos, rest[i+1])
			i += 2
		case a == "--path" && i+1 < len(rest):
			path = rest[i+1]
			i += 2
		default:
			positionals = append(positionals, a)
			i++
		}
	}
	if len(positionals) != 2 || positionals[0] != "rocinante" || positionals[1] != "feat/x" {
		t.Fatalf("positionals = %v", positionals)
	}
	if len(repos) != 1 || repos[0] != "/r" {
		t.Fatalf("repos = %v", repos)
	}
	if !noScan || !force || path != "/p" {
		t.Fatalf("flags: noScan=%v force=%v path=%q", noScan, force, path)
	}
}
