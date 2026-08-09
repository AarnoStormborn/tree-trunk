package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWorktreesBasic(t *testing.T) {
	out := []byte("worktree /repo/main\x00HEAD 1111111111111111111111111111111111111111\x00branch refs/heads/main\x00\x00worktree /wt/feat\x00HEAD 2222222222222222222222222222222222222222\x00branch refs/heads/feat\x00locked\x00\x00")
	wts, err := ParseWorktrees(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("len = %d, want 2", len(wts))
	}
	main, feat := wts[0], wts[1]
	if !main.IsMain || main.Path != "/repo/main" || main.Branch != "main" {
		t.Fatalf("main = %+v", main)
	}
	if main.Head != "1111111111111111111111111111111111111111" {
		t.Fatalf("main head = %q (full 40-hex)", main.Head)
	}
	if feat.IsMain || feat.Branch != "feat" {
		t.Fatalf("feat = %+v", feat)
	}
	if !feat.Locked || feat.LockReason != "" {
		t.Fatalf("locked with empty reason: %+v", feat)
	}
}

func TestParseWorktreesDetached(t *testing.T) {
	out := []byte("worktree /wt/det\x00HEAD 3333333333333333333333333333333333333333\x00detached\x00prunable gitdir file points to non-existent location\x00\x00")
	wts, err := ParseWorktrees(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 1 {
		t.Fatalf("len = %d", len(wts))
	}
	w := wts[0]
	if w.Branch != "" {
		t.Fatalf("detached worktree branch = %q, want empty", w.Branch)
	}
	if !w.Prunable || !w.IsPathMissing {
		t.Fatalf("prunable = %v, IsPathMissing = %v — want true/true", w.Prunable, w.IsPathMissing)
	}
}

func TestParseWorktreesLockedWithReason(t *testing.T) {
	out := []byte("worktree /wt/x\x00HEAD 4444444444444444444444444444444444444444\x00branch refs/heads/x\x00locked on a portable drive\x00\x00")
	wts, err := ParseWorktrees(out)
	if err != nil {
		t.Fatal(err)
	}
	if !wts[0].Locked || wts[0].LockReason != "on a portable drive" {
		t.Fatalf("locked = %+v", wts[0])
	}
}

func TestSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"feature/x", "feature-x"},
		{"Feature/My-Thing", "feature-my-thing"},
		{"feat(a)/x", "feata-x"},
		{"..", ""},
		{"-", ""},
		{"main", "main"},
	}
	for _, tc := range cases {
		if got := Slug(tc.in); got != tc.want {
			t.Fatalf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugLong(t *testing.T) {
	long := strings.Repeat("branch-name/", 10) // ~120 chars
	if got := Slug(long); len(got) > 80 {
		t.Fatalf("slug length %d exceeds 80", len(got))
	}
}

// --- integration (real git) ---

func TestWorktreeLifecycle(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	// List: main only.
	wts, err := ListWorktrees(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 1 || !wts[0].IsMain || wts[0].Branch != "main" {
		t.Fatalf("initial worktrees = %+v", wts)
	}

	// Add a worktree (new branch).
	wtPath := filepath.Join(filepath.Dir(main), filepath.Base(main)+"-feat")
	if err := AddWorktree(ctx, runner, main, AddOptions{Branch: "feat", Path: wtPath}); err != nil {
		t.Fatal(err)
	}
	wts, err = ListWorktrees(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 || wts[1].Branch != "feat" {
		t.Fatalf("after add = %+v", wts)
	}
	if !dirExists(t, wtPath) {
		t.Fatal("worktree dir missing")
	}

	// Guard: same branch checked out elsewhere → typed error.
	err = AddWorktree(ctx, runner, main, AddOptions{Branch: "feat", Path: filepath.Join(filepath.Dir(main), "dup")})
	var bce *BranchCheckedOutElsewhereError
	if !errors.As(err, &bce) {
		t.Fatalf("expected BranchCheckedOutElsewhereError, got %v", err)
	}

	// Dirty worktree → typed error, then force remove works.
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = RemoveWorktree(ctx, runner, main, wtPath, false)
	var wd *WorktreeDirtyError
	if !errors.As(err, &wd) {
		t.Fatalf("expected WorktreeDirtyError, got %v", err)
	}
	if err := RemoveWorktree(ctx, runner, main, wtPath, true); err != nil {
		t.Fatalf("force remove: %v", err)
	}
	if dirExists(t, wtPath) {
		t.Fatal("worktree dir still exists after force remove")
	}

	// Lock / unlock.
	wtPath2 := filepath.Join(filepath.Dir(main), filepath.Base(main)+"-locked")
	if err := AddWorktree(ctx, runner, main, AddOptions{Branch: "locked", Path: wtPath2}); err != nil {
		t.Fatal(err)
	}
	if err := LockWorktree(ctx, runner, main, wtPath2, "portable drive"); err != nil {
		t.Fatal(err)
	}
	wts, _ = ListWorktrees(ctx, runner, main)
	if !wts[1].Locked || wts[1].LockReason != "portable drive" {
		t.Fatalf("locked worktree = %+v", wts[1])
	}
	if err := UnlockWorktree(ctx, runner, main, wtPath2); err != nil {
		t.Fatal(err)
	}

	// Prune dry-run must be harmless.
	if _, err := PruneWorktree(ctx, runner, main, true); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeDirty(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	clean, err := WorktreeDirty(ctx, runner, main)
	if err != nil || clean {
		t.Fatalf("clean worktree: dirty=%v err=%v", clean, err)
	}
	if err := os.WriteFile(filepath.Join(main, "mod.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := WorktreeDirty(ctx, runner, main)
	if err != nil || !dirty {
		t.Fatalf("dirty worktree: dirty=%v err=%v", dirty, err)
	}
}

func TestSlugCollisionSuffix(t *testing.T) {
	// The slug itself is collision-free by construction; the caller appends
	// -2/-3 when the path exists. Assert the base slug stays stable.
	if got := Slug("feat/a"); got != "feat-a" {
		t.Fatalf("got %q", got)
	}
}

func dirExists(t *testing.T, path string) bool {
	t.Helper()
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
