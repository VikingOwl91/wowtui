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

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
