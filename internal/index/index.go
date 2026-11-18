// Package index loads the signed static addon index wowtui publishes.
//
// The index is a JSON document (see File) plus a detached ed25519 signature
// over its exact bytes. It only holds metadata and references to public
// github or gitlab source repositories; releases are always resolved from the
// source itself, so the index never names a download URL.
//
// Load verifies the signature before it parses anything and then validates
// the whole document. Decoding is strict: an unknown "format", an unknown,
// duplicate or differently cased key at any level, trailing data after the
// document or any invalid entry rejects the whole index. Format 1 has no
// optional extension points, so any new field needs a new format number.
package index

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

// FormatVersion is the original, metadata-only index format.
const FormatVersion = 1

// FingerprintFormatVersion adds optional per-release file manifests to entries.
const FingerprintFormatVersion = 2

// Provider ID kinds, the keys of an entry's "ids" object.
const (
	KindCurseForge   = "curseforge"
	KindWago         = "wago"
	KindWoWInterface = "wowinterface"
)

// File is a signed index document. GeneratedAt is RFC 3339. Entries
// must be present, an empty list is valid.
type File struct {
	Format      int       `json:"format"`
	GeneratedAt time.Time `json:"generated_at"`
	Entries     []Entry   `json:"entries"`
}

// Entry is one addon package in the index.
//
//   - ID: unique lowercase slug matching [a-z0-9][a-z0-9._-]*.
//   - Name: display name, not blank, no control characters.
//   - Source: reference that provider.ParseReference resolves to github or gitlab.
//   - Folders: addon folders the package installs, each passing
//     installer.SafeFolderName, unique ignoring case.
//   - GameTypes: wow.GameType values the package supports, unique.
//   - IDs: optional provider IDs for matching installed TOC hints; each value
//     is unique per kind across the index.
//   - ReleaseJSON: the source publishes a packager release.json.
//   - Funding: FUNDING.yml values, see Funding.
//   - Reviewed: a human reviewed the entry.
type Entry struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Source      string         `json:"source"`
	Folders     []string       `json:"folders"`
	GameTypes   []wow.GameType `json:"game_types"`
	IDs         ProviderIDs    `json:"ids,omitzero"`
	ReleaseJSON bool           `json:"release_json"`
	Funding     []Funding      `json:"funding,omitempty"`
	Reviewed    bool           `json:"reviewed"`

	// Fingerprints are optional starting with format 2.
	Fingerprints []fingerprint.Release `json:"fingerprints,omitempty"`
}

// ProviderIDs holds an entry's catalog IDs. Empty means not set. A set value
// must not contain whitespace or control characters.
type ProviderIDs struct {
	CurseForge   string `json:"curseforge,omitempty"`
	Wago         string `json:"wago,omitempty"`
	WoWInterface string `json:"wowinterface,omitempty"`
}

func (ids ProviderIDs) kinds() [3][2]string {
	return [3][2]string{
		{KindCurseForge, ids.CurseForge},
		{KindWago, ids.Wago},
		{KindWoWInterface, ids.WoWInterface},
	}
}

