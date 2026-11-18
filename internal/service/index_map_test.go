package service

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func signedIndex(t *testing.T, entries ...index.Entry) *index.Index {
	t.Helper()
	if entries == nil {
		entries = []index.Entry{}
	}
	data, err := json.Marshal(index.File{
		Format:      index.FormatVersion,
		GeneratedAt: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
		Entries:     entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Load(data, index.Sign(data, priv), pub)
	if err != nil {
		t.Fatalf("index.Load: %v", err)
	}
	return idx
}

func bossModsEntry() index.Entry {
	return index.Entry{
		ID:        "bossmods",
		Name:      "Boss Mods",
		Source:    "github:ExampleOrg/BossMods",
		Folders:   []string{"BossMods", "BossMods_Core", "BossMods_Options"},
		GameTypes: []wow.GameType{wow.GameMainline},
		IDs:       index.ProviderIDs{CurseForge: "12345", Wago: "aB3dE5fG", WoWInterface: "6789"},
		Reviewed:  true,
	}
}

func extrasEntry() index.Entry {
	return index.Entry{
		ID:        "bossmods-extras",
		Name:      "Boss Mods Extras",
		Source:    "https://gitlab.com/example-group/addons/bossmods-extras",
		Folders:   []string{"BossMods_Extras", "bossmods_core"},
		GameTypes: []wow.GameType{wow.GameVanilla},
		Reviewed:  true,
	}
}

func questHelperEntry() index.Entry {
	return index.Entry{
		ID:        "questhelper",
		Name:      "Quest Helper",
		Source:    "https://github.com/ExampleDev/QuestHelper",
		Folders:   []string{"QuestHelper"},
		GameTypes: []wow.GameType{wow.GameMainline},
		IDs:       index.ProviderIDs{CurseForge: "555"},
	}
}

func indexMapService(t *testing.T, folders ...registry.Folder) (*Service, string) {
	t.Helper()
	svc, install, _ := newTestService(t, &fakeProvider{})
	svc.Index = signedIndex(t, bossModsEntry(), extrasEntry(), questHelperEntry())
	seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: folders})
	return svc, install.Path
}

func TestMapUnmanagedMatchesIndexFolder(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("BossMods", nil), hintFolder("bossmods_options", nil))

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{
		{path, "BossMods", "github", "github:ExampleOrg/BossMods", "index:folder"},
		{path, "bossmods_options", "github", "github:ExampleOrg/BossMods", "index:folder"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %+v, want %+v", got, want)
	}
}

func TestMapUnmanagedMatchesIndexProviderIDs(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("Renamed", map[string]string{
		"x-curse-project-id": "12345",
		"x-wago-id":          "aB3dE5fG",
		"x-wowi-id":          "6789",
	}))

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{
		{path, "Renamed", "github", "github:ExampleOrg/BossMods", "index:curseforge,index:wago,index:wowinterface"},
		{path, "Renamed", "curseforge", "curseforge:12345", "x-curse-project-id"},
		{path, "Renamed", "wago", "wago:aB3dE5fG", "x-wago-id"},
		{path, "Renamed", "wowinterface", "wowinterface:6789", "x-wowi-id"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMapUnmanagedMergesIndexReasons(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("BossMods", map[string]string{
		"x-curse-project-id": "12345",
		"x-website":          "https://github.com/ExampleOrg/BossMods",
	}))

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{
		{path, "BossMods", "github", "github:ExampleOrg/BossMods", "index:folder,index:curseforge,x-website"},
		{path, "BossMods", "curseforge", "curseforge:12345", "x-curse-project-id"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMapUnmanagedMarksUnreviewedIndexEntry(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("QuestHelper", map[string]string{"x-curse-project-id": "555"}))

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{
		{path, "QuestHelper", "github", "github:ExampleDev/QuestHelper", "index:folder,index:curseforge,unreviewed"},
		{path, "QuestHelper", "curseforge", "curseforge:555", "x-curse-project-id"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMapUnmanagedSortsIndexBeforeHints(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("BossMods_Extras", map[string]string{
		"x-website":          "https://github.com/Other/Fork",
		"x-source-repo":      "https://git.example.org/team/extras.git",
		"x-curse-project-id": "999",
	}), hintFolder("BossMods_Core", nil))

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{
		{path, "BossMods_Core", "github", "github:ExampleOrg/BossMods", "index:folder"},
		{path, "BossMods_Core", "gitlab", "gitlab:example-group/addons/bossmods-extras", "index:folder"},
		{path, "BossMods_Extras", "gitlab", "gitlab:example-group/addons/bossmods-extras", "index:folder"},
		{path, "BossMods_Extras", "curseforge", "curseforge:999", "x-curse-project-id"},
		{path, "BossMods_Extras", "git", "git:https://git.example.org/team/extras.git", "x-source-repo"},
		{path, "BossMods_Extras", "github", "github:Other/Fork", "x-website"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMapUnmanagedIndexCandidateAvailability(t *testing.T) {
	svc, _ := indexMapService(t, hintFolder("BossMods_Core", nil))
	inactive := func(name string) Availability {
		if name == "gitlab" {
			return AvailabilityDisabled
		}
		return AvailabilityUnsupported
	}

	got := availabilityByProvider(mapCandidates(t, svc, inactive))
	want := map[string]Availability{"github": AvailabilityActive, "gitlab": AvailabilityDisabled}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("availability = %v, want %v", got, want)
	}
}

func TestMapUnmanagedIndexSkipsManagedAndUnknownFolders(t *testing.T) {
	svc, path := indexMapService(t, hintFolder("BossMods", nil), hintFolder("Unrelated", map[string]string{"x-curse-project-id": "1"}))
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	reg.UpsertAddon(registry.Addon{Provider: "github", Ref: "Other/Suite", Flavor: "retail", InstallPath: path, Folders: []string{"bossmods"}, Version: "v1", Source: "auto"})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{{path, "Unrelated", "curseforge", "curseforge:1", "x-curse-project-id"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %+v, want %+v", got, want)
	}
}
