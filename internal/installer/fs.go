package installer

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

func CopyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination %s already exists", dst)
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			dirInfo, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, dirInfo.Mode().Perm())
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported file type at %s", path)
		}
		return copyFile(path, target, d)
	})
}

func copyFile(src, dst string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func MoveDir(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && linkErr.Err == syscall.EXDEV {
		if err := CopyDir(src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}
	return err
}
