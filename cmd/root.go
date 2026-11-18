package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/service"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "wowtui",
		Short:         "A slim, safe WoW addon updater",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE:          runTUI,
	}
	root.PersistentFlags().String("config", "", "path to config file (default: XDG config path)")
	root.PersistentFlags().String("registry", "", "path to registry file (default: XDG data path)")
	root.AddCommand(newScanCmd(), newListCmd(), newAddCmd(), newCheckCmd(), newUpdateCmd(), newInstallsCmd(), newRenameCmd(), newProvidersCmd(), newMapCmd(), newAdoptCmd(), newIndexCmd())
	return root
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	return config.Load(cfgPath)
}

func registryPath(cmd *cobra.Command) string {
	regPath, _ := cmd.Flags().GetString("registry")
	if regPath == "" {
		regPath = registry.DefaultPath()
	}
	return regPath
}

func httpCacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "wowtui", "http")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "wowtui", "http")
}

func buildProviders(cfg *config.Config) map[string]provider.Provider {
	gh := provider.NewGitHub(cfg.Providers.GitHub.Token)
	if base := os.Getenv("WOWTUI_GITHUB_API"); base != "" {
		gh.SetAPIBase(base)
	}
	gh.SetCacheDir(httpCacheDir())
	hub := provider.NewWowUpHub()
	if base := os.Getenv("WOWTUI_WOWUPHUB_API"); base != "" {
		hub.SetBaseURL(base)
	}
	curseForge := provider.NewCurseForge(cfg.Providers.CurseForge.APIKey)
	all := map[string]provider.Provider{
		"github":     gh,
		"direct":     provider.NewDirect(),
		"gitlab":     provider.NewGitLab(cfg.Providers.GitLab.APIKey),
		"git":        provider.NewGit(),
		"wowuphub":   hub,
		"curseforge": curseForge,
	}
	active := make(map[string]provider.Provider)
	for name, p := range all {
		if cfg.Providers.Enabled(name) {
			active[name] = p
		}
	}
	return active
}

func newService(cmd *cobra.Command, cfg *config.Config) *service.Service {
	regPath := registryPath(cmd)
	out := cmd.OutOrStdout()
	return &service.Service{
		Providers:    buildProviders(cfg),
		RegistryPath: regPath,
		BackupRoot:   filepath.Join(filepath.Dir(regPath), "backups"),
		OnEvent: func(e service.Event) {
			if e.Message != "" {
				fmt.Fprintln(out, e.Message)
			}
		},
	}
}
