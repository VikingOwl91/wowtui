package addon

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Folder struct {
	Name          string
	Tocs          []*Toc
	changelogRepo string
}

func (f *Folder) Hints() map[string]string {
	hints := make(map[string]string)
	if f.changelogRepo != "" {
		hints["packager:changelog"] = f.changelogRepo
	}
	for _, toc := range f.Tocs {
		for key, value := range toc.Fields {
			if value == "" {
				continue
			}
			if key == "x-website" || key == "x-source-repo" || key == "x-sourcerepo" || key == "x-wago" || key == "x-wago-id" || key == "x-wowi-id" || key == "x-tukui-projectid" || key == "x-curse" || key == "x-curseforge" || key == "x-curse-project-id" {
				if key == "x-sourcerepo" {
					key = "x-source-repo"
				}
				hints[key] = value
			}
		}
	}
	return hints
}

func (f *Folder) Version() string {
	for _, toc := range f.Tocs {
		if v := toc.Version(); v != "" {
			return v
		}
	}
	return ""
}

func (f *Folder) Interfaces() []int {
	seen := map[int]bool{}
	var nums []int
	for _, toc := range f.Tocs {
		for _, n := range toc.Interfaces() {
			if seen[n] {
				continue
			}
			seen[n] = true
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	return nums
}

func Scan(installPath string) ([]*Folder, error) {
	addonsDir := filepath.Join(installPath, "Interface", "AddOns")
	entries, err := os.ReadDir(addonsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var folders []*Folder
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tocs, err := parseFolder(filepath.Join(addonsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if len(tocs) == 0 {
			continue
		}
		folder := &Folder{Name: entry.Name(), Tocs: tocs}
		folder.changelogRepo = changelogRepository(filepath.Join(addonsDir, entry.Name(), "CHANGELOG.md"), folder)
		folders = append(folders, folder)
	}

	sort.Slice(folders, func(i, j int) bool { return folders[i].Name < folders[j].Name })
	return folders, nil
}

func parseFolder(dir string) ([]*Toc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toc") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	var tocs []*Toc
	for _, name := range names {
		toc, err := ParseToc(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		tocs = append(tocs, toc)
	}
	return tocs, nil
}
