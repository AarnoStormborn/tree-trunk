# Data Model — tree-trunk

> Design doc. Domain entities and the discovery model. Conventions: git terms
> follow `git-worktree(1)` / `git-status(1)`; facts verified in research track 1.

## 1. Entities

### 1.1 `Repo`

A repository **as a collection of worktrees** sharing one object store.

```go
type Repo struct {
    ID        string          // canonical: common git dir path (see §2)
    Name      string          // basename of common dir / display name
    Path      string          // main worktree path (empty for bare)
    GitDir    string          // common git dir (source of truth for identity)
    Bare      bool
    Worktrees []Worktree      // main first, then linked (from `git worktree list`)
    Branch    string          // current branch of the MAIN worktree ("HEAD" if detached)
    Status    RepoStatus      // aggregated from main worktree status
    RefState  string          // fingerprint for refresh dedup (D9/§3.3)
    Lifecycle RefreshState    // stale | refreshing | fresh | error
    LastError error
}
```

Key decisions:
- **Identity = canonicalized common git dir**, not the working directory.
  Linked worktrees point at the same common dir, so they fold under one
  `Repo` row (research open question #10 answered: worktrees are children,
  not top-level entries).
- **Canonicalization rule (design review M1):** the raw `--git-common-dir`
  value is NOT a stable key — `git rev-parse --git-common-dir` returns a
  *relative* path (`.git`) from the main worktree but an absolute path from a
  linked worktree, and macOS symlinked paths diverge (`/tmp` → `/private/tmp`).
  Key = `filepath.EvalSymlinks(filepath.Abs(commonDir))`. For discovery via a
  linked worktree's `.git` *file*: resolve `gitdir:` relative to the worktree
  dir, then strip the `worktrees/<name>` suffix (or simply run
  `git rev-parse --git-common-dir` from that directory). Fixture test:
  discovering a repo via its main worktree and via one of its linked
  worktrees must yield identical IDs.
- `Name` is the repo directory basename; a `--repo /path/to/foo` flag resolves
  to the same `Repo` even when the path is a linked worktree.

### 1.2 `Worktree`

```go
type Worktree struct {
    Path      string   // absolute working directory
    GitDir    string   // <.git dir>/worktrees/<name> for linked; common dir for main
    Branch    string   // short branch name, "" when detached
    IsMain    bool
    IsCurrent bool     // worktree whose path matches the cwd tree-trunk was
                       // launched from (lazygit semantics) — the only
                       // meaningful definition; git has no "current" notion
    Locked    bool
    LockReason string
    Prunable  bool     // git marks it for pruning (missing path etc.)
    Head      string   // full 40-hex commit hash (shorten at render time)
    Dirty     bool     // modified/untracked/conflicted files present (see §1.6)
    IsPathMissing bool // prunable reason text is exactly
                       // "gitdir file points to non-existent location" (match on it)
}
```

- Source of truth: `git worktree list --porcelain -z` (stable, NUL-safe;
  research track 1 §2.1). The `worktree_parse.go` parser mirrors lazygit's
  `worktree_loader.go` semantics: multi-line records, `worktree`, `HEAD`,
  `branch`, `detached`, `locked`, `prunable` fields. Byte-level format in
  `03-git-layer.md` §4.3 (review m1).
- **Branches are shared across worktrees; a branch is checked out in exactly
  one worktree at a time.** The `CheckedOutByOtherWorktree` guard lives in
  the git layer and prevents creating a second worktree on a checked-out
  branch without `--detach`. *(01-existing-tooling §2.1, 04 §1.3.)*

### 1.3 `Branch`

```go
type Branch struct {
    Name         string
    Head         string   // commit hash
    Upstream     string   // "origin/main" or ""
    Ahead, Behind int
    IsCurrent    bool
    IsCheckedOut bool     // by ANY worktree (blocks non-detached add)
}
```

- Read via `git for-each-ref refs/heads --format=%(refname:short)%00%(objectname:short)…`
  and `git rev-list --left-right --count HEAD...@{upstream}` for ahead/behind
  (only when upstream exists).
- `IsCheckedOut` computed by cross-referencing `git worktree list`.

### 1.4 `Commit` (log row)

```go
type Commit struct {
    Hash      string   // short hash
    Author    string
    AuthorDate time.Time
    Subject   string
}
```

- Log rendering uses `git log --format=%h%x00%an%x00%aI%x00%s -z -n N` (N = page
  size, default 200; "load more" on scroll to end). Never load full history.
  *(04-inspiration §1.4, §6 P0-13.)*

### 1.5 `RepoStatus`

```go
type RepoStatus struct {
    Staged       int    // count of staged files
    Unstaged     int    // count of modified/untracked
    Untracked    int
    Conflicts    int
    Ahead, Behind int   // vs upstream, from the main worktree branch
    Summary      string // compact one-line rendering for the repo list row
}
```

- Parsed from `git status --porcelain=v1 -z` (`XY path` records, NUL-separated
  with `\0` between fields; rename records carry two paths). Parse with the
  `-z` variant only — never the human-readable text. *(03-go-packages §2.2.)*

### 1.6 Per-worktree dirty state (design review M5)

- `Worktree.Dirty` is set by running `git status --porcelain` **in that
  worktree's directory** (`-c core.worktree` not needed; run with cwd = worktree
  path).
- **Collection scope (lazy):** only for worktrees of the *focused* repo and
  repos currently visible in the list — never for every worktree of every
  repo. `Repo.Status` covers the main worktree; linked worktrees get their own
  dirty flags on demand.
- Render: child rows and the Worktrees tab show a dirty icon (e.g. `~`) next
  to the branch; the repo row's summary only reflects the main worktree
  (documented limitation), but the worktree child rows surface the rest.

