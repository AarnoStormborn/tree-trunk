# Research Track 3 — Go Packages & Projects That Help

> **Project:** `tree-trunk` — a Go TUI to list git repos and manage git worktrees
> (repo list + detail panels + keybindings + help overlay).
> **Agent:** Research 3 of 4. Scope: which Go libraries/projects help build this.
>
> All versions are **the latest tagged release** verified against the Go module
> proxy (`proxy.golang.org/<module>/@latest`) and star counts against the GitHub
> API on the research date unless noted. Activity "pushed" dates are the repo's
> last push (GitHub). "Fact" = verified against a primary source (pkg.go.dev,
> GitHub, official docs). "Opinion" = my recommendation/rationale, flagging it.

---

## TL;DR — recommended stack (opinion)

| Concern | Pick | Why |
|---|---|---|
| TUI framework | **charmbracelet/bubbletea v1** + **bubbles v1** + **lipgloss** | Component model matches repo-list + detail-panel + help-overlay; largest active ecosystem; official components for list/table/viewport/help. |
| Alternative to TUI | **rivo/tview** | If we prefer a ready-made widget/grid library over Elm-style architecture. |
| Git interaction | **Shell out to the `git` binary** (primary); keep **go-git v5** only for metadata reads if needed | Worktrees in go-git are experimental (`x/`), slow, and incomplete (no remove/prune/lock). Correctness + performance win by shelling out. |
| Repo discovery | stdlib `filepath.WalkDir` + `git rev-parse`/`.git` parsing | No magic needed; handle `.git` **file** (worktree/submodule) vs **dir** (repo). |
| Config | **stdlib `flag` + TOML/YAML stdlib-file parse** (or **koanf** if we want layers) | For a single-purpose TUI, viper/cobra are overkill. |
| TUI testing | **charmbracelet/teatest** (experimental) + lazygit-style subprocess integration tests | teatest golden-file tests + real-git sandbox integration. |

Tool that already does ~80% of the idea: **lazygit** (shells out to git, gocui-based). Research track 4 covers its architecture; this file covers the library-level primitives.

---

## 1. TUI frameworks

### 1.1 charmbracelet/bubbletea — RECOMMENDED
- **Verdict:** Fact — **v1.3.10** (latest stable), 44,259★, active (pushed 2026-08-07), MIT.
- Elm-architecture (Model / Update / View via `tea.Cmd`/`tea.Msg`). Explicit `tea.KeyMsg`, `tea.WindowSizeMsg`.
- **Why it fits:** The project's bill of materials is *literal* Bubbles components:
  - `bubbles/list` → the repo list pane
  - `bubbles/table` or `bubbles/list` → worktree / branch detail
  - `bubbles/viewport` → scrollable `git log` / diff / status output
  - `bubbles/help` → the help overlay / keybinding bar
  - `bubbles/spinner`, `bubbles/statusline`/`statusbar`, `bubbles/progress` → scanning & status chrome
  - `bubbles/key` → centralized keybinding definitions (also used by `help`)
- **Component maturity (fact):** bubbles **v1.0.0** (8,768★), lipgloss **v1.1.0** (11,680★). A **v2 alpha** of bubbletea + lipgloss v2 is in active development (charm.land/blog/v2). For a production tool today, pin bubbletea **v1.3.x** (stable); treat v2 as future migration.
- **Opinion:** bubbletea is the best *architecture fit* for multi-pane apps because each pane is a composable model and keybindings are data (easy to render a help overlay from the same key registry).

### 1.2 rivo/tview — strong alternative
- **Verdict:** Fact — **v0.42.0**, 14,029★, active (pushed 2026-08-07), Apache-2.0. Built on tcell.
- Widget + layout library: `Grid`, `Flex`, `Pages`, `List`, `Table`, `TextView`, `TreeView`, modals/forms.
- **Why it's a fit:** multi-pane (sidebar + main + bottom status) is its bread-and-butter; focus management + global keybindings built in.
- **Opinion/tradeoff vs bubbletea:** tview is *immediate* — faster to a working multi-pane screen with focus switching, but styling is more imperative and can feel less composable for custom panes. bubbletea gives finer control and a cleaner extension story (Bubbles) at the cost of more boilerplate (you assemble panes yourself). Either is defensible; I'd pick bubbletea for long-term composability and the help-overlay component, tview if we want the quickest widget-rich skeleton.

