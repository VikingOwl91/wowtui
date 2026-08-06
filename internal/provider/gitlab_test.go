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

func newGitLabTest(t *testing.T, handler http.HandlerFunc) (*GitLab, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	gl := NewGitLab("secret")
	gl.SetAPIBase(srv.URL + "/api/v4")
	return gl, srv
}

func TestGitLabReleaseAndToken(t *testing.T) {
	var token string
	gl, srv := newGitLabTest(t, func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("PRIVATE-TOKEN")
		if strings.HasSuffix(r.URL.Path, "/releases/permalink/latest") {
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.2.3",
				"assets":   map[string]any{"sources": []map[string]any{{"format": "zip", "url": "http://" + r.Host + "/download/addon.zip"}}},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"path_with_namespace": "group/addon", "web_url": "https://gitlab.com/group/addon", "default_branch": "main"})
	})

	addon, err := gl.Resolve(context.Background(), Reference{Provider: "gitlab", ID: "group/addon"})
	if err != nil {
		t.Fatal(err)
	}
	release, err := gl.Latest(context.Background(), addon, wow.Retail, "release")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindRelease || release.Tag != "v1.2.3" || release.DownloadURL != srv.URL+"/download/addon.zip" {
		t.Errorf("release = %+v", release)
	}
	if token != "secret" {
		t.Errorf("token = %q", token)
	}
}

func TestGitLabFallsBackToTag(t *testing.T) {
	gl, srv := newGitLabTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/permalink/latest") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/repository/tags") {
			json.NewEncoder(w).Encode([]map[string]any{{"name": "v2.0.0"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	release, err := gl.Latest(context.Background(), RemoteAddon{Provider: "gitlab", ID: "group/addon"}, wow.Retail, "auto")
	if err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/group/addon/-/archive/v2.0.0/addon-v2.0.0.zip"
	if release.Kind != KindTag || release.Tag != "v2.0.0" || release.DownloadURL != want {
		t.Errorf("release = %+v, want URL %s", release, want)
	}
}

func TestGitLabDownload(t *testing.T) {
	gl, srv := newGitLabTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("zip"))
	})
	body, err := gl.Download(context.Background(), Release{DownloadURL: srv.URL + "/download.zip", AssetName: "download.zip"})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != "zip" {
		t.Errorf("data=%q err=%v", data, err)
	}
}
