package git

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
)

// failRunner returns a fixed error for worktree add (no real git needed).
type failRunner struct {
	stderr    string
	addCalls  atomic.Int32
	showRefOk atomic.Bool // branchExists result
}

func (f *failRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return nil, errors.New("n/a")
}
func (f *failRunner) RunIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "show-ref" {
		if f.showRefOk.Load() {
			return nil, nil // branch exists locally → plain add path
		}
		return nil, errors.New("not found")
	}
	if len(args) > 0 && args[0] == "worktree" && args[1] == "add" {
		f.addCalls.Add(1)
		return nil, &CommandError{Args: args, Stderr: f.stderr, Code: 128}
	}
	return nil, errors.New("n/a")
}
func (f *failRunner) RunStream(ctx context.Context, w io.Writer, args ...string) error {
	return errors.New("n/a")
}
func (f *failRunner) RunPaged(ctx context.Context, args []string, onLines func(line []byte) error) error {
	return errors.New("n/a")
}

// TestGuardMatchesGitVersionMessages covers the checked-out-elsewhere
// messages across git versions (2.39 "is already checked out at", 2.47
// "already used by worktree" — CI caught the 2.47 wording).
func TestGuardMatchesGitVersionMessages(t *testing.T) {
	for _, stderr := range []string{
		"Preparing worktree (checking out 'feat')\nfatal: 'feat' is already checked out at '/wt/feat'",
		"Preparing worktree (checking out 'feat')\nfatal: 'feat' is already used by worktree at '/wt/feat'",
	} {
		r := &failRunner{stderr: stderr, showRefOk: *newAtomicBool(true)}
		var bce *BranchCheckedOutElsewhereError
		err := AddWorktree(context.Background(), r, "/repo", AddOptions{Branch: "feat", Path: "/dup"})
		if !errors.As(err, &bce) {
			t.Fatalf("stderr %q: got %v, want BranchCheckedOutElsewhereError", stderr, err)
		}
		if bce.Branch != "feat" {
			t.Fatalf("branch = %q", bce.Branch)
		}
	}
}

// TestGuardBranchExistsError covers "a branch named ... already exists".
func TestGuardBranchExistsError(t *testing.T) {
	r := &failRunner{stderr: "fatal: a branch named 'feat' already exists", showRefOk: *newAtomicBool(false)}
	var be *BranchExistsError
	err := AddWorktree(context.Background(), r, "/repo", AddOptions{Branch: "feat", Path: "/dup"})
	if !errors.As(err, &be) {
		t.Fatalf("got %v, want BranchExistsError", err)
	}
}

func newAtomicBool(v bool) *atomic.Bool {
	b := &atomic.Bool{}
	b.Store(v)
	return b
}
