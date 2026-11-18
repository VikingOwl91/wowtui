package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultSearchInterval = 7 * time.Second
	defaultMaxRetries     = 6
	maxRateLimitWait      = 15 * time.Minute
	serverErrorAttempts   = 4
	serverErrorWait       = 10 * time.Second
)

// Clock lets tests control time.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type ClientConfig struct {
	CacheDir       string
	Clock          Clock
	SearchInterval time.Duration
	MaxRetries     int
	// Logf receives progress notes such as rate limit waits. Optional.
	Logf func(format string, args ...any)
}

// Stats counts answers served from the network and from the cache.
type Stats struct {
	Live   int
	Cached int
}

// Client wraps a GH with an on-disk cache keyed by request (misses and 404s
// included), a spacing rule for code search and waits for rate limits.
type Client struct {
	gh  GH
	cfg ClientConfig

	mu         sync.Mutex
	lastSearch time.Time
	stats      Stats
	dataTime   time.Time
}

func NewClient(gh GH, cfg ClientConfig) *Client {
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.SearchInterval == 0 {
		cfg.SearchInterval = defaultSearchInterval
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	return &Client{gh: gh, cfg: cfg}
}

func (c *Client) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// DataTime is the newest fetch time of any answer this client returned.
func (c *Client) DataTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dataTime
}

type cacheEntry struct {
	Request    Request   `json:"request"`
	Status     int       `json:"status"`
	FetchedAt  time.Time `json:"fetched_at"`
	Body       string    `json:"body,omitempty"`
	BodyBase64 string    `json:"body_base64,omitempty"`
}

func cacheKey(req Request) string {
	var b strings.Builder
	b.WriteString("GET ")
	b.WriteString(req.Path)
	for _, key := range sortedKeys(req.Params) {
		fmt.Fprintf(&b, "\nparam %q=%q", key, req.Params[key])
	}
	headers := map[string]string{}
	for key, value := range req.Headers {
		headers[strings.ToLower(key)] = value
	}
	for _, key := range sortedKeys(headers) {
		fmt.Fprintf(&b, "\nheader %q=%q", key, headers[key])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (c *Client) cachePath(req Request) string {
	return filepath.Join(c.cfg.CacheDir, cacheKey(req)+".json")
}

func (c *Client) note(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.After(c.dataTime) {
		c.dataTime = t
	}
}

func (c *Client) Get(ctx context.Context, req Request) (Response, error) {
	path := c.cachePath(req)
	if data, err := os.ReadFile(path); err == nil {
		var entry cacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return Response{}, fmt.Errorf("corrupt cache entry %s: %w", path, err)
		}
		body := []byte(entry.Body)
		if entry.BodyBase64 != "" {
			if body, err = base64.StdEncoding.DecodeString(entry.BodyBase64); err != nil {
				return Response{}, fmt.Errorf("corrupt cache entry %s: %w", path, err)
			}
		}
		c.mu.Lock()
		c.stats.Cached++
		c.mu.Unlock()
		c.note(entry.FetchedAt)
		return Response{Status: entry.Status, Body: body}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Response{}, err
	}

	resp, err := c.fetch(ctx, req)
	if err != nil {
		return Response{}, err
	}
	now := c.cfg.Clock.Now().UTC()
	entry := cacheEntry{Request: req, Status: resp.Status, FetchedAt: now}
	if utf8.Valid(resp.Body) {
		entry.Body = string(resp.Body)
	} else {
		entry.BodyBase64 = base64.StdEncoding.EncodeToString(resp.Body)
	}
	if err := writeCache(path, entry); err != nil {
		return Response{}, err
	}
	c.note(now)
	return resp, nil
}

func writeCache(path string, entry cacheEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func isCodeSearch(req Request) bool {
	return req.Path == "search/code"
}

// fetch performs one live request, waiting out rate limits and retrying
// server errors. Only answers worth caching are returned.
func (c *Client) fetch(ctx context.Context, req Request) (Response, error) {
	clock := c.cfg.Clock
	limited, failed := 0, 0
	for {
		if isCodeSearch(req) {
			c.mu.Lock()
			last := c.lastSearch
			c.mu.Unlock()
			if !last.IsZero() {
				if wait := c.cfg.SearchInterval - clock.Now().Sub(last); wait > 0 {
					if err := clock.Sleep(ctx, wait); err != nil {
						return Response{}, err
					}
				}
			}
		}
		resp, err := c.gh.Get(ctx, req)
		if isCodeSearch(req) {
			c.mu.Lock()
			c.lastSearch = clock.Now()
			c.mu.Unlock()
		}
		if err != nil {
			return Response{}, err
		}
		c.mu.Lock()
		c.stats.Live++
		c.mu.Unlock()

		switch {
		case rateLimited(resp):
			limited++
			if limited > c.cfg.MaxRetries {
				return Response{}, fmt.Errorf("%s: still rate limited after %d waits", req.Path, c.cfg.MaxRetries)
			}
			wait := rateLimitWait(resp, clock.Now())
			c.logf("rate limited on %s, waiting %s", req.Path, wait.Round(time.Second))
			if err := clock.Sleep(ctx, wait); err != nil {
				return Response{}, err
			}
		case resp.Status >= 500:
			failed++
			if failed >= serverErrorAttempts {
				return Response{}, fmt.Errorf("%s: server error %d after %d attempts", req.Path, resp.Status, failed)
			}
			c.logf("server error %d on %s, retrying", resp.Status, req.Path)
			if err := clock.Sleep(ctx, serverErrorWait); err != nil {
				return Response{}, err
			}
		default:
			return resp, nil
		}
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

func rateLimited(resp Response) bool {
	if resp.Status != 403 && resp.Status != 429 {
		return false
	}
	if _, ok := resp.Header["retry-after"]; ok {
		return true
	}
	if resp.Header["x-ratelimit-remaining"] == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(string(resp.Body)), "rate limit")
}

func rateLimitWait(resp Response, now time.Time) time.Duration {
	wait := time.Minute
	if v, ok := resp.Header["retry-after"]; ok {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			wait = time.Duration(secs+1) * time.Second
		}
	} else if v, ok := resp.Header["x-ratelimit-reset"]; ok {
		if reset, err := strconv.ParseInt(v, 10, 64); err == nil {
			wait = max(time.Unix(reset, 0).Sub(now)+2*time.Second, 5*time.Second)
		}
	}
	return min(wait, maxRateLimitWait)
}
