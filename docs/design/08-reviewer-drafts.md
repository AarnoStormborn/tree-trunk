# Reviewer Drafts — 08 (M7 config schema + M4 keybinding registry)

> Two drafts for verbatim (or near-verbatim) merge into the design docs.
> **Draft A** fixes M7 (complete TOML config schema) and lands in
> `01-architecture.md` §2 (config.go) or as a new doc `docs/design/config.md`.
> **Draft B** fixes M4 (keybinding conflicts) and replaces `04-tui-layout.md` §3
> (keybinding registry). Both drafts also carry the keys/bindings needed by M6,
> M8, M10, m8, m9. A resolved-findings checklist is appended (§3).
>
> Decisions made on the owner's behalf in these drafts (state them when merging):
> - `--scan-root` **REPLACES** default roots and config `discover.scan_roots`
>   (flag wins); config `discover.scan_roots` **ADDS** to the default `$HOME`
>   scan; new `discover.scan_home` bool lets config-only users opt out of
>   `$HOME` (m9).
> - Lock/unlock = **`L` everywhere** (single key, single verb); `u` is freed.
> - Delete worktree = **`d` everywhere**; `D` is freed.
> - Tab-level expand/paging moved off `+`/`-` (now global-fullscreen-only):
>   `x`/`X` = expand/collapse (Status), `pgup`/`pgdn` + `ctrl+u`/`ctrl+d` =
>   page / half-page (Log, Diff).
> - Prune = **`P`** (RepoList worktree rows + Worktrees tab), dry-run preview +
>   confirm (M6).
> - Forms/Help/Search are **modal override contexts**: while open they suspend
>   the global set (enter/esc/tab are form keys there). This is a deliberate,
>   documented exception to "no global reuse" — standard modal-TUI behavior.
> - New config keys (marked **[new]**): `discover.scan_home`,
>   `discover.follow_symlinks`, `discover.hidden_dirs`, `discover.hidden_peek_depth`,
>   `refresh.poll_interval_ms`, `[clipboard] enabled`.

---

# DRAFT A — Complete TOML config schema (M7)

## A.1 Reference table

Config file: `~/.config/tree-trunk/config.toml` (D7); override with
`--config PATH`. A missing file is not an error (defaults apply). All keys are
optional. `[new]` = introduced by this draft (fixes M7/M8/M10/m8/m9).

| Key | Type | Default | Example | Source |
|---|---|---|---|---|
| `workers` | int | `8` | `8` | 01-architecture §3.1 — bounded pool 4–16; clamped into range |
| `refresh.poll_interval_ms` | int | `0` (off) | `5000` | **[new]** (M10) — background poll cadence; status always re-read, ref reads fingerprint-gated; 0 disables; nonzero clamps to ≥ 1000 |
| `discover.max_depth` | int | `8` | `8` | 02-data-model §2.2 — depth cap from scan root; `0` = unlimited (use with care) |
| `discover.scan_roots` | []string | `[]` | `["~/src", "~/code"]` | 02-data-model §2.2, D4 — **ADDS** to default `$HOME` scan (see A.2.2) |
| `discover.scan_home` | bool | `true` | `true` | **[new]** (m9) — `false` drops the default `$HOME` root (config-only opt-out) |
| `discover.ignore` | []string | see A.3 | `["node_modules", "vendor"]` | 02-data-model §2.2, 08-review §M8 — path segments or trailing-path suffixes; `.git` is structural and always skipped |
| `discover.include_bare` | bool | `false` | `false` | 02-data-model §2.1 — opt-in bare-repo detection (v1 default off, F8) |
| `discover.follow_symlinks` | bool | `false` | `false` | **[new]** (M8) — follow dir symlinks with loop protection (visited-set of `EvalSymlinks`-resolved paths); depth cap still applies |
| `discover.hidden_dirs` | `"skip"` \| `"peek"` \| `"scan"` | `"peek"` | `"peek"` | **[new]** (M8) — see A.2.3 |
| `discover.hidden_peek_depth` | int | `2` | `2` | **[new]** (M8) — levels of hidden subtree searched when `hidden_dirs = "peek"` |
| `worktrees.directory` | string | `"~/.worktrees"` | `"~/wt"` | D8 — global worktree destination root |
| `[[worktrees.repos]] .path` | string | — | `"~/src/myproject"` | D8 — per-repo override selector (see A.2.4) |
| `[[worktrees.repos]] .directory` | string | — | `"~/wt/myproject"` | D8 — per-repo destination root |
| `theme.name` | string | `"default"` | `"catppuccin"` | 04-tui-layout §8 — built-in theme name; unknown name → warning + fallback to `"default"` |
| `theme.variant` | `"auto"` \| `"light"` \| `"dark"` | `"auto"` | `"dark"` | 04-tui-layout §8 — `auto` = terminal-background detection (P1; until implemented, `auto` behaves as `dark`) |
| `theme.overrides.<key>` | string (hex `#rrggbb`) | — | `"#a6e3a1"` | 04-tui-layout §8 — palette keys: `normal`, `dim`, `accent`, `dirty`, `conflict`, `clean`, `worktree_child` |
| `clipboard.enabled` | bool | `true` | `true` | **[new]** (m8/D11) — `false` = `o` prints path only (headless/WSL/SSH); on copy failure: toast + print-only fallback |

