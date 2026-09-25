# Changelog

All notable changes to tree-trunk are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Agent exit-code contract.** `tree-trunk wt` (and the CLI generally) now
  exit `0` ok / `1` usage-IO / `3` not found / `4` dirty-blocked / `5` locked.
  Coded failures write the JSON envelope to stdout and nothing to stderr, so
  agents can branch on process status instead of parsing text. The mapping is
  machine-readable via `tree-trunk describe` (`exit_codes`).
- `tree-trunk wt --help` / `-h` / `help` prints the ops + exit codes and exits 0.
- `wt create/delete/lock/unlock/prune`: mutating worktree API with structured
  errors (`repo_not_found`, `worktree_not_found`, `worktree_dirty`,
  `worktree_locked`, `branch_checked_out_elsewhere`, `branch_exists`,
  `git_error`), `--json`/`--no-json`, `--force`, `--dry-run`, `--from`,
  `--path`, `--reason`, and flags that may appear before or after positionals.
- `wt list`: cross-repo worktree aggregate.
- `tree-trunk query`: deterministic JSON read-API with `--filter k=v`,
  `--fields`, `--repo`, `--scan-root`, `--no-scan`.
- `tree-trunk describe`: self-describing CLI schema (subcommands, flags, exit
  codes) for agents that cannot parse prose help.
- `tree-trunk completion zsh|bash`; man page (`man tree-trunk`); `AGENTS.md`
  agent contract.
- `--help` on any subcommand is a success (exit 0) instead of an error.

### Fixed

- Removing a **locked** worktree now reports `worktree_locked` (exit 5) instead
  of a generic `git_error`: git's `cannot remove a locked working tree` message
  was not matched by the classifier, and the CLI now also checks the parsed
  `locked` flag before calling git. This also restores the typed error the TUI
  relies on. Covered by `TestWorktreeLifecycle`.

### Changed

- `wt` error envelopes no longer include an empty `"op": ""` field.
- Docs refreshed: man page gained the `wt` section + a real EXIT STATUS
  section; `docs/README.md` no longer claims the project is unimplemented;
  implementation plan and open-questions docs reflect reality.

### Known limitations

- Windows is best-effort (untested clipboard/terminal handling).
- `theme.variant = "auto"` behaves as `dark` (terminal-background detection is
  still pending).
- No syntax highlighting in diffs; side-by-side word-level highlight is pending.

## [0.1.0] - unreleased

First release. TUI + CLI: zero-config repo discovery, per-repo status,
worktree management (create/delete/lock/unlock/prune), log and diff views,
configurable scan roots/themes/clipboard, CI on git 2.38 + latest, and a
goreleaser release pipeline with a Homebrew formula.

[Unreleased]: https://github.com/AarnoStormborn/tree-trunk/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/AarnoStormborn/tree-trunk/releases/tag/v0.1.0
