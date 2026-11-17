package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDotEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoadDotEnvDoesNotOverrideEnvironment(t *testing.T) {
	path := writeDotEnv(t, "# comment\nWOWTUI_GITHUB_TOKEN=\"dummy-file-token\"\nWOWTUI_CURSEFORGE_API_KEY=dummy-file-key\n")
	t.Setenv("WOWTUI_GITHUB_TOKEN", "")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "dummy-env-key")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("WOWTUI_GITHUB_TOKEN"); got != "" {
		t.Errorf("github token = %q, want existing empty environment value", got)
	}
	if got := os.Getenv("WOWTUI_CURSEFORGE_API_KEY"); got != "dummy-env-key" {
		t.Errorf("curseforge key = %q, want environment value", got)
	}
}

func TestLoadDotEnvLoadsCredentialVariables(t *testing.T) {
	path := writeDotEnv(t, "WOWTUI_GITHUB_TOKEN=\"dummy$token\"\nexport WOWTUI_WAGO_API_KEY='dummy wago key'\nWOWTUI_CURSEFORGE_API_KEY=dummy-key\n")
	unsetEnv(t, "WOWTUI_GITHUB_TOKEN", "WOWTUI_WAGO_API_KEY", "WOWTUI_CURSEFORGE_API_KEY")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("WOWTUI_GITHUB_TOKEN"); got != "dummy$token" {
		t.Errorf("github token = %q", got)
	}
	if got := os.Getenv("WOWTUI_WAGO_API_KEY"); got != "dummy wago key" {
		t.Errorf("wago key = %q", got)
	}
	if got := os.Getenv("WOWTUI_CURSEFORGE_API_KEY"); got != "dummy-key" {
		t.Errorf("curseforge key = %q", got)
	}
}

func TestLoadDotEnvIgnoresNonCredentialVariables(t *testing.T) {
	path := writeDotEnv(t, "WOWTUI_GITHUB_API=http://127.0.0.1:18765\nWOWTUI_WOWUPHUB_API=http://127.0.0.1:18766\nDOTENV_TEST_OTHER=value\nWOWTUI_GITHUB_TOKEN=dummy-token\n")
	unsetEnv(t, "WOWTUI_GITHUB_API", "WOWTUI_WOWUPHUB_API", "DOTENV_TEST_OTHER", "WOWTUI_GITHUB_TOKEN")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"WOWTUI_GITHUB_API", "WOWTUI_WOWUPHUB_API", "DOTENV_TEST_OTHER"} {
		if value, ok := os.LookupEnv(key); ok {
			t.Errorf("%s = %q, want unset", key, value)
		}
	}
	if got := os.Getenv("WOWTUI_GITHUB_TOKEN"); got != "dummy-token" {
		t.Errorf("github token = %q", got)
	}
}
