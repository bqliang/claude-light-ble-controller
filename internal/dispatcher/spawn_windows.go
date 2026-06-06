//go:build windows

package dispatcher

// spawn_windows.go: 在 Windows 上以 detached 模式拉起 ble 子命令。
//
// shell 版用的是 nohup ... & + disown，等价于"父进程退出后子进程继续跑、
// 不在同一个终端会话里、stdout/stderr 重定向到日志文件"。Windows 上没有这些
// POSIX 概念，但可以用 CreationFlags = DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
// 让子进程不继承控制台、独立成组，从而让 dispatch 可以立刻 exit。

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// spawnBLE fork 一个独立的 "claude-light ble <mode>" 进程在后台运行，
// stdout/stderr 都重定向到日志文件，dispatch 立刻返回。
func spawnBLE(exe, mode, logPath string) error {
	cmd := exec.Command(exe, "ble", mode)

	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
			defer f.Close()
		}
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup | createNoWindow,
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	// Release 让 Go runtime 不再追踪子进程 handle，等价 disown
	_ = cmd.Process.Release()
	return nil
}
