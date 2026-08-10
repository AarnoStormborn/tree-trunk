# TUI Layout & Keybindings — tree-trunk

> Design doc. bubbletea v1.3.x (D2). Layout and interaction patterns adapted
> from lazygit / lazydocker / herdr (research track 4).

## 1. Layout

**Implemented 2026-08-10 (ui-split): the app is a framed sidebar/main split.**
Outer rounded frame; header bar (title + version); horizontal rules between
sections; sidebar (repo list) with its own header and a right border that
spans the full body height as the vertical boundary; main section padded
away from the boundary, with a repo header + highlighted tab bar (active tab
has an accent background) + unboxed content; footer with a SINGLE
context-aware help legend (list vs active tab) + status, separated by rules.
Fullscreen (`+/_`) hides the sidebar. The bubbles list's built-in help footer
is disabled so exactly one legend renders.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ tree-trunk                                                      v0.1.0  │  ← header
├───────────────────────────────┬──────────────────────────────────────────┤
│ repos (21)                    │ repo·main                                │  ← pane headers
│ ✓ myproject   main ↑2↓1       │ status │ worktrees │ log │ diff         │  ← tab bar
│   ├─ worktree feat/x  ~       │ ┌──────────────────────────────────────┐ │
│ ~ otherrepo    dev ~3 +1      │ │ content (boxed)                      │ │
│ ↻ bigrepo                     │ └──────────────────────────────────────┘ │
│ e brokenrepo                  │                                          │
│                               │                                          │
│ j/k move · / filter · l expand│                                          │
├───────────────────────────────┴──────────────────────────────────────────┤
│ [s]tatus [l]og [d]iff [w]orktrees  [n]ew worktree [D]elete  / filter   │ ← help
├──────────────────────────────────────────────────────────────────────────┤
│ refreshing 3 repos…                                        [↑2↓1]      │ ← status
└──────────────────────────────────────────────────────────────────────────┘
```
│ ┌───────────────────────────┐ │ ┌──────────────────────────────────────┐ │
│ │ ✓ myproject   main ↑2↓1   │ │ │  On branch main                       │ │
│ │   ├─ worktree feat/x      │ │ │  Your branch is up to date…           │ │
│ │   ├─ worktree feat/y  ✱L  │ │ │                                      │ │
│ │ ~ otherrepo    dev ~3 +1  │ │ │  Changes not staged:                  │ │
│ │ ↻ bigrepo                 │ │ │   M src/main.go                       │ │
│ │ e brokenrepo              │ │ │  Untracked:                           │ │
│ │                           │ │ │   ?? newfile.go                       │ │
│ │                           │ │ │                                      │ │
│ │                           │ │ │                                      │ │
│ │                           │ │ │                                      │ │
│ └───────────────────────────┘ │ └──────────────────────────────────────┘ │
├───────────────────────────────┴──────────────────────────────────────────┤
│ [s]tatus [l]og [d]iff [w]orktrees  [n]ew worktree [D]elete  / filter   │ ← hint bar (context keys)
│ refreshing 3 repos…                                        [↑2↓1]      │ ← status bar (spinner + info)
└──────────────────────────────────────────────────────────────────────────┘
```

- **Left pane (40%, resizable):** repo list — herdr-style token rows
  (02-data-model §3), worktrees as expandable children. `bubbles/list`.
- **Right pane (60%):** tabbed inspection views for the focused repo:
  `Status | Log | Diff | Worktrees`. Tab bar inside the pane (lazygit
  `[`/`]`-style toggling, but we use visible tabs + keys).
- **Bottom:** hint bar (current context's keybindings — lazygit `options`
  view) and status bar (spinner, busy text, branch info, mode).
- **Modals:** confirm dialogs, toasts, help cheatsheet, worktree-create form
  — centered overlays over both panes.

**Fullscreen:** `+`/`_` toggles the main pane to full width (lazygit screen
modes); the repo list hides.

## 2. Focus & navigation model

