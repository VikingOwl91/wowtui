//go:build !unix

package worker

import (
	"errors"
	"runtime"
)

// CloseInherited relies on os/exec's explicit PROC_THREAD_ATTRIBUTE_HANDLE_LIST
// on Windows. The launcher supplies only the three protocol/diagnostic handles.
func CloseInherited() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return errors.New("worker descriptor isolation unavailable on this platform")
}
