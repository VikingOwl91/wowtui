package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

const pkgmetaWithExternals = "package-as: MyAddon\n\nexternals:\n  Libs/LibStub: https://repos.wowace.com/wow/libstub/trunk\n"

func TestSourceArchivePackagerWarnings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     provider.ReleaseKind
		files    map[string]string
		folders  []string
		warnings [][]string
	}{
		{
			name: "tag with .pkgmeta externals in wrapper",
			kind: provider.KindTag,
			files: map[string]string{
				"Repo-v1.0.0/.pkgmeta":                          pkgmetaWithExternals,
				"Repo-v1.0.0/MyAddon/MyAddon.toc":               "## Version: v1.0.0\n",
				"Repo-v1.0.0/MyAddon_Config/MyAddon_Config.toc": "## Version: v1.0.0\n",
			},
			folders:  []string{"MyAddon", "MyAddon_Config"},
			warnings: [][]string{{filepath.Join("Repo-v1.0.0", ".pkgmeta"), "externals", "packager", "--source release"}},
		},
		{
			name: "branch with top-level pkgmeta.yaml externals",
			kind: provider.KindBranch,
			files: map[string]string{
				"pkgmeta.yaml":        pkgmetaWithExternals,
				"MyAddon/MyAddon.toc": "## Version: v1.0.0\n",
			},
			folders:  []string{"MyAddon"},
			warnings: [][]string{{"pkgmeta.yaml", "externals", "packager", "--source release"}},
		},
		{
			name: "tag with unreplaced project version",
			kind: provider.KindTag,
			files: map[string]string{
				"MyAddon-v1.0.0/MyAddon.toc": "## Title: MyAddon\r\n## Version: @project-version@\r\n",
				"MyAddon-v1.0.0/core.lua":    "local x = 1",
			},
			folders:  []string{"MyAddon"},
			warnings: [][]string{{filepath.Join("MyAddon", "MyAddon.toc"), "@project-version@", "--source release"}},
		},
		{
			name: "tag with externals and unreplaced project version",
			kind: provider.KindTag,
			files: map[string]string{
				"MyAddon-v1.0.0/.pkgmeta":    pkgmetaWithExternals,
				"MyAddon-v1.0.0/MyAddon.toc": "## Version: @project-version@\n",
			},
			folders: []string{"MyAddon"},
			warnings: [][]string{
				{filepath.Join("MyAddon-v1.0.0", ".pkgmeta"), "externals"},
				{filepath.Join("MyAddon", "MyAddon.toc"), "@project-version@"},
			},
		},
		{
			name: "release with externals and unreplaced project version",
			kind: provider.KindRelease,
			files: map[string]string{
				"MyAddon/.pkgmeta":    pkgmetaWithExternals,
				"MyAddon/MyAddon.toc": "## Version: @project-version@\n",
			},
			folders: []string{"MyAddon"},
		},
		{
			name: "tag with .pkgmeta without top-level externals",
			kind: provider.KindTag,
			files: map[string]string{
				"Repo-v1.0.0/.pkgmeta":            "package-as: MyAddon\n# externals: none\nmove-folders:\n  externals: nested\n ignore:\n",
				"Repo-v1.0.0/MyAddon/MyAddon.toc": "## Version: v1.0.0\n",
			},
			folders: []string{"MyAddon"},
		},
		{
			name: "tag where .pkgmeta takes precedence over pkgmeta.yaml",
			kind: provider.KindTag,
			files: map[string]string{
				"Repo-v1.0.0/.pkgmeta":            "package-as: MyAddon\n",
				"Repo-v1.0.0/pkgmeta.yaml":        pkgmetaWithExternals,
				"Repo-v1.0.0/MyAddon/MyAddon.toc": "## Version: v1.0.0\n",
			},
			folders: []string{"MyAddon"},
		},
		{
			name: "tag with pkgmeta deeper than the wrapper",
			kind: provider.KindTag,
			files: map[string]string{
				"Repo-v1.0.0/MyAddon/.pkgmeta":    pkgmetaWithExternals,
				"Repo-v1.0.0/MyAddon/MyAddon.toc": "## Version: v1.0.0\n",
				"Repo-v1.0.0/Other/Other.toc":     "## Version: v1.0.0\n",
			},
			folders: []string{"MyAddon", "Other"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakeProvider{
				release:    provider.Release{Kind: tc.kind, Tag: "v1.0.0", Commit: "abc123def456abc123def456abc123def456abcd", AssetName: "MyAddon-v1.0.0.zip"},
				zipContent: buildZip(t, tc.files),
			}
			if tc.kind == provider.KindRelease {
				fp.release.ID = 100
			}
			svc, install, out := newTestService(t, fp)

			ref := provider.Reference{Provider: "github", ID: "Owner/MyAddon"}
			if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err != nil {
				t.Fatalf("warnings must not block the install: %v", err)
			}
			addonsDir := filepath.Join(install.Path, "Interface", "AddOns")
			for _, folder := range tc.folders {
				if _, err := os.Stat(filepath.Join(addonsDir, folder, folder+".toc")); err != nil {
					t.Errorf("folder %s not installed: %v", folder, err)
				}
			}

			warnings := eventsWithStage(out, StageWarning)
			if len(warnings) != len(tc.warnings) {
				t.Fatalf("got %d warnings, want %d: %+v", len(warnings), len(tc.warnings), warnings)
			}
			for i, want := range tc.warnings {
				if warnings[i].Ref != ref.ID {
					t.Errorf("warning %d ref = %q", i, warnings[i].Ref)
				}
				for _, part := range want {
					if !strings.Contains(warnings[i].Message, part) {
						t.Errorf("warning %d = %q, want it to contain %q", i, warnings[i].Message, part)
					}
				}
			}
		})
	}
}
