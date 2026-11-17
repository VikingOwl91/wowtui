package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

var dotEnvKeys = map[string]bool{
	"WOWTUI_GITHUB_TOKEN":       true,
	"WOWTUI_WAGO_API_KEY":       true,
	"WOWTUI_CURSEFORGE_API_KEY": true,
}

// LoadDotEnv loads credential variables from a dotenv file without replacing
// values that are already present in the process environment. Other keys in
// the file are ignored.
func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !dotEnvKeys[key] {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
