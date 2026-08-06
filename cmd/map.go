package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/provider"
)

func newMapCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "map",
		Short: "Show provider candidates for unmanaged installed addons",
		RunE:  runMap,
	}
}

func runMap(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	svc := newService(cmd, cfg)
	candidates, err := svc.MapUnmanaged()
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no provider candidates found")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FOLDER\tFLAVOR\tPROVIDER\tREFERENCE\tREASON\tINSTALL")
	for _, candidate := range candidates {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", candidate.Folder, candidate.Flavor, candidate.Provider, candidate.Reference, candidate.Reason, candidate.InstallPath)
	}
	return w.Flush()
}

func newAdoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <install-path> <folder> <reference>",
		Short: "Adopt an installed addon under an active provider",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			ref, err := provider.ParseReference(args[2])
			if err != nil {
				return err
			}
			svc := newService(cmd, cfg)
			if err := svc.Adopt(context.Background(), args[0], args[1], ref); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "adopted %s from %s\n", args[1], ref.ID)
			return nil
		},
	}
}
