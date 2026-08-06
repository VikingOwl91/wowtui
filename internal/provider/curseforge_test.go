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