## 2. Repo identity & discovery model

### 2.1 Detection rules (`internal/discover`)

Walking a tree, at each directory entry named `.git`:

| Entry kind | Meaning | Action |
|---|---|---|
| **Directory** | Normal (non-bare) repo — main worktree | Emit as repo root; **do not descend**; skip its contents |
| **File** starting with `gitdir: ` | Linked worktree **or submodule** | Resolve `gitdir:` (relative to the worktree dir!); map to common dir; fold under the parent `Repo` if already known, else emit as repo with a linked worktree |
| **File** not starting with `gitdir:` | Submodule / odd layout | Skip (submodules belong to their parent repo; v1 does not treat submodules as repos) |
| — (bare: no `.git` entry but `HEAD` + `objects/` + `config` with `core.bare=true`) | Bare repo | Optionally emit with `Bare=true` (config `discover.include_bare`, default **false** for v1) |

**Bare-repo probing cost (review m10):** do not stat every directory for
`HEAD`/`objects/` during the walk. Only probe a directory as potentially bare
when `discover.include_bare` is true **and** the dir name looks like a repo
(contains `HEAD` + `objects/`), checking at the same spots where a `.git`
entry would appear (repo-looking dirs at shallow depth).

This is **the single most likely bug source** in repo scanning — `.git` as a
file must be checked *before* assuming a directory. *(03-go-packages §3.1;
01-existing-tooling §2.2.8: `-type d -name .git` misses linked worktrees.)*

### 2.2 Scan roots, hidden dirs & symlinks

- **Default roots** (D4, Q1): **`$HOME` only**, bounded by
  `discover.max_depth` (default 8). `--scan-root DIR` **replaces** the default
  roots; config `discover.scan_roots` **adds** to them (review m9). A repo
  found at depth d stops descent below it.
- **Skip rules** (config `discover.ignore`, sensible defaults): any path
  segment matching `.git`, `node_modules`, `vendor`, `.venv`, `venv`,
  `Pods`, `DerivedData`, `.Trash`, `Library`, `.cache`, `go/pkg/mod`,
  `target`, `dist`, `build`. **Curated heavy-app dirs** are skipped even
  though they live under hidden dirs: `.config/Code`, `.config/google-chrome`,
  `.config/gh`, `.npm`, `.cargo/registry`, `.rustup/toolchains`, `.local/lib`.
- **Hidden dirs (review M8):** do NOT blanket-skip. Hidden directories are
  descended only to detect repos at *their* top level or shallow depth
  (default 2) — this finds dotfiles repos (`~/.emacs.d`, `~/.vim`,
  `~/.dotfiles`, `~/.oh-my-zsh`) without walking `~/.config/Code/...` trees.
  A hidden dir that is itself a repo stops descent. The `.config` carve-out
  is replaced by the curated list above.
- **Symlinks (review M8):** default **do not follow**; config
  `discover.follow_symlinks` (default false) enables following with **loop
  protection**: visited real-path set (via `EvalSymlinks`) + depth cap, so
  `~/dev -> /Volumes/…` and iCloud/Dropbox dirs are found when enabled.
- **Depth cap:** config `discover.max_depth` (default 8).
- **Performance:** single `WalkDir` pass in a goroutine; `SkipDir` on skips;
  results streamed to the store as found (incremental display, architecture
  §5). *(01-existing-tooling §4.1; 03-go-packages §3.1.)*

### 2.3 `--repo` flags

- Repeatable; accepts paths to: a repo root, a linked worktree, or a bare
  repo dir. Each is resolved via `git rev-parse --git-common-dir` and merged
  into the same `Repo` key. A `--repo` path that isn't inside a git repo is a
  hard startup error.
- `--no-scan` runs only on `--repo` inputs (useful when the machine scan is
  too slow or the user wants a curated session).

## 3. Aggregation: the repo-list row

Each repo row renders (left → right), herdr-sidebar-token style
*(04-inspiration §3.2.1, §6 P0-5)*:

```
[✓] myproject  main   ↑2↓1  ~3 +1  (2 worktrees)
 │   │          │      │     │      └─ worktree count when > 0
 │   │          │      │     └─ status summary: ~modified +added
 │   │          │      └─ ahead/behind vs upstream (only when nonzero)
 │   │          └─ main-worktree branch (collapsed when detached: "HEAD")
 │   └─ repo name
 └─ lifecycle/state icon: ✓ clean · ~ dirty · ! conflict · e error · ↻ refreshing
```

Missing values collapse (ahead/behind 0 → hidden; clean → no status summary).
Worktrees render as indented children rows under their repo (expandable).

**Glyph rule (review m13)** — the status summary maps counts to glyphs:
- `~N` = unstaged modifications (Unstaged count, minus untracked),
- `+N` = untracked files (Untracked),
- `*N` = staged (Staged),
- `!N` = conflicts (Conflicts, always shown first in red),
- only nonzero segments render; order: conflicts → staged → unstaged →
  untracked.

## 4. Open data-model questions (tracked)

1. **Submodules:** treated as skips in v1. `git worktree` itself warns that
   submodules + multiple checkouts are experimental (man page BUGS) — so
   ignoring them aligns with git's own guidance. *(01-existing-tooling §2.2.1.)*
2. **Bare repos:** default off (config opt-in). A bare repo has no worktree;
   the main use (grove-style managed repos, F8) is deferred.
3. **Two repos with the same basename:** identity is by common-dir path, so
   no collision; `Name` may duplicate — the row shows a path suffix
   (`myproject …/a/myproject`) when ambiguous.
4. **Re-scan:** manual `R` refresh re-runs discovery; removed repos disappear
   from the store (with a toast). fsnotify live-watch is P1 (research:
   03-go-packages §3.2 — not needed for v1).
