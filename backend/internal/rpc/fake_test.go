package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const rpcFakeSession = "rpc-sess"

type rpcFakeOptions struct {
	twoFA     bool
	instances []any
	// dictionaryData 是 /data_dictionary/table_list/ 的 data：{letter: [[table, comment], ...]}。
	dictionaryData any
	// tableInfoData 是 /data_dictionary/table_info/ 的 data。
	tableInfoData any
	// dashboardHTML 是 /dashboard/ 返回的首页 HTML；为空时返回 500，用于验证公告降级。
	dashboardHTML string
	// twoFAState 是 /api/v1/user/2fa/state/ 返回的 data。
	twoFAState map[string]any
	// passwordRequiresStaff 模拟非 staff 账号被 Django admin 重定向到登录页。
	passwordRequiresStaff bool
}

type rpcFake struct {
	*httptest.Server

	opts rpcFakeOptions

	mu          sync.Mutex
	logins      int
	queryForm   url.Values
	privList    url.Values
	myPrivList  url.Values
	privApply   url.Values
	auditForm   url.Values
	workflowLog url.Values
	auditorsReq url.Values
	valid       bool
	pending     bool

	twoFAForm    url.Values
	passwordForm url.Values
}

func newRPCFake(t *testing.T, opts rpcFakeOptions) *rpcFake {
	t.Helper()
	fake := &rpcFake{opts: opts}
	mux := http.NewServeMux()

	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<input type="hidden" name="csrfmiddlewaretoken" value="csrf">`))
	})

	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("username") != "tester" || r.PostForm.Get("password") != "secret" {
			writeJSON(w, map[string]any{"status": 1, "msg": "用户名或密码错误，请重新输入！"})
			return
		}
		fake.mu.Lock()
		fake.logins++
		fake.pending = fake.opts.twoFA
		fake.mu.Unlock()
		if fake.opts.twoFA {
			writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": "tmp-sess"})
			return
		}
		fake.mu.Lock()
		fake.valid = true
		fake.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: rpcFakeSession, Path: "/"})
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": nil})
	})

	mux.HandleFunc("/api/v1/user/2fa/verify/", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["otp"] != "123456" {
			writeJSON(w, map[string]any{"status": 1, "msg": "验证码不正确！"})
			return
		}
		fake.mu.Lock()
		fake.valid = true
		fake.pending = false
		fake.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: rpcFakeSession, Path: "/"})
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})

	mux.HandleFunc("/dashboard/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if fake.opts.dashboardHTML == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(fake.opts.dashboardHTML))
	})

	mux.HandleFunc("/group/user_all_instances/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fake.opts.instances})
	})

	mux.HandleFunc("/data_dictionary/table_list/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fake.opts.dictionaryData})
	})

	mux.HandleFunc("/data_dictionary/table_info/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fake.opts.tableInfoData})
	})

	mux.HandleFunc("/query/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.mu.Lock()
		valid := fake.valid
		fake.queryForm = r.PostForm
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		writeJSON(w, map[string]any{
			"status": 0,
			"msg":    "ok",
			"data": map[string]any{
				"column_list": []any{"n"},
				"rows":        []any{[]any{1}},
				"query_time":  "0.02",
			},
		})
	})

	mux.HandleFunc("/query/querylog/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		writeJSON(w, map[string]any{"total": 0, "rows": []any{}})
	})

	mux.HandleFunc("/query/favorite/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	// ---- 权限管理 ----

	mux.HandleFunc("/query/applylist/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.privList = r.PostForm
		fake.mu.Unlock()
		writeJSON(w, map[string]any{
			"total": 1,
			"rows": []any{map[string]any{
				"apply_id":                7,
				"title":                   "查询数据",
				"instance__instance_name": "demo-order-rw",
				"db_list":                 "shop,report",
				"priv_type":               1,
				"table_list":              "",
				"limit_num":               1000,
				"valid_date":              "2027-10-31",
				"user_display":            "示例用户A",
				"status":                  1,
				"create_time":             "2026-10-31 10:03:36",
				"group_name":              "Demo-Group-C",
			}},
		})
	})

	mux.HandleFunc("/query/userprivileges/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.myPrivList = r.PostForm
		fake.mu.Unlock()
		writeJSON(w, map[string]any{
			"total": 1,
			"rows": []any{map[string]any{
				"privilege_id":            11,
				"user_display":            "示例用户A",
				"instance__instance_name": "demo-mysql-rw",
				"db_name":                 "demo_config",
				"priv_type":               1,
				"table_name":              "",
				"limit_num":               1000,
				"valid_date":              "2027-10-31",
			}},
		})
	})

	mux.HandleFunc("/query/applyforprivileges/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.privApply = r.PostForm
		fake.mu.Unlock()
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": []any{}})
	})

	mux.HandleFunc("/queryapplylist/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<select id="group_name"><option value="Demo-Group-C">Demo-Group-C</option></select>`))
	})

	mux.HandleFunc("/group/instances/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"status": 0,
			"msg":    "ok",
			"data": []any{
				map[string]any{"id": 3, "type": "master", "db_type": "mysql", "instance_name": "demo-order-rw"},
			},
		})
	})

	mux.HandleFunc("/queryapplydetail/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><input type="submit" id="btnPass" value="审核通过"/></body></html>`))
	})

	mux.HandleFunc("/query/privaudit/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.auditForm = r.PostForm
		fake.mu.Unlock()
		http.Redirect(w, r, "/queryapplydetail/7/", http.StatusFound)
	})

	// 审批流按 workflow_type 区分：1=查询权限工单，2=SQL 上线工单。
	mux.HandleFunc("/group/auditors/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.auditorsReq = r.PostForm
		fake.mu.Unlock()
		display := "TEST-Leader-Express->DBA"
		if r.PostForm.Get("workflow_type") == "1" {
			display = "TEST-Leader-Express"
		}
		writeJSON(w, map[string]any{
			"status": 0,
			"msg":    "ok",
			"data": map[string]any{
				"auditors":         "1,2",
				"auditors_display": display,
			},
		})
	})

	mux.HandleFunc("/workflow/log/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.workflowLog = r.PostForm
		fake.mu.Unlock()
		writeJSON(w, map[string]any{
			"total": 1,
			"rows": []any{map[string]any{
				"operation_type_desc": "提交申请",
				"operator_display":    "示例用户A",
				"operation_time":      "2026-10-31 10:03:36",
				"operation_info":      "申请查询权限",
			}},
		})
	})

	mux.HandleFunc("/api/v1/user/2fa/state/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.twoFAForm = r.PostForm
		fake.mu.Unlock()
		state := fake.opts.twoFAState
		if state == nil {
			state = map[string]any{"totp": "disabled", "sms": "disabled"}
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": state})
	})

	mux.HandleFunc("/api/v1/user/2fa/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		valid := fake.valid
		fake.mu.Unlock()
		if !valid {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.twoFAForm = r.PostForm
		fake.mu.Unlock()
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	mux.HandleFunc("/api/v1/user/auth/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("password") != "secret" {
			writeJSON(w, map[string]any{"status": 1, "msg": "用户名或密码错误！"})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "认证成功"})
	})

	mux.HandleFunc("/admin/password_change/", func(w http.ResponseWriter, r *http.Request) {
		if fake.opts.passwordRequiresStaff {
			http.Redirect(w, r, "/admin/login/?next=/admin/password_change/", http.StatusFound)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<form method="post"><input type="hidden" name="csrfmiddlewaretoken" value="csrf"></form>`))
			return
		}
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.passwordForm = r.PostForm
		fake.mu.Unlock()
		http.Redirect(w, r, "/admin/password_change/done/", http.StatusFound)
	})

	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Server.Close)
	return fake
}

func (f *rpcFake) LoginCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logins
}

func (f *rpcFake) PrivListForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.privList
}

func (f *rpcFake) MyPrivilegesForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.myPrivList
}

func (f *rpcFake) PrivApplyForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.privApply
}

func (f *rpcFake) AuditForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auditForm
}

func (f *rpcFake) AuditorsForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auditorsReq
}

func (f *rpcFake) WorkflowLogForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.workflowLog
}

func (f *rpcFake) TwoFAForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.twoFAForm
}

func (f *rpcFake) PasswordForm() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.passwordForm
}

func (f *rpcFake) hostPort(t *testing.T) (string, int) {
	t.Helper()
	trimmed := strings.TrimPrefix(f.URL, "http://")
	parts := strings.Split(trimmed, ":")
	port := 0
	for _, char := range parts[1] {
		port = port*10 + int(char-'0')
	}
	return parts[0], port
}

func connectParams(t *testing.T, fake *rpcFake, mutate func(payload map[string]any)) json.RawMessage {
	t.Helper()
	host, port := fake.hostPort(t)
	payload := map[string]any{
		"connectionId": "conn-1",
		"connection": map[string]any{
			"id":       "conn-1",
			"name":     "Archery",
			"host":     host,
			"port":     port,
			"username": "tester",
			"password": "secret",
			"external_config": map[string]any{
				"scheme":               "http",
				"base_path":            "",
				"auth_mode":            "session",
				"read_only":            false,
				"default_limit_num":    100,
				"connect_timeout_secs": 5,
				"query_timeout_secs":   5,
			},
		},
		"runtime": map[string]any{"host": host, "port": port},
	}
	if mutate != nil {
		mutate(payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return encoded
}

func simpleParams(t *testing.T, extra map[string]any) json.RawMessage {
	t.Helper()
	base := map[string]any{"connectionId": "conn-1"}
	for key, value := range extra {
		base[key] = value
	}
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return encoded
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
