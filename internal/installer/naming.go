package installer

import (
	"slices"
	"strings"
)

// Game types, aliases, families and legacy suffixes from https://warcraft.wiki.gg/wiki/TOC_format.
var clientTocSuffixes = []string{
	"Standard", "Forever", "Camelot", "Mists", "Cata", "Wrath", "TBC", "Vanilla",
	"Plunderstorm", "WoWLabs", "WoWHack",
	"Mainline", "Classic",
	"BCC", "WOTLK",
}

func tocFolderNames(tocBase string) []string {
	names := []string{tocBase}
	for _, suffix := range clientTocSuffixes {
		cut := len(tocBase) - len(suffix) - 1
		if cut <= 0 || (tocBase[cut] != '_' && tocBase[cut] != '-') {
			continue
		}
		if strings.EqualFold(tocBase[cut+1:], suffix) {
			return append(names, tocBase[:cut])
		}
	}
	return names
}

func safeFolderName(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, "/\\\x00")
}

// SafeFolderName reports whether name is a plain addon folder name the
// installer accepts: not empty, no leading dot, no path separator or NUL.
func SafeFolderName(name string) bool {
	return safeFolderName(name)
}

func folderNameForTocs(current string, tocBases []string) string {
	if len(tocBases) == 0 {
		return current
	}
	common := tocFolderNames(tocBases[0])
	for _, base := range tocBases[1:] {
		valid := tocFolderNames(base)
		common = slices.DeleteFunc(common, func(name string) bool {
			return !slices.Contains(valid, name)
		})
	}
	if len(common) == 0 || slices.Contains(common, current) {
		return current
	}
	return common[0]
}
