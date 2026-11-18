package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

const pawnZipPrefix = "Pawn-"

// mainWorld models the situations S1 found: an original among collections, a
// copy, suffixed TOCs, a star tie-break, a monorepo, an ambiguous pair, a
// hint-only repository and a hint that leads nowhere.
func mainWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld()

	w.repo("WeakAuras/WeakAuras2", 1449, "World of Warcraft addon that provides a powerful framework to display customizable graphics on your screen.")
	w.repo("someone/WoW-Addons-Pack", 40, "My addon collection")
	w.repo("Questie/Questie", 1161, "A quest helper")
	w.repo("3majed/Questie-335", 3, "")
	w.repo("VgerMods/Pawn", 300, "Pawn tells you if an item is an upgrade")
	w.repo("zzz/Pawn-Fixes", 2, "")
	w.repo("muleyo/OmniCD", 4, "Fork of the AddOn OmniCD")
	w.repo("hippuli/Details", 0, "")
	w.repo("Tercioo/Details-Damage-Meter", 900, "Damage meter")
	w.repo("Tercioo/Details-Streamer", 5, "Streamer plugin")
	w.repo("cjber/forever-core", 1, "Forever core")
	w.repo("neonvoidx/wowbackup", 1, "Backup of my WTF")
	w.repo("aaa/AmbiAddon", 12, "")
	w.repo("bbb/AmbiAddon-Classic", 12, "")

	// WeakAuras: collection first, original second; two TOCs with client suffix.
	w.hit("curse", "65387", "someone/WoW-Addons-Pack", "Interface/AddOns/WeakAuras/WeakAuras.toc")
	w.hit("curse", "65387", "WeakAuras/WeakAuras2", "WeakAuras/WeakAuras.toc")
	w.hit("curse", "65387", "WeakAuras/WeakAuras2", "WeakAuras/WeakAuras_Mainline.toc")
	w.hit("wago", "abcWago", "WeakAuras/WeakAuras2", "WeakAuras/WeakAuras.toc")
	w.file("WeakAuras/WeakAuras2", "WeakAuras/WeakAuras.toc", tocText("|cff9482c9Weak|rAuras", "110200, 50503", "## X-Curse-Project-ID: 65387"))
	w.file("WeakAuras/WeakAuras2", "WeakAuras/WeakAuras_Mainline.toc", tocText("|cff9482c9Weak|rAuras", "120000"))
	w.file("WeakAuras/WeakAuras2", ".pkgmeta", string(readTestdata(t, "pkgmeta_weakauras.txt")))
	w.file("WeakAuras/WeakAuras2", "WeakAurasOptions/WeakAurasOptions.toc", tocText("WeakAuras Options", "110200"))
	w.release("WeakAuras/WeakAuras2", "5.22.1", map[string]string{
		"release.json":         string(readTestdata(t, "release_json_weakauras.json")),
		"WeakAuras-5.22.1.zip": "zip",
	})
	w.file("WeakAuras/WeakAuras2", ".github/FUNDING.yml", "github: [WeakAuras]\n")

	// TomTom: only collections.
	w.hit("curse", "19", "someone/WoW-Addons-Pack", "Interface/AddOns/TomTom/TomTom.toc")

	// Questie: suffixed TOCs, a wrongly named port, a collection.
	w.hit("curse", "14075", "someone/WoW-Addons-Pack", "Interface/AddOns/Questie/Questie.toc")
	w.hit("curse", "14075", "3majed/Questie-335", "Questie-335.toc")
	w.hit("curse", "14075", "Questie/Questie", "Questie.toc")
	w.hit("curse", "14075", "Questie/Questie", "Questie_Forever.toc")
	w.hit("curse", "14075", "Questie/Questie", "Questie_Camelot.toc")
	w.file("Questie/Questie", "Questie.toc", tocText("Questie", "11508"))
	w.file("Questie/Questie", "Questie_Forever.toc", tocText("Questie", "16000"))
	w.file("Questie/Questie", "Questie_Camelot.toc", tocText("Questie", "20505"))
	w.release("Questie/Questie", "v9.0.0", map[string]string{"Questie-v9.0.0.zip": "zip"})
	w.file("Questie/Questie", ".github/FUNDING.yml", "patreon: questie # support\ncustom: [\"https://example.org/donate\"]\n")

	// Pawn: two plausible repos, the star rule decides.
	w.hit("wowi", "11", "someone/WoW-Addons-Pack", "Interface/AddOns/Pawn/Pawn.toc")
	w.hit("wowi", "11", "zzz/Pawn-Fixes", "Pawn.toc")
	w.hit("wowi", "11", "VgerMods/Pawn", "Pawn.toc")
	w.file("VgerMods/Pawn", "Pawn.toc", tocText("Pawn", "110200"))
	w.release("VgerMods/Pawn", "2.14.4", map[string]string{pawnZipPrefix + "2.14.4-Classic.zip": "zip", pawnZipPrefix + "2.14.4.zip": "zip"})

	// OmniCD: the only candidate describes itself as a fork.
	w.hit("curse", "1", "muleyo/OmniCD", "OmniCD.toc")

	// Details: a mirror account and the monorepo, then a plugin whose stale
	// standalone repository ranks first.
	w.hit("wago", "detailsWago", "hippuli/Details", "Details.toc")
	w.hit("wago", "detailsWago", "Tercioo/Details-Damage-Meter", "Details/Details.toc")
	w.hit("curse", "1000", "Tercioo/Details-Streamer", "Details_Streamer.toc")
	w.hit("curse", "1000", "Tercioo/Details-Damage-Meter", "plugins/Details_Streamer/Details_Streamer.toc")
	w.file("Tercioo/Details-Damage-Meter", "Details/Details.toc", tocText("Details! Damage Meter", "110200"))
	w.file("Tercioo/Details-Damage-Meter", "plugins/Details_Streamer/Details_Streamer.toc", tocText("Details!: Streamer", "110200"))
	w.file("Tercioo/Details-Damage-Meter", ".pkgmeta", "package-as: Details\nmove-folders:\n  Details/plugins/Details_Streamer: Details_Streamer\n  Details/plugins/Details_Vanguard: Details_Vanguard\n")
	w.release("Tercioo/Details-Damage-Meter", "v1", map[string]string{})

	// ForeverCore: code search only finds a backup repository, the hint is right.
	w.hit("curse", "1796337", "neonvoidx/wowbackup", "ForeverCore.toc")
	w.file("cjber/forever-core", "ForeverCore.toc", tocText("Forever Core", "16000", "## X-Curse-Project-ID: 1796337"))
	w.file("cjber/forever-core", "ForeverCoreUI/ForeverCoreUI.toc", tocText("Forever Core UI", "16000"))

	// Two equal candidates.
	w.hit("wago", "amb1", "aaa/AmbiAddon", "AmbiAddon.toc")
	w.hit("wago", "amb1", "bbb/AmbiAddon-Classic", "AmbiAddon.toc")

	return w
}

