package main

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

var buildTime = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

func weakAurasPackage() Package {
	return Package{
		Repo:          "WeakAuras/WeakAuras2",
		Source:        "github:WeakAuras/WeakAuras2",
		Folders:       []string{"WeakAuras", "WeakAurasOptions", "bad/name", "weakauras", ".hidden"},
		SeedFolders:   []string{"WeakAuras"},
		PrimaryFolder: "WeakAuras",
		IDs:           map[string][]string{index.KindCurseForge: {"65387", "99"}, index.KindWago: {"abcWago"}},
		Title:         "|cff9482c9Weak|rAuras",
		Interfaces:    []int{110200},
		Release: &ReleaseInfo{
			Tag: "5.22.1", Assets: []string{"release.json"}, ReleaseJSON: true,
			ReleaseJSONGameTypes: []wow.GameType{wow.GameMainline, wow.GameVanilla},
		},
		FundingYML: "github: [WeakAuras]\npatreon: weak\nunknown_platform: x\n",
	}
}

func candidatesOf(pkgs ...Package) *Candidates {
	return &Candidates{Format: candidatesFormat, GeneratedAt: buildTime, Packages: pkgs}
}

func simplePackage(repo string, folders ...string) Package {
	return Package{
		Repo: repo, Source: "github:" + repo, Folders: folders, SeedFolders: folders[:1], PrimaryFolder: folders[0],
		Interfaces: []int{110200},
	}
}

func TestBuildEntryFields(t *testing.T) {
	res, err := Build(candidatesOf(weakAurasPackage()))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.File.Entries) != 1 {
		t.Fatalf("entries = %+v", res.File.Entries)
	}
	got := res.File.Entries[0]
	want := index.Entry{
		ID: "weakauras2", Name: "WeakAuras", Source: "github:WeakAuras/WeakAuras2",
		Folders:     []string{"WeakAuras", "WeakAurasOptions"},
		GameTypes:   []wow.GameType{wow.GameMainline, wow.GameVanilla},
		IDs:         index.ProviderIDs{CurseForge: "65387", Wago: "abcWago"},
		ReleaseJSON: true,
		Funding:     []index.Funding{{Platform: "github", Handle: "WeakAuras"}, {Platform: "patreon", Handle: "weak"}},
		Reviewed:    false,
	}
	if !equalJSON(t, got, want) {
		t.Errorf("entry = %+v\nwant   %+v", got, want)
	}
	if res.File.Format != index.FormatVersion || !res.File.GeneratedAt.Equal(buildTime) {
		t.Errorf("header = %d %v", res.File.Format, res.File.GeneratedAt)
	}
	joined := strings.Join(res.Warnings, "\n")
	for _, needle := range []string{"bad/name", ".hidden", "99", "unknown_platform"} {
		if !strings.Contains(joined, needle) {
			t.Errorf("warnings lack %q:\n%s", needle, joined)
		}
	}
}

func equalJSON(t *testing.T, a, b any) bool {
	t.Helper()
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	return string(x) == string(y)
}

