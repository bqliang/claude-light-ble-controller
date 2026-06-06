package cmd

import (
	"github.com/bqliang/claude-light/internal/gate"
	"github.com/bqliang/claude-light/internal/paths"
	"github.com/spf13/cobra"
)

var gateCmd = &cobra.Command{
	Use:   "gate <action> <mode>",
	Short: "状态机防抖门控，决定是否真的往 BLE 发送 mode",
	Long: `通过 state.lock 文件锁，对 state.json 做原子读改写，
决定本次是否真的需要往 BLE 设备发 mode（防止并行 hook 重复扫描）。

输出到 stdout：
  yes:<mode>   命中，dispatcher 会 fork ble 子命令发送 <mode>
  no:<reason>  跳过，防抖或阶段冲突导致本次不发

参数：
  <action>  状态机动作：turn-start/busy/thinking/alarm/idle/stop-success/stop-error/...
  <mode>    灯光模式：thinking/busy/success/error/yellow/green/off/alarm 等

示例：
  claude-light gate turn-start thinking   # 新一轮开始，返回 yes:thinking 或 no:turn-start debounce
  claude-light gate busy busy              # 工具调用中，可能返回 no:sticky busy`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		action, mode := args[0], args[1]
		layout := paths.Resolve()
		gate.Run(action, mode, layout.StateFile, layout.LockFile, cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(gateCmd)
}
