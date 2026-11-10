//go:build windows

package rpc

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Windows 原生「另存为」对话框：sidecar 是与 DBX 同用户桌面会话的本机进程，
// 可以直接弹 comdlg32 的 GetSaveFileNameW，不需要宿主提供能力。
var (
	comdlg32              = syscall.NewLazyDLL("comdlg32.dll")
	procGetSaveFileNameW  = comdlg32.NewProc("GetSaveFileNameW")
	procCommDlgExtendedEr = comdlg32.NewProc("CommDlgExtendedError")
)

const (
	ofnOverwritePrompt = 0x00000002
	ofnPathMustExist   = 0x00000800
	ofnNoChangeDir     = 0x00000008
	// GetSaveFileNameW 返回 0 且 CommDlgExtendedError 为 0 表示用户取消。
	fnerrSuccess = 0
	// 文件名缓冲区上限（UTF-16 单元数），与资源管理器一致。
	maxPathBuf = 32768
)

type openFileNameW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LPSTRFilter       *uint16
	LPSTRCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LPSTRFile         *uint16
	NMaxFile          uint32
	LPSTRFileTitle    *uint16
	NMaxFileTitle     uint32
	LPSTRInitialDir   *uint16
	LPSTRTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LPSTRDefExt       *uint16
	LCustData         uintptr
	LPFNHook          uintptr
	LPTemplateName    *uint16
	PVReserved        uintptr
	DWReserved        uint32
	FlagsEx           uint32
}

// promptSavePathOS 是 windows 平台的对话框实现。
func promptSavePathOS(defaultName, initialDir string) (string, bool, error) {
	spec := saveDialogFilter(defaultName)
	filterUTF16, err := utf16WithDoubleNul(spec.Filter)
	if err != nil {
		return "", false, err
	}
	nameUTF16, err := syscall.UTF16FromString(padFileNameBuffer(defaultName))
	if err != nil {
		return "", false, err
	}
	if len(nameUTF16) < maxPathBuf {
		nameUTF16 = append(nameUTF16, make([]uint16, maxPathBuf-len(nameUTF16))...)
	}
	var initialPtr *uint16
	if initialDir != "" {
		if ptr, ptrErr := syscall.UTF16PtrFromString(initialDir); ptrErr == nil {
			initialPtr = ptr
		}
	}
	var defExtPtr *uint16
	if spec.DefExt != "" {
		if ptr, ptrErr := syscall.UTF16PtrFromString(spec.DefExt); ptrErr == nil {
			defExtPtr = ptr
		}
	}

	ofn := openFileNameW{
		LStructSize:     uint32(unsafe.Sizeof(openFileNameW{})),
		LPSTRFilter:     &filterUTF16[0],
		NFilterIndex:    1,
		LPSTRFile:       &nameUTF16[0],
		NMaxFile:        uint32(len(nameUTF16)),
		LPSTRInitialDir: initialPtr,
		LPSTRDefExt:     defExtPtr,
		Flags:           ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir,
	}
	ret, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		code, _, _ := procCommDlgExtendedEr.Call()
		if code == fnerrSuccess {
			return "", true, nil
		}
		return "", false, fmt.Errorf("GetSaveFileNameW failed: code %d", code)
	}
	path := syscall.UTF16ToString(nameUTF16)
	if path == "" {
		return "", true, nil
	}
	return path, false, nil
}

// utf16WithDoubleNul 编码以 \x00 分隔、\x00\x00 结尾的对话框 filter 字符串。
func utf16WithDoubleNul(s string) ([]uint16, error) {
	// 逐段 UTF16FromString 后手工拼接：StringToUTF16Ptr 拒绝内嵌 NUL。
	runes := make([]uint16, 0, len(s)+2)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == 0 {
			segment, err := syscall.UTF16FromString(s[start:i])
			if err != nil {
				return nil, err
			}
			runes = append(runes, segment...)
			start = i + 1
			if i == len(s) {
				break
			}
		}
	}
	runes = append(runes, 0)
	return runes, nil
}

// padFileNameBuffer 给默认文件名留出完整缓冲区语义（截断超长部分避免溢出）。
func padFileNameBuffer(name string) string {
	runes := []rune(name)
	if len(runes) > maxPathBuf-1 {
		runes = runes[:maxPathBuf-1]
	}
	return string(runes)
}
