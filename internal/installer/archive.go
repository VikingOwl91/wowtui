package installer

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Limits struct {
	MaxFiles     int
	MaxTotalSize int64
	MaxFileSize  int64
}

func ExtractZip(zipPath, dest string, limits Limits) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	dest = filepath.Clean(dest)
	var totalSize int64
	fileCount := 0

	for _, file := range reader.File {
		if err := validateEntryPath(file.Name, dest); err != nil {
			return err
		}
		target := filepath.Join(dest, file.Name)
		mode := file.Mode()

		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode.IsRegular():
			fileCount++
			if limits.MaxFiles > 0 && fileCount > limits.MaxFiles {
				return fmt.Errorf("archive exceeds maximum file count of %d", limits.MaxFiles)
			}
			if err := extractFile(file, target, limits, &totalSize); err != nil {
				return err
			}
		default:
			return fmt.Errorf("archive entry %q has unsupported file type %v", file.Name, mode.Type())
		}
	}
	return nil
}

func validateEntryPath(name, dest string) error {
	if name == "" {
		return fmt.Errorf("archive entry has empty name")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("archive entry %q has absolute path", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("archive entry %q escapes destination", name)
		}
	}
	target := filepath.Join(dest, name)
	if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
		return fmt.Errorf("archive entry %q escapes destination", name)
	}
	return nil
}

func extractFile(file *zip.File, target string, limits Limits, totalSize *int64) error {
	if limits.MaxFileSize > 0 && file.UncompressedSize64 > uint64(limits.MaxFileSize) {
		return fmt.Errorf("archive entry %q exceeds maximum file size", file.Name)
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			written += int64(n)
			*totalSize += int64(n)
			if limits.MaxFileSize > 0 && written > limits.MaxFileSize {
				return fmt.Errorf("archive entry %q exceeds maximum file size", file.Name)
			}
			if limits.MaxTotalSize > 0 && *totalSize > limits.MaxTotalSize {
				return fmt.Errorf("archive exceeds maximum total size")
			}
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
