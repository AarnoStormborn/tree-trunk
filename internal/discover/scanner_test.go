package discover

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// mkTree builds fixture directories. spec maps path → kind:
//
//	"d"        directory
//	"g"        directory named .git (main repo marker)
//	"l:target" file named .git containing "gitdir: target"
//	"s:target" file named .git containing "gitdir: target" (submodule-style:
//	           target contains "/.git/modules/")
//	"f"        plain file
func mkTree(t *testing.T, root string, spec map[string]string) {
	t.Helper()
	names := make([]string, 0, len(spec))
	for name := range spec {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		kind := spec[name]
		p := filepath.Join(root, name)
		switch {
		case kind == "d" || kind == "g":
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
		case len(kind) >= 2 && kind[:2] == "l:":
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("gitdir: "+kind[2:]+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		case len(kind) >= 2 && kind[:2] == "s:":
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("gitdir: "+kind[2:]+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		case kind == "f":
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("bad spec %q", kind)
		}
	}
}

func scan(t *testing.T, opts Options) []Hit {
	t.Helper()
	var hits []Hit
	err := Scanner(context.Background(), opts, func(h Hit) error {
		hits = append(hits, h)
		return nil
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return hits
}

func names(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = filepath.Base(h.Path)
	}
	return out
}

func TestScanMainAndLinkedAndSubmodule(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		"a/.git":                "g",                       // main repo
		"linked/.git":           "l:a/.git/worktrees/feat", // linked worktree OUTSIDE the repo dir
		"sub/.git":              "s:a/.git/modules/sub",    // submodule → skipped
		"b/.git":                "g",                       // second main repo
		"node_modules/pkg/.git": "g",                       // inside ignored dir → skipped
	})

	hits := scan(t, Options{Roots: []string{root}, MaxDepth: 8, Ignore: DefaultIgnore})
	got := names(hits)
	sort.Strings(got)
	want := []string{"a", "b", "linked"} // linked worktree is its own hit (folded later by canonical ID)
	if len(got) != len(want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hits = %v, want %v", got, want)
		}
	}
	// The linked hit must carry the resolved gitdir line.
	for _, h := range hits {
		if h.Kind == KindLinked && h.GitDirLine != "" {
			return // found
		}
	}
	t.Fatalf("expected a linked-worktree hit with GitDirLine, got %+v", hits)
}

