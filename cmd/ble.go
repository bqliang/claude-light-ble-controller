package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/bqliang/claude-light/internal/ble"
	"github.com/spf13/cobra"
)

var bleCmd = &cobra.Command{
	Use:   "ble <mode>",
	Short: "扫描并连接 ClaudeLight BLE 设备，写入指定 mode",
	Long: `扫描名为 ClaudeLight 的 BLE 外设，连接后向预定义 GATT 特征写入 mode 文本。

该子命令是同步阻塞的（扫描+连接可能需要几秒），正常情况下由 dispatch 在后台 fork 调用。
你也可以手动执行它来测试灯光（跳过 gate 防抖）。

参数：
  <mode>  灯光模式，必须是以下之一：
          ` + strings.Join(ble.SortedValidModes(), ", ") + `

示例：
  claude-light ble green    # 同步执行，手动测试绿灯
  claude-light ble error  	# 手动测试错误模式

退出码：
  0   成功
  1   参数错误（mode 不在白名单）
  2   未找到设备
  3   连接失败
  4   其它错误（UUID 解析 / 特征未找到 / 写入失败等）`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mode := ble.Normalize(args[0])
		if !ble.IsValidMode(mode) {
			fmt.Fprintf(cmd.ErrOrStderr(), "未知 mode: %s\n", mode)
			fmt.Fprintf(cmd.ErrOrStderr(), "可用 mode: %s\n", strings.Join(ble.SortedValidModes(), ", "))
			os.Exit(1)
		}

		if err := ble.WriteMode(mode, cmd.OutOrStdout()); err != nil {
			// ble.WriteMode 已经打印过友好提示了，这里只取退出码
			if e, ok := ble.AsErr(err); ok {
				os.Exit(e.Code)
			}
			os.Exit(4)
		}
	},
}

func init() {
	rootCmd.AddCommand(bleCmd)
}
