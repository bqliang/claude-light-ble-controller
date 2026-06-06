// Claude Code 在各种 hook 事件上调用 dispatch 子命令；dispatcher 负责：
//  1. 校验 hook 传进来的 mode 是否合法；
//  2. 从 stdin 读取注入的 JSON（事件名/工具名/消息），写日志；
//  3. 把 mode 翻译成 gate 用的 action（thinking → turn-start 等）；
//  4. 调用 gate 子命令拿到 yes/no 决策；
//  5. 命中后 fork ble 子命令在后台跑，本进程立刻返回，让 hook 不被 BLE 延迟拖累。
//
// 关键不变量：dispatch 必须是非阻塞的——hook timeout 一般只有 5 秒，
// 整个 BLE 扫描+连接动辄好几秒，所以一定要 fork 异步执行。
package dispatcher

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/bqliang/claude-light/internal/logx"
)

// ValidModes 是 dispatch 这一层接受的 mode 名集合，
// 对照 shell 中 case "$MODE" 分支列表。
var ValidModes = map[string]struct{}{
	"thinking": {},
	"busy":     {},
	"success":  {},
	"error":    {},
	"yellow":   {},
	"green":    {},
	"off":      {},
	"alarm":    {},
	"demo":     {},
	"traffic":  {},
	"red":      {},
}

// IsValidMode 检查 mode 是否在白名单中。
func IsValidMode(mode string) bool {
	_, ok := ValidModes[mode]
	return ok
}

// Run 是 dispatch 子命令的入口。
// logger 由上层创建好（基于 paths.Resolve()），stdin 是 hook 注入的 JSON 流。
// exe 是二进制的绝对路径，用来 fork gate/ble 子命令。
func Run(mode, hookLabel, exe string, logger *logx.Logger, stdin io.Reader) {
	if mode == "" {
		logger.Mode, logger.Label = "none", hookLabel
		logger.Write("skip missing mode")
		return
	}
	if !IsValidMode(mode) {
		logger.Mode, logger.Label = mode, hookLabel
		logger.Write("skip invalid mode")
		return
	}

	logger.Mode, logger.Label = mode, hookLabel

	// 读 stdin（hook 给的 JSON 体积不大）。出错就当没读到，对应 shell INPUT="$(cat 2>/dev/null || true)"
	data := readAllStdin(stdin)
	event, tool, msg := parseHookFields(data)
	if event != "" || tool != "" || msg != "" {
		logger.Write(fmt.Sprintf(
			"event=%s tool=%s message=%s",
			defaultStr(event, "unknown"),
			defaultStr(tool, "none"),
			defaultStr(msg, "none"),
		))
	}

	// mode -> gate action 翻译表，与 shell case "$MODE" 一致
	gateAction := translateAction(mode)

	// 调 gate 子命令。失败时兜底成 yes:${MODE}，宁可发也别让灯不响应。
	result := callGate(exe, gateAction, mode)
	if result == "" {
		result = "yes:" + mode
	}

	if !hasPrefix(result, "yes:") {
		// no:reason —— gate 决定跳过
		logger.Write("skip gate=" + stripPrefix(result, "no:"))
		return
	}

	sendMode := stripPrefix(result, "yes:")
	logger.Write("send " + sendMode)

	// 后台拉起 ble 子命令。spawn* 用平台特化的方式让子进程脱离父进程，
	// 输出统一重定向到日志，与 shell nohup ... >>"$LOG_FILE" & + disown 等价。
	if err := spawnBLE(exe, sendMode, logger.Path); err != nil {
		logger.Write("spawn ble failed: " + err.Error())
	}
}

// translateAction 是 mode -> gate action 的翻译表，对照 shell case "$MODE"。
func translateAction(mode string) string {
	switch mode {
	case "thinking":
		return "turn-start"
	case "busy":
		return "busy"
	case "success":
		return "stop-success"
	case "error":
		return "stop-error"
	case "yellow", "alarm":
		return "alarm"
	case "green", "off", "demo", "traffic", "red":
		return "idle"
	default:
		return "alarm"
	}
}

// callGate 同步执行 "claude-light gate <action> <mode>" 并返回它的 stdout 第一行。
// 出错返回 ""，让上层走兜底逻辑 yes:${MODE}。
func callGate(exe, action, mode string) string {
	cmd := exec.Command(exe, "gate", action, mode)
	cmd.Stderr = io.Discard
	out, err := cmdOutputWithTimeout(cmd, 1500*time.Millisecond)
	if err != nil {
		return ""
	}
	line := string(bytes.TrimRight(out, "\r\n"))
	if i := bytes.IndexByte([]byte(line), '\n'); i >= 0 {
		line = line[:i]
	}
	return line
}

// cmdOutputWithTimeout 在指定超时内运行命令，返回 stdout。超时则 Kill 子进程并返回错误。
func cmdOutputWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.Bytes(), err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return buf.Bytes(), fmt.Errorf("gate timeout")
	}
}

// readAllStdin 尽力把 stdin 全部读出来；hook 不一定会有 stdin，读不到就当空。
// 加 1MiB 上限，避免某个奇怪的 hook 灌进来海量数据。
func readAllStdin(r io.Reader) []byte {
	br := bufio.NewReader(r)
	const cap = 1 << 20
	data, _ := io.ReadAll(io.LimitReader(br, cap))
	return data
}

// parseHookFields 从 hook JSON 里取三个字段：hook_event_name / tool_name / message，
// 对照 shell 中通过 jq 取字段的语义。解析失败时全部返回空串。
func parseHookFields(data []byte) (event, tool, message string) {
	if len(data) == 0 {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	event = stringField(m, "hook_event_name")
	tool = stringField(m, "tool_name")
	message = stringField(m, "message")
	return
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
func stripPrefix(s, prefix string) string {
	if hasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}