func mainRegistry() *registry.Registry {
	return testRegistry(
		[]regFolder{
			{name: "WeakAuras", hints: map[string]string{"x-curse-project-id": "65387", "x-wago-id": "abcWago", "x-website": "https://www.curseforge.com/wow/addons/weakauras"}},
			{name: "WeakAurasOptions"},
			{name: "Questie", hints: map[string]string{"x-curse-project-id": "14075"}},
			{name: "TomTom", hints: map[string]string{"x-curse-project-id": "19"}},
			{name: "Pawn", hints: map[string]string{"x-wowi-id": "11"}},
			{name: "OmniCD", hints: map[string]string{"x-curse-project-id": "1"}},
			{name: "Details", hints: map[string]string{"x-wago-id": "detailsWago"}},
			{name: "Details_Streamer", hints: map[string]string{"x-curse-project-id": "1000"}},
			{name: "ForeverCore", hints: map[string]string{"x-curse-project-id": "1796337", "x-website": "https://github.com/cjber/forever-core"}},
			{name: "ForeverCoreUI"},
			{name: "GhostHint", hints: map[string]string{"x-curse-project-id": "4242", "x-source-repo": "https://github.com/nobody/missing"}},
			{name: "AmbiAddon", hints: map[string]string{"x-wago-id": "amb1"}},
			{name: "Blizzard_Test"},
		},
		regFolder{name: "weakauras", hints: map[string]string{"x-curse-project-id": "65387"}},
	)
}

