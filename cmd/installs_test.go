package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func TestInstallsAndRename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)

	regPath := filepath.Join(dataDir, "wowtui", "registry.json")
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{
		{Path: "/games/wow/_retail_", Flavor: "retail"},
		{Path: "/games/wow/_classic_", Flavor: "classic", Title: "Classic Main"},
	}}
	if err := os.MkdirAll(filepath.Dir(regPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := reg.Save(regPath); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, "installs", "--registry", regPath)
	if !strings.Contains(out, "retail") || !strings.Contains(out, "Classic Main") {
		t.Errorf("installs output: %s", out)
	}

	runCmd(t, "rename", "/games/wow/_retail_", "Retail Main", "--registry", regPath)

	out = runCmd(t, "installs", "--registry", regPath)
	if !strings.Contains(out, "Retail Main") {
		t.Errorf("installs output after rename: %s", out)
	}

	reg2, err := registry.Load(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if reg2.Installations[0].Title != "Retail Main" {
		t.Errorf("title = %q", reg2.Installations[0].Title)
	}
}

func TestRenameUnknownInstallErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)

	regPath := filepath.Join(dataDir, "wowtui", "registry.json")
	reg := &registry.Registry{Version: 1}
	if err := os.MkdirAll(filepath.Dir(regPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := reg.Save(regPath); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd()
	root.SetArgs([]string{"rename", "/nope", "x", "--registry", regPath})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown installation")
	}
}
