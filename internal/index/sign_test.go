package index_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestRoundTripGenerateSignLoad(t *testing.T) {
	data := fixture(t, "index.json")
	pub, priv := newKey(t)

	idx, err := index.Load(data, index.Sign(data, priv), pub)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if want := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC); !idx.GeneratedAt().Equal(want) {
		t.Errorf("GeneratedAt = %v, want %v", idx.GeneratedAt(), want)
	}
	entries := idx.Entries()
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	want := index.Entry{
		ID:          "bossmods",
		Name:        "Boss Mods",
		Source:      "github:ExampleOrg/BossMods",
		Folders:     []string{"BossMods", "BossMods_Core", "BossMods_Options"},
		GameTypes:   []wow.GameType{wow.GameMainline, wow.GameMists, wow.GameVanilla},
		IDs:         index.ProviderIDs{CurseForge: "12345", Wago: "aB3dE5fG", WoWInterface: "6789"},
		ReleaseJSON: true,
		Funding: []index.Funding{
			{Platform: "github", Handle: "exampleorg"},
			{Platform: "ko_fi", Handle: "example"},
		},
		Reviewed: true,
	}
	if !reflect.DeepEqual(entries[0], want) {
		t.Errorf("entries[0] = %+v\nwant %+v", entries[0], want)
	}
	if got := []string{entries[0].ID, entries[1].ID, entries[2].ID}; !reflect.DeepEqual(got, []string{"bossmods", "bossmods-classic-extras", "questhelper"}) {
		t.Errorf("entry order = %v", got)
	}
}

func TestSignOutputIsBase64WithTrailingNewline(t *testing.T) {
	_, priv := newKey(t)
	sig := index.Sign([]byte("payload"), priv)

	text, ok := bytes.CutSuffix(sig, []byte("\n"))
	if !ok {
		t.Fatalf("signature %q has no trailing newline", sig)
	}
	raw, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		t.Fatalf("signature is not standard base64: %v", err)
	}
	if len(raw) != ed25519.SignatureSize {
		t.Fatalf("signature has %d bytes, want %d", len(raw), ed25519.SignatureSize)
	}
	if !ed25519.Verify(priv.Public().(ed25519.PublicKey), []byte("payload"), raw) {
		t.Fatal("signature does not verify with the standard library")
	}
}

func TestLoadAcceptsSignatureWithoutTrailingNewline(t *testing.T) {
	data := fixture(t, "index.json")
	pub, priv := newKey(t)
	sig := bytes.TrimSuffix(index.Sign(data, priv), []byte("\n"))

	if _, err := index.Load(data, sig, pub); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoadRejectsTamperedData(t *testing.T) {
	data := fixture(t, "index.json")
	pub, priv := newKey(t)
	sig := index.Sign(data, priv)

	tampered := bytes.Replace(data, []byte(`"reviewed": false`), []byte(`"reviewed": true `), 1)
	if bytes.Equal(tampered, data) {
		t.Fatal("fixture did not change")
	}
	idx, err := index.Load(tampered, sig, pub)
	if !errors.Is(err, index.ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
	if idx != nil {
		t.Fatal("got an index for tampered data")
	}

	trailing := append(append([]byte{}, data...), '\n')
	if _, err := index.Load(trailing, sig, pub); !errors.Is(err, index.ErrInvalidSignature) {
		t.Fatalf("appended newline: err = %v, want ErrInvalidSignature", err)
	}
}

func TestLoadRejectsWrongKey(t *testing.T) {
	data := fixture(t, "index.json")
	_, priv := newKey(t)
	otherPub, _ := newKey(t)

	idx, err := index.Load(data, index.Sign(data, priv), otherPub)
	if !errors.Is(err, index.ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
	if idx != nil {
		t.Fatal("got an index for a foreign key")
	}
}

func TestLoadRejectsMalformedSignature(t *testing.T) {
	data := fixture(t, "index.json")
	pub, priv := newKey(t)
	good := strings.TrimSuffix(string(index.Sign(data, priv)), "\n")
	raw, err := base64.StdEncoding.DecodeString(good)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		sig  string
	}{
		{"empty", ""},
		{"only newline", "\n"},
		{"not base64", "not a signature!"},
		{"url encoding", base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 64))},
		{"raw bytes", string(raw)},
		{"too short", base64.StdEncoding.EncodeToString(raw[:63])},
		{"too long", base64.StdEncoding.EncodeToString(append(append([]byte{}, raw...), 0))},
		{"missing padding", strings.TrimRight(good, "=")},
		{"two trailing newlines", good + "\n\n"},
		{"crlf", good + "\r\n"},
		{"leading space", " " + good},
		{"trailing space", good + " "},
		{"embedded newline", good[:20] + "\n" + good[20:]},
		{"all zero", base64.StdEncoding.EncodeToString(make([]byte, 64))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := index.Load(data, []byte(tt.sig), pub)
			if !errors.Is(err, index.ErrInvalidSignature) {
				t.Fatalf("err = %v, want ErrInvalidSignature", err)
			}
			if idx != nil {
				t.Fatal("got an index for a malformed signature")
			}
		})
	}
}

