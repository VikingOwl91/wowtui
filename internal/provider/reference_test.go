package provider

import "testing"

func TestParseReferenceGitHubPrefix(t *testing.T) {
	ref, err := ParseReference("github:WeakAuras/WeakAuras2")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Provider != "github" || ref.ID != "WeakAuras/WeakAuras2" {
		t.Errorf("got %+v", ref)
	}
}

func TestParseReferenceBareOwnerRepo(t *testing.T) {
	ref, err := ParseReference("WeakAuras/WeakAuras2")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Provider != "github" || ref.ID != "WeakAuras/WeakAuras2" {
		t.Errorf("got %+v", ref)
	}
}

func TestParseReferenceGitHubURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://github.com/WeakAuras/WeakAuras2", "WeakAuras/WeakAuras2"},
		{"https://github.com/WeakAuras/WeakAuras2/", "WeakAuras/WeakAuras2"},
		{"https://github.com/WeakAuras/WeakAuras2.git", "WeakAuras/WeakAuras2"},
		{"https://github.com/WeakAuras/WeakAuras2/releases", "WeakAuras/WeakAuras2"},
		{"http://github.com/WeakAuras/WeakAuras2", "WeakAuras/WeakAuras2"},
	}
	for _, tt := range tests {
		ref, err := ParseReference(tt.in)
		if err != nil {
			t.Fatalf("%s: %v", tt.in, err)
		}
		if ref.Provider != "github" || ref.ID != tt.want {
			t.Errorf("%s: got %+v, want github %s", tt.in, ref, tt.want)
		}
	}
}

func TestParseReferenceDirectPrefix(t *testing.T) {
	ref, err := ParseReference("direct:https://example.com/addon.zip")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Provider != "direct" || ref.ID != "https://example.com/addon.zip" {
		t.Errorf("got %+v", ref)
	}
}

func TestParseReferenceGenericGit(t *testing.T) {
	ref, err := ParseReference("git:https://git.example.org/team/addon.git")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Provider != "git" || ref.ID != "https://git.example.org/team/addon.git" {
		t.Errorf("ref = %+v", ref)
	}
}

func TestParseReferenceForeignURLIsDirect(t *testing.T) {
	ref, err := ParseReference("https://example.com/addon.zip")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Provider != "direct" || ref.ID != "https://example.com/addon.zip" {
		t.Errorf("got %+v", ref)
	}
}

func TestParseReferenceDirectLoopbackAllowed(t *testing.T) {
	tests := []string{
		"direct:http://127.0.0.1/addon.zip",
		"direct:http://localhost/addon.zip",
		"http://127.0.0.1:8080/addon.zip",
	}
	for _, tt := range tests {
		ref, err := ParseReference(tt)
		if err != nil {
			t.Fatalf("%s: %v", tt, err)
		}
		if ref.Provider != "direct" {
			t.Errorf("%s: got %+v, want direct", tt, ref)
		}
	}
}

func TestParseReferenceInvalid(t *testing.T) {
	tests := []string{
		"",
		"just-a-name",
		"github:",
		"github:onlyowner",
		"github:bad owner/repo",
		"github:owner/repo/extra",
		"https://github.com/onlyowner",
		"ftp://example.com/x.zip",
		"direct:http://example.com/x.zip",
		"http://example.com/x.zip",
	}
	for _, tt := range tests {
		if _, err := ParseReference(tt); err == nil {
			t.Errorf("%q: expected error", tt)
		}
	}
}

func TestParseReferenceLegacyCurseForgeHostIsNeverDirect(t *testing.T) {
	ref, err := ParseReference("https://wow.curseforge.com/projects/tidy-plates-threat-plates")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Provider != "curseforge" || ref.ID != "tidy-plates-threat-plates" {
		t.Errorf("got %+v, want curseforge tidy-plates-threat-plates", ref)
	}

	for _, in := range []string{
		"https://wow.curseforge.com",
		"https://wow.curseforge.com/projects",
		"https://wow.curseforge.com/addons/foo",
	} {
		if ref, err := ParseReference(in); err == nil {
			t.Errorf("%s: got %+v, want error", in, ref)
		}
	}
}

func TestParseReferenceIndexID(t *testing.T) {
	for _, id := range []string{"bossmods", "bossmods-classic-extras", "0day", "a.b_c-1"} {
		ref, err := ParseReference("index:" + id)
		if err != nil {
			t.Fatalf("index:%s: %v", id, err)
		}
		if ref != (Reference{Provider: "index", ID: id}) {
			t.Errorf("index:%s: got %+v", id, ref)
		}
	}
}

func TestFormatReferenceParsesBack(t *testing.T) {
	cases := []struct {
		ref  Reference
		want string
	}{
		{Reference{Provider: "github", ID: "WeakAuras/WeakAuras2"}, "github:WeakAuras/WeakAuras2"},
		{Reference{Provider: "gitlab", ID: "group/project"}, "gitlab:group/project"},
		{Reference{Provider: "gitlab", ID: "group/sub/project"}, "gitlab:group/sub/project"},
		{Reference{Provider: "wowuphub", ID: "123"}, "wowup:123"},
		{Reference{Provider: "curseforge", ID: "12345"}, "curseforge:12345"},
		{Reference{Provider: "git", ID: "https://git.example.org/team/addon.git"}, "git:https://git.example.org/team/addon.git"},
		{Reference{Provider: "git", ID: "ssh://git@git.example.org/team/addon.git"}, "git:ssh://git@git.example.org/team/addon.git"},
		{Reference{Provider: "direct", ID: "https://example.com/addon.zip"}, "direct:https://example.com/addon.zip"},
		{Reference{Provider: "index", ID: "bossmods"}, "index:bossmods"},
	}
	for _, tc := range cases {
		got := FormatReference(tc.ref)
		if got != tc.want {
			t.Errorf("FormatReference(%+v) = %q, want %q", tc.ref, got, tc.want)
		}
		back, err := ParseReference(got)
		if err != nil {
			t.Errorf("ParseReference(%q): %v", got, err)
			continue
		}
		if back != tc.ref {
			t.Errorf("ParseReference(%q) = %+v, want %+v", got, back, tc.ref)
		}
	}
}

func TestFormatReferenceInformationalKinds(t *testing.T) {
	for _, tc := range []struct {
		ref  Reference
		want string
	}{
		{Reference{Provider: "wago", ID: "aB3dE5fG"}, "wago:aB3dE5fG"},
		{Reference{Provider: "wowinterface", ID: "6789"}, "wowinterface:6789"},
	} {
		got := FormatReference(tc.ref)
		if got != tc.want {
			t.Errorf("FormatReference(%+v) = %q, want %q", tc.ref, got, tc.want)
		}
		if ref, err := ParseReference(got); err == nil {
			t.Errorf("%q is informational, ParseReference = %+v, want error", got, ref)
		}
	}
}

func TestParseReferenceRejectsInvalidIndexID(t *testing.T) {
	for _, in := range []string{
		"index:",
		"index:BossMods",
		"index:-bossmods",
		"index:.bossmods",
		"index:_bossmods",
		"index:boss mods",
		"index:owner/repo",
		"index:bossmods:extra",
		"index:bössmods",
	} {
		if ref, err := ParseReference(in); err == nil {
			t.Errorf("%q: got %+v, want error", in, ref)
		}
	}
}