func TestBuildNameStripsUIEscapes(t *testing.T) {
	cases := []struct{ title, repo, want string }{
		{"|cff9482c9Weak|r|cffffffffAuras|r", "o/Repo", "WeakAuras"},
		{"|TInterface\\Icons\\x:16|t  Boss Mods", "o/Repo", "Boss Mods"},
		{"|cff00ff00|r", "o/FallbackRepo", "FallbackRepo"},
		{"", "o/Plain", "Plain"},
	}
	for _, tc := range cases {
		pkg := simplePackage(tc.repo, "A")
		pkg.Title = tc.title
		res, err := Build(candidatesOf(pkg))
		if err != nil {
			t.Fatal(err)
		}
		if got := res.File.Entries[0].Name; got != tc.want {
			t.Errorf("title %q gave name %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestBuildGameTypesFromInterfaces(t *testing.T) {
	pkg := simplePackage("o/Multi", "Multi")
	pkg.Interfaces = []int{110200, 11508, 50503, 20505, 100, 70000, 11508}
	res, err := Build(candidatesOf(pkg))
	if err != nil {
		t.Fatal(err)
	}
	want := []wow.GameType{wow.GameMainline, wow.GameVanilla, wow.GameTBC, wow.GameMists}
	if got := res.File.Entries[0].GameTypes; !slices.Equal(got, want) {
		t.Errorf("game types = %v, want %v", got, want)
	}

	// release.json metadata wins over the TOC numbers.
	pkg.Release = &ReleaseInfo{ReleaseJSON: true, ReleaseJSONGameTypes: []wow.GameType{wow.GameCata}}
	res, err = Build(candidatesOf(pkg))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.File.Entries[0].GameTypes; !slices.Equal(got, []wow.GameType{wow.GameCata}) {
		t.Errorf("game types = %v", got)
	}

	// A release.json without known flavors falls back to the TOC numbers.
	pkg.Release = &ReleaseInfo{ReleaseJSON: true}
	res, err = Build(candidatesOf(pkg))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.File.Entries[0].GameTypes; !slices.Equal(got, want) || !res.File.Entries[0].ReleaseJSON {
		t.Errorf("entry = %+v", res.File.Entries[0])
	}
}

func TestBuildSlugsAreUniqueAndOrderIndependent(t *testing.T) {
	pkgs := []Package{
		simplePackage("a/Foo", "Foo"),
		simplePackage("b/Foo", "Foo2"),
		simplePackage("d/foo", "Foo3"),
		simplePackage("c/Bar_Add-On", "Bar"),
		simplePackage("e/__Weird__Name__", "Weird"),
		simplePackage("f/---", "Dashes"),
	}
	res, err := Build(candidatesOf(pkgs...))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range res.File.Entries {
		ids = append(ids, e.ID)
	}
	want := []string{"a-foo", "addon", "b-foo", "bar_add-on", "d-foo", "weird__name"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}

	slices.Reverse(pkgs)
	res2, err := Build(candidatesOf(pkgs...))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := marshalJSON(res.File)
	b, _ := marshalJSON(res2.File)
	if string(a) != string(b) {
		t.Errorf("slugs depend on input order:\n%s\n%s", a, b)
	}
}

func TestBuildSlugsForCasingCollisions(t *testing.T) {
	res, err := Build(candidatesOf(simplePackage("a/foo", "F1"), simplePackage("A/Foo", "F2"), simplePackage("b/foo", "F3")))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range res.File.Entries {
		if seen[e.ID] {
			t.Fatalf("duplicate id %q in %v", e.ID, res.File.Entries)
		}
		seen[e.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("ids = %v", seen)
	}
}

func TestBuildSkipsPackagesTheIndexWouldReject(t *testing.T) {
	noFolders := simplePackage("o/NoFolders", "a/b")
	noTypes := simplePackage("o/NoTypes", "NoTypes")
	noTypes.Interfaces = nil
	badSource := simplePackage("o/Good", "Good")
	badSource.Source = "https://example.org/x"
	good := simplePackage("o/Fine", "Fine")
	res, err := Build(candidatesOf(noFolders, noTypes, badSource, good))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.File.Entries) != 1 || res.File.Entries[0].Source != "github:o/Fine" {
		t.Fatalf("entries = %+v", res.File.Entries)
	}
	joined := strings.Join(res.Warnings, "\n")
	for _, repo := range []string{"o/NoFolders", "o/NoTypes", "o/Good"} {
		if !strings.Contains(joined, repo) {
			t.Errorf("no warning for %s:\n%s", repo, joined)
		}
	}
}

func TestBuildDropsProviderIDsAlreadyTaken(t *testing.T) {
	a := simplePackage("a/Alpha", "Alpha")
	a.IDs = map[string][]string{index.KindCurseForge: {"1"}, index.KindWago: {"w"}}
	b := simplePackage("b/Beta", "Beta")
	b.IDs = map[string][]string{index.KindCurseForge: {"1"}, index.KindWoWInterface: {"7"}}
	res, err := Build(candidatesOf(b, a))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]index.Entry{}
	for _, e := range res.File.Entries {
		byID[e.ID] = e
	}
	if byID["alpha"].IDs.CurseForge != "1" || byID["beta"].IDs.CurseForge != "" || byID["beta"].IDs.WoWInterface != "7" {
		t.Errorf("entries = %+v", res.File.Entries)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "already used") {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	pkgs := []Package{weakAurasPackage()}
	for _, name := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"} {
		p := simplePackage("o/"+name, name, name+"_Options")
		p.Interfaces = []int{110200, 11508, 20505}
		p.IDs = map[string][]string{index.KindWago: {name + "W"}}
		pkgs = append(pkgs, p)
	}
	first, err := Build(candidatesOf(pkgs...))
	if err != nil {
		t.Fatal(err)
	}
	want, err := encodeIndex(first.File)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	for range 10 {
		shuffled := slices.Clone(pkgs)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		res, err := Build(candidatesOf(shuffled...))
		if err != nil {
			t.Fatal(err)
		}
		got, err := encodeIndex(res.File)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("output depends on package order:\n%s\nvs\n%s", got, want)
		}
	}
	if !strings.HasSuffix(string(want), "}\n") {
		t.Error("index.json must end with a newline")
	}
}

func TestBuildRejectsBadCandidates(t *testing.T) {
	if _, err := Build(&Candidates{Format: candidatesFormat}); err == nil {
		t.Error("candidates without a generation time were accepted")
	}
	if _, err := Build(&Candidates{Format: 99, GeneratedAt: buildTime}); err == nil {
		t.Error("candidates with an unknown format were accepted")
	}
}

func writeKeyPair(t *testing.T) (dir string, pub ed25519.PublicKey) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "keys")
	if err := keygen(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "signing.pub"))
	if err != nil {
		t.Fatal(err)
	}
	pub, err = index.DecodePublicKey(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return dir, pub
}

