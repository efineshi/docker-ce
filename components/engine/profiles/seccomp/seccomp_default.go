// +build linux,seccomp

package seccomp // import "github.com/docker/docker/profiles/seccomp"

import (
	"github.com/docker/docker/api/types"
	"golang.org/x/sys/unix"
)

func arches() []types.Architecture {
	return []types.Architecture{
		{
			Arch:      types.ArchX86_64,
			SubArches: []types.Arch{types.ArchX86, types.ArchX32},
		},
		{
			Arch:      types.ArchAARCH64,
			SubArches: []types.Arch{types.ArchARM},
		},
		{
			Arch:      types.ArchMIPS64,
			SubArches: []types.Arch{types.ArchMIPS, types.ArchMIPS64N32},
		},
		{
			Arch:      types.ArchMIPS64N32,
			SubArches: []types.Arch{types.ArchMIPS, types.ArchMIPS64},
		},
		{
			Arch:      types.ArchMIPSEL64,
			SubArches: []types.Arch{types.ArchMIPSEL, types.ArchMIPSEL64N32},
		},
		{
			Arch:      types.ArchMIPSEL64N32,
			SubArches: []types.Arch{types.ArchMIPSEL, types.ArchMIPSEL64},
		},
		{
			Arch:      types.ArchS390X,
			SubArches: []types.Arch{types.ArchS390},
		},
	}
}

