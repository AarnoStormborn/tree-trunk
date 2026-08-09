// Package model defines the domain types for tree-trunk.
// See docs/design/02-data-model.md.
package model

import "time"

// RefreshState describes the lifecycle of a repo's data freshness
// (docs/design/01-architecture.md §3.2).
type RefreshState string

const (
	StateStale      RefreshState = "stale"
	StateRefreshing RefreshState = "refreshing"
	StateFresh      RefreshState = "fresh"
	StateError      RefreshState = "error"
)

// Repo is a repository as a collection of worktrees sharing one object
// store. Identity is the canonicalized common git dir (Abs + EvalSymlinks)
// — see docs/design/02-data-model.md §1.1.
type Repo struct {
	ID        string // canonical common git dir (stable key)
	Name      string // basename of common dir / display name
	Path      string // main worktree path ("" for bare)
	GitDir    string // raw common git dir as reported by git
	Bare      bool
	Worktrees []Worktree // main first, then linked
	Branch    string     // main worktree branch; "HEAD" when detached
	Status    RepoStatus // aggregated from main worktree status
	RefState  string     // fingerprint for refresh dedup
	Lifecycle RefreshState
	LastError error
}

// Worktree is one working tree attached to a Repo.
// Source of truth: `git worktree list --porcelain -z`
// (docs/design/03-git-layer.md §4.3).
type Worktree struct {
	Path          string // absolute working directory
	GitDir        string // <.git dir>/worktrees/<name> for linked; common dir for main
	Branch        string // short branch name, "" when detached
	IsMain        bool
	IsCurrent     bool // path matches the cwd tree-trunk was launched from
	Locked        bool
	LockReason    string
	Prunable      bool
	Head          string // full 40-hex commit hash (shorten at render time)
	Dirty         bool   // modified/untracked/conflicted files present
	IsPathMissing bool   // prunable reason is "gitdir file points to non-existent location"
}

// Branch is one local branch, with upstream tracking when present.
type Branch struct {
	Name          string
	Head          string // commit hash
	Upstream      string // "origin/main" or ""
	Ahead, Behind int
	IsCurrent     bool
	IsCheckedOut  bool // by ANY worktree (blocks non-detached add)
}

// Commit is one log row (docs/design/03-git-layer.md §4.2).
type Commit struct {
	Hash       string // short hash
	Author     string
	AuthorDate time.Time
	Subject    string
}

// RepoStatus is the aggregated working-tree state of one worktree
// (docs/design/02-data-model.md §1.5).
type RepoStatus struct {
	Staged    int // count of staged files
	Unstaged  int // count of modified files (not untracked)
	Untracked int
	Conflicts int
	// Ahead/Behind vs upstream, from the branch of the worktree this status
	// belongs to.
	Ahead, Behind int
}

// Dirty reports whether the status has any changes.
func (s RepoStatus) Dirty() bool {
	return s.Staged > 0 || s.Unstaged > 0 || s.Untracked > 0 || s.Conflicts > 0
}

// Summary renders the compact status glyphs for a repo-list row
// (docs/design/02-data-model.md §3): conflicts → staged → unstaged →
// untracked, nonzero segments only.
func (s RepoStatus) Summary() string {
	var b []byte
	if s.Conflicts > 0 {
		b = append(b, []byte("!")...)
		b = appendInt(b, s.Conflicts)
	}
	if s.Staged > 0 {
		b = append(b, []byte(" *")...)
		b = appendInt(b, s.Staged)
	}
	if s.Unstaged > 0 {
		b = append(b, []byte(" ~")...)
		b = appendInt(b, s.Unstaged)
	}
	if s.Untracked > 0 {
		b = append(b, []byte(" +")...)
		b = appendInt(b, s.Untracked)
	}
	return string(b)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, tmp[i:]...)
}
