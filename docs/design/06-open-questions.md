# Open Questions — tree-trunk

> **Resolved on 2026-08-09:** Q1 (scan `$HOME` only), Q2 (git ≥ 2.38), Q3
> (`~/.worktrees/<repo>/<branch>`, configurable), Q7 (open = print + copy).
> **Closed on 2026-09-25:** Q4, Q5 (decided in practice during M0/M1), Q6
> (resolved at M3) and Q8 (re-ranked after the agent CLI shipped). Nothing is
> open here; new forks belong in a new doc.

## ~~Q1. Exact default scan roots~~ ✅ resolved (2026-08-09)

Scan `$HOME` only, bounded by `discover.max_depth`; `--scan-root` / config to
override. See D4 in `00-decisions.md`.

## ~~Q2. Minimum git version: 2.30 vs 2.38~~ ✅ resolved (2026-08-09)

Require **≥ 2.38**. See D1 in `00-decisions.md`.

## ~~Q3. Worktree path template details~~ ✅ resolved (2026-08-09)

Default `~/.worktrees/<repo>/<branch>`; configurable globally
(`[worktrees] directory`) and per-repo. Branch-slug sanitization: `/` → `-`
(feature/x → feature-x). See D8 in `00-decisions.md`.

## ~~Q7. Open-worktree semantics~~ ✅ resolved (2026-08-09)

Print path + copy to clipboard (D11). Shell spawn / lazygit launch deferred.

---

## Q4. Repo list ordering & grouping

**Settled in M0/M1.** Default sort is alphabetical by name; worktrees are
inline-expandable children in the sidebar (`l` toggles) and also available as
a dedicated tab (`2`). Dirty-first sorting was considered and dropped:
selection is stable across refreshes instead.

## Q5. Untracked worktrees in discovery

**Settled in M0.** A standalone worktree path passed via `--repo` resolves to
its **parent repo** (02-data-model §2.3) — identity is the canonical common
git dir, so a linked worktree never appears as its own entry.

## ~~Q6. Scope of the Diff tab in v1~~ ✅ resolved at M3

Resolved as option (a): the diff tab exposes toggles (`m` cycles
working → staged → vs main; `p` toggles stat/raw). Main-branch auto-detection
via `symbolic-ref refs/remotes/origin/HEAD` is implemented for the vs-main
mode (`MainBranch` detection) with `main`/`master` fallbacks.

## Q8. Post-v1 backlog priority

**Re-ranked 2026-09-25.** F3 (`--json` / machine-readable output) shipped as
`tree-trunk query` + `tree-trunk wt` + `describe`; the agent CLI gap is closed.
Remaining order: custom command DSL (F2), then reflog undo (F4), then
daemon/socket + `--jsonl` (F5), batch worktrees (F1), bootstrap hooks (F6),
Windows QA (F7).

---

*These are tracked in the research docs as well: 01-existing-tooling §8,
02-go-suitability "Open questions", 03-go-packages "Risks & open questions",
04-inspiration §7.*
