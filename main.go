package main

import (
	"fmt"
	"os"

	"somegit.dev/vikingowl/wowtui/cmd"
	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
)

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case worker.ProbeArgument:
			return
		case worker.Argument:
			if err := worker.Serve(os.Stdin, os.Stdout); err != nil {
				os.Exit(1)
			}
			return
		}
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "load .env:", err)
		os.Exit(1)
	}
	cmd.Execute()
}
