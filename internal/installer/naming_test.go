package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func writeNamingTocs(t *testing.T, dir string, tocs ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range tocs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("## Version: 1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func singleRootName(t *testing.T, staging string) string {
	t.Helper()
	roots, err := FindAddonRoots(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1: %+v", len(roots), roots)
	}
	return roots[0].Name
}

func TestNamingFromExtractedSourceArchive(t *testing.T) {
	tests := []struct {
		name string
		tocs []string
		want string
	}{
		{"plain and client toc", []string{"MyAddon.toc", "MyAddon_Vanilla.toc"}, "MyAddon"},
		{"only client tocs", []string{"MyAddon_Mainline.toc", "MyAddon_Vanilla.toc"}, "MyAddon"},
		{"single plain toc", []string{"MyAddon.toc"}, "MyAddon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := []zipEntry{{name: "MyAddon-v1.0/"}, {name: "MyAddon-v1.0/MyAddon.lua", content: "print('hi')"}}
			for _, toc := range tt.tocs {
				entries = append(entries, zipEntry{name: "MyAddon-v1.0/" + toc, content: "## Interface: 110200"})
			}
			zipPath := filepath.Join(t.TempDir(), "addon.zip")
			writeZip(t, zipPath, entries)
			staging := filepath.Join(t.TempDir(), "staging")
			if err := ExtractZip(zipPath, staging, defaultLimits()); err != nil {
				t.Fatal(err)
			}

			if got := singleRootName(t, staging); got != tt.want {
				t.Errorf("root name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNamingStripsClientSuffixes(t *testing.T) {
	tests := []struct {
		name string
		tocs []string
		want string
	}{
		{
			name: "every current game type and family suffix",
			tocs: []string{
				"MyAddon_Standard.toc", "MyAddon_Forever.toc", "MyAddon_Camelot.toc",
				"MyAddon_Mists.toc", "MyAddon_Cata.toc", "MyAddon_Wrath.toc",
				"MyAddon_TBC.toc", "MyAddon_Vanilla.toc", "MyAddon_Plunderstorm.toc",
				"MyAddon_WoWLabs.toc", "MyAddon_WoWHack.toc", "MyAddon_Mainline.toc",
				"MyAddon_Classic.toc",
			},
			want: "MyAddon",
		},
		{
			name: "legacy suffixes",
			tocs: []string{"MyAddon.toc", "MyAddon_BCC.toc", "MyAddon_WOTLK.toc"},
			want: "MyAddon",
		},
		{
			name: "hyphen separator",
			tocs: []string{"MyAddon.toc", "MyAddon-Classic.toc", "MyAddon-BCC.toc"},
			want: "MyAddon",
		},
		{
			name: "suffix case does not matter",
			tocs: []string{"MyAddon_mainline.toc", "MyAddon-VANILLA.toc", "MyAddon_Wotlk.toc"},
			want: "MyAddon",
		},
		{
			name: "addon name ending in a suffix word",
			tocs: []string{"DBM-Party-Vanilla.toc", "DBM-Party-Vanilla_Mainline.toc"},
			want: "DBM-Party-Vanilla",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staging := t.TempDir()
			writeNamingTocs(t, filepath.Join(staging, "MyAddon-v1.0"), tt.tocs...)

			if got := singleRootName(t, staging); got != tt.want {
				t.Errorf("root name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNamingKeepsNames(t *testing.T) {
	tests := []struct {
		name string
		root string
		tocs []string
		want string
	}{
		{"non-client suffix stays", "Details_Streamer", []string{"Details_Streamer.toc"}, "Details_Streamer"},
		{"non-client suffix from wrapper", "Details_Streamer-1.0", []string{"Details_Streamer.toc"}, "Details_Streamer"},
		{"folder already matches a client toc", "MyAddon", []string{"MyAddon_Vanilla.toc"}, "MyAddon"},
		{"folder name ending in a suffix word", "BigWigs_Classic", []string{"BigWigs_Classic.toc"}, "BigWigs_Classic"},
		{"single client toc keeps full toc name", "MyAddon-v1.0", []string{"MyAddon_Vanilla.toc"}, "MyAddon_Vanilla"},
		{"different bases after stripping", "MyAddon-v1.0", []string{"MyAddon_Mainline.toc", "Other_Vanilla.toc"}, "MyAddon-v1.0"},
		{"unknown suffix is not stripped", "MyAddon-v1.0", []string{"MyAddon.toc", "MyAddon_Retail.toc"}, "MyAddon-v1.0"},
		{"bare suffix is not a base", "MyAddon-v1.0", []string{"Vanilla.toc", "_Vanilla.toc"}, "MyAddon-v1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staging := t.TempDir()
			writeNamingTocs(t, filepath.Join(staging, tt.root), tt.tocs...)

			if got := singleRootName(t, staging); got != tt.want {
				t.Errorf("root name = %q, want %q", got, tt.want)
			}
		})
	}
}
