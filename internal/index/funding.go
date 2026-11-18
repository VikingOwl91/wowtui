package index

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Funding is one FUNDING.yml value: a platform key and the handle written for
// it. Platform keys and handle shapes follow
// https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/displaying-a-sponsor-button-in-your-repository.
// A GitHub Sponsors or custom list becomes one Funding per item.
type Funding struct {
	Platform string `json:"platform"`
	Handle   string `json:"handle"`
}

// fundingPrefixes are the link prefixes GitHub renders for each platform on
// a repository's sponsor dialog, checked against live repositories on
// 2026-11-18.
var fundingPrefixes = map[string]string{
	"github":           "https://github.com/sponsors/",
	"patreon":          "https://patreon.com/",
	"open_collective":  "https://opencollective.com/",
	"ko_fi":            "https://ko-fi.com/",
	"tidelift":         "https://tidelift.com/funding/github/",
	"community_bridge": "https://funding.communitybridge.org/projects/",
	"liberapay":        "https://liberapay.com/",
	"issuehunt":        "https://issuehunt.io/r/",
	"polar":            "https://polar.sh/",
	"buy_me_a_coffee":  "https://buymeacoffee.com/",
	"thanks_dev":       "https://thanks.dev/",
}

var tideliftPlatforms = []string{"npm", "pypi", "rubygems", "maven", "packagist", "nuget"}

var (
	handlePattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	tideliftPackagePattern = regexp.MustCompile(`^[A-Za-z0-9@][A-Za-z0-9._-]*$`)
)

// URL returns the https link for f. It fails for platforms GitHub does not
// document, for handles that are not plain path segments of the documented
// shape, and for custom links that are not https URLs.
func (f Funding) URL() (string, error) {
	if f.Platform == "custom" {
		if !validCustomURL(f.Handle) {
			return "", fmt.Errorf("custom funding link %q is not an https URL", f.Handle)
		}
		return f.Handle, nil
	}
	prefix, ok := fundingPrefixes[f.Platform]
	if !ok {
		return "", fmt.Errorf("unknown funding platform %q", f.Platform)
	}
	if !validHandle(f.Platform, f.Handle) {
		return "", fmt.Errorf("invalid %s handle %q", f.Platform, f.Handle)
	}
	return prefix + f.Handle, nil
}

func validHandle(platform, handle string) bool {
	switch platform {
	case "tidelift":
		parts := strings.Split(handle, "/")
		if len(parts) < 2 || !slices.Contains(tideliftPlatforms, parts[0]) {
			return false
		}
		for _, part := range parts[1:] {
			if !tideliftPackagePattern.MatchString(part) {
				return false
			}
		}
		return true
	case "thanks_dev":
		user, ok := strings.CutPrefix(handle, "u/gh/")
		return ok && handlePattern.MatchString(user)
	default:
		return handlePattern.MatchString(handle)
	}
}

func validCustomURL(s string) bool {
	if !strings.HasPrefix(s, "https://") || strings.ContainsFunc(s, isSpaceOrControl) {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.User == nil && u.Hostname() != ""
}
