# Design Review — tree-trunk (pre-M0)

> **Status: RESOLVED 2026-08-09.** All findings accepted and addressed — see
> [§9 Resolutions](#9-resolutions). Review pass 2 (`08-reviewer-drafts.md`)
> drafted the config schema (now `09-config.md`) and the reallocated
> keybinding registry (now `04-tui-layout.md` §3).

> **Reviewer:** design reviewer pass, 2026-08-09.
> **Method:** all seven design docs read in full; research tracks 1–4 skimmed for
> claim-checking. Git behaviors verified against the live binary (git 2.39.5,
> Apple Git-154) in sandbox repos; git version history verified against the git
> release notes (Documentation/RelNotes, git@master clone). Every finding below
> cites doc + section. Claims marked **[verified]** were reproduced locally.

---

## 1. Executive summary

**Verdict: the design is coherent, well-grounded, and *conditionally ready* for
M0 — not rubber-stampable as-is.** The architecture (state store + bounded
worker pool + typed git layer, D9/D1) is a faithful, well-sourced port of
patterns proven in lazygit/lazydocker/herdr, and the research is unusually
strong. But the docs contain one locked-decision contradiction (the git-version
floor), two concrete command-level bugs that would produce wrong behavior if
implemented literally (repo-identity keys, refresh fingerprint), a
keybinding-conflict cluster in the TUI spec, and several spec gaps that will
surface during M0–M1 (config schema, scan policy, refresh policy, per-worktree
state). None of these is architectural — they are fixable in hours, not weeks —
but three of them would land as *silent wrongness* (duplicate repo rows, stale
status, missed repos) rather than loud failures, which is exactly the class of
bug that survives into release.

**Bottom line:** do the "Top 5 changes before M0" (§6) as a revision pass, then
start M0. Do not start M0 with the 2.30 version text or the un-canonicalized
identity key as written.

---

## 2. Findings by severity

### 2.1 BLOCKER

**B1 — Git version floor contradicts the locked decision (2.30 vs 2.38).**
*Files:* `01-architecture.md` §1 system diagram ("min ≥ 2.30, D1"), §5 step 2
("Verify git on PATH + version ≥ 2.30"); `05-implementation-plan.md` M0
("version check (≥ 2.30)") and M5 ("Integration test suite on git 2.30 +
latest"). *Contradicts:* `00-decisions.md` D1 ("require ≥ 2.38", user decision
2026-08-09) and `03-git-layer.md` §3.2/§4.3/§8 (2.38, twice).
*Problem:* D1/Q2 is a locked user decision; the 2.30 text would gate in users
on git 2.30–2.37 — and `git worktree list --porcelain -z` (the design's own
"source of truth" parser input) only landed in **git 2.36** **[verified via
RelNotes/2.36.0: "…introducing NUL terminated output format with -z"]**.
An implementer following the architecture doc would silently ship a broken
worktree parser for a whole version range the design claims to support. Also,
2.36 fixed path/lock-reason c-quoting in porcelain output, so pre-2.36 output
breaks the "raw bytes" parsing assumption too.
*Fix:* sweep every "2.30" to "2.38" (architecture §1/§5, plan M0/M5); make the
startup gate read a single `MinGitVersion` constant; CI matrix = 2.38 + latest
(as `03-git-layer.md` §8 already says).

### 2.2 MAJOR

**M1 — Repo identity is not canonicalized; two discovery paths can produce
different store keys.**
*Files:* `02-data-model.md` §1.1 ("ID = common git dir path"), §2.3; `01-architecture.md` §2
(store keyed "common git dir"); `03-git-layer.md` §3.1 (`rev-parse --git-common-dir`).
*Problem:* **[verified]** `git rev-parse --git-common-dir` returns `.git`
(relative) when run from the main worktree but the absolute common-dir path
when run from a linked worktree; macOS also diverges on symlinked paths
(`/tmp` → `/private/tmp`, which git itself resolved in its porcelain output).
A repo discovered via its `.git` directory vs. via a linked worktree's `.git`
file (whose `gitdir:` points at `<common>/worktrees/<name>`, needing a suffix
strip the docs never specify) can therefore produce different ID strings →
duplicate repo rows, broken folding, and `--repo <linked-worktree>` not merging
into the parent `Repo` as §2.3 promises.
*Fix:* specify a canonicalization step — `filepath.Abs` + `filepath.EvalSymlinks`
on the common-dir string before keying — and the exact `.git`-file → common-dir
mapping (strip the `worktrees/<name>` suffix, or run `rev-parse --git-common-dir`
from the worktree). Add a fixture test asserting main-vs-linked discovery of
the same repo yield identical IDs.

**M2 — Refresh dedup would skip *working-tree* changes.**
*Files:* `01-architecture.md` §3.3 ("If a poll sees an unchanged fingerprint,
skip the expensive re-read"), §7 budget ("fingerprint-skipped"); `03-git-layer.md` §3.1.
*Problem:* the fingerprint is refs+HEAD hashes. **[verified]** staging/editing
files and adding untracked files change **no ref**. If the whole per-repo
refresh (status + branch/ahead-behind) is gated on the fingerprint — as §3.3
reads — a dirty repo would never appear dirty in the overview until a manual
`R`. The overview (the product's reason to exist) silently goes stale.
*Fix:* split the reads. The fingerprint gates only ref-dependent loads
(branch, ahead/behind, log cache); `git status --porcelain` runs on **every**
poll (it is the cheap 30 ms op, per the doc's own §7 numbers). State this
explicitly in §3.3.

**M3 — The fingerprint command as written excludes HEAD.**
*Files:* `03-git-layer.md` §3.1 (Fingerprint row: `git for-each-ref refs/heads
refs/remotes --format=%(objectname) HEAD`); `01-architecture.md` §3.3 and D9
say the fingerprint is "refs/ + HEAD".
*Problem:* **[verified]** `HEAD` as a trailing positional argument to
`git for-each-ref` is a *ref pattern* that matches nothing (a ref named "HEAD"
doesn't exist under `refs/`). The command hashes only `refs/heads` +
`refs/remotes`. A detached-HEAD move (`git checkout <sha>`) — or an unborn HEAD
— changes HEAD with no ref change, so M2's dedup would skip the refresh. The
architecture doc *says* "`git rev-parse` set"; the command catalog doesn't do it.
*Fix:* `git for-each-ref --format=%(objectname) refs/heads refs/remotes`
**plus** a separate `git rev-parse --verify HEAD`, joined in the fingerprint.

**M4 — Keybinding conflicts in `04-tui-layout.md`.**
*Files:* §3.2 (repo list), §3.1 (global), §3.3 (inspection tabs).
*Problem:*
1. **`l` bound twice in the same context:** §3.2 row "`→` / `l` — Expand repo
   row" **and** row "`l` / `L` — Lock / unlock worktree". One of these will
   silently win, breaking the other — a direct violation of the doc's own
   "single source of truth" registry (§3).
2. **Global `+`/`_` fullscreen (§3.1) vs tab-context `+`/`-` expand/collapse
   and paging (§3.3).** `+` does two things whenever a tab has focus.
3. Same verb, different keys across contexts: unlock is `l`/`L` in the repo
   list but `u` in the Worktrees tab; delete is `D` in the repo list but `d` in
   the Worktrees tab.
*Fix:* one allocation pass against the registry before M2 (e.g., keep
`l` = expand, move lock/unlock to `L`/`u` everywhere; keep `+`/`_` global and
rebind tab expand/paging to something free like `x`/`X`; unify delete on `d`).

**M5 — Dirty linked worktrees are invisible in the model and UI.**
*Files:* `02-data-model.md` §1.1 ("Status … aggregated from **main worktree**
status"), §1.2 `Worktree` (no status field), §3 (children rows); `04-tui-layout.md`
§5.4 (Worktrees table columns: path, branch, head, locked, prunable, current).
*Problem:* the machine-wide overview — the product's differentiator — shows
dirty state only for the *main* worktree. A linked worktree with modified or
untracked files (the #1 worktree-cleanup scenario) appears clean everywhere:
not in the repo row, not in its child row, not in the Worktrees tab.
*Fix:* add a status/dirty flag to `Worktree` (lazily populated: run
`git status --porcelain` per worktree of the *focused* repo, not all repos),
render an icon on child rows and a status column in the Worktrees tab.

**M6 — Prune has no keybinding or UI surface.**
*Files:* `05-implementation-plan.md` M2 ("prune dry-run"), `03-git-layer.md`
§3.2 (Prune), vs. `04-tui-layout.md` §3 (entire key registry — no prune).
*Problem:* prune is in M2's exit criteria but unreachable from the UI; it dies
as dead code. `git worktree prune` is also the *only* remedy for the prunable
state the design models (§1.2 `Prunable`), so the model field feeds nothing.
*Fix:* bind prune (e.g., `P` in Worktrees tab / worktree context) with dry-run
preview + confirm; or explicitly cut it from M2.

**M7 — Config schema is referenced but never specified.**
*Files:* `01-architecture.md` §2 (`config.go`: "scan_roots, ignore,
worktrees.directory, theme"), §3.1 (`workers`); `02-data-model.md` §2.2
(`discover.max_depth`, `discover.ignore`, `discover.include_bare`); `00-decisions.md`
D8 (`[worktrees] directory`, per-repo override).
*Problem:* M0 includes config loading, but no doc defines the actual TOML
schema — key names, types, defaults, the per-repo override syntax D8 promises,
or `~`-expansion rules (D8's own default is `~/.worktrees/...`). Implementers
will invent a schema in M0 and churn it in M1–M2.
*Fix:* add one reference table (key · type · default · example) to the design
before M0; rule: leading `~` expands via `os.UserHomeDir()`.

**M8 — Scan policy hides real, common repos (hidden dirs + symlinks).**
*Files:* `02-data-model.md` §2.2 ("skip symlinks (do not follow)" and "hidden
dirs except `.config`-style user dirs").
*Problem:* two silent-miss classes. (a) **Dotfiles repos**: `~/.emacs.d`,
`~/.vim`, `~/.dotfiles`, `~/.oh-my-zsh` are hidden-dir git repos — all skipped,
yet they are often the most important repos on a machine. (b) **Symlinked dev
roots**: `~/dev -> /Volumes/...`, iCloud/Dropbox dirs — skipped entirely,
defeating "list all repos on the machine" (D4's stated differentiator).
Meanwhile the `.config` carve-out can descend into enormous app dirs
(`.config/Code`, `google-chrome`) that hold no repos — slow, not just wrong.
*Fix:* explicit policy — curated heavy-dir ignore list *including* under
`.config`; hidden dirs skipped **unless** they are themselves repos or contain
one at shallow depth; optional `discover.follow_symlinks` with loop protection
(visited-set / depth cap). Add fixture tests for dotfiles and symlinked roots.

**M9 — Per-file diff commands missing from the git catalog.**
*Files:* `04-tui-layout.md` §5.1 ("`enter` on a file → diff view scoped to
that file") vs `03-git-layer.md` §3.3 (only repo-level diffs).
*Problem:* a shipped M3 flow has no backing command spec: `git diff -- <path>`
and `git diff --cached -- <path>`, plus the status-code-aware cases (`??`
untracked has no diff; rename records need old-path handling). Without this,
the status→diff flow is unbuildable as specified.
*Fix:* add the per-file variants to §3.3 with the untracked/rename rules.

**M10 — Refresh trigger model unspecified.**
*Files:* `01-architecture.md` §4–5 (workers "emit deltas", "background
refreshes"), `04-tui-layout.md` §3.1 (`R` manual), D9 ("background workers").
*Problem:* the docs never say what *triggers* refreshes in v1 — selection
change? a timer? only `R`? fs-watch is explicitly P1, so with no poller the
overview goes stale silently while the TUI is open, which M2/M5 make worse.
*Fix:* state v1 policy explicitly (recommended: on-selection + `R` +
optional per-repo poll on a configurable cadence, with focused-repo priority —
the research's own P0-12 idea); or declare "no periodic refresh in v1" and add
a `stale` indicator so staleness is visible.

### 2.3 MINOR

- **m1 — Worktree porcelain parser spec is wrong/incomplete in two places.**
  `03-git-layer.md` §4.3. **[verified]** (a) a **`detached` key** appears in the
  record instead of `branch` when a worktree is detached — omitted from the
  field list; (b) records terminate with **`\0\0`** (NUL-terminated fields plus
  an extra record-separator NUL) — the diagram shows a single trailing `\0`;
  (c) the "main record has no branch line" uncertainty resolves to **it does**
  (`branch refs/heads/main` is present for the main worktree when on a branch);
  (d) `HEAD` is the full 40-hex hash while `02-data-model.md` §1.2 says "short
  commit hash". *Fix:* rewrite §4.3 as a byte-level spec incl. `detached`,
  `locked`/`prunable` with empty reason (`locked\0` + record sep), and the
  double-NUL; shorten hashes at render time.
- **m2 — Log format yields double-NUL terminators.** `03-git-layer.md` §4.2.
  **[verified]** `--format=...%s%x00 -z` emits `…subject\0\0` per record
  (trailing `%x00` + the `-z` record terminator); "split on NUL" then yields an
  empty trailing field per record. *Fix:* drop the trailing `%x00` (or spec the
  double-NUL explicitly in §4.2).
- **m3 — M0 "print a JSON/dumb repo list" vs deferred `--json`.** `05-implementation-plan.md`
  M0 goal vs `00-decisions.md` F3 (deferred) and D7's flag list (no JSON flag).
  Define M0's headless mode as a plain-text list, or pull `--json` into scope —
  as written, M0 partially re-opens a deferred decision.
- **m4 — `Worktree.IsCurrent` definition is ambiguous.** `02-data-model.md`
  §1.2: "the one the main repo considers current" — git has no such notion
  (every worktree has its own HEAD). *Fix:* define as "the worktree whose path
  matches the cwd tree-trunk was launched from" (lazygit's semantics) or drop
  the field if nothing consumes it.
- **m5 — The `Branch` model has no UI consumer.** `02-data-model.md` §1.3,
  `03-git-layer.md` §3.1 ("lazy, only for focused repo") vs `04-tui-layout.md`
  (no branches view anywhere). The per-branch ahead/behind + `IsCheckedOut`
  load is specified with no view using it (only the create-form completions
  plausibly do). Clarify scope or cut the branch load from v1.
- **m6 — Remote-only branches can't be created.** `03-git-layer.md` §3.2,
  `04-tui-layout.md` §4 (completions from "existing branches" — local refs).
  `git worktree add <path> <branch>` on an `origin/`-only branch fails with
  "invalid reference" **[verified]**. Suggest `--guess-remote` for the
  not-found-locally case.
- **m7 — Branch-slug collisions unhandled.** D8/Q3: `/` → `-` only.
  `feat/a` and `feat-a` collide on the same path; no rules for empty names,
  reserved names (`.`/`..`), length, unicode, or collision resolution. Add a
  slug spec + collision suffix rule to D8.
- **m8 — Clipboard mechanism unspecified.** D11: "copy to clipboard" — no
  library/mechanism (pbcopy vs xclip vs `atotto/clipboard` vs
  `charmbracelet/x/clipboard`) and no degradation policy for headless Linux
  (WSL/SSH), which is a first-class platform (D6). *Fix:* pin a lib; degrade
  to print-only + toast on failure.
- **m9 — `--scan-root` add-vs-replace ambiguity.** D4 ("override/add"),
  `02-data-model.md` §2.2 ("replace/augment it"), `01-architecture.md` §5
  ("merge"). Pin semantics (recommend: `--scan-root` **replaces** default
  roots; config `scan_roots` adds).
- **m10 — Bare-repo detection method during the walk unspecified.**
  `02-data-model.md` §2.1 (bare row) — checking *every* directory for
  HEAD+objects+config is expensive; the default-off flag suggests an
  opportunistic check, but no mechanism is given. *Fix:* only probe dirs that
  plausibly match (has `HEAD` + `objects/`), and only when
  `discover.include_bare` is set.
- **m11 — Small doc errors:** `01-architecture.md` §2 comment "(D2 in
  02-data-model.md)" should read "see 02-data-model.md" (D2 is bubbletea);
  `03-git-layer.md` §6 error table omits `NotARepo` and `CommandFailed` UI
  behaviors (add rows so the UI mapping is total).
- **m12 — `Prunable` reason → `IsPathMissing` mapping unspecified.**
  `02-data-model.md` §1.2. **[verified]** prunable reason text is
  "gitdir file points to non-existent location" — define the string match so
  `IsPathMissing` is populated.
- **m13 — Status-summary glyph mapping implicit.** `02-data-model.md` §3
  (`~3 +1` = "modified +added") doesn't say how `Staged/Unstaged/Untracked/
  Conflicts` map to the row glyphs. Define the rendering rule.

---

## 3. Cross-doc consistency check

| Decision | Status | Notes / contradictions |
|---|---|---|
| D1 Go + shell-out | ⚠️ | **Version floor inconsistent**: 2.38 in D1/03 §3.2/§4.3/§8 vs 2.30 in 01 §1/§5 and 05 M0/M5 (B1). Shell-out itself consistent everywhere. |
| D2 bubbletea v1.3.x | ✅ | Consistent (01 §2, 04 header). Note: research 02 §8 recommended v2; D2 overrides with documented rationale — fine, but keep the v2-migration note alive at M5. |
| D3 per-repo worktrees | ✅ | Consistent (03 §3.2, 05 §5 "batch stays out"). |
| D4 discovery | ⚠️ | Consistent on flags; scan *policy* (hidden/symlink) under-specified (M8); `--scan-root` semantics ambiguous (m9). |
| D5 native views | ✅ | Consistent; delegation deferred consistently (04 §3.2 note). |
| D6 platforms | ✅ | Consistent. |
| D7 flags + TOML | ⚠️ | Consistent; M0's "JSON/dumb list" leaks into deferred F3 (m3); no config schema (M7). |
| D8 worktree paths | ⚠️ | Template consistent (02/03/04 examples agree); slug rules incomplete (m7); `~` expansion unspecified (M7). |
| D9 state store | ⚠️ | Store/events consistent across 00/01/02/05; but fingerprint command contradicts D9's "refs/ + HEAD" (M3) and dedup granularity is wrong (M2). |
| D10 no daemon | ✅ | Consistent (01 §8 door-open note). |
| D11 open = print+copy | ✅ | Consistent (04 §3.2, §3.3). |

**Other cross-doc deltas found:**
- Keybinding registry vs itself: `l` double-bound, `+` global-vs-tab, verb-key
  drift (`l`/`u`, `D`/`d`) — M4. The registry is the *only* spec of keys and it
  contradicts its own tables.
- `03 §8` says CI on "git 2.38 (oldest supported)" while `05 M5` says "2.30 +
  latest" — same B1 family.
- Entity model: `02 §1.2 Worktree.Head` "short" vs porcelain full hash (m1);
  `IsCurrent` undefined semantics (m4); `Repo.GitDir` = common dir vs
  `Worktree.GitDir` = per-worktree git dir — consistent, but the *derivation*
  of `Worktree.GitDir` (common dir + admin-dir name) is never specified (m1
  companion).
- M2's prune (05) has no keybinding (04) — M6.
- Status→file diff flow (04 §5.1) has no command (03 §3.3) — M9.
- No version mismatches found in D2/D3/D5/D6/D10/D11 — those hold.

---

## 4. Missing pieces (needed for M0–M2, not currently specified)

| When | Missing | Where it bites |
|---|---|---|
| M0 | Config schema reference (keys/types/defaults/per-repo override syntax) | M7; config loading is an M0 deliverable |
| M0 | Repo-identity canonicalization algorithm + test fixture | M1; duplicate/folded rows from day one |
| M0 | Headless output mode definition (plain list vs JSON) | m3; M0 exit criteria unbuildable as written |
| M0 | Version-gate constant + startup message wording (2.38) | B1 |
| M0 | `~`-expansion rule for all path configs | M7/m9 |
| M1 | Refresh trigger/cadence policy; dedup granularity split | M2/M10 |
| M1 | Per-worktree dirty state (model + collection strategy) | M5 |
| M1 | Detached-HEAD status header parse case (`## HEAD (no branch)`) | status parser |
| M2 | Prune keybinding + confirm flow | M6 |
| M2 | Remote-branch creation path (`--guess-remote`) | m6 |
| M2 | Branch-slug spec (charset, collisions, reserved names) | m7 |
| M2 | Run-dir policy for worktree commands (which dir each command runs in) | worktree ops correctness |
| M2 | Complete error-code → UI table (incl. `NotARepo`, `CommandFailed`) | m11 |
| M2 | Keyboard-driven E2E harness decision (teatest vs custom PTY driver) | 05 §2; experimental dep needs a fallback |
| M3 | Per-file diff commands (incl. untracked/rename cases) | M9 |
| M3 | Log search (`n`/`N`) implementation note — bubbles has no search component | 04 §3.1 |
| Global | Clipboard lib + headless degradation | m8 |
| Global | Windows best-effort: `git.exe` PATH resolution test note | 03 §2 (present, but untested per D6) |

---

## 5. Feasibility risks (vs the 4–8 week estimate)

1. **Repo-list→detail navigation is the unproven core UX and the plan knows it**
   (05 §4: "prototype … early"; 04 §2: "riskiest UX"). If the spike fails, the
   context-stack/layout work is thrown away mid-M1/M2. Mitigation exists
   (spike at M1/M2 boundary) but the estimate has no buffer line for a
   redesign. **Recommend:** timebox the spike in M1 and treat its output as a
   hard gate for M2.
2. **M3 (L) is the single largest item and sits at the end of the critical
   path.** The lazygit `tasks` port (single-flight streaming, lazy line reads,
   throttle, cancellation) is genuinely fiddly, and M3's scope is only *loosened*
   by one deferral (hunk folding). Q6 (diff-mode decision) is deferred *to* M3,
   which can expand scope mid-milestone. **Recommend:** decide Q6 at M2 review;
   hold M3 to stat/raw toggle only.
3. **TUI test tooling is the weakest dependency in the plan:** teatest is
   explicitly experimental (05 §2), and subprocess-driven PTY tests are
   notoriously flaky in CI. The 05 §2 mitigation (pure functions + golden
   smoke tests only) is sound — but budget M5 for harness debugging.
4. **Git-version matrix CI needs real multi-version git** (2.38 + latest) —
   provisioning (brew/apt/containers) is a setup cost the plan doesn't price.
   The 2.30/2.38 confusion (B1) makes this riskier than it needs to be.
5. **Scan/refresh on real homes will surprise:** the 100-repo budget (01 §7)
   assumes skip rules work on the reviewer's actual `$HOME`; M8's hidden/symlink
   policy and the `Library`-style skips will miss or over-walk real trees.
   Measure against a real home dir in M0 (the exit criteria already say "scanning
   100+ repos" — make it a *real* home, not a synthetic fixture).
6. **Estimate overall:** M0 S–M + M1 M + M2 M–L + M3 L + M4 M + M5 M ≈
   "L (4–8 weeks)" is plausible for one experienced developer **only if** the
   B1/M1–M4 fixes land pre-M0 and M3 scope is held. The 4-week end of the range
   is optimistic; plan and communicate 6–8.

---

## 6. Top 5 changes before M0

1. **Sweep the git-version floor to 2.38 everywhere** (01 §1/§5, 05 M0/M5) and
   gate startup on a single constant (B1). Two-line text change, prevents a
   whole class of "works on my machine, broken on 2.30–2.37" bugs.
2. **Specify repo-identity canonicalization** — `Abs` + `EvalSymlinks` on the
   common-dir key, explicit `.git`-file → common-dir mapping, and a
   main-vs-linked discovery test asserting identical IDs (M1). Without this,
   the store's central invariant (one row per repo) fails on day one.
3. **Fix the refresh design:** fingerprint = for-each-ref + explicit
   `rev-parse HEAD` (M3), and fingerprint gates *ref reads only* — status
   re-reads every poll (M2). Also declare the v1 refresh trigger policy (M10).
4. **Resolve the keybinding conflicts** in 04 §3 (`l` double-bound; global `+`
   vs tab `+`; `l`/`u`, `D`/`d` drift) with a single allocation pass against
   the registry (M4).
5. **Write the config schema and the scan policy** — a TOML reference table
   (M7) and a concrete hidden-dir/symlink rule that finds dotfiles repos and
   symlinked dev roots without descending into `.config/Code`-style dirs (M8).

*Runner-up (fold into #3's milestone, not M0): per-worktree dirty state (M5) —
without it the overview can't show the state of the very worktrees it exists
to manage.*

---

## 9. Resolutions (2026-08-09)

| Finding | Resolution |
|---|---|
| **B1** version floor 2.30 vs 2.38 | Swept to **2.38** in `01` §1/§5, `05` M0/M5; single `MinGitVersion` constant; CI matrix 2.38 + latest |
| **M1** identity not canonicalized | `02` §1.1: `Abs`+`EvalSymlinks` key; `.git`-file → common-dir mapping; main-vs-linked same-ID fixture in `05` M0 |
| **M2** dedup skips working-tree changes | `01` §3.3: fingerprint gates **ref reads only**; `git status` re-reads every poll |
| **M3** fingerprint excludes HEAD | `01` §3.3 + `03` §3.1: explicit `git rev-parse --verify HEAD` joined with for-each-ref |
| **M4** keybinding conflicts | `04` §3 replaced wholesale (Draft B): `l`=expand, `L`=lock/unlock, `d`=delete, `+`/`_` global-only, paging `pgup/pgdn`+`ctrl+u/ctrl+d`, `x`/`X` expand; conflict-verified |
| **M5** dirty linked worktrees invisible | `02` §1.6 `Worktree.Dirty` (lazy, focused-repo scope); icons in child rows + Worktrees tab |
| **M6** prune no UI surface | `04` §3.2/§3.3: `P` with dry-run preview + confirm; in `05` M2 |
| **M7** config schema missing | New `09-config.md` (Draft A): full key table, precedence rules, default ignore list, example file |
| **M8** scan policy hides dotfiles/symlinks | `02` §2.2 + `09` §2.3/§3: hidden `peek` policy, curated heavy-dir ignores, opt-in loop-protected symlink following |
| **M9** per-file diff commands missing | `03` §3.3: per-file variants incl. untracked placeholder + rename old-path |
| **M10** refresh triggers unspecified | `01` §3.4: on-selection + `R` + optional `refresh.poll_interval_ms`; stale indicator |
| m1 porcelain spec | `03` §4.3 byte-level spec: `detached`, `\0\0` separator, `branch` on main, empty locked/prunable reasons, 40-hex HEAD |
| m2 log double-NUL | `03` §4.2: drop trailing `%x00` |
| m3 M0 JSON vs deferred F3 | `05` M0 → `--list` plain-text paths; `--json` stays F3 |
| m4 `IsCurrent` undefined | `02` §1.2: defined as "path matches launch cwd" (lazygit semantics) |
| m6 remote-only branches | `03` §3.2: `--guess-remote` on not-found-locally |
| m7 slug collisions | `00` D8: slug spec (charset, collision suffix `-2`, reserved names, 80-char cap) |
| m8 clipboard unspecified | `00` D11: `charmbracelet/x/clipboard` + print-only fallback; `09` `[clipboard] enabled` |
| m9 `--scan-root` semantics | `09` §2.2: flags REPLACE, config ADDS, `scan_home` opt-out |
| m10 bare detection cost | `02` §2.1: probe only when `include_bare` + repo-shaped dir |
| m11 doc errors / error table | `01` §2 comment fixed; `03` §6 table completed (`NotARepo`, `CommandFailed`) |
| m12 prunable reason mapping | `02` §1.2 + `03` §4.3: exact string match on "gitdir file points to non-existent location" |
| m13 status glyph mapping | `02` §3: conflicts→staged→unstaged→untracked order, nonzero segments only |
| Feasibility #1 nav spike gate | `05` §4: spike output is a hard gate for M2 |
| Feasibility #2 M3 scope | `05` M3 + `04` §5.3: stat/raw only; Q6 decided at M2 review |
| Feasibility #4 git matrix | `05` M5: 2.38 + latest |
