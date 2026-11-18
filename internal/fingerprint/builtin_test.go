package fingerprint

import (
	"slices"
	"testing"
)

func TestBuiltinWeakAurasManifests(t *testing.T) {
	const ref = "github:WeakAuras/WeakAuras2"
	releases := Builtin()[ref]
	want := map[string]int{"5.21.1": 742, "5.22.1": 741}
	folders := []string{"WeakAuras", "WeakAurasArchive", "WeakAurasModelPaths", "WeakAurasOptions", "WeakAurasTemplates"}
	if len(releases) != len(want) {
		t.Fatalf("got %d releases", len(releases))
	}
	for _, r := range releases {
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		if len(r.Files) != want[r.Version] || !slices.Equal(r.Folders(), folders) {
			t.Fatalf("unexpected manifest %s with %d files: %v", r.Version, len(r.Files), r.Folders())
		}
		delete(want, r.Version)
	}
	releases[0].Version = ""
	releases[0].Files[0].Path = ""
	first := Builtin()[ref][0]
	if first.Version == "" || first.Files[0].Path == "" {
		t.Fatal("builtin data can be mutated through a lookup")
	}
}
