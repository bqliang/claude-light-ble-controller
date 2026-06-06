// 扫描名为 ClaudeLight 的 BLE 外设，连接后向预定义 GATT 特征写入 mode 文本。
//
// 通过 tinygo.org/x/bluetooth：
//   - Windows: WinRT，无需 CGO；
//   - macOS/Linux: CoreBluetooth / BlueZ，central role 与 bleak 行为接近。
package ble

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"
)

// 与 ESP32 端固件约定的常量
const (
	DeviceName   = "ClaudeLight"
	ModeCharUUID = "b8b7e002-7a6b-4f4f-9a8b-11c0ffee0001"
	ScanTimeout  = 10 * time.Second
)

// 允许通过 BLE 写入的 mode 集合
var ValidModes = map[string]struct{}{
	"red":      {},
	"yellow":   {},
	"green":    {},
	"busy":     {},
	"error":    {},
	"thinking": {},
	"ai":       {},
	"success":  {},
	"traffic":  {},
	"alarm":    {},
	"demo":     {},
	"off":      {},
}

// SortedValidModes 返回字典序的 mode 列表，主要用于打印帮助信息。
func SortedValidModes() []string {
	modes := make([]string, 0, len(ValidModes))
	for m := range ValidModes {
		modes = append(modes, m)
	}
	sort.Strings(modes)
	return modes
}

// IsValidMode 判断 mode 是否在白名单中。调用方在传入前应自行 trim+lower。
func IsValidMode(mode string) bool {
	_, ok := ValidModes[mode]
	return ok
}

// Err 包装一个错误及其对应的进程退出码，方便 cobra 命令层 errors.As 提取。
// 退出码：1=参数错；2=找不到设备；3=连接失败；4=其它失败。
type Err struct {
	Code  int
	Msg   string
	Cause error
}

func (e *Err) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}
func (e *Err) Unwrap() error { return e.Cause }

// AsErr 把 error 拆出 *Err 来读取退出码；不是 *Err 时返回 (nil, false)。
func AsErr(err error) (*Err, bool) {
	var e *Err
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// WriteMode 完成一次完整的"扫描 -> 连接 -> 发现服务/特征 -> 写入"流程。
// out 是状态信息输出流（cobra 的 cmd.OutOrStdout()），便于日志重定向。
// 任何阶段失败都会向 out 写一条用户友好的提示，并返回带退出码的 *Err。
func WriteMode(mode string, out io.Writer) error {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		fmt.Fprintln(out, "无法启用蓝牙适配器，请确认本机蓝牙已开启")
		return &Err{Code: 4, Msg: "enable adapter", Cause: err}
	}

	fmt.Fprintf(out, "正在扫描 BLE 设备：%s ...\n", DeviceName)

	// Scan 是阻塞调用，回调里命中目标后调用 StopScan() 才返回。
	// 用一个 goroutine 在 ScanTimeout 后强制 StopScan，实现"超时未找到就放弃"。
	var found *bluetooth.ScanResult
	stopByTimeout := time.AfterFunc(ScanTimeout, func() { _ = adapter.StopScan() })
	defer stopByTimeout.Stop()

	if err := adapter.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
		if r.LocalName() == DeviceName {
			cp := r // 复制一份，避免回调返回后底层缓冲被复用
			found = &cp
			_ = a.StopScan()
		}
	}); err != nil {
		fmt.Fprintln(out, "扫描出错")
		return &Err{Code: 2, Msg: "scan", Cause: err}
	}

	if found == nil {
		fmt.Fprintln(out, "没有找到 ClaudeLight。请确认：")
		fmt.Fprintln(out, "1. ESP32 已通电")
		fmt.Fprintln(out, "2. 代码已刷入 BLE 增强版")
		fmt.Fprintln(out, "3. 距离足够近")
		fmt.Fprintln(out, "4. 蓝牙已打开，并给当前进程蓝牙权限")
		return &Err{Code: 2, Msg: "device not found"}
	}

	fmt.Fprintf(out, "找到设备: %s\n", found.Address.String())

	device, err := adapter.Connect(found.Address, bluetooth.ConnectionParams{})
	if err != nil {
		fmt.Fprintln(out, "连接失败")
		return &Err{Code: 3, Msg: "connect", Cause: err}
	}
	defer device.Disconnect()

	fmt.Fprintf(out, "已连接，发送 mode=%s\n", mode)

	targetUUID, err := bluetooth.ParseUUID(ModeCharUUID)
	if err != nil {
		return &Err{Code: 4, Msg: "parse uuid", Cause: err}
	}

	// 不知道目标特征属于哪个 service，就拉全部 service 再逐个查 UUID。
	svcs, err := device.DiscoverServices(nil)
	if err != nil {
		return &Err{Code: 4, Msg: "discover services", Cause: err}
	}
	var targetChar *bluetooth.DeviceCharacteristic
	for i := range svcs {
		chars, derr := svcs[i].DiscoverCharacteristics([]bluetooth.UUID{targetUUID})
		if derr != nil || len(chars) == 0 {
			continue
		}
		c := chars[0]
		targetChar = &c
		break
	}
	if targetChar == nil {
		return &Err{
			Code: 4,
			Msg:  fmt.Sprintf("Characteristic %s was not found!", ModeCharUUID),
		}
	}

	if _, err := targetChar.Write([]byte(mode)); err != nil {
		return &Err{Code: 4, Msg: "write", Cause: err}
	}

	fmt.Fprintln(out, "发送完成")

	// 留一点窗口让 BLE stack 真正把 ACK 收到再断开
	time.Sleep(150 * time.Millisecond)
	return nil
}
