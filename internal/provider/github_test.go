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

func newTestGitHub(t *testing.T, handler http.HandlerFunc, token string) (*GitHub, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	gh := NewGitHub(token)
	gh.apiBase = srv.URL
	return gh, srv
}

func repoHandler(t *testing.T, authSeen *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if authSeen != nil {
			*authSeen = r.Header.Get("Authorization")
		}
		switch r.URL.Path {
		case "/repos/Owner/Repo":
			json.NewEncoder(w).Encode(map[string]any{
				"full_name": "Owner/Repo",
				"html_url":  "https://github.com/Owner/Repo",
			})
		case "/repos/Owner/Repo/releases/latest":
			json.NewEncoder(w).Encode(map[string]any{
				"id":       4242,
				"tag_name": "v1.2.3",
				"assets": []map[string]any{
					{"name": "Repo-1.2.3.zip", "browser_download_url": "http://" + r.Host + "/dl/Repo-1.2.3.zip", "size": 1234},
				},
			})
		case "/repos/Owner/NoRelease/releases/latest":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/Owner/NoRelease":
			json.NewEncoder(w).Encode(map[string]any{
				"full_name": "Owner/NoRelease",
				"html_url":  "https://github.com/Owner/NoRelease",
			})
		case "/repos/Missing/Repo", "/repos/Missing/Repo/releases/latest":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestGitHubResolve(t *testing.T) {
	gh, _ := newTestGitHub(t, repoHandler(t, nil), "")
	addon, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err != nil {
		t.Fatal(err)
	}
	if addon.Name != "Owner/Repo" || addon.URL != "https://github.com/Owner/Repo" {
		t.Errorf("got %+v", addon)
	}
}

func TestGitHubResolveMissingRepo(t *testing.T) {
	gh, _ := newTestGitHub(t, repoHandler(t, nil), "")
	_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Missing/Repo"})
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestGitHubLatestRelease(t *testing.T) {
	gh, srv := newTestGitHub(t, repoHandler(t, nil), "")
	release, err := gh.LatestRelease(context.Background(),
		RemoteAddon{Provider: "github", ID: "Owner/Repo"}, wow.Retail)
	if err != nil {
		t.Fatal(err)
	}
	if release.ID != 4242 || release.Tag != "v1.2.3" {
		t.Errorf("got %+v", release)
	}
	if release.AssetName != "Repo-1.2.3.zip" || release.Size != 1234 {
		t.Errorf("got %+v", release)
	}
	if release.DownloadURL != srv.URL+"/dl/Repo-1.2.3.zip" {
		t.Errorf("download url = %q", release.DownloadURL)
	}
}

func TestGitHubLatestReleaseNoReleases(t *testing.T) {
	gh, _ := newTestGitHub(t, repoHandler(t, nil), "")
	_, err := gh.LatestRelease(context.Background(),
		RemoteAddon{Provider: "github", ID: "Owner/NoRelease"}, wow.Retail)
	if err == nil {
		t.Fatal("expected error for repo without releases")
	}
}

func TestGitHubSendsToken(t *testing.T) {
	var auth string
	gh, _ := newTestGitHub(t, repoHandler(t, &auth), "secret-token")
	_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", auth)
	}
}

func TestGitHubOmitsAuthWithoutToken(t *testing.T) {
	var auth string
	gh, _ := newTestGitHub(t, repoHandler(t, &auth), "")
	_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want empty", auth)
	}
}

func TestGitHubDownload(t *testing.T) {
	gh, srv := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dl/addon.zip" {
			w.Write([]byte("zip-bytes"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}, "")

	body, err := gh.Download(context.Background(), Release{DownloadURL: srv.URL + "/dl/addon.zip"})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "zip-bytes" {
		t.Errorf("downloaded %q", data)
	}
}

func TestGitHubDownloadErrorStatus(t *testing.T) {
	gh, srv := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}, "")

	if _, err := gh.Download(context.Background(), Release{DownloadURL: srv.URL + "/dl/addon.zip"}); err == nil {
		t.Fatal("expected error for non-200 download")
	}
}

func TestGitHubImplementsProvider(t *testing.T) {
	var _ Provider = (*GitHub)(nil)
	if !strings.HasPrefix(NewGitHub("").apiBase, "https://") {
		t.Error("default api base must be https")
	}
}
