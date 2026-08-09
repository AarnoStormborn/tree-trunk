# Git Layer — tree-trunk

> Design doc. All interaction with the `git` binary lives in `internal/git`
> (architecture §2). Nothing else in the codebase shells out.

## 1. Design principles

1. **Shell out to the system `git`** (D1). Full worktree semantics, hooks,
   credentials, submodules, LFS — for free, always correct. go-git's worktree
   support is experimental and incomplete (`v6/x/plumbing/worktree`: only
   `Add`; no remove/prune/lock), and slower. *(03-go-packages §2.)*
2. **Machine-parseable output only:** `--porcelain`, `-z`/`--null` everywhere.
   Never parse human-readable output.
3. **Typed command objects** (lazygit `git_commands` style) returning typed
   models, never raw strings leaking into the UI.
4. **Every exec is cancellable** (context) and **bounded** (worker pool, §3 of
   architecture).
5. **Errors carry codes + suggested actions** (§6).

## 2. `GitRunner` interface

```go
// Runner is the single git access point. UI/state depend on this interface,
// not on exec — tests can swap a fake or a sandboxed real git.
type Runner interface {
    Run(ctx context.Context, args ...string) ([]byte, error)                    // capture stdout
    RunStream(ctx context.Context, args ...string, w io.Writer) error           // stream stdout
    RunIn(ctx context.Context, dir string, args ...string) ([]byte, error)      // set working dir
    RunPaged(ctx context.Context, args ...string, onLines func(line []byte) error) error
}
```

Implementation notes:

- `exec.CommandContext` + `Cmd.WaitDelay` (Go ≥ 1.20) so a killed process
  can't linger; `Cmd.Cancel` on context cancel. On Unix, SIGTERM then SIGKILL
  after grace. *(02-go-suitability §7.2; 04-inspiration §1.3.)*
- **Lock-retry wrapper** (`runner_lock.go`): on stderr matching
  `index.lock` / `cannot lock ref`, retry with exponential backoff —
  20 ms initial, doubled, up to 7 attempts (≈1 s). tree-trunk races its own
  refreshes against its own worktree ops, exactly the lazygit race that
  motivated this. *(04-inspiration §1.3, §6 P0-1.)*
- `GIT_OPTIONAL_LOCKS=0` for read-only commands (status/log) to avoid
  creating lock files at all.
