package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Discover WoW installations and scan installed addons",
		RunE:  runScan,
	}
}

func runScan(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	svc := newService(cmd, cfg)
	if _, err := svc.Scan(cmd.Context(), cfg.WoW.Paths); err != nil {
		return err
	}

	regPath := registryPath(cmd)
	fmt.Fprintf(cmd.OutOrStdout(), "registry: %s\n", regPath)
	return nil
}
