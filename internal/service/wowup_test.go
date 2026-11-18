package service

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func TestScanImportsWowUpFolderOwnership(t *testing.T) {
	svc, retail, _ := newTestService(t, &fakeProvider{})
	svc.Providers["curseforge"] = &fakeProvider{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	profile := filepath.Join(home, "WowUpCf")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(home, "other", "_retail_")
	for _, path := range []string{retail.Path, second} {
		for _, folder := range []string{"Core", "DataStore", "Plugin", "Ignored"} {
			toc := "## Version: 1.0\n"
			if folder == "Core" {
				toc += "## X-Curse-Project-ID: 42\n"
			}
			writeAddonFolder(t, filepath.Join(path, "Interface", "AddOns"), folder, toc)
		}
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(profile, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("preferences.json", fmt.Sprintf(`{"wow_installations":[{"id":"one","location":%q}]}`, filepath.Join(retail.Path, "Wow.exe")))
	write("addons.json", `{
		"bundle":{"installationId":"one","providerName":"Curse","externalId":"42","externalIds":[{"providerName":"Curse","id":"42"}],"installedFolderList":["Core","DataStore","Missing","../Plugin"]},
		"alternate":{"installationId":"one","providerName":"WowInterface","externalId":"7032","externalIds":[{"providerName":"Curse","id":"43"}],"installedFolders":"Plugin"},
		"other-owner":{"installationId":"one","providerName":"GitHub","externalId":"Other/Plugin","installedFolderList":["Plugin"]},
		"ignored":{"installationId":"one","isIgnored":true,"providerName":"Curse","externalId":"44","installedFolderList":["Ignored"]},
		"stale":{"installationId":"gone","providerName":"Curse","externalId":"45","installedFolderList":["Ignored"]},
		"other-name":{"installationId":"one","providerName":"Curse","externalId":"46","installedFolderList":[" Ignored "]},
		"bad":{"installationId":"one","providerName":"Curse","externalId":"no-id","installedFolderList":["Ignored"]},
		"malformed":{"installedFolderList":123}
	}`)
	scan := func() {
		t.Helper()
		if _, err := svc.Scan(t.Context(), []string{retail.Path, second}); err != nil {
			t.Fatal(err)
		}
	}
	scan()
	selection := mapCandidates(t, svc, nil)
	want := map[string]string{
		retail.Path + "/Core/curseforge:42":         "x-curse-project-id,wowup:installed-folder",
		retail.Path + "/DataStore/curseforge:42":    "wowup:installed-folder",
		retail.Path + "/Plugin/curseforge:43":       "wowup:installed-folder",
		retail.Path + "/Plugin/github:Other/Plugin": "wowup:installed-folder",
		second + "/Core/curseforge:42":              "x-curse-project-id",
	}
	got := map[string]string{}
	for _, c := range selection {
		if c.Availability == AvailabilityActive {
			got[c.InstallPath+"/"+c.Folder+"/"+c.Reference] = c.Reason
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active candidates = %v, want %v", got, want)
	}
	var bundle []MappingCandidate
	for _, c := range selection {
		if c.InstallPath == retail.Path && c.Reference == "curseforge:42" {
			bundle = append(bundle, c)
		}
	}
	results, err := svc.Import(t.Context(), bundle)
	if err != nil || len(results) != 1 || results[0].Err != nil {
		t.Fatalf("import = %v, %v", results, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil || len(reg.Addons) != 1 || !reflect.DeepEqual(reg.Addons[0].Folders, []string{"Core", "DataStore"}) {
		t.Fatalf("adopted package = %+v (%v)", reg, err)
	}
	write("addons.json", "{")
	scan()
	remaining := mapCandidates(t, svc, nil)
	if len(remaining) != 1 || remaining[0].InstallPath != second || remaining[0].Reason != "x-curse-project-id" {
		t.Fatalf("stale ownership survived a rescan or TOC fallback failed: %+v", remaining)
	}
}
