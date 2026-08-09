package ui

import (
	"path/filepath"
	"testing"
)

func TestStateTouchRecent(t *testing.T) {
	var st stateFile
	st.touchRecent("/a")
	st.touchRecent("/b")
	st.touchRecent("/a") // moves to front
	if len(st.RecentRepos) != 2 || st.RecentRepos[0] != "/a" || st.RecentRepos[1] != "/b" {
		t.Fatalf("recent = %v", st.RecentRepos)
	}
	// Cap at 20.
	for i := 0; i < 25; i++ {
		st.touchRecent("/r" + itoa(i))
	}
	if len(st.RecentRepos) > 20 {
		t.Fatalf("recent cap exceeded: %d", len(st.RecentRepos))
	}
}

func TestStateSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	st := stateFile{RecentRepos: []string{"/a", "/b"}}
	saveState(home, st)

	got := loadState(home)
	if len(got.RecentRepos) != 2 || got.RecentRepos[0] != "/a" {
		t.Fatalf("round-trip = %v", got.RecentRepos)
	}

	// Missing file → empty, no error.
	empty := loadState(filepath.Join(t.TempDir(), "nope"))
	if empty.RecentRepos != nil {
		t.Fatalf("missing file should yield empty state, got %v", empty.RecentRepos)
	}
}

func TestNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if !noColorEnv() {
		t.Fatal("NO_COLOR set (even empty) must be honored")
	}
	t.Setenv("NO_COLOR", "1")
	if !noColorEnv() {
		t.Fatal("NO_COLOR=1 must be honored")
	}
}

func TestApplyOverrides(t *testing.T) {
	th := DefaultTheme()
	applyOverrides(&th, map[string]string{"accent": "#ff0000"})
	if th.Accent != "#ff0000" {
		t.Fatalf("accent = %q", th.Accent)
	}
	// Unknown keys ignored; nil safe.
	applyOverrides(&th, nil)
}

func TestShortPathSanitizes(t *testing.T) {
	// Non-UTF8 bytes must not crash rendering.
	got := shortPath(string([]byte{0xff, 0xfe, '/', 'x'}))
	if got == "" {
		t.Fatal("expected sanitized path")
	}
}
