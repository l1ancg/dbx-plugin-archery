package rpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nixing178/dbx-plugin-archery/internal/archery"
)

func TestDictTablesReturnsComments(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
		dictionaryData: map[string]any{
			"b": []any{[]any{"customers", "客户信息"}},
		},
	})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	result, rpcErr := service.Handle(ctx, "archery/dictTables", json.RawMessage(
		`{"connectionId":"conn-1","instanceName":"demo-mysql","dbName":"demo_schedule","dbType":"mysql"}`,
	))
	if rpcErr != nil {
		t.Fatalf("dictTables: %+v", rpcErr)
	}
	payload := result.(map[string]any)
	if payload["commentAvailable"] != true {
		t.Fatalf("commentAvailable = %v", payload["commentAvailable"])
	}
	tables := payload["tables"].([]archery.DictionaryTable)
	if len(tables) != 1 || tables[0].Name != "customers" || tables[0].Comment != "客户信息" {
		t.Fatalf("tables = %+v", tables)
	}
}

func TestTableMetaReturnsSections(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
		tableInfoData: map[string]any{
			"meta_data": map[string]any{
				"column_list": []any{"table_name", "table_comment"},
				"rows":        []any{"teams", "班组信息表"},
			},
			"desc": map[string]any{
				"column_list": []any{"列名"},
				"rows":        []any{[]any{"id"}},
			},
			"index": map[string]any{
				"column_list": []any{"索引名"},
				"rows":        []any{[]any{"PRIMARY"}},
			},
			"create_sql": []any{[]any{"teams", "CREATE TABLE `teams` (`id` bigint)"}},
		},
	})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	result, rpcErr := service.Handle(ctx, "archery/tableMeta", json.RawMessage(
		`{"connectionId":"conn-1","instanceName":"demo-mysql","dbName":"demo_schedule","tbName":"teams","dbType":"mysql"}`,
	))
	if rpcErr != nil {
		t.Fatalf("tableMeta: %+v", rpcErr)
	}
	meta := result.(map[string]any)["meta"].(*archery.TableMetadata)
	if len(meta.Meta.Rows) != 1 || len(meta.Meta.Columns) != 2 {
		t.Fatalf("meta = %+v", meta.Meta)
	}
	if len(meta.Desc.Rows) != 1 || len(meta.Index.Rows) != 1 {
		t.Fatalf("desc/index = %+v / %+v", meta.Desc, meta.Index)
	}
	if !strings.Contains(meta.CreateSQL, "CREATE TABLE") {
		t.Fatalf("createSql = %q", meta.CreateSQL)
	}
}

func TestDictTablesRequiresInstanceAndDatabase(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}
	_, err := service.Handle(ctx, "archery/dictTables", json.RawMessage(
		`{"connectionId":"conn-1","instanceName":"demo-mysql"}`,
	))
	if err == nil || err.Code != -32602 {
		t.Fatalf("expected invalid params, got %+v", err)
	}
}

func TestUnknownMethod(t *testing.T) {
	service := NewService()
	_, err := service.Handle(context.Background(), "archery/nope", nil)
	if err == nil || err.Code != -32601 {
		t.Fatalf("expected -32601, got %+v", err)
	}
}

func TestConnectRequiresConnectionID(t *testing.T) {
	service := NewService()
	_, err := service.Handle(context.Background(), "connection/connect", json.RawMessage(`{}`))
	if err == nil || err.Code != -32602 {
		t.Fatalf("expected invalid params, got %+v", err)
	}
}

func TestNotConnected(t *testing.T) {
	service := NewService()
	_, err := service.Handle(context.Background(), "archery/instances", simpleParams(t, nil))
	if err == nil || err.Kind != "not_connected" {
		t.Fatalf("expected not_connected, got %+v", err)
	}
}

