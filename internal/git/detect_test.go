package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitAvailable returns the git binary path, skipping the test if git is
// missing or too old for worktrees.
func gitAvailable(t *testing.T) string {
	t.Helper()
	path, err := LookPath()
	if err != nil {
		t.Skip("git not available")
	}
	if _, err := CheckVersion(context.Background(), path); err != nil {
		t.Skipf("git too old: %v", err)
	}
	return path
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newSandboxRepo creates a repo with one commit and returns its main path.
// A repo-local identity is configured because CI runners have no global
// git user.name/email (commits would fail with exit 128).
func newSandboxRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main", ".")
	runGit(t, dir, "config", "user.name", "tree-trunk test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func TestResolveMainWorktree(t *testing.T) {
	main := newSandboxRepo(t)
	resolvedMain, err := filepath.EvalSymlinks(main)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewExecRunner(gitAvailable(t))

	repo, err := Resolve(context.Background(), runner, main)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if repo.Name != filepath.Base(resolvedMain) {
		t.Fatalf("Name = %q, want %q", repo.Name, filepath.Base(resolvedMain))
	}
	if repo.Bare {
		t.Fatal("expected non-bare")
	}
	if repo.Path != resolvedMain {
		t.Fatalf("Path = %q, want %q", repo.Path, resolvedMain)
	}
	if repo.ID == "" {
		t.Fatal("empty ID")
	}
}

// TestResolveMainVsLinkedSameID is the review-M1 fixture: a repo discovered
// via its main worktree and via a linked worktree must yield identical IDs.
func TestResolveMainVsLinkedSameID(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	// Create a linked worktree outside the repo dir (sibling layout).
	base := filepath.Dir(main)
	wt := filepath.Join(base, filepath.Base(main)+"-feat")
	runGit(t, main, "worktree", "add", "-b", "feat", wt)

	mainRepo, err := Resolve(ctx, runner, main)
	if err != nil {
		t.Fatalf("Resolve(main): %v", err)
	}
	wtRepo, err := Resolve(ctx, runner, wt)
	if err != nil {
		t.Fatalf("Resolve(linked): %v", err)
	}
	if mainRepo.ID != wtRepo.ID {
		t.Fatalf("IDs differ: main=%q linked=%q", mainRepo.ID, wtRepo.ID)
	}
	if wtRepo.Path == mainRepo.Path {
		t.Fatalf("linked Path should differ from main Path: %q", wtRepo.Path)
	}
}

func TestResolveCanonicalizesSymlinks(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	ctx := context.Background()

	link := filepath.Join(filepath.Dir(main), "alias-"+filepath.Base(main))
	if err := os.Symlink(main, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	defer os.Remove(link)

	direct, err := Resolve(ctx, runner, main)
	if err != nil {
		t.Fatalf("Resolve(direct): %v", err)
	}
	viaLink, err := Resolve(ctx, runner, link)
	if err != nil {
		t.Fatalf("Resolve(via symlink): %v", err)
	}
	if direct.ID != viaLink.ID {
		t.Fatalf("IDs differ after symlink: direct=%q via=%q", direct.ID, viaLink.ID)
	}
}

func TestResolveNotARepo(t *testing.T) {
	dir := t.TempDir()
	runner := NewExecRunner(gitAvailable(t))
	_, err := Resolve(context.Background(), runner, dir)
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
	if !IsNotARepo(err) {
		t.Fatalf("expected NotARepo, got %v", err)
	}
}

func TestCheckVersion(t *testing.T) {
	path := gitAvailable(t)
	v, err := CheckVersion(context.Background(), path)
	if err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}
	if v[0] < MinGitVersion[0] || (v[0] == MinGitVersion[0] && v[1] < MinGitVersion[1]) {
		t.Fatalf("version %v below floor %v", v, MinGitVersion)
	}
}

// Lock-retry behavior is unit-tested in M1 with a fake git script that
// fails with "index.lock" N times then succeeds (03-git-layer.md §2).
func TestRunnerBasic(t *testing.T) {
	main := newSandboxRepo(t)
	runner := NewExecRunner(gitAvailable(t))
	out, err := runner.RunIn(context.Background(), main, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("RunIn: %v", err)
	}
	if strings.TrimSpace(string(out)) != "main" {
		t.Fatalf("branch = %q, want main", strings.TrimSpace(string(out)))
	}
}