- **Context stack** (lazygit `pkg/gui/context.go`): the app has contexts
  `repoList`, `status`, `log`, `diff`, `worktrees`, `form`, `help`,
  `search`. `esc` pops transient states (search → form → tab → repoList);
  `enter`/`tab` move focus between panes. Repo selection (left) drives which
  repo the right pane shows.
- **Navigation (unproven territory — prototype first):** repo→repo is `j/k`
  + `enter`; jump to a repo with `/` filter then `enter`. Back from a tab's
  sub-selection (e.g. a commit in log) via `esc`. This repo-list→repo-detail
  hop is the part lazygit doesn't have; research flags it as the riskiest UX
  (04 §7) — spike it early.
- **Mouse:** keyboard-first (D6 platforms are terminal-native); mouse
  optional enhancement (click repo row, click tab, scroll viewport) via
  `tea.WithMouseCellMotion` — verify it doesn't fight keybindings.

## 3. Keybinding registry (`internal/ui/keys.go`)

Single source of truth: one `[]KeyBinding{Key, Contexts, Action, Help,
DisabledWhen}` table → drives both the hint bar and the `?` cheatsheet
(bubbles/key + bubbles/help). Disabled bindings render grayed with a reason
(lazygit VISION: "if a keybinding is disabled, give a reason why").

> Registry reallocated by the design review (M4) — `08-reviewer-drafts.md`
> Draft B. Rules: (1) one verb, one key, one context — a key appears at most
> once per context, and a verb uses the same key wherever it appears;
> (2) global keys are never reused by persistent contexts; (3) Forms / help /
> search are **modal override contexts** that suspend the global set while
> open (standard modal-TUI behavior; do not "fix"); (4) `+`/`_` are global
> fullscreen only; (5) every binding carries a DisabledWhen reason.

### 3.1 Global (all contexts)

| Key | Action | DisabledWhen |
|---|---|---|
| `?` | Help cheatsheet (filterable) | — |
| `q` / `ctrl+c` | Quit (confirm if busy) | — |
| `ctrl+z` | Suspend TUI to shell (`tea.Suspend`) | — |
| `R` | Refresh: re-scan + re-status (fingerprint-deduped) | — |
| `/` | Search/filter current list (RepoList: filter; Log: search) | context has no search/filter (Status, Diff, Worktrees: v1 no-op) |
| `tab` / `shift+tab` | Next / previous pane | — |
| `+` / `_` | Main pane fullscreen / restore | — |
| `g` / `G` | Go to top / bottom | — |
| `ctrl+r` | Recent repos (persisted) | — |
| `esc` | Pop transient state (search → form → tab → repoList) | — |
| `j`/`k`, `↑`/`↓` | Movement (bound per context; `h`/`l` + `←`/`→` likewise) | per context |