func writeCandidatesFile(t *testing.T, cands *Candidates) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "candidates.json")
	if err := writeJSONFile(path, cands); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildAndWriteSignsAndVerifies(t *testing.T) {
	keyDir, pub := writeKeyPair(t)
	candPath := writeCandidatesFile(t, candidatesOf(weakAurasPackage(), simplePackage("o/Other", "Other")))
	out := filepath.Join(t.TempDir(), "index.json")
	res, err := buildFiles(candPath, filepath.Join(keyDir, "signing.key"), out, index.Load)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries != 2 {
		t.Fatalf("entries = %d", res.Entries)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(out + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Load(data, sig, pub)
	if err != nil {
		t.Fatalf("signature round trip: %v", err)
	}
	if len(idx.Entries()) != 2 {
		t.Fatalf("loaded entries = %d", len(idx.Entries()))
	}
	if found, ok := idx.ByProviderID(index.KindCurseForge, "65387"); !ok || found.ID != "weakauras2" {
		t.Errorf("lookup by provider id = %+v, %v", found, ok)
	}

	tampered := slices.Clone(data)
	tampered[len(tampered)/2] ^= 1
	if _, err := index.Load(tampered, sig, pub); !errors.Is(err, index.ErrInvalidSignature) {
		t.Errorf("tampered index: %v", err)
	}
	otherPub, _, _ := index.GenerateKey()
	if _, err := index.Load(data, sig, otherPub); !errors.Is(err, index.ErrInvalidSignature) {
		t.Errorf("wrong key: %v", err)
	}
}

func TestBuildFilesWritesNothingWhenVerificationFails(t *testing.T) {
	keyDir, _ := writeKeyPair(t)
	candPath := writeCandidatesFile(t, candidatesOf(simplePackage("o/Other", "Other")))
	outDir := t.TempDir()
	out := filepath.Join(outDir, "index.json")
	failing := func([]byte, []byte, ed25519.PublicKey) (*index.Index, error) {
		return nil, errors.New("boom")
	}
	if _, err := buildFiles(candPath, filepath.Join(keyDir, "signing.key"), out, failing); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
	left, _ := os.ReadDir(outDir)
	if len(left) != 0 {
		t.Fatalf("files left behind: %v", left)
	}
}

func TestBuildFilesRefusesLooseKeyPermissionsAndBadKeys(t *testing.T) {
	keyDir, _ := writeKeyPair(t)
	keyPath := filepath.Join(keyDir, "signing.key")
	candPath := writeCandidatesFile(t, candidatesOf(simplePackage("o/Other", "Other")))
	out := filepath.Join(t.TempDir(), "index.json")

	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := buildFiles(candPath, keyPath, out, index.Load); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("loose key permissions: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := buildFiles(candPath, keyPath, out, index.Load); err == nil {
		t.Fatal("garbage key was accepted")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("index written despite the key error")
	}
}

func TestKeygen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if err := keygen(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, %v", info, err)
	}
	keyInfo, err := os.Stat(filepath.Join(dir, "signing.key"))
	if err != nil || keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v, %v", keyInfo, err)
	}
	keyText, _ := os.ReadFile(filepath.Join(dir, "signing.key"))
	pubText, _ := os.ReadFile(filepath.Join(dir, "signing.pub"))
	priv, err := index.DecodePrivateKey(string(keyText))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := index.DecodePublicKey(string(pubText))
	if err != nil {
		t.Fatal(err)
	}
	if !priv.Public().(ed25519.PublicKey).Equal(pub) {
		t.Fatal("public key does not belong to the private key")
	}
	if strings.Contains(string(pubText), string(keyText[:20])) {
		t.Fatal("public file leaks key material")
	}

	// Never overwrite: not a second run, and not when only one file exists.
	if err := keygen(dir); err == nil {
		t.Fatal("second keygen overwrote the keys")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "signing.key"))
	if string(after) != string(keyText) {
		t.Fatal("private key changed")
	}
	partial := t.TempDir()
	if err := os.WriteFile(filepath.Join(partial, "signing.pub"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := keygen(partial); err == nil {
		t.Fatal("keygen ignored an existing signing.pub")
	}
	if _, err := os.Stat(filepath.Join(partial, "signing.key")); err == nil {
		t.Fatal("a key was written next to an existing signing.pub")
	}
}