## A.2 Prose rules

### A.2.1 `~` expansion (all path values)

- A leading `~` or `~/…` in **any** path-valued key (`worktrees.directory`,
  `[[worktrees.repos]] .path` / `.directory`, `discover.scan_roots` entries)
  expands via `os.UserHomeDir()`; failure to determine HOME is a startup error.
- `~user/…` forms are **not** supported.
- Non-`~` relative paths resolve against the **current working directory**
  (not the config file's directory) — document this in the config reference;
  recommend absolute or `~`-prefixed paths in config.
- `--scan-root` and `--repo` flag values get the same `~` expansion and cwd
  resolution.

### A.2.2 Scan-root precedence (m9 decision)

Effective scan roots are computed as:

1. `--repo PATH` flags: always scanned, regardless of everything below
   (explicit repos are never lost).
2. `--scan-root DIR` flags, if any: **REPLACE** both the default `$HOME` root
   and config `discover.scan_roots` (the flag is the most explicit statement
   of intent).
3. Else config `discover.scan_roots`, if non-empty: **ADDS** to the default
   `$HOME` root (i.e., effective = `$HOME` ∪ config roots).
4. Else the default root: `$HOME` — unless `discover.scan_home = false`
   (then config roots, if any, are the only roots; with `scan_home = false`
   and no config roots and no flags, the app has nothing to scan — warn at
   startup and show an empty list).
5. `--no-scan`: suppresses *all* scanning (steps 2–4); only `--repo` inputs
   are loaded.

All roots are deduplicated by canonical path (`Abs` + `EvalSymlinks`, the same
canonicalization used for repo identity — review M1).

### A.2.3 Hidden-dir and symlink policy (M8)

- `discover.hidden_dirs = "skip"` — never descend into dot-directories;
  repos in hidden dirs are invisible (fastest; only for users who don't keep
  repos in dot dirs).
- `"peek"` (default) — descend into hidden subtrees up to
  `hidden_peek_depth` (default 2) *levels below the hidden dir itself*; a repo
  found at any level stops descent (existing rule). This finds the common
  dotfiles repos (`~/.emacs.d`, `~/.dotfiles`, `~/.vim`, `~/.oh-my-zsh`,
  `~/.config/nvim`) while bounding cost in `.cache`/`.local`-style dirs.
  Cost note: at depth 2, `WalkDir` lists entries of dirs like
  `~/.config/Code/User` — bounded, but set `"skip"` or add to `ignore` if a
  machine is pathological.
- `"scan"` — treat hidden dirs like any other (subject only to `ignore` and
  `max_depth`).
- `discover.follow_symlinks = false` (default): symlinked directories are not
  followed (a symlinked dev root like `~/dev -> /Volumes/…` is invisible —
  use `--scan-root` or `discover.scan_roots` on the *resolved* path, or opt
  in). When `true`: follow directory symlinks with a visited-set of
  `EvalSymlinks`-resolved paths to prevent loops; `max_depth` and `ignore`
  still apply to the followed tree.
- The heavy-dir default `ignore` list (A.3) applies regardless of
  `hidden_dirs`; `Library` (non-hidden on macOS) and `go/pkg/mod` must stay in
  the defaults — they are not hidden, so the hidden policy never touched them.

### A.2.4 Per-repo worktree directory override (D8)

```toml
[worktrees]
directory = "~/.worktrees"            # global default

[[worktrees.repos]]                   # per-repo override (repeatable)
path = "~/src/myproject"              # selector: any worktree path of the repo
directory = "~/wt/myproject"          # destination root for THIS repo only
```

- `path` matches the repo's **main worktree path, any linked worktree path, or
  its common git dir** — the value is `~`-expanded, `Abs`+`EvalSymlinks`
  canonicalized, then compared against the same canonicalization of each
  discovered repo's paths. A `--repo` flag pointing at a linked worktree
  resolves to the parent repo, so the override applies to the parent.
- Exact match only (no globs, no basename matching). If a configured `path`
  matches no discovered repo, warn at startup (non-fatal).
- Precedence for a repo's destination root:
  `[[worktrees.repos]] .directory` > `worktrees.directory` > built-in default
  `~/.worktrees`. The per-repo value inherits the `~`-expansion and
  `<repo-basename>/<branch-slug>` layout rules of D8 (the override replaces
  the *root*, not the `<repo>/<branch>` sub-layout).
- Future per-repo keys (e.g., slug overrides) extend this table without
  changing the shape.

### A.2.5 Validation, strictness, reload

- TOML type mismatches and malformed values are **fatal** startup errors that
  name the offending key (e.g., `theme.variant: want "auto"|"light"|"dark"`).
- Unknown keys: decoded strictly (BurntSushi/toml `MetaData.Undecoded()`) →
  **warning** at startup, non-fatal (forward-compatible configs).
- Out-of-range numerics: clamped with a warning — `workers` to [4, 16];
  nonzero `refresh.poll_interval_ms` to ≥ 1000; `hidden_peek_depth` to ≥ 1.
- `discover.max_depth = 0` means "no cap" (accepted deliberately; keep the
  startup warning).
- Config is read **once** at startup; no live reload in v1 (documented; a
  future `R`-reload is a post-v1 candidate).
- Precedence overall: flags > config > defaults. Flags in v1: `--repo`,
  `--scan-root`, `--no-scan`, `--config`, `--version`, `--help` (D7). No
  config key exists for git binary location — `git` resolves via PATH
  (`exec.LookPath`, 03-git-layer §2).

## A.3 Default `discover.ignore` list

Basename segments (match any path segment):
`.git` (structural — always skipped, cannot be removed), `node_modules`,
`vendor`, `.venv`, `venv`, `Pods`, `DerivedData`, `.Trash`, `Library`,
`.cache`, `target`, `dist`, `build`, `.next`, `.gradle`, `.m2`, `.conda`,
`.rustup`, `.cargo`, `.npm`, `.bun`, `.pnpm-store`, `.dart_tool`,
`__pycache__`.

Trailing-path suffix (matches the trailing segments of a dir's path relative
to the scan root): `go/pkg/mod`.

Matching rule: an entry without `/` matches any single path segment; an entry
with `/` matches the *trailing* segments of a directory's relative path
(e.g., `go/pkg/mod` skips `~/go/pkg/mod` but not `~/src/go` or `~/src/gomod`).

## A.4 Example `config.toml` (complete)

```toml
# tree-trunk configuration. Defaults shown commented out.
# Loaded from ~/.config/tree-trunk/config.toml (override: --config PATH).

# Worker pool for git commands and scan fan-out (4-16, clamped).
workers = 8

[discover]
max_depth = 8                # 0 = unlimited
scan_roots = ["~/src", "~/code"]   # ADDS to the default $HOME scan
scan_home = true             # false = don't scan $HOME at all
include_bare = false         # opt-in bare-repo detection (v1 default off)

# Override the defaults from A.3 (this REPLACES the default list,
# so copy the defaults you still want; .git is always skipped).
# ignore = ["node_modules", "vendor", ".venv", "venv", "Pods", "DerivedData",
#           ".Trash", "Library", ".cache", "target", "dist", "build",
#           ".next", ".gradle", ".m2", ".conda", ".rustup", ".cargo",
#           ".npm", ".bun", ".pnpm-store", ".dart_tool", "__pycache__",
#           "go/pkg/mod"]

follow_symlinks = false      # true = follow dir symlinks (loop-protected)
hidden_dirs = "peek"         # "skip" | "peek" | "scan"
hidden_peek_depth = 2        # levels below a hidden dir searched when "peek"

[worktrees]
directory = "~/.worktrees"   # global worktree root: <root>/<repo>/<branch-slug>

# Per-repo override (D8): this repo's worktrees live elsewhere.
# path matches the repo's main worktree path, any linked worktree path,
# or its common git dir (canonicalized; exact match only).
[[worktrees.repos]]
path = "~/src/myproject"
directory = "~/wt/myproject"

[refresh]
poll_interval_ms = 0         # 0 = off (on-demand + R only);
                             # e.g. 5000 = background poll every 5s
                             # (status re-read each poll; branch/ahead-behind
                             # only when the refs+HEAD fingerprint changed)

[theme]
name = "default"             # built-in theme; unknown -> warning + "default"
variant = "auto"             # "auto" | "light" | "dark" (auto: P1 -> dark)
[theme.overrides]            # palette keys: normal, dim, accent, dirty,
accent = "#a6e3a1"           #   conflict, clean, worktree_child
                             # NO_COLOR is always respected (04 §8)

[clipboard]
enabled = true               # false = "o" prints path only (headless);
                             # on copy failure: toast + print-only fallback
```

---

# DRAFT B — Reallocated keybinding registry (M4)

Replaces `04-tui-layout.md` §3. Single source of truth: one `[]KeyBinding{Key,
Contexts, Action, Help, DisabledWhen}` table drives the hint bar and the `?`
cheatsheet. Rules:

1. **One verb, one key, one context.** A key appears at most once per context,
   and a verb uses the same key wherever it appears (`o` open, `n` new
   worktree, `d` delete, `L` lock/unlock, `P` prune, `c` copy, `p` stat/raw).
2. **Global keys are never reused by persistent contexts.** The reserved
   global set: `? q ctrl+c ctrl+z R / tab shift+tab + _ g G ctrl+r esc`
   plus movement primitives `j k h l` and arrow keys, and `enter` as the
   per-context confirm/activate key.
3. **Modal override exception:** Forms, the help cheatsheet, and search input
   are transient modals that **suspend** the global set while open; their keys
   (`enter`, `esc`, `tab`, `ctrl+space`, typing) are scoped to the modal.
   This is deliberate and mirrors lazygit; do not "fix" it.
4. **`+`/`_` are global fullscreen only** — no tab-level binding may use
   `+`, `-`, or `_`.
5. **Every binding carries a DisabledWhen reason** (grayed in hint bar /
   cheatsheet, per lazygit VISION).
6. Verified at the end: no key appears twice in any context; no persistent
   context reuses a global key.

## B.1 Global (all contexts)

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

## B.2 RepoList

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

## B.3 Inspection tabs — common

Available in Status, Log, Diff, Worktrees:

| Key | Action | DisabledWhen |
|---|---|---|
| `1`/`2`/`3`/`4` | Switch tab (Status/Log/Diff/Worktrees) | — |
| `[` / `]` | Cycle tab (prev / next) | — |
| `j`/`k` | Per-tab movement (see below) | — |

## B.4 Status

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | — (unbound) | — |
| `enter` | Open file → diff scoped to that file | no file selected (clean status) |
| `x` / `X` | Expand / collapse file detail | no file selected / nothing to expand |
| `c` | — | — |
| `d` | — (delete is a worktree op; not here) | — |
| `n` | — | — |

## B.5 Log

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | Move commit selection | — |
| `enter` | Show commit diff (`commit^..commit`) | no commit selected |
| `w` | Create worktree from commit | no commit selected |
| `c` | Copy commit hash | no commit selected |
| `n` / `N` | Next / previous search match | search inactive (opened with `/`) |
| `pgup` / `pgdn` | Page up / down | — |
| `ctrl+u` / `ctrl+d` | Half-page up / down | — |
| — (auto) | "Load more" at scroll end (page size 200) | — |

## B.6 Diff

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | Scroll | — |
| `enter` | — (unbound) | — |
| `p` | Toggle stat summary / full diff | — |
| `c` | Copy path under cursor (stat mode) | not stat mode / no path |
| `pgup` / `pgdn` | Page up / down | — |
| `ctrl+u` / `ctrl+d` | Half-page up / down | — |
| — (auto) | Truncation footer ("truncated — N lines") | — |

## B.7 Worktrees

| Key | Action | DisabledWhen |
|---|---|---|
| `j`/`k` | Move row | — |
| `enter` | — (unbound; `o` is the open verb — one verb, one key) | — |
| `n` | New worktree in this repo | — |
| `d` | Delete (two-step: safe → force confirm) | row is the main worktree (git refuses: "is a main working tree") |
| `o` | Open: print path + copy to clipboard | — |
| `L` | Lock / unlock (toggle; shows reason) | row is the main worktree (git refuses: "The main working tree cannot be locked or unlocked") |
| `P` | Prune (dry-run preview → confirm) | — (locked worktrees skipped) |
| `u` | — (freed; unlock is `L`) | — |

## B.8 Forms & modals (modal override — see rule 3)

| Key | Action |
|---|---|
| `enter` | Confirm (create-worktree form, delete-confirm, prune-confirm) — DisabledWhen: form invalid (e.g., empty branch, path exists) |
| `esc` | Cancel |
| `tab` | Next field (create form: branch → base → path) |
| `ctrl+space` | Toggle suggestion (existing-branch completions) |

Help cheatsheet (`?`): `?`/`esc` close, `/` filters bindings. Search input
(Log, opened with `/`): typing edits the query, `enter` jumps, `esc` exits,
`n`/`N` iterate (B.5).

## B.9 Key-allocation notes (merge these into §3 prose)

- **Freed keys:** `D` (delete → `d`), `u` (unlock → `L`), `-` and tab-level
  `+` (paging → `pgup`/`pgdn`, `ctrl+u`/`ctrl+d`; expand → `x`/`X`). `D` and
  `-` are reserved for future use (e.g., batch ops F1, hunk folding P1).
- **New bindings:** `x`/`X` (Status expand/collapse — free in every context),
  `pgup`/`pgdn` + `ctrl+u`/`ctrl+d` (Log/Diff paging — free everywhere),
  `P` (prune — free everywhere), `L` (lock/unlock — unified from `l`/`L` +
  `u`).
- **Deliberate same-key/different-object pairs (allowed):** `c` = copy in
  Log (hash) and Diff (path) — same verb, different contexts, never together.
- **Verified conflict-free:** each key appears at most once per context
  table above; no persistent context reuses `? q ctrl+c ctrl+z R / tab
  shift+tab + _ g G ctrl+r esc`; `+`/`-`/`_` no longer appear in any
  non-global table; `l` appears once in RepoList (expand — the lock verb
  moved to `L`).

---

# §3 Resolved-findings checklist (M7, M4 and supporting)

| Finding | Status | Resolved by |
|---|---|---|
| **M7** config schema missing | ✅ done | Draft A §A.1 reference table (all keys incl. `discover.*`, `worktrees.*`, `workers`, `theme.*`, new `refresh.poll_interval_ms`, `[clipboard] enabled`), §A.2 prose rules, §A.3 default ignore list, §A.4 example file |
| **M4** keybinding conflicts (`l` double-bound; global `+` vs tab `+`; `l`/`u` and `D`/`d` verb drift) | ✅ done | Draft B §B.2–B.9 — `l` = expand only; `L` = lock/unlock everywhere; `d` = delete everywhere (`D` freed); `+`/`_` global-only; paging on `pgup`/`pgdn` + `ctrl+u`/`ctrl+d`; expand/collapse on `x`/`X`; conflict verification in §B.9 |
| **M6** prune had no keybinding | ✅ done | Draft B §B.2 (`P` on worktree rows) + §B.7 (`P` in Worktrees tab), dry-run preview → confirm; verified `git worktree prune -n` skips locked worktrees |
| **M10** refresh trigger policy | ✅ done (config half) | Draft A — `refresh.poll_interval_ms` (default 0 = off) + A.2.5 precedence; policy text (on-selection + `R` + optional poll; status always re-read, ref reads fingerprint-gated) belongs in 01-architecture §3.3 alongside the M2/M3 fix the owner is applying |
| **M8** scan policy (hidden/symlink) | ✅ done (schema half) | Draft A — `discover.hidden_dirs` (skip/peek/scan, default peek), `hidden_peek_depth`, `follow_symlinks` (loop-protected), A.2.3 prose; walker-behavior text pairs with the owner's M8 implementation |
| **m9** `--scan-root` add-vs-replace | ✅ done (decision) | Draft A §A.2.2 — flags REPLACE defaults+config; config `scan_roots` ADDS to `$HOME`; `scan_home` opt-out; `--no-scan` overrides all |
| **m8** clipboard unspecified | ✅ done | Draft A — `[clipboard] enabled` + failure degradation (toast + print-only) |

Not drafted here (owner applying separately): B1 version sweep, M1 identity
canonicalization, M2/M3 fingerprint split, M9 per-file diffs, M5 worktree
dirty state, m1/m2/m4/m6/m11/m12/m13 minors.
