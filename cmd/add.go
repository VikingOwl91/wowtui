package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <reference>",
		Short: "Install an addon from a provider (github:Owner/Repo, GitHub URL, or direct:https URL)",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdd,
	}
	cmd.Flags().String("flavor", "retail", "target flavor (retail, classic, classic-era, beta, ptr)")
	cmd.Flags().String("install", "", "explicit install path (overrides flavor selection)")
	return cmd
}

func runAdd(cmd *cobra.Command, args []string) error {
	ref, err := provider.ParseReference(args[0])
	if err != nil {
		return err
	}

	flavorStr, _ := cmd.Flags().GetString("flavor")
	flavor := wow.GameFlavor(flavorStr)
	if !knownFlavor(flavor) {
		return fmt.Errorf("unknown flavor %q (retail, classic, classic-era, beta, ptr)", flavorStr)
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	install, err := pickInstall(cmd, cfg, flavor)
	if err != nil {
		return err
	}

	svc := newService(cmd, cfg)
	return svc.Add(cmd.Context(), ref, flavor, install)
}

func knownFlavor(flavor wow.GameFlavor) bool {
	switch flavor {
	case wow.Retail, wow.Classic, wow.ClassicEra, wow.Beta, wow.PTR:
		return true
	}
	return false
}

func pickInstall(cmd *cobra.Command, cfg *config.Config, flavor wow.GameFlavor) (wow.Installation, error) {
	explicit, _ := cmd.Flags().GetString("install")

	installs, err := wow.Discover(cfg.WoW.Paths)
	if err != nil {
		return wow.Installation{}, err
	}

	if explicit != "" {
		for _, inst := range installs {
			if inst.Path == explicit {
				return inst, nil
			}
		}
		return wow.Installation{Path: explicit, Flavor: flavor}, nil
	}

	var matches []wow.Installation
	for _, inst := range installs {
		if inst.Flavor == flavor {
			matches = append(matches, inst)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return wow.Installation{}, fmt.Errorf("no %s installation found, add paths to [wow] in your config", flavor)
	default:
		paths := make([]string, 0, len(matches))
		for _, inst := range matches {
			paths = append(paths, inst.Path)
		}
		return wow.Installation{}, fmt.Errorf("multiple %s installations found, choose one with --install: %s",
			flavor, strings.Join(paths, ", "))
	}
}
