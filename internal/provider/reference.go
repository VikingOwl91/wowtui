package provider

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

type Reference struct {
	Provider string
	ID       string
}

var ownerRepoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
var gitLabProjectPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)+$`)
var numericIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
var indexIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func ParseReference(s string) (Reference, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Reference{}, fmt.Errorf("empty addon reference")
	}

	if rest, ok := strings.CutPrefix(s, "github:"); ok {
		if !ownerRepoPattern.MatchString(rest) {
			return Reference{}, fmt.Errorf("invalid github reference %q, want Owner/Repo", s)
		}
		return Reference{Provider: "github", ID: rest}, nil
	}

	if rest, ok := strings.CutPrefix(s, "gitlab:"); ok {
		if !gitLabProjectPattern.MatchString(rest) {
			return Reference{}, fmt.Errorf("invalid gitlab reference %q", s)
		}
		return Reference{Provider: "gitlab", ID: rest}, nil
	}

	if rest, ok := strings.CutPrefix(s, "wowup:"); ok {
		if !numericIDPattern.MatchString(rest) {
			return Reference{}, fmt.Errorf("invalid wowup reference %q", s)
		}
		return Reference{Provider: "wowuphub", ID: rest}, nil
	}

	if rest, ok := strings.CutPrefix(s, "curseforge:"); ok {
		if !numericIDPattern.MatchString(rest) {
			return Reference{}, fmt.Errorf("invalid CurseForge reference %q", s)
		}
		return Reference{Provider: "curseforge", ID: rest}, nil
	}

	if rest, ok := strings.CutPrefix(s, "index:"); ok {
		if !indexIDPattern.MatchString(rest) {
			return Reference{}, fmt.Errorf("invalid index reference %q, want a lowercase index id", s)
		}
		return Reference{Provider: "index", ID: rest}, nil
	}

	if rest, ok := strings.CutPrefix(s, "git:"); ok {
		if err := validateGitURL(rest); err != nil {
			return Reference{}, err
		}
		return Reference{Provider: "git", ID: rest}, nil
	}

	if rest, ok := strings.CutPrefix(s, "direct:"); ok {
		if err := validateDirectURL(rest); err != nil {
			return Reference{}, err
		}
		return Reference{Provider: "direct", ID: rest}, nil
	}

	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		u, err := url.Parse(s)
		if err != nil {
			return Reference{}, fmt.Errorf("invalid URL %q: %w", s, err)
		}
		if u.Host == "github.com" || u.Host == "www.github.com" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) < 2 {
				return Reference{}, fmt.Errorf("github URL %q has no Owner/Repo", s)
			}
			id := strings.TrimSuffix(parts[0]+"/"+parts[1], ".git")
			if !ownerRepoPattern.MatchString(id) {
				return Reference{}, fmt.Errorf("invalid github reference in %q", s)
			}
			return Reference{Provider: "github", ID: id}, nil
		}
		if u.Host == "gitlab.com" || u.Host == "www.gitlab.com" {
			id := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
			id = strings.TrimPrefix(id, "/")
			if !gitLabProjectPattern.MatchString(id) {
				return Reference{}, fmt.Errorf("invalid gitlab reference in %q", s)
			}
			return Reference{Provider: "gitlab", ID: id}, nil
		}
		if u.Host == "wowup.io" || u.Host == "www.wowup.io" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 2 && parts[0] == "addons" && numericIDPattern.MatchString(parts[1]) {
				return Reference{Provider: "wowuphub", ID: parts[1]}, nil
			}
			return Reference{}, fmt.Errorf("invalid WowUp addon URL %q", s)
		}
		if u.Host == "curseforge.com" || u.Host == "www.curseforge.com" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 3 && parts[0] == "wow" && parts[1] == "addons" {
				return Reference{Provider: "curseforge", ID: parts[2]}, nil
			}
			return Reference{}, fmt.Errorf("invalid CurseForge addon URL %q", s)
		}
		if u.Host == "wow.curseforge.com" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 && parts[0] == "projects" && parts[1] != "" {
				return Reference{Provider: "curseforge", ID: parts[1]}, nil
			}
			return Reference{}, fmt.Errorf("invalid CurseForge addon URL %q", s)
		}
		if err := validateDirectURL(s); err != nil {
			return Reference{}, err
		}
		return Reference{Provider: "direct", ID: s}, nil
	}

	if ownerRepoPattern.MatchString(s) {
		return Reference{Provider: "github", ID: s}, nil
	}

	return Reference{}, fmt.Errorf("unrecognized addon reference %q", s)
}

// FormatReference returns the prefixed text form of ref that ParseReference
// reads back, such as github:Owner/Repo, gitlab:group/project, wowup:123 or
// git:https://host/team/repo.git. Other providers, such as the informational
// wago and wowinterface hints, print as provider:id.
func FormatReference(ref Reference) string {
	prefix := ref.Provider
	if prefix == "wowuphub" {
		prefix = "wowup"
	}
	return prefix + ":" + ref.ID
}

func validateDirectURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return fmt.Errorf("direct reference must be a valid URL, got %q", s)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("direct reference must be an https URL (http only allowed for localhost), got %q", s)
}

func validateGitURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "ssh" && u.Scheme != "git") {
		return fmt.Errorf("git reference must be an https, ssh, or git URL, got %q", s)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
