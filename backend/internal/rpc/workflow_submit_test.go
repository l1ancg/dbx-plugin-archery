package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// 提交表单的数据源与列表页不同：
//   - 组下拉来自 /submitsql/ 的 option（value=组名，group-id=组 ID）；
//   - 实例下拉来自 /group/instances/，tag_code=can_write（「支持上线」标签）。
type submitFake struct {
	*httptest.Server

	mu        sync.Mutex
	groupForm url.Values
}

func newSubmitFake(t *testing.T) *submitFake {
	t.Helper()
	fake := &submitFake{}
	mux := http.NewServeMux()

	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<input type="hidden" name="csrfmiddlewaretoken" value="csrf">`))
	})

	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "submit-sess", Path: "/"})
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": nil})
	})

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})

	mux.HandleFunc("/submitsql/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
<select class="form-control selectpicker" id="group_name" name="group_name">
  <option value="is-empty" disabled selected>请选择组:</option>
  <option value="Test-Express" group-id="3">Test-Express</option>
  <option value="Test-Materials" group-id="7">Test-Materials</option>
</select>
<select class="form-control" id="is_backup" name="is_backup">
  <option value="True" selected>备份SQL</option>
  <option value="False">不备份SQL</option>
</select>
</body></html>`))
	})

	mux.HandleFunc("/group/instances/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.groupForm = r.PostForm
		fake.mu.Unlock()
		writeJSON(w, map[string]any{
			"status": 0,
			"msg":    "ok",
			"data": []any{
				map[string]any{"id": 12, "type": "master", "db_type": "mysql", "instance_name": "test-express-rw"},
			},
		})
	})

	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Server.Close)
	return fake
}

func (f *submitFake) GroupForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.groupForm
}

func (f *submitFake) connect(t *testing.T, readOnly bool) *Service {
	t.Helper()
	trimmed := strings.TrimPrefix(f.URL, "http://")
	parts := strings.Split(trimmed, ":")
	port := 0
	for _, char := range parts[1] {
		port = port*10 + int(char-'0')
	}
	host := parts[0]
	payload := map[string]any{
		// 与 workflow_test.go 的 wfParams 共用同一个 connectionId。
		"connectionId": "conn-wf",
		"connection": map[string]any{
			"id":       "conn-wf",
			"host":     host,
			"port":     port,
			"username": "tester",
			"password": "secret",
			"external_config": map[string]any{
				"scheme":               "http",
				"auth_mode":            "session",
				"read_only":            readOnly,
				"connect_timeout_secs": 5,
				"query_timeout_secs":   5,
			},
		},
		"runtime": map[string]any{"host": host, "port": port},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal connect params: %v", err)
	}
	service := NewService()
	if _, handleErr := service.Handle(context.Background(), "connection/connect", encoded); handleErr != nil {
		t.Fatalf("connect: %+v", handleErr)
	}
	return service
}

func TestSqlSubmitOptionsParsesGroups(t *testing.T) {
	fake := newSubmitFake(t)
	service := fake.connect(t, false)

	result, err := service.Handle(context.Background(), "archery/sqlSubmitOptions", wfParams(t, nil))
	if err != nil {
		t.Fatalf("sqlSubmitOptions: %+v", err)
	}
	encoded, _ := json.Marshal(result)
	var payload map[string]any
	_ = json.Unmarshal(encoded, &payload)

	groups := wfList(t, payload["groups"])
	if len(groups) != 2 {
		t.Fatalf("unexpected groups: %v", groups)
	}
	first := wfObject(t, groups[0])
	if wfNumber(t, first["id"]) != 3 || first["name"] != "Test-Express" {
		t.Fatalf("unexpected group option: %v", first)
	}
	if payload["backupEnabled"] != true {
		t.Fatalf("backup switch should be detected: %v", payload)
	}
}

func TestSqlWorkflowTargetsUsesCanWriteTag(t *testing.T) {
	fake := newSubmitFake(t)
	service := fake.connect(t, false)

	result, err := service.Handle(context.Background(), "archery/sqlWorkflowTargets", wfParams(t, map[string]any{
		"groupName": "Test-Express",
	}))
	if err != nil {
		t.Fatalf("sqlWorkflowTargets: %+v", err)
	}
	encoded, _ := json.Marshal(result)
	var payload map[string]any
	_ = json.Unmarshal(encoded, &payload)

	instances := wfList(t, payload["instances"])
	if len(instances) != 1 {
		t.Fatalf("unexpected instances: %v", instances)
	}
	instance := wfObject(t, instances[0])
	if wfNumber(t, instance["id"]) != 12 || instance["name"] != "test-express-rw" {
		t.Fatalf("unexpected instance: %v", instance)
	}
	form := fake.GroupForm()
	if form.Get("group_name") != "Test-Express" || form.Get("tag_code") != "can_write" {
		t.Fatalf("unexpected group instance form: %v", form)
	}

	if err := wfCallErr(t, service, "archery/sqlWorkflowTargets", map[string]any{}); err.Code != -32602 {
		t.Fatalf("expected groupName error, got %+v", err)
	}
}
