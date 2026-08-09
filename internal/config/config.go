// Package config handles CLI flags and config.toml.
// Schema: docs/design/09-config.md.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AarnoStormborn/tree-trunk/internal/discover"
	"github.com/BurntSushi/toml"
)

// Config is the fully-resolved runtime configuration (flags > config >
// defaults; 09-config.md §2.5).
type Config struct {
	Workers   int
	Discover  Discover
	Worktrees Worktrees
	Refresh   Refresh
	Theme     Theme
	Clipboard Clipboard
	// CLI-only:
	Repos       []string // --repo (always scanned, never lost)
	ScanRoots   []string // --scan-root (REPLACE default + config roots)
	NoScan      bool     // --no-scan (suppress all scanning)
	ConfigPath  string   // --config
	List        bool     // --list (headless: print repo paths)
	ShowVersion bool
	Home        string // resolved $HOME
}

type Discover struct {
	MaxDepth        int
	ScanRoots       []string // config scan_roots (ADD to $HOME)
	ScanHome        bool
	Ignore          []string
	IncludeBare     bool
	FollowSymlinks  bool
	HiddenDirs      discover.HiddenPolicy
	HiddenPeekDepth int
}

type Worktrees struct {
	Directory string
	Repos     []WorktreeRepo
}

type WorktreeRepo struct {
	Path      string
	Directory string
}

type Refresh struct {
	PollIntervalMS int
}

type Theme struct {
	Name      string
	Variant   string
	Overrides map[string]string
}

type Clipboard struct {
	Enabled bool
}

// Defaults returns the built-in defaults (09-config.md §1).
func Defaults() Config {
	return Config{
		Workers: 8,
		Discover: Discover{
			MaxDepth:        8,
			ScanHome:        true,
			Ignore:          discover.DefaultIgnore,
			HiddenDirs:      discover.HiddenPeek,
			HiddenPeekDepth: 2,
		},
		Worktrees: Worktrees{Directory: "~/.worktrees"},
		Theme:     Theme{Name: "default", Variant: "auto"},
		Clipboard: Clipboard{Enabled: true},
	}
}

// fileConfig mirrors the TOML schema (unknown keys become warnings).
type fileConfig struct {
	Workers   *int           `toml:"workers"`
	Discover  *fileDiscover  `toml:"discover"`
	Worktrees *fileWorktrees `toml:"worktrees"`
	Refresh   *fileRefresh   `toml:"refresh"`
	Theme     *fileTheme     `toml:"theme"`
	Clipboard *fileClipboard `toml:"clipboard"`
}

type fileDiscover struct {
	MaxDepth        *int     `toml:"max_depth"`
	ScanRoots       []string `toml:"scan_roots"`
	ScanHome        *bool    `toml:"scan_home"`
	Ignore          []string `toml:"ignore"`
	IncludeBare     *bool    `toml:"include_bare"`
	FollowSymlinks  *bool    `toml:"follow_symlinks"`
	HiddenDirs      *string  `toml:"hidden_dirs"`
	HiddenPeekDepth *int     `toml:"hidden_peek_depth"`
}

type fileWorktrees struct {
	Directory *string            `toml:"directory"`
	Repos     []fileWorktreeRepo `toml:"repos"`
}

type fileWorktreeRepo struct {
	Path      string `toml:"path"`
	Directory string `toml:"directory"`
}

type fileRefresh struct {
	PollIntervalMS *int `toml:"poll_interval_ms"`
}

type fileTheme struct {
	Name      *string           `toml:"name"`
	Variant   *string           `toml:"variant"`
	Overrides map[string]string `toml:"overrides"`
}

type fileClipboard struct {
	Enabled *bool `toml:"enabled"`
}

// ParseFlags defines the v1 flag set (D7) and returns the Config with
// CLI-only fields populated, plus a flag.FlagSet for --help/--version
// handling by the caller.
func ParseFlags(args []string) (*Config, *flag.FlagSet, error) {
	cfg := Defaults()
	fs := flag.NewFlagSet("tree-trunk", flag.ContinueOnError)
	var repoList, scanRoots multiFlag
	var configPath string
	var noScan, list, showVersion bool
	fs.Var(&repoList, "repo", "explicit repo path (repeatable; always scanned)")
	fs.Var(&scanRoots, "scan-root", "scan root (repeatable; REPLACES default $HOME + config roots)")
	fs.BoolVar(&noScan, "no-scan", false, "do not scan the filesystem; use --repo inputs only")
	fs.StringVar(&configPath, "config", "", "path to config.toml (default ~/.config/tree-trunk/config.toml)")
	fs.BoolVar(&list, "list", false, "headless: print discovered repo paths, one per line")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "tree-trunk — TUI for git repos & worktrees\n\nUsage:\n  tree-trunk [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return nil, fs, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fs, fmt.Errorf("cannot determine home dir: %w", err)
	}

	cfg.Home = home
	cfg.Repos = repoList
	cfg.ScanRoots = scanRoots
	cfg.NoScan = noScan
	cfg.List = list
	cfg.ShowVersion = showVersion
	cfg.ConfigPath = configPath
	return &cfg, fs, nil
}

