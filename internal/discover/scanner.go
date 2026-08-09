// Package discover scans the filesystem for git repositories.
// Rules: docs/design/02-data-model.md §2 and docs/design/09-config.md §2.3.
package discover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind classifies a discovered repo entry.
type Kind int

const (
	// KindMain: a directory named .git (normal repo / main worktree).
	KindMain Kind = iota
	// KindLinked: a file named .git whose first line is "gitdir: <path>"
	// (linked worktree).
	KindLinked
	// KindBare: no .git entry, but HEAD + objects/ present with core.bare
	// (only reported when IncludeBare is set).
	KindBare
)

func (k Kind) String() string {
	switch k {
	case KindMain:
		return "main"
	case KindLinked:
		return "linked"
	case KindBare:
		return "bare"
	}
	return "unknown"
}

// Hit is one discovered repo entry.
type Hit struct {
	Path string // directory containing the .git entry (or the bare dir)
	Kind Kind
	// GitDirLine is the resolved target of a linked worktree's "gitdir:"
	// pointer (empty otherwise).
	GitDirLine string
}

// HiddenPolicy controls descent into dot-directories (09-config.md §2.3).
type HiddenPolicy int

const (
	// HiddenSkip: never descend into hidden dirs.
	HiddenSkip HiddenPolicy = iota
	// HiddenPeek: descend up to HiddenPeekDepth levels below a hidden dir.
	HiddenPeek
	// HiddenScan: treat hidden dirs like any other.
	HiddenScan
)

// ParseHiddenPolicy maps config strings to HiddenPolicy.
func ParseHiddenPolicy(s string) (HiddenPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "skip":
		return HiddenSkip, nil
	case "", "peek":
		return HiddenPeek, nil
	case "scan":
		return HiddenScan, nil
	}
	return HiddenSkip, fmt.Errorf("discover.hidden_dirs: want \"skip\"|\"peek\"|\"scan\", got %q", s)
}

// Options configures a scan (09-config.md §1).
type Options struct {
	Roots           []string
	MaxDepth        int // 0 = unlimited
	Ignore          []string
	IncludeBare     bool
	FollowSymlinks  bool
	Hidden          HiddenPolicy
	HiddenPeekDepth int // levels below a hidden dir searched when HiddenPeek
}

// DefaultIgnore is the default discover.ignore list (09-config.md §3).
// Entries without "/" match any path segment; entries with "/" match the
// trailing segments of a dir's path relative to the scan root.
var DefaultIgnore = []string{
	"node_modules", "vendor", ".venv", "venv", "Pods", "DerivedData",
	".Trash", "Library", ".cache", "target", "dist", "build", ".next",
	".gradle", ".m2", ".conda", ".rustup", ".cargo", ".npm", ".bun",
	".pnpm-store", ".dart_tool", "__pycache__", "go/pkg/mod",
}

// ErrCancelled is returned when the scan context is cancelled.
var ErrCancelled = errors.New("scan cancelled")

// Scanner walks Roots and calls onHit for each repo entry found. Hits are
// emitted in deterministic (sorted) order per directory.
func Scanner(ctx context.Context, opts Options, onHit func(Hit) error) error {
	if len(opts.Roots) == 0 {
		return nil
	}
	if opts.HiddenPeekDepth <= 0 {
		opts.HiddenPeekDepth = 2
	}
	segIgnore, suffixIgnore := splitIgnore(opts.Ignore)
	visited := map[string]bool{} // EvalSymlinks-resolved dirs (symlink loop protection)

	for _, root := range opts.Roots {
		root = filepath.Clean(root)
		if err := walkDir(ctx, root, 0, false, 0, opts, segIgnore, suffixIgnore, visited, onHit); err != nil {
			if errors.Is(err, context.Canceled) {
				return ErrCancelled
			}
			return err
		}
	}
	return nil
}

func splitIgnore(list []string) (seg, suffix map[string]bool) {
	seg = map[string]bool{}
	suffix = map[string]bool{}
	for _, e := range list {
		e = strings.TrimSpace(e)
		if e == "" || e == ".git" {
			continue // .git is structural and always handled by the walker
		}
		if strings.Contains(e, "/") {
			suffix[strings.Trim(e, "/")] = true
		} else {
			seg[e] = true
		}
	}
	return seg, suffix
}