// Index is a verified and validated index. It is read-only: every method
// returns copies, so callers cannot change it.
type Index struct {
	generatedAt time.Time
	entries     []Entry
	byFolder    map[string][]int
	byProvider  map[[2]string]int
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

var knownGameTypes = []wow.GameType{
	wow.GameMainline, wow.GameVanilla, wow.GameTBC, wow.GameWrath,
	wow.GameTitan, wow.GameCata, wow.GameMists, wow.GameForever,
}

// Load verifies sig, the signature file content, against the exact bytes of
// data with pub and only then parses and validates data. A signature error
// wraps ErrInvalidSignature.
func Load(data, sig []byte, pub ed25519.PublicKey) (*Index, error) {
	if err := verify(data, sig, pub); err != nil {
		return nil, err
	}
	return parse(data)
}

func parse(data []byte) (*Index, error) {
	var probe struct {
		Format *int `json:"format"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&probe); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	if probe.Format == nil {
		return nil, errors.New("index format is missing")
	}
	if *probe.Format != FormatVersion && *probe.Format != FingerprintFormatVersion {
		return nil, fmt.Errorf("unsupported index format %d, want %d or %d", *probe.Format, FormatVersion, FingerprintFormatVersion)
	}

	if err := checkKeys(data, *probe.Format); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	// The outer generated_at shadows File.GeneratedAt so a bad timestamp is
	// reported with its field name.
	var wire struct {
		File
		GeneratedAt json.RawMessage `json:"generated_at"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("decode index: trailing data after the JSON document")
	}
	file := wire.File
	if wire.GeneratedAt != nil {
		if err := file.GeneratedAt.UnmarshalJSON(wire.GeneratedAt); err != nil {
			return nil, fmt.Errorf("index generated_at: %w", err)
		}
	}
	if file.GeneratedAt.IsZero() {
		return nil, errors.New("index generated_at is missing")
	}
	if file.Entries == nil {
		return nil, errors.New("index entries is missing")
	}

	idx := &Index{
		generatedAt: file.GeneratedAt,
		entries:     file.Entries,
		byFolder:    map[string][]int{},
		byProvider:  map[[2]string]int{},
	}
	ids := map[string]int{}
	var errs []error
	for i, entry := range file.Entries {
		entryErrs := validateEntry(entry)
		if first, ok := ids[entry.ID]; ok {
			entryErrs = append(entryErrs, fmt.Errorf("duplicate id %q, first used by entries[%d]", entry.ID, first))
		} else {
			ids[entry.ID] = i
		}
		for _, kind := range entry.IDs.kinds() {
			if kind[1] == "" {
				continue
			}
			if first, ok := idx.byProvider[kind]; ok {
				entryErrs = append(entryErrs, fmt.Errorf("ids.%s %q is already used by entries[%d]", kind[0], kind[1], first))
				continue
			}
			idx.byProvider[kind] = i
		}
		for _, err := range entryErrs {
			errs = append(errs, fmt.Errorf("entries[%d] (id %q): %w", i, entry.ID, err))
		}
		for _, folder := range entry.Folders {
			key := folderKey(folder)
			if !slices.Contains(idx.byFolder[key], i) {
				idx.byFolder[key] = append(idx.byFolder[key], i)
			}
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid index: %w", errors.Join(errs...))
	}
	return idx, nil
}

func validateEntry(entry Entry) []error {
	var errs []error
	if !idPattern.MatchString(entry.ID) {
		errs = append(errs, fmt.Errorf("id %q is not a lowercase slug", entry.ID))
	}
	if strings.TrimSpace(entry.Name) == "" {
		errs = append(errs, errors.New("name is empty"))
	} else if strings.ContainsFunc(entry.Name, unicode.IsControl) {
		errs = append(errs, errors.New("name contains a control character"))
	}

	if ref, err := provider.ParseReference(entry.Source); err != nil {
		errs = append(errs, fmt.Errorf("source %q: %w", entry.Source, err))
	} else if ref.Provider != "github" && ref.Provider != "gitlab" {
		errs = append(errs, fmt.Errorf("source %q resolves to provider %q, want github or gitlab", entry.Source, ref.Provider))
	}

	if len(entry.Folders) == 0 {
		errs = append(errs, errors.New("folders is empty"))
	}
	seenFolders := map[string]bool{}
	for _, folder := range entry.Folders {
		if !installer.SafeFolderName(folder) {
			errs = append(errs, fmt.Errorf("folder %q is not a safe folder name", folder))
			continue
		}
		if seenFolders[folderKey(folder)] {
			errs = append(errs, fmt.Errorf("duplicate folder %q", folder))
		}
		seenFolders[folderKey(folder)] = true
	}

	versions := map[string]bool{}
	for i, release := range entry.Fingerprints {
		if err := release.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("fingerprints[%d]: %w", i, err))
			continue
		}
		if versions[release.Version] {
			errs = append(errs, fmt.Errorf("duplicate fingerprint version %q", release.Version))
		}
		versions[release.Version] = true
		for _, folder := range release.Folders() {
			if !slices.Contains(entry.Folders, folder) {
				errs = append(errs, fmt.Errorf("fingerprint %q contains undeclared folder %q", release.Version, folder))
			}
		}
	}

	if len(entry.GameTypes) == 0 {
		errs = append(errs, errors.New("game_types is empty"))
	}
	for i, gameType := range entry.GameTypes {
		if !slices.Contains(knownGameTypes, gameType) {
			errs = append(errs, fmt.Errorf("unknown game type %q", gameType))
		} else if slices.Contains(entry.GameTypes[:i], gameType) {
			errs = append(errs, fmt.Errorf("duplicate game type %q", gameType))
		}
	}

	for _, kind := range entry.IDs.kinds() {
		if strings.ContainsFunc(kind[1], isSpaceOrControl) {
			errs = append(errs, fmt.Errorf("ids.%s %q contains whitespace or a control character", kind[0], kind[1]))
		}
	}

	for i, link := range entry.Funding {
		if _, err := link.URL(); err != nil {
			errs = append(errs, fmt.Errorf("funding[%d]: %w", i, err))
		} else if slices.Contains(entry.Funding[:i], link) {
			errs = append(errs, fmt.Errorf("duplicate funding %s %q", link.Platform, link.Handle))
		}
	}
	return errs
}

var keyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// checkKeys rejects duplicate object keys and keys that are not plain
// lowercase ASCII. encoding/json matches keys ignoring case, including
// Unicode folds such as "ſ" for "s"; every format key is lowercase ASCII, so
// together with DisallowUnknownFields only exact key names are accepted.
func checkKeys(data []byte, format int) error {
	type object struct {
		keys    map[string]bool
		wantKey bool
	}
	var stack []*object
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var top *object
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if top != nil && top.wantKey {
			if tok == json.Delim('}') {
				stack = stack[:len(stack)-1]
				continue
			}
			key, _ := tok.(string)
			if !keyPattern.MatchString(key) || (format == FormatVersion && key == "fingerprints") {
				return fmt.Errorf("unknown field %q", key)
			}
			if top.keys[key] {
				return fmt.Errorf("duplicate field %q", key)
			}
			top.keys[key] = true
			top.wantKey = false
			continue
		}
		if top != nil {
			top.wantKey = true
		}
		switch tok {
		case json.Delim('{'):
			stack = append(stack, &object{keys: map[string]bool{}, wantKey: true})
		case json.Delim('['):
			stack = append(stack, nil)
		case json.Delim(']'):
			stack = stack[:len(stack)-1]
		}
	}
}

func isSpaceOrControl(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}

func folderKey(folder string) string {
	return strings.ToLower(folder)
}

// GeneratedAt returns the build time the index states.
func (idx *Index) GeneratedAt() time.Time {
	return idx.generatedAt
}

// Entries returns copies of all entries in index order.
func (idx *Index) Entries() []Entry {
	entries := make([]Entry, 0, len(idx.entries))
	for _, entry := range idx.entries {
		entries = append(entries, entry.clone())
	}
	return entries
}

// ByFolder returns copies of every entry that installs folder, compared
// ignoring case, in index order.
func (idx *Index) ByFolder(folder string) []Entry {
	positions := idx.byFolder[folderKey(folder)]
	entries := make([]Entry, 0, len(positions))
	for _, i := range positions {
		entries = append(entries, idx.entries[i].clone())
	}
	return entries
}

// ByProviderID returns a copy of the entry whose IDs hold id for kind
// (KindCurseForge, KindWago or KindWoWInterface). IDs compare exactly.
func (idx *Index) ByProviderID(kind, id string) (Entry, bool) {
	i, ok := idx.byProvider[[2]string{kind, id}]
	if !ok {
		return Entry{}, false
	}
	return idx.entries[i].clone(), true
}

// Search returns copies of the entries whose id or name contains query,
// ignoring case, in index order. An empty query matches every entry.
func (idx *Index) Search(query string) []Entry {
	query = strings.ToLower(query)
	entries := []Entry{}
	for _, entry := range idx.entries {
		if strings.Contains(entry.ID, query) || strings.Contains(strings.ToLower(entry.Name), query) {
			entries = append(entries, entry.clone())
		}
	}
	return entries
}

func (e Entry) clone() Entry {
	e.Folders = slices.Clone(e.Folders)
	e.GameTypes = slices.Clone(e.GameTypes)
	e.Funding = slices.Clone(e.Funding)
	e.Fingerprints = slices.Clone(e.Fingerprints)
	for i := range e.Fingerprints {
		e.Fingerprints[i].Files = slices.Clone(e.Fingerprints[i].Files)
	}
	return e
}
