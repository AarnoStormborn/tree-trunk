// Command tree-trunk is a TUI for listing git repos and managing worktrees.
package main

import (
	"context"
	"errors"
	"flag"
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
		var ec exitCoder
		switch {
		// Commands that already reported on stdout (the `wt` JSON envelope)
		// pick their own exit code and print nothing more.
		case errors.As(err, &ec):
			os.Exit(ec.ExitCode())
		// --help/-h is a successful request, not a failure.
		case errors.Is(err, flag.ErrHelp):
			os.Exit(0)
		default:
			fmt.Fprintln(os.Stderr, "tree-trunk:", err)
			os.Exit(1)
		}
	}
}

// exitCoder lets a command choose the process exit code instead of the
// default 1. Exit codes are part of the agent contract
// (docs/design/10-agent-cli.md §Exit codes): 0 ok, 1 usage/IO, 3 not-found,
// 4 dirty-blocked, 5 locked.
type exitCoder interface{ ExitCode() int }

// codedError carries an exit code for an error whose message was already
// written to stdout, so main does not duplicate it on stderr.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }

// ExitCode implements exitCoder.
func (e *codedError) ExitCode() int { return e.code }

func run(args []string) error {
	cfg, _, err := config.ParseFlags(args)
	if err != nil {
		// -h/--help on the top-level flags is not an error.
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if cfg.ShowVersion {
		fmt.Printf("tree-trunk %s\n", version)
		return nil
	}
	// Subcommands are handled before flag parsing (flag.Parse stops at the
	// first non-flag argument). Flags for a subcommand follow it, e.g.
	// `tree-trunk query --json --filter dirty=true`.
	if len(args) > 0 && args[0] == "query" {
		return runQueryCommand(args[1:])
	}
	// tree-trunk describe: emit the machine-readable CLI schema.
	if len(args) > 0 && args[0] == "describe" {
		return printDescribe(version)
	}
	// tree-trunk wt <op>: the mutating worktree API.
	if len(args) > 0 && args[0] == "wt" {
		return runWTCommand(args[1:], version)
	}
	// Hidden-ish subcommand: tree-trunk completion zsh|bash (M4 backlog).
	if len(args) > 0 && args[0] == "completion" {
		if len(args) < 2 {
			return fmt.Errorf("usage: tree-trunk completion zsh|bash")
		}
		return printCompletions(args[1])
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
