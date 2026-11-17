package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/service"
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
		return updateAll(cmd, svc, dryRun)
	}
	if len(args) != 1 {
		return errors.New("specify an addon (ref or repo name) or use --all")
	}
	return svc.Update(cmd.Context(), args[0], dryRun)
}

func updateAll(cmd *cobra.Command, svc *service.Service, dryRun bool) error {
	var total, current, failed int
	emit := svc.OnEvent
	svc.OnEvent = func(e service.Event) {
		emit(e)
		total = e.Total
		switch e.Stage {
		case service.StageCurrent:
			current++
		case service.StageError:
			failed++
		}
	}

	err := svc.UpdateAll(cmd.Context(), dryRun)
	if total > 0 {
		changed := total - current - failed
		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "summary: %d to update, %d current, %d failed (dry run)\n", changed, current, failed)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "summary: %d updated, %d current, %d failed\n", changed, current, failed)
		}
	}
	return err
}
