package archery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// 提交页选中资源组后要显示「审批流程：TEST-Leader-Express->DBA」，
// 数据来自 POST /group/auditors/（workflow_type 固定 2）。
func TestGroupAuditors(t *testing.T) {
	var form url.Values
	mux := http.NewServeMux()

	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<input type="hidden" name="csrfmiddlewaretoken" value="csrf">`))
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": nil})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})
	mux.HandleFunc("/group/auditors/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"msg":    "ok",
			"data": map[string]any{
				"auditors":         "1,2",
				"auditors_display": "TEST-Leader-Express->DBA",
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := NewClient(Config{
		BaseURL:      server.URL,
		Username:     "tester",
		Password:     "secret",
		AuthMode:     "session",
		Timeout:      5 * time.Second,
		QueryTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	display, auditors, auditorsErr := client.GroupAuditors(context.Background(), "Test-Express", WorkflowTypeSqlReview)
	if auditorsErr != nil {
		t.Fatalf("GroupAuditors: %v", auditorsErr)
	}
	if display != "TEST-Leader-Express->DBA" || auditors != "1,2" {
		t.Fatalf("unexpected auditors: display=%q auditors=%q", display, auditors)
	}
	if form.Get("group_name") != "Test-Express" || form.Get("workflow_type") != "2" {
		t.Fatalf("unexpected auditors form: %v", form)
	}
}

// 查询权限申请页选中资源组后同样要显示审批流程，
// 此时 workflow_type 必须是 1（WorkflowType.QUERY_PRIVILEGES），否则会拿到 SQL 上线审批流。
func TestGroupAuditorsQueryPriv(t *testing.T) {
	var form url.Values
	mux := http.NewServeMux()

	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<input type="hidden" name="csrfmiddlewaretoken" value="csrf">`))
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": nil})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})
	mux.HandleFunc("/group/auditors/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"msg":    "ok",
			"data": map[string]any{
				"auditors":         "1",
				"auditors_display": "TEST-Leader-Express",
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := NewClient(Config{
		BaseURL:      server.URL,
		Username:     "tester",
		Password:     "secret",
		AuthMode:     "session",
		Timeout:      5 * time.Second,
		QueryTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	display, auditors, auditorsErr := client.GroupAuditors(context.Background(), "Test-Express", WorkflowTypeQueryPriv)
	if auditorsErr != nil {
		t.Fatalf("GroupAuditors: %v", auditorsErr)
	}
	if display != "TEST-Leader-Express" || auditors != "1" {
		t.Fatalf("unexpected auditors: display=%q auditors=%q", display, auditors)
	}
	if form.Get("group_name") != "Test-Express" || form.Get("workflow_type") != "1" {
		t.Fatalf("unexpected auditors form: %v", form)
	}
}

func TestGroupAuditorsSurfacesMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<input type="hidden" name="csrfmiddlewaretoken" value="csrf">`))
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": nil})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})
	mux.HandleFunc("/group/auditors/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 1,
			"msg":    "该资源组未配置审批流",
			"data":   nil,
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := NewClient(Config{
		BaseURL:      server.URL,
		Username:     "tester",
		Password:     "secret",
		AuthMode:     "session",
		Timeout:      5 * time.Second,
		QueryTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, _, auditorsErr := client.GroupAuditors(context.Background(), "Test-Express", WorkflowTypeSqlReview)
	if !IsKind(auditorsErr, KindServer) || !strings.Contains(auditorsErr.Error(), "未配置审批流") {
		t.Fatalf("expected server error, got %v", auditorsErr)
	}
}
