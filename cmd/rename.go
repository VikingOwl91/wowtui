package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <install-path> <title>",
		Short: "Set a display title for an installation",
		Args:  cobra.ExactArgs(2),
		RunE:  runRename,
	}
}

func runRename(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	svc := newService(cmd, cfg)
	if err := svc.SetInstallTitle(args[0], args[1]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "renamed %s to %q\n", args[0], args[1])
	return nil
}
