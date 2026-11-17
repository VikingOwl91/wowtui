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

func assertNoTempDirs(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wowtui-") {
			t.Errorf("temp dir %s left behind in %s", entry.Name(), dir)
		}
	}
}

func assertRollbackDirBesideAddons(t *testing.T, addonsDir, asideDst string) {
	t.Helper()
	if asideDst == "" {
		t.Fatal("old folder was never moved aside")
	}
	rollbackDir := filepath.Dir(asideDst)
	if filepath.Dir(rollbackDir) != filepath.Dir(addonsDir) {
		t.Errorf("rollback dir %s is not directly inside %s", rollbackDir, filepath.Dir(addonsDir))
	}
	if !strings.HasPrefix(filepath.Base(rollbackDir), ".") {
		t.Errorf("rollback dir %s is not hidden", rollbackDir)
	}
}

func TestInstallKeepsRollbackDirBesideAddonsDir(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")

	var asideDst string
	mover := func(src, dst string) error {
		if src == filepath.Join(addonsDir, "Alpha") {
			asideDst = dst
		}
		return MoveDir(src, dst)
	}
	if _, err := installWithMover(plan, mover); err != nil {
		t.Fatal(err)
	}
	assertRollbackDirBesideAddons(t, addonsDir, asideDst)
	assertNoTempDirs(t, filepath.Dir(addonsDir))
}

func TestInstallRemovesRollbackDirOnFailure(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	stageAddon(t, plan.StagingDir, "Beta", "## Version: new-b")

	var asideDst string
	mover := func(src, dst string) error {
		if src == filepath.Join(addonsDir, "Alpha") {
			asideDst = dst
		}
		if src == filepath.Join(plan.StagingDir, "Beta") {
			return errors.New("simulated failure")
		}
		return MoveDir(src, dst)
	}
	if _, err := installWithMover(plan, mover); err == nil {
		t.Fatal("expected install to fail")
	}
	assertRollbackDirBesideAddons(t, addonsDir, asideDst)
	assertNoTempDirs(t, filepath.Dir(addonsDir))
}

func readToc(t *testing.T, dir, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, name, name+".toc"))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(content)
}

func TestInstallRemovesDroppedFolders(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
	stageAddon(t, addonsDir, "Legacy", "## Version: old-legacy")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	plan.Remove = []string{"Legacy"}

	result, err := Install(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "Legacy")); !os.IsNotExist(err) {
		t.Error("dropped folder must be removed after a successful install")
	}
	if got := readToc(t, addonsDir, "Alpha"); got != "## Version: new-a" {
		t.Errorf("Alpha = %q", got)
	}
	if got := readToc(t, result.BackupPath, "Legacy"); got != "## Version: old-legacy" {
		t.Errorf("backup of dropped folder = %q", got)
	}
	if len(result.Removed) != 1 || result.Removed[0] != "Legacy" {
		t.Errorf("removed = %v, want [Legacy]", result.Removed)
	}
	assertNoTempDirs(t, filepath.Dir(addonsDir))
}

func TestInstallRestoresDroppedFolderOnFailure(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
	stageAddon(t, addonsDir, "Legacy", "## Version: old-legacy")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	stageAddon(t, plan.StagingDir, "Beta", "## Version: new-b")
	plan.Remove = []string{"Legacy"}

	movedLegacy := false
	mover := func(src, dst string) error {
		if src == filepath.Join(addonsDir, "Legacy") {
			movedLegacy = true
		}
		if src == filepath.Join(plan.StagingDir, "Beta") {
			return errors.New("simulated failure")
		}
		return MoveDir(src, dst)
	}
	if _, err := installWithMover(plan, mover); err == nil {
		t.Fatal("expected install to fail")
	}
	if !movedLegacy {
		t.Fatal("dropped folder was never moved aside, rollback not exercised")
	}
	if got := readToc(t, addonsDir, "Legacy"); got != "## Version: old-legacy" {
		t.Errorf("dropped folder not restored, content = %q", got)
	}
	if got := readToc(t, addonsDir, "Alpha"); got != "## Version: old-a" {
		t.Errorf("Alpha not rolled back, content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "Beta")); !os.IsNotExist(err) {
		t.Error("partially installed folder must be removed on rollback")
	}
	assertNoTempDirs(t, filepath.Dir(addonsDir))
}

func TestInstallSkipsRemoveEntriesThatAreNotDroppedFolders(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	linkTarget := filepath.Join(filepath.Dir(addonsDir), "elsewhere")
	stageAddon(t, linkTarget, "Linked", "## Version: dev")
	if err := os.Symlink(filepath.Join(linkTarget, "Linked"), filepath.Join(addonsDir, "Linked")); err != nil {
		t.Fatal(err)
	}
	plan.Remove = []string{"Alpha", "Missing", "Linked"}

	result, err := Install(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 {
		t.Errorf("removed = %v, want none", result.Removed)
	}
	if got := readToc(t, addonsDir, "Alpha"); got != "## Version: new-a" {
		t.Errorf("Alpha = %q, a staged root must win over a remove entry", got)
	}
	if info, err := os.Lstat(filepath.Join(addonsDir, "Linked")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlinked folder must stay untouched: %v", err)
	}
	if got := readToc(t, linkTarget, "Linked"); got != "## Version: dev" {
		t.Errorf("symlink target = %q", got)
	}
}

func TestInstallRejectsUnsafeRemoveNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../outside", "a/b", `a\b`} {
		t.Run(name, func(t *testing.T) {
			plan, addonsDir := installPlan(t)
			stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
			stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
			stageAddon(t, filepath.Dir(addonsDir), "outside", "## Version: outside")
			plan.Remove = []string{name}

			if _, err := Install(plan); err == nil {
				t.Fatalf("expected refusal for remove entry %q", name)
			}
			if got := readToc(t, addonsDir, "Alpha"); got != "## Version: old-a" {
				t.Errorf("Alpha touched by refused install: %q", got)
			}
			if got := readToc(t, filepath.Dir(addonsDir), "outside"); got != "## Version: outside" {
				t.Errorf("folder outside AddOns touched: %q", got)
			}
			if _, err := os.Stat(plan.BackupRoot); !os.IsNotExist(err) {
				t.Error("refused install must not create a backup")
			}
		})
	}
}

func TestInstallReportsReplacedFolders(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, addonsDir, "Alpha", "## Version: old-a")
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	stageAddon(t, plan.StagingDir, "Beta", "## Version: new-b")

	result, err := Install(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replaced) != 1 || result.Replaced[0] != "Alpha" {
		t.Errorf("replaced = %v, want [Alpha]", result.Replaced)
	}
}

func TestInstallUsesProvidedRoots(t *testing.T) {
	plan, addonsDir := installPlan(t)
	stageAddon(t, plan.StagingDir, "Alpha", "## Version: new-a")
	stageAddon(t, plan.StagingDir, "Beta", "## Version: new-b")
	plan.Roots = []StagedRoot{{Src: filepath.Join(plan.StagingDir, "Alpha"), Name: "Alpha"}}

	result, err := Install(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Folders) != 1 || result.Folders[0] != "Alpha" {
		t.Errorf("folders = %v, want [Alpha]", result.Folders)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "Beta")); !os.IsNotExist(err) {
		t.Error("roots outside plan.Roots must not be installed")
	}
}
