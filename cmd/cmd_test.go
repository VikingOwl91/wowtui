package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func setupFakeInstall(t *testing.T) (home, cfgPath, dataDir string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	dataDir = t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)

	addonDir := filepath.Join(home, "wow", "_retail_", "Interface", "AddOns", "WeakAuras")
	if err := os.MkdirAll(addonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toc := "## Title: WeakAuras\n## Version: 5.18.1\n## Interface: 110200\n"
	if err := os.WriteFile(filepath.Join(addonDir, "WeakAuras.toc"), []byte(toc), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgPath = filepath.Join(home, "config.toml")
	content := fmt.Sprintf("[wow]\npaths = [%q]\n", filepath.Join(home, "wow", "_retail_"))
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, cfgPath, dataDir
}

func runCmd(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v\noutput: %s", args, err, buf.String())
	}
	return buf.String()
}

func TestScanWritesRegistry(t *testing.T) {
	_, cfgPath, dataDir := setupFakeInstall(t)

	out := runCmd(t, "scan", "--config", cfgPath)
	if !strings.Contains(out, "retail") {
		t.Errorf("scan output missing flavor: %s", out)
	}

	regPath := filepath.Join(dataDir, "wowtui", "registry.json")
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Installations) != 1 {
		t.Fatalf("got %d installations, want 1", len(reg.Installations))
	}
	inst := reg.Installations[0]
	if inst.Flavor != "retail" {
		t.Errorf("flavor = %q", inst.Flavor)
	}
	if len(inst.Folders) != 1 || inst.Folders[0].Name != "WeakAuras" || inst.Folders[0].Version != "5.18.1" {
		t.Errorf("folders = %+v", inst.Folders)
	}
}

func TestListShowsScannedAddons(t *testing.T) {
	_, cfgPath, dataDir := setupFakeInstall(t)
	runCmd(t, "scan", "--config", cfgPath)

	regPath := filepath.Join(dataDir, "wowtui", "registry.json")
	out := runCmd(t, "list", "--registry", regPath)
	if !strings.Contains(out, "WeakAuras") || !strings.Contains(out, "5.18.1") {
		t.Errorf("list output missing addon info: %s", out)
	}
}

func TestListEmptyRegistry(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	out := runCmd(t, "list", "--registry", filepath.Join(t.TempDir(), "registry.json"))
	if !strings.Contains(out, "scan") {
		t.Errorf("expected hint to run scan, got: %s", out)
	}
}

func TestScanMissingConfiguredPathFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(home, "config.toml")
	content := fmt.Sprintf("[wow]\npaths = [%q]\n", filepath.Join(home, "nope"))
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"scan", "--config", cfgPath})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for missing configured path")
	}
}
