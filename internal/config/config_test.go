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

func TestProviderOrderAndDefaults(t *testing.T) {
	path := writeConfig(t, `
[providers]
order = ["github", "wago"]

[providers.github]
enabled = false
token = "token"

[providers.wago]
enabled = true
api_key = "wago"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers.Enabled("github") {
		t.Error("github should be disabled explicitly")
	}
	if cfg.Providers.Enabled("wago") || !cfg.Providers.Enabled("direct") {
		t.Error("wago should be retired and direct should be enabled")
	}
	if cfg.Providers.State("wago") != "pending-approval" {
		t.Errorf("wago state = %q", cfg.Providers.State("wago"))
	}
	if cfg.Providers.Enabled("curseforge") {
		t.Error("curseforge should be disabled without an API key")
	}
	if cfg.Providers.State("curseforge") != "requires-api-key" {
		t.Errorf("curseforge state = %q", cfg.Providers.State("curseforge"))
	}
	order := cfg.Providers.Order()
	if order[0] != "github" || order[1] != "wago" {
		t.Errorf("order = %v", order)
	}
}

func TestCurseForgeIsEnabledByAPIKey(t *testing.T) {
	path := writeConfig(t, `
[providers.curseforge]
enabled = false
api_key = "cf-key"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Providers.Enabled("curseforge") {
		t.Error("curseforge should be enabled when an API key is configured")
	}
	if cfg.Providers.State("curseforge") != "enabled" {
		t.Errorf("curseforge state = %q", cfg.Providers.State("curseforge"))
	}
	if err := cfg.Providers.SetEnabled("curseforge", true); err == nil {
		t.Error("expected key-controlled CurseForge toggle to be rejected")
	}
}

func TestSaveProviderSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	cfg := &Config{}
	if err := cfg.Providers.SetEnabled("gitlab", true); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Providers.SetOrder([]string{"gitlab", "github", "direct"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Providers.Enabled("gitlab") || loaded.Providers.Order()[0] != "gitlab" {
		t.Errorf("provider settings = %+v", loaded.Providers)
	}
}
