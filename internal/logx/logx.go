// Package logx 是一个最小化的"追加到 claude-light.log"日志写入器。
//
//	[YYYY-MM-DD HH:MM:SS] claude-light mode=X label=Y <msg>
//
// 任何 IO 错误都会被吞掉——日志失败不应阻断 hook。
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger 持有日志文件路径以及与"本次调用上下文"绑定的 mode/label。
// 一次 dispatch 只会构造一个 Logger，期间多次 Write 共享 mode/label。
type Logger struct {
	Path  string // claude-light.log 的绝对路径
	Mode  string // 当前 mode（例如 thinking/busy/error）
	Label string // hook 标签（例如 user-prompt / pre-tool）
}

// New 用给定的路径创建 Logger。mode/label 后续可以通过字段直接修改。
func New(path string) *Logger {
	return &Logger{Path: path}
}

// Write 追加一行到日志。msg 已经是完整的尾部内容，本函数补齐时间戳和 mode/label 头部。
func (l *Logger) Write(msg string) {
	if l == nil || l.Path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	mode := l.Mode
	if mode == "" {
		mode = "none"
	}
	label := l.Label
	if label == "" {
		label = "claude"
	}

	fmt.Fprintf(f, "[%s] claude-light mode=%s label=%s %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		mode,
		label,
		msg,
	)
}
