package main

import (
	"fmt"
	"os"

	"somegit.dev/vikingowl/wowtui/cmd"
	"somegit.dev/vikingowl/wowtui/internal/config"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "load .env:", err)
		os.Exit(1)
	}
	cmd.Execute()
}
