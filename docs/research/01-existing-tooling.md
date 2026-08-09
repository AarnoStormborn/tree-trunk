# Research Track 1 — Existing Tooling and Their Gaps

> **Status:** Research complete (planning phase). Feeds directly into the tree-trunk design doc.
> **Scope:** `git worktree` built-ins, worktree managers, repo discovery/listers, git TUIs, and the gap matrix.
> **Conventions:** ✅ = verified against a primary source (official docs, repo README, man page, release notes). ⚠️ = partially verified or inferred from multiple secondary sources. 💬 = opinion / community anecdote, not a hard fact.

---

## 1. Executive summary

The "multi-repo + worktrees + TUI" intersection is **genuinely empty** as of mid-2026. The landscape splits cleanly into four camps, each covering two of the four capabilities tree-trunk wants, never three:

| Capability | Best existing tools | Gap |
|---|---|---|
| List repos across machine | `find` scripts, ghq, gita, mr, gitup, git-rexec, RepoZ, GitKraken Workspaces | All CLI/GUI; none have a TUI; none index *and* show live state in one view |
| Multi-repo worktree create/delete | zootree, Canopy (both tiny/young); grove (bare-clone pattern) | Two young hobby tools; mainstream tooling is single-repo |
| Inspect log/diff/status per repo | lazygit, gitui, tig, magit, GitLens | Excellent per-repo, but scoped to one repo at a time |
| TUI experience | lazygit (~80k★), gitui (22k★), tig | lazygit is the benchmark, but deliberately single-repo |

**The empty cell:** a TUI that combines (a) machine-wide repo discovery, (b) cross-repo worktree creation/deletion, and (c) per-repo log/diff/status inspection. Today a user who wants all four must **compose** `ghq list` + a worktree CLI + lazygit, one repo at a time. That composition, and the "many repos, many worktrees, what's the state of everything?" overview, is what tree-trunk can own.

Key evidence for the gap (verified): lazygit's own worktree UX discussion scoped everything to *a single repo* ([#2803](https://github.com/jesseduffield/lazygit/discussions/2803)); a 2026 practitioner essay titled *"Git Worktrees Are Great. Managing Them Across 3 Repos Is Not."* ([Medium](https://medium.com/@ashmitbbiswas/git-worktrees-are-great-managing-them-across-3-repos-is-not-12262be5ddb0)) describes the pain and a home-grown tool (Canopy) as the workaround; and the only two multi-repo-worktree tools found (zootree, Canopy) are brand-new, low-download hobby projects.

---

## 2. `git worktree` built-in support

