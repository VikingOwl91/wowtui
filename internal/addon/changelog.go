package addon

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
)

var releaseHeading = regexp.MustCompile(`^## \[([A-Za-z0-9._+-]+)\]\(https://github\.com/([A-Za-z0-9_-][A-Za-z0-9._-]*/[A-Za-z0-9_-][A-Za-z0-9._-]*)/tree/([A-Za-z0-9._+-]+)\)( |$)`)
var releaseComparison = regexp.MustCompile(`^\[Full Changelog\]\(https://github\.com/([^/() ]+/[^/() ]+)/compare/[A-Za-z0-9._+-]+\.\.\.([A-Za-z0-9._+-]+)\)( |$)`)

// Match the release header produced by the addon packager. Links in release
// notes and changelogs for embedded libraries do not identify the addon.
func changelogRepository(path string, folder *Folder) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(io.LimitReader(f, 8192))
	var lines []string
	for len(lines) < 3 && scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 3 {
		return ""
	}
	title, ok := strings.CutPrefix(lines[0], "# ")
	heading := releaseHeading.FindStringSubmatch(lines[1])
	compare := releaseComparison.FindStringSubmatch(lines[2])
	if !ok || title == "" || heading == nil || compare == nil {
		return ""
	}
	version, repo := heading[1], heading[2]
	if heading[3] != version || compare[2] != version || compare[1] != repo {
		return ""
	}
	for _, toc := range folder.Tocs {
		if toc.Version() == version && (toc.Title() == title || folder.Name == title) {
			return "github:" + repo
		}
	}
	return ""
}
