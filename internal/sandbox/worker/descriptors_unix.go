//go:build unix

package worker

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

// CloseInherited drops ambient descriptors in the private worker process.
// Descriptors opened by the Go runtime after exec are marked close-on-exec.
func CloseInherited() error {
	directory := "/dev/fd"
	if runtime.GOOS == "linux" {
		directory = "/proc/self/fd"
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		fd, err := strconv.Atoi(name)
		if err != nil || fd <= 2 {
			continue
		}
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if errors.Is(err, unix.EBADF) {
			continue // already closed since enumeration
		}
		if err != nil {
			return fmt.Errorf("inspect descriptor %d: %w", fd, err)
		}
		if flags&unix.FD_CLOEXEC == 0 {
			if err := unix.Close(fd); err != nil {
				return fmt.Errorf("close inherited descriptor %d: %w", fd, err)
			}
		}
	}
	return nil
}