// DefaultProfile defines the whitelist for the default seccomp profile.
func DefaultProfile() *types.Seccomp {
	syscalls := []*types.Syscall{
		{
			Names: []string{
				"accept",
				"accept4",
				"access",
				"adjtimex",
				"alarm",
				"bind",
				"brk",
				"capget",
				"capset",
				"chdir",
				"chmod",
				"chown",
				"chown32",
				"clock_getres",
				"clock_gettime",
				"clock_nanosleep",
				"close",
				"connect",
				"copy_file_range",
				"creat",
				"dup",
				"dup2",
				"dup3",
				"epoll_create",
				"epoll_create1",
				"epoll_ctl",
				"epoll_ctl_old",
				"epoll_pwait",
				"epoll_wait",
				"epoll_wait_old",
				"eventfd",
				"eventfd2",
				"execve",
				"execveat",
				"exit",
				"exit_group",
				"faccessat",
				"fadvise64",
				"fadvise64_64",
				"fallocate",
				"fanotify_mark",
				"fchdir",
				"fchmod",
				"fchmodat",
				"fchown",
				"fchown32",
				"fchownat",
				"fcntl",
				"fcntl64",
				"fdatasync",
				"fgetxattr",
				"flistxattr",
				"flock",
				"fork",
				"fremovexattr",
				"fsetxattr",
				"fstat",
				"fstat64",
				"fstatat64",
				"fstatfs",
				"fstatfs64",
				"fsync",
				"ftruncate",
				"ftruncate64",
				"futex",
				"futimesat",
				"getcpu",
				"getcwd",
				"getdents",
				"getdents64",
				"getegid",
				"getegid32",
				"geteuid",
				"geteuid32",
				"getgid",
				"getgid32",
				"getgroups",
				"getgroups32",
				"getitimer",
				"getpeername",
				"getpgid",
				"getpgrp",
				"getpid",
				"getppid",
				"getpriority",
				"getrandom",
				"getresgid",
				"getresgid32",
				"getresuid",
				"getresuid32",
				"getrlimit",
				"get_robust_list",
				"getrusage",
				"getsid",
				"getsockname",
				"getsockopt",
				"get_thread_area",
				"gettid",
				"gettimeofday",
				"getuid",
				"getuid32",
				"getxattr",
				"inotify_add_watch",
				"inotify_init",
				"inotify_init1",
				"inotify_rm_watch",
				"io_cancel",
				"ioctl",
				"io_destroy",
				"io_getevents",
				"ioprio_get",
				"ioprio_set",
				"io_setup",
				"io_submit",
				"ipc",
				"kill",
				"lchown",
				"lchown32",
				"lgetxattr",
				"link",
				"linkat",
				"listen",
				"listxattr",
				"llistxattr",
				"_llseek",
				"lremovexattr",
				"lseek",
				"lsetxattr",
				"lstat",
				"lstat64",
				"madvise",
				"memfd_create",
				"mincore",
				"mkdir",
				"mkdirat",
				"mknod",
				"mknodat",
				"mlock",
				"mlock2",
				"mlockall",
				"mmap",
				"mmap2",
				"mprotect",
				"mq_getsetattr",
				"mq_notify",
				"mq_open",
				"mq_timedreceive",
				"mq_timedsend",
				"mq_unlink",
				"mremap",
				"msgctl",
				"msgget",
				"msgrcv",
				"msgsnd",
				"msync",
				"munlock",
				"munlockall",
				"munmap",
				"nanosleep",
				"newfstatat",
				"_newselect",
				"open",
				"openat",
				"pause",
				"pipe",
				"pipe2",
				"poll",
				"ppoll",
				"prctl",
				"pread64",
				"preadv",
				"preadv2",
				"prlimit64",
				"pselect6",
				"pwrite64",
				"pwritev",
				"pwritev2",
				"read",
				"readahead",
				"readlink",
				"readlinkat",
				"readv",
				"recv",
				"recvfrom",
				"recvmmsg",
				"recvmsg",
				"remap_file_pages",
				"removexattr",
				"rename",
				"renameat",
				"renameat2",
				"restart_syscall",
				"rmdir",
				"rt_sigaction",
				"rt_sigpending",
				"rt_sigprocmask",
				"rt_sigqueueinfo",
				"rt_sigreturn",
				"rt_sigsuspend",
				"rt_sigtimedwait",
				"rt_tgsigqueueinfo",
				"sched_getaffinity",
				"sched_getattr",
				"sched_getparam",
				"sched_get_priority_max",
				"sched_get_priority_min",
				"sched_getscheduler",
				"sched_rr_get_interval",
				"sched_setaffinity",
				"sched_setattr",
				"sched_setparam",
				"sched_setscheduler",
				"sched_yield",
				"seccomp",
				"select",
				"semctl",
				"semget",
				"semop",
				"semtimedop",
				"send",
				"sendfile",
				"sendfile64",
				"sendmmsg",
				"sendmsg",
				"sendto",
				"setfsgid",
				"setfsgid32",
				"setfsuid",
				"setfsuid32",
				"setgid",
				"setgid32",
				"setgroups",
				"setgroups32",
				"setitimer",
				"setpgid",
				"setpriority",
				"setregid",
				"setregid32",
				"setresgid",
				"setresgid32",
				"setresuid",
				"setresuid32",
				"setreuid",
				"setreuid32",
				"setrlimit",
				"set_robust_list",
				"setsid",
				"setsockopt",
				"set_thread_area",
				"set_tid_address",
				"setuid",
				"setuid32",
				"setxattr",
				"shmat",
				"shmctl",
				"shmdt",
				"shmget",
				"shutdown",
				"sigaltstack",
				"signalfd",
				"signalfd4",
				"sigprocmask",
				"sigreturn",
				"socket",
				"socketcall",
				"socketpair",
				"splice",
				"stat",
				"stat64",
				"statfs",
				"statfs64",
				"statx",
				"symlink",
				"symlinkat",
				"sync",
				"sync_file_range",
				"syncfs",
				"sysinfo",
				"tee",
				"tgkill",
				"time",
				"timer_create",
				"timer_delete",
				"timerfd_create",
				"timerfd_gettime",
				"timerfd_settime",
				"timer_getoverrun",
				"timer_gettime",
				"timer_settime",
				"times",
				"tkill",
				"truncate",
				"truncate64",
				"ugetrlimit",
				"umask",
				"uname",
				"unlink",
				"unlinkat",
				"utime",
				"utimensat",
				"utimes",
				"vfork",
				"vmsplice",
				"wait4",
				"waitid",
				"waitpid",
				"write",
				"writev",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
		},
		{
			Names:  []string{"personality"},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index: 0,
					Value: 0x0,
					Op:    types.OpEqualTo,
				},
			},
		},
		{
			Names:  []string{"personality"},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index: 0,
					Value: 0x0008,
					Op:    types.OpEqualTo,
				},
			},
		},
		{
			Names:  []string{"personality"},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index: 0,
					Value: 0x20000,
					Op:    types.OpEqualTo,
				},
			},
		},
		{
			Names:  []string{"personality"},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index: 0,
					Value: 0x20008,
					Op:    types.OpEqualTo,
				},
			},
		},
		{
			Names:  []string{"personality"},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index: 0,
					Value: 0xffffffff,
					Op:    types.OpEqualTo,
				},
			},
		},
		{
			Names: []string{
				"sync_file_range2",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Arches: []string{"ppc64le"},
			},
		},
		{
			Names: []string{
				"arm_fadvise64_64",
				"arm_sync_file_range",
				"sync_file_range2",
				"breakpoint",
				"cacheflush",
				"set_tls",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Arches: []string{"arm", "arm64"},
			},
		},
		{
			Names: []string{
				"arch_prctl",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Arches: []string{"amd64", "x32"},
			},
		},
		{
			Names: []string{
				"modify_ldt",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Arches: []string{"amd64", "x32", "x86"},
			},
		},
		{
			Names: []string{
				"s390_pci_mmio_read",
				"s390_pci_mmio_write",
				"s390_runtime_instr",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Arches: []string{"s390", "s390x"},
			},
		},
		{
			Names: []string{
				"open_by_handle_at",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_DAC_READ_SEARCH"},
			},
		},
		{
			Names: []string{
				"bpf",
				"clone",
				"fanotify_init",
				"lookup_dcookie",
				"mount",
				"name_to_handle_at",
				"perf_event_open",
				"quotactl",
				"setdomainname",
				"sethostname",
				"setns",
				"syslog",
				"umount",
				"umount2",
				"unshare",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_ADMIN"},
			},
		},
		{
			Names: []string{
				"clone",
			},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index:    0,
					Value:    unix.CLONE_NEWNS | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC | unix.CLONE_NEWUSER | unix.CLONE_NEWPID | unix.CLONE_NEWNET,
					ValueTwo: 0,
					Op:       types.OpMaskedEqual,
				},
			},
			Excludes: types.Filter{
				Caps:   []string{"CAP_SYS_ADMIN"},
				Arches: []string{"s390", "s390x"},
			},
		},
		{
			Names: []string{
				"clone",
			},
			Action: types.ActAllow,
			Args: []*types.Arg{
				{
					Index:    1,
					Value:    unix.CLONE_NEWNS | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC | unix.CLONE_NEWUSER | unix.CLONE_NEWPID | unix.CLONE_NEWNET,
					ValueTwo: 0,
					Op:       types.OpMaskedEqual,
				},
			},
			Comment: "s390 parameter ordering for clone is different",
			Includes: types.Filter{
				Arches: []string{"s390", "s390x"},
			},
			Excludes: types.Filter{
				Caps: []string{"CAP_SYS_ADMIN"},
			},
		},
		{
			Names: []string{
				"reboot",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_BOOT"},
			},
		},
		{
			Names: []string{
				"chroot",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_CHROOT"},
			},
		},
		{
			Names: []string{
				"delete_module",
				"init_module",
				"finit_module",
				"query_module",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_MODULE"},
			},
		},
		{
			Names: []string{
				"acct",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_PACCT"},
			},
		},
		{
			Names: []string{
				"kcmp",
				"process_vm_readv",
				"process_vm_writev",
				"ptrace",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_PTRACE"},
			},
		},
		{
			Names: []string{
				"iopl",
				"ioperm",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_RAWIO"},
			},
		},
		{
			Names: []string{
				"settimeofday",
				"stime",
				"clock_settime",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_TIME"},
			},
		},
		{
			Names: []string{
				"vhangup",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_TTY_CONFIG"},
			},
		},
		{
			Names: []string{
				"get_mempolicy",
				"mbind",
				"set_mempolicy",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYS_NICE"},
			},
		},
		{
			Names: []string{
				"syslog",
			},
			Action: types.ActAllow,
			Args:   []*types.Arg{},
			Includes: types.Filter{
				Caps: []string{"CAP_SYSLOG"},
			},
		},
	}

	return &types.Seccomp{
		DefaultAction: types.ActErrno,
		ArchMap:       arches(),
		Syscalls:      syscalls,
	}
}
