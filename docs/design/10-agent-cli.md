# 10 — Agent-facing CLI

Status: **done** (read-API + mutating wt API + exit-code contract)
Date: 2026-08-24 (exit codes 2026-09-25)

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

### Mutate: `tree-trunk wt` (implemented)

```bash
tree-trunk wt list [--repo]... [--json]
tree-trunk wt create <repo> <branch> [--json]
tree-trunk wt delete <repo> <branch> [--force] [--json]
tree-trunk wt lock <repo> <branch> [--reason S] [--json]
tree-trunk wt unlock <repo> <branch> [--json]
tree-trunk wt prune <repo> [--dry-run] [--json]
```

Reuses the guarded `internal/git` ops (branch-exists / checked-out-elsewhere /
dirty / locked). **Implemented (2026-08):** flags may appear before or after
positionals (interleaved parser). Default create path is
`~/.worktrees/<repo>/<slug>`. Errors structured:

```json
{ "ok": false, "error": { "code": "branch_checked_out_elsewhere",
  "message": "...", "repo": "tree-trunk", "branch": "feat/x" } }
```

### Planned

- `tree-trunk goto <repo> <branch>` — cd into a (created) worktree.
- `--stdin-action` JSON batch (plan → execute, `--dry-run` first).

### Exit codes (implemented 2026-09-25)

The contract is live: `0` ok, `1` usage/IO, `3` not-found, `4` dirty-blocked,
`5` locked. `query`, `describe` and `completion` use 0/1 only. The mapping is
in `cmd/tree-trunk/wt.go` (`exitCodeFor`) and is published to agents in
`tree-trunk describe` (`exit_codes`) and in the man page.

| Exit | `wt` error codes |
|---|---|
| 0 | success (also `--help`) |
| 1 | `branch_exists`, `branch_checked_out_elsewhere`, `git_error`, usage/IO |
| 3 | `repo_not_found`, `worktree_not_found` |
| 4 | `worktree_dirty` (`--force` overrides) |
| 5 | `worktree_locked` (unlock first; `--force` does not override) |

A coded failure writes the JSON envelope to stdout and **nothing** to stderr,
so `2>/dev/null` never hides the diagnosis.

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

1. ✅ Subcommand skeleton + `query --json` (PR #11).
2. ✅ `wt list` (read-only cross-repo aggregate).
3. ✅ Structured errors on mutate (`wt` ops return `{ok, error:{code,...}}`).
4. ✅ `wt create/delete/lock/unlock/prune` via CLI (PR #13).
5. ✅ Exit-code contract (0/1/3/4/5) — implemented 2026-09-25 (`exitCodeFor`);
   published via `describe`, the man page and `AGENTS.md`.
6. `--jsonl` streaming; daemon/socket (F5) later if scan latency matters.

## Discovery for agents

An agent that hasn't seen tree-trunk before must be able to find its
capabilities. We ship three complementary surfaces:

1. **`tree-trunk --help`** — top-level help now lists every subcommand
   (`query`, `describe`, `completion`) so a shell agent can discover them
   without reading the repo. `query --help` documents its flags.
2. **`tree-trunk describe`** — a machine-readable JSON schema of the CLI:
   `{command, description, version, default, subcommands:[{name, usage, desc,
   flags:[{name, kind, desc}]}]}`. Agents that can't parse prose help can
   introspect this at any installed version.
3. **Man page** (`docs/man/tree-trunk.1`) — classic discovery via
   `man tree-trunk`, installed automatically by homebrew.

Also available for repo-reading agents: `AGENTS.md` at the repo root (if
added) and `docs/design/10-agent-cli.md`.
