package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

type packagerHosts struct {
	gh      *GitHub
	mu      sync.Mutex
	dlAuth  []string
	dlPaths []string
}

func (p *packagerHosts) seen() (auth, paths []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.dlAuth...), append([]string(nil), p.dlPaths...)
}

func (p *packagerHosts) record(r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dlAuth = append(p.dlAuth, r.Header.Get("Authorization"))
	p.dlPaths = append(p.dlPaths, r.URL.Path)
}

// newPackagerGitHub serves a release on the API host whose assets live on a
// separate download host that redirects to a third objects host, like
// github.com redirecting to objects.githubusercontent.com.
func newPackagerGitHub(t *testing.T, token string, assets []string, serveReleaseJSON http.HandlerFunc) *packagerHosts {
	t.Helper()
	hosts := &packagerHosts{}

	objects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts.record(r)
		switch {
		case r.URL.Path == "/obj/release.json" && serveReleaseJSON != nil:
			serveReleaseJSON(w, r)
		case strings.HasPrefix(r.URL.Path, "/obj/") && r.URL.Path != "/obj/release.json":
			w.Write([]byte("zip-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(objects.Close)

	downloads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts.record(r)
		if name, found := strings.CutPrefix(r.URL.Path, "/dl/"); found {
			http.Redirect(w, r, objects.URL+"/obj/"+name, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(downloads.Close)

	hosts.gh, _ = newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/repos/Owner/Pkg/releases/latest" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		list := make([]map[string]any, 0, len(assets))
		for i, name := range assets {
			list = append(list, map[string]any{
				"name":                 name,
				"browser_download_url": downloads.URL + "/dl/" + name,
				"size":                 100 + i,
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"id": 77, "tag_name": "v1", "assets": list})
	}, token)
	return hosts
}

func serveString(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}
}

const packagerManifest = `{"releases":[
{"name":"Pkg","version":"v1","filename":"Pkg-B.zip","nolib":false,"metadata":[{"flavor":"classic","interface":11507}]},
{"name":"Pkg","version":"v1","filename":"Pkg-C.zip","nolib":false,"metadata":[{"flavor":"bcc","interface":20505}]},
{"name":"Pkg","version":"v1","filename":"Pkg-A.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":110200},{"flavor":"mainline","interface":110205}]}
]}`

func TestGitHubReleaseJSONPicksAssetPerGameType(t *testing.T) {
	assets := []string{"Pkg-D.zip", "Pkg-B.zip", "Pkg-C.zip", "Pkg-A.zip", "release.json"}
	tests := []struct {
		flavor wow.GameFlavor
		want   string
	}{
		{wow.Retail, "Pkg-A.zip"},
		{wow.ClassicEra, "Pkg-B.zip"},
		{wow.Anniversary, "Pkg-C.zip"},
		{wow.PTR, "Pkg-A.zip"},
		{wow.ClassicEraPTR, "Pkg-C.zip"},
	}
	for _, tt := range tests {
		t.Run(string(tt.flavor), func(t *testing.T) {
			hosts := newPackagerGitHub(t, "", assets, serveString(packagerManifest))
			release, err := hosts.gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Pkg"}, tt.flavor, "")
			if err != nil {
				t.Fatal(err)
			}
			if release.Kind != KindRelease || release.AssetName != tt.want {
				t.Errorf("got %+v, want asset %s", release, tt.want)
			}
			if !strings.HasSuffix(release.DownloadURL, "/dl/"+tt.want) {
				t.Errorf("download url = %q", release.DownloadURL)
			}
		})
	}
}

func TestGitHubReleaseJSONPrefersFullBuildOverNolib(t *testing.T) {
	manifest := `{"releases":[
{"name":"Pkg","version":"v1","filename":"Pkg-v1-nolib.zip","nolib":true,"metadata":[{"flavor":"mainline","interface":110200}]},
{"name":"Pkg","version":"v1","filename":"Pkg-v1.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":110200}]}
]}`
	hosts := newPackagerGitHub(t, "", []string{"Pkg-v1-nolib.zip", "Pkg-v1.zip", "release.json"}, serveString(manifest))
	release, err := hosts.gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Pkg"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.AssetName != "Pkg-v1.zip" {
		t.Errorf("asset = %q, want the full build Pkg-v1.zip", release.AssetName)
	}
}

func TestGitHubReleaseJSONIsAuthoritativeForMissingGameType(t *testing.T) {
	manifest := `{"releases":[
{"name":"Pkg","version":"v1","filename":"Pkg-v1.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":110200},{"flavor":"classic","interface":11507}]}
]}`
	hosts := newPackagerGitHub(t, "", []string{"Pkg-v1.zip", "Pkg-v1-bcc.zip", "release.json"}, serveString(manifest))
	_, err := hosts.gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Pkg"}, wow.Anniversary, "")
	if err == nil {
		t.Fatal("expected an error when release.json lists no build for the game type")
	}
	for _, want := range []string{"Owner/Pkg", "release.json", "tbc", "mainline, vanilla"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestGitHubReleaseJSONFallsBackToHeuristic(t *testing.T) {
	assets := []string{"Pkg-v1.zip", "Pkg-v1-classic.zip"}
	withManifest := append(append([]string(nil), assets...), "release.json")
	tests := []struct {
		name   string
		assets []string
		serve  http.HandlerFunc
	}{
		{"missing asset", assets, serveString(packagerManifest)},
		{"fetch fails", withManifest, nil},
		{"malformed", withManifest, serveString(`{"releases":[{"filename":`)},
		{"no entries", withManifest, serveString(`{"releases":[]}`)},
		{"unknown filename", withManifest, serveString(`{"releases":[{"filename":"Other.zip","nolib":false,"metadata":[{"flavor":"classic","interface":11507}]}]}`)},
		{"too large", withManifest, serveString(`{"releases":[{"filename":"Pkg-v1.zip","nolib":false,"metadata":[{"flavor":"classic","interface":11507}]}],"pad":"` + strings.Repeat("x", 1<<20) + `"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts := newPackagerGitHub(t, "", tt.assets, tt.serve)
			release, err := hosts.gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Pkg"}, wow.ClassicEra, "")
			if err != nil {
				t.Fatal(err)
			}
			if release.Kind != KindRelease || release.AssetName != "Pkg-v1-classic.zip" {
				t.Errorf("got %+v, want heuristic pick Pkg-v1-classic.zip", release)
			}
		})
	}
}

func TestGitHubReleaseJSONSkipsFetchWithoutAsset(t *testing.T) {
	hosts := newPackagerGitHub(t, "", []string{"Pkg-v1.zip"}, serveString(packagerManifest))
	if _, err := hosts.gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Pkg"}, wow.Retail, ""); err != nil {
		t.Fatal(err)
	}
	if _, paths := hosts.seen(); len(paths) != 0 {
		t.Errorf("download hosts saw %v, want no requests", paths)
	}
}

func TestGitHubReleaseJSONNeverSendsTokenToDownloadHosts(t *testing.T) {
	hosts := newPackagerGitHub(t, "dummy-token", []string{"Pkg-B.zip", "Pkg-C.zip", "Pkg-A.zip", "release.json"}, serveString(packagerManifest))
	release, err := hosts.gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Pkg"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.AssetName != "Pkg-A.zip" {
		t.Fatalf("asset = %q, want Pkg-A.zip", release.AssetName)
	}
	body, err := hosts.gh.Download(context.Background(), release)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()

	auth, paths := hosts.seen()
	wantPaths := []string{"/dl/release.json", "/obj/release.json", "/dl/Pkg-A.zip", "/obj/Pkg-A.zip"}
	if strings.Join(paths, " ") != strings.Join(wantPaths, " ") {
		t.Errorf("download hosts saw %v, want %v", paths, wantPaths)
	}
	for i, value := range auth {
		if value != "" {
			t.Errorf("request %s carried Authorization %q", paths[i], value)
		}
	}
}

func rateLimitedGitHub(t *testing.T, status int, remaining, reset, token string) *GitHub {
	t.Helper()
	gh, _ := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if remaining != "" {
			w.Header().Set("X-RateLimit-Remaining", remaining)
		}
		if reset != "" {
			w.Header().Set("X-RateLimit-Reset", reset)
		}
		w.WriteHeader(status)
	}, token)
	return gh
}

func TestGitHubRateLimitExhaustedWithoutToken(t *testing.T) {
	reset := time.Date(2026, 11, 18, 14, 32, 10, 0, time.UTC).Unix()
	resetAt := time.Unix(reset, 0).Format("15:04:05 MST")
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			gh := rateLimitedGitHub(t, status, "0", strconv.FormatInt(reset, 10), "")
			_, resolveErr := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
			_, latestErr := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Repo"}, wow.Retail, "")
			for _, err := range []error{resolveErr, latestErr} {
				if err == nil {
					t.Fatal("expected a rate limit error")
				}
				for _, want := range []string{"rate limit exhausted", "resets at " + resetAt, "WOWTUI_GITHUB_TOKEN", "providers.github.token"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not mention %q", err, want)
					}
				}
			}
		})
	}
}

func TestGitHubRateLimitExhaustedWithToken(t *testing.T) {
	reset := time.Date(2026, 11, 18, 14, 32, 10, 0, time.UTC).Unix()
	gh := rateLimitedGitHub(t, http.StatusForbidden, "0", strconv.FormatInt(reset, 10), "dummy-token")
	_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err == nil {
		t.Fatal("expected a rate limit error")
	}
	if !strings.Contains(err.Error(), "rate limit exhausted") || !strings.Contains(err.Error(), "resets at "+time.Unix(reset, 0).Format("15:04:05 MST")) {
		t.Errorf("error = %q", err)
	}
	if strings.Contains(err.Error(), "WOWTUI_GITHUB_TOKEN") || strings.Contains(err.Error(), "dummy-token") {
		t.Errorf("error %q suggests a token although one is configured, or leaks it", err)
	}
}

func TestGitHubRateLimitExhaustedWithoutReset(t *testing.T) {
	gh := rateLimitedGitHub(t, http.StatusForbidden, "0", "soon", "")
	_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err == nil {
		t.Fatal("expected a rate limit error")
	}
	if !strings.Contains(err.Error(), "rate limit exhausted") || strings.Contains(err.Error(), "resets at") {
		t.Errorf("error = %q, want no reset time without a valid X-RateLimit-Reset", err)
	}
}

func TestGitHubForbiddenWithRemainingQuotaIsNoRateLimit(t *testing.T) {
	for _, remaining := range []string{"", "12"} {
		gh := rateLimitedGitHub(t, http.StatusForbidden, remaining, "1795000000", "")
		_, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
		if err == nil {
			t.Fatal("expected an error for 403")
		}
		if strings.Contains(err.Error(), "rate limit") {
			t.Errorf("remaining %q: error %q claims a rate limit", remaining, err)
		}
	}
}