// relDepth computes the depth of dir relative to root (0 for root itself).
func relDepth(root, dir string) int {
	if dir == root {
		return 0
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

func walkDir(ctx context.Context, dir string, depth int, inHidden bool, hiddenDepth int, opts Options, segIgnore, suffixIgnore map[string]bool, visited map[string]bool, onHit func(Hit) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Ignore check (not applied to a scan root itself).
	if depth > 0 {
		base := filepath.Base(dir)
		if segIgnore[base] || matchesSuffix(dir, suffixIgnore) {
			return nil
		}
	}

	// Hidden policy — MUST gate before repo detection: "skip" means repos
	// inside hidden dirs are invisible, "peek" bounds the subtree budget.
	isHidden := depth > 0 && strings.HasPrefix(filepath.Base(dir), ".")
	if isHidden {
		switch opts.Hidden {
		case HiddenSkip:
			return nil
		case HiddenPeek:
			inHidden = true
			hiddenDepth = 1 // the hidden dir itself is level 1
		}
	} else if inHidden {
		hiddenDepth++
		if hiddenDepth > opts.HiddenPeekDepth {
			return nil
		}
	}

	// Repo detection.
	if hit, isRepo, err := detectRepo(dir, opts.IncludeBare); err != nil {
		return err
	} else if isRepo {
		return onHit(hit)
	}

	// Depth cap: dirs at maxDepth do not descend (repos at this depth were
	// already caught above).
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, fs.ErrPermission) {
			return nil // unreadable dirs are skipped silently
		}
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		name := e.Name()
		if name == ".git" {
			continue // handled by detectRepo; never descend into it
		}
		if segIgnore[name] {
			continue
		}
		full := filepath.Join(dir, name)

		if e.Type()&fs.ModeSymlink != 0 {
			if !opts.FollowSymlinks {
				continue
			}
			target, err := filepath.EvalSymlinks(full)
			if err != nil {
				continue // dangling symlink
			}
			fi, err := os.Stat(target)
			if err != nil || !fi.IsDir() {
				continue
			}
			resolved, err := filepath.EvalSymlinks(target)
			if err != nil {
				continue
			}
			if visited[resolved] {
				continue // loop protection
			}
			visited[resolved] = true
			// Re-run the dir logic on the resolved target so a symlinked
			// repo root is detected; depth+1 to respect caps.
			if err := walkDir(ctx, target, depth+1, inHidden, hiddenDepth, opts, segIgnore, suffixIgnore, visited, onHit); err != nil {
				return err
			}
			continue
		}

		if !e.IsDir() {
			continue
		}
		if err := walkDir(ctx, full, depth+1, inHidden, hiddenDepth, opts, segIgnore, suffixIgnore, visited, onHit); err != nil {
			return err
		}
	}
	return nil
}

// detectRepo checks dir for .git (dir → main, file → linked) or, when
// includeBare, HEAD+objects (bare).
func detectRepo(dir string, includeBare bool) (Hit, bool, error) {
	dotgit := filepath.Join(dir, ".git")
	if fi, err := os.Lstat(dotgit); err == nil {
		if fi.IsDir() {
			return Hit{Path: dir, Kind: KindMain}, true, nil
		}
		if fi.Mode().IsRegular() {
			line, err := readGitDirLine(dotgit)
			if err != nil {
				return Hit{}, false, nil // unreadable or malformed: skip
			}
			if strings.HasPrefix(line, "gitdir: ") {
				target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				// Submodule discrimination (02-data-model.md §2.1): a
				// submodule's gitdir resolves under <super>/.git/modules/;
				// a linked worktree's resolves under <common>/worktrees/.
				if isSubmoduleGitDir(target) {
					return Hit{}, false, nil
				}
				return Hit{Path: dir, Kind: KindLinked, GitDirLine: target}, true, nil
			}
		}
		return Hit{}, false, nil // .git exists but is neither: skip (no descend)
	}
	if includeBare && looksBare(dir) {
		return Hit{Path: dir, Kind: KindBare}, true, nil
	}
	return Hit{}, false, nil
}

func readGitDirLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}
	return string(bytes.TrimSpace(buf[:n])), nil
}

func isSubmoduleGitDir(target string) bool {
	clean := filepath.Clean(target)
	parts := strings.Split(clean, string(filepath.Separator))
	for i, p := range parts {
		if p == ".git" && i+1 < len(parts) && parts[i+1] == "modules" {
			return true
		}
	}
	return false
}

// looksBare reports whether dir plausibly is a bare repo (HEAD + objects +
// config with core.bare=true). Only called when IncludeBare is set.
func looksBare(dir string) bool {
	if !hasFile(dir, "HEAD") || !isDir(dir, "objects") {
		return false
	}
	if !hasFile(dir, "config") {
		return false
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		return false
	}
	return bytes.Contains(cfg, []byte("bare = true"))
}

func hasFile(dir, name string) bool {
	fi, err := os.Stat(filepath.Join(dir, name))
	return err == nil && fi.Mode().IsRegular()
}

func isDir(dir, name string) bool {
	fi, err := os.Stat(filepath.Join(dir, name))
	return err == nil && fi.IsDir()
}

// matchesSuffix reports whether dir's path ends with one of the slash
// entries (e.g. "go/pkg/mod" matches ~/go/pkg/mod but not ~/src/go).
func matchesSuffix(dir string, suffixes map[string]bool) bool {
	dir = filepath.Clean(dir)
	for s := range suffixes {
		if strings.HasSuffix(dir, string(filepath.Separator)+s) || dir == s {
			return true
		}
	}
	return false
}
