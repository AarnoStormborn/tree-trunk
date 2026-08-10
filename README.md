# tree-trunk

**TUI for all your git repos & worktrees.** List every repo on your machine,
create and delete git worktrees, and inspect status / log / diff against each
repo's main branch, all in one terminal session.

> **Status: M0–M5 complete.** Discovery, config, git layer, live per-repo status
> refresh (deduped, sequenced), the split-pane TUI (repo list + status +
> worktrees + log + diff), **worktree management** (create/delete with
> two-step safety, lock/unlock, prune), **log/diff inspection**, **polish**
> (cheatsheet, theming, toasts, recent repos, fullscreen), and **hardening**
> (CI matrix incl. git 2.38, leak checks, cross-compile, release flow). See
> [`docs/`](docs/) for the full research and design.

## Install & run

Requires **git ≥ 2.38** on PATH.

```sh
go install github.com/AarnoStormborn/tree-trunk@latest   # or:
brew install aarnostormborn/tap/tree-trunk               # once the tap releases
# or download a binary from the GitHub releases page
```

```sh
tree-trunk                    # scan $HOME and open the TUI
tree-trunk --list             # headless: print repo paths, one per line
tree-trunk --repo ~/a --repo ~/b --no-scan   # explicit repos only
tree-trunk --scan-root ~/src                  # scan a specific root
tree-trunk completion zsh     # shell completion
```

### Layout

```
┌ Repos (40%) ──────────────┬ 1 status │ 2 worktrees │ 3 log │ 4 diff ─┐
│ ✓ myproject   main ↑2↓1   │ On branch main                             │
│   ├─ worktree feat/x  ~   │ Changes not staged for commit:             │
│ ~ otherrepo    dev ~3 +1  │   M src/main.go                            │
│ ↻ bigrepo                 │ Untracked:                                 │
│ e brokenrepo              │   ?? newfile.go                            │
├───────────────────────────┴───────────────────────────────────────────┤
│ [s]tatus [l]og [d]iff [w]orktrees  [n]ew [D]elete  / filter          │
└ refreshing 3 repos…                                        [↑2↓1]    ┘
```

## What works (M0)

- **Discovery**: walks `$HOME` (depth-capped) with sensible skips
  (`node_modules`, `Library`, caches…), hidden-dir "peek" policy (finds
  `~/.dotfiles`, `~/.config/nvim` without walking `.config/Code/…`), opt-in
  symlink following and bare-repo detection.
- **Identity folding**: a repo discovered via its main worktree or any linked
  worktree (`.git` file with `gitdir:`) keys to the same canonical ID
  (`Abs` + `EvalSymlinks`), submodules are skipped.
- **Config**: `~/.config/tree-trunk/config.toml` (schema in
  [`docs/design/09-config.md`](docs/design/09-config.md)) — flags > config >
  defaults, `~`-expansion, `--scan-root` replaces / config adds.
- **TUI**: split layout — repo list (left, lifecycle/status tokens) + status
  detail (right); selection-driven refresh, event bridge, spinner, `/`
  filter, `tab` pane focus, `R` refresh, `?` help, `ctrl+z` suspend, `q`
  quit.
- **Refresh**: bounded worker pool, fingerprint dedup (ref reads only —
  status re-reads every poll), sequenced loads, per-repo lifecycle
  (stale/refreshing/fresh/error), optional `refresh.poll_interval_ms`.
- **Worktrees (M2)**: `git worktree list --porcelain -z` parser; create via
  form (auto-suggested `~/.worktrees/<repo>/<branch>` path, existing-vs-new
  branch semantics, remote tracking); two-step delete (safe → explicit
  `--force` confirm; branches never deleted); lock/unlock; prune; dirty `~`,
  locked `🔒`, prunable `⚠` markers; expandable worktree children in the
  repo list.

## Keybindings (M1–M3)

`j/k` move · `enter` select/focus · `tab` focus pane · `1`-`4` tabs
(status/worktrees/log/diff) · `[`/`]` cycle · `n` new worktree · `d` delete
(two-step) · `L` lock · `P` prune · `m` diff mode · `p` stat/raw · `o` open ·
`+/_` fullscreen · `ctrl+r` recent repos · `/` filter · `R` refresh · `?`
filterable cheatsheet · `ctrl+z` suspend · `q` quit

Full registry: [`docs/design/04-tui-layout.md`](docs/design/04-tui-layout.md).

## Roadmap

| Milestone | Scope                                               | Status  |
| --------- | --------------------------------------------------- | ------- |
| M0        | skeleton, discovery, flags, repo-list TUI           | ✅ done |
| M1        | status refresh, per-repo detail, dedup              | next    |
| M2        | worktree create/delete/lock/prune (two-step safety) | —       |
| M3        | log/diff streaming views                            | —       |
| M4        | theming, help cheatsheet, polish                    | —       |
| M5        | hardening, release                                  | —       |

## Docs

- [`docs/research/`](docs/research/) — four parallel research tracks (gaps,
  Go suitability, packages, lazygit/lazydocker/herdr inspiration)
- [`docs/design/`](docs/design/) — decisions (D1–D11), architecture, data
  model, git layer, TUI layout, implementation plan, config schema
