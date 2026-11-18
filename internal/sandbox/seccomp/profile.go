// Package seccomp builds the syscall filter passed to Bubblewrap before the
// extraction worker starts. Filesystem and network isolation belong to bwrap.
package seccomp

import "errors"

// ErrUnsupported means there is no worker policy for this operating system/ABI.
var ErrUnsupported = errors.New("no seccomp profile for this platform")
