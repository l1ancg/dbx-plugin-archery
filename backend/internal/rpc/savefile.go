package rpc

import (
	"os"
	"path/filepath"
	"strings"
)

// saveFilter 是「另存为」对话框的过滤器与默认扩展名。
type saveFilter struct {
	Filter string
	DefExt string
}

// saveDialogFilter 按落盘文件名的扩展名选过滤器：SQL / Excel / 其他。
func saveDialogFilter(name string) saveFilter {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sql":
		return saveFilter{Filter: "SQL (*.sql)\x00*.sql\x00\x00", DefExt: "sql"}
	case ".xlsx":
		return saveFilter{Filter: "Excel 工作簿 (*.xlsx)\x00*.xlsx\x00\x00", DefExt: "xlsx"}
	default:
		return saveFilter{Filter: "所有文件 (*.*)\x00*.*\x00\x00"}
	}
}

// resolveSaveDir 校验请求里的初始目录，不可用（不存在或不是目录）时回退默认下载目录。
func resolveSaveDir(requested, fallback string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return fallback
	}
	info, err := os.Stat(requested)
	if err != nil || !info.IsDir() {
		return fallback
	}
	return requested
}
