package service

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

type importProvider struct {
	fakeProvider
	resolve func(ref provider.Reference) error
	calls   int
}

func (p *importProvider) Resolve(ctx context.Context, ref provider.Reference) (provider.RemoteAddon, error) {
	p.calls++
	if p.resolve != nil {
		if err := p.resolve(ref); err != nil {
			return provider.RemoteAddon{}, err
		}
	}
	return p.fakeProvider.Resolve(ctx, ref)
}

func TestImportGroupsSelectedIndexFoldersAndPreservesManagedState(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("BossMods", nil), hintFolder("BossMods_Core", nil), hintFolder("BossMods_Options", nil))
	p := &importProvider{}
	svc.Providers["github"] = p
	candidates := mapCandidates(t, svc, nil)
	var selected []MappingCandidate
	for _, c := range candidates {
		if c.Provider == "github" {
			selected = append(selected, c)
		}
	}
	result, err := svc.Import(t.Context(), selected[:2])
	if err != nil || len(result) != 1 || result[0].Err != nil {
		t.Fatalf("import result=%+v, err=%v", result, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	a := reg.FindAddon("ExampleOrg/BossMods", path)
	if a == nil || a.Version != "1.0" || !slices.Equal(a.Folders, []string{"BossMods", "BossMods_Core"}) {
		t.Fatalf("addon=%+v", a)
	}
	a.ReleaseID = 123
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result, err = svc.Import(t.Context(), selected)
		if err != nil || len(result) != 1 || result[0].Err != nil {
			t.Fatalf("extend/retry=%+v err=%v", result, err)
		}
	}
	reg, err = registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	a = reg.FindAddon("ExampleOrg/BossMods", path)
	if a.Version != "1.0" || a.ReleaseID != 123 || !slices.Equal(a.Folders, []string{"BossMods", "BossMods_Core", "BossMods_Options"}) {
		t.Errorf("extended addon=%+v", a)
	}
	if p.calls != 1 || p.downloads != 0 {
		t.Errorf("resolve calls=%d, downloads=%d", p.calls, p.downloads)
	}
}

func TestImportSiblingReleaseIdentity(t *testing.T) {
	for _, test := range []struct {
		name, managedVersion, siblingVersion string
		alreadyOwned, wantUpdate             bool
	}{
		{name: "matching version", managedVersion: "1.0", siblingVersion: "1.0"},
		{name: "different version", managedVersion: "1.0", siblingVersion: "0.9", wantUpdate: true},
		{name: "missing sibling version", managedVersion: "1.0", wantUpdate: true},
		{name: "both versions missing", wantUpdate: true},
		{name: "already owned", managedVersion: "1.0", siblingVersion: "0.9", alreadyOwned: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 123, Tag: "1.0"}}
			svc, inst, _ := newTestService(t, p)
			sibling := registry.Folder{Name: "Extra", Version: test.siblingVersion, Hints: map[string]string{"x-source-repo": "https://github.com/Example/Addon"}}
			seedMapping(t, svc, registry.Installation{Path: inst.Path, Flavor: "retail", Folders: []registry.Folder{sibling}})
			selected := mapCandidates(t, svc, nil)
			reg, err := registry.Load(svc.RegistryPath)
			if err != nil {
				t.Fatal(err)
			}
			want := registry.Addon{
				Provider: "github", Ref: "Example/Addon", Name: "Addon", Author: "Example", InstallPath: inst.Path,
				Flavor: "retail", Folders: []string{"Core"}, Source: "release", InstalledAt: time.Unix(12345, 0).UTC(),
				Version: test.managedVersion, ReleaseID: 123, AssetName: "release.zip", SHA256: "installed-hash",
			}
			if test.alreadyOwned {
				want.Folders = append(want.Folders, sibling.Name)
			}
			reg.UpsertAddon(want)
			if err := reg.Save(svc.RegistryPath); err != nil {
				t.Fatal(err)
			}
			results, err := svc.Import(t.Context(), selected)
			if err != nil || len(results) != 1 || results[0].Err != nil {
				t.Fatalf("results=%+v, err=%v", results, err)
			}
			statuses, err := svc.Check(t.Context())
			if err != nil || len(statuses) != 1 || statuses[0].Error != nil {
				t.Fatalf("statuses=%+v, err=%v", statuses, err)
			}
			if statuses[0].HasUpdate != test.wantUpdate {
				t.Errorf("Check returned HasUpdate=%v, want %v", statuses[0].HasUpdate, test.wantUpdate)
			}
			if !test.alreadyOwned {
				want.Folders = append(want.Folders, sibling.Name)
			}
			if test.wantUpdate {
				want.Version, want.ReleaseID, want.AssetName, want.SHA256 = "", 0, "", ""
			}
			if !reflect.DeepEqual(statuses[0].Addon, want) {
				t.Errorf("stored addon=%+v, want %+v", statuses[0].Addon, want)
			}
		})
	}
}

func TestImportRejectsConflictingSourcesAndContinuesIndependentGroups(t *testing.T) {
	svc, inst, _ := newTestService(t, &fakeProvider{})
	seedMapping(t, svc, registry.Installation{Path: inst.Path, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("TwoSources", map[string]string{"x-source-repo": "https://github.com/One/Addon", "x-website": "https://github.com/Two/Addon"}),
		hintFolder("Good", map[string]string{"x-source-repo": "https://github.com/Good/Addon"}),
		hintFolder("Bad", map[string]string{"x-source-repo": "https://github.com/Bad/Addon"}),
	}})
	p := &importProvider{resolve: func(ref provider.Reference) error {
		if ref.ID == "Bad/Addon" {
			return errors.New("provider rejected request")
		}
		return nil
	}}
	svc.Providers["github"] = p
	all := mapCandidates(t, svc, nil)
	results, err := svc.Import(t.Context(), all)
	if err != nil || len(results) != 4 {
		t.Fatalf("results=%+v, err=%v", results, err)
	}
	for _, r := range results {
		if (r.Err == nil) != (r.Reference == "github:Good/Addon") {
			t.Errorf("unexpected result %+v", r)
		}
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Addons) != 1 || reg.Addons[0].Ref != "Good/Addon" || p.calls != 2 {
		t.Errorf("addons=%+v resolves=%d", reg.Addons, p.calls)
	}
}

