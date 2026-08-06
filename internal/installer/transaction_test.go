package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stageAddon(t *testing.T, staging, name, content string) {
	t.Helper()
	dir := filepath.Join(staging, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".toc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindAddonRootsFlatLayout(t *testing.T) {
	staging := t.TempDir()
	stageAddon(t, staging, "Alpha", "## Version: 1")
	stageAddon(t, staging, "Beta", "## Version: 1")
	if err := os.MkdirAll(filepath.Join(staging, "NoToc"), 0o755); err != nil {
		t.Fatal(err)
	}

	roots, err := FindAddonRoots(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("got %d roots, want 2", len(roots))
	}
	if roots[0].Name != "Alpha" || roots[1].Name != "Beta" {
		t.Errorf("roots = %+v", roots)
	}
}

func TestFindAddonRootsWrapperLayout(t *testing.T) {
	staging := t.TempDir()
	wrapper := filepath.Join(staging, "Release-1.0")
	stageAddon(t, wrapper, "Alpha", "## Version: 1")
	stageAddon(t, wrapper, "Beta", "## Version: 1")

	roots, err := FindAddonRoots(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("got %d roots, want 2", len(roots))
	}
	if roots[0].Src != filepath.Join(wrapper, "Alpha") {
		t.Errorf("root src = %q", roots[0].Src)
	}
}

func TestFindAddonRootsNoTocErrors(t *testing.T) {
	staging := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staging, "NoToc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := FindAddonRoots(staging); err == nil {
		t.Fatal("expected error when no toc files exist")
	}
}

func TestFindAddonRootsNormalizesVersionedWrapperToTocName(t *testing.T) {
	staging := t.TempDir()
	wrapper := filepath.Join(staging, "StreamerPrivacy-0.4.0")
	if err := os.MkdirAll(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrapper, "StreamerPrivacy.toc"), []byte("## Version: 0.4.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := FindAddonRoots(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(roots))
	}
	if roots[0].Name != "StreamerPrivacy" {
		t.Errorf("root name = %q, want StreamerPrivacy (toc base name)", roots[0].Name)
	}
	if roots[0].Src != wrapper {
		t.Errorf("root src = %q, want wrapper path", roots[0].Src)
	}
}

func TestFindAddonRootsKeepsNameWhenAlreadyMatching(t *testing.T) {
	staging := t.TempDir()
	stageAddon(t, staging, "MyAddon", "## Version: 1")

	roots, err := FindAddonRoots(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].Name != "MyAddon" {
		t.Fatalf("roots = %+v", roots)
	}
}

func TestFindAddonRootsKeepsNameOnAmbiguousTocs(t *testing.T) {
	staging := t.TempDir()
	dir := filepath.Join(staging, "SomeAddon-1.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Foo.toc", "Bar.toc"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("## Version: 1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	roots, err := FindAddonRoots(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].Name != "SomeAddon-1.0" {
		t.Fatalf("roots = %+v, want folder name kept on ambiguous tocs", roots)
	}
}

func installPlan(t *testing.T) (InstallPlan, string) {
	t.Helper()
	base := t.TempDir()
	addonsDir := filepath.Join(base, "AddOns")
	staging := filepath.Join(base, "staging")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	return InstallPlan{
		AddonsDir:  addonsDir,
		StagingDir: staging,
		BackupRoot: filepath.Join(base, "backups"),
		Label:      "test",
	}, addonsDir
}

func TestInstallRollsBackWhenMoveAsideFails(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, filepath.Join(addonsDir), "Alpha", "## Version: old-a")
	stageAddon(t, filepath.Join(addonsDir), "Beta", "## Version: old-b")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	stageAddon(t, plan.StagingDir, "Beta", "## Version: new-b")

	mover := func(src, dst string) error {
		if strings.Contains(src, "Beta") && strings.Contains(src, "AddOns") {
			return errors.New("simulated failure")
		}
		return MoveDir(src, dst)
	}

	if _, err := installWithMover(plan, mover); err == nil {
		t.Fatal("expected install to fail")
	}

	contentA, err := os.ReadFile(filepath.Join(addonsDir, "Alpha", "Alpha.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contentA) != "## Version: old-a" {
		t.Errorf("Alpha lost during failed move-aside, content = %q", contentA)
	}
	contentB, err := os.ReadFile(filepath.Join(addonsDir, "Beta", "Beta.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contentB) != "## Version: old-b" {
		t.Errorf("Beta lost during failed move-aside, content = %q", contentB)
	}
}

func TestInstallFresh(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: 2")

	result, err := Install(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Folders) != 1 || result.Folders[0] != "Alpha" {
		t.Errorf("folders = %v", result.Folders)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "Alpha", "Alpha.toc")); err != nil {
		t.Error(err)
	}
}

func TestInstallReplacesExistingAndBacksUp(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, filepath.Join(addonsDir), "Alpha", "## Version: 1")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: 2")

	result, err := Install(plan)
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(addonsDir, "Alpha", "Alpha.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 2" {
		t.Errorf("installed content = %q", content)
	}

	backup, err := os.ReadFile(filepath.Join(result.BackupPath, "Alpha", "Alpha.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "## Version: 1" {
		t.Errorf("backup content = %q", backup)
	}
}

func TestInstallRejectsFileConflict(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: 2")
	if err := os.WriteFile(filepath.Join(addonsDir, "Alpha"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(plan); err == nil {
		t.Fatal("expected error when target name is a file")
	}
	content, err := os.ReadFile(filepath.Join(addonsDir, "Alpha"))
	if err != nil || string(content) != "file" {
		t.Error("conflicting file must stay untouched")
	}
}

func TestInstallRollsBackOnFailure(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, filepath.Join(addonsDir), "Alpha", "## Version: old-a")
	stageAddon(t, filepath.Join(addonsDir), "Beta", "## Version: old-b")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	stageAddon(t, plan.StagingDir, "Beta", "## Version: new-b")

	mover := func(src, dst string) error {
		if strings.Contains(src, "Beta") && strings.Contains(src, "staging") {
			return errors.New("simulated failure")
		}
		return MoveDir(src, dst)
	}

	if _, err := installWithMover(plan, mover); err == nil {
		t.Fatal("expected install to fail")
	}

	contentA, err := os.ReadFile(filepath.Join(addonsDir, "Alpha", "Alpha.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contentA) != "## Version: old-a" {
		t.Errorf("Alpha not rolled back, content = %q", contentA)
	}
	contentB, err := os.ReadFile(filepath.Join(addonsDir, "Beta", "Beta.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contentB) != "## Version: old-b" {
		t.Errorf("Beta not rolled back, content = %q", contentB)
	}
}
