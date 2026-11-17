package wow

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type Installation struct {
	Path   string
	Flavor GameFlavor
}

func Discover(configuredPaths []string) ([]Installation, error) {
	seen := map[string]bool{}
	var installs []Installation
	add := func(inst Installation) {
		key := filepath.Clean(inst.Path)
		if seen[key] {
			return
		}
		seen[key] = true
		installs = append(installs, inst)
	}

	for _, p := range configuredPaths {
		found, err := probePath(p, false)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, errors.New("configured WoW path does not exist: " + p)
		}
		for _, inst := range found {
			add(inst)
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		for _, p := range defaultSearchPaths(home) {
			found, err := probePath(p, true)
			if err != nil {
				continue
			}
			for _, inst := range found {
				add(inst)
			}
		}
	}

	sort.Slice(installs, func(i, j int) bool {
		if installs[i].Flavor != installs[j].Flavor {
			return installs[i].Flavor < installs[j].Flavor
		}
		return installs[i].Path < installs[j].Path
	})
	return installs, nil
}

func probePath(path string, requireInterface bool) ([]Installation, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	hasInterface := dirExists(filepath.Join(path, "Interface"))

	if flavor, ok := FlavorFromDirName(filepath.Base(path)); ok {
		if requireInterface && !hasInterface {
			return nil, nil
		}
		return []Installation{{Path: path, Flavor: flavor}}, nil
	}

	if hasInterface {
		return []Installation{{Path: path}}, nil
	}

	var installs []Installation
	for _, client := range clientDirs {
		sub := filepath.Join(path, client.dir)
		if !dirExists(sub) {
			continue
		}
		if requireInterface && !dirExists(filepath.Join(sub, "Interface")) {
			continue
		}
		installs = append(installs, Installation{Path: sub, Flavor: client.flavor})
	}
	return installs, nil
}

func defaultSearchPaths(home string) []string {
	paths := []string{
		filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft"),
		filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "World of Warcraft"),
	}
	patterns := []string{
		filepath.Join(home, "Games", "*", "drive_c", "Program Files", "World of Warcraft"),
		filepath.Join(home, "Games", "*", "drive_c", "Program Files (x86)", "World of Warcraft"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		paths = append(paths, matches...)
	}
	return paths
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
