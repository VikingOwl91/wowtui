package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "wowtui",
		Short:         "A slim, safe WoW addon updater",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().String("config", "", "path to config file (default: XDG config path)")
	root.PersistentFlags().String("registry", "", "path to registry file (default: XDG data path)")
	root.AddCommand(newScanCmd(), newListCmd())
	return root
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