### 1.3 jroimartin/gocui — opinion: avoid the original, note the fork
- **Verdict (fact):** original `jroimartin/gocui` unmaintained (last push 2025-05-01, 10,598★, MIT); community fork `awesome-gocui/gocui` **v1.1.0**, 385★, active (pushed 2026-02-09) "many improvements over the original."
- gocui is **minimalist/low-level** (views + keybindings; no widget library). **lazygit** is built on it (`pkg/gui` wraps gocui) — this is the strongest real-world proof it can power a large git TUI.
- **Opinion:** gocui is viable (lazygit proves it) but low-level; for tree-trunk we'd rebuild what bubbles/tview already give. Only choose if we deliberately want lazygit-style architecture parity.

### 1.4 gdamore/tcell/v2 — low-level (foundation, not a choice)
- **Verdict (fact):** **v2.13.10**, repo `gdamore/tcell` 5,213★, active (pushed 2026-08-02), Apache-2.0.
- Cell/event-level terminal library (the "assembly language" of TUIs). tview is built on it; bubbletea uses its own input + lipgloss rendering and doesn't require tcell (it uses termenv + a terminfo/ANSI path).
- **Opinion:** don't use tcell directly; it only matters as the foundation for tview or if we drop to custom rendering.

### 1.5 muesli/termenv — color/env detection
- **Verdict (fact):** **v0.16.0**, 2,018★, active (pushed 2025-11-21), MIT. Detects terminal color profile (`termenv.EnvColorProfile`), respects `NO_COLOR`/`CLICOLOR_FORCE`, handles truecolor/256/ANSI.
- **Opinion:** lipgloss already uses termenv under the hood; no need to depend on it directly unless we need raw profile detection for logging or non-TUI output.

**Framework decision matrix (opinion):**
| Need | bubbletea | tview |
|---|---|---|
| Repo list + detail panes | list + viewport/table | List + TextView/Tree |
| Keybindings + help overlay | `bubbles/key` + `bubbles/help` (built-in) | `SetInputCapture` (hand-rolled help) |
| Multi-pane layout management | manual (Flex/margins via lipgloss) | `Grid`/`Flex` built-in |
| Component ecosystem / docs | large (Bubbles, huh, Glamour) | smaller but self-contained |
| Styling control | lipgloss (excellent) | tview color tags / styles |

---

## 2. Git interaction — the critical decision

This is the highest-stakes choice. Tradeoffs below.

