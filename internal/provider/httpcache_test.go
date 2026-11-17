package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type conditionalServer struct {
	mu          sync.Mutex
	ifNoneMatch []string
	ifModified  []string
	full        int
	etag        string
	modified    string
	description string
}

func (c *conditionalServer) seen() (ifNoneMatch, ifModified []string, full int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.ifNoneMatch...), append([]string(nil), c.ifModified...), c.full
}

func (c *conditionalServer) handler(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/repos/Owner/Repo" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.ifNoneMatch = append(c.ifNoneMatch, r.Header.Get("If-None-Match"))
		c.ifModified = append(c.ifModified, r.Header.Get("If-Modified-Since"))
		if (c.etag != "" && r.Header.Get("If-None-Match") == c.etag) ||
			(c.modified != "" && r.Header.Get("If-Modified-Since") == c.modified) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		c.full++
		if c.etag != "" {
			w.Header().Set("ETag", c.etag)
		}
		if c.modified != "" {
			w.Header().Set("Last-Modified", c.modified)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"full_name":   "Owner/Repo",
			"html_url":    "https://github.com/Owner/Repo",
			"description": c.description,
			"owner":       map[string]any{"login": "Owner"},
		})
	}
}

func resolveRepo(t *testing.T, gh *GitHub) RemoteAddon {
	t.Helper()
	addon, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Owner/Repo"})
	if err != nil {
		t.Fatal(err)
	}
	return addon
}

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	return files
}

func TestGitHubCacheSendsIfNoneMatchAndServesCachedBodyOn304(t *testing.T) {
	srv := &conditionalServer{etag: `W/"v1"`, description: "cached description"}
	gh, _ := newTestGitHub(t, srv.handler(""), "")
	gh.SetCacheDir(t.TempDir())

	first := resolveRepo(t, gh)
	second := resolveRepo(t, gh)

	ifNoneMatch, _, full := srv.seen()
	if strings.Join(ifNoneMatch, "|") != `|W/"v1"` {
		t.Errorf("If-None-Match per request = %q, want none then W/\"v1\"", ifNoneMatch)
	}
	if full != 1 {
		t.Errorf("server sent %d full responses, want 1", full)
	}
	if second != first || second.Summary != "cached description" {
		t.Errorf("second resolve = %+v, want cached %+v", second, first)
	}
}

func TestGitHubCacheSendsIfModifiedSince(t *testing.T) {
	modified := "Wed, 21 Oct 2026 07:28:00 GMT"
	srv := &conditionalServer{modified: modified, description: "by date"}
	gh, _ := newTestGitHub(t, srv.handler(""), "")
	gh.SetCacheDir(t.TempDir())

	resolveRepo(t, gh)
	second := resolveRepo(t, gh)

	ifNoneMatch, ifModified, full := srv.seen()
	if strings.Join(ifModified, "|") != "|"+modified {
		t.Errorf("If-Modified-Since per request = %q", ifModified)
	}
	if strings.Join(ifNoneMatch, "") != "" {
		t.Errorf("If-None-Match = %q, want none without an ETag", ifNoneMatch)
	}
	if full != 1 || second.Summary != "by date" {
		t.Errorf("full responses = %d, second = %+v", full, second)
	}
}

func TestGitHubCacheFilesHoldNoTokenAndArePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wowtui", "http")
	srv := &conditionalServer{etag: `"abc"`, modified: "Wed, 21 Oct 2026 07:28:00 GMT", description: "private"}
	gh, _ := newTestGitHub(t, srv.handler("dummy-token"), "dummy-token")
	gh.SetCacheDir(dir)

	resolveRepo(t, gh)
	resolveRepo(t, gh)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("cache dir mode = %o, want 700", perm)
	}
	files := cacheFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("cache files = %v, want one entry", files)
	}
	info, err = os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("cache file mode = %o, want 600", perm)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"dummy-token", "Bearer", "Authorization", "authorization"} {
		if strings.Contains(string(data), secret) {
			t.Errorf("cache file contains %q: %s", secret, data)
		}
	}
}

func TestGitHubCacheRefetchesCorruptEntry(t *testing.T) {
	corruptions := map[string]func(t *testing.T, data []byte) []byte{
		"truncated file": func(t *testing.T, data []byte) []byte {
			return data[:len(data)/2]
		},
		"truncated body": func(t *testing.T, data []byte) []byte {
			var entry cachedResponse
			if err := json.Unmarshal(data, &entry); err != nil {
				t.Fatal(err)
			}
			entry.Body = entry.Body[:len(entry.Body)/2]
			out, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			return out
		},
	}
	for name, corrupt := range corruptions {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			srv := &conditionalServer{etag: `"v2"`, description: "fresh"}
			gh, _ := newTestGitHub(t, srv.handler(""), "")
			gh.SetCacheDir(dir)

			resolveRepo(t, gh)
			files := cacheFiles(t, dir)
			if len(files) != 1 {
				t.Fatalf("cache files = %v, want one entry", files)
			}
			data, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(files[0], corrupt(t, data), 0o600); err != nil {
				t.Fatal(err)
			}

			second := resolveRepo(t, gh)
			resolveRepo(t, gh)

			ifNoneMatch, _, full := srv.seen()
			if strings.Join(ifNoneMatch, "|") != `||"v2"` {
				t.Errorf("If-None-Match per request = %q, want a refetch without it after corruption", ifNoneMatch)
			}
			if full != 2 || second.Summary != "fresh" {
				t.Errorf("full responses = %d, second = %+v", full, second)
			}
		})
	}
}

