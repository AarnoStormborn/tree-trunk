package git

import (
	"bytes"
	"context"
	"strings"
)

// DiffMode selects which diff to produce (03-git-layer.md §3.3).
type DiffMode int

const (
	DiffWorking     DiffMode = iota // working tree vs index+HEAD
	DiffStaged                      // --cached
	DiffMain                        // main...current branch
	DiffCommit                      // commit^..commit
	DiffFileWorking                 // git diff -- <path>
	DiffFileStaged                  // git diff --cached -- <path>
)

func (m DiffMode) String() string {
	switch m {
	case DiffWorking:
		return "working tree"
	case DiffStaged:
		return "staged"
	case DiffMain:
		return "vs main"
	case DiffCommit:
		return "commit"
	case DiffFileWorking:
		return "file (working)"
	case DiffFileStaged:
		return "file (staged)"
	}
	return "diff"
}

// DiffOptions configures a diff run.
type DiffOptions struct {
	Mode   DiffMode
	Commit string // for DiffCommit: the commit hash
	Path   string // for DiffFile*: the file path
	Stat   bool   // --stat summary only
}

// Diff runs git diff and returns the (bounded) output. Large diffs are
// capped at maxDiffBytes; a truncated marker is appended.
const maxDiffBytes = 2 << 20 // 2 MiB

// Diff executes the requested diff in dir.
func Diff(ctx context.Context, r Runner, dir string, opts DiffOptions) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv"}
	if opts.Stat {
		args = append(args, "--stat")
	}
	switch opts.Mode {
	case DiffWorking:
		// plain
	case DiffStaged:
		args = append(args, "--cached")
	case DiffMain:
		main := mainBranchGuess(ctx, r, dir)
		cur := currentBranch(ctx, r, dir)
		args = append(args, main+"..."+cur)
	case DiffCommit:
		// `git show --format=` renders the commit's patch (diff vs parent,
		// or vs the empty tree for the root commit — no parent fallback
		// needed; `git diff <c>^ <c>` fails on root commits and
		// `git diff --root` is a no-op on git 2.39).
		showArgs := append([]string{"show", "--no-color", "--format="}, args[1:]...)
		out, err := r.RunIn(ctx, dir, append(showArgs, opts.Commit)...)
		if err != nil {
			return "", err
		}
		return boundedDiff(out), nil
	case DiffFileWorking:
		args = append(args, "--", opts.Path)
	case DiffFileStaged:
		args = append(args, "--cached", "--", opts.Path)
	}

	out, err := r.RunIn(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	// Untracked files have no diff — return a placeholder (review M9).
	if isFileUntracked(ctx, r, dir, opts.Path) {
		return "", ErrUntrackedFile{Path: opts.Path}
	}

	return boundedDiff(out), nil
}

// boundedDiff caps large outputs (03-git-layer.md §7).
func boundedDiff(out []byte) string {
	s := string(out)
	if len(s) > maxDiffBytes {
		s = s[:maxDiffBytes] + "\n… diff truncated (large)\n"
	}
	return s
}

// ErrUntrackedFile is returned when the requested file has no diff because
// it is untracked (review M9: "untracked file" placeholder).
type ErrUntrackedFile struct{ Path string }

func (e ErrUntrackedFile) Error() string { return "untracked file: " + e.Path }

func isFileUntracked(ctx context.Context, r Runner, dir, path string) bool {
	if path == "" {
		return false
	}
	out, err := r.RunIn(ctx, dir, "status", "--porcelain=v1", "-z", "--", path)
	if err != nil {
		return false
	}
	// First field of the first record starts with "??" for untracked.
	fields := bytes.Split(out, []byte{0})
	return len(fields) > 0 && len(fields[0]) >= 3 && fields[0][0] == '?' && fields[0][1] == '?'
}

// MainBranch resolves the repo's main active branch (Q6): origin/HEAD
// symbolic-ref, else refs/heads/main, else refs/heads/master.
func MainBranch(ctx context.Context, r Runner, dir string) string {
	if b := mainBranchGuess(ctx, r, dir); b != "" {
		return b
	}
	return "main"
}

func mainBranchGuess(ctx context.Context, r Runner, dir string) string {
	for _, ref := range []string{
		"refs/remotes/origin/HEAD",
		"refs/heads/main",
		"refs/heads/master",
	} {
		out, err := r.RunIn(ctx, dir, "symbolic-ref", "--quiet", "--short", ref)
		if err == nil {
			if b := strings.TrimSpace(string(out)); b != "" {
				return b
			}
		}
	}
	return ""
}

func currentBranch(ctx context.Context, r Runner, dir string) string {
	out, err := r.RunIn(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "HEAD"
	}
	if b := strings.TrimSpace(string(out)); b != "" {
		return b
	}
	return "HEAD"
}
