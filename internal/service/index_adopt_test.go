package service

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

var bossModsRef = provider.Reference{Provider: "index", ID: "bossmods"}

func TestAddResolvesIndexReference(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 7, Tag: "v1.0.0", AssetName: "BossMods.zip"},
		zipContent: addonZip(t, "1.0.0"),
	}
	svc, install, out := newTestService(t, fp)
	svc.Index = signedIndex(t, bossModsEntry(), questHelperEntry())

	if err := svc.Add(context.Background(), bossModsRef, install.Flavor, install, ""); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Addons) != 1 || reg.Addons[0].Provider != "github" || reg.Addons[0].Ref != "ExampleOrg/BossMods" {
		t.Fatalf("addons = %+v, want the entry's github source", reg.Addons)
	}
	if reg.FindAddon("bossmods", install.Path) != nil {
		t.Error("the index id must not become a registry ref")
	}
	if !strings.Contains(out.String(), "index:bossmods is github:ExampleOrg/BossMods") {
		t.Errorf("output does not name the resolved source:\n%s", out)
	}
}

func TestAddMarksUnreviewedIndexEntry(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 7, Tag: "v1.0.0", AssetName: "QuestHelper.zip"},
		zipContent: addonZip(t, "1.0.0"),
	}
	svc, install, out := newTestService(t, fp)
	svc.Index = signedIndex(t, questHelperEntry())

	if err := svc.Add(context.Background(), provider.Reference{Provider: "index", ID: "questhelper"}, install.Flavor, install, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "index:questhelper is github:ExampleDev/QuestHelper (unreviewed)") {
		t.Errorf("output does not mark the unreviewed entry:\n%s", out)
	}
}

func TestAddIndexReferenceFailsWithoutIndexOrEntry(t *testing.T) {
	cases := []struct {
		name string
		ref  provider.Reference
		load bool
		want string
	}{
		{"no index", bossModsRef, false, "no addon index is loaded"},
		{"unknown id", provider.Reference{Provider: "index", ID: "missing"}, true, `index has no entry "missing"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakeProvider{zipContent: addonZip(t, "1.0.0")}
			svc, install, _ := newTestService(t, fp)
			if tc.load {
				svc.Index = signedIndex(t, bossModsEntry())
			}
			err := svc.Add(context.Background(), tc.ref, install.Flavor, install, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
			if fp.downloads != 0 {
				t.Errorf("downloads = %d, want none", fp.downloads)
			}
			if _, err := os.Stat(svc.RegistryPath); !os.IsNotExist(err) {
				t.Errorf("registry was written: %v", err)
			}
		})
	}
}

func seedIndexInstall(t *testing.T, svc *Service, installPath string, owners ...registry.Addon) {
	t.Helper()
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{{
		Path:   installPath,
		Flavor: "retail",
		Folders: []registry.Folder{
			{Name: "BossMods", Version: "10.2.1"},
			{Name: "bossmods_core", Version: "10.2.0"},
			{Name: "Unrelated", Version: "1.0"},
		},
	}}}
	for _, owner := range owners {
		reg.UpsertAddon(owner)
	}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptIndexReferenceRegistersAllEntryFolders(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	svc.Index = signedIndex(t, bossModsEntry())
	seedIndexInstall(t, svc, install.Path)

	if err := svc.Adopt(context.Background(), install.Path, "bossmods_core", bossModsRef); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("ExampleOrg/BossMods", install.Path)
	if addon == nil {
		t.Fatalf("addons = %+v, want ExampleOrg/BossMods", reg.Addons)
	}
	if addon.Provider != "github" || addon.Version != "10.2.0" {
		t.Errorf("addon = %+v, want github with the named folder's version", addon)
	}
	if want := []string{"BossMods", "bossmods_core"}; !reflect.DeepEqual(addon.Folders, want) {
		t.Errorf("folders = %q, want %q", addon.Folders, want)
	}
}

func TestAdoptIndexReferenceRefuses(t *testing.T) {
	other := registry.Addon{Provider: "github", Ref: "Other/Suite", Flavor: "retail", Folders: []string{"BOSSMODS_CORE"}, Version: "v1", Source: "auto"}
	cases := []struct {
		name   string
		folder string
		ref    provider.Reference
		owners bool
		load   bool
		want   []string
	}{
		{"folder owned by another addon", "BossMods", bossModsRef, true, true, []string{"bossmods_core", "Other/Suite"}},
		{"named folder not in entry", "Unrelated", bossModsRef, false, true, []string{"Unrelated", "bossmods"}},
		{"no index", "BossMods", bossModsRef, false, false, []string{"no addon index is loaded"}},
		{"unknown id", "BossMods", provider.Reference{Provider: "index", ID: "missing"}, false, true, []string{`index has no entry "missing"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, install, _ := newTestService(t, &fakeProvider{})
			if tc.load {
				svc.Index = signedIndex(t, bossModsEntry())
			}
			var owners []registry.Addon
			if tc.owners {
				owner := other
				owner.InstallPath = install.Path
				owners = append(owners, owner)
			}
			seedIndexInstall(t, svc, install.Path, owners...)
			before, err := os.ReadFile(svc.RegistryPath)
			if err != nil {
				t.Fatal(err)
			}

			err = svc.Adopt(context.Background(), install.Path, tc.folder, tc.ref)
			if err == nil {
				t.Fatal("expected adopt to fail")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			after, err := os.ReadFile(svc.RegistryPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("refused adopt changed the registry:\n%s", after)
			}
		})
	}
}

func TestAdoptIndexReferenceResolvesBeforeRegistryChecks(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})

	err := svc.Adopt(context.Background(), "/no/such/install", "Nope", bossModsRef)
	if err == nil || !strings.Contains(err.Error(), "no addon index is loaded") {
		t.Fatalf("error = %v, want the missing index reported first", err)
	}
}
