package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			ts := time.Now()
			name := fmt.Sprintf(".sdd-crash-%d.log", ts.Unix())
			content := fmt.Sprintf("sdd crash report\ntimestamp: %s\nargs: %s\npanic: %v\n\nstack trace:\n%s",
				ts.Format(time.RFC3339),
				strings.Join(os.Args, " "),
				r,
				debug.Stack(),
			)
			if err := os.WriteFile(name, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "sdd: panic recovered; failed to write crash log: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "sdd: panic recovered; crash log written to %s\n", name)
			}
			os.Exit(3)
		}
	}()

	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
