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

### M2 — Worktree management (the core) (M–L)

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

**Exit:** full create/delete cycle on a real repo with confirmations; force
path requires explicit confirm; branches never deleted; dirty worktrees
refused without force.

### M3 — Log, diff, streaming inspection (L)

**Goal:** lazygit-grade log/diff views against the selected repo and its main
branch (product requirement #4).

- [ ] `internal/git/log.go` paged log + parser (03 §4.2)
- [ ] `internal/git/diff.go` streamed diff (03 §3.3) incl. **per-file variants**
  (review M9: untracked placeholder, rename old-path)
- [ ] `internal/ui/streaming.go`: single-flight, lazy line reads, 30 ms throttle, cancel-previous (04-inspiration §1.3; 03 §7)
- [ ] UI: log view with "load more"; diff view with stat/raw toggle and main-branch mode; commit→diff; status-file→diff
- [ ] **Q6 decision at M2 review** (diff modes); **hold M3 scope to stat/raw toggle only** (review feasibility #2)
- [ ] **Tests:** log parser fixtures (newlines in subjects!), streaming cancellation, diff modes integration

**Exit:** scrolling a large repo's log/diff is smooth, memory bounded, no
frozen UI; switching repos cancels in-flight streams.

### M4 — Polish (M)

- [ ] `?` filterable help cheatsheet from the key registry (04 §3)
- [ ] Theming: palette, light/dark, `NO_COLOR` (04 §8)
- [ ] Toasts/alerts/confirm polish, destructive styling, disabled-with-reason hints
- [ ] Recent-repos persistence (`ctrl+r`)
- [ ] Fullscreen toggle, terminal-size edge cases, non-UTF8 path handling
- [ ] `--version`, man-ish `--help`, shell completion (P1 if time)

### M5 — Hardening & release (M)

- [ ] Integration test suite on **git 2.38 + latest** (03 §8; review B1)
- [ ] Performance pass on a real large home dir (100+ repos): budgets in 01-architecture §7
- [ ] Goroutine-leak checks (`runtime.NumGoroutine` assertions), `go vet`, race detector (`-race`) in CI
- [ ] Cross-compile matrix (macOS arm64/amd64, Linux amd64/arm64; Windows builds best-effort)
- [ ] Homebrew tap + `go install` release flow
- [ ] README with screenshots, keybinding cheatsheet, docs index update
- [ ] bubbletea v2 migration note review (D2 rationale stays v1; keep the
  upgrade-guide item alive, 02-go-suitability §8)

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
