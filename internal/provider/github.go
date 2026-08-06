package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type GitHub struct {
	client  *http.Client
	token   string
	apiBase string
}

func NewGitHub(token string) *GitHub {
	return &GitHub{
		client:  &http.Client{Timeout: 30 * time.Second},
		token:   token,
		apiBase: "https://api.github.com",
	}
}

func (g *GitHub) SetAPIBase(base string) {
	g.apiBase = base
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
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
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
	return RemoteAddon{Provider: "github", ID: ref.ID, Name: repo.FullName, URL: repo.HTMLURL}, nil
}

func (g *GitHub) LatestRelease(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor) (Release, error) {
	var rel ghRelease
	status, err := g.get(ctx, g.apiBase+"/repos/"+addon.ID+"/releases/latest", &rel)
	if err != nil {
		return Release{}, err
	}
	if status == http.StatusNotFound {
		return Release{}, fmt.Errorf("no stable release found for %s", addon.ID)
	}
	if status != http.StatusOK {
		return Release{}, fmt.Errorf("github api returned %s for %s", http.StatusText(status), addon.ID)
	}

	names := make([]string, 0, len(rel.Assets))
	for _, asset := range rel.Assets {
		names = append(names, asset.Name)
	}
	picked, err := PickAsset(names, flavor)
	if err != nil {
		return Release{}, fmt.Errorf("%s: %w", addon.ID, err)
	}

	for _, asset := range rel.Assets {
		if asset.Name == picked {
			return Release{
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

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decoding github response: %w", err)
		}
	}
	return resp.StatusCode, nil
}
