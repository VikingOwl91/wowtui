package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func TestAlternateSourceGuardsOwnedFiles(t *testing.T) {
	for _, mode := range []string{"import", "adopt", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 2, Tag: "v2"}}
			fp.zipContent = buildZip(t, map[string]string{
				"BossMods/BossMods.toc":                 "## Version: 2",
				"BossMods_Options/BossMods_Options.toc": "## Version: 2",
			})
			svc, _, _ := newTestService(t, fp)
			svc.Providers["curseforge"] = fp
			inst, knownFiles := seedFingerprintPackage(t, svc, true)
			const folder = "BossMods_Options"
			ref := provider.Reference{Provider: "curseforge", ID: "42"}
			reg, err := registry.Load(svc.RegistryPath)
			if err != nil {
				t.Fatal(err)
			}
			for i, f := range reg.Installations[0].Folders {
				if f.Name == folder {
					reg.Installations[0].Folders[i].Hints = map[string]string{"x-curse-project-id": ref.ID}
					reg.Installations[0].Folders[i].Version = ""
				}
			}
			if mode == "legacy" {
				reg.UpsertAddon(registry.Addon{Provider: ref.Provider, Ref: ref.ID, InstallPath: inst.Path, Flavor: inst.Flavor, Folders: []string{folder}})
			}
			if err := reg.Save(svc.RegistryPath); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "import":
				options := mapCandidates(t, svc, nil)
				i := slices.IndexFunc(options, func(c MappingCandidate) bool { return c.Provider == ref.Provider && c.Folder == folder })
				if i < 0 || options[i].ReleaseVersion != "" || len(options[i].PackageFolders) != 0 {
					t.Fatalf("missing standalone provider hint: %+v", options)
				}
				res, err := svc.Import(t.Context(), options[i:i+1])
				if err != nil || len(res) != 1 || res[0].Err != nil {
					t.Fatalf("import = %+v, %v", res, err)
				}
			case "adopt":
				if err := svc.Adopt(t.Context(), inst.Path, folder, ref); err != nil {
					t.Fatal(err)
				}
			case "legacy":
				idx := svc.Index
				svc.Index = nil
				svc.LoadFingerprintIndex = func() (*index.Index, error) { return idx, nil }
			}

			if mode != "legacy" {
				reg, err = registry.Load(svc.RegistryPath)
				if err != nil {
					t.Fatal(err)
				}
				a := reg.FindAddon(ref.ID, inst.Path)
				var want []fingerprint.File
				for _, file := range knownFiles {
					if strings.HasPrefix(file.Path, folder+"/") {
						want = append(want, file)
					}
				}
				if a == nil || a.Provider != ref.Provider || !slices.Equal(a.Folders, []string{folder}) || !slices.Equal(a.ReleaseFiles, want) || a.Version != "" || a.ReleaseID != 0 || a.SHA256 != "" {
					t.Fatalf("wrong provider, ownership or pristine baseline: %+v", a)
				}
			}
			err = svc.UpdateInstalled(t.Context(), inst.Path, ref.ID, false)
			var review *LocalChangesError
			if !errors.As(err, &review) || len(review.Changes) != 1 || review.Changes[0].Kind != "modified" || review.Changes[0].Path != folder+"/Main.lua" || fp.downloads != 0 {
				t.Fatalf("unreviewed edit: %v, downloads %d", err, fp.downloads)
			}
			err = svc.UpdateReviewed(t.Context(), review, false)
			if err == nil || !strings.Contains(err.Error(), "BossMods") || !strings.Contains(err.Error(), "outside the tracked release") {
				t.Fatalf("unguarded sibling: %v", err)
			}
			path := filepath.Join(inst.Path, "Interface", "AddOns", folder, "Main.lua")
			if data, err := os.ReadFile(path); err != nil || string(data) != "-- my settings" {
				t.Fatalf("local edit changed: %q %v", data, err)
			}
			if mode == "legacy" {
				// A second edit removes the evidence identifying this legacy
				// installation. Earlier consent must never authorize these bytes.
				writeLocalEdit(t, filepath.Join(filepath.Dir(path), folder+".toc"), "second local edit")
				downloads := fp.downloads
				if err := svc.UpdateReviewed(t.Context(), review, false); err == nil || fp.downloads != downloads {
					t.Fatalf("stale legacy approval after losing the match: err=%v, downloads=%d", err, fp.downloads)
				}
			}
		})
	}
}

func TestScopedBaselineRequiresCoverageAndConsistentHashes(t *testing.T) {
	a := fingerprint.File{Path: "Core/A", SHA256: strings.Repeat("a", 64)}
	b := fingerprint.File{Path: "Core/B", SHA256: strings.Repeat("b", 64)}
	c := fingerprint.File{Path: "Other/C", SHA256: strings.Repeat("c", 64)}
	release := func(files ...fingerprint.File) releaseMatch {
		return releaseMatch{release: fingerprint.Release{Version: "v1", Files: files}}
	}
	files, err := scopedBaseline([]releaseMatch{release(a, b, c), release(b, a)}, []string{"Core"})
	if err != nil || !slices.Equal(files, []fingerprint.File{a, b}) {
		t.Fatalf("matching mirrors: %+v, %v", files, err)
	}
	if _, err := scopedBaseline([]releaseMatch{release(a)}, []string{"Core", "Other"}); err == nil {
		t.Fatal("silently accepted a baseline that excludes an owned folder")
	}
	if _, err := scopedBaseline([]releaseMatch{release(a, b), release(a)}, []string{"Core"}); err == nil {
		t.Fatal("silently accepted two different file lists for one folder")
	}
}

func TestHintAdoptionCannotLoseBaselineDuringResolution(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	inst, _ := seedFingerprintPackage(t, svc, true)
	const folder = "BossMods_Options"
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range reg.Installations[0].Folders {
		if f.Name == folder {
			reg.Installations[0].Folders[i].Hints = map[string]string{"x-curse-project-id": "42"}
		}
	}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}
	svc.Providers["curseforge"] = &importProvider{resolve: func(provider.Reference) error {
		return os.WriteFile(filepath.Join(inst.Path, "Interface", "AddOns", folder, folder+".toc"), []byte("a second edit"), 0600)
	}}
	options := mapCandidates(t, svc, nil)
	i := slices.IndexFunc(options, func(c MappingCandidate) bool { return c.Provider == "curseforge" })
	if i < 0 {
		t.Fatal("alternate hint not found")
	}
	res, err := svc.Import(t.Context(), options[i:i+1])
	if err != nil || len(res) != 1 || res[0].Err == nil {
		t.Fatalf("lost the baseline during resolution: %+v, %v", res, err)
	}
	writeLocalEdit(t, filepath.Join(inst.Path, "Interface", "AddOns", folder, folder+".toc"), "## Version: 1.0\n")
	if err := svc.Adopt(t.Context(), inst.Path, folder, provider.Reference{Provider: "curseforge", ID: "42"}); err == nil {
		t.Fatal("manual adoption lost its baseline during resolution")
	}
	reg, err = registry.Load(svc.RegistryPath)
	if err != nil || len(reg.Addons) != 0 {
		t.Fatalf("stored an unprotected record: %+v, %v", reg, err)
	}
}
