package registry

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Registry struct {
	Version       int            `json:"version"`
	Installations []Installation `json:"installations"`
}

type Installation struct {
	Path    string   `json:"path"`
	Flavor  string   `json:"flavor"`
	Folders []Folder `json:"folders"`
}

type Folder struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Interfaces []int    `json:"interfaces,omitempty"`
	TocFiles   []string `json:"toc_files"`
}

func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Registry{}, nil
		}
		return nil, err
	}
	reg := &Registry{}
	if err := json.Unmarshal(data, reg); err != nil {
		return nil, err
	}
	return reg, nil
}

func (r *Registry) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "wowtui", "registry.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "wowtui", "registry.json")
}