### 2.1 Option A: go-git/go-git (pure Go) — fact + strong caveats
- **Verdict (fact):** v5 latest **v5.19.2** (7,653★, active, pushed 2026-08-08, Apache-2.0); **v6 still pre-release** (`v6.0.0-alpha.5`; no stable tag, migration guide marked "subject to change").
- **Strengths:** pure Go (no CGo, no dependency on a system `git`), embedded/ports fine, `config` pkg for reading config files, `billy` filesystem abstraction, reads `.git` config, detects refs/objects.
- **Worktree support — THE key finding (fact, from go-git official docs & pkg.go.dev):**
  - Linked-worktree support lives in the **experimental** package `github.com/go-git/go-git/v6/x/plumbing/worktree` (only in v6, not v5).
  - It is incomplete: **only `Add` is fully supported**; **lock, move, prune are not supported**; not all `git worktree` flags/subcommands implemented.
  - Requires a `storage.Storer` implementing `WorktreeStorer` — **only `storage/filesystem` satisfies this** (no in-memory/badger backends).
  - Performance issue: creating a worktree / checking out decompresses every file from the object store even when already present (open issue go-git#1956).
  - Historical reliability issues (e.g., `Worktree.Status()` failures, go-git#299).
- **Opinion:** go-git is a poor fit for *full-featured worktree management* today. Because `tree-trunk`'s core value prop is worktrees, and go-git can't even `remove`/`prune`/`lock` worktrees, building on go-git for that path is a non-starter. Its niche is **read-only metadata** (list branches, log, object parsing) where paying pure-Go is worth it.

### 2.2 Option B: shell out to the system `git` binary — RECOMMENDED (opinion)
- **What:** run `git` via `os/exec` (`git worktree add/remove/list`, `git status`, `git log`, `git diff`, `git rev-parse`).
- **Why (opinion):** full, correct, always-up-to-date feature set — every worktree verb, plus hooks, credentials, submodules, LFS, signed commits. Git is guaranteed present because the user is a git workflow tool. Performance: native C git object handling beats go-git (which decompresses everything).
- **Proof this pattern works (fact):** **lazygit** (81,162★) shells out to `git` commands rather than embedding go-git; it documents binding arbitrary shell/git commands and has an open issue about shell env for git hooks. Its whole integration-test strategy runs real git in sandboxed repos. This is the proven path for a serious git TUI.
- **Costs (fact/opinion):** requires `git` on PATH (fine for this tool); output parsing (use `--porcelain`, `-z`/`--null` for robust parsing); concurrency safety (run commands serially or be careful with lock files); no pure-Go embedded distribution.
- **Worktree detection nuance (fact):** a linked-worktree checkout has a **`.git` *file*** containing `gitdir: <path>` (not a directory). Shelling out `git rev-parse --git-common-dir` / `git worktree list` handles this correctly for us.

### 2.3 Option C: git2go (libgit2 bindings) — opinion: avoid
- **Verdict (fact):** `github.com/libgit2/git2go/v34` **v34.0.0** (2,006★, last push **2024-03-04**, i.e. mostly dormant), LGPL/CPL (mixed licensing), **CGo dependency** on native libgit2.
- **Tradeoffs (fact):** module version must align to a specific libgit2 release; CGo complicates cross-compilation/distribution; licensing (GPL for the C lib portions) conflicts with easy embedding.
- Older Stack Overflow answers recommended it, but it's dated. **Opinion:** reject — CGo + licensing + dormancy outweigh benefits; libgit2 also historically lags some worktree edge cases.

**Decision (opinion):** **Shell out to `git`** for all mutation + state (status/diff/log/worktrees). Optionally use **go-git v5** only for pure-metadata reads if we ever want to render without the binary — but honestly `git rev-parse`/`git for-each-ref` cover that too. Keep the git layer behind an interface (`GitRunner`) so we can mock/substitute.

---

## 3. Filesystem / repo discovery

### 3.1 stdlib `filepath.WalkDir` — RECOMMENDED
- **Verdict (fact):** stdlib `io/fs` + `filepath.WalkDir` is the idiomatic, zero-dependency walker (Go 1.16+, so any current toolchain). Respect `SkipDir` to cut into `.git`, node_modules, etc.
- **Key correctness rule (fact, from Rust `marver` scan.rs + general git semantics):** a `.git` **directory** ⇒ normal repo; a `.git` **file** starting with `gitdir: <path>` ⇒ **linked worktree or submodule** — walk must NOT treat these as plain dirs to descend into, and SHOULD be flagged so we can offer to open the linked worktree. This is the single most likely bug in repo-scanning code.
- **Detecting bare repos (fact):** a bare repo has `.git` as the repo dir with `core.bare=true` and no worktree; detect via `git rev-parse --is-bare-repository` or config. Decide policy (opinion) — likely show bare repos but treat "no working tree" specially.
- **Performance (opinion):** `WalkDir` is fine for scanning a home dir at startup; run it in a goroutine + `bubbles/spinner`, and make scan path list + ignore patterns configurable to bound cost. Avoid `filepath.Walk` (reflection-heavy, slower; deprecated guidance favors WalkDir).

### 3.2 fsnotify/fsnotify — NOT needed for v1 (opinion)
- **Verdict (fact):** **v1.10.1** (active, MIT), cross-platform inotify/FSEvents/ReadDirectoryChangesW.
- **Opinion:** live re-scan of *repos on disk* is out of scope; refresh on demand (manual key / re-run scan) is simpler and correct. Keep fsnotify as a possible future enhancement (e.g., watching the git dir for state change) — not a v1 dependency.

### 3.3 go-git `filesystem`/`plumbing` readers — optional
- go-git reads `.git/config`, refs, and objects (see §2). **Opinion:** only use if we want pure-Go metadata rendering. For discovery + worktree handling, prefer shelling out (`git worktree list --porcelain`, `git for-each-ref`) because it's correct and cheap.
- go-billy (`github.com/go-git/go-billy/v6`) is a generic abstraction; only relevant if we embed go-git.

### 3.4 Dedicated repo-scanning libraries
- **Verdict (fact):** No mature, widely-adopted Go "scan my machine for repos" library emerged from research. Small examples exist (`keegancsmith/counsel-repo`, `osteele/gh-repo-tools`), but none is a standard. **Opinion:** this is a ~50-line `WalkDir` + git-detection function; don't add a dependency.

---

## 4. Configuration & CLI flags

### 4.1 CLI flags
- **spf13/pflag** — **v1.0.10** (stdlib-`flag` compatible, POSIX/GNU style: `--flag`, `--flag=val`, short flags).
- **spf13/cobra** — **v1.10.2**, 44,419★. Full command framework (subcommands, help, completion).
- **stdlib `flag`** — zero-dependency, but no `--flag=value` long/short split, no subcommands, awkward `--help`/bool handling.
- **Opinion:** `tree-trunk` is a *single-purpose TUI*, not a multi-command CLI. `tree-trunk --repo PATH ... --scan-root DIR` is enough. **Use stdlib `flag` or pflag**; save **cobra** for if we later split into `list`/`worktree`/`config` subcommands. Cobra pulls in pflag anyway; adding cobra only if we want its help/completion out of the box.

### 4.2 Config files (TOML/YAML)
- **Verdict (fact — latest versions):**
  - `spf13/viper` **v1.21.0** (30,423★) — heavyweight: env, remote, multiple formats, watch.
  - `knadh/koanf/v2` **v2.3.6** (4,152★) — lightweight, pluggable providers/parsers (env, file, TOML, YAML, JSON), zero global state, actively maintained. Community/team sentiment (lumberjack issue, blog posts) favors **koanf over viper** for clean empty-value handling and lighter weight — but that's anecdotal, not consensus (fact).
  - Parsers: `gopkg.in/yaml.v3` **v3.0.1** (canonical YAML); `github.com/BurntSushi/toml` (canonical TOML); `github.com/pelletier/go-toml/v2`.
- **Opinion:** For `tree-trunk`, config is small (scan roots, ignore patterns, per-repo overrides). Simplest robust option: **stdlib `flag` for CLI + a single TOML/YAML file parsed with `BurntSushi/toml` or `yaml.v3` directly**. Add **koanf** only if we later want layered env+file+defaults without hand-rolling merge logic. **Avoid viper** — its weight and global state don't pay off here (opinion). (TUI config is also often just defaulted + `~/.config/tree-trunk/config.toml`.)

---

## 5. TUI helpers & testing

### 5.1 Bubbles components (reuse, don't rebuild)
- **Fact:** `bubbles v1.0.0` includes: **list** (filterable, paged, styled), **table** (sortable/selectable rows), **viewport** (scrollable pager + mouse wheel), **key** + **help** (keybinding registry + bottom help bar), **spinner**, **statusline/statusbar**, **progress**, **textinput/textarea**, **paginator**, **filepicker**.
- **Huh (`charmbracelet/huh/v2` v2.0.3, 7,087★):** forms/survey library; standalone or embedded in bubbletea; has an accessibility mode. **Opinion:** useful later for "create worktree" forms (pick branch, name, path); note community reports of rendering glitches when embedding huh inside larger custom bubbletea layouts (fact: open discussion) — validate before committing. For v1, plain keybindings + a textinput may suffice.
- **Glamour (`charmbracelet/glamour`, 3,635★):** renders Markdown with styles — but we don't render markdown here; skip. Could style `git log`/`diff` output with lipgloss directly.

### 5.2 Testing
- **teatest (`charmbracelet/x` subpackage; module path is `charmbracelet/x/exp/teatest`, not `charmbracelet/teatest`):** the official Bubble Tea integration-test harness — drives a `tea.Program` in a headless terminal, supports **golden-file** output tests (charm.land/blog/teatest). **Explicitly experimental, no compat guarantee** (fact).
- **Functional-core pattern (opinion, echoing community posts):** keep a pure `core`/`Model` layer whose `Update`/business logic can be unit-tested without a terminal; use teatest only for golden-render smoke tests. Strive to test state transitions, not pixels.
- **Git mocking (opinion):** the lazygit-recommended pattern (jesseduffield.com "Integration Tests") is to run the **real TUI as a separate subprocess** against **sandboxed real git repos**, scripting real `git` operations — rather than mocking the git layer. Mocking a `GitRunner` interface is fine for fast unit tests, but the authoritative end-to-end check shells out to real git in temp repos. (Fact: bubbletea discussions #1528 acknowledge no single comprehensive official CLI/TUI testing package; teatest + custom harness is the practical path.)

### 5.3 Layout/styling
- **lipgloss v1.1.0** (per-component style, margins, borders, width-aware wrapping, tables/inheritance) — the styling engine for bubbletea (fact). **Rate limit/render batching:** for heavy `git log`/diff rendering, render to strings and feed `viewport` (not raw `tea.Println`).

---

## 6. Prior art in Go (and one Rust) — what already does parts of this

> Research track 4 deep-dives lazygit/lazydocker/herdr architecture; here I list the landscape + URLs.

### 6.1 The big git TUIs
- **lazygit** (`jesseduffield/lazygit`, 81,162★, Go, gocui-based, shells out to git) — the canonical feature-rich git TUI. Gaps it doesn't specifically nail: *multi-repo* listing + *worktree creation/management* as the primary UX (it's single-repo, branch-centric). **This is the strongest prior art for architecture** (tracks 2 & 4).
- **gitui** (`gitui-org/gitui`, 22,352★, **Rust**/Ratatui) — fast, single-repo git TUI; performance reference, not Go.

### 6.2 Worktree-specific tools
- **wtp** (`satococoa/wtp`, **Go**, 608★, active pushed 2026-03-30) — Git worktree CLI (not a TUI) with auto-setup, branch tracking, smart navigation, shell completion, installable via `go install`/Homebrew. **Closest Go prior art for the *worktree model***.
- **grove** (`captainsafia/grove`, Go, 84★) — CLI worktree workflow around a bare clone (init/add/go/remove/list/sync).
- **LazyWorktree** (`chmouel/lazyworktree`, Go, 279★, active pushed 2026-08-03) — keyboard-first **TUI** managing worktrees/branches/PRs/CI; positions worktrees as the default workflow. Direct TUI prior art.
- **git-worktree-runner** (`coderabbitai/git-worktree-runner`, 1,732★, Bash-based) — repo-scoped worktrees, config-over-flags, editor/AI integrations.
- **gtr-wtm etc.** — various small/0–1★ Go worktree TUIs (e.g., FredrikMWold/git-worktree-tui, late-2025, 1★). **Fact:** negligible maturity; not reference-grade.

### 6.3 Repo-list / project tools
- **git-fuzzy** (`bigH/git-fuzzy`, 2,434★) — interactive git with fzf, **Shell not Go** (correct attribution; earlier "joshrosso" attribution was wrong).
- Small Go repo-scanners (`keegancsmith/counsel-repo`, `osteele/gh-repo-tools`) — example-only, not libraries.
- **Opinion:** no mature Go project cleanly combines *multi-repo listing + worktree management TUI* — that's the whitespace `tree-trunk` occupies. Closest combined inspirations are lazygit (TUI patterns) + wtp (worktree semantics) + LazyWorktree (worktree-first TUI).

**Curated lists (facts):** `rothgar/awesome-tuis`, `awesome-go.com/advanced-console-uis/` (lists gocui), `awesometui.com`.

---

## Risks & open questions

1. **go-git worktree support is experimental and incomplete (only `Add`; no remove/prune/lock).** *Open:* do we ever want pure-Go, or is shelling out to `git worktree` acceptable? (My rec: shell out. The design doc should confirm we won't require an embedded-git port.)
2. **Git-binary dependency.** *Open:* acceptable for a dev tool? (lazygit says yes; but confirm team's distribution expectations — `go install`, brew tap, Docker none.)
3. **Bubble Tea v2 alpha** vs stable **v1** — pin v1 for now; *open:* how much we care about forward-compat to v2 (lipgloss v2, bubbletea v2).
4. **gocui original vs awesome-gocui fork** — we should NOT use gocui (bubbletea chosen), but if we ever revisit lazygit-parity, use the fork.
5. **Repo-scan performance/scope** — default scan roots, ignore rules, handling bare repos + linked worktrees (`.git` file) correctly. *Open:* should a linked worktree appear as its own entry or fold into the parent repo?
6. **teatest is experimental** — no stability guarantee; need a fallback rendering-test strategy; also confirm import path (`charmbracelet/x/exp/teatest`).
7. **huh embedding glitches** in custom layouts — decide v1 form approach (huh vs hand-rolled textinput).
8. Versions verified on the research date; **re-verify latest versions** at design/implementation time (esp. go-git v6 alpha cadence and bubbletea v2).

---

*Written by Research Agent 3 — Go packages/projects track. Facts are primary-source verified (pkg.go.dev, GitHub API/stars, go-git official docs, official READMEs); recommendations are marked `(opinion)`.*
