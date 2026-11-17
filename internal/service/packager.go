package service

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

const packagerHint = "if the project publishes packaged releases, use --source release"

func packagerWarnings(staging string, roots []installer.StagedRoot) []string {
	var warnings []string
	if file := findPkgmeta(staging); file != "" && scanLines(filepath.Join(staging, file), func(line string) bool {
		return strings.HasPrefix(line, "externals:")
	}) {
		warnings = append(warnings, fmt.Sprintf("%s lists externals; libraries from externals are only bundled by the BigWigs packager and may be missing from this source archive (%s)", file, packagerHint))
	}

	var tocs []string
	for _, root := range roots {
		entries, err := os.ReadDir(root.Src)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toc") {
				continue
			}
			if scanLines(filepath.Join(root.Src, entry.Name()), func(line string) bool {
				return strings.Contains(line, "@project-version@")
			}) {
				tocs = append(tocs, filepath.Join(root.Name, entry.Name()))
			}
		}
	}
	if len(tocs) > 0 {
		warnings = append(warnings, fmt.Sprintf("@project-version@ is not replaced in %s; this source archive was not built by the BigWigs packager (%s)", strings.Join(tocs, ", "), packagerHint))
	}
	return warnings
}

func findPkgmeta(staging string) string {
	dirs := []string{staging}
	if entries, err := os.ReadDir(staging); err == nil {
		var sub []string
		for _, entry := range entries {
			if entry.IsDir() {
				sub = append(sub, entry.Name())
			}
		}
		if len(sub) == 1 {
			dirs = append(dirs, filepath.Join(staging, sub[0]))
		}
	}
	for _, dir := range dirs {
		for _, name := range []string{".pkgmeta", "pkgmeta.yaml"} {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				rel, err := filepath.Rel(staging, path)
				if err != nil {
					return ""
				}
				return rel
			}
		}
	}
	return ""
}

func scanLines(path string, match func(string) bool) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if match(scanner.Text()) {
			return true
		}
	}
	return false
}