func runDiscover(t *testing.T, w *world, reg *registry.Registry, cacheDir string, clock Clock) (*Candidates, *Review, *fakeGH, *Client) {
	t.Helper()
	gh := &fakeGH{handler: w.handle}
	c := NewClient(gh, ClientConfig{CacheDir: cacheDir, Clock: clock})
	cands, review, err := Discover(context.Background(), c, reg, DiscoverConfig{MirrorOwners: []string{"hippuli"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return cands, review, gh, c
}

func packageFor(t *testing.T, cands *Candidates, repo string) Package {
	t.Helper()
	for _, pkg := range cands.Packages {
		if pkg.Repo == repo {
			return pkg
		}
	}
	t.Fatalf("no package for %s in %v", repo, packageRepos(cands))
	return Package{}
}

func packageRepos(cands *Candidates) []string {
	var repos []string
	for _, pkg := range cands.Packages {
		repos = append(repos, pkg.Repo)
	}
	return repos
}

func reviewItems(review *Review, reason string) []ReviewItem {
	var items []ReviewItem
	for _, item := range review.Items {
		if item.Reason == reason {
			items = append(items, item)
		}
	}
	return items
}

func reviewFor(t *testing.T, review *Review, reason, folder string) ReviewItem {
	t.Helper()
	for _, item := range reviewItems(review, reason) {
		if slices.Contains(item.Folders, folder) {
			return item
		}
	}
	t.Fatalf("no %s review item for %s in %+v", reason, folder, review.Items)
	return ReviewItem{}
}

func TestDiscoverChoosesOriginals(t *testing.T) {
	cands, _, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	want := []string{"Tercioo/Details-Damage-Meter", "VgerMods/Pawn", "WeakAuras/WeakAuras2", "Questie/Questie", "cjber/forever-core"}
	got := packageRepos(cands)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("packages = %v, want %v", got, want)
	}
	for _, repo := range []string{"someone/WoW-Addons-Pack", "3majed/Questie-335", "muleyo/OmniCD", "hippuli/Details", "zzz/Pawn-Fixes", "neonvoidx/wowbackup"} {
		if slices.Contains(got, repo) {
			t.Errorf("%s must not be chosen", repo)
		}
	}
	if cands.Format != candidatesFormat || cands.GeneratedAt.IsZero() {
		t.Errorf("header = %d, %v", cands.Format, cands.GeneratedAt)
	}
	if !slices.IsSortedFunc(cands.Packages, func(a, b Package) int { return strings.Compare(strings.ToLower(a.Repo), strings.ToLower(b.Repo)) }) {
		t.Errorf("packages are not sorted: %v", got)
	}
}

func TestDiscoverWeakAurasPackage(t *testing.T) {
	cands, _, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	pkg := packageFor(t, cands, "WeakAuras/WeakAuras2")
	if pkg.Source != "github:WeakAuras/WeakAuras2" || pkg.Stars != 1449 || pkg.DefaultBranch != "main" {
		t.Errorf("header = %+v", pkg)
	}
	wantFolders := []string{"WeakAuras", "WeakAurasArchive", "WeakAurasModelPaths", "WeakAurasOptions", "WeakAurasTemplates"}
	if !slices.Equal(pkg.Folders, wantFolders) {
		t.Errorf("folders = %v, want %v", pkg.Folders, wantFolders)
	}
	if !slices.Equal(pkg.SeedFolders, []string{"WeakAuras"}) || pkg.PrimaryFolder != "WeakAuras" {
		t.Errorf("seed = %v, primary = %q", pkg.SeedFolders, pkg.PrimaryFolder)
	}
	if !slices.Equal(pkg.IDs[index.KindCurseForge], []string{"65387"}) || !slices.Equal(pkg.IDs[index.KindWago], []string{"abcWago"}) {
		t.Errorf("ids = %v", pkg.IDs)
	}
	if pkg.Title != "|cff9482c9Weak|rAuras" {
		t.Errorf("title = %q", pkg.Title)
	}
	if !slices.Equal(pkg.Interfaces, []int{50503, 110200, 120000}) {
		t.Errorf("interfaces = %v", pkg.Interfaces)
	}
	if pkg.Release == nil || !pkg.Release.ReleaseJSON || pkg.Release.Tag != "5.22.1" {
		t.Fatalf("release = %+v", pkg.Release)
	}
	wantTypes := []wow.GameType{wow.GameMainline, wow.GameVanilla, wow.GameTitan, wow.GameCata, wow.GameMists}
	if !slices.Equal(pkg.Release.ReleaseJSONGameTypes, wantTypes) {
		t.Errorf("release.json game types = %v", pkg.Release.ReleaseJSONGameTypes)
	}
	if pkg.FundingYML != "github: [WeakAuras]\n" {
		t.Errorf("funding = %q", pkg.FundingYML)
	}
	if pkg.Pkgmeta == nil || pkg.Pkgmeta.PackageAs != "WeakAuras" {
		t.Errorf("pkgmeta = %+v", pkg.Pkgmeta)
	}
	if len(pkg.Evidence) == 0 || pkg.Evidence[0].Kind != "code-search" || pkg.Evidence[0].Query == "" {
		t.Errorf("evidence = %+v", pkg.Evidence)
	}
}

func TestDiscoverSuffixMatchingAndWrongNames(t *testing.T) {
	cands, _, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	pkg := packageFor(t, cands, "Questie/Questie")
	if !slices.Equal(pkg.Folders, []string{"Questie"}) {
		t.Errorf("folders = %v", pkg.Folders)
	}
	// The _Forever and _Camelot TOCs belong to the Questie folder; the
	// Questie-335 port is a different name and its repository is not chosen.
	for _, want := range []string{"Questie.toc", "Questie_Camelot.toc", "Questie_Forever.toc"} {
		if !slices.Contains(pkg.TocFiles, want) {
			t.Errorf("toc files %v lack %s", pkg.TocFiles, want)
		}
	}
	if pkg.Release == nil || pkg.Release.ReleaseJSON || len(pkg.Release.ReleaseJSONGameTypes) != 0 {
		t.Errorf("release = %+v", pkg.Release)
	}
	if !strings.Contains(pkg.FundingYML, "patreon: questie") {
		t.Errorf("funding = %q", pkg.FundingYML)
	}
}

func TestDiscoverFiltersCollectionsAndCopies(t *testing.T) {
	cands, review, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	for _, folder := range []string{"TomTom", "OmniCD"} {
		for _, pkg := range cands.Packages {
			if slices.Contains(pkg.SeedFolders, folder) {
				t.Errorf("%s resolved to %s", folder, pkg.Repo)
			}
		}
	}
	tomtom := reviewFor(t, review, reasonFiltered, "TomTom")
	if len(tomtom.Candidates) != 1 || tomtom.Candidates[0].Repo != "someone/WoW-Addons-Pack" || !strings.Contains(tomtom.Candidates[0].Why, "collection") {
		t.Errorf("TomTom review = %+v", tomtom)
	}
	omnicd := reviewFor(t, review, reasonFiltered, "OmniCD")
	if len(omnicd.Candidates) != 1 || omnicd.Candidates[0].Repo != "muleyo/OmniCD" || !strings.Contains(omnicd.Candidates[0].Why, "copy") {
		t.Errorf("OmniCD review = %+v", omnicd)
	}
}

func TestDiscoverMirrorOwnerAndStarRule(t *testing.T) {
	cands, _, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	details := packageFor(t, cands, "Tercioo/Details-Damage-Meter")
	if !slices.Contains(details.SeedFolders, "Details") {
		t.Errorf("Details seed missing: %v", details.SeedFolders)
	}
	pawn := packageFor(t, cands, "VgerMods/Pawn")
	if len(pawn.Evidence) == 0 || !strings.Contains(pawn.Evidence[0].Basis, "stars") {
		t.Errorf("pawn evidence = %+v", pawn.Evidence)
	}
	if pawn.Release == nil || pawn.Release.ReleaseJSON {
		t.Errorf("pawn release = %+v", pawn.Release)
	}
}

func TestDiscoverMonorepoExpansion(t *testing.T) {
	cands, _, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	for _, pkg := range cands.Packages {
		if pkg.Repo == "Tercioo/Details-Streamer" {
			t.Fatal("the stale standalone plugin repository was chosen")
		}
	}
	pkg := packageFor(t, cands, "Tercioo/Details-Damage-Meter")
	if !slices.Equal(pkg.SeedFolders, []string{"Details", "Details_Streamer"}) {
		t.Errorf("seed folders = %v", pkg.SeedFolders)
	}
	want := []string{"Details", "Details_Streamer", "Details_Vanguard"}
	if !slices.Equal(pkg.Folders, want) {
		t.Errorf("folders = %v, want %v (Details_Vanguard from move-folders)", pkg.Folders, want)
	}
	if pkg.Release == nil || len(pkg.Release.Assets) != 0 || pkg.Release.ReleaseJSON {
		t.Errorf("release = %+v", pkg.Release)
	}
	monorepoEvidence := false
	for _, ev := range pkg.Evidence {
		monorepoEvidence = monorepoEvidence || strings.Contains(ev.Basis, "monorepo")
	}
	if !monorepoEvidence {
		t.Errorf("evidence = %+v", pkg.Evidence)
	}
}

func TestDiscoverUsesHintAndRootDirectoryExpansion(t *testing.T) {
	w := mainWorld(t)
	cands, _, gh, _ := runDiscover(t, w, mainRegistry(), t.TempDir(), newFakeClock())
	pkg := packageFor(t, cands, "cjber/forever-core")
	if !slices.Equal(pkg.Folders, []string{"ForeverCore", "ForeverCoreUI"}) {
		t.Errorf("folders = %v", pkg.Folders)
	}
	if len(pkg.Evidence) == 0 || pkg.Evidence[0].Kind != "hint" {
		t.Errorf("evidence = %+v", pkg.Evidence)
	}
	if pkg.Release != nil {
		t.Errorf("release = %+v, want none", pkg.Release)
	}
	// The hint settled the folder, so its ID was never searched.
	for _, req := range gh.calls {
		if req.Path == "search/code" && strings.Contains(req.Params["q"], "1796337") {
			t.Errorf("searched an ID the hint already settled: %v", req.Params)
		}
	}
}

func TestDiscoverReportsAmbiguousAndUnconfirmed(t *testing.T) {
	cands, review, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	for _, pkg := range cands.Packages {
		if slices.Contains(pkg.SeedFolders, "AmbiAddon") || slices.Contains(pkg.SeedFolders, "GhostHint") {
			t.Errorf("%v must stay unresolved", pkg.SeedFolders)
		}
	}
	amb := reviewFor(t, review, reasonAmbiguous, "AmbiAddon")
	if len(amb.Candidates) != 2 {
		t.Errorf("ambiguous candidates = %+v", amb.Candidates)
	}
	hint := reviewFor(t, review, reasonHintUnconfirmed, "GhostHint")
	if !strings.Contains(hint.Source, "nobody/missing") {
		t.Errorf("hint item = %+v", hint)
	}
	un := reviewFor(t, review, reasonUnresolved, "GhostHint")
	if !strings.Contains(un.Source, "4242") {
		t.Errorf("unresolved item = %+v", un)
	}
}

func TestDiscoverSummaryAndNoInstallPaths(t *testing.T) {
	cands, review, _, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	want := ReviewSummary{RegistryFolders: 13, SeededFolders: 10, ResolvedFolders: 6, CoveredFolders: 8, Packages: 5, ReviewItems: len(review.Items)}
	if review.Summary != want {
		t.Errorf("summary = %+v, want %+v", review.Summary, want)
	}
	if !slices.Equal(review.FoldersWithoutIDs, []string{"Blizzard_Test"}) {
		t.Errorf("folders without ids = %v", review.FoldersWithoutIDs)
	}
	for name, v := range map[string]any{"candidates": cands, "review": review} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"privateuser", "Games", "World of Warcraft", "_retail_", "_classic_", "/home/"} {
			if strings.Contains(string(data), secret) {
				t.Errorf("%s output leaks %q", name, secret)
			}
		}
	}
}

