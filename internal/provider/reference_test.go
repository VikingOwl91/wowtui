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
