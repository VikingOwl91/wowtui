package provider

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func directZip(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	fw, err := w.Create("Standalone/Standalone.toc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("## Version: 1.0")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newDirectServer(t *testing.T, zipContent []byte, etag, lastModified string, methods *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if methods != nil {
			*methods = append(*methods, r.Method)
		}
		if r.URL.Path != "/addon.zip" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		if lastModified != "" {
			w.Header().Set("Last-Modified", lastModified)
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write(zipContent)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDirectResolve(t *testing.T) {
	srv := newDirectServer(t, directZip(t), "", "", nil)
	d := NewDirect()
	addon, err := d.Resolve(context.Background(), Reference{Provider: "direct", ID: srv.URL + "/addon.zip"})
	if err != nil {
		t.Fatal(err)
	}
	if addon.Provider != "direct" || addon.Name != "addon" {
		t.Errorf("got %+v", addon)
	}
}

func TestDirectResolveMissing(t *testing.T) {
	srv := newDirectServer(t, directZip(t), "", "", nil)
	d := NewDirect()
	if _, err := d.Resolve(context.Background(), Reference{Provider: "direct", ID: srv.URL + "/nope.zip"}); err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestDirectLatestUsesETagWithoutDownload(t *testing.T) {
	var methods []string
	srv := newDirectServer(t, directZip(t), `"v42"`, "", &methods)
	d := NewDirect()

	release, err := d.Latest(context.Background(), RemoteAddon{Provider: "direct", ID: srv.URL + "/addon.zip"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.Kind != KindDirect || release.Tag != "v42" {
		t.Errorf("got %+v", release)
	}
	for _, m := range methods {
		if m == http.MethodGet {
			t.Errorf("expected no GET when ETag present, methods = %v", methods)
		}
	}
}

func TestDirectLatestUsesLastModified(t *testing.T) {
	srv := newDirectServer(t, directZip(t), "", "Wed, 06 Aug 2026 12:00:00 GMT", nil)
	d := NewDirect()
	release, err := d.Latest(context.Background(), RemoteAddon{Provider: "direct", ID: srv.URL + "/addon.zip"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}
	if release.Tag != "Wed, 06 Aug 2026 12:00:00 GMT" {
		t.Errorf("got %+v", release)
	}
}

func TestDirectLatestFallsBackToSHA(t *testing.T) {
	content := directZip(t)
	srv := newDirectServer(t, content, "", "", nil)
	d := NewDirect()

	release, err := d.Latest(context.Background(), RemoteAddon{Provider: "direct", ID: srv.URL + "/addon.zip"}, wow.Retail, "")
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	if release.Tag != hex.EncodeToString(sum[:]) {
		t.Errorf("tag = %q, want sha256 of content", release.Tag)
	}
	if release.Kind != KindDirect {
		t.Errorf("kind = %q", release.Kind)
	}
}

func TestDirectDownload(t *testing.T) {
	content := directZip(t)
	srv := newDirectServer(t, content, "", "", nil)
	d := NewDirect()

	body, err := d.Download(context.Background(), Release{DownloadURL: srv.URL + "/addon.zip", AssetName: "addon.zip"})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	buf := &bytes.Buffer{}
	if _, err := buf.ReadFrom(body); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Error("downloaded content mismatch")
	}
}

func TestDirectImplementsProvider(t *testing.T) {
	var _ Provider = (*Direct)(nil)
}
