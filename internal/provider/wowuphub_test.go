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