func TestDiscoverRerunWithCacheMakesNoCallsAndIsIdentical(t *testing.T) {
	w := mainWorld(t)
	dir := t.TempDir()
	cands1, review1, gh1, _ := runDiscover(t, w, mainRegistry(), dir, newFakeClock())
	first := len(gh1.calls)
	if first == 0 {
		t.Fatal("first run made no calls")
	}
	// A different clock proves the output does not depend on the run time.
	later := newFakeClock()
	later.now = later.now.Add(72 * time.Hour)
	cands2, review2, gh2, c2 := runDiscover(t, w, mainRegistry(), dir, later)
	if len(gh2.calls) != 0 || c2.Stats().Live != 0 {
		t.Fatalf("rerun made %d live calls", len(gh2.calls))
	}
	a, _ := marshalJSON(cands1)
	b, _ := marshalJSON(cands2)
	if string(a) != string(b) {
		t.Errorf("candidates differ between runs")
	}
	a, _ = marshalJSON(review1)
	b, _ = marshalJSON(review2)
	if string(a) != string(b) {
		t.Errorf("review differs between runs")
	}
}

func TestDiscoverThrottlesCodeSearch(t *testing.T) {
	w := mainWorld(t)
	clock := newFakeClock()
	_, _, gh, _ := runDiscover(t, w, mainRegistry(), t.TempDir(), clock)
	searches := gh.count("search/code")
	if searches < 8 {
		t.Fatalf("only %d searches", searches)
	}
	if len(clock.sleeps) != searches-1 {
		t.Fatalf("%d searches but %d waits: %v", searches, len(clock.sleeps), clock.sleeps)
	}
	for _, d := range clock.sleeps {
		if d < 7*time.Second {
			t.Errorf("wait %v is shorter than 7s", d)
		}
	}
}

