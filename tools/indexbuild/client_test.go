package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeGH struct {
	mu      sync.Mutex
	handler func(Request) Response
	calls   []Request
}

func (f *fakeGH) Get(_ context.Context, req Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	return f.handler(req), nil
}

func (f *fakeGH) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, req := range f.calls {
		if strings.HasPrefix(req.Path, prefix) {
			n++
		}
	}
	return n
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

func ok(body string) Response {
	return Response{Status: 200, Body: []byte(body)}
}

func newTestClient(t *testing.T, gh GH, clock Clock) *Client {
	t.Helper()
	return NewClient(gh, ClientConfig{CacheDir: t.TempDir(), Clock: clock})
}

func TestParseGHOutput(t *testing.T) {
	out := "HTTP/2.0 200 OK\r\nContent-Type: application/json\r\nX-Ratelimit-Remaining: 9\r\nRetry-After: 30\r\n\r\n{\"a\":1}"
	resp, err := parseGHOutput([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || string(resp.Body) != `{"a":1}` {
		t.Fatalf("got %+v", resp)
	}
	if resp.Header["x-ratelimit-remaining"] != "9" || resp.Header["retry-after"] != "30" {
		t.Fatalf("headers = %v", resp.Header)
	}

	nf, err := parseGHOutput([]byte("HTTP/2.0 404 Not Found\nContent-Type: application/json\n\n{\"message\":\"Not Found\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if nf.Status != 404 || !strings.Contains(string(nf.Body), "Not Found") {
		t.Fatalf("got %+v", nf)
	}

	for _, bad := range []string{"", "not http", "HTTP/2.0 abc\r\n\r\n"} {
		if _, err := parseGHOutput([]byte(bad)); err == nil {
			t.Errorf("parseGHOutput(%q) succeeded", bad)
		}
	}
}

func TestExecGHArgsAreGetOnlyAndCarryNoToken(t *testing.T) {
	var got [][]string
	gh := &execGH{run: func(_ context.Context, args []string) ([]byte, []byte, error) {
		got = append(got, args)
		return []byte("HTTP/2.0 200 OK\r\n\r\n{}"), nil, nil
	}}
	_, err := gh.Get(context.Background(), Request{
		Path:    "search/code",
		Params:  map[string]string{"q": "b", "per_page": "100"},
		Headers: map[string]string{"Accept": "application/octet-stream"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "api -i -X GET -H Accept: application/octet-stream -f per_page=100 -f q=b search/code"
	if line := strings.Join(got[0], " "); line != want {
		t.Fatalf("args = %q\nwant   %q", line, want)
	}
	for _, arg := range got[0] {
		if strings.Contains(strings.ToLower(arg), "token") || arg == "auth" {
			t.Fatalf("argument %q looks like a token or auth call", arg)
		}
	}
}

func TestExecGHUsesStdoutOfFailedCalls(t *testing.T) {
	gh := &execGH{run: func(context.Context, []string) ([]byte, []byte, error) {
		return []byte("HTTP/2.0 404 Not Found\r\n\r\n{\"message\":\"Not Found\"}"), []byte("gh: Not Found (HTTP 404)"), errors.New("exit status 1")
	}}
	resp, err := gh.Get(context.Background(), Request{Path: "repos/a/b"})
	if err != nil || resp.Status != 404 {
		t.Fatalf("got %+v, %v", resp, err)
	}

	gh = &execGH{run: func(context.Context, []string) ([]byte, []byte, error) {
		return nil, []byte("gh: error connecting to api.github.com"), errors.New("exit status 1")
	}}
	if _, err := gh.Get(context.Background(), Request{Path: "repos/a/b"}); err == nil || !strings.Contains(err.Error(), "error connecting") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientCachesHitsAndMisses(t *testing.T) {
	gh := &fakeGH{handler: func(req Request) Response {
		if req.Path == "repos/a/missing" {
			return Response{Status: 404, Body: []byte(`{"message":"Not Found"}`)}
		}
		return ok(`{"path":"` + req.Path + `"}`)
	}}
	dir := t.TempDir()
	cfg := ClientConfig{CacheDir: dir, Clock: newFakeClock()}
	c := NewClient(gh, cfg)
	for range 2 {
		for _, path := range []string{"repos/a/b", "repos/a/missing"} {
			if _, err := c.Get(context.Background(), Request{Path: path}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(gh.calls) != 2 {
		t.Fatalf("live calls = %d, want 2", len(gh.calls))
	}
	if s := c.Stats(); s.Live != 2 || s.Cached != 2 {
		t.Fatalf("stats = %+v", s)
	}

	// A new client on the same cache directory makes no call at all.
	c2 := NewClient(gh, cfg)
	resp, err := c2.Get(context.Background(), Request{Path: "repos/a/missing"})
	if err != nil || resp.Status != 404 {
		t.Fatalf("got %+v, %v", resp, err)
	}
	if _, err := c2.Get(context.Background(), Request{Path: "repos/a/b"}); err != nil {
		t.Fatal(err)
	}
	if len(gh.calls) != 2 {
		t.Fatalf("live calls after rerun = %d, want 2", len(gh.calls))
	}
}

func TestClientCacheKeyCoversParamsAndHeaders(t *testing.T) {
	gh := &fakeGH{handler: func(Request) Response { return ok(`{}`) }}
	c := newTestClient(t, gh, newFakeClock())
	reqs := []Request{
		{Path: "search/code", Params: map[string]string{"q": "a"}},
		{Path: "search/code", Params: map[string]string{"q": "b"}},
		{Path: "search/code", Params: map[string]string{"q": "a"}, Headers: map[string]string{"Accept": "x"}},
		{Path: "search/code", Params: map[string]string{"q": "a"}},
	}
	for _, req := range reqs {
		if _, err := c.Get(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if len(gh.calls) != 3 {
		t.Fatalf("live calls = %d, want 3", len(gh.calls))
	}
}

func TestClientThrottlesCodeSearch(t *testing.T) {
	gh := &fakeGH{handler: func(Request) Response { return ok(`{}`) }}
	clock := newFakeClock()
	c := newTestClient(t, gh, clock)
	ctx := context.Background()
	for _, q := range []string{"a", "b", "c"} {
		if _, err := c.Get(ctx, Request{Path: "search/code", Params: map[string]string{"q": q}}); err != nil {
			t.Fatal(err)
		}
		// Core calls in between are never throttled.
		if _, err := c.Get(ctx, Request{Path: "repos/x/" + q}); err != nil {
			t.Fatal(err)
		}
	}
	if len(clock.sleeps) != 2 {
		t.Fatalf("sleeps = %v, want 2 waits", clock.sleeps)
	}
	for _, d := range clock.sleeps {
		if d < 7*time.Second {
			t.Fatalf("code search wait %v is under 7s", d)
		}
	}

	// Time that already passed counts towards the interval.
	clock.now = clock.now.Add(20 * time.Second)
	before := len(clock.sleeps)
	if _, err := c.Get(ctx, Request{Path: "search/code", Params: map[string]string{"q": "d"}}); err != nil {
		t.Fatal(err)
	}
	if len(clock.sleeps) != before {
		t.Fatalf("waited %v although 20s passed", clock.sleeps[before:])
	}

	// Cached searches neither wait nor count as a call.
	before = len(clock.sleeps)
	if _, err := c.Get(ctx, Request{Path: "search/code", Params: map[string]string{"q": "a"}}); err != nil {
		t.Fatal(err)
	}
	if len(clock.sleeps) != before {
		t.Fatal("cached search waited")
	}
}

func TestClientWaitsOutRateLimits(t *testing.T) {
	calls := 0
	gh := &fakeGH{handler: func(Request) Response {
		calls++
		if calls == 1 {
			return Response{Status: 403, Header: map[string]string{"retry-after": "5"}, Body: []byte(`{"message":"secondary rate limit"}`)}
		}
		if calls == 2 {
			return Response{Status: 429, Body: []byte(`{"message":"API rate limit exceeded"}`)}
		}
		return ok(`{"done":true}`)
	}}
	clock := newFakeClock()
	c := newTestClient(t, gh, clock)
	resp, err := c.Get(context.Background(), Request{Path: "repos/a/b"})
	if err != nil || resp.Status != 200 {
		t.Fatalf("got %+v, %v", resp, err)
	}
	if len(clock.sleeps) != 2 || clock.sleeps[0] != 6*time.Second || clock.sleeps[1] < time.Second {
		t.Fatalf("sleeps = %v", clock.sleeps)
	}
	// The rate-limited answers were not cached.
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/b"}); err != nil || calls != 3 {
		t.Fatalf("calls = %d, err = %v", calls, err)
	}
}

func TestClientWaitsForRateLimitReset(t *testing.T) {
	clock := newFakeClock()
	reset := clock.now.Add(90 * time.Second).Unix()
	calls := 0
	gh := &fakeGH{handler: func(Request) Response {
		calls++
		if calls == 1 {
			return Response{Status: 403, Header: map[string]string{"x-ratelimit-remaining": "0", "x-ratelimit-reset": strconv.FormatInt(reset, 10)}, Body: []byte(`{}`)}
		}
		return ok(`{}`)
	}}
	c := newTestClient(t, gh, clock)
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/b"}); err != nil {
		t.Fatal(err)
	}
	if len(clock.sleeps) != 1 || clock.sleeps[0] < 90*time.Second || clock.sleeps[0] > 100*time.Second {
		t.Fatalf("sleeps = %v", clock.sleeps)
	}
}

func TestClientKeepsPlain403AndGivesUpOnEndlessLimits(t *testing.T) {
	gh := &fakeGH{handler: func(Request) Response {
		return Response{Status: 403, Body: []byte(`{"message":"Repository access blocked"}`)}
	}}
	c := newTestClient(t, gh, newFakeClock())
	resp, err := c.Get(context.Background(), Request{Path: "repos/a/b"})
	if err != nil || resp.Status != 403 {
		t.Fatalf("got %+v, %v", resp, err)
	}
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/b"}); err != nil || len(gh.calls) != 1 {
		t.Fatalf("plain 403 was not cached: calls = %d, err = %v", len(gh.calls), err)
	}

	limited := &fakeGH{handler: func(Request) Response {
		return Response{Status: 429, Header: map[string]string{"retry-after": "1"}}
	}}
	c = newTestClient(t, limited, newFakeClock())
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/c"}); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientRetriesServerErrorsThenFails(t *testing.T) {
	calls := 0
	gh := &fakeGH{handler: func(Request) Response {
		calls++
		if calls < 3 {
			return Response{Status: 502}
		}
		return ok(`{}`)
	}}
	c := newTestClient(t, gh, newFakeClock())
	if resp, err := c.Get(context.Background(), Request{Path: "repos/a/b"}); err != nil || resp.Status != 200 {
		t.Fatalf("got %+v, %v", resp, err)
	}
	always := &fakeGH{handler: func(Request) Response { return Response{Status: 503} }}
	c = newTestClient(t, always, newFakeClock())
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/b"}); err == nil {
		t.Fatal("persistent 503 succeeded")
	}
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/b"}); err == nil || len(always.calls) <= 3 {
		t.Fatalf("a failed call was cached: calls = %d", len(always.calls))
	}
}

func TestClientTracksNewestFetchTime(t *testing.T) {
	gh := &fakeGH{handler: func(Request) Response { return ok(`{}`) }}
	dir := t.TempDir()
	clock := newFakeClock()
	c := NewClient(gh, ClientConfig{CacheDir: dir, Clock: clock})
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/1"}); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Hour)
	if _, err := c.Get(context.Background(), Request{Path: "repos/a/2"}); err != nil {
		t.Fatal(err)
	}
	want := clock.now
	clock.now = clock.now.Add(24 * time.Hour)
	c2 := NewClient(gh, ClientConfig{CacheDir: dir, Clock: clock})
	for _, path := range []string{"repos/a/1", "repos/a/2"} {
		if _, err := c2.Get(context.Background(), Request{Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	if got := c2.DataTime(); !got.Equal(want) {
		t.Fatalf("DataTime = %v, want %v", got, want)
	}
}
