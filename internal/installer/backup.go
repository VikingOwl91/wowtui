package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var unsafeLabelChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func Backup(addonsDir string, folderNames []string, backupRoot, label string) (string, error) {
	stamp := time.Now().Format("20060102-150405")
	base := filepath.Join(backupRoot, unsafeLabelChars.ReplaceAllString(label, "-")+"-"+stamp)
	dest := base
	for i := 1; dirExists(dest); i++ {
		dest = fmt.Sprintf("%s-%d", base, i)
	}

	for _, name := range folderNames {
		src := filepath.Join(addonsDir, name)
		if err := CopyDir(src, filepath.Join(dest, name)); err != nil {
			return "", err
		}
	}
	return dest, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
