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

// workflowFake 是 SQL 上线工单链路的假 Archery：只实现本组测试用到的端点。
// 断言统一走 JSON 层（而不是 *archery.Xxx 类型），这样红测试可以先于实现编译通过。
type workflowFake struct {
	*httptest.Server

	mu           sync.Mutex
	valid        bool
	forms        map[string]url.Values
	bodies       map[string]map[string]any
	calls        map[string]int
	fail         bool
	rollbackRows []any
	// auditRows / pendingRows 覆盖两个列表接口的返回，用于构造「待我审核」交集场景。
	auditRows   []any
	pendingRows []any
}

func newWorkflowFake(t *testing.T) *workflowFake {
	t.Helper()
	fake := &workflowFake{
		forms:  map[string]url.Values{},
		bodies: map[string]map[string]any{},
		calls:  map[string]int{},
	}
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
		fake.valid = true
		fake.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "wf-sess", Path: "/"})
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": nil})
	})

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"archery": map[string]any{"version": "1.14.0"}})
	})

	mux.HandleFunc("/group/user_all_instances/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": 0, "msg": "ok", "data": []any{}})
	})

	// 工单列表页：只关心三个筛选下拉的 option（value = id，文本 = 名称）。
	mux.HandleFunc("/sqlworkflow/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
<select class="form-control" id="navStatus" name="navStatus">
  <option value="">全部</option>
  <option value="workflow_manreviewing">待审核</option>
</select>
<select class="form-control" id="instance_id" name="instance_id">
  <option value="">全部</option>
  <option value="12">test-express-rw</option>
  <option value="13">materials_test</option>
</select>
<select class="form-control" id="group_id" name="group_id">
  <option value="">全部</option>
  <option value="3">Test-Express</option>
</select>
</body></html>`))
	})

	mux.HandleFunc("/sqlworkflow_list/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/sqlworkflow_list/", r.PostForm, nil)
		fake.mu.Lock()
		pending := fake.pendingRows
		fake.mu.Unlock()
		if pending != nil {
			writeJSON(w, map[string]any{"total": len(pending), "rows": pending})
			return
		}
		writeJSON(w, map[string]any{
			"total": 2,
			"rows": []any{
				map[string]any{
					"id":                      1024,
					"workflow_name":           "库存盘点任务",
					"engineer_display":        "喻量",
					"status":                  "workflow_finish",
					"is_backup":               1,
					"create_time":             "2026-10-28 17:45:03",
					"instance__instance_name": "materials_test",
					"db_name":                 "materials_test",
					"group_name":              "Test-Materials",
					"syntax_type":             2,
					"export_format":           nil,
				},
				map[string]any{
					"id":                      1023,
					"workflow_name":           "DDL - 车辆表结构修改",
					"engineer_display":        "廖思源",
					"status":                  "workflow_abort",
					"is_backup":               0,
					"create_time":             "2026-10-27 16:28:31",
					"instance__instance_name": "test-express-rw",
					"db_name":                 "schedule-dev",
					"group_name":              "Test-Express",
					"syntax_type":             1,
					"export_format":           "",
				},
			},
		})
	})

	mux.HandleFunc("/sqlworkflow_list_audit/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/sqlworkflow_list_audit/", r.PostForm, nil)
		writeJSON(w, map[string]any{"total": 0, "rows": []any{}})
	})

	// 待办列表：Archery common/workflow.py::lists 的返回结构。
	mux.HandleFunc("/workflow/list/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/workflow/list/", r.PostForm, nil)
		fake.mu.Lock()
		audits := fake.auditRows
		fake.mu.Unlock()
		if audits != nil {
			writeJSON(w, map[string]any{"total": len(audits), "rows": audits})
			return
		}
		writeJSON(w, map[string]any{
			"total": 1,
			"rows": []any{
				map[string]any{
					"audit_id":            118,
					"workflow_type":       2,
					"workflow_title":      "DDL - 车辆表结构修改",
					"create_user_display": "廖思源",
					"create_time":         "2026-11-05 10:13:08",
					"current_status":      0,
					"audit_auth_groups":   "1,2",
					"current_audit":       "2",
					"group_name":          "Test-Express",
				},
			},
		})
	})

	// 待办详情：/workflow/<audit_id>/ 会 302 到对应工单详情页。
	mux.HandleFunc("/workflow/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.Trim(strings.TrimPrefix(r.URL.Path, "/workflow/"), "/") {
		case "118":
			http.Redirect(w, r, "/detail/5329/", http.StatusFound)
		case "117":
			http.Redirect(w, r, "/queryapplydetail/40/", http.StatusFound)
		case "116":
			http.Redirect(w, r, "/archive/9/", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/detail/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
<h3>工单名称：<span>库存盘点任务</span></h3>
<h4>审批流</h4>
<div>
  <span class="text-success">Test-Materials:Leader</span>
  <span class="glyphicon glyphicon-arrow-right"></span>
  <span class="text-danger"><u>DBA组</u><span class="text-primary">审核人A, 审核人B</span></span>
</div>
<h4>其他信息</h4>
<table><tr><th>操作信息</th><td>喻量 2026-10-28 17:45:03 提交工单</td></tr></table>
</body></html>`))
	})

	mux.HandleFunc("/sqlworkflow/detail_content/", func(w http.ResponseWriter, r *http.Request) {
		fake.record("/sqlworkflow/detail_content/", r.URL.Query(), nil)
		writeJSON(w, map[string]any{
			"rows": []any{
				map[string]any{
					"sequence":      "0_0_00000000",
					"stage":         "阶段1",
					"stagestatus":   "Audit completed",
					"errlevel":      2,
					"errormessage":  "syntax error",
					"sql":           "alter table stock add column remark varchar(64)",
					"affected_rows": 0,
					"backup_dbname": "",
					"execute_time":  "0.01",
				},
			},
		})
	})

	mux.HandleFunc("/passed/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/passed/", r.PostForm, nil)
		fake.mu.Lock()
		fail := fake.fail
		fake.mu.Unlock()
		if fail {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><h4>你无权操作当前工单！</h4></body></html>`))
			return
		}
		http.Redirect(w, r, "/detail/9/", http.StatusFound)
	})

	mux.HandleFunc("/cancel/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/cancel/", r.PostForm, nil)
		http.Redirect(w, r, "/detail/9/", http.StatusFound)
	})

	mux.HandleFunc("/execute/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/execute/", r.PostForm, nil)
		http.Redirect(w, r, "/detail/9/", http.StatusFound)
	})

	mux.HandleFunc("/timingtask/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/timingtask/", r.PostForm, nil)
		http.Redirect(w, r, "/detail/9/", http.StatusFound)
	})

	mux.HandleFunc("/alter_run_date/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/alter_run_date/", r.PostForm, nil)
		http.Redirect(w, r, "/detail/9/", http.StatusFound)
	})

	// 提交工单走 DRF：/api/v1/workflow/，请求体是 {workflow:{...}, sql_content:"..."}。
	mux.HandleFunc("/api/v1/workflow/", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		fake.record("/api/v1/workflow/", nil, payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       77,
			"workflow": map[string]any{"id": 77, "status": "workflow_manreviewing"},
		})
	})

	mux.HandleFunc("/api/v1/workflow/sqlcheck/", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		fake.record("/api/v1/workflow/sqlcheck/", nil, payload)
		writeJSON(w, map[string]any{
			"is_execute":    true,
			"is_backup":     true,
			"error_count":   1,
			"warning_count": 2,
			"rows":          []any{},
		})
	})

	// 回滚 SQL 是 goInception 的 [执行语句, 回滚语句] 二元数组，按执行顺序倒序。
	mux.HandleFunc("/sqlworkflow/backup_sql/", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		rows := fake.rollbackRows
		fake.mu.Unlock()
		if rows == nil {
			rows = []any{
				[]any{"delete from vehicles where id = 1;", "insert into vehicles (id) values (1)"},
				[]any{"alter table vehicles add column axle_number int;", "alter table vehicles drop column axle_number"},
			}
		}
		writeJSON(w, map[string]any{"status": 0, "msg": "", "rows": rows})
	})

	mux.HandleFunc("/workflow/log/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.record("/workflow/log/", r.PostForm, nil)
		writeJSON(w, map[string]any{
			"total": 1,
			"rows": []any{map[string]any{
				"operation_type_desc": "提交工单",
				"operator_display":    "喻量",
				"operation_time":      "2026-10-28 17:45:03",
				"operation_info":      "提交 SQL 上线工单",
			}},
		})
	})

	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Server.Close)
	return fake
}

func (f *workflowFake) record(path string, form url.Values, body map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[path]++
	if form != nil {
		copied := url.Values{}
		for key, values := range form {
			copied[key] = append([]string(nil), values...)
		}
		f.forms[path] = copied
	}
	if body != nil {
		f.bodies[path] = body
	}
}

func (f *workflowFake) Form(path string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forms[path]
}

func (f *workflowFake) Body(path string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[path]
}

// Calls 返回端点被请求的次数（Form/Body 只保留最后一次，无法区分是否被调用）。
func (f *workflowFake) Calls(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

func (f *workflowFake) setAuditRows(rows []any) {
	f.mu.Lock()
	f.auditRows = rows
	f.mu.Unlock()
}

func (f *workflowFake) setPendingRows(rows []any) {
	f.mu.Lock()
	f.pendingRows = rows
	f.mu.Unlock()
}

func (f *workflowFake) setFail(value bool) {
	f.mu.Lock()
	f.fail = value
	f.mu.Unlock()
}

func (f *workflowFake) setRollbackRows(rows []any) {
	f.mu.Lock()
	f.rollbackRows = rows
	f.mu.Unlock()
}

func (f *workflowFake) hostPort(t *testing.T) (string, int) {
	t.Helper()
	trimmed := strings.TrimPrefix(f.URL, "http://")
	parts := strings.Split(trimmed, ":")
	port := 0
	for _, char := range parts[1] {
		port = port*10 + int(char-'0')
	}
	return parts[0], port
}

func wfParams(t *testing.T, extra map[string]any) json.RawMessage {
	t.Helper()
	base := map[string]any{"connectionId": "conn-wf"}
	for key, value := range extra {
		base[key] = value
	}
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return encoded
}

func wfConnect(t *testing.T, fake *workflowFake, readOnly bool) *Service {
	t.Helper()
	host, port := fake.hostPort(t)
	payload := map[string]any{
		"connectionId": "conn-wf",
		"connection": map[string]any{
			"id":       "conn-wf",
			"name":     "Archery",
			"host":     host,
			"port":     port,
			"username": "tester",
			"password": "secret",
			"external_config": map[string]any{
				"scheme":               "http",
				"base_path":            "",
				"auth_mode":            "session",
				"read_only":            readOnly,
				"default_limit_num":    100,
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

// wfCall 调用 RPC 并按 JSON 解码，避免测试依赖尚未实现的 DTO 类型。
func wfCall(t *testing.T, service *Service, method string, params map[string]any) map[string]any {
	t.Helper()
	result, err := service.Handle(context.Background(), method, wfParams(t, params))
	if err != nil {
		t.Fatalf("%s: %+v", method, err)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal %s result: %v", method, marshalErr)
	}
	var decoded map[string]any
	if decodeErr := json.Unmarshal(encoded, &decoded); decodeErr != nil {
		t.Fatalf("decode %s result: %v", method, decodeErr)
	}
	return decoded
}

func wfCallErr(t *testing.T, service *Service, method string, params map[string]any) *Error {
	t.Helper()
	_, err := service.Handle(context.Background(), method, wfParams(t, params))
	if err == nil {
		t.Fatalf("%s: expected an error", method)
	}
	return err
}

func wfObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%v)", value, value)
	}
	return object
}

func wfList(t *testing.T, value any) []any {
	t.Helper()
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("expected array, got %T (%v)", value, value)
	}
	return list
}

func wfNumber(t *testing.T, value any) float64 {
	t.Helper()
	number, ok := value.(float64)
	if !ok {
		t.Fatalf("expected number, got %T (%v)", value, value)
	}
	return number
}

func TestSqlWorkflowListParams(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflows", map[string]any{
		"limit":       500,
		"offset":      -5,
		"search":      "  库存 ",
		"status":      "workflow_manreviewing",
		"instanceId":  12,
		"groupId":     3,
		"startDate":   "2026-10-01",
		"endDate":     "2026-10-31",
		"syntaxTypes": []string{"1", "2"},
	})

	page := wfObject(t, payload["page"])
	if wfNumber(t, page["total"]) != 2 {
		t.Fatalf("unexpected total: %v", page["total"])
	}
	rows := wfList(t, page["rows"])
	if len(rows) != 2 {
		t.Fatalf("unexpected rows: %v", rows)
	}
	row := wfObject(t, rows[0])
	if row["workflowName"] != "库存盘点任务" {
		t.Fatalf("workflowName = %v", row["workflowName"])
	}
	if row["engineerDisplay"] != "喻量" || row["status"] != "workflow_finish" {
		t.Fatalf("unexpected row: %v", row)
	}
	if row["instanceName"] != "materials_test" || row["dbName"] != "materials_test" {
		t.Fatalf("unexpected instance/db: %v", row)
	}
	if row["groupName"] != "Test-Materials" {
		t.Fatalf("groupName = %v", row["groupName"])
	}
	if row["isBackup"] != true {
		t.Fatalf("isBackup = %v", row["isBackup"])
	}
	if wfNumber(t, row["syntaxType"]) != 2 {
		t.Fatalf("syntaxType = %v", row["syntaxType"])
	}
	if row["createTime"] != "2026-10-28 17:45:03" {
		t.Fatalf("createTime = %v", row["createTime"])
	}

	form := fake.Form("/sqlworkflow_list/")
	if form == nil {
		t.Fatal("expected /sqlworkflow_list/ to be called")
	}
	if form.Get("limit") != "200" {
		t.Fatalf("limit should clamp to 200, got %q", form.Get("limit"))
	}
	if form.Get("offset") != "0" {
		t.Fatalf("offset should clamp to 0, got %q", form.Get("offset"))
	}
	if form.Get("search") != "库存" {
		t.Fatalf("search should be trimmed, got %q", form.Get("search"))
	}
	if form.Get("navStatus") != "workflow_manreviewing" {
		t.Fatalf("navStatus = %q", form.Get("navStatus"))
	}
	if form.Get("instance_id") != "12" || form.Get("group_id") != "3" {
		t.Fatalf("unexpected filters: %v", form)
	}
	if form.Get("start_date") != "2026-10-01" || form.Get("end_date") != "2026-10-31" {
		t.Fatalf("unexpected date range: %v", form)
	}
	if got := form["syntax_type[]"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("syntax_type[] = %v", got)
	}
}

// Archery 只在 start_date 与 end_date 同时存在时才按时间过滤。
func TestSqlWorkflowListIgnoresPartialDateRange(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/sqlWorkflows", map[string]any{"startDate": "2026-10-01"})
	form := fake.Form("/sqlworkflow_list/")
	if form == nil {
		t.Fatal("expected /sqlworkflow_list/ to be called")
	}
	if form.Get("start_date") != "" || form.Get("end_date") != "" {
		t.Fatalf("partial date range should be dropped: %v", form)
	}
}

func TestSqlWorkflowOptionsParsesFilterSelects(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowOptions", map[string]any{})

	instances := wfList(t, payload["instances"])
	if len(instances) != 2 {
		t.Fatalf("unexpected instances: %v", instances)
	}
	first := wfObject(t, instances[0])
	if wfNumber(t, first["id"]) != 12 || first["name"] != "test-express-rw" {
		t.Fatalf("unexpected instance option: %v", first)
	}

	groups := wfList(t, payload["groups"])
	if len(groups) != 1 {
		t.Fatalf("unexpected groups: %v", groups)
	}
	group := wfObject(t, groups[0])
	if wfNumber(t, group["id"]) != 3 || group["name"] != "Test-Express" {
		t.Fatalf("unexpected group option: %v", group)
	}
}

func TestSqlWorkflowDetail(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowDetail", map[string]any{"workflowId": 9})
	detail := wfObject(t, payload["detail"])

	if wfNumber(t, detail["workflowId"]) != 9 {
		t.Fatalf("workflowId = %v", detail["workflowId"])
	}
	if detail["title"] != "库存盘点任务" {
		t.Fatalf("title = %v", detail["title"])
	}
	nodes := wfList(t, detail["reviewNodes"])
	if len(nodes) != 2 {
		t.Fatalf("unexpected review nodes: %v", nodes)
	}
	if wfObject(t, nodes[0])["state"] != "passed" {
		t.Fatalf("first node should be passed: %v", nodes[0])
	}
	current := wfObject(t, nodes[1])
	if current["state"] != "current" {
		t.Fatalf("second node should be current: %v", current)
	}
	reviewers := wfList(t, current["reviewers"])
	if len(reviewers) != 2 || reviewers[0] != "审核人A" {
		t.Fatalf("unexpected reviewers: %v", reviewers)
	}
	if last, _ := detail["lastOperation"].(string); !strings.Contains(last, "提交工单") {
		t.Fatalf("lastOperation = %v", detail["lastOperation"])
	}

	rows := wfList(t, detail["reviewRows"])
	if len(rows) != 1 {
		t.Fatalf("unexpected review rows: %v", rows)
	}
	row := wfObject(t, rows[0])
	if row["stage"] != "阶段1" || row["status"] != "Audit completed" {
		t.Fatalf("unexpected review row: %v", row)
	}
	if wfNumber(t, row["errorLevel"]) != 2 || row["errorMessage"] != "syntax error" {
		t.Fatalf("unexpected error fields: %v", row)
	}
	if row["sql"] != "alter table stock add column remark varchar(64)" {
		t.Fatalf("sql = %v", row["sql"])
	}

	query := fake.Form("/sqlworkflow/detail_content/")
	if query.Get("workflow_id") != "9" {
		t.Fatalf("detail_content should be queried by workflow_id: %v", query)
	}
}

func TestSqlWorkflowAuditPass(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowAudit", map[string]any{
		"workflowId": 9,
		"action":     "pass",
		"remark":     "可以上线",
	})
	if payload["success"] != true {
		t.Fatalf("unexpected audit result: %v", payload)
	}
	form := fake.Form("/passed/")
	if form == nil {
		t.Fatal("pass should post to /passed/")
	}
	if form.Get("workflow_id") != "9" || form.Get("audit_remark") != "可以上线" {
		t.Fatalf("unexpected pass form: %v", form)
	}
}

func TestSqlWorkflowAuditReject(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/sqlWorkflowAudit", map[string]any{
		"workflowId": 9,
		"action":     "reject",
		"remark":     "语句影响面过大",
	})
	form := fake.Form("/cancel/")
	if form == nil {
		t.Fatal("reject should post to /cancel/")
	}
	if form.Get("workflow_id") != "9" || form.Get("cancel_remark") != "语句影响面过大" {
		t.Fatalf("unexpected cancel form: %v", form)
	}
}

func TestSqlWorkflowAuditValidation(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	if err := wfCallErr(t, service, "archery/sqlWorkflowAudit", map[string]any{"workflowId": 0, "action": "pass"}); err.Code != -32602 || !strings.Contains(err.Message, "workflowId") {
		t.Fatalf("expected workflowId error, got %+v", err)
	}
	if err := wfCallErr(t, service, "archery/sqlWorkflowAudit", map[string]any{"workflowId": 9, "action": "unknown"}); err.Code != -32602 || !strings.Contains(err.Message, "action") {
		t.Fatalf("expected action error, got %+v", err)
	}
	if err := wfCallErr(t, service, "archery/sqlWorkflowAudit", map[string]any{"workflowId": 9, "action": "reject", "remark": " "}); err.Code != -32602 || !strings.Contains(err.Message, "remark") {
		t.Fatalf("reject without remark should fail, got %+v", err)
	}
}

// Archery 失败时返回 200 + error.html，需要把 <h4> 文案带回 UI。
func TestSqlWorkflowAuditSurfacesErrorPage(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)
	fake.setFail(true)

	err := wfCallErr(t, service, "archery/sqlWorkflowAudit", map[string]any{
		"workflowId": 9,
		"action":     "pass",
		"remark":     "ok",
	})
	if err.Kind != "server" || !strings.Contains(err.Message, "你无权操作当前工单") {
		t.Fatalf("expected error page message, got %+v", err)
	}
}

func TestSqlWorkflowExecute(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/sqlWorkflowExecute", map[string]any{"workflowId": 9, "mode": "auto"})
	form := fake.Form("/execute/")
	if form == nil {
		t.Fatal("execute should post to /execute/")
	}
	if form.Get("workflow_id") != "9" || form.Get("mode") != "auto" {
		t.Fatalf("unexpected execute form: %v", form)
	}

	wfCall(t, service, "archery/sqlWorkflowExecute", map[string]any{"workflowId": 9, "mode": "manual"})
	if form := fake.Form("/execute/"); form.Get("mode") != "manual" {
		t.Fatalf("mode should switch to manual, got %v", form)
	}

	if err := wfCallErr(t, service, "archery/sqlWorkflowExecute", map[string]any{"workflowId": 9, "mode": "later"}); err.Code != -32602 {
		t.Fatalf("expected invalid mode error, got %+v", err)
	}
}

func TestSqlWorkflowSchedule(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/sqlWorkflowSchedule", map[string]any{
		"workflowId": 9,
		"runDate":    "2026-11-05 03:00",
	})
	form := fake.Form("/timingtask/")
	if form == nil {
		t.Fatal("schedule should post to /timingtask/")
	}
	if form.Get("workflow_id") != "9" || form.Get("run_date") != "2026-11-05 03:00" {
		t.Fatalf("unexpected timing form: %v", form)
	}

	if err := wfCallErr(t, service, "archery/sqlWorkflowSchedule", map[string]any{"workflowId": 9, "runDate": "2026/11/05 03:00"}); err.Code != -32602 {
		t.Fatalf("expected runDate format error, got %+v", err)
	}
}

func TestSqlWorkflowSubmitBuildsWorkflowJSON(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowSubmit", map[string]any{
		"title":        "库存盘点任务",
		"groupId":      3,
		"instanceId":   12,
		"dbName":       "materials_test",
		"sql":          "alter table stock add column remark varchar(64);",
		"backup":       true,
		"runDateStart": "2026-11-02 12:00",
		"runDateEnd":   "2026-11-02 14:00",
		"demandUrl":    "https://example.com/req/1",
	})
	if wfNumber(t, payload["workflowId"]) != 77 {
		t.Fatalf("unexpected workflowId: %v", payload)
	}

	body := fake.Body("/api/v1/workflow/")
	if body == nil {
		t.Fatal("submit should post to /api/v1/workflow/")
	}
	if body["sql_content"] != "alter table stock add column remark varchar(64);" {
		t.Fatalf("sql_content = %v", body["sql_content"])
	}
	workflow := wfObject(t, body["workflow"])
	if workflow["workflow_name"] != "库存盘点任务" {
		t.Fatalf("workflow_name = %v", workflow["workflow_name"])
	}
	if wfNumber(t, workflow["group_id"]) != 3 || wfNumber(t, workflow["instance"]) != 12 {
		t.Fatalf("unexpected group/instance: %v", workflow)
	}
	if workflow["db_name"] != "materials_test" || workflow["is_backup"] != true {
		t.Fatalf("unexpected db/backup: %v", workflow)
	}
	if workflow["run_date_start"] != "2026-11-02 12:00" || workflow["run_date_end"] != "2026-11-02 14:00" {
		t.Fatalf("unexpected run date: %v", workflow)
	}
	if wfNumber(t, workflow["is_offline_export"]) != 0 {
		t.Fatalf("is_offline_export = %v", workflow["is_offline_export"])
	}
	if workflow["demand_url"] != "https://example.com/req/1" {
		t.Fatalf("demand_url = %v", workflow["demand_url"])
	}
}

func TestSqlWorkflowSubmitValidation(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	base := func() map[string]any {
		return map[string]any{
			"title":      "库存盘点任务",
			"groupId":    3,
			"instanceId": 12,
			"dbName":     "materials_test",
			"sql":        "alter table stock add column remark varchar(64);",
		}
	}

	cases := []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{name: "missing title", patch: map[string]any{"title": " "}, want: "title"},
		{name: "title too long", patch: map[string]any{"title": strings.Repeat("x", 51)}, want: "title"},
		{name: "missing group", patch: map[string]any{"groupId": 0}, want: "groupId"},
		{name: "missing instance", patch: map[string]any{"instanceId": 0}, want: "instanceId"},
		{name: "missing database", patch: map[string]any{"dbName": ""}, want: "dbName"},
		{name: "missing sql", patch: map[string]any{"sql": " "}, want: "sql"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := base()
			for key, value := range tc.patch {
				params[key] = value
			}
			err := wfCallErr(t, service, "archery/sqlWorkflowSubmit", params)
			if err.Code != -32602 || !strings.Contains(err.Message, tc.want) {
				t.Fatalf("expected %q error, got %+v", tc.want, err)
			}
		})
	}

	if fake.Body("/api/v1/workflow/") != nil {
		t.Fatal("invalid submit must not reach Archery")
	}
}

// 提交回滚请求 = 用回滚语句新建 SQL 上线工单：不开启备份、不带执行时间窗。
func TestSqlWorkflowRollbackSubmitBuildsWorkflowJSON(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowRollbackSubmit", map[string]any{
		"workflowId": 9,
		"title":      "【回滚工单】原工单Id:9 ,库存盘点任务",
		"groupId":    3,
		"instanceId": 12,
		"dbName":     "materials_test",
	})
	if wfNumber(t, payload["workflowId"]) != 77 {
		t.Fatalf("unexpected workflowId: %v", payload)
	}

	body := fake.Body("/api/v1/workflow/")
	if body == nil {
		t.Fatal("rollback submit should post to /api/v1/workflow/")
	}
	// 回滚语句按 Archery 返回顺序（执行顺序倒序）拼接，先执行的语句最后回滚。
	if body["sql_content"] != "insert into vehicles (id) values (1);\n\nalter table vehicles drop column axle_number;" {
		t.Fatalf("sql_content = %v", body["sql_content"])
	}
	workflow := wfObject(t, body["workflow"])
	if workflow["workflow_name"] != "【回滚工单】原工单Id:9 ,库存盘点任务" {
		t.Fatalf("workflow_name = %v", workflow["workflow_name"])
	}
	if wfNumber(t, workflow["group_id"]) != 3 || wfNumber(t, workflow["instance"]) != 12 {
		t.Fatalf("unexpected group/instance: %v", workflow)
	}
	if workflow["db_name"] != "materials_test" {
		t.Fatalf("db_name = %v", workflow["db_name"])
	}
	if workflow["is_backup"] != false {
		t.Fatalf("rollback ticket must not enable backup: %v", workflow["is_backup"])
	}
}

// 回滚语句带着 goInception 补的 `原工单库名`.`表` 前缀：新工单已经在自己的库上执行，
// 前缀既冗余又会在换库/换实例时打错库，提交前按 dbName 剥掉；别的库引用与字符串字面量必须保留。
func TestSqlWorkflowRollbackSubmitStripsDatabaseQualifier(t *testing.T) {
	fake := newWorkflowFake(t)
	fake.setRollbackRows([]any{
		[]any{
			"alter table stock add column remark varchar(64);",
			"ALTER TABLE `materials_test`.`stock` DROP COLUMN `remark`;",
		},
		[]any{
			"delete from stock where id = 1;",
			"insert into materials_test.stock (id) values (1);",
		},
		[]any{
			"delete from material_ledger.stock where id = 2;",
			"insert into material_ledger.stock (id) values (2);",
		},
		[]any{
			"delete from stock where note = 'materials_test.x';",
			"insert into stock (note) values ('materials_test.x');",
		},
	})
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/sqlWorkflowRollbackSubmit", map[string]any{
		"workflowId": 9,
		"title":      "【回滚工单】原工单Id:9 ,库存盘点任务",
		"groupId":    3,
		"instanceId": 12,
		"dbName":     "materials_test",
	})

	body := fake.Body("/api/v1/workflow/")
	if body == nil {
		t.Fatal("rollback submit should post to /api/v1/workflow/")
	}
	want := "ALTER TABLE `stock` DROP COLUMN `remark`;\n\n" +
		"insert into stock (id) values (1);\n\n" +
		"insert into material_ledger.stock (id) values (2);\n\n" +
		"insert into stock (note) values ('materials_test.x');"
	if body["sql_content"] != want {
		t.Fatalf("sql_content = %v", body["sql_content"])
	}
}

func TestSqlWorkflowRollbackSubmitValidation(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	base := func() map[string]any {
		return map[string]any{
			"workflowId": 9,
			"title":      "【回滚工单】原工单Id:9 ,库存盘点任务",
			"groupId":    3,
			"instanceId": 12,
			"dbName":     "materials_test",
		}
	}

	cases := []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{name: "missing workflow", patch: map[string]any{"workflowId": 0}, want: "workflowId"},
		{name: "missing title", patch: map[string]any{"title": " "}, want: "title"},
		{name: "missing group", patch: map[string]any{"groupId": 0}, want: "groupId"},
		{name: "missing instance", patch: map[string]any{"instanceId": 0}, want: "instanceId"},
		{name: "missing database", patch: map[string]any{"dbName": ""}, want: "dbName"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := base()
			for key, value := range tc.patch {
				params[key] = value
			}
			err := wfCallErr(t, service, "archery/sqlWorkflowRollbackSubmit", params)
			if err.Code != -32602 || !strings.Contains(err.Message, tc.want) {
				t.Fatalf("expected %q error, got %+v", tc.want, err)
			}
		})
	}

	// 没有可用的回滚语句时报业务错误，而不是提交一张空工单。
	fake.setRollbackRows([]any{})
	err := wfCallErr(t, service, "archery/sqlWorkflowRollbackSubmit", base())
	if err.Kind != "server" || !strings.Contains(err.Message, "没有可用的回滚语句") {
		t.Fatalf("expected empty-rollback server error, got %+v", err)
	}
	if fake.Body("/api/v1/workflow/") != nil {
		t.Fatal("empty rollback must not reach Archery")
	}
}

func TestSqlWorkflowCheck(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowCheck", map[string]any{
		"instanceId": 12,
		"dbName":     "materials_test",
		"sql":        "alter table stock add column remark varchar(64);",
	})
	if wfNumber(t, payload["errorCount"]) != 1 || wfNumber(t, payload["warningCount"]) != 2 {
		t.Fatalf("unexpected check summary: %v", payload)
	}

	body := fake.Body("/api/v1/workflow/sqlcheck/")
	if body == nil {
		t.Fatal("check should post to /api/v1/workflow/sqlcheck/")
	}
	// v1.14 的模板发 instance_id，DRF 序列化器读 instance；两个键都带，兼容两种实现。
	if wfNumber(t, body["instance"]) != 12 {
		t.Fatalf("instance = %v", body["instance"])
	}
	if body["db_name"] != "materials_test" || body["full_sql"] == "" {
		t.Fatalf("unexpected check body: %v", body)
	}

	if err := wfCallErr(t, service, "archery/sqlWorkflowCheck", map[string]any{"instanceId": 12, "sql": "x"}); err.Code != -32602 {
		t.Fatalf("expected dbName error, got %+v", err)
	}
}

// SQL 上线工单日志是 workflow_type=2；现有实现写死 1，需要参数化。
func TestWorkflowLogAcceptsWorkflowType(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/workflowLog", map[string]any{
		"workflowId":   9,
		"workflowType": 2,
	})
	page := wfObject(t, payload["page"])
	if wfNumber(t, page["total"]) != 1 {
		t.Fatalf("unexpected log page: %v", page)
	}
	row := wfObject(t, wfList(t, page["rows"])[0])
	if row["operationType"] != "提交工单" || row["operator"] != "喻量" {
		t.Fatalf("unexpected log row: %v", row)
	}

	form := fake.Form("/workflow/log/")
	if form.Get("workflow_type") != "2" {
		t.Fatalf("workflow_type should follow the request, got %v", form)
	}
}

// 只读连接必须拦住会真正改库的动作；列表、详情、SQL 检测读取不受影响。
func TestSqlWorkflowWritesBlockedInReadOnly(t *testing.T) {
	writes := []struct {
		method string
		params map[string]any
	}{
		{"archery/sqlWorkflowSubmit", map[string]any{"title": "t", "groupId": 3, "instanceId": 12, "dbName": "d", "sql": "alter table t add c int;"}},
		{"archery/sqlWorkflowRollbackSubmit", map[string]any{"workflowId": 9, "title": "回滚", "groupId": 3, "instanceId": 12, "dbName": "d"}},
		{"archery/sqlWorkflowAudit", map[string]any{"workflowId": 9, "action": "pass"}},
		{"archery/sqlWorkflowExecute", map[string]any{"workflowId": 9, "mode": "auto"}},
		{"archery/sqlWorkflowSchedule", map[string]any{"workflowId": 9, "runDate": "2026-11-05 03:00"}},
		{"archery/sqlWorkflowAlterRunDate", map[string]any{"workflowId": 9, "runDateStart": "2026-11-06 09:00", "runDateEnd": "2026-11-06 18:00"}},
	}

	for _, item := range writes {
		t.Run(item.method, func(t *testing.T) {
			fake := newWorkflowFake(t)
			service := wfConnect(t, fake, true)
			err := wfCallErr(t, service, item.method, item.params)
			if err.Kind != "read_only" {
				t.Fatalf("expected read_only, got %+v", err)
			}
		})
	}

	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, true)
	if _, err := service.Handle(context.Background(), "archery/sqlWorkflows", wfParams(t, map[string]any{"limit": 20})); err != nil {
		t.Fatalf("listing must stay available in read-only mode: %+v", err)
	}
	if _, err := service.Handle(context.Background(), "archery/sqlWorkflowDetail", wfParams(t, map[string]any{"workflowId": 9})); err != nil {
		t.Fatalf("detail must stay available in read-only mode: %+v", err)
	}
	if _, err := service.Handle(context.Background(), "archery/sqlWorkflowCheck", wfParams(t, map[string]any{"instanceId": 12, "dbName": "d", "sql": "alter table t add c int;"})); err != nil {
		t.Fatalf("sql check must stay available in read-only mode: %+v", err)
	}
}

// 审核人修改可执行时间：POST /alter_run_date/，两端留空表示清空为「无限制」。
func TestSqlWorkflowAlterRunDate(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/sqlWorkflowAlterRunDate", map[string]any{
		"workflowId":   9,
		"runDateStart": "2026-11-06 09:00",
		"runDateEnd":   "2026-11-06 18:00",
	})
	if payload["success"] != true {
		t.Fatalf("unexpected result: %v", payload)
	}
	form := fake.Form("/alter_run_date/")
	if form == nil {
		t.Fatal("altering the run date should post to /alter_run_date/")
	}
	if form.Get("workflow_id") != "9" {
		t.Fatalf("workflow_id = %q", form.Get("workflow_id"))
	}
	if form.Get("run_date_start") != "2026-11-06 09:00" || form.Get("run_date_end") != "2026-11-06 18:00" {
		t.Fatalf("unexpected run date form: %v", form)
	}

	// 两端都留空 = 清空窗口：Archery 落库为 None，详情页展示「无限制」。
	clearing := newWorkflowFake(t)
	clearingService := wfConnect(t, clearing, false)
	wfCall(t, clearingService, "archery/sqlWorkflowAlterRunDate", map[string]any{"workflowId": 9})
	cleared := clearing.Form("/alter_run_date/")
	if cleared == nil || cleared.Get("run_date_start") != "" || cleared.Get("run_date_end") != "" {
		t.Fatalf("empty window should clear both fields, got %v", cleared)
	}
}

func TestSqlWorkflowAlterRunDateValidation(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	cases := []struct {
		name   string
		params map[string]any
	}{
		{"missing workflow", map[string]any{"runDateStart": "2026-11-06 09:00", "runDateEnd": "2026-11-06 18:00"}},
		{"bad format", map[string]any{"workflowId": 9, "runDateStart": "2026/11/06 09:00", "runDateEnd": "2026/11/06 18:00"}},
		{"only start", map[string]any{"workflowId": 9, "runDateStart": "2026-11-06 09:00"}},
		{"only end", map[string]any{"workflowId": 9, "runDateEnd": "2026-11-06 18:00"}},
		{"reversed", map[string]any{"workflowId": 9, "runDateStart": "2026-11-06 18:00", "runDateEnd": "2026-11-06 09:00"}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if err := wfCallErr(t, service, "archery/sqlWorkflowAlterRunDate", item.params); err.Code != -32602 {
				t.Fatalf("expected invalid params, got %+v", err)
			}
		})
	}
	if fake.Form("/alter_run_date/") != nil {
		t.Fatal("invalid input must not reach Archery")
	}
}
