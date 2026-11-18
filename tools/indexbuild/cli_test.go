package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/index"
)

func runCLI(t *testing.T, d deps, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func TestCLIUsageErrors(t *testing.T) {
	d := deps{gh: &fakeGH{handler: func(Request) Response { return notFound() }}, clock: newFakeClock()}
	cases := [][]string{
		{},
		{"nope"},
		{"keygen"},
		{"discover", "--registry", "r.json"},
		{"discover", "--out", "c.json", "--cache", "c"},
		{"build", "--candidates", "c.json", "--key", "k"},
		{"build", "--bogus"},
	}
	for _, args := range cases {
		code, _, stderr := runCLI(t, d, args...)
		if code != 2 || !strings.Contains(stderr, "usage") && !strings.Contains(stderr, "Usage") && !strings.Contains(stderr, "required") {
			t.Errorf("run(%v) = %d, stderr %q", args, code, stderr)
		}
	}
}

func TestCLIKeygenRefusesOverwrite(t *testing.T) {
	d := deps{clock: newFakeClock()}
	dir := filepath.Join(t.TempDir(), "keys")
	if code, _, stderr := runCLI(t, d, "keygen", "--out", dir); code != 0 {
		t.Fatalf("keygen = %d, %s", code, stderr)
	}
	code, _, stderr := runCLI(t, d, "keygen", "--out", dir)
	if code != 1 || !strings.Contains(stderr, "refusing to overwrite") {
		t.Fatalf("second keygen = %d, %q", code, stderr)
	}
}

func TestCLIDiscoverNeedsRegistryFile(t *testing.T) {
	d := deps{gh: &fakeGH{handler: func(Request) Response { return notFound() }}, clock: newFakeClock()}
	dir := t.TempDir()
	code, _, stderr := runCLI(t, d, "discover", "--registry", filepath.Join(dir, "missing.json"), "--out", filepath.Join(dir, "c.json"), "--cache", filepath.Join(dir, "cache"))
	if code != 1 || stderr == "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "c.json")); err == nil {
		t.Fatal("candidates written without a registry")
	}
}

func TestCLIEndToEnd(t *testing.T) {
	w := mainWorld(t)
	gh := &fakeGH{handler: w.handle}
	d := deps{gh: gh, clock: newFakeClock()}
	dir := t.TempDir()
	regPath := filepath.Join(dir, "registry.json")
	if err := mainRegistry().Save(regPath); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	cands := filepath.Join(dir, "candidates.json")
	keys := filepath.Join(dir, "keys")

	if code, _, stderr := runCLI(t, d, "keygen", "--out", keys); code != 0 {
		t.Fatalf("keygen: %d %s", code, stderr)
	}
	code, stdout, stderr := runCLI(t, d, "discover", "--registry", regPath, "--out", cands, "--cache", cache)
	if code != 0 {
		t.Fatalf("discover: %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "5 packages") {
		t.Errorf("discover summary = %q", stdout)
	}
	reviewPath := filepath.Join(dir, "candidates.review.json")
	reviewData, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("review file: %v", err)
	}
	if !strings.Contains(string(reviewData), `"reason": "ambiguous"`) {
		t.Errorf("review file lacks the ambiguous item:\n%s", reviewData)
	}
	firstCalls := len(gh.calls)

	// A rerun on the same cache makes no GitHub call and writes the same bytes.
	candBytes, _ := os.ReadFile(cands)
	if code, _, stderr := runCLI(t, d, "discover", "--registry", regPath, "--out", cands, "--cache", cache); code != 0 {
		t.Fatalf("second discover: %d\n%s", code, stderr)
	}
	if len(gh.calls) != firstCalls {
		t.Errorf("rerun made %d new calls", len(gh.calls)-firstCalls)
	}
	again, _ := os.ReadFile(cands)
	if string(again) != string(candBytes) {
		t.Error("candidates.json differs on rerun")
	}

	out := filepath.Join(dir, "index.json")
	code, stdout, stderr = runCLI(t, d, "build", "--candidates", cands, "--key", filepath.Join(keys, "signing.key"), "--out", out)
	if code != 0 {
		t.Fatalf("build: %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "signature verified") {
		t.Errorf("build output = %q", stdout)
	}
	first, _ := os.ReadFile(out)
	sig, _ := os.ReadFile(out + ".sig")
	pubText, _ := os.ReadFile(filepath.Join(keys, "signing.pub"))
	pub, err := index.DecodePublicKey(string(pubText))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Load(first, sig, pub)
	if err != nil {
		t.Fatalf("index.Load: %v", err)
	}
	var ids []string
	for _, e := range idx.Entries() {
		ids = append(ids, e.ID)
		if e.Reviewed {
			t.Errorf("%s is marked reviewed", e.ID)
		}
	}
	want := []string{"details-damage-meter", "forever-core", "pawn", "questie", "weakauras2"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if hit := idx.ByFolder("WeakAurasTemplates"); len(hit) != 1 || hit[0].Name != "WeakAuras" || !hit[0].ReleaseJSON {
		t.Errorf("ByFolder(WeakAurasTemplates) = %+v", hit)
	}
	if hit := idx.ByFolder("Details_Vanguard"); len(hit) != 1 || hit[0].ID != "details-damage-meter" {
		t.Errorf("ByFolder(Details_Vanguard) = %+v", hit)
	}

	// A rebuild from the same candidates is byte-identical.
	out2 := filepath.Join(dir, "second", "index.json")
	if code, _, stderr := runCLI(t, d, "build", "--candidates", cands, "--key", filepath.Join(keys, "signing.key"), "--out", out2); code != 0 {
		t.Fatalf("second build: %d\n%s", code, stderr)
	}
	second, _ := os.ReadFile(out2)
	secondSig, _ := os.ReadFile(out2 + ".sig")
	if string(first) != string(second) || string(sig) != string(secondSig) {
		t.Error("rebuilding the same candidates changed index.json or its signature")
	}
}
