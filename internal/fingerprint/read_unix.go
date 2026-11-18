//go:build unix

package fingerprint

import "syscall"

// If a regular file is exchanged for a FIFO while opening, return to the
// descriptor type check instead of waiting for a writer.
const readFlags = syscall.O_RDONLY | syscall.O_NONBLOCK
