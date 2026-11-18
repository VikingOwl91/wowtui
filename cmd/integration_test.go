package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/registry"
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
	if !strings.Contains(out, "extract backend: ") {
		t.Fatalf("add did not report its extraction backend: %s", out)
	}
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
	if strings.Contains(out, "extract backend: ") {
		t.Fatalf("check started extraction: %s", out)
	}
	if !strings.Contains(out, "current") {
		t.Errorf("check output: %s", out)
	}

	gh.bump("v2.0.0", buildCmdZip(t, "2.0.0"))

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "update") {
		t.Errorf("check output after bump: %s", out)
	}

	out = runCmd(t, "update", "--all", "--dry-run", "--config", cfgPath)
	if strings.Contains(out, "extract backend: ") {
		t.Fatalf("dry-run started extraction: %s", out)
	}
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
	if !strings.Contains(out, "extract backend: ") {
		t.Fatalf("update did not report its extraction backend: %s", out)
	}
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

func TestAddDirectURL(t *testing.T) {
	content := buildCmdZip(t, "1.0.0")
	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write(content)
	}))
	t.Cleanup(directSrv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	installPath := filepath.Join(home, "wow", "_retail_")
	if err := os.MkdirAll(filepath.Join(installPath, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("[wow]\npaths = [%q]\n", filepath.Join(home, "wow"))), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, "add", "direct:"+directSrv.URL+"/addon.zip", "--config", cfgPath)
	if !strings.Contains(out, "installed") {
		t.Fatalf("add output: %s", out)
	}

	toc, err := os.ReadFile(filepath.Join(installPath, "Interface", "AddOns", "TestAddon", "TestAddon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(toc) != "## Version: 1.0.0" {
		t.Errorf("installed toc = %q", toc)
	}

	out = runCmd(t, "check", "--config", cfgPath)
	if !strings.Contains(out, "current") {
		t.Errorf("check output: %s", out)
	}
}

func executeCmd(args ...string) (string, error) {
	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func setupUpdateAllWithFailingAddon(t *testing.T) (cfgPath, tocPath string) {
	t.Helper()
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	tocPath = filepath.Join(installPath, "Interface", "AddOns", "TestAddon", "TestAddon.toc")
	runCmd(t, "add", "github:Owner/TestAddon", "--config", cfgPath)

	regPath := registry.DefaultPath()
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatal(err)
	}
	missing := registry.Addon{Provider: "github", Ref: "Owner/Missing", Name: "Owner/Missing", Flavor: "retail", InstallPath: installPath, Folders: []string{"Missing"}, Version: "v1.0.0", Source: "auto"}
	reg.Addons = append([]registry.Addon{missing}, reg.Addons...)
	if err := reg.Save(regPath); err != nil {
		t.Fatal(err)
	}

	gh.bump("v2.0.0", buildCmdZip(t, "2.0.0"))
	return cfgPath, tocPath
}

func assertTocFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Errorf("toc = %q, want %q", content, want)
	}
}

func TestUpdateAllContinuesPastFailingAddon(t *testing.T) {
	cfgPath, tocPath := setupUpdateAllWithFailingAddon(t)

	out, err := executeCmd("update", "--all", "--dry-run", "--config", cfgPath)
	if err == nil || !strings.Contains(err.Error(), "Owner/Missing") {
		t.Fatalf("dry run error = %v, want failure naming Owner/Missing\noutput: %s", err, out)
	}
	if !strings.Contains(out, "dry run") || !strings.Contains(out, "1 to update, 0 current, 1 failed") {
		t.Errorf("dry run output: %s", out)
	}
	assertTocFile(t, tocPath, "## Version: 1.0.0")

	out, err = executeCmd("update", "--all", "--config", cfgPath)
	if err == nil || !strings.Contains(err.Error(), "Owner/Missing") {
		t.Fatalf("update error = %v, want failure naming Owner/Missing\noutput: %s", err, out)
	}
	if strings.Contains(err.Error(), "Owner/TestAddon") {
		t.Errorf("error = %v names an addon that did not fail", err)
	}
	if !strings.Contains(out, "1 updated, 0 current, 1 failed") {
		t.Errorf("update output: %s", out)
	}
	assertTocFile(t, tocPath, "## Version: 2.0.0")
}

func TestExecuteHelperProcess(t *testing.T) {
	if os.Getenv("WOWTUI_EXECUTE_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	os.Args = append([]string{"wowtui"}, args...)
	Execute()
	os.Exit(0)
}

func TestUpdateAllExitStatusOnFailure(t *testing.T) {
	cfgPath, tocPath := setupUpdateAllWithFailingAddon(t)

	child := exec.Command(os.Args[0], "-test.run=^TestExecuteHelperProcess$", "--", "update", "--all", "--config", cfgPath)
	child.Env = append(os.Environ(), "WOWTUI_EXECUTE_HELPER=1")
	out, err := child.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit = %v, want status 1\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "Owner/Missing") {
		t.Errorf("output does not name the failed addon: %s", out)
	}
	assertTocFile(t, tocPath, "## Version: 2.0.0")
}
