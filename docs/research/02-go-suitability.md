# Track 2 — Suitability of Go for `tree-trunk`

> **Research question:** How suitable is Go for a TUI + git-worktree-management
> CLI? Runtime, concurrency, process spawning, TUI ecosystem, distribution,
> and an honest comparison against Rust / Python / TypeScript / C++.
>
> **Status:** research output for design phase. Facts are cited; anything
> labeled **OPINION** is an interpretation by this agent, not a source claim.
> Coordinates with Track 3 (Go packages — `03-go-packages.md`) on library-level
> specifics; this file focuses on ecosystem-level maturity and language fit.

---

## TL;DR

Go is an **excellent fit** for this specific tool, and the two most successful
existing projects in this exact niche prove it out in production:

| Claim | Evidence |
|---|---|
| Go TUI ecosystem is mature enough for a git TUI | lazygit (~79k stars), lazydocker, k9s, gh-style tools, Charm's bubbletea (~38k stars, v2 released Feb 2026) |
| Shelling out to the real `git` binary is the proven architecture | lazygit's git layer runs `git` subprocesses via `os/exec` — source verified |
| Single static binaries + trivial cross-compilation | Go toolchain default; lazygit ships macOS/Linux/Windows builds; `go install`, Homebrew, Scoop, AUR |
| Concurrency maps perfectly to the scan-N-repos workload | goroutines + `errgroup` + worker pools are the canonical Go pattern; hundreds of thousands of goroutines are supported |
| Startup + memory are a non-issue for a TUI | Go binaries start in ms; a TUI rendering text needs tens of MB at most |

The main **genuine risks** are: (1) go-git (pure-Go git library) is *not* a
drop-in replacement for the git binary — perf and correctness gaps documented
below — so the tool should shell out to `git`; (2) goroutine/process leaks in a
long-running TUI need deliberate cancellation discipline; (3) bubbletea v2
(Feb 2026) is a major breaking release — pin a version and read the upgrade
guide before starting.

---

## 1. Language / runtime fit

### 1.1 The workload profile

`tree-trunk` is I/O-bound, not CPU-bound:

- **Scan:** walk filesystem trees looking for `.git` (dirs) or `.git` files
  (linked worktrees) — mostly `stat()`/`readdir()` syscalls.
- **Inspect:** run `git status` / `git log` / `git diff` on N repos in parallel
  — mostly waiting on child processes and disk.
- **Render:** redraw a text UI on keypress/refresh — string assembly in memory.