func TestConnectOverviewInstances(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{
			map[string]any{"id": 1, "instance_name": "demo-mysql", "type": "master", "db_type": "mysql"},
			map[string]any{"id": 2, "instance_name": "demo-pg", "type": "master", "db_type": "pgsql"},
		},
	})
	service := NewService()
	params := connectParams(t, fake, nil)

	result, err := service.Handle(context.Background(), "connection/connect", params)
	if err != nil {
		t.Fatalf("connect: %+v", err)
	}
	connected := result.(map[string]any)
	if connected["success"] != true {
		t.Fatalf("connect result = %+v", connected)
	}
	if connected["version"] != "1.14.0" {
		t.Fatalf("version = %v", connected["version"])
	}
	if connected["pendingTwofa"] != false {
		t.Fatalf("pendingTwofa = %v", connected["pendingTwofa"])
	}

	overview, err := service.Handle(context.Background(), "archery/overview", simpleParams(t, nil))
	if err != nil {
		t.Fatalf("overview: %+v", err)
	}
	info := overview.(map[string]any)
	if info["instanceCount"] != 2 || info["username"] != "tester" || info["authMode"] != "session" {
		t.Fatalf("overview = %+v", info)
	}

	instances, err := service.Handle(context.Background(), "archery/instances", simpleParams(t, nil))
	if err != nil {
		t.Fatalf("instances: %+v", err)
	}
	list := instances.(map[string]any)["instances"].([]archery.Instance)
	if len(list) != 2 || list[0].Name != "demo-mysql" {
		t.Fatalf("instances = %+v", list)
	}
}

// DBX 生命周期回调只带 connection.id，不带顶层 connectionId；
// 工作台的 invoke 才带 connectionId。两种都必须能连上。
func TestConnectWithoutTopLevelConnectionID(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()

	params := connectParams(t, fake, func(payload map[string]any) {
		delete(payload, "connectionId")
	})
	result, err := service.Handle(ctx, "connection/connect", params)
	if err != nil {
		t.Fatalf("connect without connectionId: %+v", err)
	}
	if result.(map[string]any)["connectionId"] != "conn-1" {
		t.Fatalf("connect result missing connectionId: %+v", result)
	}

	listParams, _ := json.Marshal(map[string]any{
		"connection": map[string]any{"id": "conn-1"},
	})
	list, rpcErr := service.Handle(ctx, "archery/instances", listParams)
	if rpcErr != nil {
		t.Fatalf("instances with connection.id only: %+v", rpcErr)
	}
	if len(list.(map[string]any)["instances"].([]archery.Instance)) != 1 {
		t.Fatalf("unexpected instances: %+v", list)
	}
}

// 宿主 context 缺失 connectionId 时，UI 的 invoke 只能靠「唯一会话」兜底。
func TestInvokeWithoutConnectionIDUsesSingleSession(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()

	hostParams := connectParams(t, fake, func(payload map[string]any) {
		delete(payload, "connectionId")
	})
	if _, err := service.Handle(ctx, "connection/connect", hostParams); err != nil {
		t.Fatalf("host connect: %+v", err)
	}

	// UI 的 connect 不带任何 id
	if _, err := service.Handle(ctx, "connection/connect", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("ui connect without id: %+v", err)
	}
	if _, err := service.Handle(ctx, "archery/instances", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("instances without id: %+v", err)
	}
}

func TestInvokeWithoutConnectionIDAmbiguous(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{instances: []any{}})
	service := NewService()
	ctx := context.Background()

	for _, id := range []string{"conn-1", "conn-2"} {
		params := connectParams(t, fake, func(payload map[string]any) {
			payload["connectionId"] = id
			payload["connection"].(map[string]any)["id"] = id
		})
		if _, err := service.Handle(ctx, "connection/connect", params); err != nil {
			t.Fatalf("connect %s: %+v", id, err)
		}
	}

	_, err := service.Handle(ctx, "archery/instances", json.RawMessage(`{}`))
	if err == nil || err.Kind != "not_connected" {
		t.Fatalf("expected not_connected for ambiguous sessions, got %+v", err)
	}
}

