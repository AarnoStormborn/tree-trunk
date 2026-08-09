# Open Questions — tree-trunk

> **Resolved on 2026-08-09:** Q1 (scan `$HOME` only), Q2 (git ≥ 2.38), Q3
> (`~/.worktrees/<repo>/<branch>`, configurable), Q7 (open = print + copy).
> Remaining forks below don't block the design or M0.

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

Default sort (name / path / recently modified / dirty-first)? Worktrees
inline-expanded or only via the Worktrees tab? *(02-data-model §3 proposes
alphabetical with expandable children.)* **Not blocking M0–M2.**

## Q5. Untracked worktrees in discovery

A worktree created by another tool (lazygit, CLI) lands under its parent repo
automatically via `git worktree list` — confirmed design. But: should a
standalone worktree path passed via `--repo` be treated as its parent repo or
as its own entry? *(02-data-model §2.3: resolves to parent repo.)* **Not
blocking M0–M2.**

## Q6. Scope of the Diff tab in v1

"Diff against main active branch" (product #4) — default diff mode for a repo
whose main worktree is *not* on the main branch? Options: (a) always offer
both working-tree diff and `main...current` diff as toggles; (b) auto-detect
main branch via `symbolic-ref refs/remotes/origin/HEAD`. *(03 §3.3 proposes
toggles; auto-detection P1.)* **Decide at M3.**

## Q8. Post-v1 backlog priority

From `00-decisions.md` deferred list: F1 batch worktrees, F2 custom command
DSL, F3 `--json`, F4 undo, F7 Windows. Suggest F3 + F2 first (cheap, high
value); confirm at M4 review.

---

*These are tracked in the research docs as well: 01-existing-tooling §8,
02-go-suitability "Open questions", 03-go-packages "Risks & open questions",
04-inspiration §7.*
