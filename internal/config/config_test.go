package config

import (
	"os"
	"path/filepath"
	"strings"
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
	clearCredentialEnv(t)
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

func TestEnvCredentialsApplyWithoutConfigFile(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-token")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "dummy-key")
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers.GitHub.Token != "dummy-token" {
		t.Errorf("github token = %q, want env value", cfg.Providers.GitHub.Token)
	}
	if cfg.Providers.CurseForge.APIKey != "dummy-key" {
		t.Errorf("curseforge key = %q, want env value", cfg.Providers.CurseForge.APIKey)
	}
	if got := cfg.Providers.State("curseforge"); got != "enabled" {
		t.Errorf("curseforge state = %q, want enabled", got)
	}
}

func TestSaveWithoutConfigFileDoesNotPersistEnvCredentials(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-token")
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Providers.SetEnabled("gitlab", true); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if raw := readConfigFile(t, path); strings.Contains(raw, "dummy-token") {
		t.Error("config file contains env credential")
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

func clearCredentialEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WOWTUI_GITHUB_TOKEN", "")
	t.Setenv("WOWTUI_WAGO_API_KEY", "")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "")
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSaveDoesNotPersistEnvCredentials(t *testing.T) {
	path := writeConfig(t, "[providers.gitlab]\nenabled = false\n")
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-env-token")
	t.Setenv("WOWTUI_WAGO_API_KEY", "dummy-env-wago")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "dummy-env-cf")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Providers.SetEnabled("gitlab", true); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw := readConfigFile(t, path)
	for _, secret := range []string{"dummy-env-token", "dummy-env-wago", "dummy-env-cf"} {
		if strings.Contains(raw, secret) {
			t.Errorf("config file contains env credential %q", secret)
		}
	}
	if cfg.Providers.GitHub.Token != "dummy-env-token" {
		t.Errorf("in-memory github token = %q, want env value kept", cfg.Providers.GitHub.Token)
	}

	clearCredentialEnv(t)
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Providers.Enabled("gitlab") {
		t.Error("gitlab should stay enabled after save")
	}
	if loaded.Providers.GitHub.Token != "" || loaded.Providers.Wago.APIKey != "" || loaded.Providers.CurseForge.APIKey != "" {
		t.Errorf("saved credentials = %q, %q, %q, want none", loaded.Providers.GitHub.Token, loaded.Providers.Wago.APIKey, loaded.Providers.CurseForge.APIKey)
	}
}

func TestSaveKeepsFileCredentials(t *testing.T) {
	path := writeConfig(t, `
[providers.github]
token = "dummy-file-token"

[providers.curseforge]
api_key = "dummy-file-cf"
`)
	clearCredentialEnv(t)
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-env-token")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "dummy-env-cf")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Providers.SetOrder([]string{"gitlab"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw := readConfigFile(t, path)
	for _, secret := range []string{"dummy-env-token", "dummy-env-cf"} {
		if strings.Contains(raw, secret) {
			t.Errorf("config file contains env credential %q", secret)
		}
	}

	clearCredentialEnv(t)
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Providers.GitHub.Token != "dummy-file-token" {
		t.Errorf("github token = %q, want file value", loaded.Providers.GitHub.Token)
	}
	if loaded.Providers.CurseForge.APIKey != "dummy-file-cf" {
		t.Errorf("curseforge key = %q, want file value", loaded.Providers.CurseForge.APIKey)
	}
	if loaded.Providers.Order()[0] != "gitlab" {
		t.Errorf("order = %v", loaded.Providers.Order())
	}
}

func TestSavePersistsCredentialSetByCaller(t *testing.T) {
	path := writeConfig(t, "")
	clearCredentialEnv(t)
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-env-token")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers.GitHub.Token = "dummy-new-token"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	clearCredentialEnv(t)
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Providers.GitHub.Token != "dummy-new-token" {
		t.Errorf("github token = %q, want caller value", loaded.Providers.GitHub.Token)
	}
}

func TestIndexIsOffWithoutSection(t *testing.T) {
	clearCredentialEnv(t)
	for name, path := range map[string]string{
		"missing file": filepath.Join(t.TempDir(), "nope.toml"),
		"no section":   writeConfig(t, "[wow]\npaths = []\n"),
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Index.Configured() {
				t.Errorf("index = %+v, want not configured", cfg.Index)
			}
		})
	}
}

func TestLoadIndexSection(t *testing.T) {
	clearCredentialEnv(t)
	path := writeConfig(t, `
[index]
path = "/data/index.json"
public_key = "dummy-public-key"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Index.Path != "/data/index.json" || cfg.Index.PublicKey != "dummy-public-key" {
		t.Errorf("index = %+v", cfg.Index)
	}
	if !cfg.Index.Configured() {
		t.Error("index with path and key should be configured")
	}
}

func TestIndexWithOneFieldCountsAsConfigured(t *testing.T) {
	for _, index := range []Index{{Path: "/data/index.json"}, {PublicKey: "dummy-public-key"}} {
		if !index.Configured() {
			t.Errorf("%+v should count as configured so a load reports the missing field", index)
		}
	}
}

func TestSaveIndexSectionKeepsEnvCredentialsOut(t *testing.T) {
	clearCredentialEnv(t)
	path := writeConfig(t, "[wow]\npaths = [\"/games/wow\"]\n")
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-env-token")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Index = Index{Path: "/data/index.json", PublicKey: "dummy-public-key"}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if raw := readConfigFile(t, path); strings.Contains(raw, "dummy-env-token") {
		t.Errorf("config file contains env credential:\n%s", raw)
	}

	clearCredentialEnv(t)
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Index != cfg.Index {
		t.Errorf("index = %+v, want %+v", loaded.Index, cfg.Index)
	}
	if len(loaded.WoW.Paths) != 1 || loaded.WoW.Paths[0] != "/games/wow" {
		t.Errorf("wow.paths = %v, want kept", loaded.WoW.Paths)
	}
}

func TestSaveWithoutIndexWritesNoIndexSection(t *testing.T) {
	clearCredentialEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	if raw := readConfigFile(t, path); strings.Contains(raw, "[index]") {
		t.Errorf("config without index has an index section:\n%s", raw)
	}
}
