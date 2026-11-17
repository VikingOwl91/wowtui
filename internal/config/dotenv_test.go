package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvLoadsQuotedValuesWithoutOverridingEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\nDOTENV_TEST_QUOTED=\"value$with$dollars\"\nDOTENV_TEST_SINGLE='single value'\nDOTENV_TEST_EXISTING=from-file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_QUOTED", "")
	t.Setenv("DOTENV_TEST_SINGLE", "")
	t.Setenv("DOTENV_TEST_EXISTING", "from-environment")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_TEST_QUOTED"); got != "" {
		t.Errorf("quoted value = %q, want existing empty environment value", got)
	}
	if got := os.Getenv("DOTENV_TEST_SINGLE"); got != "" {
		t.Errorf("single quoted value = %q, want existing empty environment value", got)
	}
	if got := os.Getenv("DOTENV_TEST_EXISTING"); got != "from-environment" {
		t.Errorf("existing value = %q", got)
	}
}

func TestLoadDotEnvLoadsMissingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "DOTENV_TEST_MISSING=\"value$with$dollars\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("DOTENV_TEST_MISSING")
	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_MISSING") })
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_TEST_MISSING"); got != "value$with$dollars" {
		t.Errorf("dotenv value = %q", got)
	}
}
