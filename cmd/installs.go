package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func newInstallsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "installs",
		Short: "List known WoW installations",
		RunE:  runInstalls,
	}
}

func runInstalls(cmd *cobra.Command, _ []string) error {
	reg, err := registry.Load(registryPath(cmd))
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if len(reg.Installations) == 0 {
		fmt.Fprintln(out, "no installations found, run wowtui scan first")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TITLE\tFLAVOR\tADDONS\tPATH")
	for _, inst := range reg.Installations {
		title := inst.Title
		if title == "" {
			title = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", title, inst.Flavor, len(inst.Folders), inst.Path)
	}
	return w.Flush()
}
