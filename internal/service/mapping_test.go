package service

import (
	"reflect"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func seedMapping(t *testing.T, svc *Service, installs ...registry.Installation) {
	t.Helper()
	reg := &registry.Registry{Version: 1, Installations: installs}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}
}

func hintFolder(name string, hints map[string]string) registry.Folder {
	return registry.Folder{Name: name, Version: "1.0", Hints: hints}
}

func mapCandidates(t *testing.T, svc *Service, inactive func(string) Availability) []MappingCandidate {
	t.Helper()
	candidates, err := svc.MapUnmanaged(inactive)
	if err != nil {
		t.Fatal(err)
	}
	return candidates
}

type candidateKey struct {
	InstallPath, Folder, Provider, Reference, Reason string
}

func candidateKeys(candidates []MappingCandidate) []candidateKey {
	keys := make([]candidateKey, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, candidateKey{c.InstallPath, c.Folder, c.Provider, c.Reference, c.Reason})
	}
	return keys
}

// Values seen on a real install. None of them identifies an addon a
// provider can resolve, so none may become a candidate.
var junkHintValues = []string{
	"https://raider.io",
	"https://foreverdb.net",
	"https://foreverdb.net/foreverdb-helper",
	"https://wow.curseforge.com/projects/tidy-plates-threat-plates",
	"https://www.curseforge.com/wow/addons/tidy-plates-threat-plates",
	"https://github.com/BigWigsMods",
	"https://wowup.io",
}

func TestMapUnmanagedIgnoresJunkHints(t *testing.T) {
	for _, key := range []string{"x-website", "x-source-repo"} {
		for _, value := range junkHintValues {
			t.Run(key+" "+value, func(t *testing.T) {
				svc, install, _ := newTestService(t, &fakeProvider{})
				seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
					hintFolder("Junk", map[string]string{key: value}),
				}})
				if got := mapCandidates(t, svc, nil); len(got) != 0 {
					t.Errorf("candidates = %+v, want none", got)
				}
			})
		}
	}
}

func TestMapUnmanagedRejectsNonRepositoryHints(t *testing.T) {
	cases := []struct{ key, value string }{
		{"x-website", "https://git.example.org/team/addon.git"},
		{"x-website", "https://example.com/addon.zip"},
		{"x-source-repo", "https://example.com/addon.zip"},
		{"x-source-repo", "http://git.example.org/team/addon.git"},
		{"x-source-repo", "https://git.example.org/team"},
		{"x-source-repo", "ssh://git@git.example.org"},
		{"x-source-repo", "ssh://git@github.com/BigWigsMods"},
		{"x-website", "index:bossmods"},
		{"x-source-repo", "index:bossmods"},
	}
	for _, tc := range cases {
		t.Run(tc.key+" "+tc.value, func(t *testing.T) {
			svc, install, _ := newTestService(t, &fakeProvider{})
			seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
				hintFolder("Addon", map[string]string{tc.key: tc.value}),
			}})
			if got := mapCandidates(t, svc, nil); len(got) != 0 {
				t.Errorf("candidates = %+v, want none", got)
			}
		})
	}
}

func TestMapUnmanagedKeepsRepositoryHints(t *testing.T) {
	cases := []struct{ key, value, provider, ref string }{
		{"x-website", "https://github.com/WeakAuras/WeakAuras2", "github", "github:WeakAuras/WeakAuras2"},
		{"x-website", "https://gitlab.com/group/addon", "gitlab", "gitlab:group/addon"},
		{"x-website", "https://wowup.io/addons/123", "wowuphub", "wowup:123"},
		{"x-website", "https://www.curseforge.com/wow/addons/12345", "curseforge", "curseforge:12345"},
		{"x-source-repo", "https://github.com/WeakAuras/WeakAuras2", "github", "github:WeakAuras/WeakAuras2"},
		{"x-source-repo", "https://git.example.org/team/addon.git", "git", "git:https://git.example.org/team/addon.git"},
		{"x-source-repo", "ssh://git@git.example.org/team/addon.git", "git", "git:ssh://git@git.example.org/team/addon.git"},
		{"x-source-repo", "git:https://git.example.org/team/addon.git", "git", "git:https://git.example.org/team/addon.git"},
	}
	for _, tc := range cases {
		t.Run(tc.key+" "+tc.value, func(t *testing.T) {
			svc, install, _ := newTestService(t, &fakeProvider{})
			seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
				hintFolder("Addon", map[string]string{tc.key: tc.value}),
			}})
			got := candidateKeys(mapCandidates(t, svc, nil))
			want := []candidateKey{{install.Path, "Addon", tc.provider, tc.ref, tc.key}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("candidates = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMapUnmanagedDedupesPerFolder(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	hints := map[string]string{
		"x-website":     "https://github.com/Owner/Repo",
		"x-source-repo": "https://github.com/Owner/Repo.git",
	}
	seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("Repo", hints),
		hintFolder("Repo_Options", hints),
	}})

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{
		{install.Path, "Repo", "github", "github:Owner/Repo", "x-source-repo,x-website"},
		{install.Path, "Repo_Options", "github", "github:Owner/Repo", "x-source-repo,x-website"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %+v, want %+v", got, want)
	}
}

func TestMapUnmanagedDedupesCurseForgeURLAndProjectID(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("Addon", map[string]string{
			"x-website":          "https://www.curseforge.com/wow/addons/12345",
			"x-curse-project-id": "12345",
		}),
	}})

	got := candidateKeys(mapCandidates(t, svc, nil))
	want := []candidateKey{{install.Path, "Addon", "curseforge", "curseforge:12345", "x-website,x-curse-project-id"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %+v, want %+v", got, want)
	}
}

