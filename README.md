# tree-trunk

**Every git repo on your machine, in one terminal window.**

tree-trunk is a terminal app that finds all the git repositories on your
computer and shows you their state at a glance — which branch each one is on,
what's changed, what's ahead of or behind its remote. From there you can
create and remove git *worktrees* (independent working copies of a repo for
working on multiple branches side by side), and inspect a repo's history,
diffs, and status — without hopping between directories and running one git
command at a time.

## Why

Working across several repositories quickly turns into a lot of window
switching and remembering: *which repo was that again? is anything dirty?
which branches are checked out where?* tree-trunk replaces that with a single
overview — a sidebar of every repo you own, and a detail panel for whichever
one you select.

## Install

Requires **git ≥ 2.38** on your system.

```sh
# Homebrew (once the tap releases)
brew install aarnostormborn/tap/tree-trunk

# or from source
go install github.com/AarnoStormborn/tree-trunk@latest
```

## Usage

```sh
tree-trunk                          # find everything under your home dir and open the app
tree-trunk --list                   # just print your repos, one path per line
tree-trunk --repo ~/code/app        # open with a specific project (repeatable)
tree-trunk --scan-root ~/src        # scan a specific folder instead of your home dir
tree-trunk completion zsh           # shell tab-completion
```

Once the app is open:

- **Left sidebar** — every repo found, with its branch and change status.
  Use `j`/`k` to move around, `l` to expand a repo's worktrees.
- **Main panel** — tabs for the selected repo: `1` status, `2` worktrees,
  `3` history, `4` diff. `tab` switches focus between the sidebar and the
  main panel.
- **Worktrees** — `n` creates one (path is suggested for you), `d` removes
  one (it always asks twice before deleting anything, and never deletes a
  branch), `L` locks/unlocks, `P` prunes stale ones, `o` copies its path.
- **History & diffs** — pick a commit in the log to see its diff; `m` cycles
  the diff view (working tree / staged / against main), `p` toggles a compact
  summary. Diffs are color-coded: green added, red removed.

Full key reference: press `?` inside the app.

### Configuration

tree-trunk works out of the box. If you want to tweak it, create
`~/.config/tree-trunk/config.toml` (scan folders, worktree location,
appearance). New worktrees default to `~/.worktrees/<repo>/<branch>`.
