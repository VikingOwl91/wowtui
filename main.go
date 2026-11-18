package main

import (
	"fmt"
	"os"

	"somegit.dev/vikingowl/wowtui/cmd"
	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
)

func main() {
	if handled, code := worker.Dispatch(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "load .env:", err)
		os.Exit(1)
	}
	cmd.Execute()
}
