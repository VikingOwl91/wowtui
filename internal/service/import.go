package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

// ImportResult describes one selected package in one installation. A package
// can own several folders. An unsuccessful group leaves all its folders alone.
type ImportResult struct {
	InstallPath string
	Reference   string
	Folders     []string
	Err         error
}

type importGroup struct {
	ImportResult
	provider       string
	ref            provider.Reference
	remote         *provider.RemoteAddon
	packageVersion string
	packageFolders []string
	previewChanges []fingerprint.Change
	hintBaseline   []fingerprint.File
}

// Serialize the final read/modify/write step of concurrent imports. Provider
// requests run outside this lock and only the final snapshot is written.
var importSaveMu sync.Mutex

// Import adopts only the selected mapped folders. Resolution failures are
// reported per group. A registry error or cancellation aborts the batch.
func (s *Service) Import(ctx context.Context, selected []MappingCandidate) ([]ImportResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, nil
	}
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return nil, err
	}
	groups := groupImports(selected)
	for i := range groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		g := &groups[i]
		if g.Err != nil {
			continue
		}
		if g.ref, g.Err = provider.ParseReference(g.Reference); g.Err != nil {
			continue
		}
		if g.ref.Provider != g.provider {
			g.Err = fmt.Errorf("reference belongs to provider %s, expected %s", g.ref.Provider, g.provider)
			continue
		}
		p := s.Providers[g.provider]
		if p == nil {
			g.Err = providerUnavailable(g.provider)
			continue
		}
		if _, g.Err = g.plan(ctx, s, reg); g.Err != nil {
			continue
		}
		if reg.FindAddon(g.ref.ID, g.InstallPath) == nil {
			s.emit(Event{Stage: StageInfo, Message: fmt.Sprintf("resolving import source %d/%d", i+1, len(groups))})
			var remote provider.RemoteAddon
			remote, g.Err = p.Resolve(ctx, g.ref)
			if g.Err == nil {
				g.remote = &remote
			}
		}
	}

	importSaveMu.Lock()
	defer importSaveMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reg, err = registry.Load(s.RegistryPath)
	if err != nil {
		return nil, err
	}
	results := make([]ImportResult, 0, len(groups))
	changed := false
	for i := range groups {
		g := &groups[i]
		if g.Err == nil {
			var draft *registry.Addon
			draft, g.Err = g.plan(ctx, s, reg)
			if draft != nil {
				if reg.FindAddon(g.ref.ID, g.InstallPath) == nil && g.remote == nil {
					g.Err = fmt.Errorf("managed source changed during import; reopen Import to refresh")
				} else {
					reg.UpsertAddon(*draft)
					changed = true
				}
			}
		}
		results = append(results, g.ImportResult)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if changed {
		if err := reg.Save(s.RegistryPath); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func groupImports(selected []MappingCandidate) []importGroup {
	type source struct{ path, provider, reference string }
	indices := make(map[source]int)
	var groups []importGroup
	for _, c := range selected {
		key := source{c.InstallPath, c.Provider, c.Reference}
		i, ok := indices[key]
		if !ok {
			i = len(groups)
			indices[key] = i
			groups = append(groups, importGroup{ImportResult: ImportResult{InstallPath: c.InstallPath, Reference: c.Reference}, provider: c.Provider})
		}
		g := &groups[i]
		names := []string{c.Folder}
		if len(c.PackageFolders) > 0 {
			if !slices.Contains(c.PackageFolders, c.Folder) || c.ReleaseVersion == "" ||
				(g.packageVersion != "" && (g.packageVersion != c.ReleaseVersion || !slices.Equal(g.packageFolders, c.PackageFolders) || !slices.Equal(g.previewChanges, c.FileChanges))) {
				g.Err = fmt.Errorf("inconsistent release selection for %s", c.Reference)
				continue
			}
			g.packageVersion = c.ReleaseVersion
			g.packageFolders = c.PackageFolders
			g.previewChanges = c.FileChanges
			names = c.PackageFolders
		}
		for _, name := range names {
			if containsFold(g.Folders, name) {
				continue
			}
			for j := range groups {
				if i != j && groups[j].InstallPath == c.InstallPath && containsFold(groups[j].Folders, name) {
					err := fmt.Errorf("multiple sources selected for folder %s", name)
					groups[j].Err = err
					g.Err = err
				}
			}
			g.Folders = append(g.Folders, name)
		}
	}
	return groups
}

// plan builds a detached registry entry from current metadata. Returning nil
// with no error means that this group is already fully registered.
func (g *importGroup) plan(ctx context.Context, s *Service, reg *registry.Registry) (*registry.Addon, error) {
	i := slices.IndexFunc(reg.Installations, func(inst registry.Installation) bool { return inst.Path == g.InstallPath })
	if i < 0 {
		return nil, fmt.Errorf("installation is no longer scanned: %s", g.InstallPath)
	}
	inst := reg.Installations[i]
	existing := reg.FindAddon(g.ref.ID, g.InstallPath)
	draft := registry.Addon{InstallPath: g.InstallPath, Provider: g.provider, Ref: g.ref.ID, Flavor: inst.Flavor, Source: "auto", InstalledAt: time.Now()}
	if existing != nil {
		if existing.Provider != g.provider {
			return nil, fmt.Errorf("reference %s already belongs to provider %s", g.ref.ID, existing.Provider)
		}
		draft = *existing
		draft.Folders = slices.Clone(existing.Folders)
	} else if g.remote != nil {
		draft.Name = g.remote.Name
		draft.Author = g.remote.Author
	}
	var baseline []fingerprint.File
	if g.packageVersion != "" {
		matches, err := s.matchReleases(ctx, inst, g.Reference, g.packageFolders[0])
		if err != nil {
			return nil, err
		}
		if len(matches) != 1 || matches[0].release.Version != g.packageVersion || !slices.Equal(matches[0].folders, g.packageFolders) || !slices.Equal(matches[0].report.Changes, g.previewChanges) {
			return nil, fmt.Errorf("%s changed since review; refresh the import matches", g.Reference)
		}
		for _, owned := range draft.Folders {
			if !slices.Contains(g.packageFolders, owned) {
				return nil, fmt.Errorf("managed folder %s is outside the selected release", owned)
			}
		}
		baseline = matches[0].release.Files
	}
	// A TOC hint identifies a source, but cannot establish which files
	// belong to an existing tracked release. Require a reviewed package
	// match before extending such a record.
	if existing != nil && baseline == nil && slices.ContainsFunc(g.Folders, func(name string) bool { return !containsFold(existing.Folders, name) }) {
		files, err := s.baselineFiles(ctx, reg, existing)
		if err != nil {
			return nil, err
		}
		if len(files) != 0 {
			return nil, fmt.Errorf("adding folders to %s requires a matching release fingerprint", g.Reference)
		}
	}
	changed := false
	for _, name := range g.Folders {
		if baseline != nil && !slices.Contains(g.packageFolders, name) {
			return nil, fmt.Errorf("folder %s is outside the selected release", name)
		}
		if !installer.SafeFolderName(name) {
			return nil, fmt.Errorf("invalid addon folder name %q", name)
		}
		if owner := folderOwner(reg, g.InstallPath, g.ref.ID, name); owner != nil {
			return nil, fmt.Errorf("folder %s is managed by %s:%s", name, owner.Provider, owner.Ref)
		}
		if containsFold(draft.Folders, name) {
			continue
		}
		matchesName := func(folder registry.Folder) bool { return strings.EqualFold(folder.Name, name) }
		f := slices.IndexFunc(inst.Folders, matchesName)
		if f < 0 {
			return nil, fmt.Errorf("folder %s is no longer scanned", name)
		}
		if slices.ContainsFunc(inst.Folders[f+1:], matchesName) {
			return nil, fmt.Errorf("multiple scanned folders match %s ignoring case", name)
		}
		folder := inst.Folders[f]
		candidates := mapFolderHints(inst, folder, s.Index)
		if !slices.Contains(g.packageFolders, folder.Name) && !slices.ContainsFunc(candidates, func(c MappingCandidate) bool { return c.Reference == g.Reference && c.Provider == g.provider }) {
			return nil, fmt.Errorf("folder %s no longer matches %s", name, g.Reference)
		}
		draft.Folders = append(draft.Folders, folder.Name)
		if existing == nil {
			if !changed {
				draft.Version = folder.Version
			} else if draft.Version != folder.Version {
				draft.Version = ""
			}
		} else if strings.TrimSpace(folder.Version) == "" || folder.Version != existing.Version {
			// The recorded release can no longer identify the whole package.
			draft.Version = ""
			draft.ReleaseID = 0
			draft.AssetName = ""
			draft.SHA256 = ""
		}
		changed = true
	}
	if !changed {
		return nil, nil
	}
	if baseline != nil {
		draft.Version = g.packageVersion
		draft.ReleaseFiles = baseline
	} else {
		files, err := alternateBaseline(ctx, s.Index, inst, draft.Folders)
		if err != nil {
			return nil, err
		}
		if len(g.hintBaseline) != 0 && !slices.Equal(files, g.hintBaseline) {
			return nil, fmt.Errorf("release files for %s changed during import; refresh the matches", g.Reference)
		}
		g.hintBaseline = files
		draft.ReleaseFiles = files
	}
	return &draft, nil
}
