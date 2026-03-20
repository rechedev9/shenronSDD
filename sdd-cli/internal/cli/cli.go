package cli

import (
	"fmt"
	"io"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
)

var version = "dev"

// Run is the top-level entry point. All subcommands dispatch from here.
func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errs.Usage("no command specified; run 'sdd --help' for usage")
	}

	cmd := args[0]
	rest := args[1:]

	// Handle per-command help: sdd <cmd> --help
	if len(rest) == 1 && (rest[0] == "--help" || rest[0] == "-h") {
		if text, ok := commandHelp[cmd]; ok {
			fmt.Fprintln(stdout, text)
			return nil
		}
	}

	switch cmd {
	case "init":
		return runInit(rest, stdout, stderr)
	case "new":
		return runNew(rest, stdout, stderr)
	case "context":
		return runContext(rest, stdout, stderr)
	case "write":
		return runWrite(rest, stdout, stderr)
	case "status":
		return runStatus(rest, stdout, stderr)
	case "list":
		return runList(rest, stdout, stderr)
	case "verify":
		return runVerify(rest, stdout, stderr)
	case "archive":
		return runArchive(rest, stdout, stderr)
	case "diff":
		return runDiff(rest, stdout, stderr)
	case "health":
		return runHealth(rest, stdout, stderr)
	case "--version", "version":
		fmt.Fprintln(stdout, version)
		return nil
	case "--help", "help":
		printHelp(stdout)
		return nil
	default:
		return errs.Usage(fmt.Sprintf("unknown command: %s", cmd))
	}
}

// ExitCode returns the appropriate exit code for an error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errs.IsUsage(err) {
		return 2
	}
	return 1
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "sdd — Spec-Driven Development context engine")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: sdd <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Pipeline commands:")
	fmt.Fprintln(w, "  init              Bootstrap openspec/ in current project")
	fmt.Fprintln(w, "  new <name> <desc> Start a new change, run explore assembler")
	fmt.Fprintln(w, "  context <name>    Assemble context for current (or specified) phase")
	fmt.Fprintln(w, "  write <name> <ph> Promote .pending artifact, advance state machine")
	fmt.Fprintln(w, "  verify <name>     Run build/lint/test quality gate (zero tokens)")
	fmt.Fprintln(w, "  archive <name>    Archive completed change (zero tokens)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Inspection commands:")
	fmt.Fprintln(w, "  status <name>     Show phase progress for a change")
	fmt.Fprintln(w, "  list              List all active changes")
	fmt.Fprintln(w, "  diff <name>       List files changed since 'sdd new' was run")
	fmt.Fprintln(w, "  health <name>     Pipeline health: progress, cache stats, warnings")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Other:")
	fmt.Fprintln(w, "  version           Print version")
	fmt.Fprintln(w, "  help              Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'sdd <command> --help' for details on a specific command.")
	fmt.Fprintln(w)
}

var commandHelp = map[string]string{
	"init": `sdd init — Bootstrap SDD in a project

Usage: sdd init [path] [--force]

Detects tech stack from manifest files (go.mod, package.json, etc.),
creates openspec/ directory structure, and writes config.yaml.

Flags:
  --force, -f    Reinitialize even if openspec/ already exists

Output: JSON with detected config, created directories.
Exit:   0 success, 1 error, 2 usage`,

	"new": `sdd new — Start a new change

Usage: sdd new <name> "<description>"

Creates openspec/changes/<name>/ with initial state.json, then runs
the explore context assembler and prints it to stdout.

Arguments:
  name          Change name (kebab-case)
  description   Brief intent description

Output: Explore context to stdout (SKILL + project info + file tree).
Exit:   0 success, 1 error, 2 usage`,

	"context": `sdd context — Assemble phase context

Usage: sdd context <name> [phase]

Loads the SKILL.md for the phase, relevant artifacts, and source context,
then prints the assembled context to stdout. If phase is omitted, uses
the current phase from state.json.

Arguments:
  name          Change name
  phase         Optional: explore, propose, spec, design, tasks, apply, review, clean

Output: Assembled context to stdout.
Exit:   0 success, 1 error, 2 usage`,

	"write": `sdd write — Promote artifact and advance state

Usage: sdd write <name> <phase>

Moves .pending/<phase>.md to its final location and advances the state
machine. Claude writes to .pending/, Go promotes and tracks state.

Arguments:
  name          Change name
  phase         Phase to promote (explore, propose, spec, design, tasks, apply, review, verify, clean)

Output: JSON with promoted path and new current phase.
Exit:   0 success, 1 error, 2 usage`,

	"status": `sdd status — Show change progress

Usage: sdd status <name>

Reads state.json and displays the current phase, all completed phases,
and whether the pipeline is complete.

Arguments:
  name          Change name

Output: JSON with phase statuses, completed list, is_complete flag.
Exit:   0 success, 1 error, 2 usage`,

	"list": `sdd list — List active changes

Usage: sdd list

Scans openspec/changes/ for directories with valid state.json files.
Excludes the archive/ directory.

Output: JSON with count and per-change info (name, phase, description).
Exit:   0 success, 1 error`,

	"verify": `sdd verify — Run quality gate

Usage: sdd verify <name>

Runs build, lint, and test commands from config.yaml sequentially.
Stops on first failure. Writes verify-report.md to the change directory.

This is a zero-token operation — runs entirely in Go.

Arguments:
  name          Change name

Output: JSON with passed flag and report path.
Exit:   0 all checks pass, 1 any check fails, 2 usage`,

	"health": `sdd health — Pipeline health summary

Usage: sdd health <name>

Shows progress, cache statistics, estimated token usage, staleness,
and any warnings (abandoned changes, failed verifications).

Arguments:
  name          Change name

Output: JSON with completed phases, cache hits/misses, total tokens, warnings.
Exit:   0 success, 1 error, 2 usage`,

	"diff": `sdd diff — List files changed since change was created

Usage: sdd diff <name>

Runs 'git diff --name-only <base_ref>' where base_ref is the git SHA
captured when 'sdd new' was run.

Arguments:
  name          Change name

Output: JSON with files list, count, and base_ref SHA.
Exit:   0 success, 1 error, 2 usage`,

	"archive": `sdd archive — Archive completed change

Usage: sdd archive <name>

Moves the change directory to openspec/changes/archive/<timestamp>-<name>/
and writes archive-manifest.md listing all preserved artifacts.

Requires all prerequisite phases (through clean) to be completed.

This is a zero-token operation — runs entirely in Go.

Arguments:
  name          Change name

Output: JSON with archive path and manifest location.
Exit:   0 success, 1 error, 2 usage`,
}
