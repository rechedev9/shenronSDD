package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
)

var commands = []string{
	"init", "new", "context", "write", "status", "list",
	"verify", "archive", "diff", "health", "dump", "doctor",
	"errors", "completion", "version", "help",
}

func runCompletion(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd completion <bash|zsh|fish>")
	}

	switch args[0] {
	case "bash":
		fmt.Fprintf(stdout, `# sdd bash completion
# Add to ~/.bashrc: eval "$(sdd completion bash)"
_sdd_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    COMPREPLY=($(compgen -W "%s" -- "$cur"))
}
complete -F _sdd_completions sdd
`, strings.Join(commands, " "))

	case "zsh":
		fmt.Fprintf(stdout, `#compdef sdd
# sdd zsh completion
# Add to ~/.zshrc: eval "$(sdd completion zsh)"
_sdd() {
    local -a commands
    commands=(%s)
    _describe 'command' commands
}
compdef _sdd sdd
`, strings.Join(commands, " "))

	case "fish":
		fmt.Fprintln(stdout, "# sdd fish completion")
		fmt.Fprintln(stdout, `# Add to ~/.config/fish/completions/sdd.fish`)
		for _, cmd := range commands {
			fmt.Fprintf(stdout, "complete -c sdd -n '__fish_use_subcommand' -a %s\n", cmd)
		}

	default:
		return errs.Usage(fmt.Sprintf("unknown shell: %s (use bash, zsh, or fish)", args[0]))
	}

	return nil
}