- Explicit `git` resolution: use `exec.LookPath("git")` at startup; on
  Windows, resolve `git.exe` via PATH (golang/go#66586) — best-effort (D6).

## 3. Command catalog (v1)

### 3.1 Repo-level read commands

| Purpose | Command | Parse | Notes |
|---|---|---|---|
| Identify repo | `git rev-parse --git-common-dir --git-dir --is-bare-repository --show-toplevel` | newline fields | run in candidate dir; source of `Repo.ID` |
| Current branch | `git symbolic-ref --short HEAD` (or `--quiet` + rev-parse for detached) | single line | detached → `HEAD` + short hash |
| Status | `git status --porcelain=v1 -z --branch` | NUL records (§4.1) | `--branch` gives `## main...origin/main [ahead 2, behind 1]` |
| Branches | `git for-each-ref refs/heads --format=%(refname:short)%00%(objectname:short)%00%(upstream:short)` | `%00` fields | plus `git rev-list --left-right --count` per branch-with-upstream (lazy, only for focused repo) |
| Fingerprint | `git for-each-ref --format=%(objectname) refs/heads refs/remotes` **plus** `git rev-parse --verify HEAD` (separate call), joined | refresh dedup (D9; review M3: `HEAD` as a positional arg to for-each-ref matches nothing) |
| Log | `git log --format=%h%x00%an%x00%aI%x00%s%x00 -z -n 200` | NUL records (§4.2) | paged: next page = `--skip 200` |

### 3.2 Worktree commands (the core)

| Purpose | Command | Notes |
|---|---|---|
| List | `git worktree list --porcelain -z` | **source of truth**; NUL-safe; v2 format (git ≥ 2.38, D1/Q2) |
| Add (new branch) | `git worktree add -b <branch> <path> <base>` | `base` = commit-ish (default HEAD of main worktree) |
| Add (existing branch) | `git worktree add <path> <branch>` | guard: branch must not be checked out elsewhere (§5); **if branch exists only as a remote ref** (`origin/x`), add `--guess-remote` (review m6) |
| Add (detached) | `git worktree add --detach <path> <commit-ish>` | offered when branch is checked out elsewhere |
| Remove (safe) | `git worktree remove <path>` | **first attempt**; refuses if dirty |
| Remove (forced) | `git worktree remove --force <path>` | only after explicit user confirmation (§5) |
| Lock / Unlock | `git worktree lock [--reason <txt>] <path>` / `unlock` | shown in porcelain output |
| Prune | `git worktree prune` | after manual deletions; also `--dry-run` to preview |

Flow for **create** (herdr `worktree.create` semantics, 04 §3.2):

1. User picks branch name (default: auto-suggest `<branch>` from selection or
   type one) and base ref (default: main-worktree HEAD).
2. If the branch exists locally → `add <path> <branch>`; else
   `add -b <branch> <path> <base>`.
3. Show the target path in the confirm dialog (D8 default:
   `~/.worktrees/<repo>/<branch>`).
4. On success: toast + refresh store (`WorktreeAdded` event) + offer to open
   the worktree (`cd`-helper prints path; v1 has no shell-spawn).

Flow for **remove** (two-step, never auto-force, never delete branches):

1. `git worktree remove <path>` (safe).
2. On refusal (dirty/locked) → dialog: *"Worktree has modified/untracked
   files. Remove with --force?"* — explicit confirm, destructive-action
   styling. `git worktree remove --force <path>`.
3. **Never** delete branches, even on force-remove (branch may be shared;
   user does it explicitly elsewhere). *(04-inspiration §3.2.2.)*

### 3.3 Diff

| Purpose | Command | Notes |
|---|---|---|
| Working tree vs HEAD | `git diff` | streamed; `--stat` first, expandable |
| Staged vs HEAD | `git diff --cached` | |
| Branch vs main | `git diff <main>...<branch>` | "inspect against main active branch" (product requirement #4) |
| Commit vs parent | `git diff <commit>^ <commit>` | from log view |

Per-file variants (review M9 — needed by the status → diff flow):

| Purpose | Command | Rules |
|---|---|---|
| File working-tree diff | `git diff -- <path>` | only for tracked files |
| File staged diff | `git diff --cached -- <path>` | |
| File status→diff | `git diff HEAD -- <path>` (unstaged), `git diff --cached -- <path>` (staged) | untracked (`??`) has **no diff** — show "untracked file" placeholder; rename (`R`) diffs against the **old path** (`git diff -- <oldpath>`) |

- Always `--no-color` (or `--color=always` + strip per theme), `--no-ext-diff`,
  `--no-textconv`. Stream into the viewport (lazygit `tasks`, §7).

## 4. Parsers (NUL-safe, defensive)

### 4.1 Status records

`git status --porcelain=v1 -z`:

```
XY <path>\0<origPath>\0   (X: index, Y: worktree; '?' for untracked)
```

- Rename/copy records carry two NUL-separated paths (the `-z` format
  NUL-separates *fields*; the normal format separates records). Parse fields
  first, then detect `XY` codes: `??` untracked, `UU`/`AA`/`DD` conflicts,
  `M`/`A`/`D`/`R`/`C` staged vs unstaged.
- `--branch` header line `## main...origin/main [ahead 1, behind 2]` parsed
  with regexes (defensive — format has varied across versions).

### 4.2 Log records

`--format=%h%x00%an%x00%aI%x00%s -z` → records split on NUL, fields on
NUL. **No trailing `%x00`** — `-z` already emits a record terminator, so a
format-level `%x00` produces a double NUL and an empty trailing field per
record (review m2). Subjects with newlines are safe because `-z` uses NUL
terminators (verify against real output in tests; some git versions still
emit newlines inside `%s`).

### 4.3 Worktree porcelain records (byte-level spec; review m1)

`git worktree list --porcelain -z` (git ≥ 2.38, D1/Q2) emits records:

```
worktree <path>\0
HEAD <40-hex-sha>\0
branch refs/heads/<name>\0          // present for the MAIN worktree too when on a branch
                                    // ABSENT and replaced by:  detached\0
                                    // (no value) when the worktree's HEAD is detached
locked <reason>\0                   // only when locked; empty reason => bare "locked\0"
prunable <reason>\0                 // only when prunable; reason may be empty
\0                                  // extra NUL: record separator (fields are NUL-terminated)
```

Parsing rules:
- Split the stream on NUL; fields are the non-empty tokens; `HEAD` values are
  **full 40-hex** (shorten at render time, per 02 §1.2).
- `branch` line ⇒ on a branch (value `refs/heads/<name>` → short name);
  `detached` line ⇒ detached HEAD (no branch).
- `locked`/`prunable` with an empty reason still count as present; a record
  with both `locked` + empty reason means locked with no reason.
- Unknown keys are ignored; fields may be missing — parse defensively.
- `IsPathMissing`: set when prunable reason text is exactly `gitdir file
  points to non-existent location` (review m12).
- Main record vs linked: main is listed first; linked records share the
  common dir (identity canonicalization per 02 §1.1).

**Cross-version rule:** parse only the *stable documented subset*; treat any
unknown field as ignorable. Integration tests run against git 2.38+ and the
local git. *(02-go-suitability §7.6; D1/Q2.)*

## 5. Safety invariants

- **`CheckedOutByOtherWorktree` guard** — before `add` with a branch: cross-
  reference `git worktree list`; if the branch is checked out elsewhere,
  refuse and offer `--detach` (or jump to that worktree — error code
  `BranchCheckedOutElsewhere`, §6). *(04-inspiration §1.3.5.)*
- **Two-step remove** (§3.2): the safe attempt always comes first; `--force`
  requires a second explicit confirmation. Mirrors git's own refusal message
  ("…use --force").
- **No branch deletion** anywhere in v1 (git's `worktree remove` doesn't
  delete branches; neither do we).
- **No mutation of the main worktree's checkout** — worktree ops only touch
  linked worktrees in v1 (removing the main worktree is impossible anyway —
  git refuses).
- **Lock-aware:** a locked worktree cannot be removed/pruned; the UI shows
  the lock reason and offers `unlock` first.

## 6. Error taxonomy (`internal/git/errors.go`)

```go
type Code int
const (
    GitNotFound Code = iota
    GitTooOld         // version < 2.38 (D1/Q2)
    NotARepo
    WorktreeDirty
    WorktreeLocked
    BranchCheckedOutElsewhere
    BranchNotFound
    PathExists
    LockTimeout       // index.lock retries exhausted
    CommandFailed     // non-zero exit, stderr attached
)
type GitError struct{ Code Code; Args []string; Stderr string; Cmd []string }
```

Each code has a **suggested action** the UI maps to a dialog/toast
(lazydocker `ComplexError` pattern, 04 §2.5.5):

| Code | UI behavior |
|---|---|
| `WorktreeDirty` | Offer `--force` confirm dialog |
| `WorktreeLocked` | Offer unlock first |
| `BranchCheckedOutElsewhere` | Offer "jump to that worktree" |
| `PathExists` | Offer alternate path / `-b` rename |
| `LockTimeout` | Toast: "another git process holds the lock; retry" |
| `GitTooOld` | Startup error with upgrade hint |
| `NotARepo` | Toast + remove from list; `--repo` path → startup error |
| `CommandFailed` | Show stderr excerpt in toast; suggest retry |

## 7. Streaming (`internal/ui/streaming.go`)

Port of lazygit's `tasks` machinery (04 §1.3, §6 P0-10):

- **One streamed command at a time** into the main view; starting a new one
  cancels the previous (SIGTERM + WaitDelay).
- **Lazy line reads:** the view requests more lines as the user scrolls
  (`ReadLines(originY + viewHeight)`); a viewport resize grows the read
  window. Never read all output up front.
- **Throttle:** 30 ms between command starts; a stress heuristic
  (`COMMAND_START_THRESHOLD` = 10 ms) delays starts when the machine is
  busy.
- Long output (huge diffs) is **bounded**: show a "truncated — N lines"
  footer instead of unbounded buffering.

## 8. Testing the git layer

- **Unit:** parsers (status/log/worktree) against fixture outputs, including
  rename records, conflict codes, detached HEAD, locked/prunable variants.
- **Sandboxed integration** (lazygit's strategy, 03 §5.2): real `git init`
  repos in `t.TempDir()`; script real operations; assert on typed models.
  Fixtures: normal repo, repo with linked worktrees (`.git` file), bare repo,
  dirty worktree, locked worktree, branch-checked-out-elsewhere.
- **Git-version matrix:** CI job(s) on git 2.38 (oldest supported) + latest;
  the parser must pass both. (Q2: 2.38 floor.)
- **Fake runner** for UI/state tests: an in-memory `Runner` returning canned
  models (no git binary needed).
