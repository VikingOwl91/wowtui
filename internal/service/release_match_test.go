package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func TestFingerprintMatchNeedsEvidenceFromEveryFolder(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	inst, files := seedFingerprintPackage(t, svc, false)
	dir := filepath.Join(inst.Path, "Interface", "AddOns")
	// The three matching files in the main folder outnumber the entire
	// unrelated options folder, which happens to use the expected name.
	for _, f := range files {
		if strings.HasPrefix(f.Path, "BossMods_Options/") {
			if err := os.WriteFile(filepath.Join(dir, f.Path), []byte("unrelated"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	cands := mapCandidates(t, svc, nil)
	for _, c := range cands {
		if c.ReleaseVersion != "" {
			t.Fatal("unrelated options folder claimed as part of release")
		}
	}
}

func TestFingerprintImportRejectsChangeAfterPreview(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	inst, _ := seedFingerprintPackage(t, svc, true)
	cands := mapCandidates(t, svc, nil)
	if err := os.WriteFile(filepath.Join(inst.Path, "Interface", "AddOns", "BossMods_Options", "Main.lua"), []byte("new edit"), 0600); err != nil {
		t.Fatal(err)
	}
	results, err := svc.Import(t.Context(), cands[:1])
	if err != nil || len(results) != 1 || results[0].Err == nil {
		t.Fatalf("stale preview: %v, %v", results, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil || len(reg.Addons) != 0 {
		t.Fatalf("unexpected import: %v, %v", reg, err)
	}
}

func TestFingerprintImportsWholeReleaseAndRetainsOriginalHashes(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprint("edited=", edited), func(t *testing.T) {
			svc, _, _ := newTestService(t, &fakeProvider{})
			install, files := seedFingerprintPackage(t, svc, edited)
			cands := mapCandidates(t, svc, nil)
			if len(cands) != 2 || !slices.Equal(cands[0].PackageFolders, []string{"BossMods", "BossMods_Options"}) || cands[0].ReleaseVersion != "1.0" {
				t.Fatalf("expected a whole release: %+v", cands)
			}
			if edited && (len(cands[0].FileChanges) != 1 || cands[0].FileChanges[0].Path != "BossMods_Options/Main.lua") {
				t.Fatalf("missing edited file: %+v", cands[0])
			}
			if !edited && len(cands[0].FileChanges) != 0 {
				t.Fatalf("clean release: %+v", cands[0])
			}
			results, err := svc.Import(t.Context(), cands[:1])
			if err != nil || len(results) != 1 || results[0].Err != nil {
				t.Fatalf("import result %v, %v", results, err)
			}
			reg, err := registry.Load(svc.RegistryPath)
			if err != nil {
				t.Fatal(err)
			}
			a := reg.FindAddon("ExampleOrg/BossMods", install.Path)
			if a == nil || !slices.Equal(a.Folders, []string{"BossMods", "BossMods_Options"}) || !slices.Equal(a.ReleaseFiles, files) {
				t.Fatalf("missing release baseline: %+v", a)
			}
		})
	}
}

func seedFingerprintPackage(t *testing.T, svc *Service, edited bool) (registry.Installation, []fingerprint.File) {
	t.Helper()
	install := registry.Installation{Path: filepath.Join(t.TempDir(), "_retail_"), Flavor: "retail"}
	entry := bossModsEntry()
	entry.Folders = []string{"BossMods", "BossMods_Options"}
	var files []fingerprint.File
	var folders []registry.Folder
	for _, name := range entry.Folders {
		files = append(files, fingerprint.File{Path: name + "/" + name + ".toc", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("## Version: 1.0\n")))}, fingerprint.File{Path: name + "/Main.lua", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("-- release")))})
		dir := filepath.Join(install.Path, "Interface", "AddOns", name)
		writeAddonFolder(t, filepath.Dir(dir), name, "## Version: 1.0\n")
		body := "-- release"
		if edited && name == "BossMods_Options" {
			body = "-- my settings"
		}
		if err := os.WriteFile(filepath.Join(dir, "Main.lua"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		folders = append(folders, hintFolder(name, nil))
	}
	extra := "BossMods/Extra.lua"
	files = append(files, fingerprint.File{Path: extra, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("pristine")))})
	if err := os.WriteFile(filepath.Join(install.Path, "Interface", "AddOns", extra), []byte("pristine"), 0600); err != nil {
		t.Fatal(err)
	}
	release := fingerprint.Release{Version: "1.0", Files: files}
	entry.Fingerprints = []fingerprint.Release{release}
	data, err := json.Marshal(index.File{Format: 2, GeneratedAt: time.Now(), Entries: []index.Entry{entry}})
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	svc.Index, err = index.Load(data, index.Sign(data, priv), pub)
	if err != nil {
		t.Fatal(err)
	}
	seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: folders})
	return install, files
}
