package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

const guardTestRef = "Owner/MyAddon"

func installTrackedAddon(t *testing.T) (*Service, *fakeProvider, wow.Installation, string) {
	t.Helper()
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 1, Tag: "v1", AssetName: "Addon.zip"},
		zipContent: addonZip(t, "1"),
	}
	svc, install, _ := newTestService(t, fp)
	if err := svc.Add(t.Context(), provider.Reference{Provider: "github", ID: guardTestRef}, wow.Retail, install, ""); err != nil {
		t.Fatal(err)
	}
	return svc, fp, install, filepath.Join(install.Path, "Interface", "AddOns", "MyAddon", "core.lua")
}

func writeLocalEdit(t *testing.T, path, edit string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(edit), 0600); err != nil {
		t.Fatal(err)
	}
}

func reviewRequired(t *testing.T, err error) *LocalChangesError {
	t.Helper()
	var review *LocalChangesError
	if !errors.As(err, &review) || !strings.Contains(review.Error(), "modified MyAddon/core.lua") {
		t.Fatalf("wanted a review naming the local edit, got %v", err)
	}
	return review
}

func TestUpdateReportsAnEditedFileBeforeReplacingIt(t *testing.T) {
	svc, fp, install, path := installTrackedAddon(t)
	writeLocalEdit(t, path, "custom edits")
	review := reviewRequired(t, svc.UpdateInstalled(t.Context(), install.Path, guardTestRef, false))
	if review.InstallPath != install.Path || review.Ref != guardTestRef {
		t.Fatalf("wrong scope: %+v", review)
	}
	if statuses, err := svc.Check(t.Context()); err != nil || len(statuses) != 1 {
		t.Fatalf("checking changes: %v, %v", statuses, err)
	} else if got := reviewRequired(t, statuses[0].Error); got.Token() != review.Token() {
		t.Fatal("check and update reported different changes")
	}
	reviewRequired(t, svc.Update(t.Context(), guardTestRef, true))
	reviewRequired(t, svc.UpdateAll(t.Context(), false))
	if fp.downloads != 1 {
		t.Fatalf("bulk update downloaded over a local edit: %d downloads", fp.downloads)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "custom edits" {
		t.Fatalf("initial refusal touched local edit, got %q (%v)", got, err)
	}

	// An approval for another install or an earlier edit cannot authorize this one.
	other := *review
	other.InstallPath += "-other"
	if err := svc.UpdateReviewed(t.Context(), &other, false); err == nil || !strings.Contains(err.Error(), other.InstallPath) || other.Token() == review.Token() {
		t.Fatalf("review with another installation was accepted: %v", err)
	}
	writeLocalEdit(t, path, "different edits")
	newReview := reviewRequired(t, svc.UpdateReviewed(t.Context(), review, false))
	if newReview.Token() == review.Token() {
		t.Fatal("distinct file contents produced the same confirmation token")
	}
	if err := svc.UpdateReviewed(t.Context(), newReview, false); err != nil {
		t.Fatalf("confirmed same-version repair: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "local x = 1" {
		t.Fatalf("repair installed %q (%v)", got, err)
	}
	backups, err := filepath.Glob(filepath.Join(svc.BackupRoot, "*", "MyAddon", "core.lua"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %v, %v", backups, err)
	}
	if got, err := os.ReadFile(backups[0]); err != nil || string(got) != "different edits" {
		t.Fatalf("backup contained %q (%v)", got, err)
	}
	if err := svc.UpdateReviewed(t.Context(), newReview, false); err == nil || fp.downloads != 2 {
		t.Fatalf("stale review survived the repair, err=%v downloads=%d", err, fp.downloads)
	}
}

func TestUpdateRechecksAfterDownloadingAndRecordsNewBaseline(t *testing.T) {
	svc, fp, install, path := installTrackedAddon(t)
	fp.release.ID, fp.release.Tag = 2, "v2"
	fp.zipContent = buildZip(t, map[string]string{"MyAddon/MyAddon.toc": "## Version: 2", "MyAddon/new.lua": "new content"})
	svc.OnEvent = func(e Event) {
		if e.Stage == StageInstall && e.Message == "installing" {
			writeLocalEdit(t, path, "edited during download")
		}
	}
	review := reviewRequired(t, svc.Update(t.Context(), guardTestRef, false))
	if got, err := os.ReadFile(path); err != nil || string(got) != "edited during download" {
		t.Fatalf("edit made during download was lost: %q (%v)", got, err)
	}
	svc.OnEvent = func(e Event) {
		if e.Stage == StageInstall && e.Message == "installing" {
			writeLocalEdit(t, path, "edited again after approval")
		}
	}
	second := reviewRequired(t, svc.UpdateReviewed(t.Context(), review, false))
	if second.Token() == review.Token() {
		t.Fatal("confirmation survived an edit made during download")
	}
	svc.OnEvent = nil
	if err := svc.UpdateReviewed(t.Context(), second, false); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	got := reg.FindAddon(guardTestRef, install.Path)
	if got.Version != "v2" || len(got.ReleaseFiles) != 2 || got.ReleaseFiles[1].Path != "MyAddon/new.lua" {
		t.Fatalf("stored baseline is stale: %+v", got)
	}
	if err := svc.Update(t.Context(), guardTestRef, false); err != nil {
		t.Fatalf("newly installed bytes should match the updated baseline: %v", err)
	}
}

func TestTrackedUpdateRefusesToReplaceAnUnmanagedSibling(t *testing.T) {
	svc, fp, install, _ := installTrackedAddon(t)
	dir := filepath.Join(install.Path, "Interface", "AddOns")
	writeAddonFolder(t, dir, "Sibling", "local")
	fp.release.ID, fp.release.Tag = 2, "v2"
	fp.zipContent = buildZip(t, map[string]string{"MyAddon/MyAddon.toc": "new", "Sibling/Sibling.toc": "incoming"})
	err := svc.UpdateInstalled(t.Context(), install.Path, guardTestRef, false)
	if err == nil || !strings.Contains(err.Error(), "Sibling") {
		t.Fatalf("unexpected unmanaged sibling collision result: %v", err)
	}
	assertToc(t, dir, "Sibling", "local")
}
