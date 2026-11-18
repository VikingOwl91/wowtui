package worker

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

// Validate checks the worker's actual output after the process has exited.
// WalkDir does not follow links, including a link used as the root.
func Validate(staging string, limits installer.Limits) error {
	if !validLimits(limits) {
		return errors.New("staging validation requires positive limits")
	}
	count := 0
	var total int64
	return filepath.WalkDir(staging, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if path == staging || !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected staging entry %q (%s)", path, entry.Type())
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
			return fmt.Errorf("unexpected staging file mode %s for %q", info.Mode(), path)
		}
		if err := checkSingleLink(path, info); err != nil {
			return err
		}
		count++
		if count > limits.MaxFiles {
			return errors.New("staging exceeds maximum file count")
		}
		if info.Size() > limits.MaxFileSize {
			return errors.New("staging exceeds maximum file size")
		}
		if info.Size() > limits.MaxTotalSize-total {
			return errors.New("staging exceeds maximum total size")
		}
		total += info.Size()
		return nil
	})
}
