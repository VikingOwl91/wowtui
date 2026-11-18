package index

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidSignature is returned by Load when the signature file is
// malformed or does not verify against the index bytes and the public key.
var ErrInvalidSignature = errors.New("index signature is invalid")

// GenerateKey returns a new ed25519 key pair for signing the index.
func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// EncodePublicKey returns the text form of pub: standard base64 of the 32
// key bytes.
func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

// DecodePublicKey parses the text form written by EncodePublicKey. One
// trailing newline is allowed.
func DecodePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := decodeBase64Line(s, ed25519.PublicKeySize)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	return ed25519.PublicKey(raw), nil
}

// EncodePrivateKey returns the text form of priv: standard base64 of its
// 32-byte RFC 8032 seed. The 64-byte expanded form is never written.
func EncodePrivateKey(priv ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(priv.Seed())
}

// DecodePrivateKey parses the seed text form written by EncodePrivateKey. One
// trailing newline is allowed.
func DecodePrivateKey(s string) (ed25519.PrivateKey, error) {
	seed, err := decodeBase64Line(s, ed25519.SeedSize)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// Sign returns the detached signature file content for the exact index bytes
// in data: standard base64 of the 64-byte ed25519 signature and a newline. It
// panics if priv is not a valid ed25519 private key, like ed25519.Sign.
func Sign(data []byte, priv ed25519.PrivateKey) []byte {
	sig := ed25519.Sign(priv, data)
	return []byte(base64.StdEncoding.EncodeToString(sig) + "\n")
}

func verify(data, sig []byte, pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("index public key has %d bytes, want %d", len(pub), ed25519.PublicKeySize)
	}
	raw, err := decodeBase64Line(string(sig), ed25519.SignatureSize)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}
	if !ed25519.Verify(pub, data, raw) {
		return ErrInvalidSignature
	}
	return nil
}

// decodeBase64Line decodes standard padded base64 with at most one trailing
// "\n". The encoding package skips CR and LF anywhere in its input, so line
// breaks are rejected here before decoding.
func decodeBase64Line(s string, size int) ([]byte, error) {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil, errors.New("empty")
	}
	if strings.ContainsAny(s, "\r\n") {
		return nil, errors.New("contains a line break")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("not standard base64: %w", err)
	}
	if len(raw) != size {
		return nil, fmt.Errorf("decodes to %d bytes, want %d", len(raw), size)
	}
	return raw, nil
}
