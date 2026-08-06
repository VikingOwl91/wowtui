package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type GitLab struct {
	client  *http.Client
	token   string
	apiBase string
	webBase string
}

func NewGitLab(token string) *GitLab {
	return &GitLab{
		client:  &http.Client{Timeout: 30 * time.Second},
		token:   token,
		apiBase: "https://gitlab.com/api/v4",
		webBase: "https://gitlab.com",
	}
}

func (g *GitLab) SetAPIBase(base string) {
	g.apiBase = strings.TrimRight(base, "/")
	g.webBase = g.apiBase
	if strings.HasSuffix(g.webBase, "/api/v4") {
		g.webBase = strings.TrimSuffix(g.webBase, "/api/v4")
	}
}

type gitLabRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  struct {
		Sources []struct {
			Format string `json:"format"`
			URL    string `json:"url"`
		} `json:"sources"`
	} `json:"assets"`
}

type gitLabProject struct {
	Name          string `json:"name"`
	PathWithNS    string `json:"path_with_namespace"`
	WebURL        string `json:"web_url"`
	DefaultBranch string `json:"default_branch"`
	Namespace     struct {
		Name string `json:"name"`
	} `json:"namespace"`
	Description string `json:"description"`
}

func (g *GitLab) Resolve(ctx context.Context, ref Reference) (RemoteAddon, error) {
	var project gitLabProject
	status, err := g.get(ctx, g.projectURL(ref.ID), &project)
	if err != nil {
		return RemoteAddon{}, err
	}
	if status == http.StatusNotFound {
		return RemoteAddon{}, fmt.Errorf("gitlab project %s not found", ref.ID)
	}
	if status != http.StatusOK {
		return RemoteAddon{}, fmt.Errorf("gitlab api returned %s", http.StatusText(status))
	}
	return RemoteAddon{Provider: "gitlab", ID: ref.ID, Name: project.PathWithNS, URL: project.WebURL, Author: project.Namespace.Name, Summary: project.Description}, nil
}

func (g *GitLab) Search(ctx context.Context, query string, _ wow.GameFlavor) ([]RemoteAddon, error) {
	var projects []gitLabProject
	status, err := g.get(ctx, g.apiBase+"/projects?search="+url.QueryEscape(query)+"&per_page=50", &projects)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gitlab search returned %s", http.StatusText(status))
	}
	results := make([]RemoteAddon, 0, len(projects))
	for _, project := range projects {
		results = append(results, RemoteAddon{Provider: "gitlab", ID: project.PathWithNS, Name: project.PathWithNS, URL: project.WebURL, Author: project.Namespace.Name, Summary: project.Description})
	}
	return results, nil
}

func (g *GitLab) Latest(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor, source string) (Release, error) {
	switch source {
	case "", "auto", "release":
		release, err := g.latestRelease(ctx, addon)
		if err == nil {
			return release, nil
		}
		if source == "release" {
			return Release{}, err
		}
		if release, tagErr := g.latestTag(ctx, addon); tagErr == nil {
			return release, nil
		}
		return g.latestBranch(ctx, addon)
	case "tag":
		return g.latestTag(ctx, addon)
	case "branch":
		return g.latestBranch(ctx, addon)
	default:
		return Release{}, fmt.Errorf("unknown source %q", source)
	}
}

func (g *GitLab) latestRelease(ctx context.Context, addon RemoteAddon) (Release, error) {
	var rel gitLabRelease
	status, err := g.get(ctx, g.projectURL(addon.ID)+"/releases/permalink/latest", &rel)
	if err != nil {
		return Release{}, err
	}
	if status != http.StatusOK {
		return Release{}, fmt.Errorf("no stable gitlab release found for %s", addon.ID)
	}
	for _, source := range rel.Assets.Sources {
		if strings.EqualFold(source.Format, "zip") {
			return Release{Kind: KindRelease, Tag: rel.TagName, AssetName: source.URL, DownloadURL: source.URL}, nil
		}
	}
	return Release{}, fmt.Errorf("gitlab release %s has no zip source", rel.TagName)
}

func (g *GitLab) latestTag(ctx context.Context, addon RemoteAddon) (Release, error) {
	var tags []struct {
		Name string `json:"name"`
	}
	status, err := g.get(ctx, g.projectURL(addon.ID)+"/repository/tags?order_by=version&sort=desc&per_page=1", &tags)
	if err != nil {
		return Release{}, err
	}
	if status != http.StatusOK || len(tags) == 0 {
		return Release{}, fmt.Errorf("no gitlab tags found for %s", addon.ID)
	}
	tag := tags[0].Name
	return Release{Kind: KindTag, Tag: tag, AssetName: repoBaseName(addon.ID) + "-" + tag + ".zip", DownloadURL: g.archiveURL(addon.ID, tag)}, nil
}

func (g *GitLab) latestBranch(ctx context.Context, addon RemoteAddon) (Release, error) {
	var project gitLabProject
	status, err := g.get(ctx, g.projectURL(addon.ID), &project)
	if err != nil || status != http.StatusOK {
		return Release{}, fmt.Errorf("could not resolve gitlab default branch for %s", addon.ID)
	}
	branch := project.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	return Release{Kind: KindBranch, Tag: branch, AssetName: repoBaseName(addon.ID) + "-" + branch + ".zip", DownloadURL: g.archiveURL(addon.ID, branch)}, nil
}

func (g *GitLab) Download(ctx context.Context, release Release) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	g.auth(req)
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("gitlab download returned %s", resp.Status)
	}
	return resp.Body, nil
}

func (g *GitLab) projectURL(project string) string {
	return g.apiBase + "/projects/" + url.PathEscape(project)
}

func (g *GitLab) archiveURL(project, ref string) string {
	return g.webBase + "/" + project + "/-/archive/" + url.PathEscape(ref) + "/" + repoBaseName(project) + "-" + ref + ".zip"
}

func (g *GitLab) auth(req *http.Request) {
	if g.token != "" {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}
	req.Header.Set("User-Agent", "wowtui")
}

func (g *GitLab) get(ctx context.Context, rawURL string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	g.auth(req)
	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}
