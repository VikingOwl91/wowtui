//go:build linux && (amd64 || arm64)

package seccomp

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// Offsets in the kernel UAPI's struct seccomp_data. Both supported ABIs
// are little-endian, with 64-bit syscall arguments.
const (
	number = 0
	arch   = 4
	arg0   = 16
	load   = unix.BPF_LD | unix.BPF_W | unix.BPF_ABS
	equal  = unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K
	ret    = unix.BPF_RET | unix.BPF_K
	deny   = unix.SECCOMP_RET_KILL_PROCESS
)

// Profile returns a native sock_filter array in the raw format of bwrap --seccomp.
func Profile() ([]byte, error) {
	const thread = unix.CLONE_VM | unix.CLONE_FS | unix.CLONE_FILES | unix.CLONE_SIGHAND | unix.CLONE_SYSVSEM | unix.CLONE_THREAD
	const optional = unix.CLONE_SETTLS | unix.CLONE_PARENT_SETTID | unix.CLONE_CHILD_CLEARTID | unix.CLONE_CHILD_SETTID
	cloneRule := []unix.SockFilter{
		{Code: load, K: arg0 + 4},
		{Code: equal, K: 0, Jt: 1},
		{Code: ret, K: deny},
		{Code: load, K: arg0},
		{Code: unix.BPF_ALU | unix.BPF_AND | unix.BPF_K, K: ^uint32(optional)},
		{Code: equal, K: thread, Jf: 1},
		{Code: ret, K: unix.SECCOMP_RET_ALLOW},
		{Code: ret, K: deny},
	}
	filter := []unix.SockFilter{
		{Code: load, K: arch},
		{Code: equal, K: auditArch, Jt: 1},
		{Code: ret, K: unix.SECCOMP_RET_KILL_PROCESS},
		{Code: load, K: number},
		// clone3 passes an indirect argument structure. ENOSYS tells libc
		// to use the checkable legacy thread-creation call instead.
		{Code: equal, K: unix.SYS_CLONE3, Jf: 1},
		{Code: ret, K: unix.SECCOMP_RET_ERRNO | uint32(unix.ENOSYS)},
		{Code: equal, K: unix.SYS_CLONE, Jf: uint8(len(cloneRule))},
	}
	filter = append(filter, cloneRule...)
	// Go optionally labels anonymous VMAs. Terminal detection in the Go UI
	// dependencies queries TCGETS at package initialization, even in worker
	// mode. The worker uses pipes and does not require either feature.
	for _, probe := range []struct{ nr, offset, request, errno uint32 }{
		{unix.SYS_PRCTL, arg0, unix.PR_SET_VMA, uint32(unix.EINVAL)},
		{unix.SYS_IOCTL, arg0 + 8, unix.TCGETS, uint32(unix.ENOTTY)},
	} {
		filter = append(filter,
			unix.SockFilter{Code: equal, K: probe.nr, Jf: 4},
			unix.SockFilter{Code: load, K: probe.offset},
			unix.SockFilter{Code: equal, K: probe.request, Jf: 1},
			unix.SockFilter{Code: ret, K: unix.SECCOMP_RET_ERRNO | probe.errno},
			unix.SockFilter{Code: ret, K: deny})
	}
	allow := []uint32{
		// bwrap execs the worker after installing the filter. Its PID 1
		// reaper also runs under this profile and uses wait4.
		unix.SYS_EXECVE, unix.SYS_WAIT4, unix.SYS_EXIT, unix.SYS_EXIT_GROUP,

		// Executable loader, Go heap/stacks, pthread startup and scheduling.
		unix.SYS_MMAP, unix.SYS_MUNMAP, unix.SYS_MPROTECT, unix.SYS_MADVISE,
		unix.SYS_BRK, unix.SYS_SET_TID_ADDRESS, unix.SYS_SET_ROBUST_LIST,
		unix.SYS_RSEQ, unix.SYS_GETPID, unix.SYS_GETTID, unix.SYS_FUTEX,
		unix.SYS_SCHED_GETAFFINITY, unix.SYS_SCHED_YIELD, unix.SYS_GETRLIMIT,
		unix.SYS_PRLIMIT64, unix.SYS_GETRANDOM,
		unix.SYS_RT_SIGACTION, unix.SYS_RT_SIGPROCMASK, unix.SYS_RT_SIGRETURN,
		unix.SYS_SIGALTSTACK, unix.SYS_TGKILL, unix.SYS_CLOCK_GETTIME,
		unix.SYS_NANOSLEEP, unix.SYS_CLOCK_NANOSLEEP,

		// Go poller, worker pipes, archive reads and staged files.
		unix.SYS_EPOLL_CREATE1, unix.SYS_EPOLL_CTL, unix.SYS_EPOLL_PWAIT,
		unix.SYS_EVENTFD2, unix.SYS_FCNTL, unix.SYS_READ, unix.SYS_PREAD64,
		unix.SYS_WRITE, unix.SYS_CLOSE, unix.SYS_OPENAT, unix.SYS_MKDIRAT,
		unix.SYS_FSTAT, unix.SYS_NEWFSTATAT, unix.SYS_STATX, unix.SYS_LSEEK,
		unix.SYS_GETDENTS64, unix.SYS_READLINKAT, unix.SYS_FACCESSAT,
		unix.SYS_FACCESSAT2,
	}
	for _, nr := range append(allow, abiSyscalls...) {
		filter = append(filter, unix.SockFilter{Code: equal, K: nr, Jf: 1},
			unix.SockFilter{Code: ret, K: unix.SECCOMP_RET_ALLOW})
	}
	// Equality uses the full syscall number. On amd64 this also excludes
	// the x32 syscall namespace, which shares AUDIT_ARCH_X86_64.
	filter = append(filter, unix.SockFilter{Code: ret, K: deny})
	return binary.Append(nil, binary.LittleEndian, filter)
}
