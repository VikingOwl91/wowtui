package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// statusError is an HTTP answer the caller did not expect, such as a 403 for
// a repository blocked by GitHub or a 422 for a query GitHub rejects.
type statusError struct {
	Path   string
	Status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.Path, e.Status)
}

func isStatusError(err error) bool {
	var se *statusError
	return errors.As(err, &se)
}

// api holds the typed GitHub reads discovery needs. Every call is a GET
// through the cached client.
type api struct {
	c *Client
}

type searchHit struct {
	Repo        string
	Fork        bool
	Path        string
	Description string
	OwnerType   string
}

type searchResult struct {
	Total      int
	Incomplete bool
	Hits       []searchHit
}

func (a *api) get(ctx context.Context, req Request) (Response, error) {
	resp, err := a.c.Get(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if resp.Status != 200 && resp.Status != 404 {
		return Response{}, &statusError{Path: req.Path, Status: resp.Status}
	}
	return resp, nil
}

func (a *api) searchCode(ctx context.Context, query string) (searchResult, error) {
	resp, err := a.get(ctx, Request{Path: "search/code", Params: map[string]string{"q": query, "per_page": "100"}})
	if err != nil {
		return searchResult{}, err
	}
	if resp.Status == 404 {
		return searchResult{}, &statusError{Path: "search/code", Status: 404}
	}
	var body struct {
		Total      int  `json:"total_count"`
		Incomplete bool `json:"incomplete_results"`
		Items      []struct {
			Path       string `json:"path"`
			Repository struct {
				FullName    string `json:"full_name"`
				Fork        bool   `json:"fork"`
				Description string `json:"description"`
				Owner       struct {
					Type string `json:"type"`
				} `json:"owner"`
			} `json:"repository"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return searchResult{}, fmt.Errorf("decode code search: %w", err)
	}
	res := searchResult{Total: body.Total, Incomplete: body.Incomplete}
	for _, item := range body.Items {
		res.Hits = append(res.Hits, searchHit{
			Repo:        item.Repository.FullName,
			Fork:        item.Repository.Fork,
			Path:        item.Path,
			Description: item.Repository.Description,
			OwnerType:   item.Repository.Owner.Type,
		})
	}
	return res, nil
}

type repoMeta struct {
	FullName      string
	Stars         int
	Fork          bool
	Archived      bool
	PushedAt      string
	DefaultBranch string
	Description   string
	Parent        string
}

// repoInfo returns nil when the repository does not exist.
func (a *api) repoInfo(ctx context.Context, repo string) (*repoMeta, error) {
	resp, err := a.get(ctx, Request{Path: "repos/" + repo})
	if err != nil || resp.Status == 404 {
		return nil, err
	}
	var body struct {
		FullName      string `json:"full_name"`
		Stars         int    `json:"stargazers_count"`
		Fork          bool   `json:"fork"`
		Archived      bool   `json:"archived"`
		PushedAt      string `json:"pushed_at"`
		DefaultBranch string `json:"default_branch"`
		Description   string `json:"description"`
		Parent        *struct {
			FullName string `json:"full_name"`
		} `json:"parent"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("decode %s: %w", repo, err)
	}
	meta := &repoMeta{
		FullName: body.FullName, Stars: body.Stars, Fork: body.Fork, Archived: body.Archived,
		PushedAt: body.PushedAt, DefaultBranch: body.DefaultBranch, Description: body.Description,
	}
	if body.Parent != nil {
		meta.Parent = body.Parent.FullName
	}
	return meta, nil
}

type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

func contentsPath(repo, dir string) string {
	p := "repos/" + repo + "/contents"
	for _, seg := range strings.Split(dir, "/") {
		if seg != "" {
			p += "/" + url.PathEscape(seg)
		}
	}
	return p
}

// listDir lists a directory of the default branch; dir "" is the root.
// found is false when the path does not exist or is not a directory.
func (a *api) listDir(ctx context.Context, repo, dir string) (entries []dirEntry, found bool, err error) {
	resp, err := a.get(ctx, Request{Path: contentsPath(repo, dir)})
	if err != nil || resp.Status == 404 {
		return nil, false, err
	}
	if err := json.Unmarshal(resp.Body, &entries); err != nil {
		return nil, false, nil
	}
	return entries, true, nil
}

// readFile returns the content of a file of the default branch. found is
// false for a missing path, a directory and a file too large for the API to
// inline.
func (a *api) readFile(ctx context.Context, repo, file string) (data []byte, found bool, err error) {
	resp, err := a.get(ctx, Request{Path: contentsPath(repo, file)})
	if err != nil || resp.Status == 404 {
		return nil, false, err
	}
	var body struct {
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil || body.Type != "file" || body.Encoding != "base64" {
		return nil, false, nil
	}
	data, err = base64.StdEncoding.DecodeString(strings.ReplaceAll(body.Content, "\n", ""))
	if err != nil {
		return nil, false, fmt.Errorf("decode %s/%s: %w", repo, file, err)
	}
	return data, true, nil
}

type assetInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type releaseInfo struct {
	Tag         string      `json:"tag_name"`
	PublishedAt string      `json:"published_at"`
	Assets      []assetInfo `json:"assets"`
}

// latestRelease returns nil when the repository has no release.
func (a *api) latestRelease(ctx context.Context, repo string) (*releaseInfo, error) {
	resp, err := a.get(ctx, Request{Path: "repos/" + repo + "/releases/latest"})
	if err != nil || resp.Status == 404 {
		return nil, err
	}
	var rel releaseInfo
	if err := json.Unmarshal(resp.Body, &rel); err != nil {
		return nil, fmt.Errorf("decode latest release of %s: %w", repo, err)
	}
	return &rel, nil
}

// assetBody downloads a release asset through the API.
func (a *api) assetBody(ctx context.Context, repo string, id int64) ([]byte, bool, error) {
	resp, err := a.get(ctx, Request{
		Path:    "repos/" + repo + "/releases/assets/" + strconv.FormatInt(id, 10),
		Headers: map[string]string{"Accept": "application/octet-stream"},
	})
	if err != nil || resp.Status == 404 {
		return nil, false, err
	}
	return resp.Body, true, nil
}
