package cmd

import (
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func TestMapReferencesParseBack(t *testing.T) {
	cfgPath, regPath, installPath := setupMapEnv(t, "")
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatal(err)
	}
	reg.Installations[0].Folders = append(reg.Installations[0].Folders,
		registry.Folder{Name: "Lab", Version: "1.0", Hints: map[string]string{"x-website": "https://gitlab.com/group/sub/lab"}},
		registry.Folder{Name: "SelfHostedSSH", Version: "1.0", Hints: map[string]string{"x-source-repo": "ssh://git@git.example.org/team/ssh.git"}},
	)
	if err := reg.Save(regPath); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, "map", "--config", cfgPath, "--registry", regPath)

	seen := map[string]int{}
	for _, row := range mapRows(t, out, installPath) {
		name, printed := row[2], row[4]
		seen[name]++
		ref, err := provider.ParseReference(printed)
		if name == "wago" || name == "wowinterface" {
			if err == nil {
				t.Errorf("informational %s reference %q parses as %+v", name, printed, ref)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s reference %q does not parse: %v", name, printed, err)
			continue
		}
		if ref.Provider != name {
			t.Errorf("reference %q parses as %s, want %s", printed, ref.Provider, name)
		}
	}
	for _, name := range []string{"github", "gitlab", "wowuphub", "curseforge", "git"} {
		if seen[name] == 0 {
			t.Errorf("map printed no %s row:\n%s", name, out)
		}
	}
	if seen["git"] != 2 {
		t.Errorf("want an https and an ssh git row, got %d:\n%s", seen["git"], out)
	}
}
