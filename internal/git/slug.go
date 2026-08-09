package git

import (
	"strings"
)

// Slug converts a branch name into a filesystem-safe path segment
// (00-decisions.md D8, review m7): lowercase; '/' → '-'; strip characters
// outside [a-z0-9._-]; collapse repeated '-'; trim leading '-'; reject
// empty, ".", ".."; cap at 80 chars.
func Slug(branch string) string {
	var b strings.Builder
	b.Grow(len(branch))
	for _, r := range strings.ToLower(branch) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
		case r == '/' || r == '-':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" || s == "." || s == ".." {
		return ""
	}
	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "-.")
	}
	return s
}
