// Package model defines the domain types for tree-trunk.
// See docs/design/02-data-model.md.
package model

import (
	"encoding/json"
	"time"
)

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
	ID        string       `json:"id"`             // canonical common git dir (stable key)
	Name      string       `json:"name"`           // basename of common dir / display name
	Path      string       `json:"path,omitempty"` // main worktree path ("" for bare)
	GitDir    string       `json:"git_dir"`        // raw common git dir as reported by git
	Bare      bool         `json:"bare"`
	Worktrees []Worktree   `json:"worktrees,omitempty"` // main first, then linked
	Branch    string       `json:"branch,omitempty"`    // main worktree branch; "HEAD" when detached
	Status    RepoStatus   `json:"status"`              // aggregated from main worktree status
	RefState  string       `json:"-"`                   // fingerprint for refresh dedup
	Lifecycle RefreshState `json:"-"`                   // not part of the stable agent contract
	LastError error        `json:"-"`                   // not JSON-serializable
}

// Worktree is one working tree attached to a Repo.
// Source of truth: `git worktree list --porcelain -z`
// (docs/design/03-git-layer.md §4.3).
type Worktree struct {
	Path          string `json:"path"`             // absolute working directory
	GitDir        string `json:"git_dir"`          // <.git dir>/worktrees/<name> for linked; common dir for main
	Branch        string `json:"branch,omitempty"` // short branch name, "" when detached
	IsMain        bool   `json:"is_main"`
	IsCurrent     bool   `json:"is_current"` // path matches the cwd tree-trunk was launched from
	Locked        bool   `json:"locked"`
	LockReason    string `json:"lock_reason,omitempty"`
	Prunable      bool   `json:"prunable"`
	Head          string `json:"head,omitempty"`            // full 40-hex commit hash (shorten at render time)
	Dirty         bool   `json:"dirty"`                     // modified/untracked/conflicted files present
	IsPathMissing bool   `json:"is_path_missing,omitempty"` // prunable reason is "gitdir file points to non-existent location"
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

// StatusFile is one entry from `git status --porcelain=v1 -z`
// (docs/design/03-git-layer.md §4.1).
// StatusFile is one file entry from `git status --porcelain=v1 -z`.
type StatusFile struct {
	// X/Y are the porcelain codes (X: index/staged, Y: worktree).
	X, Y byte
	// Path is the file path; OrigPath is set for renames/copies.
	Path     string
	OrigPath string
}

// Code returns the porcelain code as a string for JSON (e.g. "M", "?", "A").
func (f StatusFile) Code() string {
	if f.X == ' ' || f.X == 0 {
		return string(f.Y)
	}
	return string(f.X) + string(f.Y)
}

// MarshalJSON emits a friendly, stable shape for agents.
func (f StatusFile) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path      string `json:"path"`
		OrigPath  string `json:"orig_path,omitempty"`
		Staged    bool   `json:"staged"`
		Unstaged  bool   `json:"unstaged"`
		Untracked bool   `json:"untracked"`
		Conflict  bool   `json:"conflict"`
		Code      string `json:"code"` // porcelain XY, e.g. "M", "??", "A"
	}{
		Path:      f.Path,
		OrigPath:  f.OrigPath,
		Staged:    f.Staged(),
		Unstaged:  f.Unstaged(),
		Untracked: f.Untracked(),
		Conflict:  f.Conflict(),
		Code:      f.Code(),
	})
}

func (f StatusFile) Staged() bool   { return f.X != 0 && f.X != ' ' && f.X != '?' }
func (f StatusFile) Unstaged() bool { return f.Y != 0 && f.Y != ' ' && f.Y != '?' }
func (f StatusFile) Untracked() bool {
	return f.X == '?' && f.Y == '?'
}
func (f StatusFile) Conflict() bool {
	return f.X == 'U' || f.Y == 'U' || (f.X == 'A' && f.Y == 'A') || (f.X == 'D' && f.Y == 'D')
}

// RepoStatus is the working-tree state of one worktree
// (docs/design/02-data-model.md §1.5). Counts are computed at parse time.
type RepoStatus struct {
	Branch   string `json:"branch,omitempty"`   // current branch ("HEAD" when detached)
	Upstream string `json:"upstream,omitempty"` // "origin/main" or ""

	Ahead  int          `json:"ahead"` // commits ahead of upstream
	Behind int          `json:"behind"`
	Files  []StatusFile `json:"files,omitempty"`

	Staged    int `json:"staged"`
	Unstaged  int `json:"unstaged"`
	Untracked int `json:"untracked"`
	Conflicts int `json:"conflicts"`
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
