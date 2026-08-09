# tree-trunk — Docs

`tree-trunk` is a **TUI tool written in Go** for listing git repos across your
machine and managing their **git worktrees** — create, delete, and inspect
(log, diff, status) against each repo's main active branch.

> **Status: PLANNING.** This session produced research + design docs only.
> No implementation has started. Next step: M0 of
> [`design/05-implementation-plan.md`](design/05-implementation-plan.md).

## Document map

| Path | Contents |
|---|---|
| [`research/00-project-brief.md`](research/00-project-brief.md) | The brief all research agents worked from |
| [`research/01-existing-tooling.md`](research/01-existing-tooling.md) | Existing tooling & the gap analysis (track 1) |
| [`research/02-go-suitability.md`](research/02-go-suitability.md) | Why Go fits (track 2) |
| [`research/03-go-packages.md`](research/03-go-packages.md) | Go packages: bubbletea, git strategy, testing (track 3) |
| [`research/04-inspiration-lazygit-lazydocker-herdr.md`](research/04-inspiration-lazygit-lazydocker-herdr.md) | lazygit/lazydocker/herdr deep-dive + "steal this" (track 4) |
| [`design/00-decisions.md`](design/00-decisions.md) | Decision record (D1–D10) + deferred backlog |
| [`design/01-architecture.md`](design/01-architecture.md) | Layers, package layout, concurrency, state store |
| [`design/02-data-model.md`](design/02-data-model.md) | Repo/Worktree/Branch/Status entities + discovery |
| [`design/03-git-layer.md`](design/03-git-layer.md) | Git interaction: runner, parsers, worktree flows, safety |
| [`design/04-tui-layout.md`](design/04-tui-layout.md) | Layout, keybindings, views, forms, theming |
| [`design/05-implementation-plan.md`](design/05-implementation-plan.md) | Milestones M0–M5, testing strategy, effort |
| [`design/06-open-questions.md`](design/06-open-questions.md) | Small forks to resolve before/at implementation (Q1/Q2/Q3/Q7 resolved) |
| [`design/07-design-review.md`](design/07-design-review.md) | Independent design review: 1 blocker, 10 majors, 13 minors — all resolved (§9) |
| [`design/08-reviewer-drafts.md`](design/08-reviewer-drafts.md) | Reviewer drafts: config schema + keybinding reallocation (merged into 09 / 04) |
| [`design/09-config.md`](design/09-config.md) | Full `config.toml` schema, precedence rules, default ignore list |

## Product brief (for reference)

- List all git repos on the machine (configurable scan roots)
- Specify projects/repos explicitly via flags (`--repo`)
- Create & delete worktrees per repo (two-step safety, never delete branches)
- Inspect log, diff, status against the repo's main active branch

## Headline design decisions (full rationale in `design/00-decisions.md`)

- **Go**, shelling out to the system `git` binary (go-git's worktree support
  is experimental/incomplete)
- **bubbletea v1.3.x** + bubbles + lipgloss (Elm-style, matches the state
  store + events design)
- **Zero-config discovery**: scan `$HOME` + common dev roots + `--repo` flags
- **Per-repo worktree management** with a machine-wide overview (batch
  cross-repo worktrees deferred)
- **Native log/diff/status views**, lazy-loaded and streamed (lazygit
  `tasks` pattern)
- **macOS + Linux first**; Windows best-effort

*Research was produced by four parallel pi agents running in a herdr tab
(`tree-trunk-research`, workspace `w1`, 2026-08-09).*
