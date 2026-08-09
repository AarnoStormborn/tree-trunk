# Architecture — tree-trunk

> Design doc. Derived from `00-decisions.md` and the research tracks.
> Framework-agnostic at the top, concrete below (bubbletea per D2).

## 1. System context

```
 ┌─────────────────────────────────────────────────────────────┐
 │                        User (terminal)                      │
 │        keyboard / mouse input  ──►  tree-trunk TUI          │
 └─────────────────────────────────────────────────────────────┘
        │ input events                    │ render
        ▼                                 ▼
 ┌─────────────────────────────────────────────────────────────┐
 │                        tree-trunk (one process)             │
 │  ┌──────────┐   ┌──────────┐   ┌───────────┐   ┌─────────┐ │
 │  │  UI       │◄─►│ State    │◄─►│ Workers   │   │ Config  │ │
 │  │ (bubbletea│   │ Store    │   │ (scanner, │   │ + flags │ │
 │  │  models)  │   │ (D9)     │   │  refresh) │   │         │ │
 │  └──────────┘   └──────────┘   └───────────┘   └─────────┘ │
 │        │                │              │                    │
 │        ▼                ▼              ▼                    │
 │  ┌──────────────────────────────────────────────────────┐   │
 │  │                Git layer (internal/git)              │   │
 │  │        GitRunner interface + exec impl              │   │
 │  │        (lock-retry, cancellation, --porcelain)      │   │
 │  └──────────────────────────────────────────────────────┘   │
 └─────────────────────────────────────────────────────────────┘
        │ os/exec (bounded worker pool, context-cancelled)
        ▼
 ┌─────────────────────────────────────────────────────────────┐
 │        system `git` binary (min ≥ 2.38, D1)                │
 │        git worktree / status / log / diff / rev-parse      │
 └─────────────────────────────────────────────────────────────┘
```

