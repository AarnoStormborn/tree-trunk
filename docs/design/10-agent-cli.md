# 10 — Agent-facing CLI

Status: **in progress** (read-API first; mutate later)
Date: 2026-08-24

## Motivation

tree-trunk's TUI is great for humans, but coding agents (pi, opencode, codex)
cannot drive a TUI. They need a deterministic, scriptable CLI that answers
"what's working where, and how much" and (eventually) mutates worktrees.

The engine already exists (`internal/git`): discovery, identity, status,
worktree list/create/delete/lock/prune. This doc defines the CLI contract that
exposes it to agents.

## Principles

1. **Stable, parseable output.** JSON with a versioned schema, sorted
   deterministically, no ANSI, no prose on stdout.
2. **Exit codes are part of the contract.** Not everything is `1`.
3. **Structured errors.** Agents must react programmatically, not regex stderr.
4. **Idempotent + scoped.** An agent can ask a question, act, and re-ask
   without changing state between calls.
5. **TUI stays the default.** `tree-trunk` with no args launches the TUI;
   subcommands are opt-in.

## Commands

### Read: `tree-trunk query`

```bash
tree-trunk query [--json] [--filter k=v]... [--fields a,b] \
                 [--repo PATH]... [--scan-root ROOT]... [--no-scan]
```

Emits a single JSON document:

```json
{
  "version": "0.1.0",
  "scanned_at": "2026-08-24T14:41:12Z",
  "roots": ["/Users/harshsingh"],
  "total_repos": 3,
  "repos": [
    {
      "id": "/Users/harshsingh/Documents/personal/tree-trunk/.git",
      "name": "tree-trunk",
      "path": "/Users/harshsingh/Documents/personal/tree-trunk",
      "git_dir": "...",
      "bare": false,
      "branch": "main",
      "worktrees": [
        {
          "path": "/Users/harshsingh/Documents/personal/tree-trunk",
          "git_dir": "",
          "branch": "main",
          "is_main": true,
          "is_current": true,
          "locked": false,
          "prunable": false,
          "head": "ed448e5f...",
          "dirty": false
        }
      ],
      "status": {
        "branch": "main",
        "upstream": "origin/main",
        "ahead": 2,
        "behind": 0,
        "files": [ { "path": "src/x.go", "staged": false, "unstaged": true,
                     "untracked": false, "conflict": false, "code": "M" } ],
        "staged": 0,
        "unstaged": 1,
        "untracked": 0,
        "conflicts": 0
      }
    }
  ]
}
```

- **`--filter`** (repeatable, AND-ed): `id`, `name`, `branch`, `dirty`,
  `prunable`, `locked`, `bare`, `worktrees` (count).
- **`--fields`** (comma/repeatable): trim each repo doc to top-level fields
  (`id,name,path,branch,status,worktrees,git_dir,bare`).
- Repos sorted by `id`; `--json` is the default (pretty), `--no-json` is the
  compact form (agents may prefer compact).

### Mutate (next iteration): `tree-trunk wt`

```bash
tree-trunk wt list [--repo]... [--json]
tree-trunk wt create <repo> <branch> [--json]
tree-trunk wt delete <repo> <branch> [--force] [--json]
tree-trunk wt lock <repo> <branch> [--reason S] [--json]
tree-trunk wt unlock <repo> <branch> [--json]
tree-trunk wt prune <repo> [--dry-run] [--json]
```

Reuses the guarded `internal/git` ops (branch-exists / checked-out-elsewhere /
dirty / locked). Errors structured:

```json
{ "ok": false, "error": { "code": "branch_checked_out_elsewhere",
  "message": "...", "repo": "tree-trunk", "branch": "feat/x" } }
```

### Planned

- `tree-trunk goto <repo> <branch>` — cd into a (created) worktree.
- `--stdin-action` JSON batch (plan → execute, `--dry-run` first).
- Exit-code contract: `0` ok, `1` usage/IO, `3` not-found, `4` dirty-blocked,
  `5` locked.

## Schema notes

- `Repo.id` is the canonical common git dir — stable across worktrees and
  symlinks (data model §1.1); it's the agent's handle.
- `status.files[].code` is the porcelain XY (e.g. `M`, `??`, `A`, `R`).
- `dirty` on a worktree is **lazy** — only populated for the focused repo in
  the TUI; `query` refreshes each repo once so linked worktree dirty flags are
  best-effort.
- `status.files` can be large on a dirty repo; use `--fields id,branch,status`
  and filter to keep payloads small.

## Build order

1. ✅ Subcommand skeleton + `query --json` (this PR).
2. `wt list` (read-only cross-repo aggregate).
3. Structured errors + exit codes on mutate.
4. `wt create/delete/lock/unlock/prune` via CLI.
5. `--jsonl` streaming; daemon/socket (F5) later if scan latency matters.
