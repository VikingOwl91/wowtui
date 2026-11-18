package worker

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func checkSingleLink(path string, _ fs.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return fmt.Errorf("staging file %q must have exactly one link", path)
	}
	return nil
}
