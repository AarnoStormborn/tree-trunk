# tree-trunk

**TUI for all your git repos & worktrees.** List every repo on your machine,
create and delete git worktrees, and inspect status / log / diff against each
repo's main branch, all in one terminal session.

> **Status: M1 complete.** Discovery, config, git layer, live per-repo status
> refresh (deduped, sequenced), and the split-pane TUI (repo list + status
> detail) all work. Worktree management (M2) is next. See [`docs/`](docs/) for
> the full research and design.

## Install & run

Requires **Go 1.26+** and **git ≥ 2.38** on PATH.

```sh
go build -o tree-trunk ./cmd/tree-trunk
./tree-trunk                 # scan $HOME and open the TUI
./tree-trunk --list          # headless: print repo paths, one per line
./tree-trunk --repo ~/a --repo ~/b --no-scan   # explicit repos only
./tree-trunk --scan-root ~/src                   # scan a specific root
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

## Keybindings (M0 subset)

`j/k` move · `enter` select · `/` filter · `R` re-scan · `?` help ·
`ctrl+z` suspend · `q`/`ctrl+c` quit

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
