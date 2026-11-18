package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <reference>",
		Short: "Install an addon from a provider (github:Owner/Repo, GitHub URL, direct:https URL, or index:<id>)",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdd,
	}
	cmd.Flags().String("flavor", "retail", "target flavor ("+flavorList()+")")
	cmd.Flags().String("install", "", "explicit install path (overrides flavor selection)")
	cmd.Flags().String("source", "", "pin update source: release, tag, or branch (default: auto fallback chain)")
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
		return fmt.Errorf("unknown flavor %q (%s)", flavorStr, flavorList())
	}

	source, _ := cmd.Flags().GetString("source")
	switch source {
	case "", "release", "tag", "branch":
	default:
		return fmt.Errorf("unknown source %q (release, tag, branch)", source)
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	svc := newService(cmd, cfg)
	if err := attachIndex(cmd, cfg, svc, ref.Provider == "index"); err != nil {
		return err
	}

	install, err := pickInstall(cmd, cfg, flavor)
	if err != nil {
		return err
	}

	return svc.Add(cmd.Context(), ref, flavor, install, source)
}

func knownFlavor(flavor wow.GameFlavor) bool {
	return slices.Contains(wow.Flavors(), flavor)
}

func flavorList() string {
	flavors := wow.Flavors()
	names := make([]string, 0, len(flavors))
	for _, flavor := range flavors {
		names = append(names, string(flavor))
	}
	return strings.Join(names, ", ")
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
