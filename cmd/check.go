package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check managed addons for available updates",
		RunE:  runCheck,
	}
}

func runCheck(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	svc := newService(cmd, cfg)
	statuses, err := svc.Check(cmd.Context())
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if len(statuses) == 0 {
		fmt.Fprintln(out, "no managed addons, install one with wowtui add")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ADDON\tINSTALLED\tLATEST\tSTATUS")
	for _, status := range statuses {
		latest := shortVersion(status.Latest.Tag)
		state := "current"
		switch {
		case status.Error != nil:
			state = "error: " + status.Error.Error()
			latest = "-"
		case status.HasUpdate:
			state = "update"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", status.Addon.Ref, shortVersion(status.Addon.Version), latest, state)
	}
	return w.Flush()
}

func shortVersion(v string) string {
	if len(v) > 12 && isHex(v) {
		return v[:12]
	}
	return v
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
