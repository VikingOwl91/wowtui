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
	gh.webBase = srv.URL
	return gh, srv
}

func testHandler(t *testing.T, authSeen *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if authSeen != nil {
			*authSeen = r.Header.Get("Authorization")
		}
		writeJSON := func(v any) { json.NewEncoder(w).Encode(v) }

		switch r.URL.Path {
		case "/repos/Owner/Repo":
			writeJSON(map[string]any{"full_name": "Owner/Repo", "html_url": "https://github.com/Owner/Repo", "default_branch": "main"})
		case "/repos/Owner/Repo/releases/latest":
			writeJSON(map[string]any{
				"id":       4242,
				"tag_name": "v1.2.3",
				"assets": []map[string]any{{
					"name":                 "Repo-1.2.3.zip",
					"browser_download_url": "http://" + r.Host + "/dl/Repo-1.2.3.zip",
					"size":                 1234,
				}},
			})
		case "/repos/Owner/Repo/tags":
			writeJSON([]map[string]any{{"name": "v0.1.0"}})

		case "/repos/Owner/NoZipRelease":
			writeJSON(map[string]any{"full_name": "Owner/NoZipRelease", "html_url": "x", "default_branch": "main"})
		case "/repos/Owner/NoZipRelease/releases/latest":
			writeJSON(map[string]any{
				"id":       1,
				"tag_name": "v2.0.0",
				"assets":   []map[string]any{{"name": "sources.tar.gz", "browser_download_url": "x", "size": 1}},
			})

		case "/repos/Owner/TagsOnly":
			writeJSON(map[string]any{"full_name": "Owner/TagsOnly", "html_url": "x", "default_branch": "main"})
		case "/repos/Owner/TagsOnly/releases/latest":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/Owner/TagsOnly/tags":
			writeJSON([]map[string]any{{"name": "v1.2.3"}, {"name": "v1.10.0"}, {"name": "v1.9.0"}})

		case "/repos/Owner/BranchOnly":
			writeJSON(map[string]any{"full_name": "Owner/BranchOnly", "html_url": "x", "default_branch": "main"})
		case "/repos/Owner/BranchOnly/releases/latest":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/Owner/BranchOnly/tags":
			writeJSON([]map[string]any{})
		case "/repos/Owner/BranchOnly/commits":
			writeJSON([]map[string]any{{"sha": "abc123def456abc123def456abc123def456abcd"}})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestGitHubResolve(t *testing.T) {
	gh, _ := newTestGitHub(t, testHandler(t, nil), "")
	addon, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err != nil {
		t.Fatal(err)
	}
	if addon.Name != "Owner/Repo" || addon.URL != "https://github.com/Owner/Repo" {
		t.Errorf("got %+v", addon)
	}
}

func TestGitHubResolveMissingRepo(t *testing.T) {
	gh, _ := newTestGitHub(t, testHandler(t, nil), "")
	_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Missing/Repo"})
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestGitHubLatestReleaseWithZip(t *testing.T) {
	gh, srv := newTestGitHub(t, testHandler(t, nil), "")
	release, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Repo"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindRelease || release.ID != 4242 || release.Tag != "v1.2.3" {
		t.Errorf("got %+v", release)
	}
	if release.AssetName != "Repo-1.2.3.zip" || release.Size != 1234 {
		t.Errorf("got %+v", release)
	}
	if release.DownloadURL != srv.URL+"/dl/Repo-1.2.3.zip" {
		t.Errorf("download url = %q", release.DownloadURL)
	}
}

func TestGitHubLatestNoZipFallsBackToReleaseTag(t *testing.T) {
	gh, srv := newTestGitHub(t, testHandler(t, nil), "")
	release, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/NoZipRelease"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindTag || release.Tag != "v2.0.0" {
		t.Errorf("got %+v", release)
	}
	if release.DownloadURL != srv.URL+"/Owner/NoZipRelease/archive/refs/tags/v2.0.0.zip" {
		t.Errorf("download url = %q", release.DownloadURL)
	}
}

func TestGitHubLatestFallsBackToNewestTag(t *testing.T) {
	gh, srv := newTestGitHub(t, testHandler(t, nil), "")
	release, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/TagsOnly"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindTag || release.Tag != "v1.10.0" {
		t.Errorf("got %+v, want newest tag v1.10.0", release)
	}
	if release.DownloadURL != srv.URL+"/Owner/TagsOnly/archive/refs/tags/v1.10.0.zip" {
		t.Errorf("download url = %q", release.DownloadURL)
	}
}

func TestGitHubLatestFallsBackToBranch(t *testing.T) {
	gh, srv := newTestGitHub(t, testHandler(t, nil), "")
	release, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/BranchOnly"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindBranch || release.Tag != "main" {
		t.Errorf("got %+v", release)
	}
	if release.Commit != "abc123def456abc123def456abc123def456abcd" {
		t.Errorf("commit = %q", release.Commit)
	}
	if release.DownloadURL != srv.URL+"/Owner/BranchOnly/archive/refs/heads/main.zip" {
		t.Errorf("download url = %q", release.DownloadURL)
	}
}

func TestGitHubPinnedTagSkipsRelease(t *testing.T) {
	gh, srv := newTestGitHub(t, testHandler(t, nil), "")
	release, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Repo"}, wow.Retail, "tag")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindTag || release.Tag != "v0.1.0" {
		t.Errorf("got %+v, want pinned tag v0.1.0", release)
	}
	if release.DownloadURL != srv.URL+"/Owner/Repo/archive/refs/tags/v0.1.0.zip" {
		t.Errorf("download url = %q", release.DownloadURL)
	}
}

func TestGitHubPinnedReleaseErrorsWithoutRelease(t *testing.T) {
	gh, _ := newTestGitHub(t, testHandler(t, nil), "")
	_, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/TagsOnly"}, wow.Retail, "release")
	if err == nil {
		t.Fatal("expected error for pinned release source without releases")
	}
}

func TestGitHubUnknownSourceErrors(t *testing.T) {
	gh, _ := newTestGitHub(t, testHandler(t, nil), "")
	_, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Repo"}, wow.Retail, "bogus")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestGitHubSendsToken(t *testing.T) {
	var auth string
	gh, _ := newTestGitHub(t, testHandler(t, &auth), "secret-token")
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
	gh, _ := newTestGitHub(t, testHandler(t, &auth), "")
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
