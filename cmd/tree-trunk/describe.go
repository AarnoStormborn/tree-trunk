package main

import (
	"encoding/json"
	"fmt"
)

// cliSchema is the machine-readable description of the tree-trunk CLI.
// Agents that cannot parse prose --help can introspect this to discover the
// subcommmands, their flags, and the read-API output schema.
type cliSchema struct {
	Command     string   `json:"command"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Default     string   `json:"default"` // what `tree-trunk` with no args does
	Subcommands []subCmd `json:"subcommands"`
}

type subCmd struct {
	Name     string    `json:"name"`
	Usage    string    `json:"usage"`
	Desc     string    `json:"desc"`
	Flags    []cliFlag `json:"flags,omitempty"`
	Ancestor string    `json:"output,omitempty"` // JSON sub-schema ref name
}

type cliFlag struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // bool | string | repeatable
	Desc string `json:"desc"`
}

func buildCLISchema(version string) cliSchema {
	return cliSchema{
		Command:     "tree-trunk",
		Description: "TUI and CLI for listing git repos and managing worktrees",
		Version:     version,
		Default:     "launch the interactive TUI",
		Subcommands: []subCmd{
			{
				Name:  "describe",
				Usage: "tree-trunk describe",
				Desc:  "Emit the machine-readable CLI schema (subcommands, flags, output docs).",
			},
			{
				Name:  "wt",
				Usage: "tree-trunk wt <list|create|delete|lock|unlock|prune> [repo] [branch] [flags]",
				Desc:  "Manage git worktrees (mutating agent API): list, create, delete, lock, unlock, prune. Reuses guarded git ops; structured errors. Flags may appear before or after positionals.",
				Flags: []cliFlag{
					{Name: "json", Kind: "bool", Desc: "emit JSON (default)"},
					{Name: "force", Kind: "bool", Desc: "delete: bypass dirty check; create: bypass guards"},
					{Name: "dry-run", Kind: "bool", Desc: "prune: preview without executing"},
					{Name: "from", Kind: "string", Desc: "create: base commit/branch (default HEAD)"},
					{Name: "path", Kind: "string", Desc: "create: destination (default ~/.worktrees/<repo>/<slug>)"},
					{Name: "reason", Kind: "string", Desc: "lock: reason"},
					{Name: "repo", Kind: "repeatable", Desc: "explicit repo path"},
					{Name: "scan-root", Kind: "repeatable", Desc: "scan root"},
					{Name: "no-scan", Kind: "bool", Desc: "do not scan the filesystem"},
				},
			},
			{
				Name:  "query",
				Usage: "tree-trunk query [--json] [--filter k=v]... [--fields a,b] [--repo PATH]... [--scan-root ROOT]... [--no-scan]",
				Desc:  "Emit the repo/worktree/status state as a single JSON document (agent read-API). 'what's working where, and how much'.",
				Flags: []cliFlag{
					{Name: "json", Kind: "bool", Desc: "emit a single pretty JSON document"},
					{Name: "filter", Kind: "repeatable", Desc: "filter k=v; keys: id,name,branch,dirty,prunable,locked,bare,worktrees"},
					{Name: "fields", Kind: "repeatable", Desc: "only top-level fields; e.g. id,branch,status"},
					{Name: "repo", Kind: "repeatable", Desc: "explicit repo path"},
					{Name: "scan-root", Kind: "repeatable", Desc: "scan root"},
					{Name: "no-scan", Kind: "bool", Desc: "do not scan the filesystem"},
				},
				Ancestor: "query-output",
			},
			{
				Name:  "completion",
				Usage: "tree-trunk completion zsh|bash",
				Desc:  "Print a shell completion script.",
				Flags: []cliFlag{{Name: "shell", Kind: "string", Desc: "zsh or bash"}},
			},
		},
	}
}

func printDescribe(version string) error {
	schema := buildCLISchema(version)
	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}
