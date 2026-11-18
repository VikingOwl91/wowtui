//go:build !unix && !windows

package worker

import (
	"errors"
	"io/fs"
)

func checkSingleLink(_ string, _ fs.FileInfo) error {
	return errors.New("staging link validation is unavailable on this platform")
}
