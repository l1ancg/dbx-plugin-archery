package rpc

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// DBX 的插件 UI 跑在 opaque-origin iframe 沙箱里，<a download> 被宿主拦截，
// 「下载回滚SQL」由 sidecar 把文件写到本机下载目录。写的是本机文件、不是数据库，
// 因此只读连接同样放行。
func TestSaveTextFile(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, true)

	dir := t.TempDir()
	origDownloadDir := downloadDir
	downloadDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { downloadDir = origDownloadDir })

	payload := wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename": "rollback_5338.sql",
		"content":  "/*alter table t*/\nrollback;\n",
	})
	if payload["path"] != filepath.Join(dir, "rollback_5338.sql") {
		t.Fatalf("unexpected path: %v", payload["path"])
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "rollback_5338.sql"))
	if readErr != nil {
		t.Fatalf("file not written: %v", readErr)
	}
	if string(data) != "/*alter table t*/\nrollback;\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	// 重名时追加序号，不覆盖已有文件。
	payload = wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename": "rollback_5338.sql",
		"content":  "second",
	})
	if payload["path"] != filepath.Join(dir, "rollback_5338-1.sql") {
		t.Fatalf("expected suffixed path, got %v", payload["path"])
	}

	// 文件名必须是不带路径的普通名字，防路径穿越。
	for _, name := range []string{"..\\evil.sql", "../evil.sql", "a/b.sql", "", "."} {
		err := wfCallErr(t, service, "archery/saveTextFile", map[string]any{"filename": name, "content": "x"})
		if err.Code != -32602 {
			t.Fatalf("filename %q: expected invalid_params, got %+v", name, err)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, "evil.sql")); !os.IsNotExist(statErr) {
		t.Fatal("path traversal must not write outside the download dir")
	}
}

// askPath=true 时弹系统「另存为」对话框：确认写入所选路径，取消时不写文件。
func TestSaveTextFileAskPath(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	// 对话框确认：写入用户选的路径（覆盖确认由对话框负责，不再追加序号）。
	dir := t.TempDir()
	chosen := filepath.Join(dir, "custom", "my_rollback.sql")
	if err := os.MkdirAll(filepath.Dir(chosen), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	origPrompt := promptSavePath
	promptSavePath = func(defaultName, initialDir string) (string, bool, error) {
		if defaultName != "rollback_5338.sql" {
			t.Fatalf("unexpected default name: %q", defaultName)
		}
		return chosen, false, nil
	}
	t.Cleanup(func() { promptSavePath = origPrompt })

	payload := wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename": "rollback_5338.sql",
		"content":  "/*alter table t*/\nrollback;\n",
		"askPath":  true,
	})
	if payload["path"] != chosen {
		t.Fatalf("unexpected path: %v", payload["path"])
	}
	data, readErr := os.ReadFile(chosen)
	if readErr != nil || string(data) != "/*alter table t*/\nrollback;\n" {
		t.Fatalf("file not written to the chosen path: %v %q", readErr, string(data))
	}

	// 对话框取消：不写文件，返回 canceled。
	promptSavePath = func(string, string) (string, bool, error) { return "", true, nil }
	payload = wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename": "rollback_5338.sql",
		"content":  "x",
		"askPath":  true,
	})
	if payload["canceled"] != true {
		t.Fatalf("expected canceled, got %v", payload)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "rollback_5338.sql")); !os.IsNotExist(statErr) {
		t.Fatal("canceled dialog must not write any file")
	}
}

// Excel 导出走 base64：xlsx 是二进制，JSON 字符串装不下原始字节。
func TestSaveTextFileBase64(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	dir := t.TempDir()
	origDownloadDir := downloadDir
	downloadDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { downloadDir = origDownloadDir })

	want := []byte{0x50, 0x4b, 0x03, 0x04}
	payload := wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename":      "query-result-20261109-153012.xlsx",
		"content":       "",
		"contentBase64": base64.StdEncoding.EncodeToString(want),
	})
	path, _ := payload["path"].(string)
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("file not written: %v", readErr)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("unexpected binary content: %v", data)
	}

	// 非法 base64 必须报参数错误，而不是落一个空文件。
	err := wfCallErr(t, service, "archery/saveTextFile", map[string]any{
		"filename":      "bad.xlsx",
		"contentBase64": "!!!",
	})
	if err.Code != -32602 {
		t.Fatalf("invalid base64: expected invalid_params, got %+v", err)
	}
}

// 另存为对话框按扩展名选过滤器：Excel 导出不能继续显示 SQL 的 *.sql。
func TestSaveDialogFilter(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		defExt string
	}{
		{"query-result-20261109-153012.xlsx", "Excel 工作簿 (*.xlsx)\x00*.xlsx\x00\x00", "xlsx"},
		{"QUERY-RESULT.XLSX", "Excel 工作簿 (*.xlsx)\x00*.xlsx\x00\x00", "xlsx"},
		{"rollback_5338.sql", "SQL (*.sql)\x00*.sql\x00\x00", "sql"},
		{"export.bin", "所有文件 (*.*)\x00*.*\x00\x00", ""},
		{"noext", "所有文件 (*.*)\x00*.*\x00\x00", ""},
	}
	for _, c := range cases {
		spec := saveDialogFilter(c.name)
		if spec.Filter != c.filter || spec.DefExt != c.defExt {
			t.Fatalf("%s: unexpected filter %+v", c.name, spec)
		}
	}
}

// 另存为的初始目录来自 UI 记住的上次目录；不可用时回退下载目录。
func TestSaveTextFileAskPathInitialDir(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	downloads := t.TempDir()
	origDownloadDir := downloadDir
	downloadDir = func() (string, error) { return downloads, nil }
	t.Cleanup(func() { downloadDir = origDownloadDir })

	chosenDir := t.TempDir()
	chosen := filepath.Join(chosenDir, "query-result.xlsx")
	var gotInitial string
	origPrompt := promptSavePath
	promptSavePath = func(_ string, initialDir string) (string, bool, error) {
		gotInitial = initialDir
		return chosen, false, nil
	}
	t.Cleanup(func() { promptSavePath = origPrompt })

	payload := wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename":      "query-result.xlsx",
		"contentBase64": base64.StdEncoding.EncodeToString([]byte{0x50, 0x4b}),
		"askPath":       true,
		"initialDir":    chosenDir,
	})
	if gotInitial != chosenDir {
		t.Fatalf("expected initial dir %q, got %q", chosenDir, gotInitial)
	}
	if payload["path"] != chosen {
		t.Fatalf("unexpected path: %v", payload["path"])
	}

	// 目录不存在：回退下载目录。
	gotInitial = ""
	wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename":   "query-result.xlsx",
		"content":    "x",
		"askPath":    true,
		"initialDir": filepath.Join(chosenDir, "no-such-dir"),
	})
	if gotInitial != downloads {
		t.Fatalf("missing dir: expected fallback %q, got %q", downloads, gotInitial)
	}

	// 指向普通文件：同样回退下载目录。
	filePath := filepath.Join(chosenDir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	gotInitial = ""
	wfCall(t, service, "archery/saveTextFile", map[string]any{
		"filename":   "query-result.xlsx",
		"content":    "x",
		"askPath":    true,
		"initialDir": filePath,
	})
	if gotInitial != downloads {
		t.Fatalf("file path: expected fallback %q, got %q", downloads, gotInitial)
	}
}
