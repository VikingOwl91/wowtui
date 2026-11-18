package index_test

import (
	"strings"
	"testing"
)

func TestSignedFingerprintFormat(t *testing.T) {
	file := map[string]any{"path": "Folder_a/Code.lua", "sha256": strings.Repeat("b", 64)}
	release := map[string]any{"version": "1.2", "files": []any{file}}
	entry := validEntry("a")
	entry["fingerprints"] = []any{release}
	doc := document(entry)
	doc["format"] = 2
	idx := mustLoad(t, encode(t, doc))
	first := idx.Entries()[0]
	first.Fingerprints[0].Files[0].Path = "changed"
	first.Fingerprints[0].Version = "changed"
	reread := idx.ByFolder("Folder_a")[0].Fingerprints[0]
	if reread.Version != "1.2" || reread.Files[0].Path != "Folder_a/Code.lua" {
		t.Fatal("lookup returned mutable catalog data")
	}

	for _, value := range []any{entry["fingerprints"], nil, []any{}} {
		doc["format"] = 1
		entry["fingerprints"] = value
		_, err := signedLoad(t, encode(t, doc))
		wantErrContaining(t, err, "fingerprints")
	}
	doc["format"] = 2
	entry["fingerprints"] = []any{release, release}
	_, err := signedLoad(t, encode(t, doc))
	wantErrContaining(t, err, "duplicate fingerprint version")
	entry["fingerprints"] = []any{release}
	file["path"] = "Other/Code.lua"
	_, err = signedLoad(t, encode(t, doc))
	wantErrContaining(t, err, "Other")
	file["path"] = "Folder_a/Code.lua"
	file["SHA256"] = file["sha256"]
	_, err = signedLoad(t, encode(t, doc))
	wantErrContaining(t, err, "unknown field")
}
