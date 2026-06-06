// Package paths 集中解析 state.json / state.lock / claude-light.log 的存放位置。
//
// 目录优先级：环境变量 CLAUDE_LIGHT_DIR > 二进制所在目录 > 当前工作目录。
// state / lock / log 三个文件始终放在同一个目录下。
package paths

import (
	"os"
	"path/filepath"
)

// Layout 是一组路径，由 Resolve 一次性算出来。
type Layout struct {
	BaseDir   string // 解析得到的工作目录
	StateFile string // state.json
	LockFile  string // state.lock
	LogFile   string // claude-light.log
}

// Resolve 计算 Layout。所有文件都在同一个目录下。
func Resolve() Layout {
	dir := os.Getenv("CLAUDE_LIGHT_DIR")
	if dir == "" {
		// 二进制所在目录是默认值；这是子命令之间共享 state 的关键。
		exe, err := os.Executable()
		if err == nil {
			dir = filepath.Dir(exe)
		} else {
			dir, _ = os.Getwd()
		}
	}

	return Layout{
		BaseDir:   dir,
		StateFile: filepath.Join(dir, "state.json"),
		LockFile:  filepath.Join(dir, "state.lock"),
		LogFile:   filepath.Join(dir, "claude-light.log"),
	}
}
