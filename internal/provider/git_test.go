package provider

import "testing"

func TestGitRepoName(t *testing.T) {
	tests := map[string]string{
		"https://github.com/Owner/Repo.git":        "Repo",
		"https://git.example.org/team/addon":       "addon",
		"ssh://git@git.example.org/team/addon.git": "addon",
	}
	for input, want := range tests {
		if got := gitRepoName(input); got != want {
			t.Errorf("gitRepoName(%q) = %q, want %q", input, got, want)
		}
	}
}
