# AGENTS.md

This is **tree-trunk**, a TUI *and* CLI for listing every git repo on a
machine and managing git **worktrees** across them (create / delete / lock /
prune). It shells out to system `git` (>= 2.38).

## If you need repo & worktree facts

Use the headless JSON interface — don't try to drive the TUI:

```sh
tree-trunk query --json                          # all repos, worktrees, status
tree-trunk query --filter dirty=true --json      # repos with changes
tree-trunk query --filter prunable=true --json   # worktrees safe to prune
tree-trunk query --repo /path/to/repo --no-scan  # a specific repo, no scan
tree-trunk query --fields id,branch,status       # smaller payloads
```

## If you need to mutate worktrees

```sh
tree-trunk wt list                       # all worktrees across repos (JSON)
tree-trunk wt create MYREPO feat/x       # create a worktree
--   (flags may go before or after positionals)
tree-trunk wt delete MYREPO feat/x       # guarded: refuses if dirty unless --force
tree-trunk wt lock MYREPO feat/x         # prevent two agents touching it
tree-trunk wt unlock MYREPO feat/x
tree-trunk wt prune MYREPO --dry-run     # preview, then prune
```

Every `wt` op returns `{ "ok": bool, ... }`; on failure an `error` object
with a stable `code` (e.g. `worktree_dirty`, `branch_checked_out_elsewhere`,
`worktree_locked`, `branch_exists`, `repo_not_found`). Create default path is
`~/.worktrees/<repo>/<slug>` (override with `--path`).

Each repo document: `id` (stable canonical git dir — your handle for that
repo), `name`, `path`, `branch`, `worktrees[]` (path, branch, `is_main`,
`locked`, `prunable`, `dirty`), and `status` (branch, `ahead`/`behind`,
per-file entries with porcelain `code`).

## If you need to introspect the CLI itself

```sh
tree-trunk describe   # JSON schema: subcommands, flags, output documents
tree-trunk --help     # human-readable subcommand list
tree-trunk completion zsh | bash
```

## TUI-only (for an interactive human, not an agent)

Running `tree-trunk` with no args opens the TUI. Keys: `1` status, `2`
worktrees, `3` history, `4` diff; `n` create worktree, `d`
delete, `L` lock/unlock, `P` prune. Press `?` for full keys. You are better
off using `query` above for programmatic use.

## Design

See `docs/design/10-agent-cli.md` for the agent-facing contract and the
planned mutating `tree-trunk wt` subcommands.