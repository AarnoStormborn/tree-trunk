package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// CommandError is a failed git invocation with its exit code and stderr.
type CommandError struct {
	Args   []string
	Stderr string
	Code   int
}

func (e *CommandError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.Code)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// MinGitVersion is the minimum supported git version (D1/Q2: ≥ 2.38).
var MinGitVersion = [2]int{2, 38}

// ErrGitNotFound is returned when no git binary is on PATH.
var ErrGitNotFound = errors.New("git not found on PATH (install git ≥ 2.38)")

// ErrGitTooOld is returned when the found git is older than MinGitVersion.
type ErrGitTooOld struct {
	Found [3]int
}

func (e *ErrGitTooOld) Error() string {
	return fmt.Sprintf("git %d.%d.%d found; tree-trunk requires ≥ %d.%d",
		e.Found[0], e.Found[1], e.Found[2], MinGitVersion[0], MinGitVersion[1])
}

var versionRe = regexp.MustCompile(`^git version (\d+)\.(\d+)(?:\.(\d+))?`)

// LookPath finds the git binary on PATH.
func LookPath() (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", ErrGitNotFound
	}
	return path, nil
}

// CheckVersion verifies the git binary meets MinGitVersion. It returns the
// parsed [major, minor, patch] version.
func CheckVersion(ctx context.Context, gitPath string) ([3]int, error) {
	out, err := exec.CommandContext(ctx, gitPath, "--version").Output()
	if err != nil {
		return [3]int{}, fmt.Errorf("git --version: %w", err)
	}
	m := versionRe.FindSubmatch(out)
	if m == nil {
		return [3]int{}, fmt.Errorf("unrecognized git version output %q", strings.TrimSpace(string(out)))
	}
	var v [3]int
	v[0], _ = strconv.Atoi(string(m[1]))
	v[1], _ = strconv.Atoi(string(m[2]))
	if len(m) > 3 && len(m[3]) > 0 {
		v[2], _ = strconv.Atoi(string(m[3]))
	}
	if v[0] < MinGitVersion[0] || (v[0] == MinGitVersion[0] && v[1] < MinGitVersion[1]) {
		return v, &ErrGitTooOld{Found: v}
	}
	return v, nil
}
