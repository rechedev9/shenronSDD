package main

import (
	"os"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
