# Implementation Plan — tree-trunk

> Design doc. Milestones, testing strategy, and delivery checklist. Estimated
> sizes are relative (S/M/L) and assume one experienced Go developer.

## 1. Milestones

### M0 — Skeleton, flags, discovery (S–M) — ✅ COMPLETE 2026-08-09

**Goal:** `tree-trunk --repo …` and a bare `tree-trunk` scan print a plain-text
repo list (`--list`); TUI shell renders rows.

- [x] `go mod init` (`github.com/harshsingh/tree-trunk`), package layout
  (01-architecture §2); golangci-lint config deferred to M5 (CI skeleton)
- [x] `internal/config`: flags (D7) + `config.toml` load (schema per
  `09-config.md`; `~`-expansion via `os.UserHomeDir()`; clamping + strict
  TOML decode with unknown-key warnings)
- [x] `internal/discover`: custom walker — `.git` dir/file/bare detection,
  submodule discrimination (`/.git/modules/`), roots, ignore segments +
  trailing suffixes, depth cap, hidden peek/skip/scan, opt-in symlink
  following with loop protection (02-data-model §2, 09 §2.3)
- [x] `internal/git`: `Runner` interface + `ExecRunner` (cancellation,
  `WaitDelay`, `GIT_OPTIONAL_LOCKS=0`, index.lock retry with backoff),
  `LookPath` + `CheckVersion` (≥ 2.38, `MinGitVersion`), `Resolve` with
  identity canonicalization (`--path-format=absolute` + `EvalSymlinks`)
- [x] `internal/model`: `Repo`/`Worktree`/`Branch`/`Commit`/`RepoStatus` +
  status glyph summary (02-data-model §1–§3)
- [x] `internal/state`: Store + event bus, lifecycle preservation on replace,
  nested-scan counting (D9)
- [x] `internal/ui`: bubbletea root model; left-pane repo list renders
  scanned repos with tokens; spinner, `/` filter, `R` re-scan, `?` help,
  `ctrl+z` suspend, `q` quit
- [x] **Tests:** scanner unit (`.git` file worktrees, dotfiles repos,
  symlinked roots + loop protection, bare, depth cap, suffix ignore),
  **main-vs-linked same-ID fixture** (review M1) + symlink canonicalization,
  runner/version/resolve integration against sandboxed repos, config
  precedence + `~`-expansion, store events/sorting/lifecycle — all green
  under `-race`

**Exit (met):** scanning a real dir with main + linked worktrees streams into
the list (verified live in a herdr pane); `--repo` (incl. linked-worktree
paths), `--no-scan`, `--scan-root`, `--list` all work; identity folding
verified end-to-end (3 hits → 2 repos); quitting is clean. Scanning a real
`$HOME` still to be measured (M1 will do the performance pass).

### M1 — Status refresh + repo detail (M) — ✅ COMPLETE 2026-08-09

**Goal:** per-repo live state; right pane shows status; refresh dedup.

- [x] `internal/git/status.go` porcelain v1 `-z` parser + `--branch` header
  (renames, conflicts, untracked, ahead/behind, detached HEAD)
- [x] `internal/git/fingerprint.go` — for-each-ref + explicit `rev-parse
  HEAD` (review M3); verified: unchanged on working-tree edits, changes on
  commit AND on detached-HEAD moves
- [x] `state/refresh.go`: bounded-pool refresher, fingerprint stored as
  `RefState`, **sequenced loads** (stale results dropped — tested with a
  blocking runner), lifecycle stale/refreshing/fresh/error, bare-repo skip
- [x] UI: token rows with real branch/status/ahead-behind (02 §3), status
  detail pane (04 §5.1) with conflicts/staged/unstaged/untracked sections,
  split layout 40/60, selection-driven refresh + event bridge + `R` +
  optional `refresh.poll_interval_ms` poller
- [x] `R` refresh, `/` filter, `tab` pane focus
- [x] **Tests:** status parser fixtures, fingerprint semantics, refresh
  dedup/sequencing/lifecycle (fake runner), status integration on sandbox —
  all green under `-race`; live-verified in a herdr pane

**Exit (met):** a sandbox scan shows every repo with branch/status/ahead-behind
and lifecycle icons; selection updates the status pane live (verified: dirty
repo shows `M f.txt` + `?? untracked.txt`; clean shows "nothing to commit");
refresh storms are deduped (fingerprint + sequenced loads). Real `$HOME` scan
measurement deferred to the M5 performance pass.

### M2 — Worktree management (the core) (M–L) — ✅ COMPLETE 2026-08-09

**Goal:** create / delete / lock / unlock worktrees with the two-step safety
flows.

