package seccomp

import "golang.org/x/sys/unix"

const auditArch = unix.AUDIT_ARCH_AARCH64

// arm64 uses the generic Linux file and epoll entry points.
var abiSyscalls []uint32
