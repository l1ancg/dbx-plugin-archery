package archery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 业务页（detail.html + base.html）既带 {% csrf_token %} 表单，
// 又带 base.html 的「确认密码」模态框，两者都不能作为登录页判据。
func TestLooksLikeLoginPage(t *testing.T) {
	businessPage := []byte(`<html><body>
<form action="/rollback/" method="get"><input type="hidden" name="csrfmiddlewaretoken" value="abc"></form>
<div class="modal"><input type="password" class="form-control" id="passwd" placeholder="确认密码" required></div>
</body></html>`)
	if looksLikeLoginPage(businessPage) {
		t.Fatal("business page with csrf form and password modal must not be treated as the login page")
	}

	// Archery v1.14 登录页的真实标记：class="login-form" + #inputUsername / #inputPassword / #btnLogin。
	loginPage := []byte(`<html><head><title>Login To Archery</title></head><body>
<div class="col-sm-4 login-form-wrapper">
<form class="login-form fade-in-effect" id="login" method="post">
  <input id="inputUsername" type="text">
  <input id="inputPassword" type="password">
  <button id="btnLogin">登录</button>
</form></div>
</body></html>`)
	if !looksLikeLoginPage(loginPage) {
		t.Fatal("Archery login page not detected")
	}

	// 只有 login-form 类名（登录页主标记）也要识别。
	if !looksLikeLoginPage([]byte(`<form class="login-form fade-in-effect" id="login"></form>`)) {
		t.Fatal("login page detected by login-form class")
	}
}

// 详情页被误判为登录页时，会多跑一次登录；修复后首屏只应登录一次。
func TestSqlWorkflowDetailDoesNotReloginOnBusinessPage(t *testing.T) {
	var authCalls int32
	mux := http.NewServeMux()

	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<input type="hidden" name="csrfmiddlewaretoken" value="csrf">`))
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&authCalls, 1)
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": nil})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})
	mux.HandleFunc("/detail/9/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
<h3>工单名称：<span>DDL - 车辆表结构修改</span></h3>
<h4>其他信息</h4>
<table><thead><tr><th>发起人</th><th>目标实例</th></tr></thead>
<tbody><tr class="success"><td>廖思源</td><td>test-express-rw</td></tr></tbody></table>
<form action="/rollback/" method="get"><input type="hidden" name="csrfmiddlewaretoken" value="abc"></form>
<div class="modal"><input type="password" id="passwd" placeholder="确认密码"></div>
</body></html>`))
	})
	mux.HandleFunc("/sqlworkflow/detail_content/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": []any{}})
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

	detail, detailErr := client.SqlWorkflowDetail(context.Background(), 9)
	if detailErr != nil {
		t.Fatalf("detail on a business page must not fail: %v", detailErr)
	}
	if len(detail.InfoFields) != 2 || detail.InfoFields[0].Value != "廖思源" {
		t.Fatalf("unexpected info fields: %+v", detail.InfoFields)
	}
	if got := atomic.LoadInt32(&authCalls); got != 1 {
		t.Fatalf("detail should not trigger a relogin, authenticate called %d times", got)
	}
}
