package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func TestHintImportCannotExtendAnInstalledRelease(t *testing.T) {
	svc, _, install, _ := installTrackedAddon(t)
	ref := provider.Reference{Provider: "github", ID: guardTestRef}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	sibling := registry.Folder{Name: "Sibling", Version: "different", Hints: map[string]string{"x-source-repo": "https://github.com/" + ref.ID}}
	reg.Installations = []registry.Installation{{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{sibling}}}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}
	selected := mapCandidates(t, svc, nil)
	before, err := os.ReadFile(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	results, err := svc.Import(t.Context(), selected)
	if err != nil || len(results) != 1 || results[0].Err == nil {
		t.Fatalf("hint-only sibling import should fail: %v, %v", results, err)
	}
	if err := svc.Adopt(t.Context(), install.Path, sibling.Name, ref); err == nil {
		t.Fatal("manual adoption should refuse to replace a managed record")
	}
	after, err := os.ReadFile(svc.RegistryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("refused adoption changed the registry: %v", err)
	}
}

func TestImportRejectsHintMixedIntoFingerprintPackage(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	entry := bossModsEntry()
	entry.Folders = []string{"BossMods"}
	const toc = "## Version: 1.0"
	entry.Fingerprints = []fingerprint.Release{{Version: "1.0", Files: []fingerprint.File{{
		Path: "BossMods/BossMods.toc", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(toc))),
	}}}}
	data, err := json.Marshal(index.File{Format: index.FingerprintFormatVersion, GeneratedAt: time.Now(), Entries: []index.Entry{entry}})
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
	writeAddonFolder(t, filepath.Join(install.Path, "Interface", "AddOns"), "BossMods", toc)
	seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("BossMods", nil),
		hintFolder("Untracked", map[string]string{"x-source-repo": "https://github.com/ExampleOrg/BossMods"}),
	}})
	candidates := mapCandidates(t, svc, nil)
	if len(candidates) != 2 || len(candidates[0].PackageFolders) != 1 {
		t.Fatalf("expected a verified package and an additional hint: %+v", candidates)
	}
	results, err := svc.Import(t.Context(), candidates)
	if err != nil || len(results) != 1 || results[0].Err == nil {
		t.Fatalf("unexpected mixed selection result: %+v, %v", results, err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil || len(reg.Addons) != 0 {
		t.Fatalf("mixed selection registered a partial release: %+v, %v", reg, err)
	}
}

func TestManualAdoptPreservesRecordsRegisteredWhileResolving(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	seedUnmanagedFolder(t, svc, install.Path, "MyAddon")
	ref := provider.Reference{Provider: "github", ID: guardTestRef}
	var before []byte
	svc.Providers["github"] = &importProvider{resolve: func(provider.Reference) error {
		reg, err := registry.Load(svc.RegistryPath)
		if err != nil {
			return err
		}
		reg.UpsertAddon(registry.Addon{Ref: ref.ID, InstallPath: install.Path, Provider: "github", ReleaseID: 27, Version: "v2", Folders: []string{"MyAddon"}})
		if err := reg.Save(svc.RegistryPath); err != nil {
			return err
		}
		before, err = os.ReadFile(svc.RegistryPath)
		return err
	}}
	err := svc.Adopt(t.Context(), install.Path, "MyAddon", ref)
	if err == nil {
		t.Fatal("adoption replaced the concurrently registered release")
	}
	after, err := os.ReadFile(svc.RegistryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("concurrent registration was lost: %v", err)
	}
}
