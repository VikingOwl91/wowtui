package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

// WowUp records folder ownership against an installation ID. Resolve the ID
// within the same profile, then intersect with the folders the scan found.
func (s *Service) scanWowUpOwnership(installs []registry.Installation) {
	configDir, err := os.UserConfigDir()
	if err != nil || len(installs) == 0 {
		return
	}
	byPath := make(map[string]map[string]*registry.Folder)
	for _, inst := range installs {
		path, err := filepath.Abs(inst.Path)
		if err != nil {
			continue
		}
		folders := make(map[string]*registry.Folder, len(inst.Folders))
		for i := range inst.Folders {
			folders[inst.Folders[i].Name] = &inst.Folders[i]
		}
		byPath[path] = folders
	}
	for _, profile := range []string{"WowUpCf", "WowUp"} {
		err := readWowUpProfile(filepath.Join(configDir, profile), byPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.emit(Event{Stage: StageWarning, Message: fmt.Sprintf("%s addon records: %v", profile, err)})
		}
	}
}

type wowUpSource struct {
	ProviderName string `json:"providerName"`
	ID           string `json:"id"`
}

func (src wowUpSource) reference() string {
	var prefix string
	switch strings.ToLower(src.ProviderName) {
	case "curse", "curseforge":
		prefix = "curseforge:"
	case "github":
		prefix = "github:"
	case "wowuphub":
		prefix = "wowup:"
	default:
		return ""
	}
	ref, err := provider.ParseReference(prefix + src.ID)
	if err != nil {
		return ""
	}
	return provider.FormatReference(ref)
}

func readWowUpProfile(dir string, byPath map[string]map[string]*registry.Folder) error {
	var prefs struct {
		Installs []struct{ ID, Location string } `json:"wow_installations"`
	}
	if err := readWowUpJSON(filepath.Join(dir, "preferences.json"), &prefs); err != nil {
		return err
	}
	byID := make(map[string]map[string]*registry.Folder)
	for _, inst := range prefs.Installs {
		if inst.ID == "" || !filepath.IsAbs(inst.Location) {
			continue
		}
		path := filepath.Clean(inst.Location)
		if strings.EqualFold(filepath.Ext(path), ".exe") || strings.EqualFold(filepath.Ext(path), ".app") {
			path = filepath.Dir(path)
		}
		if _, duplicate := byID[inst.ID]; duplicate {
			byID[inst.ID] = nil
			continue
		}
		byID[inst.ID] = byPath[path]
	}
	var records map[string]json.RawMessage
	if err := readWowUpJSON(filepath.Join(dir, "addons.json"), &records); err != nil {
		return err
	}
	for _, raw := range records {
		var record struct {
			InstallationID, ProviderName, ExternalID string
			ExternalIDs                              []wowUpSource
			InstalledFolderList                      []string
			InstalledFolders                         string
			IsIgnored                                bool
		}
		if json.Unmarshal(raw, &record) != nil || record.IsIgnored {
			continue
		}
		folders := byID[record.InstallationID]
		if len(folders) == 0 {
			continue
		}
		names := record.InstalledFolderList
		if names == nil {
			names = strings.Split(record.InstalledFolders, ",")
			for i := range names {
				names[i] = strings.TrimSpace(names[i])
			}
		}
		sources := append(record.ExternalIDs, wowUpSource{ProviderName: record.ProviderName, ID: record.ExternalID})
		for _, src := range sources {
			ref := src.reference()
			if ref == "" {
				continue
			}
			for _, name := range names {
				folder := folders[name]
				if folder == nil {
					continue
				}
				i, found := slices.BinarySearch(folder.WowUpRefs, ref)
				if !found {
					folder.WowUpRefs = slices.Insert(folder.WowUpRefs, i, ref)
				}
			}
		}
	}
	return nil
}

func readWowUpJSON(path string, dst any) error {
	const limit = 32 << 20
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return fmt.Errorf("%s must be a regular file of at most 32 MiB", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("%s exceeds 32 MiB", filepath.Base(path))
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("could not read %s: %w", filepath.Base(path), err)
	}
	return nil
}