func TestMapUnmanagedReferencesParseBack(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	seedMapping(t, svc, registry.Installation{Path: install.Path, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("GitHub", map[string]string{"x-website": "https://github.com/Owner/Repo"}),
		hintFolder("GitLab", map[string]string{"x-website": "https://gitlab.com/group/sub/project"}),
		hintFolder("Hub", map[string]string{"x-website": "https://wowup.io/addons/123"}),
		hintFolder("Curse", map[string]string{"x-curse-project-id": "12345"}),
		hintFolder("Git", map[string]string{"x-source-repo": "https://git.example.org/team/addon.git"}),
		hintFolder("GitSSH", map[string]string{"x-source-repo": "ssh://git@git.example.org/team/addon.git"}),
	}})

	candidates := mapCandidates(t, svc, nil)
	seen := map[string]bool{}
	for _, c := range candidates {
		ref, err := provider.ParseReference(c.Reference)
		if err != nil {
			t.Errorf("%s: reference %q does not parse: %v", c.Folder, c.Reference, err)
			continue
		}
		if ref.Provider != c.Provider {
			t.Errorf("%s: reference %q parses as %s, want %s", c.Folder, c.Reference, ref.Provider, c.Provider)
		}
		seen[c.Provider] = true
	}
	for _, name := range []string{"github", "gitlab", "wowuphub", "curseforge", "git"} {
		if !seen[name] {
			t.Errorf("no %s candidate in %+v", name, candidates)
		}
	}
}

func TestMapUnmanagedOrdersByInstallFolderProvider(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	hints := func(repo string) map[string]string {
		return map[string]string{
			"x-wowi-id":          "11",
			"x-wago-id":          "abc",
			"x-curse-project-id": "22",
			"x-website":          "https://github.com/Owner/" + repo,
			"x-source-repo":      "https://git.example.org/team/" + repo + ".git",
		}
	}
	seedMapping(t, svc,
		registry.Installation{Path: "/games/z/_retail_", Flavor: "retail", Folders: []registry.Folder{
			hintFolder("Zeta", hints("Zeta")),
			hintFolder("Alpha", hints("Alpha")),
		}},
		registry.Installation{Path: "/games/a/_classic_", Flavor: "classic", Folders: []registry.Folder{
			hintFolder("Mid", hints("Mid")),
		}},
	)

	var want []candidateKey
	for _, entry := range []struct{ install, folder string }{
		{"/games/a/_classic_", "Mid"},
		{"/games/z/_retail_", "Alpha"},
		{"/games/z/_retail_", "Zeta"},
	} {
		want = append(want,
			candidateKey{entry.install, entry.folder, "curseforge", "curseforge:22", "x-curse-project-id"},
			candidateKey{entry.install, entry.folder, "git", "git:https://git.example.org/team/" + entry.folder + ".git", "x-source-repo"},
			candidateKey{entry.install, entry.folder, "github", "github:Owner/" + entry.folder, "x-website"},
			candidateKey{entry.install, entry.folder, "wago", "wago:abc", "x-wago-id"},
			candidateKey{entry.install, entry.folder, "wowinterface", "wowinterface:11", "x-wowi-id"},
		)
	}
	for i := 0; i < 20; i++ {
		if got := candidateKeys(mapCandidates(t, svc, nil)); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: candidates =\n%+v\nwant\n%+v", i, got, want)
		}
	}
}

func availabilityFixture(t *testing.T, svc *Service, installPath string) {
	t.Helper()
	seedMapping(t, svc, registry.Installation{Path: installPath, Flavor: "retail", Folders: []registry.Folder{
		hintFolder("Addon", map[string]string{
			"x-website":          "https://github.com/Owner/Addon",
			"x-source-repo":      "https://git.example.org/team/addon.git",
			"x-wowi-id":          "11",
			"x-wago-id":          "abc",
			"x-curse-project-id": "22",
		}),
		hintFolder("Hub", map[string]string{"x-website": "https://wowup.io/addons/123"}),
		hintFolder("Lab", map[string]string{"x-website": "https://gitlab.com/group/lab"}),
	}})
}

func availabilityByProvider(candidates []MappingCandidate) map[string]Availability {
	got := map[string]Availability{}
	for _, c := range candidates {
		got[c.Provider] = c.Availability
	}
	return got
}

func TestMapUnmanagedReportsAvailability(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	svc.Providers["git"] = &fakeProvider{}
	availabilityFixture(t, svc, install.Path)

	inactive := func(name string) Availability {
		switch name {
		case "wowuphub", "wago":
			return AvailabilityPaused
		case "curseforge":
			return AvailabilityNeedsAPIKey
		case "gitlab", "github":
			return AvailabilityDisabled
		}
		return AvailabilityUnsupported
	}
	got := availabilityByProvider(mapCandidates(t, svc, inactive))
	want := map[string]Availability{
		"github":       AvailabilityActive,
		"git":          AvailabilityActive,
		"gitlab":       AvailabilityDisabled,
		"wowuphub":     AvailabilityPaused,
		"wago":         AvailabilityPaused,
		"curseforge":   AvailabilityNeedsAPIKey,
		"wowinterface": AvailabilityUnsupported,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("availability = %v, want %v", got, want)
	}
}

func TestMapUnmanagedWithoutStateLookupMarksInactiveUnsupported(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	availabilityFixture(t, svc, install.Path)

	got := availabilityByProvider(mapCandidates(t, svc, nil))
	want := map[string]Availability{
		"github":       AvailabilityActive,
		"git":          AvailabilityUnsupported,
		"gitlab":       AvailabilityUnsupported,
		"wowuphub":     AvailabilityUnsupported,
		"wago":         AvailabilityUnsupported,
		"curseforge":   AvailabilityUnsupported,
		"wowinterface": AvailabilityUnsupported,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("availability = %v, want %v", got, want)
	}
}