- [ ] `internal/git/worktree.go` + `worktree_parse.go` (porcelain `-z` parser)
- [ ] `CheckedOutByOtherWorktree` guard; create flow (existing vs new branch,
  `--guess-remote` for remote-only branches — review m6; base ref, D8 path
  template + slug spec)
- [ ] Two-step remove (safe → force confirm); lock/unlock; prune with **UI
  binding** (review M6: dry-run preview + confirm)
- [ ] UI: worktrees tab (04 §5.4), create form (04 §4), repo-row expandable
  worktree children (+ dirty icons, review M5), `n`/`d`/`o`/`u`/`L` bindings
  per final registry (08-reviewer-drafts.md Draft B)
- [ ] Error codes → dialogs/toasts (03 §6)
- [ ] **Tests:** worktree parser fixtures; create/remove/lock integration against sandbox repos; the two-step force flow end-to-end; branch-checked-out-elsewhere guard

**Exit (met):** full create/delete cycle on a real repo with confirmations;
force path requires explicit confirm; branches never deleted; dirty worktrees
refused without force; the TUI marks dirty worktrees `~`, locked `🔒`,
prunable `⚠`/`[missing]`; prune cleans immediately (verified live).

### M3 — Log, diff, streaming inspection (L) — ✅ COMPLETE 2026-08-09

