package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	WoW       WoW       `toml:"wow"`
	Providers Providers `toml:"providers"`
}

type WoW struct {
	Paths []string `toml:"paths"`
}

type Providers struct {
	PriorityOrder []string       `toml:"order"`
	GitHub        GitHub         `toml:"github"`
	GitLab        ProviderConfig `toml:"gitlab"`
	Git           ProviderConfig `toml:"git"`
	WowUpHub      ProviderConfig `toml:"wowuphub"`
	Wago          Wago           `toml:"wago"`
	CurseForge    CurseForge     `toml:"curseforge"`
	Direct        ProviderConfig `toml:"direct"`
}

type GitHub struct {
	Enabled *bool  `toml:"enabled"`
	Token   string `toml:"token"`
}

type Wago struct {
	Enabled *bool  `toml:"enabled"`
	APIKey  string `toml:"api_key"`
}

type CurseForge struct {
	Enabled *bool  `toml:"enabled"`
	APIKey  string `toml:"api_key"`
}

type ProviderConfig struct {
	Enabled *bool  `toml:"enabled"`
	APIKey  string `toml:"api_key"`
}

var defaultProviderOrder = []string{"wowuphub", "wago", "curseforge", "github", "gitlab", "git", "direct"}
var retiredProviders = map[string]bool{"wago": true}
var pausedProviders = map[string]bool{"wowuphub": true, "curseforge": true}
var pausedProviderStates = map[string]string{"wowuphub": "pending-upstream", "curseforge": "pending-approval"}

func (p Providers) Enabled(name string) bool {
	if retiredProviders[name] || pausedProviders[name] {
		return false
	}
	var configured *bool
	switch name {
	case "github":
		configured = p.GitHub.Enabled
	case "wago":
		configured = p.Wago.Enabled
	case "curseforge":
		configured = p.CurseForge.Enabled
	case "gitlab":
		configured = p.GitLab.Enabled
	case "git":
		configured = p.Git.Enabled
	case "wowuphub":
		configured = p.WowUpHub.Enabled
	case "direct":
		configured = p.Direct.Enabled
	default:
		return false
	}
	if configured != nil {
		return *configured
	}
	return name == "github" || name == "gitlab" || name == "git" || name == "direct"
}

func (p Providers) Order() []string {
	seen := make(map[string]bool)
	order := make([]string, 0, len(defaultProviderOrder))
	for _, name := range append(p.PriorityOrder, defaultProviderOrder...) {
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}
	return order
}

func KnownProviders() []string {
	return append([]string(nil), defaultProviderOrder...)
}

func (p *Providers) SetEnabled(name string, enabled bool) error {
	if enabled && retiredProviders[name] {
		return fmt.Errorf("provider %q is retired", name)
	}
	if enabled && pausedProviders[name] {
		return fmt.Errorf("provider %q is paused pending upstream approval", name)
	}
	value := enabled
	switch name {
	case "github":
		p.GitHub.Enabled = &value
	case "gitlab":
		p.GitLab.Enabled = &value
	case "git":
		p.Git.Enabled = &value
	case "wowuphub":
		p.WowUpHub.Enabled = &value
	case "wago":
		p.Wago.Enabled = &value
	case "curseforge":
		p.CurseForge.Enabled = &value
	case "direct":
		p.Direct.Enabled = &value
	default:
		return fmt.Errorf("unknown provider %q", name)
	}
	return nil
}

func (p Providers) State(name string) string {
	if retiredProviders[name] {
		return "retired"
	}
	if pausedProviders[name] {
		return pausedProviderStates[name]
	}
	if name == "github" || name == "direct" || name == "gitlab" || name == "git" || name == "curseforge" {
		if p.Enabled(name) {
			return "enabled"
		}
		return "disabled"
	}
	if p.Enabled(name) {
		return "planned-enabled"
	}
	return "planned"
}

func (p *Providers) SetOrder(order []string) error {
	known := make(map[string]bool)
	for _, name := range defaultProviderOrder {
		known[name] = true
	}
	seen := make(map[string]bool)
	for _, name := range order {
		if !known[name] {
			return fmt.Errorf("unknown provider %q", name)
		}
		if seen[name] {
			return fmt.Errorf("provider %q appears more than once", name)
		}
		seen[name] = true
	}
	p.PriorityOrder = append([]string(nil), order...)
	return nil
}

func Load(path string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	applyEnv(cfg)
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("WOWTUI_GITHUB_TOKEN"); v != "" {
		cfg.Providers.GitHub.Token = v
	}
	if v := os.Getenv("WOWTUI_WAGO_API_KEY"); v != "" {
		cfg.Providers.Wago.APIKey = v
	}
	if v := os.Getenv("WOWTUI_CURSEFORGE_API_KEY"); v != "" {
		cfg.Providers.CurseForge.APIKey = v
	}
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wowtui", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "wowtui", "config.toml")
}
