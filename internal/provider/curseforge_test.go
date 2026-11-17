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

func TestCurseForgeResolveLatestAndDownload(t *testing.T) {
	cf, _ := newCurseForgeTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/mods/42" {
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": 42, "name": "TestAddon", "slug": "test-addon",
				"latestFilesIndexes": []map[string]any{{"fileId": 7, "gameVersionTypeId": 517}},
			}})
			return
		}
		if r.URL.Path == "/v1/games/1/version-types" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 517, "name": "WoW Retail", "slug": "retail"}}})
			return
		}
		if r.URL.Path == "/v1/mods/42/files/7" {
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 7, "fileName": "TestAddon.zip", "displayName": "1.2.3", "downloadUrl": "http://" + r.Host + "/download.zip"}})
			return
		}
		if r.URL.Path == "/download.zip" {
			w.Write([]byte("zip"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	addon, err := cf.Resolve(context.Background(), Reference{Provider: "curseforge", ID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	release, err := cf.Latest(context.Background(), addon, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.ID != 7 || release.Tag != "1.2.3" {
		t.Errorf("release = %+v", release)
	}
	body, err := cf.Download(context.Background(), release)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "zip" {
		t.Errorf("data = %q", data)
	}
}

func TestCurseForgeLatestPerGameType(t *testing.T) {
	fileByVersionType := map[int64]int64{517: 1, 67408: 2, 73246: 3, 73713: 4, 81212: 5, 77522: 6, 79434: 7, 88568: 8}
	cf, _ := newCurseForgeTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/mods/42":
			indexes := []map[string]any{}
			for versionType, fileID := range fileByVersionType {
				indexes = append(indexes, map[string]any{"fileId": fileID, "gameVersionTypeId": versionType})
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 42, "name": "TestAddon", "slug": "test-addon", "latestFilesIndexes": indexes}})
		case strings.HasPrefix(r.URL.Path, "/v1/mods/42/files/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/mods/42/files/")
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": json.Number(id), "fileName": "TestAddon-" + id + ".zip", "downloadUrl": "http://" + r.Host + "/" + id + ".zip"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	want := map[wow.GameFlavor]int64{
		wow.Retail:        1,
		wow.PTR:           1,
		wow.XPTR:          1,
		wow.Beta:          1,
		wow.Classic:       7,
		wow.ClassicPTR:    7,
		wow.ClassicBeta:   8,
		wow.ClassicEra:    2,
		wow.ClassicEraPTR: 3,
		wow.Anniversary:   3,
		wow.ClassicTitan:  5,
		wow.Forever:       8,
	}
	addon := RemoteAddon{Provider: "curseforge", ID: "42"}
	for _, flavor := range wow.Flavors() {
		release, err := cf.Latest(context.Background(), addon, flavor, "")
		if err != nil {
			t.Errorf("Latest(%q): %v", flavor, err)
			continue
		}
		if release.ID != want[flavor] {
			t.Errorf("Latest(%q) file = %d, want %d", flavor, release.ID, want[flavor])
		}
	}
	if _, err := cf.Latest(context.Background(), addon, "", ""); err == nil {
		t.Error("Latest with unknown flavor: expected error")
	}
}

func TestCurseForgeLatestNoFileForGameType(t *testing.T) {
	cf, _ := newCurseForgeTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/mods/42":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 42, "latestFilesIndexes": []map[string]any{{"fileId": 1, "gameVersionTypeId": 517}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if _, err := cf.Latest(context.Background(), RemoteAddon{Provider: "curseforge", ID: "42"}, wow.Anniversary, ""); err == nil {
		t.Fatal("expected error when the mod has no file for the game type")
	}
}

func TestCurseForgeSearch(t *testing.T) {
	cf, _ := newCurseForgeTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/mods/search") {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 42, "name": "TestAddon", "slug": "test-addon"}}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	results, err := cf.Search(context.Background(), "test", wow.Retail)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "42" {
		t.Errorf("results = %+v", results)
	}
}

func newCurseForgeTest(t *testing.T, handler http.HandlerFunc) (*CurseForge, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cf := NewCurseForge("test-key")
	cf.SetBaseURL(srv.URL)
	return cf, srv
}
