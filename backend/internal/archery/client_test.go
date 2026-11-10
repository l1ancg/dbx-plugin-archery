package archery

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBuildBaseURL(t *testing.T) {
	cases := []struct {
		scheme   string
		host     string
		port     int
		basePath string
		want     string
	}{
		{"https", "archery.example.com", 443, "", "https://archery.example.com:443"},
		{"http", "10.0.0.5", 8080, "/archery/", "http://10.0.0.5:8080/archery"},
		{"https", "archery.example.com", 0, "", "https://archery.example.com"},
	}
	for _, tc := range cases {
		got, err := BuildBaseURL(tc.scheme, tc.host, tc.port, tc.basePath)
		if err != nil {
			t.Fatalf("BuildBaseURL(%v): %v", tc, err)
		}
		if got != tc.want {
			t.Fatalf("BuildBaseURL(%v) = %q, want %q", tc, got, tc.want)
		}
	}
	if _, err := BuildBaseURL("ftp", "host", 21, ""); !IsKind(err, KindInvalidConfig) {
		t.Fatalf("expected invalid config for ftp scheme, got %v", err)
	}
}

func TestLoginAndInstances(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{
			map[string]any{"id": 1, "instance_name": "demo-mysql", "type": "master", "db_type": "mysql"},
			map[string]any{"id": 2, "instance_name": "demo-pg", "type": "master", "db_type": "pgsql"},
		},
	})
	client := newTestClient(t, fs)

	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	instances, err := client.Instances(context.Background())
	if err != nil {
		t.Fatalf("Instances: %v", err)
	}
	if len(instances) != 2 || instances[0].Name != "demo-mysql" || instances[1].DBType != "pgsql" {
		t.Fatalf("unexpected instances: %+v", instances)
	}
	if fs.LoginCount() != 1 {
		t.Fatalf("login count = %d, want 1", fs.LoginCount())
	}
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession reuse: %v", err)
	}
	if fs.LoginCount() != 1 {
		t.Fatalf("session reused but login count = %d", fs.LoginCount())
	}
}

func TestLoginWrongPassword(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs, func(cfg *Config) { cfg.Password = "wrong" })

	err := client.EnsureSession(context.Background())
	if err == nil || !IsKind(err, KindAuthRequired) {
		t.Fatalf("expected auth_required, got %v", err)
	}
	if !strings.Contains(err.Error(), "用户名或密码错误") {
		t.Fatalf("expected server message, got %q", err.Error())
	}
}

func TestTwoFAFlow(t *testing.T) {
	fs := newFake(t, fakeOptions{
		twoFA:     true,
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	client := newTestClient(t, fs)

	err := client.EnsureSession(context.Background())
	if err == nil || !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected twofa_required, got %v", err)
	}

	if err := client.Login(context.Background(), OTP{Code: fakeOTP, Type: "totp"}); err != nil {
		t.Fatalf("Login with OTP: %v", err)
	}
	instances, err := client.Instances(context.Background())
	if err != nil {
		t.Fatalf("Instances after 2FA: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("unexpected instances: %+v", instances)
	}
}

func TestTwoFAWrongCode(t *testing.T) {
	fs := newFake(t, fakeOptions{twoFA: true})
	client := newTestClient(t, fs)

	if err := client.EnsureSession(context.Background()); !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected twofa_required, got %v", err)
	}
	err := client.Login(context.Background(), OTP{Code: "000000", Type: "totp"})
	if !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected twofa_required for wrong code, got %v", err)
	}
	if !strings.Contains(err.Error(), "验证码不正确") {
		t.Fatalf("expected message from server, got %q", err.Error())
	}
}

func TestTwoFAUsesConfiguredType(t *testing.T) {
	fs := newFake(t, fakeOptions{twoFA: true, requireSMS: "13800000000"})
	client := newTestClient(t, fs, func(cfg *Config) {
		cfg.TwoFAType = "sms"
		cfg.TwoFAPhone = "13800000000"
	})

	if err := client.EnsureSession(context.Background()); !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected twofa_required, got %v", err)
	}
	// 不显式传 Type/Phone，应使用连接配置里的方式。
	if err := client.Login(context.Background(), OTP{Code: fakeOTP}); err != nil {
		t.Fatalf("Login with configured sms: %v", err)
	}
}

func TestSMSTypeWithoutPhone(t *testing.T) {
	fs := newFake(t, fakeOptions{twoFA: true, requireSMS: "13800000000"})
	client := newTestClient(t, fs, func(cfg *Config) {
		cfg.TwoFAType = "sms"
	})

	if err := client.EnsureSession(context.Background()); !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected twofa_required, got %v", err)
	}
	err := client.Login(context.Background(), OTP{Code: fakeOTP})
	if !IsKind(err, KindInvalidConfig) {
		t.Fatalf("expected invalid_config for missing phone, got %v", err)
	}
}