func TestQueryLimitRules(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	query := func(limit int) *archery.QueryResult {
		t.Helper()
		params := simpleParams(t, map[string]any{
			"instanceName": "prod",
			"dbName":       "shop",
			"sqlContent":   "select 1",
			"limitNum":     limit,
		})
		raw, err := service.Handle(ctx, "archery/query", params)
		if err != nil {
			t.Fatalf("query(%d): %+v", limit, err)
		}
		return raw.(map[string]any)["result"].(*archery.QueryResult)
	}

	query(0)
	if got := fake.queryForm.Get("limit_num"); got != "100" {
		t.Fatalf("default limit = %q, want 100", got)
	}
	query(-1)
	if got := fake.queryForm.Get("limit_num"); got != "0" {
		t.Fatalf("unlimited limit = %q, want 0", got)
	}
	result := query(500)
	if got := fake.queryForm.Get("limit_num"); got != "500" {
		t.Fatalf("explicit limit = %q, want 500", got)
	}
	if result.RowCount != 1 || result.ElapsedMS != 20 {
		t.Fatalf("result = %+v", result)
	}
}

func TestReadOnlyGuard(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := NewService()
	ctx := context.Background()
	params := connectParams(t, fake, func(payload map[string]any) {
		config := payload["connection"].(map[string]any)["external_config"].(map[string]any)
		config["read_only"] = true
	})
	result, err := service.Handle(ctx, "connection/connect", params)
	if err != nil {
		t.Fatalf("connect: %+v", err)
	}
	if result.(map[string]any)["readOnly"] != true {
		t.Fatalf("readOnly not propagated")
	}

	_, err = service.Handle(ctx, "archery/workflowSubmit", simpleParams(t, nil))
	if err == nil || err.Kind != "read_only" {
		t.Fatalf("expected read_only, got %+v", err)
	}
}

func TestTwoFAConnectPending(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		twoFA:     true,
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()

	result, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil))
	if err != nil {
		t.Fatalf("connect: %+v", err)
	}
	if result.(map[string]any)["pendingTwofa"] != true {
		t.Fatalf("expected pendingTwofa, got %+v", result)
	}
	if result.(map[string]any)["twofaType"] != "totp" {
		t.Fatalf("expected twofaType totp, got %+v", result)
	}
	if result.(map[string]any)["username"] != "tester" {
		t.Fatalf("expected username in connect result, got %+v", result)
	}

	_, err = service.Handle(ctx, "archery/instances", simpleParams(t, nil))
	if err == nil || err.Kind != "twofa_required" {
		t.Fatalf("expected twofa_required, got %+v", err)
	}

	otpParams := connectParams(t, fake, func(payload map[string]any) {
		payload["otp"] = "123456"
		payload["otpType"] = "totp"
	})
	if _, err := service.Handle(ctx, "connection/connect", otpParams); err != nil {
		t.Fatalf("connect with otp: %+v", err)
	}
	if _, err := service.Handle(ctx, "archery/instances", simpleParams(t, nil)); err != nil {
		t.Fatalf("instances after otp: %+v", err)
	}
}

