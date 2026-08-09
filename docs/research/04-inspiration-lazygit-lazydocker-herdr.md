# Research Track 4 — Inspiration: lazygit, lazydocker, herdr (+ gitui / tig)

> **Scope**: Deep dive into the three named projects plus quick looks at gitui and
> tig. Goal: extract architecture, UX patterns, process-management techniques,
> and performance tricks that `tree-trunk` (Go TUI for listing git repos +
> managing worktrees + inspecting log/diff/status) should borrow.
>
> **Method**: Primary sources only where possible — the actual repos were cloned
> locally (`/tmp/pi-github-repos/jesseduffield/lazygit`, `/tmp/lazydocker`,
> `/tmp/pi-github-repos/extrawurst/gitui`), docs fetched from `herdr.dev`, and the
> herdr CLI was inspected **live** (this research session runs inside herdr, env
> `HERDR_ENV=1`, session `grove`). Star counts / languages verified via GitHub API
> on 2026-08-09.
>
> **Verified status snapshot (2026-08-09)**:
>
> | Project | Language | Stars | Last push | Notes |
> |---|---|---|---|---|
> | jesseduffield/lazygit | Go | 81,162 | 2026-08-09 | very active |
> | jesseduffield/lazydocker | Go | 52,369 | 2026-04-19 | active, slower than lazygit |
> | herdrdev/herdr | Rust | 26,201 | 2026-08-09 | very active, Apache-2.0 |
> | gitui-org/gitui | Rust | 22,352 | 2026-08-04 | active |
> | jonas/tig | C | 13,301 | 2026-07-27 | classic, slow-moving |

**Legend for this doc**: `[FACT]` = verified against source/docs; `[OPINION]` = my
analysis / judgment for tree-trunk; `[MEASURED]` = measured in this session.

---

## 0. TL;DR — what each project teaches us

| Project | One-line lesson | What tree-trunk should take |
|---|---|---|
| **lazygit** | The gold standard for *single-repo* git TUIs: context-sensitive single-key bindings with an on-screen hint bar, panels that stream command output lazily, and a strict separation of git-command layer from UI. | Keybinding model, panel layout, `tasks` streaming machinery, refresh dedup, worktree-loading code, error toasts, per-view search/filter. |
| **lazydocker** | The closest architectural sibling to tree-trunk's *entity-list* problem (containers/services/images ≈ repos/worktrees/branches): multiple side lists + one main streaming panel, ticker-based log tailing, tabbed side panels. | Entity abstraction (`Project/Container/Service/Image`), log-stream ticker, tabbed side windows, custom command templates, two-step destructive confirmations. |
| **herdr** | A *multi-workspace* tool with a real model for grouping (workspace → tab → pane), a client/server split with a socket API, agent state rollups in a sidebar, and **first-class git worktree management over its API** (create/open/remove, grouped workspaces, two-step forced-remove confirmation). Also: how a multiplexer thinks about layouts (BSP tree, export/apply). | The worktree → workspace grouping model, sidebar rollup of per-repo state (branch, ahead/behind, dirty), two-step `git worktree remove` confirm, session persistence thinking, events/snapshot state model. |
| **gitui** (quick) | Performance-first: async git jobs on a thread pool with channel notifications; beats lazygit 2.4× and tig 10× on a 900k-commit repo (24 s / 57 s / 4 m 20 s). | Async job layer + debounced file-watching; don't block the UI thread on git calls; avoid loading whole histories into memory. |
| **tig** (quick) | The minimal modal view model: tiny binary (0.6 MB), views + `watch`-based refresh, everything shelled out to git. | Simplicity benchmark; modal view switching (status/log/diff) as an alternative layout to panels. |

---

## 1. lazygit (jesseduffield/lazygit)

### 1.1 Architecture

**[FACT]**

- **Language**: Go 1.25 (`go.mod`). One of the most-starred Go TUI apps (81k stars).
- **TUI framework**: a **vendored fork of `jroimartin/gocui`** (`vendor/github.com/jesseduffield/gocui`, built on `gdamore/tcell/v3`), plus `jesseduffield/lazycore` (utilities/logging). Note: upstream gocui is effectively unmaintained; lazygit maintains its own fork. This matters for track 3 (go-packages): **gocui is not a safe direct dependency; bubbletea/tview are the live alternatives.**
- **Package layout** (from `docs/dev/Codebase_Guide.md` and repo structure):
  - `pkg/app` — startup, config load, error handling, and a short-lived "daemon" used as `GIT_EDITOR` for interactive rebase TODO files (not a long-running daemon).
  - `pkg/commands/git_commands` — **all** communication with the git binary: `StatusCommands`, `WorktreeCommands`, `BranchCommands`, `CommitCommands`, etc. Each returns typed models (`pkg/commands/models`).
  - `pkg/commands/oscommands` — OS command invocation wrapper (`CmdObj`), process management.
  - `pkg/tasks` — streaming command output into a view (see §1.3/§1.4).
  - `pkg/gui` — the UI, organised as **View → Context → Controller → Helper**:
    - **View**: a gocui buffer region of the screen.
    - **Context**: a view + per-view state and logic (e.g. `local_commits_context`).
    - **Controller**: a set of keybindings + handlers; one controller serves many contexts (e.g. the list-navigation controller is attached to every list context); one context can host many controllers.
    - **Helper**: shared code across controllers (refresh, popups, staging, worktree).
    - Dependency rule (from the codebase guide): controllers → helpers → contexts → views; views never reference up.
  - `pkg/gui/context.go` — a **context stack** manages focus and back-navigation (escape pops transient states).
  - `pkg/config`, `pkg/i18n` (all user-facing strings translated; keybinding docs auto-generated from `pkg/i18n` via `pkg/cheatsheet`), `pkg/theme`, `pkg/updates`, `pkg/utils`.
