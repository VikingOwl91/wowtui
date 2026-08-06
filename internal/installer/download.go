package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func WriteArchive(reader io.Reader, destPath string, maxSize int64) (string, int64, error) {
	out, err := os.Create(destPath)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()

	hash := sha256.New()
	source := io.Reader(reader)
	if maxSize > 0 {
		source = io.LimitReader(reader, maxSize+1)
	}
	written, err := io.Copy(out, io.TeeReader(source, hash))
	if err != nil {
		return "", 0, err
	}
	if maxSize > 0 && written > maxSize {
		return "", written, fmt.Errorf("download exceeds maximum size of %d bytes", maxSize)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}
