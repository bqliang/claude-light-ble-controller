# ClaudeLight BLE Controller

通过 Claude Code hooks 控制 ClaudeLight BLE 交通灯的 Go CLI 工具，使用 Cobra 框架构建清晰的 CLI 结构。

## 致谢 / Acknowledgements

本项目是基于以下项目的 Go 语言重写版本：

- **原项目**: [cursor_agent_status_light](https://github.com/JasonLam08/cursor_agent_status_light) by [@JasonLam08](https://github.com/JasonLam08) - 基于 ESP32-C3 + BLE 的 Cursor Agent 状态灯项目，包含完整的硬件接线指南和固件代码。

- **Claude Code Hooks**: 来自 [@minivv](https://github.com/minivv)（详情见[该 Issue](https://github.com/JasonLam08/cursor_agent_status_light/issues/1)），提供了 Claude Code 的 hooks 配置和状态映射逻辑。

### 为什么用 Go 重写？

原项目使用 Python 脚本控制 BLE 通信，需要安装 Python 环境和 `bleak` 依赖，我个人只是不想在我电脑上安装 Python 😂。故将 Python 代码用 Go 重写，编译为单个二进制文件，无需安装 Python 运行时。

---

## 项目结构

```
claude-light/
  ├── main.go                              # 极简入口，调用 cmd.Execute()
  ├── cmd/                                 # Cobra 命令层（CLI 交互 + 参数解析）
  │   ├── root.go                          # 根命令 + 帮助文档
  │   ├── dispatch.go                      # dispatch 子命令
  │   ├── gate.go                          # gate 子命令
  │   └── ble.go                           # ble 子命令
  └── internal/                            # 业务逻辑包（不导出）
      ├── paths/paths.go                   # 解析 state/log/lock 路径
      ├── logx/logx.go                     # 日志写入器
      ├── gate/                            # 状态机 + 防抖逻辑 + 文件锁
      │   ├── gate.go                      # 纯逻辑：Decide() + Run()
      │   ├── lock_windows.go              # Windows LockFileEx
      │   └── lock_unix.go                 # Unix flock
      ├── ble/ble.go                       # BLE 扫描/连接/写入（tinygo.org/x/bluetooth）
      └── dispatcher/                      # dispatch 业务逻辑 + 后台进程 spawn
          ├── dispatcher.go                # JSON 解析 + gate 调用 + 日志
          ├── spawn_windows.go             # Windows DETACHED_PROCESS
          └── spawn_unix.go                # Unix setsid
```

## 三个子命令

### 1. `claude-light dispatch <mode> [hook-label]`

Claude Code hook 的入口。从 stdin 读取 JSON，调用 gate 判断，命中后 fork ble 在后台执行。

**测试方法：**
```bash
echo '{"hook_event_name":"PostToolUseFailure"}' | ./claude-light.exe dispatch error post-tool-failure
```

### 2. `claude-light gate <action> <mode>`

通过文件锁保护 `state.json` 原子读改写，决定本次是否真的往 BLE 发 mode（防止并行 hook 重复扫描）。

**输出：**
- `yes:<mode>` → 命中，dispatcher 会 fork ble 发送
- `no:<reason>` → 跳过（防抖/阶段冲突）

**测试方法：**
```bash
./claude-light.exe gate turn-start thinking   # 输出 yes:thinking 或 no:turn-start debounce
./claude-light.exe gate busy busy              # 可能输出 no:sticky busy
```

### 3. `claude-light ble <mode>`

扫描名为 `ClaudeLight` 的 BLE 设备，连接后向预定义 GATT 特征写入 mode 文本（UTF-8）。

**允许的 mode：**
`ai`, `alarm`, `busy`, `demo`, `error`, `green`, `off`, `red`, `success`, `thinking`, `traffic`, `yellow`

**测试方法（手动测灯光，跳过 gate）：**
```bash
./claude-light.exe ble green      # 同步执行，测试绿灯
```

**退出码：**
- `0` — 成功
- `1` — 参数错误（mode 不在白名单）
- `2` — 未找到设备
- `3` — 连接失败
- `4` — 其它错误（UUID 解析/特征未找到/写入失败）

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CLAUDE_LIGHT_DIR` | 二进制所在目录 | `state.json` / `state.lock` / `claude-light.log` 的工作目录 |

**示例（指定工作目录）：**
```bash
export CLAUDE_LIGHT_DIR=~/.claude/hooks/claude-light
./claude-light dispatch thinking test
```

## 构建

```bash
go mod tidy
go build -o claude-light .
```

**跨平台编译：**

```bash
# Windows
go build -o claude-light.exe .

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o claude-light-darwin-amd64 .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o claude-light-darwin-arm64 .

# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o claude-light-linux-amd64 .

# Linux (arm64, 如树莓派)
GOOS=linux GOARCH=arm64 go build -o claude-light-linux-arm64 .
```

**二进制大小：** ~7 MB

**平台依赖：**
- Windows：WinRT 蓝牙栈
- macOS：CoreBluetooth
- Linux：BlueZ（需安装 `bluez` 包）

## 功能特性

✅ gate 状态机（`turn-start`, `busy`, `thinking`, `idle`, `stop-success/error`, `await-user`, `plan-detect`, `denied-*`）  
✅ 防抖窗口（`thinking 5s`, `busy 8s`, `alarm 0.5s`, `success/error/green 3s`）  
✅ `turn_phase` 迁移和字段清理  
✅ `await-user` 模式强制转 `alarm`  
✅ mode→action 翻译（`thinking→turn-start`, `success→stop-success`）  
✅ stdin JSON 字段解析（`hook_event_name`, `tool_name`, `message`）  
✅ 日志格式：`[YYYY-MM-DD HH:MM:SS] claude-light mode=X label=Y <msg>`  
✅ 跨平台文件锁（Windows `LockFileEx` / Unix `flock`）  
✅ 后台进程 spawn（Windows `DETACHED_PROCESS` / Unix `setsid`）  

## 使用方式

### 配置 Claude Code hooks

将 `settings.json.snippet` 的内容添加到 Claude Code 的配置文件 `~/.claude/settings.json` 中：

> 如果使用 cc switch 来管理 Claude Code，将配置写入通用配置。
> 别忘了将二进制所在目录添加到环境变量。


**依赖：**
- [`github.com/spf13/cobra`](https://github.com/spf13/cobra) v1.8.1 — CLI 框架
- [`tinygo.org/x/bluetooth`](https://github.com/tinygo-org/bluetooth) v0.13.0 — 跨平台 BLE（Windows WinRT / macOS CoreBluetooth / Linux BlueZ）
