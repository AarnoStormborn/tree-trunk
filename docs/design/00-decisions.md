# Design Decisions — tree-trunk

> **Status:** Proposed for review. Each decision cites the research that
> justifies it (see `docs/research/`). Decisions marked **(user)** were made
> by the project owner on 2026-08-09; the rest follow directly from research.

---

## D1. Language: Go — shell out to the system `git` binary

- **Decision:** Implement in Go. All git interaction shells out to the real
  `git` binary via `os/exec`. **go-git is out of scope** (even for reads, v1).
- **Why:** Go is the proven language for this exact niche (lazygit ~81k★,
  lazydocker ~52k★) — static binaries, trivial cross-compilation, goroutines
  map perfectly to scan-N-repos fan-out. `git worktree` support in go-git is
  experimental (`v6/x/plumbing/worktree`), incomplete (**only `Add`; no
  `remove`/`prune`/`lock`**), and slower (decompresses objects on checkout).
  Shelling out gets full, always-correct worktree semantics for free.
  *(Research: 02-go-suitability §1–3, 03-go-packages §2.)*
- **Costs accepted:** `git` must be on PATH (fine — the user is a git
  workflow tool); output parsing discipline (`--porcelain`, `-z`); bounded
  concurrency around process spawning (macOS `forkExec` hang, golang/go#61080).
- **Minimum git version:** require **≥ 2.38** **(user, 2026-08-09)**. Cleaner
  `--porcelain` parsing guarantees; Apple bundled git (2.39+) and Homebrew
  qualify; older distro git is out of scope. *(02-go-suitability §7.6; design
  Q2.)*

## D2. TUI framework: bubbletea v1.3.x (stable) + bubbles v1 + lipgloss v1

- **Decision:** `charmbracelet/bubbletea` **v1.3.x** (pin), `bubbles` v1.0.0,
  `lipgloss` v1.1.0. Do **not** start on bubbletea v2 (breaking release,
  Feb 2026; most ecosystem docs/examples target v1). Treat v2 as a future
  migration with an upgrade-guide review.
- **Why:** Elm-style Model/Update/View matches the state-store + event design
  (D9). The repo list / worktree table / log viewport / help overlay map to
  stock bubbles components. **(user)** — chosen over tview and gocui.
- **Do not use:** `jroimartin/gocui` (unmaintained; lazygit maintains an
  in-house fork — not a safe dependency for us).
  *(Research: 03-go-packages §1, 04-inspiration §1.1.)*

## D3. Worktree scope: per-repo operations, machine-wide overview

- **Decision (user):** v1 creates/deletes worktrees **per selected repo**,
  within a TUI that lists **all repos on the machine**. No batch
  "same-branch-across-N-repos" action in v1.
- **Why:** The gap analysis shows the whitespace is the *overview* —
  machine-wide repo index + live state + per-repo inspection — not batch
  worktree creation (only two-week-old hobby tools do that, and it's riskier).
  Architecture keeps the door open for batch later (D10 state store).
  *(Research: 01-existing-tooling §6, 04-inspiration §1.5.)*

## D4. Repo discovery: zero-config scan + explicit flags

- **Decision (user):** Scan **`$HOME` only** by default (bounded by
  `discover.max_depth`), with sensible skips; support explicit `--repo PATH`
  flags (repeatable), `--scan-root DIR` overrides, and a config file for
  custom roots/ignores. No registration required (contrast: ghq/mr/gita).
  *(design Q1: home-only scan.)*
- **Why:** "List all git repos" without setup is the differentiator; the
  `--repo` flag path is something no existing tool has. Scanning is a bounded
  `filepath.WalkDir` + `.git` dir/file detection — ~50 lines, no dependency.
  *(Research: 01-existing-tooling §4, §6.2.4; 03-go-packages §3.)*

## D5. Inspection: native log / diff / status views

- **Decision (user):** Build native views inside tree-trunk — `git status`,
  `git log`, and `git diff` rendered in-tree, lazy-loaded. **No
  open-in-lazygit delegation** in v1 (delegation may appear later as a
  keybinding).
- **Why:** Single-session UX across repos is the product. Lazy-loading +
  single-flight streaming (lazygit `tasks` pattern) keeps it fast without
  loading full histories (lazygit used 2.6 GB on the Linux repo).
  *(Research: 04-inspiration §1.3–1.4, §6 P0-10.)*

## D6. Platforms: macOS + Linux first-class; Windows best-effort

- **Decision (user):** v1 targets macOS + Linux. Windows compiles
  (`CGO_ENABLED=0`) but is untested/best-effort until a later milestone.
- **Why:** Windows carries the most terminal/git quirks (ConPTY, path limits,
  git skew); QA time is better spent on core UX first.
  *(Research: 02-go-suitability §7.5; 01-existing-tooling §2.2.9.)*

## D7. Config & CLI: stdlib `flag` + single TOML file

- **Decision:** CLI flags via stdlib `flag` (or `spf13/pflag` if POSIX-style
  `--flag=value` ergonomics matter); config via `~/.config/tree-trunk/
  config.toml` parsed with `BurntSushi/toml`. **No cobra, no viper** (weight
  and global state not justified for a single-purpose TUI).
- **v1 flags:** `--repo PATH` (repeatable, add explicit repos), `--scan-root
  DIR` (repeatable, **replaces** default roots), `--no-scan` (repos from flags
  only), `--config PATH`, `--list` (headless: print repo paths one per line —
  M0 exit criteria; `--json` stays deferred F3), `--version`, `--help`.
  *(Research: 03-go-packages §4.)*

## D8. Worktree paths: `~/.worktrees/<repo-name>/<branch>` default layout

- **Decision (user):** Default worktree destination for created worktrees:
  `~/.worktrees/<repo-basename>/<branch-slug>` (herdr-style grouping), made
  **configurable** via `[worktrees] directory` (global) and per-repo override
  in config (syntax in `08-reviewer-drafts.md` / config schema). Creation
  always shows the target path in the confirm dialog.
- **Branch-slug spec (review m7):** lowercase the branch name; replace `/`
  with `-`; strip characters outside `[a-z0-9._-]`; collapse repeated `-`;
  reject empty results, `.`, `..`, and names starting with `-`; cap at 80
  chars; on collision with an existing path, append `-2`, `-3`, … and show
  the final path in the confirm dialog. *(design Q3.)*
- **Why:** Every serious worktree manager ships a path template; a single
  root keeps `find`-style scans sane and mirrors herdr's
  `<dir>/<repo>/<branch-slug>` convention.
  *(Research: 01-existing-tooling §7, 04-inspiration §3.2.)*

## D9. State model: single authoritative store + events

- **Decision:** One `state.Store` holding the app model (repos, worktrees,
  per-repo status, refresh lifecycle). Background workers mutate the store
  and post change events; the bubbletea model subscribes and re-renders.
  Snapshot + event deltas (herdr's `session.snapshot` + `events.subscribe`
  pattern, in Go).
- **Why:** This is the foundation for background refresh, per-repo spinners,
  toasts, and a future `--json`/socket mode. *(Research: 04-inspiration
  §3.2.4, §6 P0-3.)*

## D11. "Open worktree" = print path + copy to clipboard

- **Decision (user):** v1 `o` prints the worktree path and copies it to the
  clipboard (no shell spawn, no lazygit launch — those are post-v1
  candidates). **Clipboard mechanism (review m8):** `charmbracelet/x/clipboard`
  (pure Go; consistent with the charm stack). **Degradation:** on failure
  (headless Linux, WSL/SSH, no clipboard binary) fall back to print-only +
  toast; never fail the action because the clipboard is unavailable.
  *(design Q7.)*

## D10. No daemon in v1

- **Decision:** Single-process TUI. No herdr-style client/server split, no
  socket API in v1. The state store (D9) keeps that door open.
  *(Research: 04-inspiration §3.4, [OPINION] note.)*

---

## Decisions deferred (post-v1 candidates)

| # | Item | Research ref |
|---|---|---|
| F1 | Batch worktree create/cleanup across N repos | 01-existing-tooling §6.2.2, §8.1 |
| F2 | Custom command DSL (user git recipes bound to keys) | 04-inspiration §6 P1-14 |
| F3 | `--json` machine-readable output | 04-inspiration §6 P1-15 |
| F4 | Reflog-based undo (`z`/`Z`) | 04-inspiration §6 P2-18 |
| F5 | Daemon/socket mode | 04-inspiration §6 P2-19 |
| F6 | Worktree bootstrap hooks/plugins (herdr `worktree.created`) | 04-inspiration §6 P2-20 |
| F7 | Windows first-class support + QA | 02-go-suitability §7.5 |
| F8 | Bare-clone (grove-style) managed repos | 01-existing-tooling §7, §8.4 |
