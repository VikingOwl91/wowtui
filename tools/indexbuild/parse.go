package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

// canonicalGameTypes fixes the order of game types in all output.
var canonicalGameTypes = []wow.GameType{
	wow.GameMainline, wow.GameVanilla, wow.GameTBC, wow.GameWrath,
	wow.GameTitan, wow.GameCata, wow.GameMists, wow.GameForever,
}

func sortGameTypes(types []wow.GameType) []wow.GameType {
	out := slices.Clone(types)
	slices.SortFunc(out, func(a, b wow.GameType) int {
		return slices.Index(canonicalGameTypes, a) - slices.Index(canonicalGameTypes, b)
	})
	return slices.Compact(out)
}

// parseTOC reads the "## Key: value" header lines of a TOC file with
// lower-cased keys, like addon.ParseToc does for files on disk.
func parseTOC(data []byte) map[string]string {
	fields := map[string]string{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
		line = bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(line, []byte("##")) {
			continue
		}
		rest := strings.TrimLeft(strings.TrimPrefix(string(line), "##"), " \t")
		key, value, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return fields
}

func tocInterfaces(fields map[string]string) []int {
	var nums []int
	for _, part := range strings.Split(fields["interface"], ",") {
		num, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || num <= 0 {
			continue
		}
		nums = append(nums, num)
	}
	return nums
}

var (
	colorCode   = regexp.MustCompile(`\|c[0-9a-fA-F]{8}`)
	colorName   = regexp.MustCompile(`\|cn[^:|]*:`)
	textureTag  = regexp.MustCompile(`\|T[^|]*\|t`)
	atlasTag    = regexp.MustCompile(`\|A[^|]*\|a`)
	linkOpen    = regexp.MustCompile(`\|H[^|]*\|h`)
	pipeEscaped = "\x00pipe\x00"
)

// stripUIEscapes removes WoW UI escape sequences (colors, textures, atlases,
// hyperlinks, newlines), turns "||" into "|" and collapses whitespace.
func stripUIEscapes(s string) string {
	s = strings.ReplaceAll(s, "||", pipeEscaped)
	for _, re := range []*regexp.Regexp{colorCode, colorName, textureTag, atlasTag, linkOpen} {
		s = re.ReplaceAllString(s, "")
	}
	s = strings.NewReplacer("|r", "", "|h", "", "|n", " ").Replace(s)
	s = strings.ReplaceAll(s, pipeEscaped, "|")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// tocMatchesFolder reports which of folders (lower-case name to name) a TOC
// file path belongs to, looking through client suffixes like _Mainline.
func tocMatchesFolder(tocPath string, folders map[string]string) (string, bool) {
	file := path.Base(tocPath)
	if len(file) < 5 || !strings.EqualFold(file[len(file)-4:], ".toc") {
		return "", false
	}
	for _, name := range installer.TocFolderNames(file[:len(file)-4]) {
		if folder, ok := folders[strings.ToLower(name)]; ok {
			return folder, true
		}
	}
	return "", false
}

type pkgmeta struct {
	PackageAs    string
	MoveFolders  []string
	SkippedMoves int
}

func stripYAMLComment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return strings.TrimRight(line[:i], " \t")
		}
	}
	return strings.TrimRight(line, " \t")
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// parsePkgmeta reads the two .pkgmeta settings that name addon folders:
// package-as and the destination of every move-folders entry. Destinations
// that are not plain folder names (libraries nested inside an addon) are
// counted, not returned.
func parsePkgmeta(data []byte) pkgmeta {
	var meta pkgmeta
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = stripYAMLComment(strings.TrimRight(line, "\r"))
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = unquote(value)
		if line[0] != ' ' && line[0] != '\t' {
			section = key
			if key == "package-as" {
				meta.PackageAs = value
			}
			continue
		}
		if section != "move-folders" || strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		switch {
		case !installer.SafeFolderName(value):
			meta.SkippedMoves++
		case !slices.ContainsFunc(meta.MoveFolders, func(f string) bool { return strings.EqualFold(f, value) }):
			meta.MoveFolders = append(meta.MoveFolders, value)
		}
	}
	return meta
}

type fundingResult struct {
	Links   []index.Funding
	Skipped []string
}

