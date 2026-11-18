package fingerprint

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

func digest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func put(t *testing.T, dir, name, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCompareRelease(t *testing.T) {
	dir := t.TempDir()
	release := Release{"1.0", []File{{"Core/core.lua", digest("core")}, {"Options/options.lua", digest("options")}, {"Core/sub/util.lua", digest("util")}}}
	put(t, dir, "Core/core.lua", "core")
	put(t, dir, "Core/sub/util.lua", "util")
	put(t, dir, "Options/options.lua", "options")
	put(t, dir, "Unowned/options.lua", "unrelated")
	report, err := Compare(t.Context(), dir, release)
	if err != nil || report.Matched != 3 || len(report.Changes) != 0 {
		t.Fatalf("exact release = %+v, %v", report, err)
	}
	put(t, dir, "Options/options.lua", "custom")
	put(t, dir, "Core/local.lua", "local")
	if err := os.Remove(filepath.Join(dir, "Core", "sub", "util.lua")); err != nil {
		t.Fatal(err)
	}
	report, err = Compare(t.Context(), dir, release)
	want := []Change{{Path: "Core/local.lua", Kind: "added", SHA256: digest("local")}, {Path: "Core/sub/util.lua", Kind: "missing"}, {Path: "Options/options.lua", Kind: "modified", SHA256: digest("custom")}}
	if err != nil || report.Matched != 1 || !slices.Equal(report.Changes, want) {
		t.Fatalf("changed release = %+v, %v, want %+v", report, err, want)
	}
}

func TestReleaseRejectsInvalidPaths(t *testing.T) {
	for _, names := range [][]string{
		{}, {"loose.file"}, {"/Core/evil"}, {"Core/../evil"}, {"Core/./evil"}, {"Core//evil"},
		{"C:/outside/file"}, {"Core/c:foo"}, {"Core/CON.lua"}, {"Core/COM1 .lua"}, {"Core/x\\y"}, {"Core/line\nbreak"},
		{"Core/ok.lua", "Core/ok.lua"}, {"Core/X.lua", "Core/x.lua"}, {"Core/Foo/a", "Core/foo/b"},
		{"Core/x", "Core/x/y"}, {"Core/x/y", "Core/x"}, {"bad./x"}, {".hidden/file"},
	} {
		r := Release{Version: "v1"}
		for _, n := range names {
			r.Files = append(r.Files, File{n, digest(n)})
		}
		if r.Validate() == nil {
			t.Errorf("accepted paths %q", names)
		}
	}
	if err := (Release{"1", []File{{"Core/a.lua", "short"}}}).Validate(); err == nil {
		t.Fatal("accepted invalid digest")
	}
	if err := (Release{"", []File{{"Core/a.lua", digest("a")}}}).Validate(); err == nil {
		t.Fatal("accepted missing version")
	}
}

func TestSnapshotAndMissingRoot(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "wrapper/core.lua", "main")
	put(t, dir, "staged-options/config.lua", "cfg")
	roots := []installer.StagedRoot{{Src: filepath.Join(dir, "staged-options"), Name: "Options"}, {Src: filepath.Join(dir, "wrapper"), Name: "Core"}}
	files, err := Snapshot(t.Context(), roots)
	want := []File{{"Core/core.lua", digest("main")}, {"Options/config.lua", digest("cfg")}}
	if err != nil || !slices.Equal(files, want) {
		t.Fatalf("snapshot = %+v, %v", files, err)
	}
	release := Release{"1", files}
	if got := release.Folders(); !slices.Equal(got, []string{"Core", "Options"}) {
		t.Fatalf("folders = %q", got)
	}
	report, err := Compare(t.Context(), dir, release)
	if err != nil || report.Matched != 0 || !slices.Equal(report.Changes, []Change{{Path: "Core/core.lua", Kind: "missing"}, {Path: "Options/config.lua", Kind: "missing"}}) {
		t.Fatalf("missing roots = %+v, %v", report, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Snapshot(ctx, roots); err != context.Canceled {
		t.Fatalf("snapshot cancellation = %v", err)
	}
	if _, err := Compare(ctx, dir, release); err != context.Canceled {
		t.Fatalf("compare cancellation = %v", err)
	}
	if _, err := Snapshot(t.Context(), []installer.StagedRoot{{Src: roots[0].Src, Name: "Core"}, roots[1]}); err == nil {
		t.Fatal("accepted two staged directories with the same install name")
	}
	if _, err := Snapshot(t.Context(), append(roots, installer.StagedRoot{Src: t.TempDir(), Name: "Empty"})); err == nil {
		t.Fatal("accepted an empty root missing from the resulting manifest")
	}
}

func TestCompareRejectsLinks(t *testing.T) {
	for _, name := range []string{"root", "file", "directory"} {
		t.Run(name, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			put(t, outside, "core.lua", "private")
			link, target := filepath.Join(dir, "Core"), outside
			if name != "root" {
				if err := os.Mkdir(link, 0700); err != nil {
					t.Fatal(err)
				}
				if name == "file" {
					link = filepath.Join(link, "core.lua")
					target = filepath.Join(outside, "core.lua")
				} else {
					link = filepath.Join(link, "nested")
				}
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skip(err)
			}
			if _, err := Compare(t.Context(), dir, Release{"1", []File{{"Core/core.lua", digest("private")}}}); err == nil {
				t.Fatal("accepted symbolic link")
			}
		})
	}
}
