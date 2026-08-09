package git

import (
	"bytes"
	"context"
	"regexp"
	"strconv"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// Status runs `git status --porcelain=v1 -z --branch` in dir and parses it
// into a typed RepoStatus (docs/design/03-git-layer.md §3.1, §4.1).
func Status(ctx context.Context, r Runner, dir string) (*model.RepoStatus, error) {
	out, err := r.RunIn(ctx, dir, "status", "--porcelain=v1", "-z", "--branch")
	if err != nil {
		return nil, err
	}
	return ParseStatus(out)
}

// The header is matched AFTER stripping the leading "## " prefix.
var branchHeaderRe = regexp.MustCompile(`^(.+?)(?:\.\.\.(.+?))?(?: \[(.*)\])?$`)

// ParseStatus parses the NUL-separated porcelain v1 -z output. The first
// field is the "## branch..." header (when --branch is used); file records
// are "XY <path>" with an extra NUL-separated <orig> field for renames.
func ParseStatus(out []byte) (*model.RepoStatus, error) {
	st := &model.RepoStatus{Branch: "HEAD"}

	fields := bytes.Split(out, []byte{0})
	// Header.
	if len(fields) > 0 {
		head := fields[0]
		if bytes.HasPrefix(head, []byte("## ")) {
			if m := branchHeaderRe.FindSubmatch(bytes.TrimSpace(head[3:])); m != nil {
				st.Branch = string(m[1])
				if st.Branch == "HEAD (no branch)" {
					st.Branch = "HEAD" // detached HEAD (data model: "HEAD")
				}
				if len(m) > 2 && len(m[2]) > 0 {
					st.Upstream = string(m[2])
				}
				if len(m) > 3 && len(m[3]) > 0 {
					parseAheadBehind(st, string(m[3]))
				}
			}
			fields = fields[1:]
		}
	}

	// File records. A rename/copy record consumes TWO fields ("XY <new>\0<old>\0").
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 3 || f[2] != ' ' {
			continue // empty or malformed; skip
		}
		code := f[:2]
		x, y := code[0], code[1]
		path := string(f[3:])

		sf := model.StatusFile{X: x, Y: y, Path: path}
		// Rename/copy: original path is the next NUL field.
		if (x == 'R' || x == 'C') && i+1 < len(fields) && len(fields[i+1]) > 0 {
			sf.OrigPath = string(fields[i+1])
			i++
		}

		st.Files = append(st.Files, sf)
		switch {
		case sf.Conflict():
			st.Conflicts++
		case sf.Untracked():
			st.Untracked++
		case sf.Staged() && sf.Unstaged():
			st.Staged++
			st.Unstaged++
		case sf.Staged():
			st.Staged++
		case sf.Unstaged():
			st.Unstaged++
		}
	}
	return st, nil
}

// parseAheadBehind parses the bracketed portion of the branch header, e.g.
// "ahead 1, behind 2" or "gone".
func parseAheadBehind(st *model.RepoStatus, s string) {
	for _, part := range splitComma(s) {
		part = bytes.TrimSpace(part)
		switch {
		case bytes.HasPrefix(part, []byte("ahead ")):
			if n, err := strconv.Atoi(string(part[len("ahead "):])); err == nil {
				st.Ahead = n
			}
		case bytes.HasPrefix(part, []byte("behind ")):
			if n, err := strconv.Atoi(string(part[len("behind "):])); err == nil {
				st.Behind = n
			}
		}
	}
}

func splitComma(s string) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, []byte(s[start:i]))
			start = i + 1
		}
	}
	out = append(out, []byte(s[start:]))
	return out
}
