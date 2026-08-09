package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestWorktreePath(t *testing.T) {
	got := suggestWorktreePath("/wt-root", "myrepo", "feature/x")
	want := filepath.Join("/wt-root", "myrepo", "feature-x")
	if got != want {
		t.Fatalf("suggest = %q, want %q", got, want)
	}
	// Empty slug fallback.
	if got := suggestWorktreePath("/wt", "r", ""); got != filepath.Join("/wt", "r", "worktree") {
		t.Fatalf("empty slug fallback = %q", got)
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "feat")
	// Non-existent → as-is.
	if got := uniquePath(base); got != base {
		t.Fatalf("unique = %q, want %q", got, base)
	}
	// Existing → -2, then -3.
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(base); got != base+"-2" {
		t.Fatalf("unique = %q, want %s", got, base+"-2")
	}
}

func TestWtErrorText(t *testing.T) {
	// Typed errors render their message; generic CommandError shows the
	// stderr tail only.
	msg := wtErrorText(genericErr("git status: fatal: bad config"))
	if msg != "fatal: bad config" {
		t.Fatalf("generic err text = %q", msg)
	}
}

type genericErr string

func (e genericErr) Error() string { return string(e) }
