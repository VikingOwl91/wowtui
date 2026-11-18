package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestUpdateRequiresCurrentReviewToken(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfg, install := setupAddEnv(t, gh)
	runCmd(t, "add", "github:Owner/TestAddon", "--config", cfg)
	toc := filepath.Join(install, "Interface", "AddOns", "TestAddon", "TestAddon.toc")
	edit := func(content string) {
		t.Helper()
		if err := os.WriteFile(toc, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	edit("first edit")
	review := func(flags ...string) string {
		t.Helper()
		_, err := executeCmd(append([]string{"update", "TestAddon", "--config", cfg}, flags...)...)
		if err == nil || !strings.Contains(err.Error(), "TestAddon/TestAddon.toc") {
			t.Fatalf("expected a file review, got %v", err)
		}
		token := regexp.MustCompile(`--replace-local=[a-f0-9]+`).FindString(err.Error())
		if token == "" {
			t.Fatalf("missing replacement command in %v", err)
		}
		return token
	}
	first := review()
	edit("second edit")
	second := review(first)
	if first == second {
		t.Fatal("changed bytes reused an earlier approval")
	}
	assertTocFile(t, toc, "second edit")
	if _, err := executeCmd("update", "--all", second, "--config", cfg); err == nil || !strings.Contains(err.Error(), "--all") {
		t.Fatalf("bulk replacement should reject the flag: %v", err)
	}
	assertTocFile(t, toc, "second edit")
	runCmd(t, "update", "TestAddon", second, "--config", cfg)
	assertTocFile(t, toc, "## Version: 1.0.0")
}

func TestUpdateSelectsInstallationForDuplicateRefs(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfg, retail := setupAddEnv(t, gh)
	classic := filepath.Join(filepath.Dir(retail), "_classic_era_")
	if err := os.MkdirAll(filepath.Join(classic, "Interface", "AddOns"), 0755); err != nil {
		t.Fatal(err)
	}
	const ref = "Owner/TestAddon"
	runCmd(t, "add", "github:"+ref, "--config", cfg, "--install", retail)
	runCmd(t, "add", "github:"+ref, "--config", cfg, "--install", classic, "--flavor", "classic-era")
	toc := func(path string) string {
		return filepath.Join(path, "Interface", "AddOns", "TestAddon", "TestAddon.toc")
	}
	for _, path := range []string{classic, retail} {
		if err := os.WriteFile(toc(path), []byte("local edits"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"update", ref, "--config", cfg, "--install", classic}
	_, err := executeCmd(args...)
	if err == nil || !strings.Contains(err.Error(), classic) {
		t.Fatalf("expected review for the selected installation, got %v", err)
	}
	token := regexp.MustCompile(`--replace-local=[a-f0-9]+`).FindString(err.Error())
	if token == "" {
		t.Fatalf("missing review token in %v", err)
	}
	if _, err := executeCmd("update", "--all", "--install", classic, "--config", cfg); err == nil || !strings.Contains(err.Error(), "--all") {
		t.Fatalf("bulk update accepted an installation: %v", err)
	}
	if _, err := executeCmd("update", ref, "--install", retail, token, "--config", cfg); err == nil {
		t.Fatal("token from Classic Era authorized Retail")
	}
	runCmd(t, append(args, token)...)
	assertTocFile(t, toc(classic), "## Version: 1.0.0")
	assertTocFile(t, toc(retail), "local edits")
}
