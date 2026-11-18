//go:build !linux || (!amd64 && !arm64)

package seccomp

func Profile() ([]byte, error) { return nil, ErrUnsupported }
