package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLogBasic(t *testing.T) {
	// Two records: hash, author, ISO date, subject (no trailing %x00).
	out := []byte("abc123\x00Alice\x002026-08-10T00:01:56+05:30\x00second commit\x0063c307b\x00Bob\x002026-08-09T10:00:00Z\x00first\x00")
	commits, err := ParseLog(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("len = %d, want 2", len(commits))
	}
	c := commits[0]
	if c.Hash != "abc123" || c.Author != "Alice" || c.Subject != "second commit" {
		t.Fatalf("commit0 = %+v", c)
	}
	if c.AuthorDate.IsZero() {
		t.Fatal("date not parsed")
	}
}

func TestParseLogEmptySubject(t *testing.T) {
	out := []byte("abc123\x00Alice\x002026-08-10T00:01:56+05:30\x00\x00")
	commits, err := ParseLog(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || commits[0].Subject != "" {
		t.Fatalf("commits = %+v", commits)
	}
}

func TestLogIntegration(t *testing.T) {
	main := newSandboxRepo(t) // 1 commit: "initial"
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	runGit(t, main, "commit", "--allow-empty", "-m", "second")

	commits, err := Log(ctx, runner, main, LogOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("len = %d, want 2", len(commits))
	}
	if commits[0].Subject != "second" || commits[1].Subject != "initial" {
		t.Fatalf("order: %q, %q", commits[0].Subject, commits[1].Subject)
	}

	// Paging: skip 1 → only "initial".
	paged, err := Log(ctx, runner, main, LogOptions{Limit: 10, Skip: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(paged) != 1 || paged[0].Subject != "initial" {
		t.Fatalf("paged = %+v", paged)
	}
}

func TestDiffModes(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	// Working-tree diff.
	if err := os.WriteFile(filepath.Join(main, "file.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Diff(ctx, runner, main, DiffOptions{Mode: DiffWorking})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "file.txt") || !strings.Contains(d, "-hello") {
		t.Fatalf("working diff missing content: %q", d[:min(120, len(d))])
	}

	// Staged diff (nothing staged yet → empty).
	d, err = Diff(ctx, runner, main, DiffOptions{Mode: DiffStaged})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(d) != "" {
		t.Fatalf("staged diff should be empty, got %q", d[:min(80, len(d))])
	}

	// Stat mode.
	runGit(t, main, "add", "file.txt")
	d, err = Diff(ctx, runner, main, DiffOptions{Mode: DiffStaged, Stat: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "file.txt") || !strings.Contains(d, "|") {
		t.Fatalf("stat diff = %q", d)
	}

	// Commit diff needs a second commit (HEAD~1 must exist).
	runGit(t, main, "commit", "-m", "second")
	head := runGit(t, main, "rev-parse", "HEAD")
	parent := runGit(t, main, "rev-parse", "HEAD~1")
	d, err = Diff(ctx, runner, main, DiffOptions{Mode: DiffCommit, Commit: head})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, parent) && !strings.Contains(d, "file.txt") {
		t.Fatalf("commit diff missing content: %q", d[:min(120, len(d))])
	}
}

func TestDiffUntrackedFile(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(main, "brand-new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Diff(ctx, runner, main, DiffOptions{Mode: DiffFileWorking, Path: "brand-new.txt"})
	var ue ErrUntrackedFile
	if !errors.As(err, &ue) {
		t.Fatalf("expected ErrUntrackedFile, got %v", err)
	}
}

func TestMainBranchDetection(t *testing.T) {
	main := newSandboxRepo(t) // init -b main
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()
	if got := MainBranch(ctx, runner, main); got != "main" {
		t.Fatalf("MainBranch = %q, want main", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestDiffRootCommit(t *testing.T) {
	main := newSandboxRepo(t) // single root commit "initial"
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	root := runGit(t, main, "rev-parse", "HEAD")
	d, err := Diff(ctx, runner, main, DiffOptions{Mode: DiffCommit, Commit: root})
	if err != nil {
		t.Fatalf("root commit diff: %v", err)
	}
	if !strings.Contains(d, "file.txt") || !strings.Contains(d, "hello") {
		t.Fatalf("root commit diff missing content: %q", d[:min(120, len(d))])
	}
}