// uiStateParams 构造一条带连接环境（host / port）的 uiState 调用参数：
// 真实工作台只下发 connectionId，这里显式带上连接配置，等价于 sidecar 已知的会话地址。
func uiStateParams(t *testing.T, connectionID, host string, state map[string]any) json.RawMessage {
	t.Helper()
	payload := map[string]any{
		"connectionId": connectionID,
		"connection": map[string]any{
			"id":              connectionID,
			"host":            host,
			"port":            443,
			"external_config": map[string]any{"scheme": "https", "base_path": "/archery"},
		},
	}
	if state != nil {
		payload["state"] = state
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return encoded
}

// uiStateSQL 读出 uiState/get 结果里第一条页签的 SQL；没有状态时返回空串。
func uiStateSQL(t *testing.T, result any) string {
	t.Helper()
	state, _ := result.(map[string]any)["state"].(map[string]any)
	if state == nil {
		return ""
	}
	tabs, _ := state["tabs"].([]any)
	if len(tabs) == 0 {
		return ""
	}
	row, _ := tabs[0].(map[string]any)
	sql, _ := row["sql"].(string)
	return sql
}

func TestUIStateRoundTrip(t *testing.T) {
	t.Setenv("DBX_PLUGIN_DATA_DIR", t.TempDir())
	service := NewService()
	ctx := context.Background()
	host := "archery-gz.example.com"

	empty, err := service.Handle(ctx, "archery/uiState/get", uiStateParams(t, "conn-1", host, nil))
	if err != nil {
		t.Fatalf("get empty: %+v", err)
	}
	if empty.(map[string]any)["state"] != nil {
		t.Fatalf("expected nil state, got %+v", empty)
	}

	state := map[string]any{"tabs": []any{map[string]any{"sql": "select 1"}}, "lastInstance": "prod"}
	if _, err := service.Handle(ctx, "archery/uiState/set", uiStateParams(t, "conn-1", host, state)); err != nil {
		t.Fatalf("set: %+v", err)
	}

	restored, err := service.Handle(ctx, "archery/uiState/get", uiStateParams(t, "conn-1", host, nil))
	if err != nil {
		t.Fatalf("get: %+v", err)
	}
	if got := uiStateSQL(t, restored); got != "select 1" {
		t.Fatalf("state = %+v", restored)
	}
}

// 不同 Archery 环境（baseURL 不同）的连接各存一份 UI 状态：
// 打开巴西环境时不能看到广州环境留下的页签与 SQL。
func TestUIStateScopedByEnvironment(t *testing.T) {
	t.Setenv("DBX_PLUGIN_DATA_DIR", t.TempDir())
	service := NewService()
	ctx := context.Background()

	const gzHost = "archery-gz.example.com"
	const brHost = "archery-br.example.com"

	setState := func(connectionID, host, sql string) {
		t.Helper()
		state := map[string]any{"tabs": []any{map[string]any{"sql": sql}}}
		if _, err := service.Handle(ctx, "archery/uiState/set", uiStateParams(t, connectionID, host, state)); err != nil {
			t.Fatalf("set %s: %+v", connectionID, err)
		}
	}
	readSQL := func(connectionID, host string) string {
		t.Helper()
		result, err := service.Handle(ctx, "archery/uiState/get", uiStateParams(t, connectionID, host, nil))
		if err != nil {
			t.Fatalf("get %s: %+v", connectionID, err)
		}
		return uiStateSQL(t, result)
	}

	setState("conn-gz", gzHost, "select 1")
	if got := readSQL("conn-br", brHost); got != "" {
		t.Fatalf("brazil connection restored guangzhou tabs: %q", got)
	}
	if got := readSQL("conn-gz", gzHost); got != "select 1" {
		t.Fatalf("guangzhou state = %q", got)
	}

	setState("conn-br", brHost, "select 2")
	if got := readSQL("conn-gz", gzHost); got != "select 1" {
		t.Fatalf("guangzhou state overwritten by brazil: %q", got)
	}
	if got := readSQL("conn-br", brHost); got != "select 2" {
		t.Fatalf("brazil state = %q", got)
	}
}

// 没有环境归属的调用既不读也不写全局状态：宁可空页签，也不能端出别的环境的数据。
func TestUIStateRequiresEnvironment(t *testing.T) {
	t.Setenv("DBX_PLUGIN_DATA_DIR", t.TempDir())
	service := NewService()
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]any{"state": map[string]any{"tabs": []any{}}})
	if _, err := service.Handle(ctx, "archery/uiState/set", payload); err == nil || err.Kind != "not_connected" {
		t.Fatalf("expected not_connected for unscoped set, got %+v", err)
	}

	empty, err := service.Handle(ctx, "archery/uiState/get", nil)
	if err != nil {
		t.Fatalf("get: %+v", err)
	}
	if empty.(map[string]any)["state"] != nil {
		t.Fatalf("expected nil state without environment, got %+v", empty)
	}
}

