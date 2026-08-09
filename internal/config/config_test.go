package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AarnoStormborn/tree-trunk/internal/discover"
)

func TestDefaults(t *testing.T) {
	c := Defaults()
	if c.Workers != 8 {
		t.Fatalf("Workers = %d, want 8", c.Workers)
	}
	if c.Discover.HiddenDirs != discover.HiddenPeek {
		t.Fatalf("HiddenDirs = %v, want peek", c.Discover.HiddenDirs)
	}
	if c.Worktrees.Directory != "~/.worktrees" {
		t.Fatalf("Worktrees.Directory = %q", c.Worktrees.Directory)
	}
	if len(c.Discover.Ignore) == 0 {
		t.Fatal("expected default ignore list")
	}
}

func TestFlagParsing(t *testing.T) {
	cfg, _, err := ParseFlags([]string{
		"--repo", "/a", "--repo", "/b",
		"--scan-root", "/src",
		"--no-scan",
		"--list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repos) != 2 || cfg.Repos[0] != "/a" || cfg.Repos[1] != "/b" {
		t.Fatalf("Repos = %v", cfg.Repos)
	}
	if len(cfg.ScanRoots) != 1 || cfg.ScanRoots[0] != "/src" {
		t.Fatalf("ScanRoots = %v", cfg.ScanRoots)
	}
	if !cfg.NoScan || !cfg.List {
		t.Fatalf("NoScan=%v List=%v, want true/true", cfg.NoScan, cfg.List)
	}
}

func TestConfigFileLoad(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "tree-trunk")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(cfgDir, "config.toml")
	toml := `workers = 2

[discover]
max_depth = 5
scan_roots = ["~/src", "~/code"]
hidden_dirs = "skip"
include_bare = true

[worktrees]
directory = "~/wt"

[refresh]
poll_interval_ms = 3000

[theme]
name = "catppuccin"
variant = "dark"

[clipboard]
enabled = false
`
	if err := os.WriteFile(cfgFile, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	cfg.Home = home
	cfg.ConfigPath = cfgFile
	if err := Load(&cfg); err != nil {
		t.Fatal(err)
	}

	// workers clamped to [4,16]
	if cfg.Workers != 4 {
		t.Fatalf("Workers = %d, want 4 (clamped)", cfg.Workers)
	}
	if cfg.Discover.MaxDepth != 5 {
		t.Fatalf("MaxDepth = %d", cfg.Discover.MaxDepth)
	}
	if len(cfg.Discover.ScanRoots) != 2 || cfg.Discover.ScanRoots[0] != filepath.Join(home, "src") {
		t.Fatalf("ScanRoots = %v (want ~ expansion)", cfg.Discover.ScanRoots)
	}
	if cfg.Discover.HiddenDirs != discover.HiddenSkip {
		t.Fatalf("HiddenDirs = %v, want skip", cfg.Discover.HiddenDirs)
	}
	if !cfg.Discover.IncludeBare {
		t.Fatal("IncludeBare should be true")
	}
	if cfg.Worktrees.Directory != filepath.Join(home, "wt") {
		t.Fatalf("Worktrees.Directory = %q", cfg.Worktrees.Directory)
	}
	if cfg.Refresh.PollIntervalMS != 3000 {
		t.Fatalf("PollIntervalMS = %d", cfg.Refresh.PollIntervalMS)
	}
	if cfg.Theme.Name != "catppuccin" || cfg.Theme.Variant != "dark" {
		t.Fatalf("theme = %+v", cfg.Theme)
	}
	if cfg.Clipboard.Enabled {
		t.Fatal("clipboard should be disabled")
	}
}

func TestConfigUnknownKeyWarns(t *testing.T) {
	home := t.TempDir()
	cfgFile := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgFile, []byte("bogus_key = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.Home = home
	cfg.ConfigPath = cfgFile
	if err := Load(&cfg); err != nil { // non-fatal warning
		t.Fatalf("Load: %v", err)
	}
}

func TestConfigInvalidVariantFails(t *testing.T) {
	home := t.TempDir()
	cfgFile := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[theme]\nvariant = \"neon\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.Home = home
	cfg.ConfigPath = cfgFile
	if err := Load(&cfg); err == nil {
		t.Fatal("expected error for invalid theme variant")
	}
}

func TestLoadMissingDefaultConfigIsFine(t *testing.T) {
	cfg := Defaults()
	cfg.Home = t.TempDir()
	if err := Load(&cfg); err != nil {
		t.Fatalf("Load with no config should be fine: %v", err)
	}
}

func TestTildeExpansionOfFlags(t *testing.T) {
	home := t.TempDir()
	cfg, _, err := ParseFlags([]string{"--repo", "~/myrepo"})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Home = home
	if err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "myrepo")
	if cfg.Repos[0] != want {
		t.Fatalf("Repos[0] = %q, want %q", cfg.Repos[0], want)
	}
}