func TestScanLinkedWorktreeAtTopLevel(t *testing.T) {
	root := t.TempDir()
	// A linked worktree outside the repo tree: .git file points to an
	// absolute common dir.
	common := filepath.Join(root, "repo", ".git")
	mkTree(t, root, map[string]string{
		"repo/.git":       "g",
		"wt/feature/.git": "l:" + common + "/worktrees/feature",
	})
	hits := scan(t, Options{Roots: []string{root}, MaxDepth: 8})
	got := names(hits)
	sort.Strings(got)
	want := []string{"feature", "repo"}
	sort.Strings(want)
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestScanHiddenPolicies(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		".dotfiles/.git":            "g", // hidden repo → found by peek/scan
		".config/nvim/.git":         "g", // hidden, depth 1 below hidden dir
		".config/Code/User/a/.git":  "g", // hidden, depth 2 below hidden dir
		".cache/big/deep/repo/.git": "g", // depth 3 below hidden dir → skipped in peek
		"visible/.git":              "g",
	})

	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{
			name: "peek default",
			opts: Options{Roots: []string{root}, MaxDepth: 10, Hidden: HiddenPeek, HiddenPeekDepth: 2},
			want: []string{"visible", ".dotfiles", "nvim"},
		},
		{
			name: "skip",
			opts: Options{Roots: []string{root}, MaxDepth: 10, Hidden: HiddenSkip},
			want: []string{"visible"},
		},
		{
			name: "scan",
			opts: Options{Roots: []string{root}, MaxDepth: 10, Hidden: HiddenScan},
			want: []string{"visible", ".dotfiles", "nvim", "a", "repo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := scan(t, tc.opts)
			got := names(hits)
			sort.Strings(got)
			sort.Strings(tc.want)
			if len(got) != len(tc.want) {
				t.Fatalf("hits = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("hits = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestScanDepthCap(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		"l1/l2/l3/.git": "g",
		"l1/l2/.git":    "g",
		"l1/.git":       "g",
	})
	hits := scan(t, Options{Roots: []string{root}, MaxDepth: 2})
	got := names(hits)
	sort.Strings(got)
	want := []string{"l1"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestScanSuffixIgnore(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		"go/pkg/mod/x/.git": "g", // trailing suffix go/pkg/mod → skipped
		"go/src/mod/.git":   "g", // not a suffix match → found
	})
	hits := scan(t, Options{
		Roots:    []string{root},
		MaxDepth: 8,
		Ignore:   []string{"go/pkg/mod"},
	})
	got := names(hits)
	if len(got) != 1 || got[0] != "mod" {
		t.Fatalf("hits = %v, want [mod]", got)
	}
}

func TestScanBare(t *testing.T) {
	root := t.TempDir()
	// Simulate a bare repo: HEAD + objects/ + config with core.bare.
	bare := filepath.Join(root, "bare.git")
	if err := os.MkdirAll(filepath.Join(bare, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "config"), []byte("[core]\n\tbare = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkTree(t, root, map[string]string{"normal/.git": "g"})

	off := scan(t, Options{Roots: []string{root}, MaxDepth: 8, IncludeBare: false})
	if len(off) != 1 {
		t.Fatalf("include_bare=false: hits = %+v, want 1", off)
	}
	on := scan(t, Options{Roots: []string{root}, MaxDepth: 8, IncludeBare: true})
	if len(on) != 2 {
		t.Fatalf("include_bare=true: hits = %+v, want 2", on)
	}
}

func TestScanSymlinks(t *testing.T) {
	base := t.TempDir() // scanned root: contains only the symlink
	real := t.TempDir() // real tree OUTSIDE base
	mkTree(t, real, map[string]string{"proj/.git": "g"})
	linkroot := filepath.Join(base, "linkroot")
	if err := os.Symlink(real, linkroot); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	// The symlink is an ENTRY inside the root; follow=false skips it.
	off := scan(t, Options{Roots: []string{base}, MaxDepth: 8, FollowSymlinks: false})
	if len(off) != 0 {
		t.Fatalf("follow=false: hits = %+v, want 0", off)
	}
	resolvedReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(resolvedReal, "proj")

	on := scan(t, Options{Roots: []string{base}, MaxDepth: 8, FollowSymlinks: true})
	if len(on) != 1 || on[0].Path != wantPath {
		t.Fatalf("follow=true: hits = %+v, want %s", on, wantPath)
	}

	// A symlink USED AS an explicit scan root is always followed (the user
	// asked for that path). The walk emits the path as traversed (through
	// the link); Resolve canonicalizes the ID later.
	asRoot := scan(t, Options{Roots: []string{linkroot}, MaxDepth: 8, FollowSymlinks: false})
	if len(asRoot) != 1 || filepath.Base(asRoot[0].Path) != "proj" || asRoot[0].Kind != KindMain {
		t.Fatalf("symlinked root: hits = %+v, want one main hit named proj", asRoot)
	}
}

func TestScanSymlinkLoopProtection(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(a, "loop")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	// Must terminate despite the self-loop.
	hits := scan(t, Options{Roots: []string{root}, MaxDepth: 6, FollowSymlinks: true})
	_ = hits // completion without stack overflow is the assertion
}

func TestRootItselfIsRepo(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{".git": "g"})
	hits := scan(t, Options{Roots: []string{root}, MaxDepth: 8})
	if len(hits) != 1 || hits[0].Kind != KindMain {
		t.Fatalf("hits = %+v, want 1 main hit for the root", hits)
	}
}
