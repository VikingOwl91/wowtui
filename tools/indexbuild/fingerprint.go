package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
)

func cmdFingerprint(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	archive := flags.String("zip", "", "upstream ZIP with top-level addon folders")
	version := flags.String("version", "", "release version")
	out := flags.String("out", "", "release manifest JSON to write")
	if ok, help := parseFlags(flags, args, stderr, map[string]*string{"zip": archive, "version": version, "out": out}); !ok {
		return exitFor(help)
	}
	release, err := manifestFromZIP(*archive, *version)
	if err == nil {
		err = writeJSONFile(*out, release)
	}
	if err != nil {
		fmt.Fprintf(stderr, "indexbuild fingerprint: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %d file hashes for %s\n", len(release.Files), release.Version)
	return 0
}

func manifestFromZIP(name, version string) (fingerprint.Release, error) {
	const (
		maxBytes     = 512 << 20
		maxFileBytes = 256 << 20
		maxEntries   = 20000
	)
	var release fingerprint.Release
	f, err := os.Open(name)
	if err != nil {
		return release, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return release, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return release, fmt.Errorf("expected a regular ZIP of at most %d bytes", maxBytes)
	}
	// archive/zip reads every central-directory record in NewReader, before
	// len(z.File) can be checked. Bound that metadata read separately.
	reader := &zipDirectoryReader{ReaderAt: f, remaining: 8 << 20}
	z, err := zip.NewReader(reader, info.Size())
	if err != nil {
		return release, err
	}
	reader.remaining = -1
	if len(z.File) > maxEntries {
		return release, fmt.Errorf("ZIP exceeds %d entries", maxEntries)
	}
	release.Version = version
	var total uint64
	for _, entry := range z.File {
		path := strings.TrimSuffix(entry.Name, "/")
		if !fs.ValidPath(path) || path == "." || strings.ContainsAny(path, "\\:\x00") {
			return release, fmt.Errorf("invalid ZIP path %q", entry.Name)
		}
		switch entry.Mode().Type() {
		case fs.ModeDir:
			continue
		case 0:
		default:
			return release, fmt.Errorf("non-regular ZIP entry %q", entry.Name)
		}
		size := entry.UncompressedSize64
		if size > maxFileBytes || size > maxBytes-total {
			return release, fmt.Errorf("uncompressed ZIP size exceeds limits at %q", entry.Name)
		}
		total += size
		hash, err := zipEntryHash(entry)
		if err != nil {
			return release, fmt.Errorf("hash %q: %w", entry.Name, err)
		}
		release.Files = append(release.Files, fingerprint.File{Path: entry.Name, SHA256: hash})
	}
	slices.SortFunc(release.Files, func(a, b fingerprint.File) int { return strings.Compare(a.Path, b.Path) })
	return release, release.Validate()
}

type zipDirectoryReader struct {
	io.ReaderAt
	remaining int
}

func (r *zipDirectoryReader) ReadAt(p []byte, off int64) (int, error) {
	if r.remaining >= 0 {
		if len(p) > r.remaining {
			return 0, errors.New("ZIP directory metadata exceeds limit")
		}
		r.remaining -= len(p)
	}
	return r.ReaderAt.ReadAt(p, off)
}

func zipEntryHash(entry *zip.File) (string, error) {
	body, err := entry.Open()
	if err != nil {
		return "", err
	}
	defer body.Close()
	h := sha256.New()
	size := int64(entry.UncompressedSize64)
	n, err := io.Copy(h, io.LimitReader(body, size+1))
	if err != nil {
		return "", err
	}
	if n != size {
		return "", fmt.Errorf("read %d bytes, expected %d", n, size)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
