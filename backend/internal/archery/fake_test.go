package archery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

const (
	fakeCSRF        = "csrf-token"
	fakeSession     = "sess-1"
	fakeSession2FA  = "sess-2"
	fakePending     = "tmp-sess"
	fakeAccessToken = "jwt-token"
	fakeOTP         = "123456"
)

type fakeOptions struct {
	twoFA bool
	// requireSMS 非空时只接受 auth_type=sms 且 phone 等于该值的校验请求。
	requireSMS     string
	expireAfter    int
	forbiddenQuery bool
	queryStatus    int
	queryMsg       string
	queryData      map[string]any
	instances      []any
	resourceRows   map[string][][]any
	describeData   map[string]any
	describeStatus int
	// 数据字典接口：data 是按首字母分组的 {letter: [[table, comment], ...]}。
	dictionaryData   any
	dictionaryStatus int
	// 表元数据接口：data 为 {meta_data, desc, index, create_sql}。
	tableInfoData any
	logs          map[string]any
	version       string
	noUserAll     bool

	// 权限管理相关端点。
	privilegeList         map[string]any
	userPrivilegeList     map[string]any
	privilegeGroupsHTML   string
	privilegeGroupsStatus int
	groupInstances        []any
	groupInstancesStatus  int
	groupInstancesMsg     string
	privilegeApplyStatus  int
	privilegeApplyMsg     string
	privilegeDetailHTML   string
	privilegeDetailStatus int
	auditErrorHTML        string
	auditForbidden        bool
	workflowLogs          map[string]any
	// 首页图表接口 /dashboard/api/ 返回的 chart 片段（key 为 line1/bar1 等）。
	dashboardChart map[string]string

	// 账户设置相关端点。
	twoFAState          map[string]any
	twoFAStartData      map[string]any
	twoFAStartStatus    int
	twoFAStartMsg       string
	twoFAVerifyStatus   int
	twoFAVerifyMsg      string
	twoFASaveStatus     int
	twoFASaveMsg        string
	qrPNG               []byte
	authPassword        string
	passwordPage        string
	passwordRequires    bool
	passwordSessionLost bool
	passwordStatus      int
	passwordBody        string
	passwordLocation    string
}

type fakeServer struct {
	*httptest.Server

	opts fakeOptions

	mu              sync.Mutex
	loginCount      int
	queryCount      int
	queryForm       url.Values
	describeForm    url.Values
	favoriteForm    url.Values
	logQuery        url.Values
	dashboardQuery  url.Values
	dictionaryQuery url.Values
	tableInfoQuery  url.Values
	validSession    string
	expired         bool
	instanceCalls   int
	resourceCalls   int
	jwtInstanceHit  int

	privilegeListForm  url.Values
	userPrivilegeForm  url.Values
	privilegeApplyForm url.Values
	groupInstancesForm url.Values
	auditForm          url.Values
	workflowLogForm    url.Values

	twoFAForm     url.Values
	twoFASaveForm url.Values
	passwordForm  url.Values
}

const defaultPasswordChangePage = `<html><body>
<form method="post" action="/admin/password_change/">
  <input type="hidden" name="csrfmiddlewaretoken" value="csrf-token">
  <input type="password" name="old_password">
  <input type="password" name="new_password1">
  <input type="password" name="new_password2">
</form>
</body></html>`

const defaultPrivilegeGroupsHTML = `<html><body>
<select id="group_name" name="group_name" class="selectpicker" title="请选择组:">
    <option value="Demo-Group-A">Demo-Group-A</option>
    <option value="Demo-Group-C">Demo-Group-C</option>
</select>
</body></html>`

const defaultPrivilegeDetailHTML = `<html><body>
<form action="/query/privaudit/" method="post">
    <input type="hidden" name="apply_id" value="7">
    <input type="hidden" id="audit_status" name="audit_status" value="1">
    <input type="submit" id="btnPass" class="btn btn-success" value="审核通过"/>
</form>
</body></html>`

