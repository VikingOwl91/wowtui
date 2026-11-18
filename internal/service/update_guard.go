package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

// LocalChangesError identifies the exact on-disk changes a single update would replace.
type LocalChangesError struct {
	InstallPath string
	Ref         string
	Changes     []fingerprint.Change
}

func (e *LocalChangesError) Error() string {
	var lines []string
	for _, change := range e.Changes {
		lines = append(lines, change.Kind+" "+change.Path)
	}
	return fmt.Sprintf("local changes in %s (%s): %s", e.Ref, e.InstallPath, strings.Join(lines, ", "))
}

// Token binds a CLI confirmation to the selected addon, installation, paths and bytes.
func (e *LocalChangesError) Token() string {
	encoded, _ := json.Marshal(e)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

type updateGuard struct {
	addon    *registry.Addon
	release  fingerprint.Release
	approved *LocalChangesError
}

func (s *Service) baselineFiles(ctx context.Context, reg *registry.Registry, addon *registry.Addon) ([]fingerprint.File, error) {
	if len(addon.ReleaseFiles) != 0 {
		return addon.ReleaseFiles, nil
	}
	return s.knownBaseline(ctx, reg, addon)
}

func (s *Service) updateGuard(ctx context.Context, reg *registry.Registry, addon *registry.Addon, approved *LocalChangesError) (updateGuard, error) {
	files, err := s.baselineFiles(ctx, reg, addon)
	if err != nil {
		return updateGuard{}, err
	}
	// A provider may use different version labels, or none, for the same
	// packaged files. Comparing the recorded bytes needs no provider version.
	return updateGuard{addon: addon, release: fingerprint.Release{Version: "installed", Files: files}, approved: approved}, nil
}

// check returns true if the installed copy needs an explicitly approved repair.
func (g updateGuard) check(ctx context.Context) (bool, error) {
	if len(g.release.Files) == 0 {
		if g.approved != nil {
			return false, fmt.Errorf("the reviewed release is no longer tracked")
		}
		return false, nil
	}
	dir := filepath.Join(g.addon.InstallPath, "Interface", "AddOns")
	report, err := fingerprint.Compare(ctx, dir, g.release)
	if err != nil {
		return false, err
	}
	if len(report.Changes) == 0 {
		if g.approved != nil {
			return false, fmt.Errorf("the reviewed local changes no longer exist in %s", g.addon.Ref)
		}
		return false, nil
	}
	review := &LocalChangesError{InstallPath: g.addon.InstallPath, Ref: g.addon.Ref, Changes: report.Changes}
	if g.approved != nil && review.InstallPath == g.approved.InstallPath && review.Ref == g.approved.Ref && slices.Equal(review.Changes, g.approved.Changes) {
		return true, nil
	}
	return false, review
}

type installedRelease struct {
	folders []string
	sha256  string
	files   []fingerprint.File
}
