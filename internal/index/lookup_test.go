package index_test

import (
	"reflect"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func entryIDs(entries []index.Entry) []string {
	ids := []string{}
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func TestByFolder(t *testing.T) {
	idx := mustLoad(t, fixture(t, "index.json"))

	tests := []struct {
		folder string
		want   []string
	}{
		{"BossMods", []string{"bossmods"}},
		{"bossmods", []string{"bossmods"}},
		{"BOSSMODS_CORE", []string{"bossmods", "bossmods-classic-extras"}},
		{"bossmods_core", []string{"bossmods", "bossmods-classic-extras"}},
		{"QuestHelper", []string{"questhelper"}},
		{"Unknown", []string{}},
		{"", []string{}},
		{"BossMods_", []string{}},
	}
	for _, tt := range tests {
		if got := entryIDs(idx.ByFolder(tt.folder)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ByFolder(%q) = %v, want %v", tt.folder, got, tt.want)
		}
	}
}

func TestByProviderID(t *testing.T) {
	idx := mustLoad(t, fixture(t, "index.json"))

	tests := []struct {
		kind, id string
		want     string
		found    bool
	}{
		{index.KindCurseForge, "12345", "bossmods", true},
		{index.KindCurseForge, "555", "questhelper", true},
		{index.KindWago, "aB3dE5fG", "bossmods", true},
		{index.KindWoWInterface, "6789", "bossmods", true},
		{index.KindWago, "ab3de5fg", "", false},
		{index.KindCurseForge, "6789", "", false},
		{index.KindWoWInterface, "12345", "", false},
		{"tukui", "12345", "", false},
		{index.KindCurseForge, "", "", false},
	}
	for _, tt := range tests {
		entry, ok := idx.ByProviderID(tt.kind, tt.id)
		if ok != tt.found || entry.ID != tt.want {
			t.Errorf("ByProviderID(%q, %q) = %q, %v; want %q, %v", tt.kind, tt.id, entry.ID, ok, tt.want, tt.found)
		}
	}
	if index.KindCurseForge != "curseforge" || index.KindWago != "wago" || index.KindWoWInterface != "wowinterface" {
		t.Error("provider kinds must match the ids keys of the format")
	}
}

func TestSearch(t *testing.T) {
	idx := mustLoad(t, fixture(t, "index.json"))

	tests := []struct {
		query string
		want  []string
	}{
		{"boss", []string{"bossmods", "bossmods-classic-extras"}},
		{"BOSS MODS", []string{"bossmods", "bossmods-classic-extras"}},
		{"classic-extras", []string{"bossmods-classic-extras"}},
		{"Classic Extras", []string{"bossmods-classic-extras"}},
		{"helper", []string{"questhelper"}},
		{"Quest Helper", []string{"questhelper"}},
		{"s", []string{"bossmods", "bossmods-classic-extras", "questhelper"}},
		{"", []string{"bossmods", "bossmods-classic-extras", "questhelper"}},
		{"BossMods_Core", []string{}},
		{"github", []string{}},
		{"nothing", []string{}},
	}
	for _, tt := range tests {
		if got := entryIDs(idx.Search(tt.query)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Search(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestReturnedEntriesCannotMutateIndex(t *testing.T) {
	idx := mustLoad(t, fixture(t, "index.json"))
	before := idx.Entries()

	mutate := func(entries []index.Entry) {
		for i := range entries {
			entries[i].ID = "changed"
			entries[i].Name = "changed"
			if len(entries[i].Folders) > 0 {
				entries[i].Folders[0] = "Changed"
			}
			if len(entries[i].GameTypes) > 0 {
				entries[i].GameTypes[0] = wow.GameForever
			}
			if len(entries[i].Funding) > 0 {
				entries[i].Funding[0].Handle = "changed"
			}
			entries[i].IDs.CurseForge = "changed"
			entries[i].Reviewed = !entries[i].Reviewed
		}
	}
	mutate(idx.Entries())
	mutate(idx.ByFolder("BossMods_Core"))
	mutate(idx.Search(""))
	if entry, ok := idx.ByProviderID(index.KindCurseForge, "12345"); ok {
		mutate([]index.Entry{entry})
	}

	if after := idx.Entries(); !reflect.DeepEqual(after, before) {
		t.Fatalf("index changed through returned entries:\nbefore %+v\nafter  %+v", before, after)
	}
	if got := entryIDs(idx.ByFolder("BossMods")); !reflect.DeepEqual(got, []string{"bossmods"}) {
		t.Fatalf("folder lookup changed: %v", got)
	}
}