- **Event/render loop**: gocui's `MainLoop` processes a keypress/resize event, then redraws by calling `pkg/gui/layout.go`'s `layout()`, which re-derives window dimensions (`getWindowDimensions` → `WindowArrangementHelper`) and re-renders the changed views. Worker goroutines must bounce back to the UI thread via `self.c.OnUIThread(...)`.
- **Busy/idle model** (`docs/dev/Busy.md`): gocui keeps a map of `Task`s. `OnWorker(f)` creates a task *before* spawning the goroutine, so the app is considered "busy" from before the worker starts until it finishes; integration tests wait for busy→idle transitions. Background goroutines (e.g. periodic `git fetch`) do **not** count as busy.

### 1.2 UX patterns to steal

**[FACT]**

- **Layout**: side panel(s) + main content + bottom bar. Tabs live *inside* windows (Files / Worktrees / Submodules / Local branches / Commits / Tags / Stash / Remotes / Reflog / Status), toggled with `[` / `]` or numbered. Screen modes: normal / half / fullscreen (`+` / `_`). Status bar: `appStatus` + `options` views at the bottom; the `informationStr()` line shows version/links, replaced by mode labels during rebase etc.
- **Keybinding model**: **single-key, context-sensitive**. Each context defines its keybindings; global bindings always apply. The bottom `options` view renders the *current context's* available keys (the "hint bar") — "we use the default options fg color for most keybindings… only use a different color if we're in a specific mode where the user is likely to want to press that key" (e.g. cherry-pick paste is highlighted). `?` opens the full keybinding menu (generated cheatsheet). Disabled bindings show *why* (VISION.md: "If a keybinding is disabled, give a reason why").
- **Notable keys**: `space` stage, `c` commit, `s` squash, `ctrl+z` undo, `z`/`Z` undo/redo **via the reflog**, `R` refresh (re-runs git status/branch in background), `ctrl+r` recent repos, `/` search/filter, `esc` cancels transient states (rebasing, diffing), `q`/`ctrl+c` quit, `ctrl+z` suspends the TUI (goes back to shell).
- **Help/discoverability**: tooltips explaining what an action will do; prominent "YOU ARE HERE" markers mid-rebase; toasts when an action's effect isn't visible on screen (VISION.md).
- **Filtering/search** (`docs/Searching.md`): `/` opens a per-view search *or* filter prompt (search = highlight + `n`/`N` iteration; filter = view shrinks to matches). Intent is per-view choice (files → filter; commits → search). Also `<c-s>` commit filtering by file path, `<c-b>` staged/unstaged file filter, `v` range select.
- **Confirmation flows**: `PopupHandler` (in `pkg/gui/popup/popup_handler.go`) provides `Confirm(opts)` / `ConfirmIf(condition, opts)`, `Alert`, `Menu`, `Toast` / `ErrorToast`, `WithWaitingStatus` (spinner + message while a worker runs). Destructive actions (e.g. `d` discard with rebase, `worktree remove`) go through confirmations; escape cancels.
- **Mouse support**: gocui has mouse capture; lazygit supports clicking panels and buttons, `g.Mouse` gates mouse-only UI (hyperlinks in the info bar).
- **Spinner/loading**: `pkg/gui/presentation/loader.go` — `Loader(now, config)` picks a frame from `SpinnerConfig{Frames, Rate}` by wall-clock, so spinners animate without a ticker per spinner. `pkg/gui/status` (`status_manager.go`) merges a "top status" (blocking message + spinner) with an "app status" line.
- **Theming**: `pkg/theme` + `user_config.gui.theme`; colors as named styles; `style` package; light/dark and 256-color support. Full keybinding cheatsheets are auto-generated and translated (10 languages).

### 1.3 Shelling out to git — process management, streaming, cancellation

**[FACT]**

- **Command layer**: `git_commands` builds `exec.Cmd`s via a `GitCommandBuilder` (`NewGitCmd("worktree").Arg("list", "--porcelain")…`) and runs them through `gitCmdObjRunner` (`pkg/commands/git_cmd_obj_runner.go`), which wraps the OS runner with **retry-on-lock-error**: `index.lock` / `cannot lock ref` → retry with exponential backoff (20 ms initial, doubled, up to 7 retries ≈ 1 s+). This matters because lazygit's own foreground `git status` refresh holds `index.lock` briefly — the retry exists to survive that race.
- **Streaming into views**: `pkg/tasks` (`ViewBufferManager`) is the crown jewel for tree-trunk:
  - At most **one streamed command at a time** (e.g. one `git show` writing into the main panel); starting a new one **cancels the previous** (`stopCurrentTask`).
  - Output is **not read all at once**: the view asks for more lines as the user scrolls (`ReadLines(originY + viewHeight)`); a resize that grows the main view triggers reading more lines.
  - A `THROTTLE_TIME = 30ms` and a stress heuristic (`COMMAND_START_THRESHOLD = 10ms`) decide whether to delay command starts when the machine is busy.
  - `Cmd` abstraction with `Terminate()` (SIGTERM on Unix via `oscommands.TerminateProcessGracefully`); Windows uses ConPTY via `CreateProcess` because `exec.Cmd` can't (golang/go#62708).
