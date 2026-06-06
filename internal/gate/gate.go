// 给定一个 action（来自 dispatch 翻译过的 hook 事件）和当前 mode，
// 在文件锁保护下读 state.json -> 决策 -> 写回 state.json，
// 输出 "yes:<mode>" 表示真的去发 BLE，"no:<reason>" 表示跳过。
package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DebounceMS 防抖窗口（毫秒）。键值对照原始 Python DEBOUNCE_MS。
// 同一个 mode 在窗口内重复触发会被吃掉。
var DebounceMS = map[string]int64{
	"thinking": 5000,
	"busy":     8000,
	"alarm":    500,
	"success":  3000,
	"error":    3000,
	"green":    3000,
}

type State struct {
	LastMode      string `json:"last_mode,omitempty"`
	LastTS        int64  `json:"last_ts,omitempty"`
	TurnPhase     string `json:"turn_phase,omitempty"`
	TurnStartedMS int64  `json:"turn_started_ms,omitempty"`
	AwaitingBuild *bool  `json:"awaiting_build,omitempty"`
	BuildStarted  *bool  `json:"build_started,omitempty"`
	PlanTouched   *bool  `json:"plan_touched,omitempty"`
}

// Decision 是 Decide 的输出。
//
//	Send=true 时 Mode 是真要发出去的 mode；
//	Send=false 时 Reason 解释为什么跳过；
//	Silent=true 表示输出端只打 "no" 而不带原因（对应 plan-detect 的特殊路径）。
type Decision struct {
	Send   bool
	Mode   string
	Reason string
	Silent bool
}

// Decide 是状态机的纯逻辑核心。
// 给定 action / mode / 当前 state / 当前时间戳（毫秒），返回新的 state 和决策。
// 分支结构与 Python 版本一一对应，便于人工对照 review。
func Decide(action, mode string, s State, nowMS int64) (State, Decision) {
	lastMode, lastTS, phase := s.LastMode, s.LastTS, s.TurnPhase
	res := Decision{Send: true, Mode: mode}

	switch action {
	case "turn-start":
		// 用户提交新 prompt：开启新一轮，允许从 busy 回到 thinking
		s.TurnPhase = "thinking"
		s.AwaitingBuild = nil
		s.BuildStarted = nil
		s.PlanTouched = nil
		s.TurnStartedMS = nowMS
		if mode == lastMode && (nowMS-lastTS) < DebounceMS["thinking"] {
			res.Send, res.Reason = false, "turn-start debounce"
		}

	case "await-user":
		// 等待用户输入：强制 alarm（黄灯）
		s.TurnPhase = "awaiting_user"
		mode = "alarm"
		res.Mode = mode
		if lastMode == "alarm" && (nowMS-lastTS) < DebounceMS["alarm"] {
			res.Send, res.Reason = false, "await-user debounce"
		}

	case "busy":
		// 工具调用进行中
		s.TurnPhase = "busy"
		if phase == "busy" && lastMode == "busy" {
			res.Send, res.Reason = false, "sticky busy"
		} else if mode == lastMode && (nowMS-lastTS) < DebounceMS["busy"] {
			res.Send, res.Reason = false, "busy debounce"
		}

	case "thinking":
		// PostToolUse 之后通常回到 thinking
		if phase == "busy" {
			// busy 期间不允许往回切回 thinking——否则灯会闪
			res.Send, res.Reason = false, "thinking blocked by busy phase"
		} else if mode == lastMode && (nowMS-lastTS) < DebounceMS["thinking"] {
			res.Send, res.Reason = false, "thinking debounce"
		} else {
			s.TurnPhase = "thinking"
		}

	case "alarm":
		if mode == lastMode && (nowMS-lastTS) < DebounceMS["alarm"] {
			res.Send, res.Reason = false, "alarm debounce"
		}

	case "idle", "stop-success", "stop-error", "stop-alarm":
		// 一轮结束：清空 turn_phase，按错误/告警/成功选不同的防抖窗口
		s.TurnPhase = ""
		debounceKey := "green"
		switch {
		case strings.Contains(action, "error"):
			debounceKey = "error"
		case strings.Contains(action, "alarm"):
			debounceKey = "alarm"
		case strings.Contains(action, "success"):
			debounceKey = "success"
		}
		if action == "stop-success" {
			s.AwaitingBuild = nil
		}
		window, ok := DebounceMS[debounceKey]
		if !ok {
			window = 3000 // 与 Python .get(..., 3000) 一致
		}
		if mode == lastMode && (nowMS-lastTS) < window {
			res.Send, res.Reason = false, fmt.Sprintf("%s debounce", action)
		}

	case "plan-detect":
		// 这个 action 在原 Python 里直接 print("no") 然后返回，不写 state。
		return s, Decision{Send: false, Silent: true}

	case "denied-thinking":
		// 权限被拒后想回 thinking：busy 阶段就粘 busy
		if phase == "busy" {
			mode = "busy"
			res.Mode = mode
			if lastMode == "busy" {
				res.Send, res.Reason = false, "denied sticky busy"
			}
		} else {
			s.TurnPhase = "thinking"
			mode = "thinking"
			res.Mode = mode
		}

	case "denied-error":
		if phase == "busy" {
			res.Send, res.Reason = false, "denied-error during busy"
		} else {
			s.TurnPhase = ""
			if mode == lastMode && (nowMS-lastTS) < DebounceMS["error"] {
				res.Send, res.Reason = false, "denied-error debounce"
			}
		}
	}

	if res.Send {
		s.LastMode = res.Mode
		s.LastTS = nowMS
	}
	return s, res
}

// loadState 读取 state.json。文件不存在或解析失败都返回零值（与 Python 一致）。
func loadState(path string) State {
	var s State
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// saveState 把 state 写回 state.json。Go 的 json.Marshal 默认 UTF-8。
func saveState(path string, s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Run 是 "claude-light gate <action> <mode>" 的入口。
// out 是结果输出流（cobra 命令把 cmd.OutOrStdout() 传进来）。
// stateFile / lockFile 由 cmd 层基于 paths.Resolve() 决定。
func Run(action, mode, stateFile, lockFile string, out io.Writer) {
	mode = normalizeMode(mode)

	if err := os.MkdirAll(filepath.Dir(lockFile), 0o755); err != nil {
		fmt.Fprintln(out, "no:lockdir")
		return
	}
	lock, err := acquireLock(lockFile)
	if err != nil {
		fmt.Fprintln(out, "no:lockfail")
		return
	}
	defer lock.Release()

	state := loadState(stateFile)
	now := time.Now().UnixMilli()
	newState, dec := Decide(action, mode, state, now)

	// plan-detect 走特殊路径：只打 "no"，不写 state
	if dec.Silent {
		fmt.Fprintln(out, "no")
		return
	}

	// 不论命中与否都要持久化——turn_phase 等字段已被本次 action 改动
	_ = saveState(stateFile, newState)

	if dec.Send {
		fmt.Fprintf(out, "yes:%s\n", dec.Mode)
	} else {
		fmt.Fprintf(out, "no:%s\n", dec.Reason)
	}
}

// normalizeMode 复刻 Python sys.argv[2].strip().lower() 的规则。
func normalizeMode(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
