package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecRunner runs the system git binary via os/exec with cancellation,
// lock-retry, and GIT_OPTIONAL_LOCKS=0 for read-only commands.
// (docs/design/03-git-layer.md §2.)
type ExecRunner struct {
	gitPath string
	// lockRetries and lockBackoff implement the index.lock retry policy:
	// initial 20ms, doubled, up to 7 attempts (≈1s total). Ported from
	// lazygit's gitCmdObjRunner (research track 4 §1.3).
	lockRetries int
	lockBackoff time.Duration
}

// NewExecRunner returns a Runner bound to the git binary at gitPath.
func NewExecRunner(gitPath string) *ExecRunner {
	return &ExecRunner{
		gitPath:     gitPath,
		lockRetries: 7,
		lockBackoff: 20 * time.Millisecond,
	}
}

// LockError reports that a git command failed because of index/ref locking.
type LockError struct {
	Stderr string
}

func (e *LockError) Error() string {
	return "git lock contention: " + e.Stderr
}

func isLockErr(stderr string) bool {
	return strings.Contains(stderr, "index.lock") ||
		strings.Contains(stderr, "cannot lock ref")
}

func (r *ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return r.run(ctx, "", args)
}

func (r *ExecRunner) RunIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return r.run(ctx, dir, args)
}

func (r *ExecRunner) run(ctx context.Context, dir string, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	err := r.execWithRetry(ctx, dir, args, &stdout, &stderr)
	if err != nil {
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

func (r *ExecRunner) RunStream(ctx context.Context, w io.Writer, args ...string) error {
	cmd := r.command(ctx, "", args)
	cmd.Stdout = w
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return wrapRunErr(args, stderr.String(), err)
	}
	return nil
}

func (r *ExecRunner) RunPaged(ctx context.Context, args []string, onLines func(line []byte) error) error {
	cmd := r.command(ctx, "", args)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return wrapRunErr(args, stderr.String(), err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if err := onLines(sc.Bytes()); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
	}
	werr := cmd.Wait()
	if werr != nil {
		return wrapRunErr(args, stderr.String(), werr)
	}
	return sc.Err()
}

// execWithRetry runs the command, retrying on index.lock / cannot lock ref
// with exponential backoff.
func (r *ExecRunner) execWithRetry(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) error {
	backoff := r.lockBackoff
	for attempt := 0; ; attempt++ {
		cmd := r.command(ctx, dir, args)
		cmd.Stdout = stdout
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf

		err := cmd.Run()
		if err == nil {
			return nil
		}
		if !isLockErr(errBuf.String()) {
			return wrapRunErr(args, errBuf.String(), err)
		}
		if attempt >= r.lockRetries {
			return &LockError{Stderr: errBuf.String()}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

func (r *ExecRunner) command(ctx context.Context, dir string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.gitPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Read-only commands must not create lock files at all.
	if !isMutating(args) {
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	}
	// WaitDelay ensures a killed process cannot linger (Go ≥ 1.20).
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

// isMutating reports whether the command may write to the repo. Read-only
// commands get GIT_OPTIONAL_LOCKS=0.
func isMutating(args []string) bool {
	for _, a := range args {
		switch a {
		case "add", "worktree", "branch", "commit", "reset", "checkout",
			"restore", "rebase", "merge", "cherry-pick", "fetch", "pull",
			"push", "gc", "prune", "tag", "stash", "rm", "mv", "clean",
			"update-ref", "remote", "config", "lock", "unlock":
			return true
		}
	}
	return false
}

func wrapRunErr(args []string, stderr string, err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &CommandError{
			Args:   args,
			Stderr: strings.TrimSpace(stderr),
			Code:   exitErr.ExitCode(),
		}
	}
	return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}
