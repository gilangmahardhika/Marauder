package main

import (
	"fmt"
	"os"

	"orchestrator/backend/internal/cli"
	"orchestrator/backend/internal/envfile"
)

func main() {
	if err := envfile.Load(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(cli.Execute(cli.Options{Args: os.Args[1:]}))
}
