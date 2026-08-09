// Package git is the ONLY place tree-trunk shells out to the git binary.
// See docs/design/03-git-layer.md.
package git

import (
	"context"
	"io"
)

// Runner is the single git access point. The UI/state layers depend on this
// interface, not on exec — tests can swap a fake or a sandboxed real git
// (docs/design/03-git-layer.md §2).
type Runner interface {
	// Run executes git with args, capturing stdout. stderr is captured and
	// attached to the returned error when the command fails.
	Run(ctx context.Context, args ...string) ([]byte, error)
	// RunIn executes git with cwd set to dir.
	RunIn(ctx context.Context, dir string, args ...string) ([]byte, error)
	// RunStream executes git and streams stdout to w.
	RunStream(ctx context.Context, w io.Writer, args ...string) error
	// RunPaged executes git and invokes onLines for each line of stdout.
	RunPaged(ctx context.Context, args []string, onLines func(line []byte) error) error
}
