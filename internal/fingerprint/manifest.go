// Package fingerprint identifies addon releases by the contents of their files.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

type File struct {
	Path   string `json:"path"` // Slash-separated, relative to Interface/AddOns.
	SHA256 string `json:"sha256"`
}

type Release struct {
	Version string `json:"version"`
	Files   []File `json:"files"`
}

type Change struct {
	Path   string
	Kind   string // added, missing, or modified
	SHA256 string // Local content hash; empty for a missing file.
}

type Report struct {
	Matched int
	Changes []Change
}

func (r Release) Validate() error {
	if strings.TrimSpace(r.Version) == "" || strings.ContainsFunc(r.Version, unicode.IsControl) {
		return fmt.Errorf("invalid fingerprint version %q", r.Version)
	}
	return validateFiles(r.Files)
}

func validateFiles(files []File) error {
	if len(files) == 0 || len(files) > maxFiles {
		return fmt.Errorf("fingerprint must contain between 1 and %d files", maxFiles)
	}
	type entry struct {
		name     string
		children map[string]entry // nil for a file
	}
	roots := map[string]entry{}
	entries := 0
	for _, file := range files {
		if len(file.Path) > maxPathBytes {
			return fmt.Errorf("fingerprint path exceeds %d bytes", maxPathBytes)
		}
		if len(file.SHA256) != sha256.Size*2 || file.SHA256 != strings.ToLower(file.SHA256) {
			return fmt.Errorf("invalid SHA-256 for %q", file.Path)
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return fmt.Errorf("invalid SHA-256 for %q", file.Path)
		}
		parts := strings.Split(file.Path, "/")
		if len(parts) < 2 || !installer.SafeFolderName(parts[0]) || !fs.ValidPath(file.Path) {
			return fmt.Errorf("invalid release file path %q", file.Path)
		}
		children := roots
		for i, part := range parts {
			if !validName(part) {
				return fmt.Errorf("invalid path component %q in %q", part, file.Path)
			}
			key := strings.ToLower(part)
			isDir := i < len(parts)-1
			node, found := children[key]
			if found {
				if node.name != part || node.children == nil || !isDir {
					return fmt.Errorf("conflicting release path %q", file.Path)
				}
			} else {
				entries++
				if entries > maxEntries {
					return fmt.Errorf("too many fingerprint path entries")
				}
				node.name = part
				if isDir {
					node.children = make(map[string]entry)
				}
				children[key] = node
			}
			children = node.children
		}
	}
	return nil
}

// Use portable filenames so a manifest describes the same files on Wine and
// Windows. Windows also reserves these device names with a file extension.
func validName(name string) bool {
	if name == "" || !fs.ValidPath(name) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") ||
		strings.ContainsAny(name, "/\\:\"<>|?*") || strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	stem, _, _ := strings.Cut(strings.ToUpper(name), ".")
	stem = strings.TrimRight(stem, " ")
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT") {
		return !slices.Contains([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"}, stem[3:])
	}
	return true
}

// Folders returns distinct release roots in lexical order.
func (r Release) Folders() []string {
	seen := map[string]bool{}
	var folders []string
	for _, f := range r.Files {
		name, _, _ := strings.Cut(f.Path, "/")
		if !seen[name] {
			folders = append(folders, name)
			seen[name] = true
		}
	}
	slices.Sort(folders)
	return folders
}