func TestReloginAfterSessionExpiry(t *testing.T) {
	fs := newFake(t, fakeOptions{
		expireAfter: 1,
		queryData: map[string]any{
			"column_list": []any{"n"},
			"rows":        []any{[]any{json.Number("1")}},
		},
	})
	client := newTestClient(t, fs)

	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := client.Query(context.Background(), QueryRequest{InstanceName: "prod", DBName: "shop", SQL: "select 1"}); err != nil {
		t.Fatalf("first query: %v", err)
	}
	if _, err := client.Query(context.Background(), QueryRequest{InstanceName: "prod", DBName: "shop", SQL: "select 1"}); err != nil {
		t.Fatalf("second query after expiry: %v", err)
	}
	if fs.LoginCount() != 2 {
		t.Fatalf("login count = %d, want 2", fs.LoginCount())
	}
}

func TestConcurrentReloginSingleFlight(t *testing.T) {
	fs := newFake(t, fakeOptions{
		expireAfter: 1,
		queryData: map[string]any{
			"column_list": []any{"n"},
			"rows":        []any{[]any{json.Number("1")}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := client.Query(context.Background(), QueryRequest{InstanceName: "prod", DBName: "shop", SQL: "select 1"}); err != nil {
		t.Fatalf("warmup query: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = client.Query(context.Background(), QueryRequest{InstanceName: "prod", DBName: "shop", SQL: "select 1"})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("query %d failed: %v", i, err)
		}
	}
	if fs.LoginCount() != 2 {
		t.Fatalf("login count = %d, want 2 (initial + single-flight relogin)", fs.LoginCount())
	}
}

func TestQueryNormalizesObjectRows(t *testing.T) {
	fs := newFake(t, fakeOptions{
		queryData: map[string]any{
			"column_list": []any{"id", "name"},
			"column_type": []any{"int", "varchar"},
			"rows": []any{
				map[string]any{"id": json.Number("1"), "name": "alpha", "extra": "x"},
			},
			"affected_rows": json.Number("1"),
			"query_time":    "0.031",
			"is_masked":     true,
			"mask_rule_hit": false,
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	result, err := client.Query(context.Background(), QueryRequest{
		InstanceName: "prod", DBName: "shop", SQL: "select id, name from t", LimitNum: 50,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if strings.Join(result.Columns, ",") != "id,name,extra" {
		t.Fatalf("columns = %v", result.Columns)
	}
	if result.RowCount != 1 || len(result.Rows[0]) != 3 {
		t.Fatalf("rows = %+v", result.Rows)
	}
	if toText(result.Rows[0][0]) != "1" || toText(result.Rows[0][1]) != "alpha" || toText(result.Rows[0][2]) != "x" {
		t.Fatalf("row values = %+v", result.Rows[0])
	}
	if result.ElapsedMS != 31 || !result.Masked || result.AffectedRows != 1 {
		t.Fatalf("metadata = %+v", result)
	}

	form := fs.QueryForm()
	if form.Get("instance_name") != "prod" || form.Get("db_name") != "shop" || form.Get("limit_num") != "50" {
		t.Fatalf("query form = %v", form)
	}
	if form.Get("sql_content") == "" {
		t.Fatalf("sql_content missing: %v", form)
	}
}

func TestQueryServerError(t *testing.T) {
	fs := newFake(t, fakeOptions{queryStatus: 1, queryMsg: "查询异常报错，错误信息：boom"})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	_, err := client.Query(context.Background(), QueryRequest{InstanceName: "prod", DBName: "shop", SQL: "bad sql"})
	if !IsKind(err, KindServer) {
		t.Fatalf("expected server kind, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestQueryForbidden(t *testing.T) {
	fs := newFake(t, fakeOptions{forbiddenQuery: true})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	_, err := client.Query(context.Background(), QueryRequest{InstanceName: "prod", DBName: "shop", SQL: "select 1"})
	if !IsKind(err, KindForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if !strings.Contains(err.Error(), "无执行查询权限") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestResourcesAndDescribe(t *testing.T) {
	fs := newFake(t, fakeOptions{
		resourceRows: map[string][][]any{
			"database": {{"shop"}, {"crm"}},
			"table":    {{"orders", "订单表"}, {"users", "用户表"}},
		},
		describeData: map[string]any{
			"column_list": []any{"COLUMN_NAME", "COLUMN_TYPE"},
			"column_type": []any{"str", "str"},
			"rows":        []any{[]any{"id", "bigint"}, []any{"name", "varchar(64)"}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	dbs, err := client.Resources(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "database", "", "", "")
	if err != nil {
		t.Fatalf("Resources database: %v", err)
	}
	if len(dbs) != 2 || dbs[0].Name != "shop" || dbs[1].Name != "crm" {
		t.Fatalf("databases = %+v", dbs)
	}

	tables, err := client.Resources(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "table", "shop", "", "")
	if err != nil {
		t.Fatalf("Resources table: %v", err)
	}
	if len(tables) != 2 || tables[0].Name != "orders" || toText(tables[0].Values[1]) != "订单表" {
		t.Fatalf("tables = %+v", tables)
	}

	describe, err := client.Describe(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "shop", "orders", "")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(describe.Columns) != 2 || len(describe.Rows) != 2 || describe.Columns[0] != "COLUMN_NAME" {
		t.Fatalf("describe = %+v", describe)
	}
}

// instance_resource?resource_type=table 走的是 SHOW TABLES，只有表名；
// 注释必须走数据字典接口 /data_dictionary/table_list/。
func TestTableDictionaryParsesGroupedRows(t *testing.T) {
	fs := newFake(t, fakeOptions{
		dictionaryData: map[string]any{
			"b": []any{
				[]any{"customers", "客户信息"},
				[]any{"drivers", "司机信息表"},
			},
			"a": []any{[]any{"addr_geo_location", "地址服务_地理位置信息表"}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	tables, comments, err := client.TableDictionary(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "demo_schedule", "mysql")
	if err != nil {
		t.Fatalf("TableDictionary: %v", err)
	}
	if !comments {
		t.Fatalf("expected commentAvailable=true")
	}
	if len(tables) != 3 {
		t.Fatalf("tables = %+v", tables)
	}
	if tables[0].Name != "addr_geo_location" || tables[0].Comment != "地址服务_地理位置信息表" {
		t.Fatalf("first table = %+v", tables[0])
	}
	if tables[1].Name != "customers" || tables[1].Comment != "客户信息" {
		t.Fatalf("second table = %+v", tables[1])
	}

	query := fs.DictionaryQuery()
	if query.Get("instance_name") != "prod" || query.Get("db_name") != "demo_schedule" || query.Get("db_type") != "mysql" {
		t.Fatalf("dictionary query = %v", query)
	}
}

// 账号没有 sql.menu_data_dictionary 菜单权限时退回资源接口：表名可用，注释为空。
func TestTableDictionaryFallsBackWithoutMenuPermission(t *testing.T) {
	fs := newFake(t, fakeOptions{
		dictionaryStatus: http.StatusForbidden,
		resourceRows: map[string][][]any{
			"table": {{"orders"}, {"users"}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	tables, comments, err := client.TableDictionary(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "shop", "mysql")
	if err != nil {
		t.Fatalf("TableDictionary: %v", err)
	}
	if comments {
		t.Fatalf("expected commentAvailable=false")
	}
	if len(tables) != 2 || tables[0].Name != "orders" || tables[0].Comment != "" {
		t.Fatalf("tables = %+v", tables)
	}
}

// 表元数据四块：表信息（单行）、列信息、索引信息、建表语句。
func TestTableMetadataParsesSections(t *testing.T) {
	fs := newFake(t, fakeOptions{
		tableInfoData: map[string]any{
			"meta_data": map[string]any{
				"column_list": []any{"table_name", "engine", "table_comment"},
				"rows":        []any{"teams", "InnoDB", "班组信息表"},
			},
			"desc": map[string]any{
				"column_list": []any{"列名", "列类型", "列说明"},
				"rows": []any{
					[]any{"id", "bigint", "id主键"},
					[]any{"dept_code", "char(50)", "组织编码"},
				},
			},
			"index": map[string]any{
				"column_list": []any{"列名", "索引名", "唯一性"},
				"rows":        []any{[]any{"id", "PRIMARY", 0}},
			},
			"create_sql": []any{[]any{"teams", "CREATE TABLE `teams` (`id` bigint)"}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	meta, err := client.TableMetadata(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "demo_schedule", "teams", "mysql")
	if err != nil {
		t.Fatalf("TableMetadata: %v", err)
	}
	if len(meta.Meta.Columns) != 3 || len(meta.Meta.Rows) != 1 {
		t.Fatalf("meta = %+v", meta.Meta)
	}
	if toText(meta.Meta.Rows[0][2]) != "班组信息表" {
		t.Fatalf("meta row = %+v", meta.Meta.Rows[0])
	}
	if len(meta.Desc.Rows) != 2 || toText(meta.Desc.Rows[1][2]) != "组织编码" {
		t.Fatalf("desc = %+v", meta.Desc)
	}
	if len(meta.Index.Rows) != 1 || toText(meta.Index.Rows[0][1]) != "PRIMARY" {
		t.Fatalf("index = %+v", meta.Index)
	}
	if !strings.Contains(meta.CreateSQL, "CREATE TABLE") {
		t.Fatalf("createSql = %q", meta.CreateSQL)
	}

	query := fs.TableInfoQuery()
	if query.Get("tb_name") != "teams" || query.Get("db_name") != "demo_schedule" || query.Get("db_type") != "mysql" {
		t.Fatalf("table info query = %v", query)
	}
}

func TestDescribeExtractsCreateTableDDL(t *testing.T) {
	const ddl = "CREATE TABLE `addr_geo_location` (\n  `id` bigint NOT NULL COMMENT '主键',\n  `name_zh` varchar(150) NOT NULL DEFAULT '' COMMENT '中文名称'\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	fs := newFake(t, fakeOptions{
		describeData: map[string]any{
			"column_list": []any{"Table", "Create Table"},
			"rows":        []any{[]any{"addr_geo_location", ddl}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	describe, err := client.Describe(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "demo_schedule", "addr_geo_location", "")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if describe.Table != "addr_geo_location" {
		t.Fatalf("table = %q", describe.Table)
	}
	if describe.DDL != ddl {
		t.Fatalf("ddl = %q", describe.DDL)
	}
}

func TestDescribeSendsEmptySchemaName(t *testing.T) {
	fs := newFake(t, fakeOptions{
		describeData: map[string]any{
			"column_list": []any{"Table", "Create Table"},
			"rows":        []any{},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := client.Describe(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "shop", "orders", ""); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	fs.mu.Lock()
	form := fs.describeForm
	fs.mu.Unlock()
	values, ok := form["schema_name"]
	if !ok {
		t.Fatalf("schema_name 未发送: %v", form)
	}
	if len(values) != 1 || values[0] != "" {
		t.Fatalf("schema_name = %v", values)
	}
}

func TestDescribeFallsBackToShowCreateTable(t *testing.T) {
	const ddl = "CREATE TABLE `orders` (\n  `id` bigint NOT NULL\n)"
	fs := newFake(t, fakeOptions{
		describeStatus: http.StatusInternalServerError,
		queryData: map[string]any{
			"column_list": []any{"Table", "Create Table"},
			"rows":        []any{[]any{"orders", ddl}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	describe, err := client.Describe(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "shop", "orders", "")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if describe.DDL != ddl || describe.Table != "orders" {
		t.Fatalf("describe = %+v", describe)
	}

	fs.mu.Lock()
	form := fs.queryForm
	fs.mu.Unlock()
	if sql := form.Get("sql_content"); !strings.Contains(sql, "SHOW CREATE TABLE `shop`.`orders`") {
		t.Fatalf("fallback sql = %q", sql)
	}
}

func TestDescribeKeepsColumnListWhenNoDDL(t *testing.T) {
	fs := newFake(t, fakeOptions{
		describeData: map[string]any{
			"column_list": []any{"COLUMN_NAME", "COLUMN_TYPE"},
			"rows":        []any{[]any{"id", "bigint"}},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	describe, err := client.Describe(context.Background(), InstanceRef{ID: 1, Name: "prod"}, "shop", "orders", "")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if describe.DDL != "" || describe.Table != "" {
		t.Fatalf("describe = %+v", describe)
	}
	if len(describe.Rows) != 1 {
		t.Fatalf("rows = %+v", describe.Rows)
	}
}

func TestQueryLogsAndFavorite(t *testing.T) {
	fs := newFake(t, fakeOptions{
		logs: map[string]any{
			"total": json.Number("1"),
			"rows": []any{
				map[string]any{
					"id":            json.Number("9"),
					"instance_name": "prod",
					"db_name":       "shop",
					"sqllog":        "select 1",
					"effect_row":    json.Number("1"),
					"cost_time":     "0.01",
					"user_display":  "张三",
					"favorite":      true,
					"alias":         "巡检",
					"create_time":   "2026-10-30 10:00:00",
				},
			},
		},
	})
	client := newTestClient(t, fs)
	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	page, err := client.QueryLogs(context.Background(), LogQuery{
		Limit:        20,
		Search:       "select",
		Star:         true,
		InstanceName: "prod",
		DBName:       "shop",
	})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if page.Total != 1 || len(page.Rows) != 1 {
		t.Fatalf("page = %+v", page)
	}
	row := page.Rows[0]
	if row.InstanceName != "prod" || !row.Favorite || row.UserDisplay != "张三" || row.ID != 9 {
		t.Fatalf("row = %+v", row)
	}
	filters := fs.LogQueryParams()
	if filters.Get("instance_name") != "prod" || filters.Get("db_name") != "shop" {
		t.Fatalf("instance/db filters not forwarded: %v", filters)
	}
	if filters.Get("search") != "select" || filters.Get("star") != "true" {
		t.Fatalf("search/star not forwarded: %v", filters)
	}

	if err := client.Favorite(context.Background(), 9, true, "巡检"); err != nil {
		t.Fatalf("Favorite: %v", err)
	}
	form := fs.FavoriteForm()
	if form.Get("query_log_id") != "9" || form.Get("star") != "true" || form.Get("alias") != "巡检" {
		t.Fatalf("favorite form = %v", form)
	}
}

func TestVersionInfo(t *testing.T) {
	fs := newFake(t, fakeOptions{version: "1.14.0"})
	client := newTestClient(t, fs)
	version, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != "1.14.0" {
		t.Fatalf("version = %q", version)
	}
}

func TestJWTMode(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{map[string]any{"id": 3, "instance_name": "jwt-instance", "db_type": "mysql"}},
		queryData: map[string]any{
			"column_list": []any{"n"},
			"rows":        []any{[]any{json.Number("1")}},
		},
	})
	client := newTestClient(t, fs, func(cfg *Config) { cfg.AuthMode = "jwt" })

	if err := client.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession jwt: %v", err)
	}
	instances, err := client.Instances(context.Background())
	if err != nil {
		t.Fatalf("Instances jwt: %v", err)
	}
	if len(instances) != 1 || instances[0].Name != "jwt-instance" {
		t.Fatalf("instances = %+v", instances)
	}
	result, err := client.Query(context.Background(), QueryRequest{InstanceName: "jwt-instance", DBName: "shop", SQL: "select 1"})
	if err != nil {
		t.Fatalf("Query jwt: %v", err)
	}
	if result.RowCount != 1 {
		t.Fatalf("result = %+v", result)
	}
}

// 2FA 账号：验过码之后侧车重启（CookieJar 全新）应复用会话，不能再要验证码。
func TestRestoreSessionSkipsLoginAnd2FA(t *testing.T) {
	fs := newFake(t, fakeOptions{
		twoFA:     true,
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	store := NewFileSessionStore(filepath.Join(t.TempDir(), "session.json"))

	first := newTestClient(t, fs, func(cfg *Config) { cfg.SessionStore = store })
	if err := first.EnsureSession(context.Background()); !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected twofa_required first time, got %v", err)
	}
	if err := first.Login(context.Background(), OTP{Code: fakeOTP, Type: "totp"}); err != nil {
		t.Fatalf("Login with OTP: %v", err)
	}

	// 新的 Client 代表 sidecar 重启后重新打开连接：CookieJar 里没有任何 Cookie。
	second := newTestClient(t, fs, func(cfg *Config) { cfg.SessionStore = store })
	if err := second.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession after restart: %v", err)
	}
	if fs.LoginCount() != 1 {
		t.Fatalf("session restored but login count = %d, want 1", fs.LoginCount())
	}
	instances, err := second.Instances(context.Background())
	if err != nil {
		t.Fatalf("Instances after restore: %v", err)
	}
	if len(instances) != 1 || instances[0].Name != "prod" {
		t.Fatalf("instances = %+v", instances)
	}
}

// 服务端会话已过期时，恢复探测必须失败回退到密码登录，并丢弃坏快照。
func TestRestoreSessionDropsExpiredSnapshot(t *testing.T) {
	fs := newFake(t, fakeOptions{twoFA: true})
	store := NewFileSessionStore(filepath.Join(t.TempDir(), "session.json"))
	store.Save(fs.URL, "tester", map[string]string{"sessionid": "dead-session", "csrftoken": fakeCSRF})

	client := newTestClient(t, fs, func(cfg *Config) { cfg.SessionStore = store })
	if err := client.EnsureSession(context.Background()); !IsKind(err, KindTwoFARequired) {
		t.Fatalf("expected fallback twofa_required, got %v", err)
	}
	if fs.LoginCount() != 1 {
		t.Fatalf("login count = %d, want 1 after restore failure", fs.LoginCount())
	}
	if saved := store.Load(fs.URL, "tester"); len(saved) != 0 {
		t.Fatalf("expired snapshot should be dropped, got %v", saved)
	}
}
