package cmd

import (
	"context"
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/service"
)

func newMapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "map",
		Short: "Show provider candidates for unmanaged installed addons",
		RunE:  runMap,
	}
	cmd.Flags().Bool("adoptable", false, "show only candidates whose provider is active")
	return cmd
}

func runMap(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	adoptable, _ := cmd.Flags().GetBool("adoptable")
	svc := newService(cmd, cfg)
	if err := attachIndex(cmd, cfg, svc, false); err != nil {
		return err
	}
	candidates, err := svc.MapUnmanaged(func(name string) service.Availability {
		return inactiveAvailability(cfg.Providers, name)
	})
	if err != nil {
		return err
	}
	if adoptable {
		candidates = slices.DeleteFunc(candidates, func(c service.MappingCandidate) bool {
			return c.Availability != service.AvailabilityActive
		})
		if len(candidates) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no adoptable provider candidates found")
			return nil
		}
	}
	if len(candidates) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no provider candidates found")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FOLDER\tFLAVOR\tPROVIDER\tAVAILABILITY\tREFERENCE\tREASON\tINSTALL")
	for _, candidate := range candidates {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", candidate.Folder, candidate.Flavor, candidate.Provider, candidate.Availability, candidate.Reference, candidate.Reason, candidate.InstallPath)
	}
	return w.Flush()
}

func inactiveAvailability(providers config.Providers, name string) service.Availability {
	if !slices.Contains(config.KnownProviders(), name) {
		return service.AvailabilityUnsupported
	}
	switch providers.State(name) {
	case "pending-upstream", "pending-approval":
		return service.AvailabilityPaused
	case "requires-api-key":
		return service.AvailabilityNeedsAPIKey
	case "disabled":
		return service.AvailabilityDisabled
	}
	return service.AvailabilityUnsupported
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
			if err := attachIndex(cmd, cfg, svc, ref.Provider == "index"); err != nil {
				return err
			}
			if err := svc.Adopt(context.Background(), args[0], args[1], ref); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "adopted %s from %s\n", args[1], provider.FormatReference(ref))
			return nil
		},
	}
}
