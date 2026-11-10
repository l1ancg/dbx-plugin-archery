//go:build !windows

package rpc

// promptSavePathOS 的非 Windows 桩：本插件仅发布 windows-x64 包，
// 其余平台无对话框实现，直接按取消处理（调用方回退直写下载目录）。
func promptSavePathOS(defaultName, initialDir string) (string, bool, error) {
	return "", true, nil
}
