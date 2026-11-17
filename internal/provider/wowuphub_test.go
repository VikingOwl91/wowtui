package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestWowUpHubResolveLatestAndDownload(t *testing.T) {
	hub, _ := newHubTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/addons/42") {
			json.NewEncoder(w).Encode(map[string]any{"addon": map[string]any{
				"id": 42, "repository_name": "TestAddon", "homepage": "https://example.test",
				"releases": []map[string]any{{"id": 7, "tag_name": "v1.2.3", "download_url": "http://" + r.Host + "/download.zip", "game_versions": []map[string]any{{"game_type": "retail"}}}},
			}})
			return
		}
		if r.URL.Path == "/download.zip" {
			w.Write([]byte("zip"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	addon, err := hub.Resolve(context.Background(), Reference{Provider: "wowuphub", ID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	release, err := hub.Latest(context.Background(), addon, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.ID != 7 || release.Tag != "v1.2.3" {
		t.Errorf("release = %+v", release)
	}
	body, err := hub.Download(context.Background(), release)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "zip" {
		t.Errorf("data = %q", data)
	}
}

func TestWowUpHubLatestPerGameType(t *testing.T) {
	hub, _ := newHubTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/addons/42" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		releases := []map[string]any{}
		for id, gameType := range map[int]string{1: "retail", 2: "classic", 3: "burningCrusade", 4: "wotlk", 5: "mists", 6: "forever", 9: "cata"} {
			releases = append(releases, map[string]any{"id": id, "tag_name": gameType, "download_url": "http://" + r.Host + "/" + gameType + ".zip", "game_versions": []map[string]any{{"game_type": gameType}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"addon": map[string]any{"id": 42, "repository_name": "TestAddon", "releases": releases}})
	})

	want := map[wow.GameFlavor]int64{
		wow.Retail:        1,
		wow.PTR:           1,
		wow.XPTR:          1,
		wow.Beta:          1,
		wow.Classic:       5,
		wow.ClassicPTR:    5,
		wow.ClassicBeta:   6,
		wow.ClassicEra:    2,
		wow.ClassicEraPTR: 3,
		wow.Anniversary:   3,
		wow.Forever:       6,
	}
	addon := RemoteAddon{Provider: "wowuphub", ID: "42"}
	for flavor, id := range want {
		release, err := hub.Latest(context.Background(), addon, flavor, "")
		if err != nil {
			t.Errorf("Latest(%q): %v", flavor, err)
			continue
		}
		if release.ID != id {
			t.Errorf("Latest(%q) release = %d, want %d", flavor, release.ID, id)
		}
	}
	if _, err := hub.Latest(context.Background(), addon, wow.ClassicTitan, ""); err == nil {
		t.Error("Latest(classic-titan): expected error, WowUpHub has no Titan Reforged game type")
	}
}

func TestWowUpHubSearchPerGameType(t *testing.T) {
	var requested []string
	hub, _ := newHubTest(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, strings.TrimPrefix(r.URL.Path, "/addons/search/"))
		json.NewEncoder(w).Encode(map[string]any{"addons": []map[string]any{{"id": 42, "repository_name": "TestAddon"}}})
	})

	want := map[wow.GameFlavor]string{
		wow.Retail:        "retail",
		wow.PTR:           "retail",
		wow.XPTR:          "retail",
		wow.Beta:          "retail",
		wow.Classic:       "mists",
		wow.ClassicPTR:    "mists",
		wow.ClassicBeta:   "forever",
		wow.ClassicEra:    "classic",
		wow.ClassicEraPTR: "burningCrusade",
		wow.Anniversary:   "burningCrusade",
		wow.Forever:       "forever",
	}
	for flavor, gameType := range want {
		requested = nil
		results, err := hub.Search(context.Background(), "test", flavor)
		if err != nil {
			t.Errorf("Search(%q): %v", flavor, err)
			continue
		}
		if len(results) != 1 || len(requested) != 1 || requested[0] != gameType {
			t.Errorf("Search(%q) requested %v, got %d results; want game type %q", flavor, requested, len(results), gameType)
		}
	}

	requested = nil
	results, err := hub.Search(context.Background(), "test", wow.ClassicTitan)
	if err != nil || len(results) != 0 || len(requested) != 0 {
		t.Errorf("Search(classic-titan) = %v, %v after %v; want no request and no results", results, err, requested)
	}
}

func TestWowUpHubSearch(t *testing.T) {
	hub, _ := newHubTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/addons/search/retail") {
			json.NewEncoder(w).Encode(map[string]any{"addons": []map[string]any{{"id": 42, "repository_name": "TestAddon"}}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	results, err := hub.Search(context.Background(), "test", wow.Retail)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "42" {
		t.Errorf("results = %+v", results)
	}
}

func newHubTest(t *testing.T, handler http.HandlerFunc) (*WowUpHub, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	hub := NewWowUpHub()
	hub.SetBaseURL(srv.URL)
	return hub, srv
}
