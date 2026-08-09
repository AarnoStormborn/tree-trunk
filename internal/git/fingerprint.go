package git

import (
	"bytes"
	"context"
)

// Fingerprint computes the refs+HEAD fingerprint used for refresh dedup
// (docs/design/01-architecture.md §3.3, review M3): `for-each-ref` over
// local+remote branches PLUS an explicit `rev-parse --verify HEAD` (HEAD is
// NOT a ref pattern — a detached-HEAD move changes no ref).
//
// The fingerprint gates ref-dependent reads (branch list, log cache); the
// cheap `git status` re-reads on every poll regardless.
func Fingerprint(ctx context.Context, r Runner, dir string) (string, error) {
	refs, err := r.RunIn(ctx, dir,
		"for-each-ref", "--format=%(objectname)", "refs/heads", "refs/remotes")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	buf.Write(refs)
	buf.WriteByte(0)
	head, err := r.RunIn(ctx, dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		// Unborn HEAD (fresh repo, no commits): not an error for the
		// fingerprint; refs still compare.
		return buf.String(), nil
	}
	buf.Write(bytes.TrimSpace(head))
	return buf.String(), nil
}