- **PTY support**: interactive commands (editor, `git commit` message editor, rebase) run in a **pseudo-terminal sized to the target view** (`pkg/gui/pty.go`, `creack/pty`); resize propagates to the PTY. The UI suspends to let a PTY app take the full screen, then resumes.
- **The daemon**: `pkg/app/daemon` is a short-lived helper process handed to git as `GIT_EDITOR` to prefill interactive-rebase TODO files — a clever pattern for "let git do the interactive part but script the input".
- **Worktrees** (`pkg/commands/git_commands/worktree.go`): `git worktree add [-b branch] [--detach] <path> <base>`, `git worktree remove [-f]`, and `worktree_loader.go` parses `git worktree list --porcelain` (multi-line records; parallel `git rev-parse` per worktree to fill `GitDir`, `IsMain`, `IsCurrent`). Guards: `CheckedOutByOtherWorktree(branch, worktrees)` prevents checking out a branch that another worktree already has.

### 1.4 Performance tricks

**[FACT]**

- **Refresh dedup** (`pkg/gui/controllers/helpers/refresh_helper.go`): a background poller stores a **refs+HEAD fingerprint**; a poll that sees an unchanged fingerprint skips the expensive re-read. `branchLoadSeq` (atomic) + `appliedBranchLoadSeq` ensure concurrent branch loads don't clobber each other out of order — a newer load's result drops if an older one already applied.
- **Lazy loading**: log/diff content is streamed on demand (see §1.3); commits are paginated-ish via the loader; file tree is collapsible so huge trees aren't all rendered.
- **Background refreshes**: `R` refresh and periodic background `git fetch` run on workers; UI stays responsive.
- **Single-flight**: one streamed command at a time prevents hammering git.
- **[MEASURED — cautionary]**: gitui's benchmark (Linux kernel repo, 900k commits) showed lazygit at **57 s load, 2.6 GB memory, occasional freezes**. Great UX patterns, but the *whole-graph* approach doesn't scale to massive repos. tree-trunk should avoid loading full history for every repo up front.

### 1.5 Multi-repo: what lazygit does and the gap

**[FACT]**

- lazygit is **single-repo at a time**. Multi-repo handling = the **recent repos list**: `ctrl+r` opens a menu of previously-opened repos; the current working dir is appended to a persisted `RecentRepos []string` in the app state file (`pkg/gui/recent_repos_panel.go`); entries whose `.git` no longer exists are pruned. `lazygit` also accepts `-p <path>` / `--use-config-dir` etc., but there is **no repo discovery across the machine**.
- Worktree support exists but is **scoped to the currently open repo** (Worktrees tab in the Files window; `w` on a commit = "New worktree"). There is **no cross-repo worktree management** — you cannot see one repo's worktrees while another repo is loaded, and there is no combined multi-repo worktree view.

