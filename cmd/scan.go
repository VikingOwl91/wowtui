package cmd

import (
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/addon"
	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Discover WoW installations and scan installed addons",
		RunE:  runScan,
	}
}

func runScan(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	installs, err := wow.Discover(cfg.WoW.Paths)
	if err != nil {
		return err
	}

	reg := &registry.Registry{Version: 1}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, inst := range installs {
		folders, err := addon.Scan(inst.Path)
		if err != nil {
			return err
		}
		flavor := inst.Flavor
		if flavor == "" {
			flavor = inferFlavor(folders)
		}

		rec := registry.Installation{Path: inst.Path, Flavor: string(flavor)}
		for _, f := range folders {
			rec.Folders = append(rec.Folders, registry.Folder{
				Name:       f.Name,
				Version:    f.Version(),
				Interfaces: f.Interfaces(),
				TocFiles:   tocNames(f.Tocs),
			})
		}
		reg.Installations = append(reg.Installations, rec)

		label := string(flavor)
		if label == "" {
			label = "unknown"
		}
		fmt.Fprintf(w, "%s\t%s\t%d addons\n", label, inst.Path, len(folders))
	}
	if err := w.Flush(); err != nil {
		return err
	}

	regPath, _ := cmd.Flags().GetString("registry")
	if regPath == "" {
		regPath = registry.DefaultPath()
	}
	if err := reg.Save(regPath); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "registry: %s\n", regPath)
	return nil
}

func inferFlavor(folders []*addon.Folder) wow.GameFlavor {
	for _, f := range folders {
		for _, num := range f.Interfaces() {
			if flavor := wow.FlavorFromInterface(num); flavor != "" {
				return flavor
			}
		}
	}
	return ""
}

func tocNames(tocs []*addon.Toc) []string {
	names := make([]string, 0, len(tocs))
	for _, toc := range tocs {
		names = append(names, filepath.Base(toc.Path))
	}
	return names
}
