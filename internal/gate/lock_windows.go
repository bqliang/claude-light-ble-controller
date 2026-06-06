//go:build windows

package gate

// lock_windows.go: Windows 上的跨进程文件锁（独占），通过 LockFileEx/UnlockFileEx 实现。

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const lockfileExclusiveLock = 0x2

// fileLock 持有锁定的文件句柄。
type fileLock struct {
	f *os.File
}

// acquireLock 打开锁文件并请求独占阻塞锁。
func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	var ol syscall.Overlapped
	r1, _, e1 := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		^uintptr(0), // 锁定字节数低 32 位
		^uintptr(0), // 高 32 位（一起表示锁定整个文件）
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		f.Close()
		return nil, e1
	}
	return &fileLock{f: f}, nil
}

// Release 解锁并关闭文件句柄。
func (l *fileLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	var ol syscall.Overlapped
	procUnlockFileEx.Call(
		l.f.Fd(),
		0,
		^uintptr(0),
		^uintptr(0),
		uintptr(unsafe.Pointer(&ol)),
	)
	l.f.Close()
}
