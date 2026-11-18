//go:build unix

package worker

import (
	"fmt"
	"io/fs"
	"syscall"
)

func checkSingleLink(path string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return fmt.Errorf("staging file %q must have exactly one link", path)
	}
	return nil
}
