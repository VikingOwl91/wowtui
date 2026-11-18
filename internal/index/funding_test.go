package index_test

import (
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/index"
)

// Expected URLs are the links GitHub renders for these FUNDING.yml values on
// https://github.com/<owner>/<repo>/funding_links (checked 2026-11-18).
func TestFundingURL(t *testing.T) {
	tests := []struct {
		platform, handle string
		want             string
	}{
		{"github", "bbatsov", "https://github.com/sponsors/bbatsov"},
		{"patreon", "bbatsov", "https://patreon.com/bbatsov"},
		{"open_collective", "rubocop", "https://opencollective.com/rubocop"},
		{"ko_fi", "s0up4200", "https://ko-fi.com/s0up4200"},
		{"tidelift", "rubygems/rubocop", "https://tidelift.com/funding/github/rubygems/rubocop"},
		{"tidelift", "pypi/structlog", "https://tidelift.com/funding/github/pypi/structlog"},
		{"tidelift", "maven/ch.vorburger.mariaDB4j", "https://tidelift.com/funding/github/maven/ch.vorburger.mariaDB4j"},
		{"tidelift", "npm/@babel/core", "https://tidelift.com/funding/github/npm/@babel/core"},
		{"tidelift", "packagist/vendor/package", "https://tidelift.com/funding/github/packagist/vendor/package"},
		{"tidelift", "nuget/Newtonsoft.Json", "https://tidelift.com/funding/github/nuget/Newtonsoft.Json"},
		{"community_bridge", "MariaDB4j", "https://funding.communitybridge.org/projects/MariaDB4j"},
		{"liberapay", "borgbackup", "https://liberapay.com/borgbackup"},
		{"issuehunt", "NickeManarin", "https://issuehunt.io/r/NickeManarin"},
		{"polar", "MordechaiHadad", "https://polar.sh/MordechaiHadad"},
		{"buy_me_a_coffee", "s0up4200", "https://buymeacoffee.com/s0up4200"},
		{"thanks_dev", "u/gh/hynek", "https://thanks.dev/u/gh/hynek"},
		{"custom", "https://www.paypal.me/bbatsov", "https://www.paypal.me/bbatsov"},
		{"custom", "https://example.com/donate?ref=wowtui#top", "https://example.com/donate?ref=wowtui#top"},
		{"github", "The-Compiler", "https://github.com/sponsors/The-Compiler"},
		{"liberapay", "some_user.name", "https://liberapay.com/some_user.name"},
	}
	for _, tt := range tests {
		got, err := index.Funding{Platform: tt.platform, Handle: tt.handle}.URL()
		if err != nil {
			t.Errorf("%s %q: %v", tt.platform, tt.handle, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s %q = %q, want %q", tt.platform, tt.handle, got, tt.want)
		}
	}
}

func TestFundingRejectsUnknownPlatforms(t *testing.T) {
	for _, platform := range []string{"", "paypal", "otechie", "lfx_crowdfunding", "GitHub", "ko-fi", "custom_url"} {
		if got, err := (index.Funding{Platform: platform, Handle: "someone"}).URL(); err == nil {
			t.Errorf("platform %q accepted: %q", platform, got)
		}
	}
}

func TestFundingRejectsUnsafeHandles(t *testing.T) {
	tests := []struct {
		platform, handle string
	}{
		{"github", ""},
		{"github", "a/b"},
		{"github", ".."},
		{"github", ".hidden"},
		{"github", "-dash"},
		{"patreon", "a b"},
		{"patreon", "a?b"},
		{"patreon", "a#b"},
		{"ko_fi", "a%2Fb"},
		{"ko_fi", "a@b"},
		{"open_collective", "a\\b"},
		{"liberapay", "a\x1bb"},
		{"polar", "https://evil.example"},
		{"buy_me_a_coffee", "a:b"},
		{"issuehunt", "owner/repo"},
		{"community_bridge", "a\nb"},

		{"tidelift", "npm"},
		{"tidelift", "npm/"},
		{"tidelift", "/npm/x"},
		{"tidelift", "cargo/serde"},
		{"tidelift", "NPM/babel"},
		{"tidelift", "npm/../x"},
		{"tidelift", "npm//x"},
		{"tidelift", "npm/a b"},
		{"tidelift", "maven/group:artifact"},
		{"tidelift", "npm/x?y"},

		{"thanks_dev", "hynek"},
		{"thanks_dev", "u/gl/hynek"},
		{"thanks_dev", "u/gh/"},
		{"thanks_dev", "u/gh/a/b"},
		{"thanks_dev", "u/gh/.."},

		{"custom", ""},
		{"custom", "http://example.com/donate"},
		{"custom", "ftp://example.com/donate"},
		{"custom", "javascript:alert(1)"},
		{"custom", "data:text/html,hi"},
		{"custom", "octocat.com"},
		{"custom", "//example.com/donate"},
		{"custom", "https://"},
		{"custom", "https:///donate"},
		{"custom", "https:example.com"},
		{"custom", "HTTPS://example.com"},
		{"custom", "https://user:pass@example.com/"},
		{"custom", "https://example.com/a b"},
		{"custom", "https://example.com/\x1b[2J"},
		{"custom", " https://example.com"},
	}
	for _, tt := range tests {
		if got, err := (index.Funding{Platform: tt.platform, Handle: tt.handle}).URL(); err == nil {
			t.Errorf("%s %q accepted: %q", tt.platform, tt.handle, got)
		}
	}
}

func TestLoadedFundingResolves(t *testing.T) {
	idx := mustLoad(t, fixture(t, "index.json"))
	want := map[string][]string{
		"bossmods":                {"https://github.com/sponsors/exampleorg", "https://ko-fi.com/example"},
		"bossmods-classic-extras": nil,
		"questhelper":             {"https://example.com/donate"},
	}
	for _, entry := range idx.Entries() {
		var got []string
		for _, link := range entry.Funding {
			url, err := link.URL()
			if err != nil {
				t.Fatalf("%s: %v", entry.ID, err)
			}
			got = append(got, url)
		}
		if len(got) != len(want[entry.ID]) {
			t.Fatalf("%s funding = %v, want %v", entry.ID, got, want[entry.ID])
		}
		for i := range got {
			if got[i] != want[entry.ID][i] {
				t.Errorf("%s funding[%d] = %q, want %q", entry.ID, i, got[i], want[entry.ID][i])
			}
		}
	}
}