**What it is:** `git worktree` (landed Git 2.5, 2015) lets one repository have a *main worktree* plus zero or more *linked worktrees* — separate working directories sharing one object store / ref database. ✅ [Official docs](https://git-scm.com/docs/git-worktree), ✅ [man page](https://man7.org/linux/man-pages/man1/git-worktree.1.html).

### 2.1 What it does (verified from the man page)

| Command | Behavior |
|---|---|
| `git worktree add [-b|-B] <path> [<commit-ish>]` | Create linked worktree; `-b` makes a new branch; `--detach` for detached HEAD; `--orphan`; `--guess-remote` bases new branch on a remote-tracking branch |
| `git worktree list [-v] [--porcelain [-z]]` | List all worktrees (main first). **`--porcelain -z` is the stable machine-parseable format** — the right interface for a programmatic tool |
| `git worktree remove [-f] <worktree>` | Remove; refuses dirty worktrees without `-f`; main worktree cannot be removed |
| `git worktree lock/unlock [--reason]` | Prevent pruning / moving (e.g., worktree on a portable drive) |
| `git worktree move` | Move (submodule-containing worktrees cannot be moved) |
| `git worktree prune [-n] [--expire]` | Clean admin metadata for deleted worktrees; auto-runs per `gc.worktreePruneExpire` |
| `git worktree repair` | Re-link after worktrees/repo were moved manually |

**Architecture facts that matter for tree-trunk (✅ man page, "DETAILS" section):**
- Each linked worktree's git dir lives at `<main>/.git/worktrees/<name>`; inside a linked worktree, `.git` is a **file** containing `gitdir: <path>`. `$GIT_COMMON_DIR` points back to the main repo.
- Refs under `refs/` are shared across worktrees; `HEAD`, `index`, and per-worktree refs (`refs/worktree`, `refs/bisect`, `refs/rewritten`) are not.
- **A branch can be checked out in only one worktree at a time** — the #1 error users hit ("is already checked out at another worktree"); bypass with `--detach` or `-f`.
- `git worktree add <path>` with no branch creates/checks out a branch named after the **basename of the path** — surprising to new users.
- Per-worktree config requires the `extensions.worktreeConfig` extension (off by default); `core.bare`/`core.worktree` are main-worktree-only otherwise.

### 2.2 Known footguns (✅ from man page / ⚠️ widely reported)

1. **Submodules are broken.** Man page BUGS: *"Multiple checkout in general is still experimental, and the support for submodules is incomplete. It is NOT recommended to make multiple checkouts of a superproject."* ✅
2. **Manual moves silently break links.** Move a worktree directory with plain `mv` and the main repo can't find it; requires `git worktree repair`. ✅ Also: **main-worktree moves** require repair in linked trees.
3. **Dirty worktree removal is refused** unless `-f` (which discards changes); `-f` twice for locked. ✅
4. **Orphaned metadata + automatic prune.** Deleting a worktree dir manually leaves admin files that get pruned automatically (default `gc.worktreePruneExpire`, 3 weeks) — usually harmless, but surprising. ✅
5. **Dependency/build dir duplication.** Each worktree gets its own `node_modules/`, `target/`, `.venv/` — large disk cost; tooling exists specifically to mitigate (Worktrunk `wt step copy-ignored`, VS Code `worktreeCopyIgnores`, pnpm hard-linked store). ✅ [gitworktree.org](https://www.gitworktree.org/guides/node-modules), ⚠️ magnitude varies by ecosystem.
6. **IDE double-indexing.** IDEs index each worktree separately (JetBrains YouTrack: *"Git log indexing performed on each worktree (redundant)"*, IDEA-255574). ✅
7. **`git gc` / repack contention** across worktrees sharing one object store. 💬 rarely an issue in practice.
8. **Naive repo discovery misses worktrees.** `find -type d -name .git` finds main worktrees but **not** linked worktrees (their `.git` is a *file*) and not bare repos. `git-rexec` explicitly added `--include-sub-repos` for this. ✅
9. **Windows**: long path limits, case-insensitive FS, junction semantics. ⚠️ less battle-tested than Unix.
10. **UX friction of raw git**: creating one worktree needs the branch name typed ~3× (`git worktree add -b feat ../repo.feat && cd ../repo.feat`), cleanup needs remove + branch delete + remembering paths. ✅ (Worktrunk's docs make this point verbatim.)

**Design implication:** tree-trunk should shell out to `git worktree list --porcelain -z` for source-of-truth state, and mirror ghq's linked-worktree detection (`.git` file → `gitdir:` line, already implemented in ghq's `worktree.go` ✅).

---

## 3. Worktree management tools (the landscape)

### 3.1 CLI worktree managers (single-repo)

| Tool | Lang | What it does | Invocation | Maturity | Gaps |
|---|---|---|---|---|---|
| [gwq](https://github.com/d-kuro/gwq) (d-kuro) | Go | "ghq for worktrees": add/list/get/cd/exec/remove/status/prune/tmux; fuzzy finder; **global mode** (`-g`) scans a configured base dir across repos | CLI + shell integration (cd without subshell) + completions | Active; release notes to v0.1.x (2026); Homebrew | No TUI; global listing is dir-scoped, not machine discovery; no log/diff views; config-file based (`config.toml`, `.gwq.toml`) |
| [grove](https://github.com/captainsafia/grove) (Safia Abdalla) | Rust | Encapsulates the **bare-clone worktree pattern**: `grove init <url>` → bare clone + worktrees; add/list/remove/go/sync/prune/self-update; auto adjective-noun branch names; `.groverc` bootstrap commands (run `npm install` etc. on create) | CLI + shell integration; works from anywhere under the project hierarchy | v1.x; blog post [git-worktrees](https://blog.safia.rocks/2025/09/03/git-worktrees/); Windows/Linux/macOS | Single repo per project; no inspection views; no TUI |
| [Worktrunk](https://worktrunk.dev) (max-sixty) | Rust | AI-agent-focused: `wt switch/list/remove/merge/step/hook/config`; worktrees addressed **by branch name** via path templates; interactive picker with diff/log preview; hooks; LLM commit messages; CI status; `wt switch -x claude -c feat -- 'prompt'`; copy-ignored build caches; `hash_port` template filter | CLI + shell install (`wt config shell install`); Homebrew/Cargo/winget | Active, documented, released (v0.2x era); "free code signing" | Single repo (per invocation); no repo discovery; no TUI (picker is a fuzzy prompt); Rust (not Go); heavy feature surface |
| [twt-cli](https://github.com/aaronhallaert/twt-cli) | Go | Worktrees + **tmux**: every worktree gets a tmux window/session; switch = move to window | CLI | Small (personal tool) | tmux-centric; single repo |
| [twt](https://github.com/todaatsushi/twt) | Shell | Bare-repo + worktree manager (create/list/remove) | CLI | Small | Minimal |
| [wtp](https://github.com/satococoa/wtp) | Go | "Powerful Git worktree CLI" with shell-init navigation + tab completion; TTY-aware (`wtp add` auto-switches dir only on TTY) | CLI + shell integration | Small | Single repo |
| [wt](https://github.com/taecontrol/wt) | Rust | Git worktrees handler; per-worktree env setup ("each environment truly isolated") | CLI | Small | Minimal info |
| [wt](https://github.com/timvw/wt) | Go | "Fast, simple" helper; per-repo `.wt.toml`; CI status via gh/glab | CLI (`go install github.com/timvw/wt@latest`) | Small | Single repo |
| [gwm-cli](https://github.com/kbrdn1/gwm-cli) | Rust | "CLI + TUI" worktree manager; creates worktree, runs your processes | CLI + TUI | 125★, young | Single repo; young |
| [git-worktree-runner](https://github.com/coderabbitai/git-worktree-runner) | Bash | Bash-based per-branch worktree automation for AI tools: copy config, install deps, workspace setup | CLI (git subcommand `git gtr`) | Young; by CodeRabbitAI | Bash; AI-tooling focus; single repo |
| [git-cu](https://gitlab.com/3point2/git-cu) | Rust | "checkout + update" multi-repo CLI; worktree support requested, not yet built (open issue #1) | CLI | Small | Multi-repo is the goal but worktrees are an open TODO |
| [forgit](https://github.com/wfxr/forgit) | Shell+fzf | Interactive git wrappers incl. **`gwt` (worktree picker), `gwa` (add), `gwd` (remove)**; fzf-based | Shell plugin (zsh/bash/fish) + `git forgit` | Mature, active, ~4.5k★ (approx) | Wraps `git worktree` one repo at a time; shell-plugin install burden |
| zsh plugins ([zsh-fzf-git-worktree](https://github.com/banyan/zsh-fzf-git-worktree), [git-worktree-zsh-plugin](https://github.com/trthomps/git-worktree-zsh-plugin), [git-worktrees-zsh-aliases](https://claydon.co/code/git-worktrees-zsh-aliases/)) | Shell | fzf/alias helpers for `git worktree` | zsh plugin | Small/medium | Single repo; shell-config burden; no inspection |

**Pattern worth stealing (✅ READMEs):** nearly every serious single-repo manager (gwq, grove, Worktrunk, wtp) ships **shell integration** so `cd` works in the current shell — a core ergonomics problem tree-trunk must solve or sidestep (a TUI spawning a shell in a chosen worktree is a natural answer).

### 3.2 Multi-repo worktree tools (closest neighbors — and both young)

| Tool | Lang | Model | What it does | Maturity |
|---|---|---|---|---|
| [zootree](https://github.com/weineel/zootree) | Rust | "Multi-repo workspace management tool. Built on Git Worktree + a terminal multiplexer (cmux/Zellij) + LazyGit" | Register repos; `zootree create` a **workspace** = same branch as worktree in N repos; `start` launches organized multiplexer layout (+ optional AI agent); `done` merges & cleans up; copy_files, hooks, templates, per-repo lazygit config | ⚠️ Very new: crates.io v0.0.10, created 2026-05, ~150 downloads |
| [Canopy](https://github.com/ashmitb95/canopy) | Python | Workspace config (`canopy.toml` listing repos) | `canopy worktree <feature>` creates worktree in **every** repo on same branch; alias resolution (ENG-412 → full name); `canopy code` opens IDE workspace; worktree dashboard; `preflight` runs hooks; `canopy done` cleans up across repos; MCP server (27 tools) for AI agents | ⚠️ Personal project from a blog post (2026); 187 tests, MIT |

Both validate the *multi-repo worktree* concept but are effectively single-author, days-old tools. **Neither is a TUI.** This is the clearest whitespace.

### 3.3 Editor / IDE integration

| Tool | Worktree support | Multi-repo? |
|---|---|---|
| [Git Worktree Manager](https://github.com/jackiotyu/git-worktree-manager) (VS Code, jackiotyu) | Create/switch/remove/lock/move/repair/prune from tree view + SCM view; favorites; copy patterns; postCreate/preRemove hooks; requires git ≥ 2.40 | ✅ manages *multiple repositories* (multi-root workspaces + git folders); still editor-bound |
| [VS Code Git Worktrees](https://github.com/alexiszamanidis/vscode-git-worktrees) | Wrapper for add/list/remove with interactive UI; autoPush/autoPull; worktree dir config; copy patterns; multi-workspace prompt; git ≥ 2.34.1 | Multi-workspace aware, one repo per operation |
| GitLens (VS Code, GitKraken) | [Worktrees](https://help.gitkraken.com/gitlens/gl-worktrees/) in the sidebar: create/open/delete; Community plan limited to public/local repos | Per repo (single window); multi-root workspaces only |
| JetBrains IDEs | ⚠️ *Partial*: request [IDEA-143404](https://youtrack.jetbrains.com/projects/IDEA/issues/IDEA-143404) open since 2015; native basic support tracked ([IJPL-204771](https://youtrack.jetbrains.com/projects/IJPL/issues/IJPL-204771)); community plugin [worktree-manager](https://github.com/metastacks/worktree-manager); redundant per-worktree indexing (IDEA-255574) | Project-per-repo model; no cross-repo worktree ops |
| GitHub Desktop | ✅ Worktree support shipped **3.6** (June 2026) — create/list/open/remove per repo | One repo per window; repo list is curated |
| GitKraken Client | Worktrees supported (was the [most-voted feature request](https://feedback.gitkraken.com/suggestions/187158/support-worktrees), 511 votes); **Workspaces** (v8.2, Dec 2021) for multi-repo views | Workspaces group repos, but worktree ops are per-repo |
| Fork (macOS/Win GUI) | ⚠️ Worktrees treated as **separate repos** — "six Fork tabs" problem (see Medium article) | No cross-repo notion |
| GitUp (macOS GUI, 12k★, GPL) | Worktree-aware but buggy (e.g., [issue #2713](https://github.com/git-up/gitup/issues/2713): files left modified after failed alternate-worktree checkout) | Single repo |
| Magit (Emacs) | ✅ `Z` transient: checkout/create/move/delete/status worktrees ([manual](https://docs.magit.vc/magit/Worktree.html)); `magit-insert-worktrees` lists them in status buffer | Per-repo; Emacs multi-project is DIY (project.el + multiple buffers) |

**Takeaway:** editors/GUIs cover single-repo worktree *management* well but are the wrong shape for "scan my whole machine and give me the state of everything" — they're open-applications, not overview dashboards. Note GitHub Desktop's timeline (request 2022 → shipped 2026) shows how long single-repo worktree UX takes even for a big team.

### 3.4 Git TUIs (see also §5)

- **lazygit** — worktrees panel: `n` new, `<space>` switch, `d` remove; `w` in branches/commits/stash panels creates a worktree from the selection. ✅ ([keybindings docs](https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md), [UX discussion #2803](https://github.com/jesseduffield/lazygit/discussions/2803)). Single repo at a time.
- **LazyWorktree** ([chmouel.github.io/lazyworktree](https://chmouel.github.io/lazyworktree/)) — a dedicated worktree TUI (Go): worktree/status/log panes, PR/CI via gh/glab, tmux/zellij sessions per worktree, notes, taskboard, hooks, `.wt` init/terminate scripts. Git ≥ 2.31. **Single repo** (run inside a repo). Young but directly adjacent to tree-trunk's scope minus the multi-repo axis.

---

## 4. Repo discovery / listing tools

### 4.1 The baseline: scripts
The canonical Stack Overflow answer ([2020812](https://stackoverflow.com/questions/2020812/how-can-i-view-all-the-git-repositories-on-my-machine)) is `find ~ -name .git` variants; the performance question ([unix.SE 333862](https://unix.stackexchange.com/questions/333862/...)) pushes for pruning node_modules etc. **Footgun:** `-type d -name .git` misses linked worktrees (`.git` is a file) and bare repos. ✅ (verified against git-rexec's `--include-sub-repos` behavior and ghq's worktree detection code.)

### 4.2 Purpose-built tools

| Tool | Lang | Model | Capabilities | Maturity |
|---|---|---|---|---|
| [ghq](https://github.com/x-motemen/ghq) | Go | **Managed clone tree** under `ghq.root` (default `~/ghq`, layout `host/owner/repo`) | `ghq get/list/root/rm/create/migrate`; `list -p` full paths (pipe to fzf/peco); multi-root; recent versions are **worktree-aware for rm/migrate** (v1.9.4–v1.10.1, ✅ CHANGELOG) | Active (v1.10.1, Apr 2026); the de-facto standard for "list my repos" in Go/Japan-ecosystem workflows |
| [gita](https://github.com/nosarthur/gita) | Python | Registered repo list (`.gita` config) | `gita ll` side-by-side status of all repos; `gita add`; delegate any git/shell command to any repo from anywhere (`gita <repo> <cmd>`); `gita fetch` etc. | Mature-ish; Show HN 2019; active |
| [mr (myrepos)](https://myrepos.branchable.com/) | Perl | `.mrconfig`-file registry (like a Makefile for repos) | `mr update/push/status/diff/commit`, per-repo command overrides, `mr -j5` parallel; works with many VCSes | Old & stable; last release 2018 (1.20180726) |
| [gitup](https://pypi.org/project/gitup/) (earwig) | Python | Discover + fetch/pull **all** repos under a path | `gitup` = `git pull` for every repo; safe skips for dirty repos | PyPI v0.5.2; single-purpose |
| [git-rexec](https://github.com/jamescherti/git-rexec) | Python (script) | Recursive discovery + parallel exec | `git-rexec -p -- git status -s`; `-j N` concurrency; `--include-sub-repos` (worktrees/submodules); `--if-exec` filter; `--print` | Small script (2019–2026) |
| [RepoZ](https://github.com/awaescher/RepoZ) | C# | **Zero-config scan** of drives for repos | Repo hub with Explorer/CLI enhancements (Windows-first, macOS partial); 1.1k★ | Maintenance-ish |
| Google [`repo`](https://gerrit.googlesource.com/git-repo/) | Python | **Manifest-based** multi-repo (Android) | `repo init/sync/forall` — checkout/update hundreds of pinned repos | Very mature (since ~2008), Android-only culture |
| [west](https://docs.zephyrproject.org/latest/guides/west/index.html) | Python | Manifest-based multi-repo (Zephyr RTOS) | `west init/update/forall` | Mature; embedded-focused |

### 4.3 Not actually repo listers (clarification)
- **zoxide** ([ajeetdsouza/zoxide](https://github.com/ajeetdsouza/zoxide)) — frecency-based `cd`; great for *jumping* to repos you've visited, but it does **not** index repos (only history). ✅
- **fzf** — a building block, not a lister; used by forgit/ghq integrations.
- **navi** — a terminal cheatsheet tool, unrelated to repos. ✅ (its name collides with repo tools in casual search)
- **fd** — fast `find`; the engine under most modern `-name .git` scripts.

### 4.4 GUI repo lists
GitKraken Workspaces (v8.2, Dec 2021, ✅), GitHub Desktop's repo sidebar, JetBrains "Recent Projects", VS Code "Git Repos"-style extensions. All are *open-a-repo* pickers, not *overview* tools; none show worktree state across repos.

---

## 5. Git TUIs: multi-repo capability audit

| TUI | Lang | Worktrees | Multi-repo | Log/diff/status | Notes |
|---|---|---|---|---|---|
| [lazygit](https://github.com/jesseduffield/lazygit) (~80k★, Go, MIT) | Go | ✅ worktrees panel (n/space/d), `w` from other panels; bare-repo support (integration tests ✅); ⚠️ bug deleting worktrees with submodules ([#4125](https://github.com/jesseduffield/lazygit/issues/4125)) | ❌ single repo at a time; has a **recent-repos** picker (⚠️ issues [#3813](https://github.com/jesseduffield/lazygit/issues/3813), [#3947](https://github.com/jesseduffield/lazygit/issues/3947)) | ✅ best-in-class | The UX benchmark; actively evolving |
| [gitui](https://github.com/gitui-org/gitui) (22.3k★, Rust, MIT) | Rust | ❌ no worktree panel; linked worktrees only via `GIT_DIR`/`GIT_WORK_TREE` env vars ([PR #1191](https://github.com/gitui-org/gitui/pull/1191), open) | ❌ | ✅ status/diff/log/stash | Fast, but stalled feature velocity vs lazygit |
| [tig](https://github.com/jonas/tig) (C, ncurses, 2006–) | C | ❌ (viewer only; works *inside* a worktree as a normal repo) | ❌ | ✅ log/diff/blame browsing | Mature (v2.6.1, 2026); read-oriented |
| gitk (Tcl/Tk) | Tcl | ❌ | ❌ | ✅ commit graph viewer | Legacy |
| [magit](https://magit.vc) | Emacs Lisp | ✅ `Z` transient + status section | ❌ per-repo buffers | ✅ | Emacs-native users love it; worktree + status in one buffer (⚠️ huonw's 2025 post shows `magit-insert-worktrees`; Reddit reports Emacs tools historically mishandle worktrees) |

**Conclusion:** all mainstream TUIs are *one-repo-at-a-time by design*. Multi-repo workflows in lazygit = run it N times or use the recent-repos picker; no cross-repo worktree operations, no machine-wide overview. This is the single most important confirmation for tree-trunk.

### Related paradigm shifts (context, not competitors)
- **GitButler** ([gitbutlerapp/gitbutler](https://github.com/GitButlerApp/gitbutler)) — GUI + `but` CLI; **virtual branches** (parallel branches in one working dir, no worktrees). Fair Source license. Not a TUI; different mental model.
- **jj (Jujutsu)** ([martinvonz/jj](https://github.com/martinvonz/jj)) — `jj workspace add` gives multiple working copies backed by one repo (✅ docs/working-copy.md); auto-snapshotting removes most reasons to want worktrees. 💬 If jj adoption grows, worktree-tool demand could shrink at the margin — but git remains the dominant toolchain.

---

## 6. GAP ANALYSIS

### 6.1 Capability matrix

Coverage: ● = native/first-class, ◐ = partial/workaround, ○ = none

| Tool | (a) List repos across machine | (b) Multi-repo worktree create/delete | (c) Inspect log/diff/status per repo | (d) TUI |
|---|---|---|---|---|
| `git worktree` alone | ○ | ○ (single repo) | ◐ (CLI only) | ○ |
| find/fd scripts | ● (raw paths only) | ○ | ○ | ○ |
| ghq | ● (managed clones) | ◐ (worktree-aware rm/migrate only) | ○ | ○ |
| gita / mr / gitup / git-rexec | ● (registered or scanned) | ○ | ◐ (status/diff batch only) | ○ |
| RepoZ | ● (zero-conf scan) | ○ | ◐ (status badges) | ○ (GUI) |
| gwq | ◐ (base-dir scan, `-g`) | ◐ (cross-repo *view*, per-repo *create*) | ◐ (status dashboard only) | ○ (fuzzy finder) |
| grove | ◐ (bare-clone projects) | ◐ (per-project) | ○ | ○ |
| Worktrunk | ○ | ◐ (single repo; multi-agent within it) | ◐ (wt list w/ status, picker previews) | ○ (interactive picker) |
| zootree | ◐ (registered repos) | ● (workspaces across repos) | ◐ (status per workspace; delegates to lazygit) | ○ (multiplexer launch, wizard TUI) |
| Canopy | ◐ (workspace config) | ● (same branch across repos) | ◐ (dashboard) | ○ (CLI + MCP) |
| lazygit | ◐ (recent repos only) | ○ (single repo) | ● | ● |
| gitui / tig / gitk | ○ | ○ | ● (per repo) | ● |
| magit | ○ | ○ | ● (per repo) | ◐ (Emacs) |
| VS Code ext / GitLens | ◐ (folder/workspace) | ◐ (per repo, multi-workspace) | ● | ○ (editor UI) |
| JetBrains | ◐ (recent projects) | ◐ (basic, per repo) | ● | ○ |
| GitHub Desktop / GitKraken / Fork / GitUp | ◐ (curated lists) | ◐ (per repo; GitKraken workspaces) | ● | ○ (GUI) |
| GitButler | ◐ | ○ (virtual branches ≠ worktrees) | ● | ○ (GUI) |
| **tree-trunk (target)** | ● | ● | ● | ● |

**No existing tool fills all four cells.**

### 6.2 The specific gaps tree-trunk should target

1. **Machine-wide repo index with live state (a + c).** Nobody gives a TUI where the left pane is "all repos on disk" and the right pane is that repo's log/diff/status. lazygit's per-repo focus is deliberate; the multi-repo overview is the opening.
2. **Cross-repo worktree operations as first-class actions (b).** The brief's core loop — "create/delete worktrees" — exists for single repos everywhere, but "I have 3 repos, make me a feature branch worktree in each, and later clean them all up" exists only in two-week-old hobby tools. Even a v1 that does this *per-repo with a global view* is differentiated; the batch operation is the stretch goal.
3. **Status/diff/log inspection without launching per-repo tools (c + d).** The common multi-repo workflow today is: `cd repo && lazygit`, exit, repeat. A single TUI session across repos is the UX tree-trunk owns.
4. **Zero-install-config discovery.** ghq/mr/gita all require registration; find-scripts require shell hygiene. "Scan home dir + common dev roots, plus explicit `--repo` flags" (per brief) is simpler than all of them — and the `--repo` flag path is a genuine differentiator no existing tool has (closest: lazygit `-p`/`--path` for *one* repo).
5. **TUI ergonomics without shell-integration tax.** Every CLI worktree manager needs shell hooks for `cd`; a TUI that spawns/opens a shell in the chosen worktree (or prints a path) sidesteps that entire class of setup.

### 6.3 Worth building vs. just scripting `git worktree`?

💬 Opinion, grounded in the above:
- **Scripting wins** if the goal is only per-repo create/delete for a handful of repos — aliases/fzf (forgit `gwa`/`gwd`, zsh plugins) cover 80% of that already.
- **A tool is worth building** because the high-value surface is *not* the raw `git worktree` plumbing (git does that fine) but the **aggregation layer**: discovery + parallel state gathering + cross-repo lifecycle + one inspection surface. The repeated failure mode in the ecosystem (Fork's "six tabs", Canopy's author, lazygit's single-repo scope, GitHub Desktop taking 4 years) is that nobody has shipped the *overview*.
- The **main risk tree-trunk faces**: lazygit could grow a repo-switcher-first-class multi-repo mode (its recent-repos issues show demand), and zootree/Canopy could mature. The counterweights: tree-trunk's explicit `--repo` flag workflow, machine-wide discovery, and worktree-centric (not branch-centric) design are not on any established roadmap.

---

## 7. Design-relevant facts for the design doc

- **State source of truth:** `git worktree list --porcelain -z` (stable, NUL-safe) — ✅ man page.
- **Linked-worktree detection:** `.git` file with `gitdir:` header; resolved relative to the worktree dir (see ghq's `worktree.go`, which implements exactly this). ✅
- **Discovery pitfalls:** `-type d` misses linked worktrees; bare repos have no worktree at all; skip `node_modules`/`.git`/vendor dirs for speed (unix.SE answers). ✅
- **`git status` is the slow op on many repos** → parallelize (mr `-j5`, gitup, git-rexec `-j` all do; a TUI needs incremental refresh). ✅
- **Removal safety:** `git worktree remove` refuses dirty trees; tools layer `--force` confirmation (grove, git-worktree-manager, lazygit all confirm). ✅
- **Naming/paths:** every serious tool computes worktree paths from templates (gwq `naming.template`, Worktrunk path templates, grove adjective-noun). ✅ — strongly consider a default layout under a single root (`.worktrees/…`).
- **Bare-clone pattern** (grove, twt, Worktrunk docs) keeps `main` pristine and makes worktrees the *only* working copy — a coherent opinionated default tree-trunk could offer, but it conflicts with "scan existing repos" (existing repos aren't bare). ⚠️ Design decision for the team.

---

## 8. Open questions (for the design phase / project owner)

1. **Multi-repo worktree scope:** does tree-trunk v1 create a worktree *per selected repo* (brief-literal), or also support "same branch across N repos in one action" (zootree/Canopy model)? This is the single biggest scope fork.
2. **Discovery policy:** scan `$HOME` + common roots only, or support explicit roots config? Depth limits? Skip symlinks/bare repos? (ghq registers clones; tree-trunk proposes scanning — tradeoff: speed vs zero-config.)
3. **Shell-out vs library:** shell out to `git` (lazygit model — battle-tested, keeps git versions compatible) vs a Go git library (go-git — cross-platform, but worktree support maturity must be verified; see track 3). ✅ lazygit shells out; gitui uses libgit2 bindings.
4. **Bare-repo setups:** should tree-trunk detect/operate on bare-clone+worktree layouts (grove-style) or only "normal" repos with a main worktree?
5. **TUI framework & architecture:** track 3/4 will inform (tview/Bubble Tea etc.; lazygit patterns). Not answered here.
6. **Windows support** is a stated platform gap in most tools (gwq shell integration excludes PowerShell; Worktrunk needs `git-wt` alias) — in or out for v1?
7. **Performance bar:** how many repos should instant-open? (gita/mr batch tools handle hundreds; a TUI with per-repo status polling needs incremental updates.)
8. **Worktree naming/layout policy:** user-configurable templates (gwq/Worktrunk) vs fixed convention?
9. **Inspection depth:** does "inspect log/diff/status" mean lazygit-class views per repo, or summary + open-in-lazygit delegation? (zootree delegates to lazygit; lazyworktree re-implements. Delegation is a legitimate v1.)
10. **Does "list repos" include worktrees as first-class list entries**, or are they only shown under their parent repo? (Affects the data model: repo ≠ directory when worktrees exist.)

---

## 9. Sources (primary where possible)

**Git core:** [git-worktree man page](https://man7.org/linux/man-pages/man1/git-worktree.1.html) · [git-scm docs](https://git-scm.com/docs/git-worktree) · [worktreeConfig docs](https://git-scm.com/docs/git-config#Documentation/git-config.txt-extensionsworktreeConfig)
**Worktree managers:** [gwq](https://github.com/d-kuro/gwq) · [grove](https://github.com/captainsafia/grove) + [Safia's blog](https://blog.safia.rocks/2025/09/03/git-worktrees/) · [Worktrunk](https://worktrunk.dev) · [LazyWorktree](https://chmouel.github.io/lazyworktree/) · [forgit](https://github.com/wfxr/forgit) · [twt-cli](https://github.com/aaronhallaert/twt-cli) · [wtp](https://github.com/satococoa/wtp) · [gwm-cli](https://github.com/kbrdn1/gwm-cli) · [git-worktree-runner](https://github.com/coderabbitai/git-worktree-runner) · [git-cu](https://gitlab.com/3point2/git-cu)
**Multi-repo worktrees:** [zootree](https://github.com/weineel/zootree) (+[crates.io](https://crates.io/crates/zootree)) · [Canopy / "Managing them across 3 repos is not"](https://medium.com/@ashmitbbiswas/git-worktrees-are-great-managing-them-across-3-repos-is-not-12262be5ddb0)
**Repo listers:** [ghq](https://github.com/x-motemen/ghq) (v1.10.1 CHANGELOG) · [gita](https://github.com/nosarthur/gita) · [myrepos](https://myrepos.branchable.com/) · [gitup (PyPI)](https://pypi.org/project/gitup/) · [git-rexec](https://github.com/jamescherti/git-rexec) · [RepoZ](https://github.com/awaescher/RepoZ) · [repo](https://gerrit.googlesource.com/git-repo/) · [west](https://docs.zephyrproject.org/latest/guides/west/index.html) · [SO 2020812](https://stackoverflow.com/questions/2020812/how-can-i-view-all-the-git-repositories-on-my-machine) · [unix.SE 333862](https://unix.stackexchange.com/questions/333862/how-to-find-all-git-repositories-within-given-folders-fast)
**TUIs:** [lazygit](https://github.com/jesseduffield/lazygit) (+[Worktrees keybindings](https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md), [UX #2803](https://github.com/jesseduffield/lazygit/discussions/2803), [submodule bug #4125](https://github.com/jesseduffield/lazygit/issues/4125)) · [gitui](https://github.com/gitui-org/gitui) (+[PR #1191](https://github.com/gitui-org/gitui/pull/1191)) · [tig](https://github.com/jonas/tig) · [magit Worktree manual](https://docs.magit.vc/magit/Worktree.html) · [GitButler](https://github.com/GitButlerApp/gitbutler) · [jj working-copy docs](https://github.com/martinvonz/jj/blob/main/docs/working-copy.md)
**Editors/GUIs:** [git-worktree-manager](https://github.com/jackiotyu/git-worktree-manager) · [vscode-git-worktrees](https://github.com/alexiszamanidis/vscode-git-worktrees) · [GitLens worktrees](https://help.gitkraken.com/gitlens/gl-worktrees/) · [GitHub Desktop 3.6 changelog](https://github.blog/changelog/2026-06-26-github-desktop-3-6-worktrees-and-deeper-copilot-integration/) · [JetBrains IDEA-143404](https://youtrack.jetbrains.com/projects/IDEA/issues/IDEA-143404) · [GitKraken Workspaces v8.2](https://www.gitkraken.com/blog/gitkraken-client-v8-2) · [GitUp #2713](https://github.com/git-up/gitup/issues/2713)
**Workflow/footguns:** [matklad, "How I Use Git Worktrees"](https://matklad.github.io/2024/07/25/git-worktrees.html) · [gitworktree.org guides](https://www.gitworktree.org/guides/node-modules) · [gitcheatsheet.dev disk-space](https://gitcheatsheet.dev/docs/advanced/worktrees/disk-space-management/)
