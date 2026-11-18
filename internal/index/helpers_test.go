package index_test

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/index"
)

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func signedLoad(t *testing.T, data []byte) (*index.Index, error) {
	t.Helper()
	pub, priv := newKey(t)
	return index.Load(data, index.Sign(data, priv), pub)
}

func mustLoad(t *testing.T, data []byte) *index.Index {
	t.Helper()
	idx, err := signedLoad(t, data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return idx
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validEntry(id string) map[string]any {
	return map[string]any{
		"id":           id,
		"name":         "Addon " + id,
		"source":       "github:Owner/" + id,
		"folders":      []any{"Folder_" + id},
		"game_types":   []any{"mainline"},
		"release_json": true,
		"reviewed":     true,
	}
}

func document(entries ...map[string]any) map[string]any {
	list := make([]any, 0, len(entries))
	for _, entry := range entries {
		list = append(list, entry)
	}
	return map[string]any{
		"format":       1,
		"generated_at": "2026-11-18T10:00:00Z",
		"entries":      list,
	}
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func wantErrContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want one containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}
