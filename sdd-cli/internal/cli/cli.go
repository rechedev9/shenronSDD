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
	case "dump":
		return runDump(rest, stdout, stderr)
	case "doctor":
		return runDoctor(rest, stdout, stderr)
	case "errors":
		return runErrors(rest, stdout, stderr)
	case "dashboard":
		return runDashboard(rest, stdout, stderr)
	case "quickstart":
		return runQuickstart(rest, stdout, stderr)
	case "completion":
		return runCompletion(rest, stdout, stderr)
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
	fmt.Fprintln(w, "  dump <name>       Dump full debug state as JSON")
	fmt.Fprintln(w, "  errors            List recorded verify failures, grouped by pattern")
	fmt.Fprintln(w, "  doctor            Diagnose config, cache, skills, and tools")
	fmt.Fprintln(w, "  quickstart        Skip planning phases — jump to apply with existing spec")
	fmt.Fprintln(w, "  dashboard         Live ops dashboard on localhost")
	fmt.Fprintln(w, "  completion <sh>   Generate shell completions (bash, zsh, fish)")
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

Usage: sdd new <name> "<description>" [--json]

Creates openspec/changes/<name>/ with initial state.json, then runs
the explore context assembler and prints it to stdout.

Arguments:
  name          Change name (kebab-case)
  description   Brief intent description

Flags:
  --json        Output JSON envelope instead of explore context

Output: Explore context to stdout (SKILL + project info + file tree).
        With --json: JSON with change name, description, change_dir, current_phase.
Exit:   0 success, 1 error, 2 usage`,

	"context": `sdd context — Assemble phase context

Usage: sdd context <name> [phase] [--json]

Loads the SKILL.md for the phase, relevant artifacts, and source context,
then prints the assembled context to stdout. If phase is omitted, uses
the current phase from state.json.

Arguments:
  name          Change name
  phase         Optional: explore, propose, spec, design, tasks, apply, review, clean

Flags:
  --json        Wrap assembled context in JSON envelope with byte/token counts

Output: Assembled context to stdout.
        With --json: JSON with context string, bytes, tokens.
Exit:   0 success, 1 error, 2 usage`,

	"write": `sdd write — Promote artifact and advance state

Usage: sdd write <name> <phase> [--force]

Moves .pending/<phase>.md to its final location and advances the state
machine. Claude writes to .pending/, Go promotes and tracks state.

Before promotion, content is validated against phase-specific rules
(required sections, file:line references for reviews, task checkboxes).
Use --force to bypass validation.

The state machine prevents re-promoting an already-completed phase.

Arguments:
  name          Change name
  phase         Phase to promote (explore, propose, spec, design, tasks, apply, review, verify, clean)

Flags:
  --force, -f   Skip artifact content validation

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

Usage: sdd verify <name> [--force]

Runs build, lint, and test commands from config.yaml sequentially.
Stops on first failure. Writes verify-report.md to the change directory.

If the same error pattern has failed 3+ times historically for this
change, verify warns and exits instead of retrying blindly. Use --force
to override.

This is a zero-token operation — runs entirely in Go.

Arguments:
  name          Change name

Flags:
  --force, -f   Skip recurring failure check and run anyway

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

	"dump": `sdd dump — Debug state snapshot

Usage: sdd dump <name>

Outputs full workflow state as indented JSON: state machine, config,
artifact inventory, cache hashes, and pipeline metrics.

Arguments:
  name          Change name

Output: JSON debug snapshot.
Exit:   0 success, 1 error, 2 usage`,

	"doctor": `sdd doctor — Diagnostic health check

Usage: sdd doctor [--json]

Validates project setup: config syntax and version, cache integrity,
orphaned .pending files, skills path, and build tool availability.

Flags:
  --json        Output results as JSON

Output: Aligned table (default) or JSON with per-check pass/warn/fail status.
Exit:   0 all checks pass or warn, 1 any check fails, 2 usage`,

	"errors": `sdd errors — List recorded verify failures

Usage: sdd errors [--json]

Reads the global error log (openspec/.cache/errors.json) and displays
verify failures grouped by error fingerprint. Shows recurrence counts.

Flags:
  --json        Output grouped errors as JSON

Output: Human-readable table (default) or JSON with error groups.
Exit:   0 success, 1 error, 2 usage`,

	"completion": `sdd completion — Generate shell completions

Usage: sdd completion <bash|zsh|fish>

Outputs a shell completion script to stdout.

Setup:
  bash    eval "$(sdd completion bash)"    # add to ~/.bashrc
  zsh     eval "$(sdd completion zsh)"     # add to ~/.zshrc
  fish    sdd completion fish > ~/.config/fish/completions/sdd.fish

Exit:   0 success, 2 usage`,

	"dashboard": `sdd dashboard — Live ops dashboard

Usage: sdd dashboard [--port PORT]

Starts a local HTTP server serving a live ops dashboard with KPI cards,
pipeline progress, and error log. Polls the SQLite store for telemetry.

Flags:
  --port, -p   Port to listen on (default: 8811, range: 1024-65535)

Output: JSON with command, status, and URL. Server runs until interrupted.
Exit:   0 success (after shutdown), 1 error, 2 usage`,

	"quickstart": `sdd quickstart — Skip planning, jump to apply

Usage: sdd quickstart <name> "<description>" --spec <path>

Creates a new change with explore, propose, spec, design, and tasks
phases pre-completed using the provided spec file. The change starts
at the apply phase, ready for implementation.

Use this when you already have a reviewed design spec and don't need
the LLM to run through planning phases.

Arguments:
  name          Change name (kebab-case)
  description   Brief intent description

Flags:
  --spec <path> Path to an existing design/spec document (required)

Output: JSON with change info, skipped phases, and current phase (apply).
Exit:   0 success, 1 error, 2 usage`,

	"archive": `sdd archive — Archive completed change

Usage: sdd archive <name> [--force]

Moves the change directory to openspec/changes/archive/<timestamp>-<name>/
and writes archive-manifest.md listing all preserved artifacts.

Requires all prerequisite phases (through clean) to be completed.
Use --force to archive even when prerequisites are not met.

This is a zero-token operation — runs entirely in Go.

Arguments:
  name          Change name

Flags:
  --force, -f   Skip prerequisite check and archive anyway

Output: JSON with archive path and manifest location.
Exit:   0 success, 1 error, 2 usage`,
}
