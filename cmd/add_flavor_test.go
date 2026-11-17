package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func runAddErr(t *testing.T, args ...string) error {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	cfgPath := filepath.Join(home, "missing-config.toml")
	root.SetArgs(append([]string{"add", "github:Owner/Repo", "--config", cfgPath}, args...))
	return root.Execute()
}

func TestAddAcceptsEveryFlavor(t *testing.T) {
	for _, flavor := range wow.Flavors() {
		err := runAddErr(t, "--flavor", string(flavor))
		if err == nil {
			t.Fatalf("add --flavor %s: expected missing installation error", flavor)
		}
		if strings.Contains(err.Error(), "unknown flavor") {
			t.Errorf("add --flavor %s rejected: %v", flavor, err)
		}
		if want := "no " + string(flavor) + " installation found"; !strings.Contains(err.Error(), want) {
			t.Errorf("add --flavor %s error = %q, want %q", flavor, err, want)
		}
	}
}

func TestAddRejectsUnknownFlavorListingValidOnes(t *testing.T) {
	for _, bad := range []string{"tbc", "Retail", "classic_era", ""} {
		err := runAddErr(t, "--flavor", bad)
		if err == nil {
			t.Fatalf("add --flavor %q: expected error", bad)
		}
		if !strings.Contains(err.Error(), "unknown flavor \""+bad+"\"") {
			t.Errorf("add --flavor %q error = %q, want unknown flavor", bad, err)
		}
		for _, flavor := range wow.Flavors() {
			if !strings.Contains(err.Error(), string(flavor)) {
				t.Errorf("add --flavor %q error %q does not list %s", bad, err, flavor)
			}
		}
	}
}

func TestAddFlavorFlagListsEveryFlavor(t *testing.T) {
	usage := newAddCmd().Flags().Lookup("flavor").Usage
	for _, flavor := range wow.Flavors() {
		if !strings.Contains(usage, string(flavor)) {
			t.Errorf("--flavor usage %q does not list %s", usage, flavor)
		}
	}
}
