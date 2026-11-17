package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

var (
	errNoRelease = errors.New("no stable release found")
	errNoTags    = errors.New("no tags found")
)

type GitHub struct {
	client  *http.Client
	token   string
	apiBase string
	webBase string
	cache   *httpCache
}

func NewGitHub(token string) *GitHub {
	return &GitHub{
		client:  &http.Client{Timeout: 30 * time.Second},
		token:   token,
		apiBase: "https://api.github.com",
		webBase: "https://github.com",
	}
}

func (g *GitHub) SetAPIBase(base string) {
	g.apiBase = base
	g.webBase = base
}

// SetCacheDir keeps API responses in dir for conditional requests. An empty
// dir disables the cache.
func (g *GitHub) SetCacheDir(dir string) {
	g.cache = newHTTPCache(dir)
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type ghRelease struct {
	ID      int64     `json:"id"`
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghRepo struct {
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	Description string `json:"description"`
	Stars       int64  `json:"stargazers_count"`
}

func (g *GitHub) Resolve(ctx context.Context, ref Reference) (RemoteAddon, error) {
	var repo ghRepo
	status, err := g.get(ctx, g.apiBase+"/repos/"+ref.ID, &repo)
	if err != nil {
		return RemoteAddon{}, err
	}
	if status == http.StatusNotFound {
		return RemoteAddon{}, fmt.Errorf("github repository %s not found", ref.ID)
	}
	if status != http.StatusOK {
		return RemoteAddon{}, fmt.Errorf("github api returned %s for %s", http.StatusText(status), ref.ID)
	}
	return RemoteAddon{Provider: "github", ID: ref.ID, Name: repo.FullName, URL: repo.HTMLURL, Author: repo.Owner.Login, Summary: repo.Description, Downloads: repo.Stars}, nil
}

func (g *GitHub) Search(ctx context.Context, query string, _ wow.GameFlavor) ([]RemoteAddon, error) {
	var response struct {
		Items []ghRepo `json:"items"`
	}
	status, err := g.get(ctx, g.apiBase+"/search/repositories?q="+url.QueryEscape(query+" wow addon")+"&per_page=50", &response)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("github search returned %s", http.StatusText(status))
	}
	results := make([]RemoteAddon, 0, len(response.Items))
	for _, repo := range response.Items {
		results = append(results, RemoteAddon{Provider: "github", ID: repo.FullName, Name: repo.FullName, URL: repo.HTMLURL, Author: repo.Owner.Login, Summary: repo.Description, Downloads: repo.Stars})
	}
	return results, nil
}

func (g *GitHub) Latest(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor, source string) (Release, error) {
	switch source {
	case "", "auto":
		release, err := g.latestRelease(ctx, addon, flavor)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, errNoRelease) {
			return Release{}, err
		}
		release, err = g.latestTag(ctx, addon)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, errNoTags) {
			return Release{}, err
		}
		return g.latestBranch(ctx, addon)
	case "release":
		release, err := g.latestRelease(ctx, addon, flavor)
		if errors.Is(err, errNoRelease) {
			return Release{}, fmt.Errorf("no stable release found for %s (source pinned to release)", addon.ID)
		}
		return release, err
	case "tag":
		release, err := g.latestTag(ctx, addon)
		if errors.Is(err, errNoTags) {
			return Release{}, fmt.Errorf("no tags found for %s (source pinned to tag)", addon.ID)
		}
		return release, err
	case "branch":
		return g.latestBranch(ctx, addon)
	default:
		return Release{}, fmt.Errorf("unknown source %q (release, tag, branch)", source)
	}
}

func (g *GitHub) latestRelease(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor) (Release, error) {
	var rel ghRelease
	status, err := g.get(ctx, g.apiBase+"/repos/"+addon.ID+"/releases/latest", &rel)
	if err != nil {
		return Release{}, err
	}
	if status == http.StatusNotFound {
		return Release{}, errNoRelease
	}
	if status != http.StatusOK {
		return Release{}, fmt.Errorf("github api returned %s for %s", http.StatusText(status), addon.ID)
	}

	names := make([]string, 0, len(rel.Assets))
	for _, asset := range rel.Assets {
		names = append(names, asset.Name)
	}
	picked, ok, err := g.releaseJSONAsset(ctx, rel.Assets, names, flavor)
	if err != nil {
		return Release{}, fmt.Errorf("%s (flavor %s): %w", addon.ID, flavor, err)
	}
	if !ok {
		picked, err = PickAsset(names, flavor)
		if err != nil {
			if rel.TagName == "" {
				return Release{}, fmt.Errorf("%s: %w", addon.ID, err)
			}
			return g.tagArchive(addon.ID, rel.TagName), nil
		}
	}

	for _, asset := range rel.Assets {
		if asset.Name == picked {
			return Release{
				Kind:        KindRelease,
				ID:          rel.ID,
				Tag:         rel.TagName,
				AssetName:   asset.Name,
				DownloadURL: asset.BrowserDownloadURL,
				Size:        asset.Size,
			}, nil
		}
	}
	return Release{}, fmt.Errorf("asset %s disappeared from release", picked)
}

