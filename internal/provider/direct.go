package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type Direct struct {
	client *http.Client
}

func NewDirect() *Direct {
	return &Direct{client: &http.Client{Timeout: 60 * time.Second}}
}

func (d *Direct) Resolve(ctx context.Context, ref Reference) (RemoteAddon, error) {
	status, err := d.probe(ctx, ref.ID)
	if err != nil {
		return RemoteAddon{}, err
	}
	if status >= 400 {
		return RemoteAddon{}, fmt.Errorf("direct URL returned %s", http.StatusText(status))
	}
	return RemoteAddon{Provider: "direct", ID: ref.ID, Name: directName(ref.ID)}, nil
}

func (d *Direct) Latest(ctx context.Context, addon RemoteAddon, _ wow.GameFlavor, _ string) (Release, error) {
	etag, lastModified, status, err := d.headers(ctx, addon.ID)
	if err == nil && status >= 200 && status < 300 {
		if etag != "" {
			return directRelease(addon.ID, etag), nil
		}
		if lastModified != "" {
			return directRelease(addon.ID, lastModified), nil
		}
	}

	body, err := d.Download(ctx, Release{DownloadURL: addon.ID})
	if err != nil {
		return Release{}, err
	}
	defer body.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, body); err != nil {
		return Release{}, err
	}
	return directRelease(addon.ID, hex.EncodeToString(hash.Sum(nil))), nil
}

func (d *Direct) Download(ctx context.Context, release Release) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download of %s returned %s", release.AssetName, resp.Status)
	}
	return resp.Body, nil
}

func (d *Direct) probe(ctx context.Context, rawURL string) (int, error) {
	status, err := d.head(ctx, rawURL)
	if err != nil {
		return 0, err
	}
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return d.getStatus(ctx, rawURL)
	}
	return status, nil
}

func (d *Direct) headers(ctx context.Context, rawURL string) (string, string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := d.client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	etag := strings.TrimPrefix(resp.Header.Get("ETag"), "W/")
	etag = strings.Trim(etag, `"`)
	return etag, resp.Header.Get("Last-Modified"), resp.StatusCode, nil
}

func (d *Direct) head(ctx context.Context, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func (d *Direct) getStatus(ctx context.Context, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "wowtui")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func directRelease(downloadURL, version string) Release {
	return Release{
		Kind:        KindDirect,
		Tag:         version,
		AssetName:   directName(downloadURL) + ".zip",
		DownloadURL: downloadURL,
	}
}

func directName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	base := path.Base(parsed.Path)
	if base == "" || base == "." || base == "/" {
		return rawURL
	}
	if ext := path.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}