**Goal:** lazygit-grade log/diff views against the selected repo and its main
branch (product requirement #4).

- [x] `internal/git/log.go` paged log + parser (03 §4.2: no trailing `%x00`,
  `-z` record terminators, empty-subject safe); `--skip` paging
- [x] `internal/git/diff.go` (03 §3.3): working / staged / vs-main / commit /
  per-file modes; `--no-color --no-ext-diff --no-textconv`; 2 MiB cap;
  **untracked placeholder** (review M9); `git show --format=` for commit
  diffs — handles root commits (`git diff <c>^ <c>` fails; `git diff --root`
  is a no-op on git 2.39); MainBranch detection (origin/HEAD → main →
  master, Q6)
- [x] Cancellation: diff/log run as tea.Cmds; stale results dropped by repo
  ID (single-flight, cancel-previous semantics, 03 §7; full lazy line-read
  streaming remains P1 — viewport covers scrolling)
- [x] UI: log tab (`3`) with paged commits, auto-select newest, load-more at
  scroll end; diff tab (`4`) with `m` mode cycle (working→staged→vs main)
  and `p` stat/raw; commit→diff; status-file→diff (selectable rows, review
  M9); `c` copy hash; `w` on a commit opens the create form pre-filled with
  the commit as base
- [x] **Q6 resolved**: diff modes are toggles (`m` cycle), per design option
  (a); **scope held to stat/raw** (no hunk folding)
- [ ] **Tests:** log parser fixtures (newlines in subjects!), streaming cancellation, diff modes integration

**Exit (met):** log/diff render fast for typical repos with a 2 MiB cap;
stale diffs are dropped when switching repos; root-commit diffs work; the
full loop (status → file diff → log → commit diff → vs-main) verified live
in a herdr pane.

### M4 — Polish (M) — ✅ COMPLETE 2026-08-09

- [x] `?` filterable help cheatsheet from the key registry (04 §3; herdr-style
  `/` filter)
- [x] Theming: `theme.go` palette (normal/dim/accent/dirty/conflict/clean/
  worktree_child + selection), light/dark variants, `theme.overrides` hex,
  `NO_COLOR` (04 §8)
- [x] Toasts (3s auto-clear) for action results + errors; confirm dialogs
  keep destructive styling; disabled-with-reason deferred to M5 hint bar
- [x] Recent-repos persistence (`ctrl+r`): `~/.local/state/tree-trunk/
  state.json`, MRU cap 20, save on quit, jump-to-repo menu
- [x] Fullscreen toggle (`+/_`, hides the repo list); non-UTF8 path
  sanitization in rendering (04 §9)
- [x] `--version` / `--help` (flag.Usage); shell completion deferred (P1)

### M5 — Hardening & release (M) — ✅ COMPLETE 2026-08-09

- [x] CI (`.github/workflows/ci.yml`): test matrix ubuntu+macOS, **git 2.38
  built from source** (min-version leg, 03 §8 / review B1) + system git;
  `go vet` + gofmt gate; `go test -race ./...`; cross-compile job
  (darwin/amd64+arm64, linux/amd64+arm64, windows/amd64 best-effort, D6)
- [x] Performance pass (measured on a real home dir): 21-repo scan ≈ **1.1 s**
  (budget <2 s/100 repos); serial status of all ≈ 0.36 s → parallel well
  under the <1 s budget; cold git status ≈ 12 ms. Scanner benchmark added
  (≈ 4.5 ms / 50-repo synthetic tree)
- [x] Goroutine-leak test (`TestNoGoroutineLeaks`: scan+refresh cycle returns
  to baseline), `-race` everywhere in CI; `go vet` + gofmt gates
- [x] Cross-compile matrix via `make cross` (verified locally: all 5 targets)
- [x] Release flow: `.goreleaser.yaml` (binaries + checksums + changelog +
  **Homebrew tap** `AarnoStormborn/homebrew-tap` formula) + `.github/workflows/
  release.yml` (tag `v*` → goreleaser); `go install` works (module path is
  the real remote); Makefile targets build/test/race/vet/fmt/cross/install/
  bench
- [x] README: install (go install / brew / binaries), ASCII layout preview,
  keybinding cheatsheet, roadmap, docs index
- [x] Shell completion: `tree-trunk completion zsh|bash` (M4 backlog)
- [x] bubbletea v2 migration note: kept alive in D2 rationale (00-decisions);
  UI models stay thin over the state store so a v2 port stays cheap

## 2. Testing strategy

### Layers (03-go-packages §5.2, 04-inspiration §1.1 busy model)

| Layer | Technique | Coverage |
|---|---|---|
| Parsers (`internal/git/*_parse.go`) | table-driven unit tests on fixture bytes | status/log/worktree records incl. edge cases |
| Git ops | **sandboxed real git** in `t.TempDir()` — never mock git | create/remove/lock flows, guards, version matrix |
| State/refresh | fake `Runner` (interface, 03 §2) + real store | dedup, sequencing, lifecycle transitions |
| UI models | pure Model/Update tests (bubbletea: send msgs, assert state) | keybindings, context stack, form validation |
| Full TUI | **subprocess-driven**: build binary, run under a PTY, send keys via `tea` test driver or `teatest` golden files | happy-path E2E (scan → select → status → create → remove) |
| Busy/idle | task-counter assertions (lazygit `Busy.md`) | no premature quit while refreshing |

- `teatest` is experimental (`charmbracelet/x/exp/teatest`) — use for golden
  smoke tests only; keep business logic in testable pure functions so a
  teatest API break costs little (03 §5.2).
- CI: `-race`, `go vet`, golangci-lint, integration jobs on git 2.38 + latest.

### Sandbox fixture catalog (03 §8)

normal repo · repo with linked worktrees (`.git` file) · bare repo · dirty
worktree · locked worktree · branch checked out in two worktrees · rename
records · conflict status · detached HEAD · worktree with missing path
(prunable) · repo with non-UTF8 filename · submodule-containing repo (skip
assertion).

## 3. Delivery checklist (definition of done, per milestone)

1. Feature works against real repos in a sandbox + a real home dir.
2. Unit + integration tests green (incl. `-race`).
3. No goroutine leaks on quit (asserted in tests).
4. Docs updated: this plan's checkbox list, README, keybinding cheatsheet.
5. Performance budgets (01-architecture §7) met or consciously deferred.

## 4. Sequencing rationale

- **Discovery before git ops:** the list is the product's spine; worktrees
  depend on repo identity (02-data-model §2).
- **Status before log/diff:** status is the cheapest win and builds the
  refresh/store machinery that log/diff reuse.
- **Worktrees (M2) before inspection depth (M3):** worktree management is the
  named differentiator; inspection can lean on lazygit-proven patterns later.
- **Prototype repo→repo navigation early (M1/M2 boundary):** research flags
  it as the unproven UX (04 §7); a spike before M2 locks the context-stack
  design. **Hard gate:** treat the spike output as a gate for M2 — if the
  navigation model fails, rework happens before M2 work starts (review
  feasibility #1).

## 5. Risks & mitigations (implementation-phase)

| Risk | Mitigation |
|---|---|
| Repo-list→detail navigation feels wrong | early spike (M1/M2); context-stack design in 04 §2; iterate on real home dirs |
| `git status` refresh cost on 100+ repos | fingerprint dedup (M1), debounced fs-watch P1, focused-repo priority, budget in 01 §7 |
| `--porcelain` format drift across git versions | version matrix in CI (03 §8), defensive parsing, min-version gate |
| bubbletea v1 API surprises (viewport/list) | components are stock bubbles; isolate in `internal/ui` |
| Worktree ops corrupt state (force remove) | two-step confirm, never-delete-branches, lock-awareness, E2E tests |
| Scope creep toward batch worktrees (F1) | D3 decision recorded; batch stays out of v1 unless re-opened by owner |

## 6. Effort estimate (rough)

| Milestone | Relative size |
|---|---|
| M0 skeleton + discovery | S–M |
| M1 status + refresh | M |
| M2 worktrees | M–L |
| M3 log/diff streaming | L |
| M4 polish | M |
| M5 hardening/release | M |
| **Total** | **~L (≈ 4–8 focused weeks)** |
