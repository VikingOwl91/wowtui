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
	cmd.Flags().String("install", "", "select an installation path using the exact addon ref from check (e.g. WeakAuras/WeakAuras2)")
	cmd.Flags().String("replace-local", "", "replace the local changes identified by a single-addon review token")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	replace := cmd.Flags().Changed("replace-local")
	token, _ := cmd.Flags().GetString("replace-local")
	install, _ := cmd.Flags().GetString("install")
	if cmd.Flags().Changed("install") {
		if all {
			return errors.New("--install requires a single addon and cannot be used with --all")
		}
		if install == "" {
			return errors.New("--install requires an installation path")
		}
	}
	if replace && all {
		return errors.New("--replace-local requires a single addon and cannot be used with --all")
	}

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
	ctx := cmd.Context()
	if install == "" {
		err = svc.Update(ctx, args[0], dryRun || replace)
	} else {
		err = svc.UpdateInstalled(ctx, install, args[0], dryRun || replace)
	}
	var review *service.LocalChangesError
	if !errors.As(err, &review) {
		if err == nil && replace {
			return errors.New("the reviewed changes no longer exist; run update again without --replace-local")
		}
		return err
	}
	if replace && token == review.Token() {
		err = svc.UpdateReviewed(ctx, review, dryRun)
		if !errors.As(err, &review) {
			return err
		}
	}
	return fmt.Errorf("%w\nTo replace these files, repeat this single-addon update with --replace-local=%s", review, review.Token())
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
	var review *service.LocalChangesError
	if errors.As(err, &review) {
		return fmt.Errorf("%w\nRun update for each affected addon to review its local changes", err)
	}
	return err
}
