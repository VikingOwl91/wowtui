package seccomp

import "golang.org/x/sys/unix"

const auditArch = unix.AUDIT_ARCH_X86_64

// amd64 has legacy loader, file and epoll entry points in addition to *at.
var abiSyscalls = []uint32{
	unix.SYS_ARCH_PRCTL, unix.SYS_ACCESS, unix.SYS_READLINK, unix.SYS_OPEN,
	unix.SYS_STAT, unix.SYS_LSTAT, unix.SYS_MKDIR, unix.SYS_EPOLL_WAIT,
}
