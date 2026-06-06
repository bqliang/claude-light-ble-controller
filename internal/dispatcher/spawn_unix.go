//go:build !windows

package dispatcher

// spawn_unix.go: 在 Unix 上以 setsid 方式拉起 ble 子命令，等价于 nohup ... & + disown。

import (
	"os"
	"os/exec"
	"syscall"
)

// spawnBLE fork 一个独立的 "claude-light ble <mode>" 进程在后台运行。
// 通过 Setsid:true 让子进程脱离父进程的会话，父进程退出/终端关闭都不会带走它。
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

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}
