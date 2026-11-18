package fingerprint

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

const (
	// File and byte ceilings match the largest release the installer accepts.
	maxFiles            = 20_000
	maxEntries          = maxFiles * 2
	maxFileBytes  int64 = 256 << 20
	maxTotalBytes int64 = 512 << 20
	maxPathBytes        = 4096
)

// Compare hashes the files in the release's owned folders. Unreadable files,
// links, and special devices make the result indeterminate and return an error.
func Compare(ctx context.Context, addonsDir string, release Release) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := release.Validate(); err != nil {
		return Report{}, err
	}
	tree := newTreeReader(ctx)
	root, err := checkedRoot(addonsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Report{}, err
	}
	if root != nil {
		defer root.Close()
		for _, name := range release.Folders() {
			info, err := root.Lstat(name)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return Report{}, err
			}
			if !info.IsDir() {
				return Report{}, fmt.Errorf("addon folder %q is not a directory", name)
			}
			if err := tree.visit(root, name, name, info); err != nil {
				return Report{}, err
			}
		}
	}
	report := Report{}
	for _, expected := range release.Files {
		actual, found := tree.files[expected.Path]
		delete(tree.files, expected.Path)
		switch {
		case !found:
			report.Changes = append(report.Changes, Change{Path: expected.Path, Kind: "missing"})
		case expected.SHA256 != actual:
			report.Changes = append(report.Changes, Change{Path: expected.Path, Kind: "modified", SHA256: actual})
		default:
			report.Matched++
		}
	}
	for name, hash := range tree.files {
		report.Changes = append(report.Changes, Change{Path: name, Kind: "added", SHA256: hash})
	}
	slices.SortFunc(report.Changes, func(a, b Change) int { return cmp.Compare(a.Path, b.Path) })
	return report, ctx.Err()
}

// Snapshot hashes staged addon folders under their final installation names.
func Snapshot(ctx context.Context, roots []installer.StagedRoot) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tree := newTreeReader(ctx)
	seen := map[string]bool{}
	for _, staged := range roots {
		if !installer.SafeFolderName(staged.Name) || !validName(staged.Name) {
			return nil, fmt.Errorf("invalid staged folder name %q", staged.Name)
		}
		key := strings.ToLower(staged.Name)
		if seen[key] {
			return nil, fmt.Errorf("duplicate staged folder %q", staged.Name)
		}
		seen[key] = true
		before := len(tree.files)
		root, err := checkedRoot(staged.Src)
		if err != nil {
			return nil, err
		}
		info, err := root.Stat(".")
		if err == nil {
			err = tree.visit(root, ".", staged.Name, info)
		}
		root.Close()
		if err != nil {
			return nil, err
		}
		if len(tree.files) == before {
			return nil, fmt.Errorf("staged folder %q contains no files", staged.Name)
		}
	}
	files := make([]File, 0, len(tree.files))
	for name, hash := range tree.files {
		files = append(files, File{Path: name, SHA256: hash})
	}
	slices.SortFunc(files, func(a, b File) int { return cmp.Compare(a.Path, b.Path) })
	if err := validateFiles(files); err != nil {
		return nil, err
	}
	return files, ctx.Err()
}

// Pin the directory for the entire walk, including if it is later renamed.
func checkedRoot(path string) (*os.Root, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		root.Close()
		return nil, fmt.Errorf("directory %q changed while opening", path)
	}
	return root, nil
}

type treeReader struct {
	ctx     context.Context
	buf     []byte
	files   map[string]string
	bytes   int64
	entries int
}

func newTreeReader(ctx context.Context) *treeReader {
	return &treeReader{ctx: ctx, buf: make([]byte, 64<<10), files: map[string]string{}}
}

func (t *treeReader) visit(root *os.Root, path, name string, before os.FileInfo) error {
	if err := t.ctx.Err(); err != nil {
		return err
	}
	t.entries++
	if t.entries > maxEntries {
		return fmt.Errorf("too many filesystem entries while fingerprinting %q", name)
	}
	if !before.Mode().IsRegular() && !before.IsDir() {
		return fmt.Errorf("unsupported file type %s at %q", before.Mode().Type(), name)
	}
	f, err := root.OpenFile(path, readFlags, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, info) || info.Mode().Type() != before.Mode().Type() {
		return fmt.Errorf("%q changed while opening", name)
	}
	if info.IsDir() {
		err = t.visitDirectory(f, root, path, name)
	} else {
		err = t.hashFile(f, name, info)
	}
	if err != nil {
		return err
	}
	after, err := root.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, after) || !after.ModTime().Equal(info.ModTime()) {
		return fmt.Errorf("%q changed while fingerprinting", name)
	}
	return nil
}

func (t *treeReader) visitDirectory(f *os.File, root *os.Root, path, name string) error {
	for {
		children, err := f.ReadDir(128)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		for _, child := range children {
			childPath := filepath.Join(path, child.Name())
			childName := name + "/" + child.Name()
			if !validName(child.Name()) || len(childName) > maxPathBytes {
				return fmt.Errorf("invalid addon path %q", childName)
			}
			childInfo, err := root.Lstat(childPath)
			if err != nil {
				return err
			}
			if err := t.visit(root, childPath, childName, childInfo); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *treeReader) hashFile(f *os.File, name string, info os.FileInfo) error {
	if _, duplicate := t.files[name]; duplicate {
		return fmt.Errorf("duplicate fingerprint path %q", name)
	}
	if len(t.files) >= maxFiles || info.Size() > maxFileBytes || info.Size() > maxTotalBytes-t.bytes {
		return fmt.Errorf("fingerprint size limit exceeded at %q", name)
	}
	h := sha256.New()
	var size int64
	for {
		if err := t.ctx.Err(); err != nil {
			return err
		}
		n, err := f.Read(t.buf)
		size += int64(n)
		if size > info.Size() {
			return fmt.Errorf("%q changed while fingerprinting", name)
		}
		h.Write(t.buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	final, err := f.Stat()
	if err != nil {
		return err
	}
	if size != info.Size() || final.Size() != size || !final.ModTime().Equal(info.ModTime()) {
		return fmt.Errorf("%q changed while fingerprinting", name)
	}
	t.bytes += size
	t.files[name] = hex.EncodeToString(h.Sum(nil))
	return nil
}