func newFake(t *testing.T, opts fakeOptions) *fakeServer {
	t.Helper()
	fs := &fakeServer{opts: opts, validSession: fakeSession}
	if fs.opts.version == "" {
		fs.opts.version = "1.14.0"
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: fakeCSRF, Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<form><input type="hidden" name="csrfmiddlewaretoken" value="` + fakeCSRF + `"></form>`))
	})

	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-CSRFToken") != fakeCSRF || !hasCookie(r, "csrftoken", fakeCSRF) {
			http.Error(w, "CSRF verification failed", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("username") != "tester" || r.PostForm.Get("password") != "secret" {
			writeJSON(w, map[string]any{"status": 1, "msg": "用户名或密码错误，请重新输入！", "data": nil})
			return
		}
		fs.mu.Lock()
		fs.loginCount++
		fs.mu.Unlock()
		if fs.opts.twoFA {
			writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fakePending})
			return
		}
		fs.mu.Lock()
		fs.validSession = fakeSession
		fs.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: fakeSession, Path: "/"})
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": nil})
	})

	mux.HandleFunc("/api/v1/user/2fa/verify/", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "json", http.StatusBadRequest)
			return
		}
		if fs.opts.twoFAVerifyStatus != 0 {
			writeJSON(w, map[string]any{"status": fs.opts.twoFAVerifyStatus, "msg": fs.opts.twoFAVerifyMsg})
			return
		}
		if toText(payload["otp"]) != fakeOTP {
			writeJSON(w, map[string]any{"status": 1, "msg": "验证码不正确！"})
			return
		}
		if !hasCookie(r, "sessionid", fakePending) {
			// 已登录账号在插件内绑定 2FA：不再发新会话，只回报校验结果。
			if !fs.authed(r) {
				writeJSON(w, map[string]any{"status": 1, "msg": "需先校验用户密码！"})
				return
			}
			writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
			return
		}
		if fs.opts.requireSMS != "" {
			if toText(payload["auth_type"]) != "sms" || toText(payload["phone"]) != fs.opts.requireSMS {
				writeJSON(w, map[string]any{"status": 1, "msg": "短信验证码校验失败！"})
				return
			}
		}
		fs.mu.Lock()
		fs.validSession = fakeSession2FA
		fs.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: fakeSession2FA, Path: "/"})
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	mux.HandleFunc("/api/v1/user/2fa/state/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.twoFAForm = r.PostForm
		fs.mu.Unlock()
		state := fs.opts.twoFAState
		if state == nil {
			state = map[string]any{"totp": "disabled", "sms": "disabled"}
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": state})
	})

	mux.HandleFunc("/api/v1/user/2fa/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.twoFAForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.twoFAStartStatus != 0 {
			writeJSON(w, map[string]any{"status": fs.opts.twoFAStartStatus, "msg": fs.opts.twoFAStartMsg})
			return
		}
		if r.PostForm.Get("enable") == "true" {
			data := fs.opts.twoFAStartData
			if data == nil && r.PostForm.Get("auth_type") == "totp" {
				data = map[string]any{"auth_type": "totp", "key": "SECRETKEY"}
			}
			writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": data})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	mux.HandleFunc("/api/v1/user/2fa/save/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.twoFASaveForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.twoFASaveStatus != 0 {
			writeJSON(w, map[string]any{"status": fs.opts.twoFASaveStatus, "msg": fs.opts.twoFASaveMsg})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	mux.HandleFunc("/api/v1/user/auth/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		expected := fs.opts.authPassword
		if expected == "" {
			expected = "secret"
		}
		if r.PostForm.Get("password") != expected {
			writeJSON(w, map[string]any{"status": 1, "msg": "用户名或密码错误！"})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "认证成功"})
	})

	mux.HandleFunc("/user/qrcode/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		png := fs.opts.qrPNG
		if len(png) == 0 {
			png = []byte("\x89PNG\r\n\x1a\nfake")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})

	mux.HandleFunc("/admin/password_change/", func(w http.ResponseWriter, r *http.Request) {
		if fs.opts.passwordSessionLost {
			fs.mu.Lock()
			fs.validSession = ""
			fs.mu.Unlock()
			http.Redirect(w, r, "/admin/login/?next=/admin/password_change/", http.StatusFound)
			return
		}
		if fs.opts.passwordRequires {
			http.Redirect(w, r, "/admin/login/?next=/admin/password_change/", http.StatusFound)
			return
		}
		if r.Method == http.MethodGet {
			page := fs.opts.passwordPage
			if page == "" {
				page = defaultPasswordChangePage
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.passwordForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.passwordStatus != 0 {
			w.WriteHeader(fs.opts.passwordStatus)
			_, _ = w.Write([]byte("<html><body>CSRF verification failed</body></html>"))
			return
		}
		if fs.opts.passwordBody != "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(fs.opts.passwordBody))
			return
		}
		location := fs.opts.passwordLocation
		if location == "" {
			location = "/admin/password_change/done/"
		}
		http.Redirect(w, r, location, http.StatusFound)
	})

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"archery": map[string]any{"version": fs.opts.version}})
	})

	mux.HandleFunc("/dashboard/api/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		fs.mu.Lock()
		fs.dashboardQuery = r.URL.Query()
		fs.mu.Unlock()
		writeJSON(w, map[string]any{"chart": fs.opts.dashboardChart})
	})

	mux.HandleFunc("/api/auth/token/", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if toText(payload["username"]) != "tester" || toText(payload["password"]) != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"detail": "No active account found"})
			return
		}
		writeJSON(w, map[string]any{"access": fakeAccessToken, "refresh": "refresh-token"})
	})

	mux.HandleFunc("/group/user_all_instances/", func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.instanceCalls++
		fs.mu.Unlock()
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.instances})
	})

	mux.HandleFunc("/api/v1/sqlquery/instances/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeAccessToken {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"detail": "authentication required"})
			return
		}
		fs.mu.Lock()
		fs.jwtInstanceHit++
		fs.mu.Unlock()
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.instances})
	})

	mux.HandleFunc("/instance/instance_resource/", func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.resourceCalls++
		fs.mu.Unlock()
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		kind := r.URL.Query().Get("resource_type")
		rows := fs.opts.resourceRows[kind]
		data := make([]any, 0, len(rows))
		for _, row := range rows {
			items := make([]any, 0, len(row))
			items = append(items, row...)
			data = append(data, items)
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": data})
	})

	// Archery 数据字典接口：只给有 sql.menu_data_dictionary 菜单权限的账号。
	mux.HandleFunc("/data_dictionary/table_list/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		fs.mu.Lock()
		fs.dictionaryQuery = r.URL.Query()
		fs.mu.Unlock()
		if fs.opts.dictionaryStatus != 0 {
			w.WriteHeader(fs.opts.dictionaryStatus)
			_, _ = w.Write([]byte("<html><body>403 Forbidden</body></html>"))
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.dictionaryData})
	})

	mux.HandleFunc("/data_dictionary/table_info/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		fs.mu.Lock()
		fs.tableInfoQuery = r.URL.Query()
		fs.mu.Unlock()
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.tableInfoData})
	})

	mux.HandleFunc("/instance/describetable/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.describeForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.describeStatus != 0 {
			w.WriteHeader(fs.opts.describeStatus)
			writeJSON(w, map[string]any{"detail": "describe failed"})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.describeData})
	})

	mux.HandleFunc("/query/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if fs.opts.forbiddenQuery {
			w.WriteHeader(http.StatusForbidden)
			writeJSON(w, map[string]any{"detail": "无执行查询权限"})
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.queryCount++
		fs.queryForm = r.PostForm
		count := fs.queryCount
		fs.mu.Unlock()
		if fs.opts.queryStatus != 0 {
			writeJSON(w, map[string]any{"status": fs.opts.queryStatus, "msg": fs.opts.queryMsg, "data": map[string]any{}})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.queryData})
		// 只模拟一次会话过期，否则并发场景下每次查询都会再次踢掉会话。
		if fs.opts.expireAfter > 0 && count >= fs.opts.expireAfter {
			fs.mu.Lock()
			if !fs.expired {
				fs.expired = true
				fs.validSession = ""
			}
			fs.mu.Unlock()
		}
	})

	mux.HandleFunc("/api/v1/sqlquery/execute/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeAccessToken {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"detail": "authentication required"})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": fs.opts.queryData})
	})

	mux.HandleFunc("/query/querylog/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		fs.mu.Lock()
		fs.logQuery = r.URL.Query()
		fs.mu.Unlock()
		writeJSON(w, fs.opts.logs)
	})

	mux.HandleFunc("/query/favorite/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.favoriteForm = r.PostForm
		fs.mu.Unlock()
		writeJSON(w, map[string]any{"status": 0, "msg": "ok"})
	})

	// ---- 权限管理 ----

	mux.HandleFunc("/query/applylist/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.privilegeListForm = r.PostForm
		fs.mu.Unlock()
		payload := fs.opts.privilegeList
		if payload == nil {
			payload = map[string]any{"total": 0, "rows": []any{}}
		}
		writeJSON(w, payload)
	})

	// 我的权限：普通账号只能看到自己的记录，过滤由 Archery 服务端完成。
	mux.HandleFunc("/query/userprivileges/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.userPrivilegeForm = r.PostForm
		fs.mu.Unlock()
		payload := fs.opts.userPrivilegeList
		if payload == nil {
			payload = map[string]any{"total": 0, "rows": []any{}}
		}
		writeJSON(w, payload)
	})

	mux.HandleFunc("/query/applyforprivileges/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.privilegeApplyForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.privilegeApplyStatus != 0 {
			writeJSON(w, map[string]any{"status": fs.opts.privilegeApplyStatus, "msg": fs.opts.privilegeApplyMsg})
			return
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": []any{}})
	})

	mux.HandleFunc("/queryapplylist/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if fs.opts.privilegeGroupsStatus != 0 {
			w.WriteHeader(fs.opts.privilegeGroupsStatus)
			return
		}
		page := fs.opts.privilegeGroupsHTML
		if page == "" {
			page = defaultPrivilegeGroupsHTML
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})

	mux.HandleFunc("/group/instances/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.groupInstancesForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.groupInstancesStatus != 0 {
			writeJSON(w, map[string]any{"status": fs.opts.groupInstancesStatus, "msg": fs.opts.groupInstancesMsg})
			return
		}
		data := fs.opts.groupInstances
		if data == nil {
			data = []any{}
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": data})
	})

	mux.HandleFunc("/queryapplydetail/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		if fs.opts.privilegeDetailStatus != 0 {
			w.WriteHeader(fs.opts.privilegeDetailStatus)
			return
		}
		page := fs.opts.privilegeDetailHTML
		if page == "" {
			page = defaultPrivilegeDetailHTML
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})

	mux.HandleFunc("/query/privaudit/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.auditForm = r.PostForm
		fs.mu.Unlock()
		if fs.opts.auditForbidden {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if fs.opts.auditErrorHTML != "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(fs.opts.auditErrorHTML))
			return
		}
		http.Redirect(w, r, "/queryapplydetail/7/", http.StatusFound)
	})

	mux.HandleFunc("/workflow/log/", func(w http.ResponseWriter, r *http.Request) {
		if !fs.authed(r) {
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
		_ = r.ParseForm()
		fs.mu.Lock()
		fs.workflowLogForm = r.PostForm
		fs.mu.Unlock()
		payload := fs.opts.workflowLogs
		if payload == nil {
			payload = map[string]any{"total": 0, "rows": []any{}}
		}
		writeJSON(w, payload)
	})

	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Server.Close)
	return fs
}

func (fs *fakeServer) authed(r *http.Request) bool {
	cookie, err := r.Cookie("sessionid")
	if err != nil {
		return false
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.validSession != "" && cookie.Value == fs.validSession
}

func (fs *fakeServer) LoginCount() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.loginCount
}

func (fs *fakeServer) QueryCount() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.queryCount
}

func (fs *fakeServer) QueryForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.queryForm
}

func (fs *fakeServer) FavoriteForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.favoriteForm
}

func (fs *fakeServer) TableInfoQuery() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.tableInfoQuery
}

func (fs *fakeServer) DictionaryQuery() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.dictionaryQuery
}

func (fs *fakeServer) LogQueryParams() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.logQuery
}

func (fs *fakeServer) PrivilegeListForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.privilegeListForm
}

func (fs *fakeServer) UserPrivilegeForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.userPrivilegeForm
}

func (fs *fakeServer) PrivilegeApplyForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.privilegeApplyForm
}

func (fs *fakeServer) GroupInstancesForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.groupInstancesForm
}

func (fs *fakeServer) AuditForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.auditForm
}

func (fs *fakeServer) WorkflowLogForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.workflowLogForm
}

func (fs *fakeServer) TwoFAForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.twoFAForm
}

func (fs *fakeServer) TwoFASaveForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.twoFASaveForm
}

func (fs *fakeServer) PasswordForm() url.Values {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.passwordForm
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func hasCookie(r *http.Request, name, value string) bool {
	cookie, err := r.Cookie(name)
	return err == nil && cookie.Value == value
}

func newTestClient(t *testing.T, fs *fakeServer, mutate ...func(*Config)) *Client {
	t.Helper()
	cfg := Config{
		BaseURL:      fs.URL,
		Username:     "tester",
		Password:     "secret",
		Timeout:      5 * time.Second,
		QueryTimeout: 5 * time.Second,
	}
	for _, apply := range mutate {
		apply(&cfg)
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}
