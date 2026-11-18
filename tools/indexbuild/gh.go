package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// Request is one GitHub REST GET. Path has no leading slash and no query;
// Params become query parameters.
type Request struct {
	Path    string            `json:"path"`
	Params  map[string]string `json:"params,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Response is the HTTP answer. Header keys are lower case and only hold the
// rate limit headers the client reads.
type Response struct {
	Status int
	Header map[string]string
	Body   []byte
}

// GH performs GitHub GET requests. The only production implementation shells
// out to `gh api`, so authentication stays inside gh and this program never
// sees a token.
type GH interface {
	Get(ctx context.Context, req Request) (Response, error)
}

type execGH struct {
	run func(ctx context.Context, args []string) (stdout, stderr []byte, err error)
}

func newExecGH() *execGH {
	return &execGH{run: func(ctx context.Context, args []string) ([]byte, []byte, error) {
		cmd := exec.CommandContext(ctx, "gh", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.Bytes(), stderr.Bytes(), err
	}}
}

func ghArgs(req Request) []string {
	args := []string{"api", "-i", "-X", "GET"}
	for _, key := range sortedKeys(req.Headers) {
		args = append(args, "-H", key+": "+req.Headers[key])
	}
	for _, key := range sortedKeys(req.Params) {
		args = append(args, "-f", key+"="+req.Params[key])
	}
	return append(args, req.Path)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// Get runs `gh api -i`. gh exits non-zero for HTTP errors but still prints
// the response, so stdout decides; only an unparsable stdout is an error.
func (g *execGH) Get(ctx context.Context, req Request) (Response, error) {
	stdout, stderr, runErr := g.run(ctx, ghArgs(req))
	resp, parseErr := parseGHOutput(stdout)
	if parseErr == nil {
		return resp, nil
	}
	if ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	msg := strings.TrimSpace(string(stderr))
	if msg == "" && runErr != nil {
		msg = runErr.Error()
	}
	if msg == "" {
		msg = parseErr.Error()
	}
	return Response{}, fmt.Errorf("gh api %s: %s", req.Path, msg)
}

func parseGHOutput(out []byte) (Response, error) {
	head, body, found := splitHead(out)
	if !found {
		return Response{}, fmt.Errorf("no HTTP response in gh output")
	}
	lines := strings.Split(strings.ReplaceAll(string(head), "\r\n", "\n"), "\n")
	fields := strings.Fields(lines[0])
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		return Response{}, fmt.Errorf("unexpected gh status line %q", lines[0])
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil {
		return Response{}, fmt.Errorf("unexpected gh status line %q", lines[0])
	}
	resp := Response{Status: status, Header: map[string]string{}, Body: body}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "retry-after", "x-ratelimit-remaining", "x-ratelimit-reset":
			resp.Header[key] = strings.TrimSpace(value)
		}
	}
	return resp, nil
}

func splitHead(out []byte) (head, body []byte, found bool) {
	crlf := bytes.Index(out, []byte("\r\n\r\n"))
	lf := bytes.Index(out, []byte("\n\n"))
	switch {
	case crlf >= 0 && (lf < 0 || crlf < lf):
		return out[:crlf], out[crlf+4:], true
	case lf >= 0:
		return out[:lf], out[lf+2:], true
	}
	if bytes.HasPrefix(out, []byte("HTTP/")) {
		return out, nil, true
	}
	return nil, nil, false
}
