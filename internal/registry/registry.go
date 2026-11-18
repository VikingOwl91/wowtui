package registry

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
)

type Registry struct {
	Version       int            `json:"version"`
	Installations []Installation `json:"installations"`
	Addons        []Addon        `json:"addons,omitempty"`
}

type Installation struct {
	Path    string   `json:"path"`
	Flavor  string   `json:"flavor"`
	Title   string   `json:"title,omitempty"`
	Folders []Folder `json:"folders"`
}

type Folder struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Interfaces []int             `json:"interfaces,omitempty"`
	TocFiles   []string          `json:"toc_files"`
	Hints      map[string]string `json:"hints,omitempty"`
	WowUpRefs  []string          `json:"wowup_refs,omitempty"`
}

type Addon struct {
	ReleaseFiles []fingerprint.File `json:"release_files,omitempty"`
	Provider     string             `json:"provider"`
	Ref          string             `json:"ref"`
	Name         string             `json:"name"`
	Author       string             `json:"author,omitempty"`
	Flavor       string             `json:"flavor"`
	InstallPath  string             `json:"install_path"`
	Folders      []string           `json:"folders"`
	Version      string             `json:"version"`
	Source       string             `json:"source,omitempty"`
	ReleaseID    int64              `json:"release_id"`
	AssetName    string             `json:"asset_name"`
	SHA256       string             `json:"sha256"`
	InstalledAt  time.Time          `json:"installed_at"`
}

func (r *Registry) FindAddon(ref, installPath string) *Addon {
	for i := range r.Addons {
		if r.Addons[i].Ref == ref && r.Addons[i].InstallPath == installPath {
			return &r.Addons[i]
		}
	}
	return nil
}

func (r *Registry) UpsertAddon(addon Addon) {
	for i := range r.Addons {
		if r.Addons[i].Ref == addon.Ref && r.Addons[i].InstallPath == addon.InstallPath {
			r.Addons[i] = addon
			return
		}
	}
	r.Addons = append(r.Addons, addon)
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
