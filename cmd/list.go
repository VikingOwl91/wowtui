package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scanned addons from the registry",
		RunE:  runList,
	}
}

func runList(cmd *cobra.Command, _ []string) error {
	regPath, _ := cmd.Flags().GetString("registry")
	if regPath == "" {
		regPath = registry.DefaultPath()
	}
	reg, err := registry.Load(regPath)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if len(reg.Installations) == 0 {
		fmt.Fprintln(out, "no installations found, run wowtui scan first")
		return nil
	}

	for _, inst := range reg.Installations {
		label := inst.Flavor
		if label == "" {
			label = "unknown"
		}
		fmt.Fprintf(out, "%s  %s\n", label, inst.Path)

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, f := range inst.Folders {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", f.Name, f.Version, interfaceList(f.Interfaces))
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func interfaceList(nums []int) string {
	parts := make([]string, 0, len(nums))
	for _, n := range nums {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ",")
}
