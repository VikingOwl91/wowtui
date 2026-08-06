package config

import (
	"errors"
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
	GitHub     GitHub     `toml:"github"`
	Wago       Wago       `toml:"wago"`
	CurseForge CurseForge `toml:"curseforge"`
}

type GitHub struct {
	Token string `toml:"token"`
}

type Wago struct {
	APIKey string `toml:"api_key"`
}

type CurseForge struct {
	APIKey string `toml:"api_key"`
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