### 3.2 Repo list context

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k`, `↑`/`↓` | Move selection | — |
| `enter` | Focus right pane (Status of selected repo) | — |
| `→` / `l` | Expand repo row (worktree children) | row already expanded |
| `←` / `h` | Collapse / go up | no expanded repo row |
| `n` | New worktree in selected repo | — |
| `d` | **Delete worktree** (two-step, safe → force confirm) | selection is a repo row (worktree child rows only) |
| `o` | Open: print path + copy to clipboard (repo row → main worktree path; child row → worktree path) | — |
| `L` | Lock / unlock worktree (toggle) | selection is a repo row — main worktree cannot be locked (git refuses) |
| `P` | Prune (dry-run preview → confirm) | — (runs for selected repo; locked worktrees skipped by git) |
| `space` | Toggle repo pin (keep in list when filtering) | — |
| `e` | Show error detail | selection not in `error` lifecycle state |
| `f` | Toggle filter: dirty/conflicted only | — |

### 3.3 Inspection tab contexts

Common to Status / Log / Diff / Worktrees:

| Key | Action |
|---|---|
| `1`/`2`/`3`/`4` | Switch tab (Status/Log/Diff/Worktrees) |
| `[` / `]` | Cycle tab (prev / next) |

#### Status

| Key | Action | DisabledWhen |
|---|---|---|
| `enter` | Open file → diff scoped to that file | no file selected (clean status) |
| `x` / `X` | Expand / collapse file detail | no file selected / nothing to expand |

#### Log

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | Move commit selection | — |
| `enter` | Show commit diff (`commit^..commit`) | no commit selected |
| `w` | Create worktree from commit | no commit selected |
| `c` | Copy commit hash | no commit selected |
| `n` / `N` | Next / previous search match | search inactive (opened with `/`) |
| `pgup` / `pgdn` | Page up / down | — |
| `ctrl+u` / `ctrl+d` | Half-page up / down | — |
| (auto) | "Load more" at scroll end (page size 200) | — |

#### Diff

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | Scroll | — |
| `p` | Toggle stat summary / full diff | — |
| `c` | Copy path under cursor (stat mode) | not stat mode / no path |
| `pgup` / `pgdn` | Page up / down | — |
| `ctrl+u` / `ctrl+d` | Half-page up / down | — |
| (auto) | Truncation footer ("truncated — N lines") | — |

#### Worktrees

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | Move row | — |
| `n` | New worktree in this repo | — |
| `d` | Delete (two-step: safe → force confirm) | row is the main worktree (git refuses: "is a main working tree") |
| `o` | Open: print path + copy to clipboard | — |
| `L` | Lock / unlock (toggle; shows reason) | row is the main worktree (git refuses: "The main working tree cannot be locked or unlocked") |
| `P` | Prune (dry-run preview → confirm) | — (locked worktrees skipped) |

### 3.4 Forms & modals (modal override — see rule 3)

| Key | Action |
|---|---|
| `enter` | Confirm (create-worktree form, delete-confirm, prune-confirm) — DisabledWhen: form invalid (e.g., empty branch, path exists) |
| `esc` | Cancel |
| `tab` | Next field (create form: branch → base → path) |
| `ctrl+space` | Toggle suggestion (existing-branch completions) |

Help cheatsheet (`?`): `?`/`esc` close, `/` filters bindings. Search input
(Log, opened with `/`): typing edits the query, `enter` jumps, `esc` exits,
`n`/`N` iterate.

### 3.5 Key-allocation notes

- **Freed keys:** `D` (delete → `d`), `u` (unlock → `L`), `-` and tab-level
  `+` (paging → `pgup`/`pgdn`, `ctrl+u`/`ctrl+d`; expand → `x`/`X`). `D` and
  `-` are reserved for future use (e.g., batch ops F1, hunk folding P1).
- **New bindings:** `x`/`X` (Status expand/collapse), `pgup`/`pgdn` +
  `ctrl+u`/`ctrl+d` (Log/Diff paging), `P` (prune), `L` (lock/unlock —
  unified from `l`/`L` + `u`).
- **Deliberate same-key/different-object pairs (allowed):** `c` = copy in
  Log (hash) and Diff (path) — same verb, different contexts, never together.
- **Verified conflict-free:** each key appears at most once per context;
  no persistent context reuses the reserved global set; `+`/`-`/`_` appear in
  no non-global table; `l` appears once in RepoList (expand — the lock verb
  moved to `L`).

## 4. Worktree create form

```
 ┌─ New worktree: myproject ────────────────────────────┐
 │ Repo        myproject                                 │
 │ Branch      [feat/my-thing        ]  (tab: pick from │
 │ Base        [main                 ]   existing 7)    │
 │ Path        ~/.worktrees/myproject/feat-my-thing     │  ← auto-suggested
 │   [ ] create branch -b (auto when branch is new)     │
 │ [ Create ]                        [ Cancel ]         │
 └──────────────────────────────────────────────────────┘
