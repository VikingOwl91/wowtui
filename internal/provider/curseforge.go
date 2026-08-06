package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

const curseForgeWoWGameID = 1

type CurseForge struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func NewCurseForge(apiKey string) *CurseForge {
	return &CurseForge{client: &http.Client{Timeout: 30 * time.Second}, apiKey: apiKey, baseURL: "https://api.curseforge.com"}
}

func (c *CurseForge) SetBaseURL(base string) { c.baseURL = strings.TrimRight(base, "/") }

func (c *CurseForge) Authenticate(ctx context.Context) error {
	var response struct {
		Data []json.RawMessage `json:"data"`
	}
	status, err := c.get(ctx, c.baseURL+"/v1/games?pageSize=1", &response)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("CurseForge authentication returned HTTP %d", status)
	}
	return nil
}

type curseForgeMod struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Summary     string                `json:"summary"`
	Links       map[string]string     `json:"links"`
	LatestFiles []curseForgeFileIndex `json:"latestFilesIndexes"`
}

type curseForgeFileIndex struct {
	FileID          int64  `json:"fileId"`
	Filename        string `json:"filename"`
	GameVersion     string `json:"gameVersion"`
	GameVersionType int64  `json:"gameVersionTypeId"`
}

type curseForgeFile struct {
	ID          int64  `json:"id"`
	FileName    string `json:"fileName"`
	DisplayName string `json:"displayName"`
	DownloadURL string `json:"downloadUrl"`
	ReleaseType int    `json:"releaseType"`
}

type curseForgeVersionType struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (c *CurseForge) Resolve(ctx context.Context, ref Reference) (RemoteAddon, error) {
	mod, err := c.getMod(ctx, ref.ID)
	if err != nil {
		return RemoteAddon{}, err
	}
	return RemoteAddon{Provider: "curseforge", ID: strconv.FormatInt(mod.ID, 10), Name: mod.Name, URL: "https://www.curseforge.com/wow/addons/" + mod.Slug}, nil
}

func (c *CurseForge) Latest(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor, _ string) (Release, error) {
	mod, err := c.getMod(ctx, addon.ID)
	if err != nil {
		return Release{}, err
	}
	versionType, err := c.versionType(ctx, flavor)
	if err != nil {
		return Release{}, err
	}

	var fileID int64
	for _, index := range mod.LatestFiles {
		if index.GameVersionType == versionType.ID {
			fileID = index.FileID
			break
		}
	}
	if fileID == 0 {
		return Release{}, fmt.Errorf("no CurseForge file matches flavor %s", flavor)
	}
	file, err := c.getFile(ctx, mod.ID, fileID)
	if err != nil {
		return Release{}, err
	}
	downloadURL := file.DownloadURL
	if downloadURL == "" {
		downloadURL, err = c.getDownloadURL(ctx, mod.ID, file.ID)
		if err != nil {
			return Release{}, err
		}
	}
	tag := file.DisplayName
	if tag == "" {
		tag = file.FileName
	}
	return Release{Kind: KindRelease, ID: file.ID, Tag: tag, AssetName: file.FileName, DownloadURL: downloadURL}, nil
}

func (c *CurseForge) Search(ctx context.Context, query string, _ wow.GameFlavor) ([]RemoteAddon, error) {
	endpoint := c.baseURL + "/v1/mods/search?gameId=" + strconv.Itoa(curseForgeWoWGameID) + "&searchFilter=" + url.QueryEscape(query) + "&pageSize=50"
	var response struct {
		Data []curseForgeMod `json:"data"`
	}
	if _, err := c.get(ctx, endpoint, &response); err != nil {
		return nil, err
	}
	results := make([]RemoteAddon, 0, len(response.Data))
	for _, mod := range response.Data {
		results = append(results, RemoteAddon{Provider: "curseforge", ID: strconv.FormatInt(mod.ID, 10), Name: mod.Name, URL: "https://www.curseforge.com/wow/addons/" + mod.Slug, Summary: mod.Summary})
	}
	return results, nil
}

func (c *CurseForge) Download(ctx context.Context, release Release) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("CurseForge download returned %s", resp.Status)
	}
	return resp.Body, nil
}

func (c *CurseForge) getMod(ctx context.Context, id string) (curseForgeMod, error) {
	var response struct {
		Data curseForgeMod `json:"data"`
	}
	status, err := c.get(ctx, c.baseURL+"/v1/mods/"+url.PathEscape(id), &response)
	if err != nil {
		return curseForgeMod{}, err
	}
	if status != http.StatusOK {
		return curseForgeMod{}, fmt.Errorf("CurseForge mod lookup returned %s", http.StatusText(status))
	}
	return response.Data, nil
}

func (c *CurseForge) getFile(ctx context.Context, modID, fileID int64) (curseForgeFile, error) {
	var response struct {
		Data curseForgeFile `json:"data"`
	}
	status, err := c.get(ctx, c.baseURL+"/v1/mods/"+strconv.FormatInt(modID, 10)+"/files/"+strconv.FormatInt(fileID, 10), &response)
	if err != nil {
		return curseForgeFile{}, err
	}
	if status != http.StatusOK {
		return curseForgeFile{}, fmt.Errorf("CurseForge file lookup returned %s", http.StatusText(status))
	}
	return response.Data, nil
}

func (c *CurseForge) getDownloadURL(ctx context.Context, modID, fileID int64) (string, error) {
	var response struct {
		Data string `json:"data"`
	}
	status, err := c.get(ctx, c.baseURL+"/v1/mods/"+strconv.FormatInt(modID, 10)+"/files/"+strconv.FormatInt(fileID, 10)+"/download-url", &response)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || response.Data == "" {
		return "", fmt.Errorf("CurseForge returned no download URL")
	}
	return response.Data, nil
}

func (c *CurseForge) versionType(ctx context.Context, flavor wow.GameFlavor) (curseForgeVersionType, error) {
	var response struct {
		Data []curseForgeVersionType `json:"data"`
	}
	status, err := c.get(ctx, c.baseURL+"/v1/games/"+strconv.Itoa(curseForgeWoWGameID)+"/version-types", &response)
	if err != nil {
		return curseForgeVersionType{}, err
	}
	if status != http.StatusOK {
		return curseForgeVersionType{}, fmt.Errorf("CurseForge version types returned %s", http.StatusText(status))
	}
	for _, item := range response.Data {
		name := strings.ToLower(item.Name + " " + item.Slug)
		if flavor == wow.Retail && strings.Contains(name, "retail") {
			return item, nil
		}
		if flavor == wow.ClassicEra && strings.Contains(name, "classic") {
			return item, nil
		}
		if flavor == wow.Classic && (strings.Contains(name, "cata") || strings.Contains(name, "mists") || strings.Contains(name, "wrath")) {
			return item, nil
		}
	}
	return curseForgeVersionType{}, fmt.Errorf("no CurseForge version type matches flavor %s", flavor)
}

func (c *CurseForge) get(ctx context.Context, endpoint string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("User-Agent", "wowtui")
	resp, err := c.client.Do(req)
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
