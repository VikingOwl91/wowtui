package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
)

const testCacheHomeEnv = "WOWTUI_TEST_CACHE_HOME"

// TestMain points XDG_CACHE_HOME at a throwaway directory so no test in this
// package reads or writes the real HTTP cache. A helper process re-executed
// by a test reuses the parent's directory, because it exits before cleanup.
func TestMain(m *testing.M) {
	if handled, code := worker.Dispatch(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	dir := os.Getenv(testCacheHomeEnv)
	owned := dir == ""
	if owned {
		var err error
		dir, err = os.MkdirTemp("", "wowtui-cmd-cache-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Setenv(testCacheHomeEnv, dir)
	}
	os.Setenv("XDG_CACHE_HOME", dir)
	code := m.Run()
	if owned {
		os.RemoveAll(dir)
	}
	os.Exit(code)
}

func TestHTTPCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	if got, want := httpCacheDir(), filepath.Join(xdg, "wowtui", "http"); got != want {
		t.Errorf("with XDG_CACHE_HOME: %q, want %q", got, want)
	}

	t.Setenv("XDG_CACHE_HOME", "relative/cache")
	if got, want := httpCacheDir(), filepath.Join(home, ".cache", "wowtui", "http"); got != want {
		t.Errorf("with relative XDG_CACHE_HOME: %q, want %q", got, want)
	}

	t.Setenv("XDG_CACHE_HOME", "")
	if got, want := httpCacheDir(), filepath.Join(home, ".cache", "wowtui", "http"); got != want {
		t.Errorf("without XDG_CACHE_HOME: %q, want %q", got, want)
	}
}

func TestBuildProvidersCachesGitHubAPIResponses(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var mu sync.Mutex
	var conditional []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/Owner/TestAddon" || r.Header.Get("Authorization") != "Bearer dummy-token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		conditional = append(conditional, r.Header.Get("If-None-Match"))
		mu.Unlock()
		if r.Header.Get("If-None-Match") == `"etag-1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-1"`)
		json.NewEncoder(w).Encode(map[string]any{"full_name": "Owner/TestAddon", "html_url": "https://github.com/Owner/TestAddon"})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WOWTUI_GITHUB_API", srv.URL)

	cfg := &config.Config{}
	cfg.Providers.GitHub.Token = "dummy-token"
	gh := buildProviders(cfg)["github"]
	for range 2 {
		addon, err := gh.Resolve(context.Background(), provider.Reference{Provider: "github", ID: "Owner/TestAddon"})
		if err != nil {
			t.Fatal(err)
		}
		if addon.Name != "Owner/TestAddon" {
			t.Errorf("resolve = %+v", addon)
		}
	}

	if strings.Join(conditional, "|") != `|"etag-1"` {
		t.Errorf("If-None-Match per request = %q, want none then the stored ETag", conditional)
	}
	dir := filepath.Join(cacheHome, "wowtui", "http")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("cache entries = %v, want one", entries)
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "dummy-token") {
		t.Errorf("cache entry contains the token: %s", data)
	}
}

func TestServiceUsesConfiguredSecurity(t *testing.T) {
	cfg := &config.Config{Security: config.Security{Sandbox: "required", Seccomp: "auto"}}
	svc := newService(NewRootCmd(), cfg)
	if svc.Security != cfg.Security {
		t.Fatalf("service security = %+v, want %+v", svc.Security, cfg.Security)
	}
}