func TestImportRechecksOwnershipAfterResolution(t *testing.T) {
	svc, inst, _ := newTestService(t, &fakeProvider{})
	seedMapping(t, svc, registry.Installation{Path: inst.Path, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("Shared", map[string]string{"x-source-repo": "https://github.com/Example/Shared"}),
	}})
	selected := mapCandidates(t, svc, nil)
	svc.Providers["github"] = &importProvider{resolve: func(provider.Reference) error {
		reg, err := registry.Load(svc.RegistryPath)
		if err != nil {
			return err
		}
		reg.Addons = []registry.Addon{{InstallPath: inst.Path, Ref: "Another/Repo", Provider: "github", Folders: []string{"SHARED"}}}
		return reg.Save(svc.RegistryPath)
	}}
	results, err := svc.Import(t.Context(), selected)
	if err != nil || len(results) != 1 || results[0].Err == nil {
		t.Fatalf("results=%+v, err=%v", results, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Addons) != 1 || reg.Addons[0].Ref != "Another/Repo" {
		t.Errorf("overwrote ownership: %+v", reg.Addons)
	}
}

func TestImportChecksCandidatesAndCancellation(t *testing.T) {
	svc, inst, _ := newTestService(t, &fakeProvider{})
	seedMapping(t, svc, registry.Installation{Path: inst.Path, Flavor: "retail", Folders: []registry.Folder{hintFolder("A", map[string]string{"x-source-repo": "https://github.com/Owner/A"})}})
	selected := mapCandidates(t, svc, nil)
	ctx, cancel := context.WithCancel(t.Context())
	p := &importProvider{resolve: func(provider.Reference) error { cancel(); return nil }}
	svc.Providers["github"] = p
	wrong := selected[0]
	wrong.Reference = "github:SomeoneElse/A"
	if r, err := svc.Import(ctx, []MappingCandidate{wrong}); err != nil || len(r) != 1 || r[0].Err == nil || p.calls != 0 {
		t.Fatalf("forged selection=%+v %v", r, err)
	}
	before, err := os.ReadFile(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Import(ctx, selected); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	after, err := os.ReadFile(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("cancelled batch changed registry")
	}
}

func TestImportUsesUnknownVersionForMixedPackageVersions(t *testing.T) {
	svc, inst, _ := newTestService(t, &fakeProvider{})
	hints := map[string]string{"x-source-repo": "https://github.com/Owner/Addon"}
	seedMapping(t, svc, registry.Installation{Path: inst.Path, Flavor: "retail", Folders: []registry.Folder{
		{Name: "Addon", Version: "1.2", Hints: hints}, {Name: "Addon_Extra", Version: "1.0", Hints: hints},
	}})
	results, err := svc.Import(t.Context(), mapCandidates(t, svc, nil))
	if err != nil || len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results %+v, err %v", results, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if v := reg.Addons[0].Version; v != "" {
		t.Fatalf("mixed folder versions recorded as %q", v)
	}
}

func TestImportAvoidsClaimingCaseCollidingScanEntries(t *testing.T) {
	svc, inst, _ := newTestService(t, &fakeProvider{})
	hints := map[string]string{"x-source-repo": "https://github.com/Owner/Addon"}
	seedMapping(t, svc, registry.Installation{Path: inst.Path, Folders: []registry.Folder{hintFolder("UI", hints), hintFolder("ui", hints)}})
	results, err := svc.Import(t.Context(), mapCandidates(t, svc, nil))
	if err != nil || len(results) != 1 || results[0].Err == nil {
		t.Fatalf("accepted ambiguous scanned folder names: %+v %v", results, err)
	}
}

func TestImportSameSourceAcrossInstallationsAndWriteFailure(t *testing.T) {
	svc, inst, _ := newTestService(t, &fakeProvider{})
	folder := hintFolder("Fel", map[string]string{"x-source-repo": "https://github.com/Owner/Fel"})
	seedMapping(t, svc, registry.Installation{Path: inst.Path, Flavor: "retail", Folders: []registry.Folder{folder}}, registry.Installation{Path: "classic", Flavor: "classic", Folders: []registry.Folder{folder}})
	candidates := mapCandidates(t, svc, nil)
	if err := os.Mkdir(svc.RegistryPath+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if results, err := svc.Import(t.Context(), candidates); err == nil || len(results) > 0 {
		t.Fatalf("expected save failure; got %+v, %v", results, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil || len(reg.Addons) != 0 {
		t.Fatalf("write failure modified registry: %+v %v", reg, err)
	}
	if err := os.Remove(svc.RegistryPath + ".tmp"); err != nil {
		t.Fatal(err)
	}
	results, err := svc.Import(t.Context(), candidates)
	if err != nil || len(results) != 2 || results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("results %+v, err %v", results, err)
	}
	reg, err = registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Addons) != 2 || reg.FindAddon("Owner/Fel", "classic").Flavor != "classic" || reg.FindAddon("Owner/Fel", inst.Path).Flavor != "retail" {
		t.Fatalf("addons %+v", reg.Addons)
	}
}
