package archery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 详情页「其他信息」是单行 10 列表格，列表接口并不返回这些字段。
const workflowInfoHTML = `<html><body>
<h4>审批流</h4>
<h5><span class="text-success">TEST-Leader:Express</span></h5>
<h4>其他信息</h4>
<table class="table table-striped table-hover">
  <thead><tr>
    <th>发起人</th><th>目标实例</th><th>数据库</th><th>发起时间</th><th>可执行时间范围</th>
    <th>结束时间</th><th>备份</th><th>当前状态</th><th>组</th><th>类型</th>
  </tr></thead>
  <tbody><tr class="success">
    <td>廖思源</td><td>test-express-rw</td><td>schedule-dev</td><td>2026-11-02 10:31:10</td>
    <td>无限制</td><td>2026-11-02 10:32:00</td><td>是</td><td>已正常结束</td>
    <td>Test-Express</td><td>DDL</td>
  </tr></tbody>
</table>
</body></html>`

func TestParseInfoFields(t *testing.T) {
	fields := parseInfoFields([]byte(workflowInfoHTML))
	if len(fields) != 10 {
		t.Fatalf("expected 10 fields, got %d (%+v)", len(fields), fields)
	}
	if fields[0].Label != "发起人" || fields[0].Value != "廖思源" {
		t.Fatalf("unexpected first field: %+v", fields[0])
	}
	if fields[3].Label != "发起时间" || fields[3].Value != "2026-11-02 10:31:10" {
		t.Fatalf("unexpected create time field: %+v", fields[3])
	}
	if fields[4].Label != "可执行时间范围" || fields[4].Value != "无限制" {
		t.Fatalf("unexpected run date field: %+v", fields[4])
	}
	if fields[7].Value != "已正常结束" || fields[9].Value != "DDL" {
		t.Fatalf("unexpected status/type: %+v", fields)
	}
}

// 真实详情页在「其他信息」表格之后还跟着 SQL 列表等表格（同一区块内、没有下一个 h4）：
// 只认「其他信息」后面第一张表，后面的表头不能混进来，否则列数对不上会整块丢弃。
const workflowInfoWithTrailingTableHTML = `<html><body>
<h4>其他信息</h4>
<table class="table">
  <thead><tr><th>发起人</th><th>目标实例</th><th>数据库</th><th>发起时间</th><th>可执行时间范围</th>
  <th>结束时间</th><th>备份</th><th>当前状态</th><th>组</th><th>类型</th></tr></thead>
  <tbody><tr class="success"><td>廖思源</td><td>test-express-rw</td><td>schedule-dev</td>
  <td>2026-11-02 10:31:10</td><td>无限制</td><td>2026-11-02 10:32:00</td><td>是</td>
  <td>已正常结束</td><td>Test-Express</td><td>DDL</td></tr></tbody>
</table>
<div class="panel">
  <table class="table">
    <thead><tr><th>ID</th><th>SQL 内容</th><th>审核状态</th></tr></thead>
    <tbody><tr><td>1</td><td>use demo</td><td>通过</td></tr></tbody>
  </table>
</div>
</body></html>`

func TestParseInfoFieldsIgnoresTrailingTables(t *testing.T) {
	fields := parseInfoFields([]byte(workflowInfoWithTrailingTableHTML))
	if len(fields) != 10 {
		t.Fatalf("expected 10 fields, got %d (%+v)", len(fields), fields)
	}
	if fields[4].Label != "可执行时间范围" || fields[4].Value != "无限制" {
		t.Fatalf("unexpected run date field: %+v", fields[4])
	}
	if fields[5].Label != "结束时间" || fields[5].Value != "2026-11-02 10:32:00" {
		t.Fatalf("unexpected finish time field: %+v", fields[5])
	}
}

func TestParseInfoFieldsMissing(t *testing.T) {
	if fields := parseInfoFields([]byte(`<html><body><h4>审批流</h4></body></html>`)); len(fields) != 0 {
		t.Fatalf("expected no fields, got %+v", fields)
	}
	// 表头与数据列数不一致时宁可整块丢弃，也不返回错位的表格。
	if fields := parseInfoFields([]byte(`<h4>其他信息</h4><table><tr><th>a</th><th>b</th></tr><tr><td>1</td></tr></table>`)); len(fields) != 0 {
		t.Fatalf("expected mismatched table to be dropped, got %+v", fields)
	}
}

