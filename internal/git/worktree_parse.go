package git

import (
	"bytes"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// ParseWorktrees parses `git worktree list --porcelain -z` output.
// Byte-level spec (docs/design/03-git-layer.md §4.3):
//
//	worktree <path>\0
//	HEAD <40-hex-sha>\0
//	branch refs/heads/<name>\0     // present when on a branch
//	detached\0                     // REPLACES the branch line when detached
//	locked <reason>\0              // only when locked; empty reason allowed
//	prunable <reason>\0            // only when prunable; empty reason allowed
//	\0                             // extra NUL: record separator
//
// Records are NUL-terminated fields; the FIRST record is the main worktree.
func ParseWorktrees(out []byte) ([]model.Worktree, error) {
	fields := bytes.Split(out, []byte{0})

	var worktrees []model.Worktree
	var cur *model.Worktree
	flush := func() {
		if cur != nil {
			worktrees = append(worktrees, *cur)
			cur = nil
		}
	}

	for _, f := range fields {
		switch {
		case len(f) == 0:
			flush() // record separator
		case bytes.HasPrefix(f, []byte("worktree ")):
			flush()
			cur = &model.Worktree{Path: string(f[len("worktree "):]), IsMain: len(worktrees) == 0}
		case cur == nil:
			// Field outside a record (should not happen): ignore.
		case bytes.HasPrefix(f, []byte("HEAD ")):
			cur.Head = string(f[len("HEAD "):])
		case bytes.HasPrefix(f, []byte("branch refs/heads/")):
			cur.Branch = string(f[len("branch refs/heads/"):])
		case bytes.Equal(f, []byte("detached")):
			cur.Branch = "" // detached
		case bytes.HasPrefix(f, []byte("locked")):
			cur.Locked = true
			cur.LockReason = stringsTrimPrefix(f, "locked")
		case bytes.HasPrefix(f, []byte("prunable")):
			cur.Prunable = true
			reason := stringsTrimPrefix(f, "prunable")
			// IsPathMissing: exact reason match (02-data-model §1.2, review m12).
			cur.IsPathMissing = reason == "gitdir file points to non-existent location"
		}
	}
	flush()
	return worktrees, nil
}

// stringsTrimPrefix trims the key prefix plus one leading space if present.
func stringsTrimPrefix(f []byte, key string) string {
	s := string(f)
	if len(s) > len(key) {
		s = s[len(key):]
		if len(s) > 0 && s[0] == ' ' {
			s = s[1:]
		}
		return s
	}
	return ""
}
