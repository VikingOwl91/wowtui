package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanPackagerChangelog(t *testing.T) {
	const header = "# Package Name\r\n\r\n## [2.0](https://github.com/Maintainer/Project/tree/2.0) (2026-01-01)\r\n[Full Changelog](https://github.com/Maintainer/Project/compare/1.9...2.0) [Previous Releases](https://github.com/Maintainer/Project/releases)\r\n"
	for _, tc := range []struct {
		name, content string
		match         bool
	}{
		{"release", header, true},
		{"wrong title", strings.Replace(header, "# Package Name", "# Embedded Library", 1), false},
		{"wrong installed version", strings.ReplaceAll(header, "2.0", "3.0"), false},
		{"different tag", strings.Replace(header, "/tree/2.0", "/tree/1.9", 1), false},
		{"different comparison", strings.Replace(header, "1.9...2.0", "1.8...1.9", 1), false},
		{"different repository", strings.Replace(header, "Project/compare", "Library/compare", 1), false},
		{"mentioned release", "# Release notes\nA dependency updated:\n" + header, false},
		{"incomplete", strings.Split(header, "[Full Changelog]")[0], false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst := t.TempDir()
			dir := filepath.Join(inst, "Interface", "AddOns", "Package")
			writeToc(t, dir, "Package.toc", "## Title: Package Name\n## Version: 2.0\n")
			if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			folders, err := Scan(inst)
			if err != nil || len(folders) != 1 {
				t.Fatalf("scan = %v, %v", folders, err)
			}
			want := ""
			if tc.match {
				want = "github:Maintainer/Project"
			}
			if got := folders[0].Hints()["packager:changelog"]; got != want {
				t.Errorf("repository = %q, want %q", got, want)
			}
		})
	}
}
