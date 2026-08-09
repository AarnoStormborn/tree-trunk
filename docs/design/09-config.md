# Configuration — tree-trunk

> Design doc (review M7). Config file: `~/.config/tree-trunk/config.toml`
> (D7); override with `--config PATH`. A missing file is not an error
> (defaults apply). All keys are optional. `[new]` = introduced by the design
> review round (fixes M7/M8/M10/m8/m9; drafted in `08-reviewer-drafts.md`).

## 1. Reference table

| Key | Type | Default | Example | Source |
|---|---|---|---|---|
| `workers` | int | `8` | `8` | 01-architecture §3.1 — bounded pool 4–16; clamped into range |
| `refresh.poll_interval_ms` | int | `0` (off) | `5000` | **[new]** (M10) — background poll cadence; status always re-read, ref reads fingerprint-gated; 0 disables; nonzero clamps to ≥ 1000 |
| `discover.max_depth` | int | `8` | `8` | 02-data-model §2.2 — depth cap from scan root; `0` = unlimited (use with care) |
| `discover.scan_roots` | []string | `[]` | `["~/src", "~/code"]` | 02-data-model §2.2, D4 — **ADDS** to default `$HOME` scan (see §2.2) |
| `discover.scan_home` | bool | `true` | `true` | **[new]** (m9) — `false` drops the default `$HOME` root (config-only opt-out) |
| `discover.ignore` | []string | see §3 | `["node_modules", "vendor"]` | 02-data-model §2.2, review M8 — path segments or trailing-path suffixes; `.git` is structural and always skipped |
| `discover.include_bare` | bool | `false` | `false` | 02-data-model §2.1 — opt-in bare-repo detection (v1 default off, F8) |
| `discover.follow_symlinks` | bool | `false` | `false` | **[new]** (M8) — follow dir symlinks with loop protection (visited-set of `EvalSymlinks`-resolved paths); depth cap still applies |
| `discover.hidden_dirs` | `"skip"` \| `"peek"` \| `"scan"` | `"peek"` | `"peek"` | **[new]** (M8) — see §2.3 |
| `discover.hidden_peek_depth` | int | `2` | `2` | **[new]** (M8) — levels of hidden subtree searched when `hidden_dirs = "peek"` |
| `worktrees.directory` | string | `"~/.worktrees"` | `"~/wt"` | D8 — global worktree destination root |
| `[[worktrees.repos]] .path` | string | — | `"~/src/myproject"` | D8 — per-repo override selector (see §2.4) |
| `[[worktrees.repos]] .directory` | string | — | `"~/wt/myproject"` | D8 — per-repo destination root |
| `theme.name` | string | `"default"` | `"catppuccin"` | 04-tui-layout §8 — built-in theme name; unknown name → warning + fallback to `"default"` |
| `theme.variant` | `"auto"` \| `"light"` \| `"dark"` | `"auto"` | `"dark"` | 04-tui-layout §8 — `auto` = terminal-background detection (P1; until implemented, `auto` behaves as `dark`) |
| `theme.overrides.<key>` | string (hex `#rrggbb`) | — | `"#a6e3a1"` | 04-tui-layout §8 — palette keys: `normal`, `dim`, `accent`, `dirty`, `conflict`, `clean`, `worktree_child` |
| `clipboard.enabled` | bool | `true` | `true` | **[new]** (m8/D11) — `false` = `o` prints path only (headless/WSL/SSH); on copy failure: toast + print-only fallback |

## 2. Prose rules

### 2.1 `~` expansion (all path values)

- A leading `~` or `~/…` in **any** path-valued key (`worktrees.directory`,
  `[[worktrees.repos]] .path` / `.directory`, `discover.scan_roots` entries)
  expands via `os.UserHomeDir()`; failure to determine HOME is a startup
  error.
- `~user/…` forms are **not** supported.
- Non-`~` relative paths resolve against the **current working directory**
  (not the config file's directory) — document this in the config reference;
  recommend absolute or `~`-prefixed paths in config.
- `--scan-root` and `--repo` flag values get the same `~` expansion and cwd
  resolution.

### 2.2 Scan-root precedence (m9)

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

### 2.3 Hidden-dir and symlink policy (M8)

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
- The heavy-dir default `ignore` list (§3) applies regardless of
  `hidden_dirs`; `Library` (non-hidden on macOS) and `go/pkg/mod` must stay in
  the defaults — they are not hidden, so the hidden policy never touched them.

### 2.4 Per-repo worktree directory override (D8)

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

### 2.5 Validation, strictness, reload

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

## 3. Default `discover.ignore` list

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

## 4. Example `config.toml` (complete)

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

# Override the defaults from §3 (this REPLACES the default list,
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
