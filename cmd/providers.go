package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
)

func newProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "List and configure addon providers",
		RunE:  runProviders,
	}
	cmd.AddCommand(newProviderToggleCmd("enable", true), newProviderToggleCmd("disable", false), newProviderOrderCmd())
	cmd.AddCommand(newProviderTestCmd())
	return cmd
}

func newProviderTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <provider>",
		Args:  cobra.ExactArgs(1),
		Short: "Test provider authentication without printing credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			providers := buildProviders(cfg)
			p, ok := providers[args[0]]
			if !ok {
				return fmt.Errorf("provider %q is %s and cannot be tested", args[0], cfg.Providers.State(args[0]))
			}
			auth, ok := p.(provider.Authenticator)
			if !ok {
				return fmt.Errorf("provider %q does not expose an authentication check", args[0])
			}
			if err := auth.Authenticate(context.Background()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s authentication succeeded\n", args[0])
			return nil
		},
	}
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