// 老版本（0.2.59 及以前）只有一份全局状态：第一个打开的环境认领它，其余环境从空白开始。
func TestUIStateAdoptsLegacyGlobalFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DBX_PLUGIN_DATA_DIR", dir)
	legacy := map[string]any{"state": map[string]any{"tabs": []any{map[string]any{"sql": "select 9"}}}}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "ui-state.json"), raw, 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	service := NewService()
	ctx := context.Background()
	readSQL := func(connectionID, host string) string {
		t.Helper()
		result, err := service.Handle(ctx, "archery/uiState/get", uiStateParams(t, connectionID, host, nil))
		if err != nil {
			t.Fatalf("get %s: %+v", connectionID, err)
		}
		return uiStateSQL(t, result)
	}

	if got := readSQL("conn-gz", "archery-gz.example.com"); got != "select 9" {
		t.Fatalf("legacy state not adopted: %q", got)
	}
	if got := readSQL("conn-br", "archery-br.example.com"); got != "" {
		t.Fatalf("legacy state leaked into another environment: %q", got)
	}
}

func TestDisconnect(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}
	if _, err := service.Handle(ctx, "connection/disconnect", simpleParams(t, nil)); err != nil {
		t.Fatalf("disconnect: %+v", err)
	}
	_, err := service.Handle(ctx, "archery/instances", simpleParams(t, nil))
	if err == nil || err.Kind != "not_connected" {
		t.Fatalf("expected not_connected after disconnect, got %+v", err)
	}
}

func TestErrorMessageCarriesKind(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{twoFA: true})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}
	_, err := service.Handle(ctx, "archery/instances", simpleParams(t, nil))
	if err == nil || !strings.HasPrefix(err.Message, "twofa_required: ") {
		t.Fatalf("message = %+v", err)
	}
}

// 会话落盘到插件数据目录后，新的 sidecar 进程（NewService）打开连接不该再要验证码。
func TestConnectRestoresPersistedSession(t *testing.T) {
	t.Setenv("DBX_PLUGIN_DATA_DIR", t.TempDir())
	fake := newRPCFake(t, rpcFakeOptions{
		twoFA:     true,
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	ctx := context.Background()

	first := NewService()
	result, err := first.Handle(ctx, "connection/connect", connectParams(t, fake, nil))
	if err != nil {
		t.Fatalf("first connect: %+v", err)
	}
	if result.(map[string]any)["pendingTwofa"] != true {
		t.Fatalf("expected pendingTwofa on first connect, got %+v", result)
	}
	otpParams := connectParams(t, fake, func(payload map[string]any) {
		payload["otp"] = "123456"
		payload["otpType"] = "totp"
	})
	if _, err := first.Handle(ctx, "connection/connect", otpParams); err != nil {
		t.Fatalf("connect with otp: %+v", err)
	}

	second := NewService()
	restored, err := second.Handle(ctx, "connection/connect", connectParams(t, fake, nil))
	if err != nil {
		t.Fatalf("second connect: %+v", err)
	}
	if restored.(map[string]any)["pendingTwofa"] != false {
		t.Fatalf("expected restored session without 2FA, got %+v", restored)
	}
	if fake.LoginCount() != 1 {
		t.Fatalf("login count = %d, want 1 after restore", fake.LoginCount())
	}
	if _, err := second.Handle(ctx, "archery/instances", simpleParams(t, nil)); err != nil {
		t.Fatalf("instances after restore: %+v", err)
	}
}