This is precisely the workload Go's design targets (network/disk-bound fan-out
with goroutines), per the consensus of the sources surveyed
([futurion.blog Rust-vs-Go-for-CLI](https://futurion.blog/rust-vs-go-for-cli-tools-picking-a-runtime-without-starting-a-tribal-war/),
[Yalantis Go-vs-Rust](https://yalantis.com/blog/rust-vs-go-comparison)).

**FACT:** Go compiles to a single static binary per platform; the standard
toolchain cross-compiles to macOS/Linux/Windows from any host with
`GOOS`/`GOARCH` + `CGO_ENABLED=0`
([OCaml forum thread on why Go dominates CLI/TUI work](https://discuss.ocaml.org/t/why-is-there-no-tradition-of-cli-and-tui-apps/14628),
[Ecostack cgo cross-compilation writeup](https://ecostack.dev/posts/go-and-cgo-cross-compilation/)).

**FACT:** Go binaries have negligible startup cost. Alex Ellis cites Go
loading near-instantly compared to Node (which can take 1.5–3.0 s to boot on a
Raspberry Pi) in [5 keys to create a killer CLI in Go](https://blog.alexellis.io/5-keys-to-a-killer-go-cli).
A TUI must feel instant on every invocation; Go's startup is effectively free.

**FACT:** Go's GC is concurrent and low-latency; Go's GC pauses are
sub-millisecond in typical workloads ([Twitch engineering: Go's march to
low-latency GC](https://blog.twitch.tv/en/2016/07/05/gos-march-to-low-latency-gc-a6fa96f06eb7/)).
A TUI rendering a text frame doesn't allocate heavily, so GC latency is a
non-issue. (The famous `#14812` GC-latency-spike bug
[golang/go#14812](https://github.com/golang/go/issues/14812) is from 2016 and
closed.)

**OPINION:** Memory footprint (~10–40 MB RSS for a Go TUI app) is irrelevant
for a developer tool run interactively in a terminal; only Rust/zig would do
dramatically better and nobody would notice.

**FACT (primary, measured on this machine — Apple Silicon, Apple Git 2.39.5):**

```
$ git --version                 → real 0.01s (pure process spawn + binary load)
$ git status --porcelain (warm) → real 0.03s
$ git status --porcelain (cold) → real 0.32s
```

So each exec of `git` costs ~10 ms of overhead plus the command itself. This
bounds the "shell out to git" strategy: serial scans of 100 repos ≈ 3–30 s;
concurrent scans (section 2) bring this to a fraction of a second wall-time in
the warm case. **Design implication:** never run git operations serially when
the UI can show results incrementally.

### 1.2 Static binary / no runtime dependency

**FACT:** Go produces statically-linked binaries with no runtime dependency
(the Go runtime is embedded). No interpreter, no JVM, no DLL hell on Windows
([discuss.ocaml.org](https://discuss.ocaml.org/t/why-is-there-no-tradition-of-cli-and-tui-apps/14628) —
Go's frictionless cross-compilation is cited as the main reason CLIs get
written in Go).

**FACT:** CGO must be avoided (or handled carefully) to keep cross-compilation
trivial. Pure-Go dependencies (bubbletea, cobra, etc.) compile with
`CGO_ENABLED=0`. If the team adds a cgo dependency later (e.g. a native sqlite
driver), cross-compilation needs a cross toolchain
([Go forum: "Binary was compiled with CGO_ENABLED=0"](https://forum.golangbridge.org/t/binary-was-compiled-with-cgo-enabled-0/11098),
[Ecostack](https://ecostack.dev/posts/go-and-cgo-cross-compilation/)).
**For this project: no cgo needed** — everything in scope is pure Go.

---

## 2. Concurrency: goroutines, channels, worker pools

### 2.1 Mapping to the workload

The scan phase has two distinct concurrency opportunities:

1. **Filesystem walk** — walking `$HOME` and dev roots for `.git` entries.
   - **FACT:** Go can run hundreds of thousands of goroutines; the runtime
     multiplexes them onto cores
     ([StackOverflow: concurrent filesystem scanning](https://stackoverflow.com/questions/44255814/concurrent-filesystem-scanning)).
   - The canonical pattern is `errgroup`/`WaitGroup` + bounded semaphore to
     avoid goroutine explosion on deep trees
     ([O'Reilly: fast parallel file searches with sync.ErrGroup](https://www.oreilly.com/content/run-strikingly-fast-parallel-file-searches-in-go-with-sync-errgroup/)).
   - **OPINION:** the filesystem-walk phase is usually *disk-bound*, not
     CPU-bound; unbounded parallelism just thrashes the page cache. A bounded
     worker pool (e.g. `runtime.NumCPU()`–4× that) walking directory subtrees
     is the right shape. Track 3 has the package specifics
     (`03-go-packages.md`).

2. **Per-repo git status** — for each discovered repo, run `git status` /
   `git worktree list`, parse output, feed results to the UI.
   - Pattern: **worker pool + results channel**. A fixed set of workers pulls
     repo paths from an input channel, runs `git`, and pushes structured
     results into an output channel consumed by the TUI event loop. This is
     textbook Go
     ([betterprogramming: file processing with concurrency](https://betterprogramming.pub/file-processing-using-concurrency-with-golang-9e08920fab63),
     [Go by example: worker pools](https://gobyexample.com/worker-pools)).
   - **OPINION:** pool size should be modest (4–16). Because each worker
     blocks on a child process or disk I/O, throughput is dominated by I/O
     contention, not goroutine count.

### 2.2 Why this is a *better* fit than the alternatives

- **Rust:** async runtimes (tokio/smol/async-std) are powerful but add
  ecosystem choice + borrow-checker friction for what is here simple blocking
  I/O ([futurion.blog](https://futurion.blog/rust-vs-go-for-cli-tools-picking-a-runtime-without-starting-a-tribal-war/)):
  "Go often reaches 'good enough parallelism' with fewer decisions."
- **Python/Node:** GIL (Python) and single-threaded event loop (Node) make
  parallel CPU/disk work harder; you fight the runtime instead of using it.

**FACT:** Go's `context` package gives Ctrl+C cancellation semantics for free
(`context.WithCancel`, `errgroup.WithContext`), which maps directly to "stop
the scan when the user quits the TUI."

**Risk (section 7):** goroutine leaks are the classic Go foot-gun in
long-running processes. In a TUI that lives for minutes and can re-scan
repeatedly, leaked goroutines accumulate silently (`runtime.NumGoroutine()`
climbs). Mitigations: always `defer` channel closes, use `errgroup` with
context, cancel in-flight git processes on quit
([Go sharp edges: goroutine leak](https://github.com/omer-metin/skills-for-antigravity/blob/main/skills/go-services/references/sharp_edges.md),
[rezmoss: live goroutine leak detector](https://rezmoss.com/blog/build-live-goroutine-visualizer-leak-detector-go/)).

---

## 3. Process spawning: exec the real `git` vs go-git

> Coordinates with Track 3 (packages agent) — this section is the
> *language/runtime-level* argument; Track 3 covers the library API details.

### 3.1 The proven architecture: shell out to `git`

**FACT:** lazygit — the closest existing product to `tree-trunk` — does **not**
use go-git for its core operations. Its `pkg/commands/oscommands/os.go`
imports `os/exec` and runs the real `git` binary
([lazygit source](https://github.com/jesseduffield/lazygit/blob/d167063b/pkg/commands/oscommands/os.go),
[cmd_obj.go](https://github.com/jesseduffield/lazygit/blob/d167063b/pkg/commands/oscommands/cmd_obj.go)).
Jesse Duffield's 5-year retrospective explicitly credits a **command log
showing exactly which git commands were run** as one of the most-liked
features ([Lazygit Turns 5](https://jesseduffield.com/Lazygit-5-Years-On)) —
an argument for transparency that shelling out gives you for free.

**FACT:** go-git (the pure-Go git implementation) has documented performance
and correctness gaps:
- [Blame is very slow (#14)](https://github.com/go-git/go-git/issues/14) —
  "Blame ... compared to CLI it is very slow. CLI takes around 1s, go-git 10s"
- [Commit.Patch() dramatically different performance (#67)](https://github.com/go-git/go-git/issues/67)
- [PackWriter.buildIndex is too slow (#813)](https://github.com/go-git/go-git/issues/813)
- [Worktree.Status() descends into nested git worktrees (#1896)](https://github.com/go-git/go-git/issues/1896)
- [Worktree.Status does not handle broken symlink (#253)](https://github.com/go-git/go-git/issues/253)
- Release cadence has historically stalled
  ([#1254 "Please release the latest version"](https://github.com/go-git/go-git/issues/1254)),
  though maintainers have added a stale-issue bot and remain active
  ([PR #849](https://github.com/go-git/go-git/pull/849)).

**FACT:** go-git is 6.2k stars / 768 forks
([repo](https://github.com/go-git/go-git)) — healthy but *not* a git
replacement; the git project's own docs on go-git warn about performance
sensitive operations ([go-git advanced usage](https://go-git.github.io/docs/tutorials/advanced-usage/)).
Big production users (Gitea) wrap the git CLI rather than relying purely on
go-git for everything ([Gitea git internals](https://deepwiki.com/go-gitea/gitea/14-git-internals)).

**OPINION — decision:** `tree-trunk` should **exec the system `git` binary**
for all git operations (status, log, diff, worktree add/remove). Benefits:
exact parity with the user's git version/config (hooks, credential helpers,
lfs, partial clones), zero re-implementation risk, free transparency (show the
command in a log view, lazygit-style). go-git may be considered only for
edge-case metadata reads where exec is awkward — but even then the perf and
compat evidence above argues against it. (See Track 3 for the concrete exec
wrapper recommendation.)

### 3.2 Cost of exec on macOS / Linux / Windows

**FACT (measured here):** `git --version` costs ~10 ms of process-spawn +
binary-load overhead on Apple Silicon; a warm `git status --porcelain` on a
real repo costs ~30 ms total, cold ~320 ms (section 1.1). So exec overhead is
~25–33% of a warm `git status` — negligible per operation, significant when
multiplied across hundreds of repos (hence concurrency, section 2).

**FACT:** on Linux, `fork`+`exec` is cheap (~1 ms); Go's `os/exec` is a thin
wrapper around `os.StartProcess` ([pkg.go.dev/os/exec](https://pkg.go.dev/os/exec)).
On Windows, process creation is more expensive (~5–20 ms) and PATH lookup has
`.exe`-resolution quirks ([golang/go#66586](https://github.com/golang/go/issues/66586) —
Go 1.22 stopped implicitly appending `.exe` when `Cmd.Path` is set manually;
Git-for-Windows ships `git.exe` and must be found in PATH or resolved
explicitly). Modern Windows Terminal + ConPTY handle VT sequences correctly
([Microsoft docs: Console Virtual Terminal Sequences](https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences));
legacy conhost quirks remain the classic Windows TUI risk
([SO: enable ANSI in conhost](https://stackoverflow.com/questions/16755142/how-to-make-win32-console-recognize-ansi-vt100-escape-sequences-in-c)).

**FACT (risk):** `os/exec` had a macOS forkExec hang when spawning many
processes concurrently ([golang/go#61080](https://github.com/golang/go/issues/61080)).
Mitigation: bound the worker pool (never spawn unbounded concurrent children)
and set `Cmd.WaitDelay` + context cancellation so stale children can't hang
the UI ([os/exec docs](https://pkg.go.dev/os/exec)).

### 3.3 Buffering & streaming into the TUI

**FACT:** `os/exec` supports `cmd.Stdout`/`cmd.Stderr` as `io.Writer`/`io.Reader`.
Two patterns:
- **Buffered:** `cmd.Output()` — collect full output, parse, then display.
  Right for `git status --porcelain`, `git worktree list --porcelain`, `git
  log --format=...` (small, bounded output).
- **Streamed:** attach a pipe and read incrementally — right for `git diff`
  on large trees or long `git log` output, feeding a pager-style view. A TUI
  should render a "loading" state while streaming, not block the event loop.
  lazygit runs git commands to completion in a goroutine and pushes messages
  into its UI loop rather than blocking on the renderer
  ([lazygit git_commands layer](https://github.com/jesseduffield/lazygit/blob/d167063b/pkg/commands/git_commands/file.go)).

**OPINION:** keep all git I/O off the TUI event-loop goroutine; use the
"command → goroutine → channel → message → view" pipeline. This is also the
bubbletea command model (`tea.Cmd` returning a `Msg`), so the framework
encourages the right shape (Track 3).

---

## 4. TUI ecosystem maturity

### 4.1 The landscape (Go)

- **bubbletea** (charmbracelet) — Elm-architecture framework, **~38k stars**
  (37,996 per [go-fan module review](https://github.com/github/gh-aw/discussions/9188),
  Jan 2026), updated "hours ago" as of Jan 2026; **v2.0.0 released 2026-02-24**
  ([release notes](https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0))
  with a declarative View rewrite and a published upgrade guide
  ([PR #1585](https://github.com/charmbracelet/bubbletea/pull/1585)). Companion
  libs: bubbles (components), lipgloss (styling), and examples for exec,
  pager, list, viewport, spinner, etc.
  ([bubbletea repo](https://github.com/charmbracelet/bubbletea)).
  Production users: Charm's own apps (Glow, Huh, Crush), plus a long list of
  community tools ([bubbletea README](https://github.com/charmbracelet/bubbletea)).
- **gocui** — the framework **lazygit and lazydocker actually use**
  (Jesse Duffield's maintained fork). Mature but idiosyncratic (immediate-mode,
  manual layout).
- **tview** — batteries-included widget tree; **k9s uses a maintained fork**
  (derailed/tview). Popular for dashboards; simpler than bubbletea but less
  "modern" (Elm vs. imperative)
  ([Reddit r/golang: TUI recommendations](https://www.reddit.com/r/golang/comments/1fgvu6y/tui_recommendations/),
  [HN: building bubbletea programs — some devs switch to tview](https://news.ycombinator.com/item?id=41369065)).
- **termui** — chart/plot-oriented; not a fit for an interactive git tool.

**FACT:** three distinct mature Go TUI frameworks each power a famous
production tool (lazygit→gocui, k9s→tview, Charm ecosystem→bubbletea). The
ecosystem is not a single point of failure. JetBrains' 2025 Go ecosystem
report calls bubbletea the go-to interactive TUI framework
([JetBrains blog](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025)).

**FACT (risk):** bubbletea v2 (Feb 2026) is a **major breaking release**.
Starting a new project on v2 is fine (v2 is current and supported), but any
v1 tutorial/blog content will mislead; pin versions and read the
[upgrade guide](https://github.com/charmbracelet/bubbletea/UPGRADE_GUIDE_V2.md)
(Track 3 will pin exact versions).

**OPINION:** bubbletea + bubbles + lipgloss is the best default for a new Go
TUI in 2026: most active development, best docs/examples, and its
command/message model matches the async git workload (section 3.3). tview is
the fallback if the team dislikes Elm architecture. Track 3 decides; from a
suitability standpoint Go offers at least two viable, production-proven paths.

### 4.2 How the alternatives' ecosystems compare

| Ecosystem | Framework | Evidence of maturity |
|---|---|---|
| Go | bubbletea | ~38k stars, v2 Feb 2026, Charm-backed, production apps |
| Rust | ratatui | **42.5M total crates.io downloads, 15.9M recent, 5,236 dependents** ([crates.io](https://crates.io/crates/ratatui)); used by Netflix (bpftop), OpenAI, AWS (amazon-q-developer-cli), Vercel ([glukhov.org](https://www.glukhov.org/developer-tools/comparisons/tui-frameworks-bubbletea-go-vs-ratatui-rust/)) |
| Python | textual | **36.7k stars**, very active (willmcgugan) ([repo](https://github.com/textualize/textual)); startup-time is a documented pain point ([Posting startup +40%](https://darren.codes/posts/python-startup-time), [app.exit 200ms](https://github.com/Textualize/textual/discussions/6497)) |
| TypeScript/Node | ink | React-renderer; last tagged release May 2025, commits through Mar 2026 ([wistrand comparison](https://github.com/wistrand/melker/blob/main/agent_docs/tui-comparison.md)); needs Node runtime (~30MB + deps per HN) ([HN](https://news.ycombinator.com/item?id=35863837)) |
| C++ | FTXUI | 9.9k stars, active (v7.0.0 Jun 2026) ([repo](https://github.com/ArthurSonzogni/FTXUI)) but C++ build/distribution friction |

---

## 5. Distribution & packaging

### 5.1 Channels

**FACT:** Go tools ship through every channel `tree-trunk` would want:
- `go install github.com/...@latest` — zero-friction install for Go users
  ([lazygit.dev install options](https://lazygit.dev/)).
- **Homebrew** — formula or tap. lazygit: 15k installs-on-request/yr in
  Homebrew + 4.6% of Arch Linux users ([Lazygit Turns 5](https://jesseduffield.com/Lazygit-5-Years-On)).
- **Prebuilt binaries** — GitHub Releases + `goreleaser` (bubbletea itself uses
  `.goreleaser.yml`; lazygit publishes per-OS assets). Go makes this trivial
  because cross-compilation needs no CI matrix gymnastics.
- **Scoop/Chocolatey** (Windows), AUR/apt/deb/rpm (Linux).

### 5.2 Cross-compilation specifics

**FACT:** `GOOS=darwin|linux|windows GOARCH=amd64|arm64 CGO_ENABLED=0 go build`
produces working static binaries for all six target combos from one machine —
this is the single biggest distribution advantage Go has over every
alternative in scope
([discuss.ocaml.org](https://discuss.ocaml.org/t/why-is-there-no-tradition-of-cli-and-tui-apps/14628),
[Ecostack](https://ecostack.dev/posts/go-and-cgo-cross-compilation/)).
Pure-Go deps (all the TUI + git-exec + config libraries in scope) keep
`CGO_ENABLED=0` valid. Apple Silicon + Intel macOS and ARM/AMD Windows/Linux
all covered.

**OPINION:** binary size (a bubbletea TUI lands ~10–25 MB uncompressed) is a
non-issue for a dev tool; `-ldflags="-s -w"` and optional UPX trims it if
anyone cares.

### 5.3 Versioning & supply chain

**FACT:** Go modules pin exact versions; `govulncheck` in CI is the Go-native
equivalent of `cargo-audit` ([futurion.blog](https://futurion.blog/rust-vs-go-for-cli-tools-picking-a-runtime-without-starting-a-tribal-war/)).
For a small project: pin `go.mod`, run `go mod tidy`, add a release workflow.

---

## 6. Alternatives comparison (honest)

> The brief names Rust (ratatui/egui), Python (rich/textual), TypeScript/Node
> (ink), C++ (FTXUI). **Correction:** egui is an immediate-mode *GUI* for
> native/web, not a terminal UI — for Rust the relevant comparison is ratatui.

### 6.1 Rust (ratatui)

**Strengths:** fastest raw rendering; smallest memory; strongest type safety.
Ratatui is excellent and extremely well-adopted (42.5M downloads — section
4.2).

**Weaknesses for THIS project:** (a) our workload is I/O-bound — Go's
performance is indistinguishable where it matters; (b) Rust's async ecosystem
(tokio vs smol vs async-std) adds decisions Go doesn't require
([futurion.blog](https://futurion.blog/rust-vs-go-for-cli-tools-picking-a-runtime-without-starting-a-tribal-war/));
(c) compile times and borrow-checker iteration cost on a small team;
(d) distribution is fine (cargo install, brew, releases) but cross-compilation
needs `cross`/target matrices, not a one-liner
([futurion.blog](https://futurion.blog/rust-vs-go-for-cli-tools-picking-a-runtime-without-starting-a-tribal-war/)).

**OPINION:** Rust wins when you need peak CPU performance or minimal memory in
a constrained environment. A git-worktree browser is neither. Go wins on
velocity with zero meaningful perf loss for this workload. Benchmarks
([Benchmarks Game: Rust vs Go](https://benchmarksgame-team.pages.debian.net/benchmarksgame/fastest/rust-go.html),
[programming-language-benchmarks](https://programming-language-benchmarks.vercel.app/go-vs-rust))
consistently show Rust ahead on CPU microbenchmarks and Go competitive on
real-world I/O-bound tools — the gap that matters here is nil.

### 6.2 Python (rich/textual)

**Strengths:** fastest prototyping; textual is genuinely excellent and active
(36.7k stars).

**Weaknesses for THIS project:** (a) **startup time and per-frame overhead** —
documented pain even by textual's own maintainers (Posting's startup was
improved 40% [darren.codes](https://darren.codes/posts/python-startup-time);
`app.exit` taking 200ms [discussion #6497](https://github.com/Textualize/textual/discussions/6497));
(b) distribution is the killer: no static single binary — you ship a Python
runtime, a venv/zipapp, or a bundled binary (PyInstaller ~50MB+ with AV
false-positive risk); (c) GIL constrains the parallel-repo-scan phase;
(d) dependency management on user machines (pip/uv) is a support burden.

**OPINION:** Python is the wrong distribution story for a tool whose whole
value is "one command, instant, everywhere." For a TUI that must feel snappy,
Go's startup advantage is structural, not cosmetic.

### 6.3 TypeScript/Node (ink)

**Strengths:** React mental model; huge contributor pool; npm distribution.

**Weaknesses for THIS project:** (a) requires the Node runtime (≈30MB + node_modules, per
[HN thread](https://news.ycombinator.com/item?id=35863837)) unless you ship a
bundled runtime (Bun/deno compile) — you're back to 50MB+ downloads;
(b) Node startup is measurably slower than a native binary
([Alex Ellis](https://blog.alexellis.io/5-keys-to-a-killer-go-cli) cites
1.5–3s load on constrained hardware); (c) ink's maintenance has slowed (last
tagged release May 2025, per [wistrand comparison](https://github.com/wistrand/melker/blob/main/agent_docs/tui-comparison.md)).

**OPINION:** if the team were TS-only, ink would be defensible. For a
terminal-native tool targeting "snappy on every platform," the runtime
dependency is a real cost with no offsetting benefit for this workload.

### 6.4 C++ (FTXUI)

**Strengths:** minimal footprint; FTXUI is a quality library (9.9k stars, active).

**Weaknesses for THIS project:** (a) build & distribution friction: compile on
every platform, link against system libs, no single-binary story comparable to
Go; (b) memory-safety risk in a tool that parses git output and walks
filesystems; (c) slow iteration for a small team. For a TUI whose bottleneck is
waiting on `git` subprocesses, C++'s performance is irrelevant.

**OPINION:** no reason to choose C++ for this tool in 2026 unless the team is
C++-only. Not competitive on velocity or safety.

### 6.5 Summary table

| Criterion | Go (bubbletea) | Rust (ratatui) | Python (textual) | TS/Node (ink) | C++ (FTXUI) |
|---|---|---|---|---|---|
| Startup speed | ms | ms | ~100–300ms+ | ~30–100ms (Node boot) | ms |
| Single static binary | ✅ native | ✅ native | ❌ runtime/bundle | ❌ runtime/bundle | ⚠️ platform builds |
| Cross-compile | ✅ one-liner | ⚠️ needs toolchain matrix | ❌ | ❌ | ❌ |
| Concurrency for N-repo scan | ✅ goroutines | ✅ async (extra choices) | ⚠️ GIL | ⚠️ single-threaded loop | ✅ threads (manual) |
| TUI framework maturity | ✅ bubbletea ~38k★ | ✅ ratatui 42M dl | ✅ textual 37k★ | ⚠️ slower maintenance | ✅ FTXUI 10k★ |
| Dev velocity (small team) | ✅ | ⚠️ borrow checker + compile times | ✅ | ✅ | ❌ |
| Memory safety | ⚠️ GC (fine here) | ✅ | ⚠️ | ⚠️ | ❌ |
| Ecosystem evidence for git TUIs | ✅ lazygit/lazydocker/k9s | ⚠️ fewer git-TUI examples | ✅ gitui? (weak) | ⚠️ | ❌ |

**FACT anchors:** bubbletea ~38k★ ([go-fan](https://github.com/github/gh-aw/discussions/9188));
ratatui 42.5M downloads ([crates.io](https://crates.io/crates/ratatui));
textual 36.7k★ ([repo](https://github.com/textualize/textual));
FTXUI 9.9k★ ([repo](https://github.com/ArthurSonzogni/FTXUI));
lazygit 79k★ ([GitHub](https://github.com/jesseduffield/lazygit)).

---

## 7. Risks & known pain points

1. **go-git is not a safe fallback for git semantics.** Documented perf bugs
   (Blame 10x slower, #14), worktree edge-case bugs (#1896, #253), and
   historically slow release cadence (#1254). **Mitigation:** exec system git;
   treat go-git as out of scope (Track 3 may revisit for read-only metadata).
2. **os/exec concurrency hazards.** macOS forkExec hang when spawning many
   processes ([#61080](https://github.com/golang/go/issues/61080)); Windows
   `.exe` PATH resolution ([#66586](https://github.com/golang/go/issues/66586)).
   **Mitigation:** bounded worker pool (never unbounded), `Cmd.WaitDelay` +
   context cancellation, resolve git explicitly per-OS
   (git-lfs tracks this same problem: [git-lfs#5612](https://github.com/git-lfs/git-lfs/issues/5612)).
3. **Goroutine leaks in a long-running TUI.** The classic Go sharp edge
   ([sharp_edges.md](https://github.com/omer-metin/skills-for-antigravity/blob/main/skills/go-services/references/sharp_edges.md));
   every rescan/command spawns goroutines that must terminate on quit.
   **Mitigation:** `errgroup` + context, `runtime.NumGoroutine()` assertions in
   tests, `go vet` + `leaktest`-style checks.
4. **bubbletea v2 breaking change (Feb 2026).** Fresh project → adopt v2
   deliberately; most tutorials/answers on the web target v1
   ([upgrade guide](https://github.com/charmbracelet/bubbletea/UPGRADE_GUIDE_V2.md)).
5. **Windows terminal divergence.** ConPTY/Windows Terminal are fine; legacy
   conhost + VT sequences are not ([SO](https://stackoverflow.com/questions/16755142/how-to-make-win32-console-recognize-ansi-vt100-escape-sequences-in-c)).
   bubbletea handles the modern cases; test on real Windows, not just WSL.
6. **Git version skew.** Exec'ing system git inherits whatever the user has
   (2.39 on this machine; 2.49+ elsewhere). `git worktree list --porcelain`
   output format changed across versions — parse defensively, and consider
   requiring a minimum git version with a clear error.
7. **GC is a non-risk for this workload** — noted here for completeness: Go
   GC latency is sub-ms and irrelevant to text rendering
   ([Twitch](https://blog.twitch.tv/en/2016/07/05/gos-march-to-low-latency-gc-a6fa96f06eb7/)).
8. **Binary size (~10–25MB)** — cosmetic only; strip with `-s -w`.

---

## 8. Verdict

**FACT-supported conclusion:** Go is the best-fit language for `tree-trunk`
among the five considered, for four structural reasons, each verified:

1. **The exact niche is already Go-proven:** lazygit (79k★, the market-leading
   git TUI) and lazydocker run on Go and shell out to the real `git` binary.
2. **I/O-bound fan-out is Go's home turf:** bounded worker pools over
   goroutines + channels for scanning N repos and running N git commands —
   the canonical pattern, with context-based Ctrl+C cancellation.
3. **Distribution is a one-liner:** static binaries, `CGO_ENABLED=0`
   cross-compilation to 6 platform/arch combos, `go install` / Homebrew /
   releases — matching the project brief's "macOS/Linux/Windows" requirement.
4. **TUI ecosystem is mature and actively maintained:** bubbletea v2 (2026),
   tview, gocui — multiple production-proven paths; ~10ms startup measured for
   git exec and negligible Go startup make the UX requirement achievable.

**The decision that matters more than the language:** exec system `git` (not
go-git), keep the UI event loop free of blocking I/O, bound all concurrency,
and pin bubbletea v2 at the start.

---

## Open questions (for design phase)

1. **Minimum supported git version?** Affects `--porcelain` parsing and
   worktree command flags (2.30+ supports `git worktree list --porcelain` v2
   output; older formats differ). Recommend: require ≥2.30, ideally ≥2.38.
2. **bubbletea v2 vs tview?** Track 3 should pin one. This agent's lean:
   bubbletea v2 (Elm model fits async git ops; best docs). Team comfort with
   the Elm architecture is the deciding factor.
3. **Windows support priority?** ConPTY handling in bubbletea is mature, but
   Windows is where terminal quirks live (section 7.5). Is Windows a first-
   class target or best-effort? Budget QA time accordingly.
4. **Do we need `git diff` streaming (pager-style) or buffered output only?**
   Large diffs (monorepos) favor streaming into a viewport; MVP could buffer
   with a size cap.
5. **Concurrency budget for repo scans:** pool size / whether to cap scan
   depth (skip `.git` in `node_modules`, `.Trash`, etc.). Default suggestion:
   bounded pool (4–16), depth cap, skip-list — confirm with Track 3.
6. **Should the scan phase be incremental/cached across launches?** A ~100ms
   full scan (measured: git status warm ≈ 30ms/repo, so 100 repos ≈ 3s serial /
   ~0.5s with 8 workers) may not justify a cache; decide in design.

---

## Appendix A — sources index

- bubbletea repo/README/upgrade guide — https://github.com/charmbracelet/bubbletea
- bubbletea v2.0.0 release — https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0
- bubbles components — https://github.com/charmbracelet/bubbles
- lazygit — https://github.com/jesseduffield/lazygit ; https://lazygit.dev
- Lazygit Turns 5 (Duffield) — https://jesseduffield.com/Lazygit-5-Years-On
- lazygit os/exec source — https://github.com/jesseduffield/lazygit/blob/d167063b/pkg/commands/oscommands/os.go
- go-git issues: #14 (blame), #67 (patch perf), #813 (packwriter), #1896 (nested worktrees), #253 (symlink), #1254 (release cadence)
- go-git advanced usage — https://go-git.github.io/docs/tutorials/advanced-usage/
- Gitea git internals — https://deepwiki.com/go-gitea/gitea/14-git-internals
- os/exec docs — https://pkg.go.dev/os/exec
- golang/go#61080 (forkExec hang), #66586 (.exe), #14812 (GC spikes)
- O'Reilly errgroup parallel file search — https://www.oreilly.com/content/run-strikingly-fast-parallel-file-searches-in-go-with-sync-errgroup/
- StackOverflow concurrent fs scanning — https://stackoverflow.com/questions/44255814/concurrent-filesystem-scanning
- Ratatui — https://crates.io/crates/ratatui ; glukhov.org BubbleTea-vs-Ratatui comparison — https://www.glukhov.org/developer-tools/comparisons/tui-frameworks-bubbletea-go-vs-ratatui-rust/
- Textual — https://github.com/textualize/textual ; Posting startup — https://darren.codes/posts/python-startup-time
- Ink — https://github.com/vadimdemedes/ink
- FTXUI — https://github.com/ArthurSonzogni/FTXUI
- Rust vs Go for CLI (futurion.blog) — https://futurion.blog/rust-vs-go-for-cli-tools-picking-a-runtime-without-starting-a-tribal-war/
- Benchmarks Game Rust vs Go — https://benchmarksgame-team.pages.debian.net/benchmarksgame/fastest/rust-go.html
- Go cross-compilation/cgo — https://ecostack.dev/posts/go-and-cgo-cross-compilation/
- Alex Ellis, 5 keys to a killer CLI — https://blog.alexellis.io/5-keys-to-a-killer-go-cli
- Twitch low-latency GC — https://blog.twitch.tv/en/2016/07/05/gos-march-to-low-latency-gc-a6fa96f06eb7/
- Go goroutine-leak sharp edges — https://github.com/omer-metin/skills-for-antigravity/blob/main/skills/go-services/references/sharp_edges.md
- Windows VT sequences — https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences
- wistrand multi-language TUI comparison — https://github.com/wistrand/melker/blob/main/agent_docs/tui-comparison.md
