package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

// candidatesFormat versions candidates.json and the review file.
const candidatesFormat = 1

// Candidates is the output of discover and the input of build. Packages are
// sorted by repository, every list inside is sorted, so equal inputs give
// equal bytes. GeneratedAt is the newest fetch time of any answer used.
type Candidates struct {
	Format      int       `json:"format"`
	GeneratedAt time.Time `json:"generated_at"`
	Packages    []Package `json:"packages"`
}

// Package is one chosen repository with the evidence that led to it.
type Package struct {
	Repo          string              `json:"repo"`
	Source        string              `json:"source"`
	Stars         int                 `json:"stars"`
	Archived      bool                `json:"archived"`
	DefaultBranch string              `json:"default_branch,omitempty"`
	PushedAt      string              `json:"pushed_at,omitempty"`
	Folders       []string            `json:"folders"`
	SeedFolders   []string            `json:"seed_folders"`
	PrimaryFolder string              `json:"primary_folder"`
	IDs           map[string][]string `json:"ids,omitempty"`
	Title         string              `json:"title,omitempty"`
	TocFiles      []string            `json:"toc_files,omitempty"`
	Interfaces    []int               `json:"interfaces,omitempty"`
	Pkgmeta       *PkgmetaInfo        `json:"pkgmeta,omitempty"`
	Release       *ReleaseInfo        `json:"release,omitempty"`
	FundingYML    string              `json:"funding_yml,omitempty"`
	Evidence      []Evidence          `json:"evidence"`
}

type PkgmetaInfo struct {
	PackageAs    string   `json:"package_as,omitempty"`
	MoveFolders  []string `json:"move_folders,omitempty"`
	SkippedMoves int      `json:"skipped_moves,omitempty"`
}

// ReleaseInfo describes releases/latest. ReleaseJSONGameTypes is only set
// when a release.json asset exists and lists known flavors.
type ReleaseInfo struct {
	Tag                  string         `json:"tag"`
	PublishedAt          string         `json:"published_at,omitempty"`
	Assets               []string       `json:"assets"`
	ReleaseJSON          bool           `json:"release_json"`
	ReleaseJSONGameTypes []wow.GameType `json:"release_json_game_types,omitempty"`
}

// Evidence says why a repository was chosen for a folder.
type Evidence struct {
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
	Folder string `json:"folder,omitempty"`
	Query  string `json:"query,omitempty"`
	Basis  string `json:"basis"`
	Rank   int    `json:"rank,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Review lists everything that needs a human look: ambiguous, filtered and
// unresolved lookups.
type Review struct {
	Format            int           `json:"format"`
	GeneratedAt       time.Time     `json:"generated_at"`
	Summary           ReviewSummary `json:"summary"`
	Items             []ReviewItem  `json:"items"`
	FoldersWithoutIDs []string      `json:"folders_without_ids,omitempty"`
}

type ReviewSummary struct {
	RegistryFolders int `json:"registry_folders"`
	SeededFolders   int `json:"seeded_folders"`
	ResolvedFolders int `json:"resolved_folders"`
	CoveredFolders  int `json:"covered_folders"`
	Packages        int `json:"packages"`
	ReviewItems     int `json:"review_items"`
}

// Review reasons.
const (
	reasonAmbiguous       = "ambiguous"
	reasonFiltered        = "filtered"
	reasonUnresolved      = "unresolved"
	reasonConflict        = "conflict"
	reasonHintUnconfirmed = "hint_unconfirmed"
	reasonHintDisagrees   = "hint_disagrees"
	reasonAPIError        = "api_error"
)

type ReviewItem struct {
	Reason     string            `json:"reason"`
	Folders    []string          `json:"folders,omitempty"`
	Source     string            `json:"source,omitempty"`
	Detail     string            `json:"detail"`
	Candidates []ReviewCandidate `json:"candidates,omitempty"`
}

type ReviewCandidate struct {
	Repo  string   `json:"repo"`
	Stars int      `json:"stars,omitempty"`
	Why   string   `json:"why"`
	Paths []string `json:"paths,omitempty"`
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFileAtomic writes data to path through a temporary file in the same
// directory, so a failure never leaves a half-written result.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func writeJSONFile(path string, v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o644)
}

func readCandidates(path string) (*Candidates, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cands Candidates
	if err := dec.Decode(&cands); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if cands.Format != candidatesFormat {
		return nil, fmt.Errorf("%s has format %d, want %d", path, cands.Format, candidatesFormat)
	}
	return &cands, nil
}