func TestLoadRejectsInvalidPublicKey(t *testing.T) {
	data := fixture(t, "index.json")
	_, priv := newKey(t)
	sig := index.Sign(data, priv)

	for _, pub := range []ed25519.PublicKey{nil, make(ed25519.PublicKey, 31), make(ed25519.PublicKey, 64)} {
		idx, err := index.Load(data, sig, pub)
		if err == nil || idx != nil {
			t.Errorf("public key of %d bytes: got %v, %v; want an error", len(pub), idx, err)
		}
	}
}

func TestLoadVerifiesBeforeParsing(t *testing.T) {
	garbage := []byte("{not json")
	pub, priv := newKey(t)
	otherPub, _ := newKey(t)

	if _, err := index.Load(garbage, index.Sign(garbage, priv), otherPub); !errors.Is(err, index.ErrInvalidSignature) {
		t.Fatalf("unsigned garbage: err = %v, want ErrInvalidSignature", err)
	}
	_, err := index.Load(garbage, index.Sign(garbage, priv), pub)
	if err == nil || errors.Is(err, index.ErrInvalidSignature) {
		t.Fatalf("signed garbage: err = %v, want a parse error", err)
	}
}

func TestPublicKeyEncoding(t *testing.T) {
	pub, _ := newKey(t)
	text := index.EncodePublicKey(pub)

	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		t.Fatalf("EncodePublicKey = %q, want base64 of 32 bytes (%v)", text, err)
	}
	for _, in := range []string{text, text + "\n"} {
		got, err := index.DecodePublicKey(in)
		if err != nil {
			t.Fatalf("DecodePublicKey(%q): %v", in, err)
		}
		if !got.Equal(pub) {
			t.Fatalf("DecodePublicKey(%q) = %x, want %x", in, got, pub)
		}
	}
}

func TestPrivateKeyEncodingUsesSeed(t *testing.T) {
	pub, priv := newKey(t)
	text := index.EncodePrivateKey(priv)

	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil || !bytes.Equal(raw, priv.Seed()) {
		t.Fatalf("EncodePrivateKey = %q, want base64 of the 32-byte seed (%v)", text, err)
	}
	for _, in := range []string{text, text + "\n"} {
		got, err := index.DecodePrivateKey(in)
		if err != nil {
			t.Fatalf("DecodePrivateKey: %v", err)
		}
		if !got.Equal(priv) {
			t.Fatal("decoded private key differs")
		}
	}

	decoded, err := index.DecodePrivateKey(text)
	if err != nil {
		t.Fatal(err)
	}
	data := fixture(t, "index.json")
	if _, err := index.Load(data, index.Sign(data, decoded), pub); err != nil {
		t.Fatalf("signature from decoded key: %v", err)
	}
}

func TestDecodeKeysRejectMalformedInput(t *testing.T) {
	pub, priv := newKey(t)
	pubText := index.EncodePublicKey(pub)
	seedText := index.EncodePrivateKey(priv)
	fullKeyText := base64.StdEncoding.EncodeToString(priv)

	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"not base64", "not a key"},
		{"too short", base64.StdEncoding.EncodeToString(make([]byte, 31))},
		{"too long", base64.StdEncoding.EncodeToString(make([]byte, 33))},
		{"full 64-byte key", fullKeyText},
		{"url encoding", base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 32))},
		{"two trailing newlines", pubText + "\n\n"},
		{"embedded newline", pubText[:10] + "\n" + pubText[10:]},
		{"leading space", " " + seedText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if key, err := index.DecodePublicKey(tt.text); err == nil {
				t.Errorf("DecodePublicKey accepted %q: %x", tt.text, key)
			}
			if _, err := index.DecodePrivateKey(tt.text); err == nil {
				t.Errorf("DecodePrivateKey accepted %q", tt.text)
			}
		})
	}
}
