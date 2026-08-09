package discover

import (
	"path/filepath"
	"strings"
)

// Roots computes the effective scan roots and deduplicates them by canonical
// path (docs/design/09-config.md §2.2). home is $HOME; scanRoots are
// --scan-root flags (REPLACE defaults); configRoots are config
// discover.scan_roots (ADD to $HOME); scanHome decides whether $HOME is
// scanned at all.
func Roots(home string, scanRoots, configRoots []string, scanHome bool) []string {
	roots := make([]string, 0, 4)
	switch {
	case len(scanRoots) > 0:
		// Flags replace both the default $HOME root and config roots.
		roots = append(roots, scanRoots...)
	default:
		if scanHome && home != "" {
			roots = append(roots, home)
		}
		roots = append(roots, configRoots...)
	}

	seen := map[string]bool{}
	clean := roots[:0]
	for _, r := range roots {
		r = filepath.Clean(r)
		key := canonicalOrRaw(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, r)
	}
	return clean
}

// canonicalOrRaw returns the canonical path when resolution succeeds, else
// the cleaned path (roots may legitimately not exist yet for scan; missing
// roots are simply skipped by the walker).
func canonicalOrRaw(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		if abs, err := filepath.Abs(resolved); err == nil {
			return abs
		}
		return resolved
	}
	return p
}

// ExpandPath expands a leading "~" or "~/" via home (09-config.md §2.1).
// "~user/..." forms are not supported. Returns p unchanged when no leading
// tilde is present.
func ExpandPath(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
