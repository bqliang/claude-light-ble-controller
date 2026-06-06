// Package cmd 包含所有 Cobra 命令的定义。
//
// rootCmd 是顶层入口，下挂三个子命令：dispatch / gate / ble。
// 每个子命令对应原来的 runDispatch / runGate / runBLE，现在改为调用
// internal/* 下的业务逻辑包，cobra 只负责 CLI 交互和参数解析。
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd 是顶层命令。执行时不带子命令名会打印 help。
var rootCmd = &cobra.Command{
	Use:   "claude-light",
	Short: "Claude Code -> ClaudeLight BLE 状态灯调度器",
	Long: `claude-light: Claude Code 钩子驱动的蓝牙交通灯控制器（Go 版）

这是一个通过 Claude Code hooks 控制 ClaudeLight BLE 交通灯的 Go CLI 工具
包含三种工作模式（通过子命令切换）：

  claude-light dispatch <mode> [hook-label]
      从 stdin 读取 Claude Code 注入的 JSON、调用 gate 做防抖判断、
      命中后 fork 出 ble 子命令在后台写蓝牙，让 hook 立刻返回。

  claude-light gate <action> <mode>
      通过 state.lock 文件锁，对 state.json 做原子读改写，
      决定本次是否真的需要往 BLE 设备发 mode（防止并行 hook 重复扫描）。
      输出 "yes:<mode>" 表示要发，"no:<reason>" 表示跳过。

  claude-light ble <mode>
      扫描名为 ClaudeLight 的 BLE 设备，连上去后向指定 GATT 特征值写入 mode 文本。

环境变量：
  CLAUDE_LIGHT_DIR   state.json / state.lock / log 的工作目录（默认二进制所在目录）`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra 已经把错误打印到 stderr 了，这里只需要设置非零退出码
		os.Exit(1)
	}
}

func init() {
	// rootCmd 本身不需要 flags，子命令会各自添加自己的 flags
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