func TestParseRollbackRows(t *testing.T) {
	rows := parseRollbackRows([]any{
		// Archery 的 goInception 引擎返回二元数组：[执行语句, 回滚语句]，按执行顺序倒序。
		[]any{
			"CREATE TABLE `r_cte_icms_daily_summary` (`id` int)",
			"DROP TABLE `express_dev`.`r_cte_icms_daily_summary`;",
		},
		// 兼容按字段名返回的版本：sql 是执行语句，rollback_sql 才是回滚语句。
		map[string]any{
			"sql":          "alter table vehicles add column axle_number int;",
			"rollback_sql": "alter table vehicles drop column axle_number;",
		},
		// 只有回滚语句时执行语句留空，不拿回滚语句去填充。
		"drop table legacy",
		nil,
		"  ",
	})
	if len(rows) != 3 {
		t.Fatalf("expected 3 statements, got %+v", rows)
	}
	if rows[0].SQL != "CREATE TABLE `r_cte_icms_daily_summary` (`id` int)" ||
		rows[0].RollbackSQL != "DROP TABLE `express_dev`.`r_cte_icms_daily_summary`;" {
		t.Fatalf("unexpected first rollback row: %+v", rows[0])
	}
	if rows[0].ID != 1 || rows[1].ID != 2 {
		t.Fatalf("expected a running id, got %+v", rows)
	}
	if rows[1].RollbackSQL != "alter table vehicles drop column axle_number;" ||
		rows[1].SQL != "alter table vehicles add column axle_number int;" {
		t.Fatalf("rollback_sql not mapped to RollbackSQL: %+v", rows[1])
	}
	if rows[2].SQL != "" || rows[2].RollbackSQL != "drop table legacy" {
		t.Fatalf("string rows stay rollback-only: %+v", rows[2])
	}
}

func newRollbackServer(t *testing.T, payload any, status int) (*httptest.Server, *map[string]string) {
	t.Helper()
	recorded := map[string]string{}
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
	mux.HandleFunc("/sqlworkflow/backup_sql/", func(w http.ResponseWriter, r *http.Request) {
		recorded["path"] = r.URL.Path
		recorded["query"] = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &recorded
}

func TestRollbackSQL(t *testing.T) {
	server, recorded := newRollbackServer(t, map[string]any{
		"status": 0,
		"msg":    "",
		"rows": []any{
			[]any{"delete from vehicles where id = 1;", "insert into vehicles (id) values (1);"},
			[]any{"alter table vehicles add column axle_number int;", "alter table vehicles drop column axle_number;"},
		},
	}, http.StatusOK)

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

	rows, rollbackErr := client.RollbackSQL(context.Background(), 9)
	if rollbackErr != nil {
		t.Fatalf("RollbackSQL: %v", rollbackErr)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rollback pairs, got %+v", rows)
	}
	if !strings.Contains(rows[0].RollbackSQL, "insert into vehicles") ||
		!strings.Contains(rows[0].SQL, "delete from vehicles") {
		t.Fatalf("unexpected first rollback row: %+v", rows[0])
	}
	if !strings.Contains(rows[1].RollbackSQL, "drop column axle_number") {
		t.Fatalf("unexpected second rollback row: %+v", rows[1])
	}
	if (*recorded)["path"] != "/sqlworkflow/backup_sql/" {
		t.Fatalf("unexpected path: %v", *recorded)
	}
	if !strings.Contains((*recorded)["query"], "workflow_id=9") {
		t.Fatalf("workflow_id not forwarded: %v", *recorded)
	}
}

func TestRollbackSQLServerError(t *testing.T) {
	server, _ := newRollbackServer(t, map[string]any{
		"status": 1,
		"msg":    "该实例不支持回滚",
		"rows":   []any{},
	}, http.StatusOK)

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

	_, rollbackErr := client.RollbackSQL(context.Background(), 9)
	if !IsKind(rollbackErr, KindServer) || !strings.Contains(rollbackErr.Error(), "不支持回滚") {
		t.Fatalf("expected server error, got %v", rollbackErr)
	}
}
