package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}
	if len(cfg.WoW.Paths) != 0 || cfg.Providers.GitHub.Token != "" {
		t.Errorf("expected zero config, got %+v", cfg)
	}
}

func TestLoadFullConfig(t *testing.T) {
	path := writeConfig(t, `
[wow]
paths = ["/games/wow/_retail_", "/games/wow/_classic_"]

[providers.github]
token = "gh-token"

[providers.wago]
api_key = "wago-key"

[providers.curseforge]
api_key = "cf-key"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WoW.Paths) != 2 || cfg.WoW.Paths[0] != "/games/wow/_retail_" {
		t.Errorf("wow.paths = %v", cfg.WoW.Paths)
	}
	if cfg.Providers.GitHub.Token != "gh-token" {
		t.Errorf("github token = %q", cfg.Providers.GitHub.Token)
	}
	if cfg.Providers.Wago.APIKey != "wago-key" {
		t.Errorf("wago key = %q", cfg.Providers.Wago.APIKey)
	}
	if cfg.Providers.CurseForge.APIKey != "cf-key" {
		t.Errorf("curseforge key = %q", cfg.Providers.CurseForge.APIKey)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	path := writeConfig(t, "this is [not toml")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestEnvOverridesFileValues(t *testing.T) {
	path := writeConfig(t, `
[providers.github]
token = "file-token"
`)
	t.Setenv("WOWTUI_GITHUB_TOKEN", "env-token")
	t.Setenv("WOWTUI_WAGO_API_KEY", "env-wago")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "env-cf")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers.GitHub.Token != "env-token" {
		t.Errorf("github token = %q, want env override", cfg.Providers.GitHub.Token)
	}
	if cfg.Providers.Wago.APIKey != "env-wago" {
		t.Errorf("wago key = %q, want env override", cfg.Providers.Wago.APIKey)
	}
	if cfg.Providers.CurseForge.APIKey != "env-cf" {
		t.Errorf("curseforge key = %q, want env override", cfg.Providers.CurseForge.APIKey)
	}
}

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	if got := DefaultPath(); got != "/custom/config/wowtui/config.toml" {
		t.Errorf("DefaultPath = %q", got)
	}
}

func TestDefaultPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	if got := DefaultPath(); got != "/tmp/fakehome/.config/wowtui/config.toml" {
		t.Errorf("DefaultPath = %q", got)
	}
}