```

- Branch field: textinput with existing-branch completions (tab cycles);
  auto-detects "new branch" vs "existing branch" (git layer decides, 03 §3.2).
- Path: auto-suggested from D8 template; editable; validated non-empty and
  not-existing (`PathExists` error surfaces here).
- Confirm shows the exact `git worktree add …` command to be run (transparency
  — lazygit-style).

## 5. Inspection views

### 5.1 Status view
- Mirrors `git status` grouping: `On branch …`, staged / unstaged / untracked
  sections, conflict section first when present. Rendered from the typed
  `RepoStatus` model (02-data-model §1.5), not raw output.
- File rows colored by code (`M`/`A`/`D`/`R`/`??`), conflicts in red.
- `enter` on a file → diff view scoped to that file (working-tree diff).

### 5.2 Log view
- `bubbles/table`-style rows: hash, date, author, subject; page size 200,
  "load more" at scroll-end (03 §3.1 paged log).
- `enter` on a commit → diff view `commit^..commit`. `w` creates a worktree
  from that commit (lazygit pattern).

### 5.3 Diff view
- Streaming viewport (03 §7): working-tree vs HEAD by default; toggles:
  staged (`--cached`), against main branch (`git diff <main>...<branch>` —
  product requirement #4), commit diff.
- `p` toggles stat summary vs full diff; hunk folding is P1 (v1: stat/raw
  toggle only; review feasibility #2).

### 5.4 Worktrees view
- Table: path, branch, head, **dirty** (review M5), locked (🔒 + reason),
  prunable (⚠), current.
- `n`/`d`/`o`/`L`/`P` per the registry (04 §3.3); delete is always two-step
  (03 §3.2); `L` disabled on the main worktree.

## 6. Status & hint bars

- **Status bar (bottom-right):** busy state ("refreshing 3 repos…" + spinner),
  selected repo branch `↑2↓1`, mode label during search/filter, version.
- **Hint bar (bottom-left):** current context's bindings, rendered from the
  same registry (§3); highlight keys for likely-intended actions (lazygit:
  highlight during cherry-pick paste; ours: highlight `d` when a dirty
  worktree row is selected).
- **Spinner:** time-based frame picker (lazygit `Loader` — no ticker per
  spinner): frame chosen from wall clock; per-repo lifecycle icons in the
  list, single status-line spinner for aggregates (04 §1.2, §3.2.5).

## 7. Toasts, confirmations, errors

Port of lazygit's popup trio (04 §1.2):
- `Confirm(opts)` — title/message/confirm-label/cancel; destructive styling
  for `worktree remove --force` and any irreversible op.
- `ConfirmIf(cond, opts)` — conditionally require confirmation.
- `Toast(text)` / `ErrorToast(text)` — 3 s auto-dismiss for actions whose
  effect isn't visible ("Worktree created at ~/.worktrees/…").
- `Alert(text)` — modal for errors needing attention.
- `WithWaitingStatus(msg, fn)` — spinner + message while a worker runs.

Error mapping: `git.GitError` codes → dialog/toast text per 03 §6.

## 8. Theming (`internal/ui/theme.go`)

- lipgloss styles; `termenv` profile detection (truecolor → 256 → 16).
- Config: `theme` name + overrides; light/dark variants (`auto` switch on
  terminal appearance — herdr-style; bubbletea supports it via terminal
  background detection, P1).
- Named palette (default): normal text, dim (secondary), accent (selection),
  dirty (yellow), conflict (red), clean (green), worktree-child (indent).
- Respect `NO_COLOR`.

## 9. Accessibility & edge cases

- 256-color fallback; no mouse-required features; all actions reachable by
  keyboard.
- Small terminals: panes collapse (repo list min-width 20; below that, tabbed
  fullscreen) — bubbletea `WindowSizeMsg` re-layout.
- Long repo names: truncate with ellipsis; full path in header or `e` detail.
- Non-UTF8 paths: git outputs raw bytes; parse and render with
  `strings.ToValidUTF8` + replacement runes (never crash on locale edge
  cases).
