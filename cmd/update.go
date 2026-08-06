package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [addon]",
		Short: "Update one addon (by ref or repo name) or all managed addons",
		RunE:  runUpdate,
	}
	cmd.Flags().Bool("all", false, "update all managed addons")
	cmd.Flags().Bool("dry-run", false, "show what would be updated without installing")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	svc := newService(cmd, cfg)

	if all {
		return svc.UpdateAll(cmd.Context(), dryRun)
	}
	if len(args) != 1 {
		return errors.New("specify an addon (ref or repo name) or use --all")
	}
	return svc.Update(cmd.Context(), args[0], dryRun)
}
