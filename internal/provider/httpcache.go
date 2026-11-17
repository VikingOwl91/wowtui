package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

const (
	maxCachedBody = 4 << 20
	// A cached body is stored base64 encoded inside a small JSON envelope.
	maxCacheEntrySize = 2 * maxCachedBody
)

// cachedResponse is what httpCache keeps per request URL. Request headers,
// and with them the token, are never part of it.
type cachedResponse struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Body         []byte `json:"body"`
}

// httpCache keeps validated 200 responses on disk so later requests can be
// conditional. A nil cache loads and stores nothing.
type httpCache struct {
	dir string
}

func newHTTPCache(dir string) *httpCache {
	if dir == "" {
		return nil
	}
	return &httpCache{dir: dir}
}

func (c *httpCache) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".json")
}

func (c *httpCache) load(url string) (cachedResponse, bool) {
	if c == nil {
		return cachedResponse{}, false
	}
	f, err := os.Open(c.path(url))
	if err != nil {
		return cachedResponse{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCacheEntrySize+1))
	if err != nil || len(data) > maxCacheEntrySize {
		return cachedResponse{}, false
	}
	var entry cachedResponse
	if err := json.Unmarshal(data, &entry); err != nil {
		return cachedResponse{}, false
	}
	if entry.URL != url || (entry.ETag == "" && entry.LastModified == "") || !json.Valid(entry.Body) {
		return cachedResponse{}, false
	}
	return entry, true
}

func (c *httpCache) store(entry cachedResponse) error {
	if c == nil || len(entry.Body) > maxCachedBody || (entry.ETag == "" && entry.LastModified == "") {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), c.path(entry.URL)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
