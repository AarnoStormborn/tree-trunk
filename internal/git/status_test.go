package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

func TestParseStatusBasic(t *testing.T) {
	out := []byte("## main...origin/main\x00 M file.txt\x00?? new.txt\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.Upstream != "origin/main" {
		t.Fatalf("branch=%q upstream=%q", st.Branch, st.Upstream)
	}
	if st.Unstaged != 1 || st.Untracked != 1 || st.Staged != 0 {
		t.Fatalf("counts: staged=%d unstaged=%d untracked=%d", st.Staged, st.Unstaged, st.Untracked)
	}
	if len(st.Files) != 2 {
		t.Fatalf("files = %d", len(st.Files))
	}
	if st.Files[0].Path != "file.txt" || st.Files[0].X != ' ' || st.Files[0].Y != 'M' {
		t.Fatalf("file0 = %+v", st.Files[0])
	}
	if st.Files[1].Path != "new.txt" || !st.Files[1].Untracked() {
		t.Fatalf("file1 = %+v", st.Files[1])
	}
}

func TestParseStatusAheadBehind(t *testing.T) {
	out := []byte("## dev...origin/dev [ahead 2, behind 1]\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("ahead=%d behind=%d", st.Ahead, st.Behind)
	}
}

func TestParseStatusAheadOnly(t *testing.T) {
	out := []byte("## dev...origin/dev [ahead 3]\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead != 3 || st.Behind != 0 {
		t.Fatalf("ahead=%d behind=%d", st.Ahead, st.Behind)
	}
}

func TestParseStatusNoUpstream(t *testing.T) {
	out := []byte("## main\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.Upstream != "" {
		t.Fatalf("branch=%q upstream=%q", st.Branch, st.Upstream)
	}
}

func TestParseStatusDetached(t *testing.T) {
	out := []byte("## HEAD (no branch)\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "HEAD" {
		t.Fatalf("branch=%q", st.Branch)
	}
}

func TestParseStatusConflicts(t *testing.T) {
	out := []byte("## main\x00UU conflicted.txt\x00AA both-added.txt\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Conflicts != 2 {
		t.Fatalf("conflicts = %d, want 2 (files: %+v)", st.Conflicts, st.Files)
	}
}

func TestParseStatusRename(t *testing.T) {
	// porcelain v1 -z rename record: "R  newname\0oldname\0"
	out := []byte("## main\x00R  new.txt\x00old.txt\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Files) != 1 {
		t.Fatalf("files = %d, want 1 (rename consumes two NUL fields)", len(st.Files))
	}
	f := st.Files[0]
	if f.Path != "new.txt" || f.OrigPath != "old.txt" {
		t.Fatalf("rename file = %+v", f)
	}
	if !f.Staged() {
		t.Fatal("rename should be staged")
	}
	if st.Staged != 1 {
		t.Fatalf("staged = %d", st.Staged)
	}
}

func TestParseStatusStagedAndUnstaged(t *testing.T) {
	out := []byte("## main\x00MM dual.txt\x00M  staged.txt\x00")
	st, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Staged != 2 || st.Unstaged != 1 {
		t.Fatalf("staged=%d unstaged=%d", st.Staged, st.Unstaged)
	}
}

func TestParseStatusEmpty(t *testing.T) {
	st, err := ParseStatus([]byte("## main...origin/main\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Dirty() {
		t.Fatal("clean repo must not be dirty")
	}
	if st.Summary() != "" {
		t.Fatalf("summary = %q", st.Summary())
	}
}

// TestStatusIntegration runs real git against a sandbox repo with staged,
// unstaged, untracked and conflicted states.
func TestStatusIntegration(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	// Untracked + modified.
	if err := os.WriteFile(filepath.Join(main, "new.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "file.txt"), []byte("hello2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Status(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if st.Unstaged != 1 || st.Untracked != 1 {
		t.Fatalf("unstaged=%d untracked=%d", st.Unstaged, st.Untracked)
	}
	if st.Branch != "main" {
		t.Fatalf("branch=%q", st.Branch)
	}

	// Stage one file.
	runGit(t, main, "add", "file.txt")
	st, err = Status(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if st.Staged != 1 || st.Unstaged != 0 {
		t.Fatalf("after add: staged=%d unstaged=%d", st.Staged, st.Unstaged)
	}
}

// TestFingerprintChangesOnRefOnly verifies the review-M2/M3 semantics: the
// fingerprint must NOT change when files are modified, and MUST change when
// a commit lands.
func TestFingerprintChangesOnRefOnly(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	fp1, err := Fingerprint(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}

	// Modify a file + add untracked: refs unchanged → same fingerprint.
	if err := os.WriteFile(filepath.Join(main, "file.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "u.txt"), []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp2, err := Fingerprint(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatal("fingerprint changed on working-tree edits — must track refs only")
	}

	// Commit: refs changed → different fingerprint.
	runGit(t, main, "add", ".")
	runGit(t, main, "commit", "-m", "second")
	fp3, err := Fingerprint(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp3 {
		t.Fatal("fingerprint unchanged after commit — must track HEAD")
	}
}

// TestFingerprintTracksDetachedHEAD verifies detached-HEAD moves change the
// fingerprint even though no ref moves (review M3).
func TestFingerprintTracksDetachedHEAD(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	// Create a second commit + detach HEAD onto the first.
	runGit(t, main, "commit", "--allow-empty", "-m", "second")
	first := runGit(t, main, "rev-parse", "HEAD~1")

	fp1, err := Fingerprint(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "checkout", "--detach", first)
	fp2, err := Fingerprint(ctx, runner, main)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Fatal("fingerprint must change on detached-HEAD move")
	}
}

func TestStatusSummaryStillWorks(t *testing.T) {
	st := &model.RepoStatus{Staged: 1, Untracked: 2}
	if got := st.Summary(); got != " *1 +2" {
		t.Fatalf("summary = %q", got)
	}
	if !st.Dirty() {
		t.Fatal("should be dirty")
	}
}

var _ = strings.TrimSpace // keep strings import if unused paths change
