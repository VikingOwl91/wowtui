package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StagedRoot struct {
	Src  string
	Name string
}

type InstallPlan struct {
	AddonsDir  string
	StagingDir string
	BackupRoot string
	Label      string
}

type InstallResult struct {
	Folders    []string
	BackupPath string
}

func FindAddonRoots(stagingDir string) ([]StagedRoot, error) {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Strings(dirs)

	roots, err := collectRoots(stagingDir, dirs)
	if err != nil {
		return nil, err
	}
	if len(roots) > 0 {
		return roots, nil
	}

	if len(dirs) == 1 {
		wrapper := filepath.Join(stagingDir, dirs[0])
		subEntries, err := os.ReadDir(wrapper)
		if err != nil {
			return nil, err
		}
		var subDirs []string
		for _, entry := range subEntries {
			if entry.IsDir() {
				subDirs = append(subDirs, entry.Name())
			}
		}
		sort.Strings(subDirs)
		roots, err := collectRoots(wrapper, subDirs)
		if err != nil {
			return nil, err
		}
		if len(roots) > 0 {
			return roots, nil
		}
	}

	return nil, fmt.Errorf("archive contains no addon folders (no .toc files found)")
}

func collectRoots(base string, dirNames []string) ([]StagedRoot, error) {
	var roots []StagedRoot
	for _, name := range dirNames {
		path := filepath.Join(base, name)
		hasToc, err := dirHasToc(path)
		if err != nil {
			return nil, err
		}
		if hasToc {
			roots = append(roots, StagedRoot{Src: path, Name: name})
		}
	}
	return roots, nil
}

func dirHasToc(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".toc") {
			return true, nil
		}
	}
	return false, nil
}

func Install(plan InstallPlan) (*InstallResult, error) {
	return installWithMover(plan, MoveDir)
}

func installWithMover(plan InstallPlan, move func(src, dst string) error) (*InstallResult, error) {
	roots, err := FindAddonRoots(plan.StagingDir)
	if err != nil {
		return nil, err
	}

	for _, root := range roots {
		target := filepath.Join(plan.AddonsDir, root.Name)
		info, err := os.Stat(target)
		if err == nil && !info.IsDir() {
			return nil, fmt.Errorf("target %s exists and is not a directory", target)
		}
	}

	var existing []string
	for _, root := range roots {
		if dirExists(filepath.Join(plan.AddonsDir, root.Name)) {
			existing = append(existing, root.Name)
		}
	}

	var backupPath string
	if len(existing) > 0 {
		backupPath, err = Backup(plan.AddonsDir, existing, plan.BackupRoot, plan.Label)
		if err != nil {
			return nil, err
		}
	}

	rollbackDir, err := os.MkdirTemp("", "wowtui-rollback-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(rollbackDir)

	var movedAside []string
	for _, name := range existing {
		if err := move(filepath.Join(plan.AddonsDir, name), filepath.Join(rollbackDir, name)); err != nil {
			rollback(plan.AddonsDir, rollbackDir, nil, movedAside, move)
			return nil, fmt.Errorf("moving old folder %s aside: %w", name, err)
		}
		movedAside = append(movedAside, name)
	}

	var installed []string
	for _, root := range roots {
		target := filepath.Join(plan.AddonsDir, root.Name)
		if err := move(root.Src, target); err != nil {
			rollback(plan.AddonsDir, rollbackDir, installed, existing, move)
			return nil, fmt.Errorf("installing folder %s: %w", root.Name, err)
		}
		installed = append(installed, root.Name)
	}

	folders := make([]string, 0, len(roots))
	for _, root := range roots {
		folders = append(folders, root.Name)
	}
	return &InstallResult{Folders: folders, BackupPath: backupPath}, nil
}

func rollback(addonsDir, rollbackDir string, installed, existing []string, move func(src, dst string) error) {
	for _, name := range installed {
		os.RemoveAll(filepath.Join(addonsDir, name))
	}
	for _, name := range existing {
		src := filepath.Join(rollbackDir, name)
		if dirExists(src) {
			move(src, filepath.Join(addonsDir, name))
		}
	}
}
