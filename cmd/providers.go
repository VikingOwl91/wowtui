package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
)

func newProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "List and configure addon providers",
		RunE:  runProviders,
	}
	cmd.AddCommand(newProviderToggleCmd("enable", true), newProviderToggleCmd("disable", false), newProviderOrderCmd())
	return cmd
}

func runProviders(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tENABLED\tIMPLEMENTATION\tPRIORITY")
	for i, name := range cfg.Providers.Order() {
		fmt.Fprintf(w, "%s\t%t\t%s\t%d\n", name, cfg.Providers.Enabled(name), cfg.Providers.State(name), i+1)
	}
	return w.Flush()
}

func newProviderToggleCmd(action string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <provider>",
		Args:  cobra.ExactArgs(1),
		Short: action + " a provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			if err := cfg.Providers.SetEnabled(args[0], enabled); err != nil {
				return err
			}
			return config.Save(configPath(cmd), cfg)
		},
	}
}

func newProviderOrderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "order <provider>...",
		Args:  cobra.MinimumNArgs(1),
		Short: "Set provider priority order",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			if err := cfg.Providers.SetOrder(args); err != nil {
				return err
			}
			return config.Save(configPath(cmd), cfg)
		},
	}
}

func configPath(cmd *cobra.Command) string {
	path, _ := cmd.Flags().GetString("config")
	if strings.TrimSpace(path) == "" {
		return config.DefaultPath()
	}
	return path
}