func TestGitHubCacheIgnoresEntryForAnotherURL(t *testing.T) {
	dir := t.TempDir()
	srv := &conditionalServer{etag: `"v3"`, description: "real"}
	gh, _ := newTestGitHub(t, srv.handler(""), "")
	gh.SetCacheDir(dir)

	resolveRepo(t, gh)
	files := cacheFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("cache files = %v, want one entry", files)
	}
	forged, err := json.Marshal(cachedResponse{URL: "https://example.invalid/other", ETag: `"v3"`, Body: []byte(`{"full_name":"Forged/Repo"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], forged, 0o600); err != nil {
		t.Fatal(err)
	}

	if got := resolveRepo(t, gh); got.Name != "Owner/Repo" {
		t.Errorf("resolve = %+v, want the server's repo", got)
	}
}

func TestGitHubCacheStoresOnlyValidated200(t *testing.T) {
	dir := t.TempDir()
	gh, _ := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Owner/Repo":
			json.NewEncoder(w).Encode(map[string]any{"full_name": "Owner/Repo"})
		default:
			w.Header().Set("ETag", `"missing"`)
			w.WriteHeader(http.StatusNotFound)
		}
	}, "")
	gh.SetCacheDir(dir)

	resolveRepo(t, gh)
	if _, err := gh.Resolve(context.Background(), Reference{Provider: "github", ID: "Missing/Repo"}); err == nil {
		t.Fatal("expected error for missing repo")
	}
	if files := cacheFiles(t, dir); len(files) != 0 {
		t.Errorf("cache files = %v, want none for unvalidated or non-200 responses", files)
	}
}

func TestGitHubCacheSkipsLargeBodies(t *testing.T) {
	dir := t.TempDir()
	srv := &conditionalServer{etag: `"big"`, description: strings.Repeat("x", maxCachedBody)}
	gh, _ := newTestGitHub(t, srv.handler(""), "")
	gh.SetCacheDir(dir)

	got := resolveRepo(t, gh)
	if len(got.Summary) != maxCachedBody {
		t.Errorf("summary length = %d, want the full body decoded", len(got.Summary))
	}
	if files := cacheFiles(t, dir); len(files) != 0 {
		t.Errorf("cache files = %v, want none for a body over the cap", files)
	}
}

func TestGitHubCacheLeavesDownloadsAlone(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var conditional []string
	gh, srv := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+r.URL.Path+`"`)
		if r.URL.Path != "/repos/Owner/Repo/releases/latest" {
			mu.Lock()
			conditional = append(conditional, r.Header.Get("If-None-Match"))
			mu.Unlock()
		}
		switch r.URL.Path {
		case "/repos/Owner/Repo/releases/latest":
			if r.Header.Get("If-None-Match") == `"`+r.URL.Path+`"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id":       1,
				"tag_name": "v1",
				"assets": []map[string]any{
					{"name": "Repo.zip", "browser_download_url": "http://" + r.Host + "/dl/Repo.zip", "size": 9},
					{"name": "release.json", "browser_download_url": "http://" + r.Host + "/dl/release.json", "size": 9},
				},
			})
		case "/dl/release.json":
			w.Write([]byte(`{"releases":[{"filename":"Repo.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":110200}]}]}`))
		case "/dl/Repo.zip":
			w.Write([]byte("zip-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}, "")
	gh.SetCacheDir(dir)

	for range 2 {
		release, err := gh.Latest(context.Background(), RemoteAddon{Provider: "github", ID: "Owner/Repo"}, wow.Retail, "release")
		if err != nil {
			t.Fatal(err)
		}
		body, err := gh.Download(context.Background(), release)
		if err != nil {
			t.Fatal(err)
		}
		body.Close()
	}

	if strings.Join(conditional, "") != "" {
		t.Errorf("downloads sent If-None-Match %q, want unconditional downloads", conditional)
	}
	files := cacheFiles(t, dir)
	if len(files) != 1 || files[0] != gh.cache.path(srv.URL+"/repos/Owner/Repo/releases/latest") {
		t.Errorf("cache files = %v, want only the releases/latest entry", files)
	}
}

func TestGitHubCacheUnusableDirWorksUncached(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	srv := &conditionalServer{etag: `"v4"`, description: "uncached"}
	gh, _ := newTestGitHub(t, srv.handler(""), "")
	gh.SetCacheDir(filepath.Join(blocker, "http"))

	resolveRepo(t, gh)
	if got := resolveRepo(t, gh); got.Summary != "uncached" {
		t.Errorf("resolve = %+v", got)
	}
	if _, _, full := srv.seen(); full != 2 {
		t.Errorf("full responses = %d, want 2 without a usable cache", full)
	}
}

func TestGitHubWithoutCacheDirSendsNoConditionalHeaders(t *testing.T) {
	srv := &conditionalServer{etag: `"v5"`, description: "plain"}
	gh, _ := newTestGitHub(t, srv.handler(""), "")

	resolveRepo(t, gh)
	resolveRepo(t, gh)
	if ifNoneMatch, _, full := srv.seen(); strings.Join(ifNoneMatch, "") != "" || full != 2 {
		t.Errorf("If-None-Match = %q, full = %d; want plain requests", ifNoneMatch, full)
	}
}
