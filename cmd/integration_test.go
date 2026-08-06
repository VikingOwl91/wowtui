package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeGitHub struct {
	srv        *httptest.Server
	mu         sync.Mutex
	releaseID  int64
	tag        string
	zipContent []byte
	tags       []string
	tagZips    map[string][]byte
}

func newFakeGitHub(t *testing.T, zipContent []byte) *fakeGitHub {
	f := &fakeGitHub{
		releaseID:  1,
		tag:        "v1.0.0",
		zipContent: zipContent,
		tagZips:    map[string][]byte{},
	}
	f.srv = httptest.NewServer(f)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) bump(tag string, zipContent []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseID++
	f.tag = tag
	f.zipContent = zipContent
}

func (f *fakeGitHub) seedTags(tag string, zipContent []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tags = []string{tag}
	f.tagZips[tag] = zipContent
}

func (f *fakeGitHub) bumpTag(tag string, zipContent []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tags = append([]string{tag}, f.tags...)
	f.tagZips[tag] = zipContent
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/repos/Owner/TestAddon":
		json.NewEncoder(w).Encode(map[string]any{
			"full_name": "Owner/TestAddon",
			"html_url":  "https://github.com/Owner/TestAddon",
		})
	case "/repos/Owner/TestAddon/releases/latest":
		json.NewEncoder(w).Encode(map[string]any{
			"id":       f.releaseID,
			"tag_name": f.tag,
			"assets": []map[string]any{{
				"name":                 "TestAddon.zip",
				"browser_download_url": f.srv.URL + "/dl/TestAddon.zip",
				"size":                 len(f.zipContent),
			}},
		})
	case "/dl/TestAddon.zip":
		w.Write(f.zipContent)

	case "/repos/Owner/TagOnly":
		json.NewEncoder(w).Encode(map[string]any{
			"full_name":      "Owner/TagOnly",
			"html_url":       "https://github.com/Owner/TagOnly",
			"default_branch": "main",
		})
	case "/repos/Owner/TagOnly/releases/latest":
		w.WriteHeader(http.StatusNotFound)
	case "/repos/Owner/TagOnly/tags":
		tags := make([]map[string]any, 0, len(f.tags))
		for _, name := range f.tags {
			tags = append(tags, map[string]any{"name": name})
		}
		json.NewEncoder(w).Encode(tags)

	default:
		if idx := strings.Index(r.URL.Path, "/archive/refs/tags/"); idx >= 0 {
			tag := strings.TrimSuffix(r.URL.Path[idx+len("/archive/refs/tags/"):], ".zip")
			if zip, found := f.tagZips[tag]; found {
				w.Write(zip)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func buildCmdZip(t *testing.T, version string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	fw, err := w.Create("TestAddon/TestAddon.toc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("## Version: " + version)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setupAddEnv(t *testing.T, gh *fakeGitHub) (cfgPath, installPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("WOWTUI_GITHUB_API", gh.srv.URL)

	installPath = filepath.Join(home, "wow", "_retail_")
	if err := os.MkdirAll(filepath.Join(installPath, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath = filepath.Join(home, "config.toml")
	content := fmt.Sprintf("[wow]\npaths = [%q]\n", filepath.Join(home, "wow"))
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, installPath
}

func TestAddCheckUpdateFlow(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	tocPath := filepath.Join(installPath, "Interface", "AddOns", "TestAddon", "TestAddon.toc")

	out := runCmd(t, "add", "github:Owner/TestAddon", "--config", cfgPath)
	if !strings.Contains(out, "installed") {
		t.Fatalf("add output: %s", out)
	}
	content, err := os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.0.0" {
		t.Fatalf("installed toc = %q", content)
	}

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "current") {
		t.Errorf("check output: %s", out)
	}

	gh.bump("v2.0.0", buildCmdZip(t, "2.0.0"))

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "update") {
		t.Errorf("check output after bump: %s", out)
	}

	out = runCmd(t, "update", "--all", "--dry-run", "--config", cfgPath)
	if !strings.Contains(out, "dry run") {
		t.Errorf("dry-run output: %s", out)
	}
	content, err = os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.0.0" {
		t.Errorf("dry run must not install, toc = %q", content)
	}

	out = runCmd(t, "update", "TestAddon", "--config", cfgPath)
	if !strings.Contains(out, "updating") {
		t.Errorf("update output: %s", out)
	}
	content, err = os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 2.0.0" {
		t.Errorf("updated toc = %q", content)
	}

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "current") {
		t.Errorf("check output after update: %s", out)
	}
}

func TestAddRejectsUnknownFlavor(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, _ := setupAddEnv(t, gh)

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"add", "github:Owner/TestAddon", "--flavor", "wrath", "--config", cfgPath})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown flavor")
	}
}

func TestUpdateWithoutTargetErrors(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, _ := setupAddEnv(t, gh)

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"update", "--config", cfgPath})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when neither addon nor --all is given")
	}
}

func buildWrappedZip(t *testing.T, wrapper, version string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	fw, err := w.Create(wrapper + "/TestAddon/TestAddon.toc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("## Version: " + version)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAddWithTagFallback(t *testing.T) {
	gh := newFakeGitHub(t, nil)
	gh.seedTags("v1.0.0", buildWrappedZip(t, "TagOnly-1.0.0", "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	tocPath := filepath.Join(installPath, "Interface", "AddOns", "TestAddon", "TestAddon.toc")

	out := runCmd(t, "add", "github:Owner/TagOnly", "--config", cfgPath)
	if !strings.Contains(out, "installed") {
		t.Fatalf("add output: %s", out)
	}
	content, err := os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.0.0" {
		t.Fatalf("installed toc = %q (wrapper folder not unwrapped?)", content)
	}

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "current") {
		t.Errorf("check output: %s", out)
	}

	gh.bumpTag("v1.10.0", buildWrappedZip(t, "TagOnly-1.10.0", "1.10.0"))

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "update") {
		t.Errorf("check output after new tag: %s", out)
	}

	out = runCmd(t, "update", "TagOnly", "--config", cfgPath)
	if !strings.Contains(out, "updating") {
		t.Errorf("update output: %s", out)
	}
	content, err = os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.10.0" {
		t.Errorf("updated toc = %q", content)
	}

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "current") {
		t.Errorf("check output after update: %s", out)
	}
}

func TestAddRejectsUnknownSource(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, _ := setupAddEnv(t, gh)

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"add", "github:Owner/TestAddon", "--source", "bogus", "--config", cfgPath})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown source")
	}
}