func (g *GitHub) latestTag(ctx context.Context, addon RemoteAddon) (Release, error) {
	var tags []struct {
		Name string `json:"name"`
	}
	status, err := g.get(ctx, g.apiBase+"/repos/"+addon.ID+"/tags?per_page=50", &tags)
	if err != nil {
		return Release{}, err
	}
	if status == http.StatusNotFound {
		return Release{}, errNoTags
	}
	if status != http.StatusOK {
		return Release{}, fmt.Errorf("github api returned %s for %s", http.StatusText(status), addon.ID)
	}

	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	best := pickTag(names)
	if best == "" {
		return Release{}, errNoTags
	}
	return g.tagArchive(addon.ID, best), nil
}

func (g *GitHub) latestBranch(ctx context.Context, addon RemoteAddon) (Release, error) {
	var repo ghRepo
	status, err := g.get(ctx, g.apiBase+"/repos/"+addon.ID, &repo)
	if err != nil {
		return Release{}, err
	}
	if status == http.StatusNotFound {
		return Release{}, fmt.Errorf("github repository %s not found", addon.ID)
	}
	branch := repo.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	var commits []struct {
		SHA string `json:"sha"`
	}
	status, err = g.get(ctx, g.apiBase+"/repos/"+addon.ID+"/commits?sha="+url.QueryEscape(branch)+"&per_page=1", &commits)
	if err != nil {
		return Release{}, err
	}
	if status != http.StatusOK || len(commits) == 0 || commits[0].SHA == "" {
		return Release{}, fmt.Errorf("could not determine latest commit on branch %s of %s", branch, addon.ID)
	}

	return Release{
		Kind:        KindBranch,
		Tag:         branch,
		Commit:      commits[0].SHA,
		AssetName:   repoBaseName(addon.ID) + "-" + branch + ".zip",
		DownloadURL: g.webBase + "/" + addon.ID + "/archive/refs/heads/" + url.PathEscape(branch) + ".zip",
	}, nil
}

func (g *GitHub) tagArchive(repo, tag string) Release {
	return Release{
		Kind:        KindTag,
		Tag:         tag,
		AssetName:   repoBaseName(repo) + "-" + tag + ".zip",
		DownloadURL: g.webBase + "/" + repo + "/archive/refs/tags/" + url.PathEscape(tag) + ".zip",
	}
}

func repoBaseName(repo string) string {
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return repo[idx+1:]
	}
	return repo
}

func (g *GitHub) Download(ctx context.Context, release Release) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download of %s returned %s", release.AssetName, resp.Status)
	}
	return resp.Body, nil
}

func (g *GitHub) get(ctx context.Context, url string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wowtui")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	cached, haveCached := g.cache.load(url)
	if haveCached {
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.LastModified != "" {
			req.Header.Set("If-Modified-Since", cached.LastModified)
		}
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return resp.StatusCode, g.rateLimitError(resp.Header.Get("X-RateLimit-Reset"))
	}
	if resp.StatusCode == http.StatusNotModified && haveCached {
		if out != nil {
			if err := json.Unmarshal(cached.Body, out); err != nil {
				return http.StatusOK, fmt.Errorf("decoding cached github response: %w", err)
			}
		}
		return http.StatusOK, nil
	}
	if resp.StatusCode == http.StatusOK && out != nil {
		head, err := io.ReadAll(io.LimitReader(resp.Body, maxCachedBody+1))
		if err != nil {
			return resp.StatusCode, fmt.Errorf("reading github response: %w", err)
		}
		if err := json.NewDecoder(io.MultiReader(bytes.NewReader(head), resp.Body)).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decoding github response: %w", err)
		}
		g.cache.store(cachedResponse{URL: url, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"), Body: head})
	}
	return resp.StatusCode, nil
}

func (g *GitHub) rateLimitError(reset string) error {
	msg := "github api rate limit exhausted"
	if g.token != "" {
		msg += " for the configured token"
	}
	if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
		msg += ", resets at " + time.Unix(seconds, 0).Format("15:04:05 MST")
	}
	if g.token == "" {
		msg += "; a token via WOWTUI_GITHUB_TOKEN or providers.github.token raises the limit"
	}
	return errors.New(msg)
}
