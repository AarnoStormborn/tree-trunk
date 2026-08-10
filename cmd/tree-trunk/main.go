// Command tree-trunk is a TUI for listing git repos and managing worktrees.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AarnoStormborn/tree-trunk/internal/config"
	"github.com/AarnoStormborn/tree-trunk/internal/discover"
	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/state"
	"github.com/AarnoStormborn/tree-trunk/internal/ui"
)

var version = "0.1.0-dev" // overridden at release build time

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tree-trunk:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, _, err := config.ParseFlags(args)
	if err != nil {
		if err.Error() == "flag: help requested" {
			return nil
		}
		return err
	}
	if cfg.ShowVersion {
		fmt.Printf("tree-trunk %s\n", version)
		return nil
	}
	if err := config.Load(cfg); err != nil {
		return err
	}

	// Git presence + version gate (D1/Q2; docs/design/03-git-layer.md §2).
	gitPath, err := git.LookPath()
	if err != nil {
		return err
	}
	if _, err := git.CheckVersion(context.Background(), gitPath); err != nil {
		return err
	}

	store := state.NewStore()
	refresher := state.NewRefresher(git.NewExecRunner(gitPath), store, cfg.Workers)

	if cfg.List {
		return listRepos(cfg, gitPath)
	}

	return ui.Run(context.Background(), cfg, store, refresher, version)
}

// listRepos implements the headless --list mode: print canonical repo paths,
// one per line (05-implementation-plan.md M0; --json stays deferred F3).
func listRepos(cfg *config.Config, gitPath string) error {
	ctx := context.Background()
	store := state.NewStore()
	runner := git.NewExecRunner(gitPath)

	collect := func(p string) error {
		repo, err := git.Resolve(ctx, runner, p)
		if err != nil {
			return nil
		}
		store.Upsert(repo)
		return nil
	}

	for _, p := range cfg.Repos {
		if err := collect(p); err != nil {
			return err
		}
	}
	if !cfg.NoScan {
		roots := discover.Roots(cfg.Home, cfg.ScanRoots, cfg.Discover.ScanRoots, cfg.Discover.ScanHome)
		opts := discover.Options{
			Roots:           roots,
			MaxDepth:        cfg.Discover.MaxDepth,
			Ignore:          cfg.Discover.Ignore,
			IncludeBare:     cfg.Discover.IncludeBare,
			FollowSymlinks:  cfg.Discover.FollowSymlinks,
			Hidden:          cfg.Discover.HiddenDirs,
			HiddenPeekDepth: cfg.Discover.HiddenPeekDepth,
		}
		if err := discover.Scanner(ctx, opts, func(hit discover.Hit) error {
			return collect(hit.Path)
		}); err != nil {
			return err
		}
	}

	for _, r := range store.List() {
		fmt.Println(r.ID)
	}
	return nil
}
