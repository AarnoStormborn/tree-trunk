package main

import (
	"fmt"
	"strings"
)

// completionScripts returns shell completion for the given shell
// (M4 backlog: shell completion, P1). Supported: zsh, bash.
func completionScripts(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshCompletion, nil
	case "bash":
		return bashCompletion, nil
	}
	return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash)", shell)
}

const zshCompletion = `#compdef tree-trunk
# tree-trunk zsh completion
_tree-trunk() {
  local -a flags
  flags=(
    '--repo[explicit repo path (repeatable)]:path:_files -/'
    '--scan-root[scan root (replaces defaults)]:dir:_directories'
    '--no-scan[do not scan the filesystem]'
    '--config[config file path]:file:_files'
    '--list[print repo paths, one per line]'
    '--version[print version]'
    '--help[show help]'
  )
  _arguments -s $flags
}
compdef _tree-trunk tree-trunk
`

const bashCompletion = `# tree-trunk bash completion
_tree-trunk() {
  local cur prev
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  local flags="--repo --scan-root --no-scan --config --list --version --help"

  case "$prev" in
    --repo|--config)
      COMPREPLY=( $(compgen -f -- "$cur") )
      return 0
      ;;
    --scan-root)
      COMPREPLY=( $(compgen -d -- "$cur") )
      return 0
      ;;
  esac

  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
  fi
  return 0
}
complete -F _tree-trunk tree-trunk
`

// printCompletions writes the completion script to stdout.
func printCompletions(shell string) error {
	script, err := completionScripts(strings.ToLower(shell))
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
}