// Load merges config.toml into cfg (flags already applied) and validates
// (09-config.md §2.5).
func Load(cfg *Config) error {
	// Tilde-expand CLI flags (09 §2.1) — must happen before any early
	// return, so it also applies when no config file exists.
	for i, r := range cfg.Repos {
		cfg.Repos[i] = discover.ExpandPath(r, cfg.Home)
	}
	for i, r := range cfg.ScanRoots {
		cfg.ScanRoots[i] = discover.ExpandPath(r, cfg.Home)
	}
	// Same for the default worktree root (09 §2.1; missing-config early
	// return below would otherwise skip the expansion).
	cfg.Worktrees.Directory = discover.ExpandPath(cfg.Worktrees.Directory, cfg.Home)

	path := cfg.ConfigPath
	if path == "" {
		path = filepath.Join(cfg.Home, ".config", "tree-trunk", "config.toml")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) && cfg.ConfigPath == "" {
			return nil // missing default config is fine
		}
		return fmt.Errorf("config: %w", err)
	}

	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		fmt.Fprintf(os.Stderr, "tree-trunk: warning: unknown config key(s): %v\n", undec)
	}

	if fc.Workers != nil {
		cfg.Workers = clamp(*fc.Workers, 4, 16)
	}
	if d := fc.Discover; d != nil {
		if d.MaxDepth != nil {
			if *d.MaxDepth < 0 {
				return fmt.Errorf("config: discover.max_depth must be >= 0")
			}
			cfg.Discover.MaxDepth = *d.MaxDepth
		}
		if d.ScanRoots != nil {
			cfg.Discover.ScanRoots = expandAll(d.ScanRoots, cfg.Home)
		}
		if d.ScanHome != nil {
			cfg.Discover.ScanHome = *d.ScanHome
		}
		if d.Ignore != nil {
			cfg.Discover.Ignore = d.Ignore // REPLACES the default list (09 §3)
		}
		if d.IncludeBare != nil {
			cfg.Discover.IncludeBare = *d.IncludeBare
		}
		if d.FollowSymlinks != nil {
			cfg.Discover.FollowSymlinks = *d.FollowSymlinks
		}
		if d.HiddenDirs != nil {
			p, err := discover.ParseHiddenPolicy(*d.HiddenDirs)
			if err != nil {
				return err
			}
			cfg.Discover.HiddenDirs = p
		}
		if d.HiddenPeekDepth != nil {
			cfg.Discover.HiddenPeekDepth = max(1, *d.HiddenPeekDepth)
		}
	}
	if w := fc.Worktrees; w != nil {
		if w.Directory != nil {
			cfg.Worktrees.Directory = discover.ExpandPath(*w.Directory, cfg.Home)
		} else {
			cfg.Worktrees.Directory = discover.ExpandPath(cfg.Worktrees.Directory, cfg.Home)
		}
		for _, r := range w.Repos {
			cfg.Worktrees.Repos = append(cfg.Worktrees.Repos, WorktreeRepo{
				Path:      discover.ExpandPath(r.Path, cfg.Home),
				Directory: discover.ExpandPath(r.Directory, cfg.Home),
			})
		}
	} else {
		cfg.Worktrees.Directory = discover.ExpandPath(cfg.Worktrees.Directory, cfg.Home)
	}
	if r := fc.Refresh; r != nil && r.PollIntervalMS != nil {
		cfg.Refresh.PollIntervalMS = *r.PollIntervalMS
		if cfg.Refresh.PollIntervalMS != 0 && cfg.Refresh.PollIntervalMS < 1000 {
			cfg.Refresh.PollIntervalMS = 1000
		}
	}
	if t := fc.Theme; t != nil {
		if t.Name != nil {
			cfg.Theme.Name = *t.Name
		}
		if t.Variant != nil {
			switch *t.Variant {
			case "auto", "light", "dark":
				cfg.Theme.Variant = *t.Variant
			default:
				return fmt.Errorf("config: theme.variant: want \"auto\"|\"light\"|\"dark\", got %q", *t.Variant)
			}
		}
		cfg.Theme.Overrides = t.Overrides
	}
	if c := fc.Clipboard; c != nil && c.Enabled != nil {
		cfg.Clipboard.Enabled = *c.Enabled
	}
	return nil
}

func expandAll(paths []string, home string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = discover.ExpandPath(p, home)
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// multiFlag collects repeated string flags (--repo, --scan-root).
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
