//go:build darwin

package cli

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/unix"
)

func init() {
	parentPIDForPID = darwinParentPIDForPID
}

// darwinParentPIDForPID returns the parent pid via the
// `kern.proc.pid.<pid>` sysctl, which fills a kinfo_proc struct.
// Single syscall, no fork — replaces the earlier `ps` shell-out which
// would have cost ~1-2ms per ancestor hop.
//
// The kinfo_proc struct layout on darwin (arm64/x86_64) places the
// parent pid (e_ppid) inside the embedded `eproc` struct at offset
// 560 from the kinfo_proc start. The offset was verified against
// XNU's bsd/sys/proc.h on macOS 14.x; it's been stable for ~15 years
// across macOS releases because `ps` and other tools rely on it.
//
// Returns 0 + error when the process no longer exists (ESRCH).
func darwinParentPIDForPID(pid int) (int, error) {
	mib := []int32{
		_CTL_KERN,
		_KERN_PROC,
		_KERN_PROC_PID,
		int32(pid),
	}
	buf, err := sysctlRawMIB(mib)
	if err != nil {
		return 0, err
	}
	if len(buf) < ePPidOffset+4 {
		return 0, errors.New("kinfo_proc: buffer too small for e_ppid")
	}
	ppid := *(*int32)(unsafe.Pointer(&buf[ePPidOffset]))
	if ppid <= 0 {
		return 0, errors.New("kinfo_proc: invalid e_ppid")
	}
	return int(ppid), nil
}

// Sysctl MIB constants for darwin (libc names; not exported by
// x/sys/unix on darwin as of go 1.22, so we declare them inline).
const (
	_CTL_KERN      = 1
	_KERN_PROC     = 14
	_KERN_PROC_PID = 1
)

// ePPidOffset is the byte offset of `e_ppid` (int32) within the
// kinfo_proc struct on darwin amd64/arm64. Stable across the platform.
const ePPidOffset = 560

// sysctlRawMIB performs a two-step sysctl: probe for size, then
// allocate and read.
func sysctlRawMIB(mib []int32) ([]byte, error) {
	var sz uintptr
	_, _, errno := unix.Syscall6(
		unix.SYS_SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		0,
		uintptr(unsafe.Pointer(&sz)),
		0,
		0,
	)
	if errno != 0 {
		return nil, errno
	}
	if sz == 0 {
		return nil, nil
	}
	buf := make([]byte, sz)
	_, _, errno = unix.Syscall6(
		unix.SYS_SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&sz)),
		0,
		0,
	)
	if errno != 0 {
		return nil, errno
	}
	return buf[:sz], nil
}