**[OPINION]** That exact gap — *a repo list, per-repo worktree management, and log/diff/status inspection across many repos in one TUI* — is tree-trunk's raison d'être. lazygit proves the per-repo UX; nothing in lazygit competes with the cross-repo layer. (herdr's worktree grouping, §3, is the closest thing and is multiplexer-shaped, not inspection-shaped.)

### 1.6 What tree-trunk should borrow from lazygit

1. **The context/controller/helper split** (adapted to Go idioms; don't copy the God-struct history). At minimum: strict separation of `git` layer from UI layer with typed models.
2. **The hint-bar keybinding model**: single-key, context-sensitive, bottom bar rendering current context's keys, `?` cheatsheet, disabled-with-reason.
3. **The `tasks` streaming machinery** for `git log` / `git diff` into the main panel — single-flight, lazy line reads on scroll, 30 ms throttle, cancellation via SIGTERM, PTY-sized main view.
4. **Refresh dedup** (fingerprint skip + sequenced loads) for background repo-status refreshes.
5. **`git worktree list --porcelain` parsing + `CheckedOutByOtherWorktree` guard** — direct port.
6. **`gitCmdObjRunner` lock-retry** — tree-trunk will race its own refreshes against worktree/gc operations.
7. **`ConfirmIf` + `Toast`/`ErrorToast` + `WithWaitingStatus`** popup primitives; **two-step confirmation for `worktree remove`** (safe first, then forced — also see herdr §3.2).
8. **`z`/`Z` reflog-based undo** — ambitious; note as v2 (cross-repo undo is risky).

---

## 2. lazydocker (jesseduffield/lazydocker)

### 2.1 Architecture

**[FACT]**

- **Language**: Go 1.22. **TUI framework**: same gocui fork as lazygit.
- **Structure**: `pkg/app`, `pkg/gui` (panels, layout, confirmation, subprocess, filtering), `pkg/commands` (the docker layer), `pkg/tasks` (task adapters), `pkg/config`, `pkg/i18n`, `pkg/utils`, `pkg/log`.
- **Key architectural difference from lazygit**: the command layer models the *domain entities* — `Project`, `Service`, `Container`, `Image`, `Volume`, `Network` (`pkg/commands/`), each bundling the data **and** the operations (`Container.Remove/Start/Stop/Pause`, `Service.Up/Down`…). This is exactly the shape tree-trunk wants: **Repo / Worktree / Branch / Commit entities with methods**, not free-floating command functions.
- **Docker access is dual**:
  - **Go API client** (`docker/docker` + `docker/cli`) for inventory/state/start/stop/remove (typed, fast, no subprocess per call). `DockerCommand` wraps `client.Client` with API-version negotiation.
  - **Shelled `docker compose` commands** (via `OSCommand`) for compose-level actions and for **custom user commands**, which are Go-templated strings (`CommandObject` holds `DockerCompose`, `Service`, `Container`, …; `mergo` merges defaults).
- **GUI**: side panels (services, containers, images, volumes, networks — tabbed), a main panel (logs/stats/top), bottom status; same gocui event loop as lazygit. `pkg/gui/panels/` has reusable list/filter machinery (`list_panel.go`, `filtered_list.go`, `side_list_panel.go`) — a **generic filtered-list panel** component worth copying.

### 2.2 UX patterns to steal

**[FACT]**

- **Layout**: left side = list of entity categories (Project/Containers/Images/Volumes/Networks as tabs), each category has a list; right side = main panel showing logs (or stats graphs via `jesseduffield/asciigraph`); bottom bar for status + keybinding hints. `[` / `]` cycle tabs; `enter` moves focus to main panel.
- **Keybindings**: single-key per context, mnemonics chosen per verb (`s` stop, `r` restart, `d` remove, `m` view logs, `a` attach, `p` pause, `u` up, `E` exec shell, `b` bulk commands, `c` custom commands, `w` open in browser). `esc` returns from main panel. `/` filters the current list.
- **Custom commands**: user-defined templates bound to keys — a generic "run my command on the selected entity" mechanism. tree-trunk analog: custom git commands on a selected repo/worktree.
- **Confirmation flows**: `confirmation_panel.go` — destructive actions (`d` remove container/image/volume) prompt for confirmation; "bulk commands" mode (`b`) applies an action to a filtered selection with confirmation.
- **Status/spinner**: `app_status_manager.go` shows app-level status; `TickerTask`s drive the log panel.
- **Theming/config**: `Config.md` + `gui` config (wrap main panel, return-immediately on logs, etc.).
- **Error UX**: `ComplexError` with error **codes** (`MustStopContainer`) so the UI can offer the right next action ("stop the container before removal or force remove") — humanized, actionable errors.

### 2.3 Shelling out to docker — streaming, cancellation

**[FACT]**

- **Log streaming** (`pkg/gui/container_logs.go`): `NewTickerTask` runs a `Func` every **200 ms** with a `context.Context`; on each tick the main view is cleared and logs re-streamed into it (`writeContainerLogs`), with `Autoscroll: true` and optional line wrapping. Cancellation = context cancel (task stopped when switching panels). After a container exits, a 100 ms ticker polls `container.Inspect()` until it's back, so the log view "follows" restarts. Separate `renderLogsToStdout` path suspends the TUI and streams raw logs to stdout for piping.
- The Go API client is the default transport (typed, cancellable via `context.Background()`); subprocesses only where the CLI is unavoidable (compose, SSH contexts, custom commands).

### 2.4 Performance tricks

**[FACT]**

- **Ticker-based incremental refresh** rather than continuous streaming — cheap, bounded work per tick; stats history is recorded per container (`StatHistory`, `MonitoringStats`, `boz/go-throttle`).
- Lists are refreshed on focus/action rather than continuously; `go-throttle` throttles stat collection.
- The dual-transport design avoids spawning a process for every read-only query.

### 2.5 What tree-trunk should borrow from lazydocker

1. **Entity-model command layer**: `Repo`, `Worktree`, `Branch`, `Commit` structs bundling data + operations (lazydocker's `Project/Service/Container` pattern) — with a `LimitedRepo`-style interface for popup/custom-command contexts.
2. **Ticker-task log/diff streaming**: 100–200 ms tick re-stream into the main view with autoscroll + wrap config + follow-on-restart behavior. (For `git log --follow`-style views, lazygit's line-streaming is better; for live `git status`/tail-like views, lazydocker's ticker is simpler.)
3. **Tabbed side windows** (`[` / `]`) — tree-trunk's repo list + per-repo views map naturally to tabs in windows.
4. **Custom command templates** — user-defined git command recipes bound to keys, templated against the selected repo/worktree/branch.
5. **`ComplexError` codes** for actionable errors (e.g. `WorktreeDirty` → offer `--force` next step, mirroring git's own "cannot remove; use --force" message).
6. **Generic filtered-list panel** — one reusable list-with-filter widget for repos, worktrees, branches.

---

## 3. herdr (herdrdev/herdr)

> Context: tree-trunk's research process itself runs inside herdr ([MEASURED]
> `HERDR_ENV=1`, session `grove`, workspace `w1` with 5 pi agents in 2 tabs —
> this doc was researched in pane `w1:p7` while peers ran tracks 1–3 in
> `w1:p4..p6`). So we have first-hand, live experience of its model.

### 3.1 Architecture

**[FACT]**

- **Language**: Rust, Apache-2.0, 26.2k stars, v0.7.5 installed (0.8.0 current) via Homebrew.
- **Client/server split** (confirmed live + `src/` tree):
  - `src/server` (+ `src/server/headless`) — a persistent background server that **owns all panes, PTYs, and process state**; survives client detach.
  - `src/client` — the terminal UI attached to the server.
  - `src/api` (+ `schema`) — a **socket API**: newline-delimited JSON over a Unix domain socket (`~/.config/herdr/sessions/<name>/herdr.sock`), with a bundled JSON Schema (`herdr api schema --json`).
  - `src/pane`, `src/pty` (`pty/actor`, `pty/backend`), `src/workspace` (+ `workspace/git` for worktrees), `src/persist` (session persistence), `src/detect` (agent detection + manifests), `src/integration`, `src/ui` (+ `ui/sidebar`), `src/config`, `src/input`, `src/protocol`, `src/ghostty`, `src/remote`.
- **State model**: **session → workspace → tab → pane**, with opaque stable IDs (`w1`, `w1:t3`, `w1:p7`) that are never reused. A pane is a real terminal (PTY) that keeps running across detach. A **workspace is the top-level project container** ("use one workspace per repo, task, or investigation"); tabs are layouts inside a workspace ("agents", "logs", "server", "review").
- **Layout model**: tabs contain a **BSP tree of splits** (`layout.export` returns `root` with `pane`/`split` nodes, direction `right|down`, ratio; `layout.apply` recreates a tab from that declarative tree; `layout.set_split_ratio` mutates). This is how a multiplexer thinks about layout: **declarative, portable, restorable**.
- **Input model**: **mouse-native** (click panes/tabs/workspaces/agents, drag borders, right-click menus) *plus* a tmux-style **prefix chord** (default `ctrl+b`): terminal mode (keys → pane), prefix mode (next key → herdr), navigate mode (persistent workspace navigation surface with plain `j/k`). `prefix+?` shows active bindings with filtering. All keys configurable; direct chords validated against a matrix of terminal/OS defaults (they recommend `ctrl+alt` chords as the only mostly-free family).
- **Agent awareness**: herdr detects agents in panes (foreground process, screen manifests, integrations) and tracks a **lifecycle state machine**: `blocked` (needs input/approval), `working`, `done` (finished, unseen), `idle` (finished, seen), `unknown`. This state **rolls up into the sidebar** so you can see "which project needs attention" ([MEASURED] our 5 agents all showed `working` with per-pane `revision` counters).

### 3.2 UX patterns to steal

**[FACT]**

- **Sidebar = state dashboard**: `[ui.sidebar.spaces] rows` defines per-row **tokens** — `state_icon`, `workspace`, `branch`, `git_status` (ahead/behind counts when nonzero), `$metadata`. Agent rows: `state_icon`, `state_text`, `workspace`, `tab`, `pane`, `agent`, `terminal_title`. Tokens auto-hide when empty; separators collapse. This is *directly* the shape of tree-trunk's repo list: **icon + name + branch + ahead/behind + dirty indicator per repo**.
- **Worktrees as grouped workspaces** (from `docs/configuration` + live `herdr worktree` CLI):
  - `worktree.create [--workspace|--cwd] [--branch NAME] [--base REF] [--path PATH] [--label]` — creates the checkout (checking out an existing local branch if the name exists, else creating it from base/HEAD), **opens it as a new herdr workspace, grouped under the source workspace**.
  - `worktree.open` lists existing checkouts for the repo; choosing a closed one opens it in the same group.
  - `worktree.remove` runs `git worktree remove`; **"Herdr first asks Git to remove safely. If Git refuses because the checkout has modified or untracked files, Herdr asks again before running the forced remove."** Two-step escalation, never auto-forcing, and **never deletes branches**.
  - Config: `[worktrees] directory = "~/.herdr/worktrees"` → checkouts under `<dir>/<repo>/<branch-slug>`.
  - Emits lifecycle events (`worktree.created`, `worktree.removed`, `workspace.updated`…); a **plugin ecosystem hooks these events** (`[[events]] on = "worktree.created"` runs a manifest command — e.g. "bootstrap a worktree with env setup").
- **Session management** (from `docs/session-state`):
  - Detach/reattach: processes keep running (`ctrl+b q` detach, `herdr` reattach).
  - Server restart: **snapshot restore** of workspaces/tabs/panes/cwd/layout/focus (processes replaced by fresh shells).
  - **Pane screen-history replay** (opt-in, `[experimental] pane_history = true`): restores recent terminal contents after restart (off by default for secret-safety).
  - **Native agent session resume**: integrations report session refs so supported agents restart with their conversation (`pi --session <id>`, `claude --resume <id>`, `codex resume <id>`, …).
  - **Live handoff** (experimental, `herdr update --handoff`): transfers live panes to a new server.
- **Notifications**: `notification.show` with toast delivery (`position`, `sound`), sanitized text, rate-limiting, `reason` responses (`disabled|rate_limited|busy|no_foreground_client`).
- **API-driven state**: `session.snapshot` = one-time bootstrap for clients that cache state locally, then `events.subscribe` for incremental updates. **One authoritative model, snapshot + event deltas** — a clean pattern for a Go TUI too (model store + render-on-change).
- **Theming**: built-in named themes (catppuccin, tokyo-night, gruvbox…), `auto_switch` light/dark on terminal appearance change, per-color overrides (`theme.custom.accent = "#a6e3a1"`).
- **Custom command keybindings**: `[[keys.command]] key = "prefix+g" type = "popup|pane|shell|plugin_action" command = "lazygit"` — popups get dimensions as cells or percentages. (Nice precedent: herdr ships a `prefix+g` → run lazygit example.)

### 3.3 Process management / streaming / cancellation

**[FACT]**

- Server owns PTYs; panes are preserved across client detach; input is written through the socket API (`pane.send_text` / `send_keys` / `send_input` with key validation; bracketed-paste honored).
- Reads are snapshots with scroll metrics (`offset_from_bottom`, `max_offset_from_bottom`, `viewport_rows`); `pane.read --source recent-unwrapped` joins soft-wrapped lines (good for logs).
- Cancellation model is per-process: panes close → process signaled; `pane.wait_for_output` / `agent.wait` are **event-driven server-owned waits** (pinned to a resolved agent so a replacement can't satisfy the wait) — with a "stalled" guard that returns `agent_prompt_stalled` if no lifecycle change within 5 s rather than hanging.
- `pane.process_info` exposes shell pid, foreground process group, argv/cwd — a model tree-trunk could mirror to show *what's running* per worktree.

### 3.4 What tree-trunk should borrow from herdr

1. **Worktree operations as first-class API surface**: create (existing branch vs new-branch-from-base), open, list, remove — with **two-step forced-remove confirmation** and **never-delete-branches** semantics. Port the `--workspace|--cwd`, `--branch`, `--base`, `--path` parameter shape.
2. **Worktree grouping**: a worktree belongs to its parent repo (grouped children). tree-trunk: repo row → expandable worktree children, parent-close closes group but never deletes checkouts. (herdr config `[worktrees] directory` + `<repo>/<branch-slug>` layout is a sensible default for tree-trunk's `create` flow.)
3. **Sidebar token rows**: `state_icon + workspace + branch + git_status` is exactly a repo-list row; copy the token/rollup idea (missing values collapse, separators vanish).
4. **Snapshot + events state model**: one authoritative state store, render on change; background refreshers emit updates rather than each view polling. (In Go: a `state.Store` + channels — see track 3 for framework options.)
5. **Lifecycle-state thinking for background jobs**: tree-trunk's per-repo "is it refreshing?" state maps to herdr's agent states — show *stale/fresh/refreshing/error* per repo instead of a single global spinner.
6. **Mouse-native + keyboard-optional philosophy** — but for a git tool, keyboard-first with mouse as enhancement (inverse of herdr) is probably right; herdr's *help overlay with filterable bindings* (`prefix+?` + `/`) is worth copying verbatim.
7. **Notifications/toasts** for long-running cross-repo refreshes or failed worktree ops.

**[OPINION]** tree-trunk does **not** need a herdr-style daemon/client split in v1 — it's a single-user inspection tool, and a long-running server adds lifecycle complexity (the memory/state it would keep is cheap to recompute with lazygit-style refresh dedup). But keep the door open: the state-store design above would make a future `--server` mode or socket API natural.

---

## 4. Peers, quick look

### 4.1 gitui (gitui-org/gitui, Rust, 22.4k stars)

**[FACT]**

- **Stack**: `ratatui` 0.30 (crossterm backend), **git2/libgit2** via the in-house **`asyncgit` crate**: git2 calls that may be slow are put on a **thread pool**; `crossbeam-channel` notifies the UI when results are ready → "the main-thread and therefore the ui stay responsive". `notify-debouncer-mini` debounces filesystem watchers. `fuzzy-matcher` for fuzzy finding. `scopetime` for micro-benchmarks.
- **Performance ethos** (README benchmark on the Linux repo, 900k+ commits): gitui **24 s / 0.17 GB / no freezes**, lazygit 57 s / 2.6 GB / freezes, tig 4 m 20 s / 1.3 GB.
- **UX**: keyboard-only, context-based help, fuzzy-find branch/commit, per-view help, themes (THEMES.md), keybindings config (KEY_CONFIG.md) with vi-mode support. Log view is lazy/windowed (logwalker).
- **[OPINION]** tree-trunk should copy the **async-job + channel** pattern (in Go: goroutines + channels — idiomatic and easy) and the **debounced fs-watch** idea (watch a repo dir; debounce; trigger refresh). Avoid gitui's dependency on libgit2 if track 3 concludes CLI-wrapping is better for worktrees (libgit2 has historically weak worktree support; lazygit's CLI approach is battle-tested).

### 4.2 tig (jonas/tig, C/ncurses, 13.3k stars)

**[FACT]**

- C + ncurses, 0.6 MB binary, 20+ years old. **View-based modal model**: main/diff/log/blame/status/tree/stash/grep/refs/reflog views (`src/status.c`, `src/log.c`, …), keybindings in **tigrc**, everything shells out to `git` subprocesses, `src/watch.c` implements timed auto-refresh of views, `src/request.c` is a big request enum → dispatch table (the C ancestor of lazygit's binding tables).
- **[OPINION]** tig proves the **modal single-view** alternative to lazygit's panel-grid: tree-trunk's `log / diff / status` inspection could be a modal view over a selected repo (tig-style) *or* lazygit panels. Panels win for simultaneous context; modal wins for simplicity/wide diffs. Consider a fullscreen "inspect" mode toggle (`+` in lazygit already does fullscreen).

### 4.3 Comparison table — architecture at a glance

| | lazygit | lazydocker | herdr | gitui | tig |
|---|---|---|---|---|---|
| Language | Go | Go | Rust | Rust | C |
| TUI lib | gocui (forked, tcell v3) | gocui (forked) | custom (Rust) | ratatui + crossterm | ncurses |
| Git/Docker layer | git CLI subprocesses, typed cmd objects | docker Go API + compose CLI | git CLI (worktrees) + integration manifests | git2/libgit2 async | git CLI subprocesses |
| Event model | gocui MainLoop + workers | gocui MainLoop + ticker tasks | server socket API + client events | ext async jobs + channels | single-threaded request loop |
| State | contexts stack + models | entity structs | session → workspace → tab → pane + agents | app state + cached git data | view state + watch |
| Multi-repo | recent-repos list only | n/a (single docker host) | many workspaces (+worktrees) | single repo | single repo |
| Undo | reflog-based z/Z | n/a | n/a | n/a | n/a |

---

## 5. Keybinding & layout patterns compared

### 5.1 Keybinding models

| Pattern | lazygit | lazydocker | herdr | Steal for tree-trunk? |
|---|---|---|---|---|
| Single-key context-sensitive | ✅ per-context bindings + hint bar | ✅ per-panel | — (prefix chords) | ✅ **yes** — core model |
| On-screen hint bar of active keys | ✅ bottom `options` view | ✅ bottom bar | `prefix+?` overlay | ✅ yes — lazygit style |
| Help overlay / cheatsheet | ✅ `?`, auto-generated docs | ✅ cheatsheet | ✅ `prefix+?`, filterable | ✅ yes, filterable like herdr |
| Modal/navigation modes | transient (rebase, range) | bulk mode | prefix / terminal / navigate modes | ✅ limited modes only |
| Vim-style movement | `j/k` + arrows both work | arrows | `prefix+h/j/k/l` + navigate mode | ✅ `j/k` + arrows, lazygit style |
| Search/filter key | `/` per view (search xor filter) | `/` filter lists | `/` filter help, search in copy mode | ✅ per-view search+filter |
| Destructive confirm | Confirm/ConfirmIf | confirmation panel | two-step force for worktree remove | ✅ two-step for worktree delete |
| Undo/redo | reflog `z`/`Z` | — | — | ⏳ v2 |
| Custom commands | full config DSL | ✅ templates | `[[keys.command]]` | ✅ config DSL is a differentiator |

### 5.2 Layout patterns

| Pattern | lazygit | lazydocker | herdr | tree-trunk candidate |
|---|---|---|---|---|
| Side list + main content | ✅ | ✅ | sidebar + tiled panes | ✅ repos list (left) + main (right) |
| Tabs inside windows | ✅ `[`/`]` | ✅ | tabs are real containers | ✅ per-repo tab: Worktrees / Log / Diff / Status |
| Bottom status/hint bar | ✅ | ✅ | status bar + prefix bar | ✅ |
| Sidebar with state rollup | — | — | ✅ tokens: icon/branch/git_status | ✅ **the repo-list row design** |
| Fullscreen toggle | ✅ `+`/`_` | — | zoom pane `prefix+z` | ✅ `+` fullscreen main view |
| Breadcrumbs | — | — | workspace/tab labels | ⏳ context stack label in status bar |
| Expandable tree rows | ✅ file tree, worktrees | — | workspace groups w/ children | ✅ repo → worktree children |

---

## 6. "Steal this" — concrete recommendations for tree-trunk

Prioritized. **P0 = ship with it; P1 = soon after; P2 = later / optional.**

### P0 — architecture

1. **Two-layer split, lazygit-style**: `internal/git` (typed command objects + models: `Repo`, `Worktree`, `Branch`, `Commit`, `Status`) strictly separate from `internal/ui`. All git access goes through one runner that adds **index.lock retry with backoff** (lazygit's `gitCmdObjRunner`, 20 ms → ~1 s, 7 retries).
2. **Entity-method model, lazydocker-style**: `Worktree.Add/Remove/List` on the entity, `Repo.Status/Log/Diff` — so UI, custom commands, and future API share one layer.
3. **State store with events** (herdr's snapshot+events, in Go): one authoritative `state.Store` mutated by workers; views subscribe and re-render. This is the foundation for everything below (background refresh, spinners, toasts).
4. **Event loop**: follow lazygit — UI thread owns rendering; workers bounce via a single UI channel; track "busy" with a task counter (their `Busy.md` pattern) so tests/UI can show a global working state.

### P0 — UX

5. **Repo list = herdr sidebar tokens**: each row shows `state icon (clean/dirty/ahead/behind/error) + repo name + branch + ahead/behind + worktree count`. Collapse missing values. This is the single most distinctive steal.
6. **Keybinding model**: single-key context-sensitive with bottom hint bar (lazygit `options` view), `?` filterable cheatsheet, `esc`-cancels-transient-states, `/` per-view search-or-filter, `R` manual refresh, `q` quit, `ctrl+z` suspend.
7. **Worktree flows, herdr semantics**: create with `--branch --base --path` (checkout existing branch if it exists, else create from base/HEAD); list via `git worktree list --porcelain` (lazygit loader, incl. `IsMain/IsCurrent/IsPathMissing`); remove = **two-step**: safe `git worktree remove`, on refusal offer explicit `--force` confirmation; never auto-delete branches; guard `CheckedOutByOtherWorktree`.
8. **Destructive confirmations + toasts**: `ConfirmIf` before any irreversible op; `Toast`/`ErrorToast` when an action's effect is invisible (lazygit popup handler trio + `WithWaitingStatus` spinner).
9. **Spinner**: time-based frame picker (lazygit `Loader`), configurable frames/rate, per-repo refresh state instead of one global spinner (herdr lifecycle thinking).

### P0 — streaming & performance

10. **Lazy log/diff streaming**: lazygit `tasks` pattern — one streamed command at a time, read lines on demand as the user scrolls, cancel previous on switch (SIGTERM), 30 ms throttle; main view sized like a PTY.
11. **Refresh dedup**: fingerprint refs+HEAD per repo; skip refresh when unchanged; sequenced loads to prevent out-of-order clobbering (lazygit `refresh_helper`).
12. **Background poller with debounced fs-watch** (gitui's `notify-debouncer-mini` idea): watch repo dirs, debounce, trigger status refresh; never block UI on `git status` — run on worker, post result to store.
13. **Don't load full histories** for all repos up front (lazygit: 2.6 GB on Linux repo). Lazy-load per-selection; cap log size; consider paging.

### P1

14. **Custom command DSL** (lazydocker templates / lazygit custom commands): user-defined git recipes keyed to selected repo/worktree/branch. Strong differentiator; moderate cost.
15. **`--json` / machine-readable output** for scripting (`tree-trunk --json list`), mirroring herdr's CLI/API split — cheap if the state store exists.
16. **Actionable error codes** (`ComplexError` pattern): e.g. `WorktreeDirty` → offer force; `BranchCheckedOutElsewhere` → jump to that worktree.
17. **Fullscreen inspect mode** (`+` toggle) for log/diff/status of the selected repo.

### P2

18. **Reflog-based undo** (lazygit `z`/`Z`) — scoped per repo, risky cross-repo; validate carefully.
19. **Daemon/socket mode** (herdr-style) if long-lived refresh or external tooling emerges as a need; design the store so this stays possible.
20. **Worktree bootstrap events/plugins** (herdr `worktree.created` hooks) — e.g. auto-open a TUI, run install commands, or feed other tools.

---

## 7. Risks & open questions

- **TUI framework choice is still open** (cross-track with 03-go-packages): lazygit's gocui is forked/maintained in-house — tree-trunk should use a *maintained* framework. The patterns above are framework-agnostic (event loop, contexts, streaming), but the hint-bar, popups, and layout helpers map most directly to **tview** (panel-ish) or **bubbletea** (message-driven — pairs naturally with the state-store/event design). Decide in the design doc; the inspiration here shouldn't force a framework.
- **Multi-repo cost**: N repos × background `git status`/`branch` refreshes can be heavy; the fingerprint-dedup + debounced watch + lazy loading (P0 items 10–13) are the mitigation, but measure on a large home dir (100+ repos). Consider capping concurrent refresh workers and prioritizing the focused repo.
- **Worktree safety**: `git worktree remove --force` destroys untracked/modified files — the two-step confirm is mandatory; also handle `.git` file worktree paths (linked worktrees use a gitdir pointer file, not a dir — `worktree_loader` already handles `pathExists`).
- **lazygit has no cross-repo story; herdr has no inspection story** — tree-trunk's niche is real, but it means the *repo-list → repo-detail* navigation (focus switching, context stack, back navigation) is unproven territory: prototype the navigation first.
- **herdr docs describe v0.7–0.8 behavior and evolve fast** (API surface is churning, e.g. `session.snapshot`); treat herdr as inspiration for *shapes*, not as a stable spec to match.
- **Undo across worktrees** (P2) is dangerous: reflog undo operates per-worktree; document limitations or drop.

---

## 8. Sources (primary)

- lazygit repo (cloned, master @ 2026-08-09): `docs/dev/Codebase_Guide.md`, `docs/dev/Busy.md`, `docs/Searching.md`, `docs/keybindings/Keybindings_en.md`, `VISION.md`, `pkg/tasks/tasks.go`, `pkg/gui/layout.go`, `pkg/gui/options_map.go`, `pkg/gui/status/status_manager.go`, `pkg/gui/popup/popup_handler.go`, `pkg/gui/recent_repos_panel.go`, `pkg/gui/controllers/helpers/refresh_helper.go`, `pkg/commands/git_cmd_obj_runner.go`, `pkg/commands/git_commands/worktree.go` + `worktree_loader.go`, `go.mod`.
- lazydocker repo (cloned): `pkg/commands/docker.go`, `container.go`, `pkg/gui/container_logs.go`, `pkg/gui/panels/`, `docs/keybindings/Keybindings_en.md`, `go.mod`.
- herdr: docs fetched from herdr.dev — `/docs/concepts`, `/docs/keyboard`, `/docs/socket-api`, `/docs/session-state`, `/docs/configuration`; live CLI inspection (`herdr --help`, `herdr workspace`, `herdr worktree`, `herdr tab`, `herdr pane list`, `herdr agent list`, `herdr workspace get`); GitHub API tree of herdrdev/herdr; local agent skill `~/.agents/skills/herdr/SKILL.md`.
- gitui repo (cloned): `Cargo.toml`, `README.md` (benchmarks), `asyncgit/README.md`.
- tig repo (GitHub): `README.adoc`, `src/` listing.
- Star counts/languages/push dates: GitHub REST API, 2026-08-09.
