package main

import (
	"os"

	"github.com/magicrew/doc7/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
