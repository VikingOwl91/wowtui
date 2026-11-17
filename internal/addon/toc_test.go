package addon

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeToc(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTocFields(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", `## Interface: 110200
## Title: Test Addon
## Version: 1.2.3
## Notes: Some notes
# a comment
SomeAddon.lua
`)
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := toc.Title(); got != "Test Addon" {
		t.Errorf("Title = %q", got)
	}
	if got := toc.Version(); got != "1.2.3" {
		t.Errorf("Version = %q", got)
	}
	if got := toc.Interface(); got != 110200 {
		t.Errorf("Interface = %d", got)
	}
}

func TestParseTocGetIsCaseInsensitive(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", "## VERSION: 9.9.9\n")
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := toc.Get("version"); got != "9.9.9" {
		t.Errorf("Get(version) = %q", got)
	}
}

func TestParseTocValueWithColon(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", "## Notes: foo: bar\n")
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := toc.Get("notes"); got != "foo: bar" {
		t.Errorf("Get(notes) = %q", got)
	}
}

func TestParseTocCRLFAndBOM(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", "\xef\xbb\xbf## Version: 2.0.0\r\n## Title: BOM Test\r\n")
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := toc.Version(); got != "2.0.0" {
		t.Errorf("Version = %q", got)
	}
	if got := toc.Title(); got != "BOM Test" {
		t.Errorf("Title = %q", got)
	}
}

func TestParseTocInvalidInterface(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", "## Interface: notanumber\n")
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := toc.Interface(); got != 0 {
		t.Errorf("Interface = %d, want 0", got)
	}
}

func TestParseTocMultiValueInterface(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", "## Interface: 110200, 50500, 11507\n")
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := toc.Interfaces(), []int{110200, 50500, 11507}; !slices.Equal(got, want) {
		t.Errorf("Interfaces = %v, want %v", got, want)
	}
	if got := toc.Interface(); got != 110200 {
		t.Errorf("Interface = %d, want 110200 (first listed value)", got)
	}
}

func TestParseTocInterfacesSkipsInvalidEntries(t *testing.T) {
	path := writeToc(t, t.TempDir(), "Test.toc", "## Interface: 110200,,abc, 0, -5,11507 ,\n")
	toc, err := ParseToc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := toc.Interfaces(), []int{110200, 11507}; !slices.Equal(got, want) {
		t.Errorf("Interfaces = %v, want %v", got, want)
	}
}

func TestParseTocInterfacesEmpty(t *testing.T) {
	for _, content := range []string{"## Title: x\n", "## Interface: notanumber\n", "## Interface:\n"} {
		path := writeToc(t, t.TempDir(), "Test.toc", content)
		toc, err := ParseToc(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := toc.Interfaces(); len(got) != 0 {
			t.Errorf("Interfaces(%q) = %v, want empty", content, got)
		}
		if got := toc.Interface(); got != 0 {
			t.Errorf("Interface(%q) = %d, want 0", content, got)
		}
	}
}

func TestParseTocMissingFile(t *testing.T) {
	if _, err := ParseToc(filepath.Join(t.TempDir(), "nope.toc")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
