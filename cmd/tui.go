package cmd

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/tui"
)

func runTUI(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	regPath := registryPath(cmd)
	deps := tui.Deps{
		Config:       cfg,
		Providers:    buildProviders(cfg),
		RegistryPath: regPath,
		BackupRoot:   filepath.Join(filepath.Dir(regPath), "backups"),
	}
	if cfg.Index.Configured() {
		deps.LoadImportIndex = func() (*index.Index, error) { return loadIndex(cfg.Index) }
	}
	p := tea.NewProgram(tui.New(deps), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