// parseFunding reads the two documented FUNDING.yml shapes, "platform: value"
// and "platform: [a, b]". Everything else, and every value that does not
// validate as a funding link, is reported in Skipped.
func parseFunding(text string) fundingResult {
	var res fundingResult
	skip := func(n int, format string, args ...any) {
		res.Skipped = append(res.Skipped, fmt.Sprintf("line %d: ", n)+fmt.Sprintf(format, args...))
	}
	for i, raw := range strings.Split(text, "\n") {
		n := i + 1
		line := stripYAMLComment(strings.TrimRight(raw, "\r"))
		if strings.TrimSpace(line) == "" || line == "---" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' || line[0] == '-' {
			skip(n, "unsupported line %q", strings.TrimSpace(line))
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if !ok {
			skip(n, "unsupported line %q", line)
			continue
		}
		key = strings.TrimSpace(key)
		items, ok := fundingItems(value)
		if !ok {
			skip(n, "%s: unsupported value %q", key, value)
			continue
		}
		for _, item := range items {
			link := index.Funding{Platform: key, Handle: item}
			if _, err := link.URL(); err != nil {
				skip(n, "%s: %v", key, err)
				continue
			}
			if !slices.Contains(res.Links, link) {
				res.Links = append(res.Links, link)
			}
		}
	}
	return res
}

// fundingItems splits a scalar or a one-line list. Empty and null values give
// no items.
func fundingItems(value string) ([]string, bool) {
	switch value {
	case "", "~", "null", "[]":
		return nil, true
	}
	if strings.HasPrefix(value, "[") {
		if !strings.HasSuffix(value, "]") {
			return nil, false
		}
		var items []string
		for _, part := range splitList(value[1 : len(value)-1]) {
			if part = unquote(part); part != "" {
				items = append(items, part)
			}
		}
		return items, true
	}
	return []string{unquote(value)}, true
}

func splitList(s string) []string {
	var parts []string
	var quote rune
	start := 0
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ',':
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

// The packager's release.json flavor names, the same map the github provider
// uses to pick assets: its "classic" is classic era.
var releaseJSONFlavors = map[string]wow.GameType{
	"mainline": wow.GameMainline,
	"classic":  wow.GameVanilla,
	"bcc":      wow.GameTBC,
	"wrath":    wow.GameWrath,
	"titan":    wow.GameTitan,
	"cata":     wow.GameCata,
	"mists":    wow.GameMists,
	"forever":  wow.GameForever,
}

// releaseJSONGameTypes returns the game types a packager release.json lists,
// in canonical order. ok is false when it lists none that are known.
func releaseJSONGameTypes(data []byte) ([]wow.GameType, bool) {
	var manifest struct {
		Releases []struct {
			Metadata []struct {
				Flavor string `json:"flavor"`
			} `json:"metadata"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, false
	}
	var types []wow.GameType
	for _, release := range manifest.Releases {
		for _, meta := range release.Metadata {
			if gameType, ok := releaseJSONFlavors[meta.Flavor]; ok {
				types = append(types, gameType)
			}
		}
	}
	if len(types) == 0 {
		return nil, false
	}
	return sortGameTypes(types), true
}

var (
	nameAffixes = regexp.MustCompile(`(?i)^(wow[-_]?)|([-_]?(wow|addon|addons))$`)
	nonAlnum    = regexp.MustCompile(`[^a-z0-9]`)
)

func normName(s string) string {
	return nonAlnum.ReplaceAllString(strings.ToLower(s), "")
}

// nameSimilar reports whether the repository name in repo ("Owner/Name")
// resembles folder: one contains the other (4+ letters) or the names have a
// similarity ratio of at least 0.7.
func nameSimilar(folder, repo string) bool {
	name := repo
	if _, after, ok := strings.Cut(repo, "/"); ok {
		name = after
	}
	f := normName(folder)
	r := normName(nameAffixes.ReplaceAllString(name, ""))
	if r == "" {
		r = normName(name)
	}
	if len(f) >= 4 && (strings.Contains(r, f) || (len(r) >= 4 && strings.Contains(f, r))) {
		return true
	}
	return ratio(f, r) >= 0.7
}

// ratio is the Ratcliff/Obershelp similarity 2*M/T used by Python's
// difflib.SequenceMatcher, which the discovery prototype used.
func ratio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra)+len(rb) == 0 {
		return 1
	}
	return 2 * float64(matchingRunes(ra, rb)) / float64(len(ra)+len(rb))
}

func matchingRunes(a, b []rune) int {
	i, j, k := longestMatch(a, b)
	if k == 0 {
		return 0
	}
	return k + matchingRunes(a[:i], b[:j]) + matchingRunes(a[i+k:], b[j+k:])
}

func longestMatch(a, b []rune) (bestI, bestJ, bestK int) {
	prev := make([]int, len(b)+1)
	for i := range a {
		cur := make([]int, len(b)+1)
		for j := range b {
			if a[i] != b[j] {
				continue
			}
			cur[j+1] = prev[j] + 1
			if cur[j+1] > bestK {
				bestI, bestJ, bestK = i-cur[j+1]+1, j-cur[j+1]+1, cur[j+1]
			}
		}
		prev = cur
	}
	return bestI, bestJ, bestK
}
