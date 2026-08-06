package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type WowUpHub struct {
	client  *http.Client
	baseURL string
}

func NewWowUpHub() *WowUpHub {
	return &WowUpHub{client: &http.Client{Timeout: 30 * time.Second}, baseURL: "https://hub.wowup.io"}
}

func (h *WowUpHub) SetBaseURL(base string) { h.baseURL = strings.TrimRight(base, "/") }

type wowUpAddonResponse struct {
	Addon wowUpAddon `json:"addon"`
}

type wowUpSearchResponse struct {
	Addons []wowUpAddon `json:"addons"`
}

type wowUpAddon struct {
	ID             int64          `json:"id"`
	RepositoryName string         `json:"repository_name"`
	Description    string         `json:"description"`
	Homepage       string         `json:"homepage"`
	Releases       []wowUpRelease `json:"releases"`
	Current        *wowUpRelease  `json:"current_release"`
}

type wowUpRelease struct {
	ID           int64              `json:"id"`
	TagName      string             `json:"tag_name"`
	DownloadURL  string             `json:"download_url"`
	Body         string             `json:"body"`
	PublishedAt  string             `json:"published_at"`
	GameVersions []wowUpGameVersion `json:"game_versions"`
}

type wowUpGameVersion struct {
	GameType string `json:"game_type"`
	Version  string `json:"version"`
	Title    string `json:"title"`
}

func (h *WowUpHub) Resolve(ctx context.Context, ref Reference) (RemoteAddon, error) {
	var response wowUpAddonResponse
	status, err := h.get(ctx, h.baseURL+"/addons/"+url.PathEscape(ref.ID), &response)
	if err != nil {
		return RemoteAddon{}, err
	}
	if status != http.StatusOK {
		return RemoteAddon{}, fmt.Errorf("WowUpHub returned %s for addon %s", http.StatusText(status), ref.ID)
	}
	return RemoteAddon{Provider: "wowuphub", ID: ref.ID, Name: response.Addon.RepositoryName, URL: "https://wowup.io/addons/" + ref.ID}, nil
}

func (h *WowUpHub) Latest(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor, source string) (Release, error) {
	var response wowUpAddonResponse
	status, err := h.get(ctx, h.baseURL+"/addons/"+url.PathEscape(addon.ID), &response)
	if err != nil {
		return Release{}, err
	}
	if status != http.StatusOK {
		return Release{}, fmt.Errorf("WowUpHub returned %s for addon %s", http.StatusText(status), addon.ID)
	}

	best := make([]wowUpRelease, 0)
	for _, release := range response.Addon.Releases {
		if wowUpReleaseMatches(release, flavor) {
			best = append(best, release)
		}
	}
	if len(best) == 0 && response.Addon.Current != nil && wowUpReleaseMatches(*response.Addon.Current, flavor) {
		best = append(best, *response.Addon.Current)
	}
	if len(best) == 0 {
		return Release{}, fmt.Errorf("no WowUpHub release matches flavor %s", flavor)
	}
	sort.Slice(best, func(i, j int) bool { return best[i].ID > best[j].ID })
	release := best[0]
	return Release{Kind: KindRelease, ID: release.ID, Tag: release.TagName, AssetName: release.DownloadURL, DownloadURL: release.DownloadURL}, nil
}

func (h *WowUpHub) Search(ctx context.Context, query string, flavor wow.GameFlavor) ([]RemoteAddon, error) {
	gameType := wowUpGameType(flavor)
	endpoint := h.baseURL + "/addons/search/" + url.PathEscape(gameType) + "?query=" + url.QueryEscape(query) + "&limit=50"
	var response wowUpSearchResponse
	status, err := h.get(ctx, endpoint, &response)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("WowUpHub search returned %s", http.StatusText(status))
	}
	results := make([]RemoteAddon, 0, len(response.Addons))
	for _, addon := range response.Addons {
		results = append(results, RemoteAddon{Provider: "wowuphub", ID: strconv.FormatInt(addon.ID, 10), Name: addon.RepositoryName, URL: "https://wowup.io/addons/" + strconv.FormatInt(addon.ID, 10), Summary: addon.Description})
	}
	return results, nil
}

func (h *WowUpHub) Download(ctx context.Context, release Release) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("WowUpHub download returned %s", resp.Status)
	}
	return resp.Body, nil
}

func (h *WowUpHub) get(ctx context.Context, endpoint string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "wowtui")
	resp, err := h.client.Do(req)
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

func wowUpReleaseMatches(release wowUpRelease, flavor wow.GameFlavor) bool {
	want := wowUpGameType(flavor)
	for _, version := range release.GameVersions {
		if version.GameType == want {
			return true
		}
		if flavor == wow.Classic && (version.GameType == "cata" || version.GameType == "mop" || version.GameType == "wotlk" || version.GameType == "bc") {
			return true
		}
	}
	return false
}

func wowUpGameType(flavor wow.GameFlavor) string {
	switch flavor {
	case wow.ClassicEra:
		return "classic"
	case wow.Classic:
		return "mists"
	default:
		return "retail"
	}
}
