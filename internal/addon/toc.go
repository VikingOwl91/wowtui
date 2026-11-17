package addon

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

type Toc struct {
	Path   string
	Fields map[string]string
}

func ParseToc(path string) (*Toc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	toc := &Toc{Path: path, Fields: map[string]string{}}
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
		line = bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(line, []byte("##")) {
			continue
		}
		rest := strings.TrimLeft(strings.TrimPrefix(string(line), "##"), " \t")
		key, value, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		toc.Fields[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return toc, nil
}

func (t *Toc) Get(key string) string {
	return t.Fields[strings.ToLower(key)]
}

func (t *Toc) Title() string {
	return t.Get("Title")
}

func (t *Toc) Version() string {
	return t.Get("Version")
}

func (t *Toc) Interface() int {
	nums := t.Interfaces()
	if len(nums) == 0 {
		return 0
	}
	return nums[0]
}

func (t *Toc) Interfaces() []int {
	var nums []int
	for _, part := range strings.Split(t.Get("Interface"), ",") {
		num, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || num <= 0 {
			continue
		}
		nums = append(nums, num)
	}
	return nums
}
