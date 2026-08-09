# Project Brief — tree-trunk

> Shared context for all research agents. Read this first, then write your
> findings to `docs/research/` in this repo.

## What we are building

`tree-trunk` — a **TUI (terminal user interface) tool, written in Go**, that lets
users:

1. **List all git repos** on their machine (scan home dir / common dev roots).
2. **Specify projects** (git repos) explicitly via flags.
3. **Manage git worktrees** for those repos — create and delete worktrees.
4. **Inspect git state** of a selected repo against its main active branch:
   - `git log` (history)
   - diffs (working tree, staged, branch-to-branch)
   - `git status` (state)

## Session scope

This session is **planning only**. We are running deep research and will produce
detailed design docs **before any implementation starts**. No code yet.

## The four research tracks (one agent per track)

| Track | Research question | Output file |
|---|---|---|
| 1. Existing tooling | What already exists (worktree managers, repo listers, git TUIs)? What are their gaps? | `docs/research/01-existing-tooling.md` |
| 2. Go suitability | How suitable is Go for this kind of tool? Runtime, concurrency, distribution, ecosystem. | `docs/research/02-go-suitability.md` |
| 3. Go packages | Which Go libraries/projects help: TUI frameworks, git libraries, discovery, config, etc. | `docs/research/03-go-packages.md` |
| 4. Inspiration | Deep dive into lazygit, lazydocker, herdr — architecture, TUI patterns, UX decisions to steal. | `docs/research/04-inspiration-lazygit-lazydocker-herdr.md` |

## Ground rules for research output

- Write **detailed, decision-oriented** findings — we will turn these into a
  design doc.
- Include **specific package names, versions, URLs, and tradeoffs** where relevant.
- Note **risks / open questions** explicitly.
- Keep each output file self-contained (the design phase may read them
  individually).
- Mark anything that is an **opinion vs. a verifiable fact**.
- Where possible, verify claims against real sources (package docs, READMEs,
  GitHub stars/activity, benchmarks). Prefer primary sources.