func TestDiscoverOnlyReadsThroughGET(t *testing.T) {
	_, _, gh, _ := runDiscover(t, mainWorld(t), mainRegistry(), t.TempDir(), newFakeClock())
	for _, req := range gh.calls {
		if strings.Contains(req.Path, "..") || strings.HasPrefix(req.Path, "/") {
			t.Errorf("odd path %q", req.Path)
		}
		if req.Path == "search/code" && req.Params["per_page"] != "100" {
			t.Errorf("search without per_page=100: %v", req.Params)
		}
	}
}

func TestSeedsMergeFoldersAndHints(t *testing.T) {
	reg := testRegistry(
		[]regFolder{
			{name: "Foo", hints: map[string]string{"x-curse-project-id": "1", "x-website": "https://github.com/Own/Foo.git"}},
			{name: "NoHints"},
		},
		regFolder{name: "foo", hints: map[string]string{"x-curse-project-id": "2", "x-source-repo": "https://github.com/Own/Foo", "x-wago-id": "bad id"}},
	)
	seeds, folders := loadSeeds(reg)
	if !slices.Equal(folders, []string{"Foo", "NoHints"}) || len(seeds) != 1 {
		t.Fatalf("folders = %v, seeds = %+v", folders, seeds)
	}
	s := seeds[0]
	if s.Folder != "Foo" || !slices.Equal(s.IDs[index.KindCurseForge], []string{"1", "2"}) {
		t.Errorf("seed = %+v", s)
	}
	if len(s.IDs[index.KindWago]) != 0 || len(s.BadHints) != 1 {
		t.Errorf("unusable id was kept: %+v", s)
	}
	if !slices.Equal(s.Repos, []string{"Own/Foo"}) {
		t.Errorf("repos = %v", s.Repos)
	}
}

func TestDiscoverRefusesMissingRegistryFile(t *testing.T) {
	if _, err := loadRegistryFile(t.TempDir() + "/missing.json"); err == nil {
		t.Fatal("missing registry file was accepted")
	}
	path := t.TempDir() + "/registry.json"
	if err := os.WriteFile(path, []byte(`{"version":1,"installations":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRegistryFile(path); err != nil {
		t.Fatal(err)
	}
}
