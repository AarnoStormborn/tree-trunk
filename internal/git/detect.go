package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// DetectError is a typed error for repo-resolution failures.
type DetectError struct {
	Path string
	Err  error
}

func (e *DetectError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }
func (e *DetectError) Unwrap() error { return e.Err }

// IsNotARepo reports whether err indicates the path is not inside a git repo.
func IsNotARepo(err error) bool {
	var de *DetectError
	if errors.As(err, &de) {
		return isNotARepoMsg(de.Err.Error())
	}
	return isNotARepoMsg(err.Error())
}

func isNotARepoMsg(s string) bool {
	return strings.Contains(s, "not a git repository") ||
		strings.Contains(s, "does not appear to be a git repository")
}

// Resolve identifies the repo containing dir and returns its model.Repo with
// a canonicalized ID (docs/design/02-data-model.md §1.1, review M1).
//
// The common git dir is the identity. We request absolute paths from git
// itself (--path-format=absolute, git ≥ 2.31 — fine under our ≥ 2.38 floor)
// and then EvalSymlinks for full canonicalization, so a repo reached via its
// main worktree, a linked worktree, or a symlink all key to the same ID.
func Resolve(ctx context.Context, r Runner, dir string) (*model.Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, &DetectError{Path: dir, Err: err}
	}
	out, err := r.RunIn(ctx, abs,
		"rev-parse", "--path-format=absolute",
		"--git-common-dir", "--git-dir", "--is-bare-repository", "--show-toplevel")
	if err != nil {
		return nil, &DetectError{Path: dir, Err: err}
	}
	lines := bytes.Split(bytes.TrimSpace(out), []byte{'\n'})
	if len(lines) < 3 {
		return nil, &DetectError{Path: dir, Err: errors.New("unexpected rev-parse output")}
	}
	commonDir := string(lines[0])
	gitDir := string(lines[1])
	isBare := string(bytes.TrimSpace(lines[2])) == "true"
	toplevel := ""
	if len(lines) >= 4 && len(bytes.TrimSpace(lines[3])) > 0 {
		toplevel = string(lines[3])
	}

	id, err := Canonicalize(commonDir)
	if err != nil {
		return nil, &DetectError{Path: dir, Err: err}
	}

	repo := &model.Repo{
		ID:        id,
		GitDir:    commonDir,
		Bare:      isBare,
		Lifecycle: model.StateStale,
	}
	if toplevel != "" {
		repo.Path = toplevel
	}
	// Stable display name: the repo dir basename, regardless of which
	// worktree resolved it. For a normal repo the common dir is
	// <repo>/.git, so the name is its parent's basename; for a bare repo
	// the common dir IS the repo dir.
	if filepath.Base(commonDir) == ".git" {
		repo.Name = filepath.Base(filepath.Dir(commonDir))
	} else {
		repo.Name = filepath.Base(id)
	}
	_ = gitDir // retained for future worktree/status work; not part of identity
	return repo, nil
}

// Canonicalize produces the stable repo ID from a common git dir path:
// resolve relative against cwd (callers pass absolute already), then
// Abs + EvalSymlinks (review M1). Fails only on missing dir or symlink
// resolution errors.
func Canonicalize(commonDir string) (string, error) {
	abs, err := filepath.Abs(commonDir)
	if err != nil {
		return "", err
	}
	// EvalSymlinks requires the path to exist; a missing repo dir is a real
	// error here (discovery never emits non-existent paths).
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("repo dir does not exist: %s", abs)
		}
		return "", err
	}
	return resolved, nil
}