**Invariants:**
- The UI thread **never** runs git commands; all git I/O happens in workers
  (lazygit's `OnWorker` + bounce-to-UI-thread pattern).
- All state lives in the `state.Store`; views are pure projections of it.
- Every long-running operation is cancellable via `context.Context`.

## 2. Package layout

```
tree-trunk/
├── cmd/tree-trunk/main.go        # flag parsing, config load, bootstrap
├── internal/
│   ├── config/                   # flags + config.toml (D7)
│   │   ├── flags.go
│   │   └── config.go             # scan_roots, ignore, worktrees.directory, theme
│   ├── discover/                 # repo scanning (D4)
│   │   ├── scanner.go            # WalkDir + .git dir/file detection
│   │   ├── roots.go              # default roots + flag/config merging
│   │   └── ignore.go             # node_modules, .git, vendor, .Trash…
│   ├── git/                      # the ONLY place that shells out (D1)
│   │   ├── runner.go             # GitRunner interface + exec impl
│   │   ├── runner_lock.go        # index.lock retry w/ backoff
│   │   ├── repo.go               # Repo methods: Status/Log/Diff/Branch
│   │   ├── worktree.go           # Add/Remove/List/Lock/Prune (--porcelain -z)
│   │   ├── worktree_parse.go     # NUL-safe parser
│   │   ├── status.go             # git status --porcelain=v1 -z parser
│   │   ├── log.go                # git log --format=… (paged)
│   │   ├── diff.go               # git diff (streamed)
│   │   ├── errors.go             # typed error codes + suggested actions
│   │   └── detect.go             # .git file/dir/bare detection
│   ├── model/                    # domain types (see 02-data-model.md)
│   │   ├── repo.go
│   │   ├── worktree.go
│   │   ├── branch.go
│   │   ├── commit.go
│   │   └── status.go
│   ├── state/                    # authoritative store (D9)
│   │   ├── store.go              # snapshot + event bus
│   │   ├── events.go             # typed events (RepoUpdated, WorktreeCreated…)
│   │   └── refresh.go            # fingerprint dedup, sequenced loads
│   └── ui/                       # bubbletea (D2)
│       ├── app.go                # root model, tea.Program bootstrap
│       ├── layout.go             # pane geometry
│       ├── models/
│       │   ├── repolist.go       # left pane (bubbles/list)
│       │   ├── statusview.go     # right pane tab: status
│       │   ├── logview.go        # right pane tab: log
│       │   ├── diffview.go       # right pane tab: diff
│       │   ├── worktrees.go      # right pane tab: worktrees (bubbles/table)
│       │   └── help.go           # filterable cheatsheet (bubbles/help)
│       ├── keys.go               # keybinding registry (single source of truth)
│       ├── popup.go              # Confirm/ConfirmIf/Toast/Alert/WithWaitingStatus
│       ├── streaming.go          # lazy line-streaming into views
│       └── theme.go              # lipgloss styles
├── test/                         # integration tests (real git sandboxes)
│   ├── sandbox/                  # test repo fixtures
│   └── tui_test.go               # subprocess-driven TUI tests
├── docs/                         # this documentation tree
└── go.mod
```

**Dependency rule** (lazygit codebase guide, adapted):
`ui → state → (git | model) → config` and `ui → git`. Lower layers never
import higher ones. `model` and `git/errors` are leaf packages.

## 3. Concurrency model

### 3.1 Worker pool

- Bounded pool of **4–16 workers** (config `workers`, default 8) for git
  commands and scan fan-out. Never unbounded: macOS `forkExec` hang
  (golang/go#61080) and Windows PATH resolution (golang/go#66586) are real.
- Implementation: `errgroup.WithContext` + semaphore, or a hand-rolled
  channel pool; every worker gets a cancellable `context.Context`.
- Ctrl+C: root context cancel → in-flight `git` processes get
  `Cmd.Cancel`/`Cmd.WaitDelay`; the TUI exits cleanly.

### 3.2 Busy/idle tracking

- Port lazygit's `Busy.md` model: a task counter increments *before* a worker
  goroutine starts, decrements when it finishes. The status bar shows a
  spinner + "N repos refreshing…" while busy. Background refreshes (D9) do
  **not** count as busy.
- Per-repo lifecycle state (herdr-style): `stale → refreshing → fresh |
  error`. Rendered as a state icon per repo row, not one global spinner.
  *(Research: 04-inspiration §3.2.5.)*

### 3.3 Refresh dedup & sequencing

Two independent reads with different dedup rules (review M2/M3):

- **Ref reads** (branch, ahead/behind, log cache): gated by a **fingerprint**
  = `git for-each-ref --format=%(objectname) refs/heads refs/remotes` **plus**
  `git rev-parse --verify HEAD` (HEAD must be explicit — detached-HEAD moves
  change no ref), joined. Unchanged fingerprint → skip these reads.
- **Status reads** (`git status --porcelain`): run on **every** refresh poll —
  staging/editing/untracked files change no ref, so fingerprint gating would
  silently hide dirtiness (the product's core signal). Status is the cheap
  30 ms op (track 2 §1.1) and must not be skipped.
- Sequenced loads: each refresh gets a monotonic seq; a result with an older
  seq than one already applied is dropped (prevents out-of-order clobbering).
  *(Research: 04-inspiration §1.4; design review M2/M3.)*

### 3.4 Refresh triggers (v1 policy; review M10)

1. **On selection change** — moving to a repo (or expanding it) triggers a
   status + ref refresh of that repo (focused-repo priority).
2. **Manual `R`** — full re-scan + refresh of all repos.
3. **Optional periodic poll** — per-repo `git status` on a configurable
   cadence (`refresh.poll_interval_ms`, default 0 = off in v1) for repos
   currently visible/selected; ref reads stay fingerprint-gated.
4. No fsnotify in v1 (P1). Staleness is visible: a repo whose last refresh
   is older than the poll interval (or never refreshed) shows a `stale`
   indicator rather than silently showing old data.

## 4. State store (D9)

```go
type Store struct {
    mu     sync.RWMutex
    repos  map[string]*model.Repo   // key: common git dir (see data model)
    events chan Event               // buffered, non-blocking fan-out
    seq    atomic.Uint64
}

type Event interface{ Kind() EventKind }   // RepoUpdated, WorktreeAdded,
                                            // WorktreeRemoved, RefreshStarted,
                                            // RefreshFinished, ScanComplete, Error
```

- **Snapshot + events:** on boot the scanner produces the initial snapshot;
  refreshers emit deltas. The UI renders on change; a future `--json` mode
  (F3) prints the snapshot, a future socket mode (F5) streams events.
- Workers never touch the UI; they mutate the store and emit events. The
  bubbletea model receives events via a `tea.Cmd` that polls the event
  channel (or a `tea.Batch` bridging goroutine).

## 5. Startup sequence

1. Parse flags (D7), load `config.toml` if present.
2. Verify `git` on PATH + version ≥ 2.38 (single `MinGitVersion`
   constant, D1/Q2); exit with a clear error otherwise.
3. Build `state.Store`, start bubbletea program with a splash/spinner model.
4. Kick off scanner worker (`tea.Cmd`): merge `--repo` flags + `--scan-root`
   flags + config roots + default roots; emit `ScanComplete` event.
5. As repos are found, enqueue initial status refresh per repo (bounded pool,
   fingerprint dedup).
6. UI becomes interactive immediately; rows appear incrementally as results
   land. *(Research: 02-go-suitability §1.1 measurement: warm `git status`
   ≈ 30 ms/repo → 100 repos ≈ 3 s serial, ~0.5 s with 8 workers.)*

## 6. Error handling

- `git/errors.go` defines typed errors with **codes + suggested actions**
  (lazydocker `ComplexError` pattern):
  `WorktreeDirty → "remove refused; offer --force confirm"`,
  `BranchCheckedOutElsewhere → "jump to that worktree"`,
  `NotARepo`, `GitTooOld`, `GitNotFound`, `LockTimeout`.
- UI maps codes to toasts / confirm dialogs / status messages.
  *(Research: 04-inspiration §2.5.5, §6 P1-16.)*

## 7. Performance budget (v1 targets)

| Op | Budget |
|---|---|
| App startup to interactive | < 300 ms (no git calls before first render) |
| Initial scan (100 repos) | < 2 s wall, incremental display |
| Full refresh of 100 repos | < 1 s with 8 workers (status re-read every poll; ref reads fingerprint-skipped) |
| log/diff open on selection | < 200 ms perceived (spinner + streaming) |
| Memory | < 100 MB RSS; never load full histories |

Measured facts backing this: `git status --porcelain` warm ≈ 0.03 s, cold ≈
0.32 s, exec overhead ≈ 0.01 s on Apple Silicon. *(02-go-suitability §1.1.)*

## 8. Risks (architectural)

| Risk | Mitigation |
|---|---|
| Goroutine/process leaks in long-running TUI | errgroup + contexts; `runtime.NumGoroutine` assertions in tests; leaktest-style checks (02 §7.3) |
| Multi-repo refresh storms on huge homes | fingerprint dedup + debounced fs-watch (P1) + capped concurrency + focused-repo priority (04 §7) |
| Git version skew in `--porcelain` parsing | defensive parser; documented min version 2.38; integration tests against 2.38 + latest (02 §7.6; review B1) |
| bubbletea v2 migration debt | pin v1; keep UI models thin over state store; isolate tea usage in `internal/ui` (03 §1.1) |
| `index.lock` races between own refreshes and worktree ops | lock-retry runner (20 ms → ~1 s, 7 retries) (04 §1.3, §6 P0-1) |
