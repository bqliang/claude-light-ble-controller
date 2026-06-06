package cmd

import (
	"os"

	"github.com/bqliang/claude-light/internal/dispatcher"
	"github.com/bqliang/claude-light/internal/logx"
	"github.com/bqliang/claude-light/internal/paths"
	"github.com/spf13/cobra"
)

var dispatchCmd = &cobra.Command{
	Use:   "dispatch <mode> [hook-label]",
	Short: "入口子命令，Claude Code hook 调用的入口",
	Long: `从 stdin 读取 Claude Code 注入的 JSON、调用 gate 做防抖判断、
命中后 fork ble 子命令在后台写蓝牙，让 hook 立刻返回（非阻塞）。

参数：
  <mode>        灯光模式：thinking/busy/success/error/yellow/green/off/alarm/demo/traffic/red
  [hook-label]  可选标签（例如 user-prompt / pre-tool），写入日志便于追踪

示例：
  echo '{"hook_event_name":"PostToolUseFailure"}' | claude-light dispatch error post-tool-failure`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		mode := args[0]
		hookLabel := "claude"
		if len(args) >= 2 {
			hookLabel = args[1]
		}

		layout := paths.Resolve()
		logger := logx.New(layout.LogFile)

		exe, err := os.Executable()
		if err != nil {
			// 拿不到二进制路径时记一行日志，但还是尽力执行（dispatcher 会回退）
			logger.Mode, logger.Label = mode, hookLabel
			logger.Write("warn: cannot get executable path")
			exe = "" // dispatcher.Run 会用空字符串兜底
		}

		dispatcher.Run(mode, hookLabel, exe, logger, cmd.InOrStdin())
	},
}

func init() {
	rootCmd.AddCommand(dispatchCmd)
}
