package archery

import (
	"context"
	"strings"
	"testing"
)

func stringSliceEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestPrivilegeAppliesParsesRows(t *testing.T) {
	fs := newFake(t, fakeOptions{
		privilegeList: map[string]any{
			"total": 2,
			"rows": []any{
				map[string]any{
					"apply_id":                7,
					"title":                   "查询数据",
					"instance__instance_name": "demo-mysql-rw",
					"db_list":                 "demo_materials,demo_config",
					"priv_type":               1,
					"table_list":              "",
					"limit_num":               1000,
					"valid_date":              "2027-10-31",
					"user_display":            "示例用户A",
					"status":                  1,
					"create_time":             "2026-10-31 10:03:36",
					"group_name":              "Demo-Group-A",
				},
				map[string]any{
					"apply_id":                6,
					"title":                   "查看数据",
					"instance__instance_name": "demo-order-rw",
					"db_list":                 "erp",
					"priv_type":               2,
					"table_list":              "orders,order_items",
					"limit_num":               500,
					"valid_date":              "2026-06-08",
					"user_display":            "示例用户A",
					"status":                  2,
					"create_time":             "2026-06-01 09:00:00",
					"group_name":              "Demo-Group-C",
				},
			},
		},
	})
	client := newTestClient(t, fs)

	page, err := client.PrivilegeApplies(context.Background(), 20, 40, "查询")
	if err != nil {
		t.Fatalf("PrivilegeApplies: %v", err)
	}
	if page.Total != 2 || len(page.Rows) != 2 {
		t.Fatalf("unexpected page: %+v", page)
	}
	first := page.Rows[0]
	if first.ApplyID != 7 || first.InstanceName != "demo-mysql-rw" || first.PrivType != PrivilegeTypeDatabase {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.LimitNum != 1000 || first.Status != 1 || first.GroupName != "Demo-Group-A" {
		t.Fatalf("unexpected first row limits: %+v", first)
	}
	if first.CreateTime != "2026-10-31 10:03:36" || first.ValidDate != "2027-10-31" {
		t.Fatalf("unexpected first row times: %+v", first)
	}
	second := page.Rows[1]
	if second.PrivType != PrivilegeTypeTable || second.TableList != "orders,order_items" || second.Status != 2 {
		t.Fatalf("unexpected second row: %+v", second)
	}

	form := fs.PrivilegeListForm()
	if form.Get("limit") != "20" || form.Get("offset") != "40" || form.Get("search") != "查询" {
		t.Fatalf("unexpected list form: %v", form)
	}
}

func TestPrivilegeAppliesEmptyPage(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	page, err := client.PrivilegeApplies(context.Background(), 0, 0, "")
	if err != nil {
		t.Fatalf("PrivilegeApplies: %v", err)
	}
	if page == nil || page.Rows == nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("unexpected empty page: %+v", page)
	}
}

func TestPrivilegeAppliesRejectsJWTMode(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs, func(cfg *Config) { cfg.AuthMode = "jwt" })

	_, err := client.PrivilegeApplies(context.Background(), 20, 0, "")
	if !IsKind(err, KindUnsupported) {
		t.Fatalf("expected unsupported in JWT mode, got %v", err)
	}
}

func TestPrivilegeGroupsParsesSelect(t *testing.T) {
	fs := newFake(t, fakeOptions{
		privilegeGroupsHTML: `<html><body>
<select id="group_name" name="group_name" class="selectpicker" title="请选择组:">
    <option value="Demo-Group-A">Demo-Group-A</option>
    <option value="Demo-Group-C">Demo-Group-C</option>
    <option value="Demo-Group-C">Demo-Group-C</option>
    <option value="A&amp;B">A&amp;B</option>
    <option value="">空</option>
</select>
</body></html>`,
	})
	client := newTestClient(t, fs)

	groups, err := client.PrivilegeGroups(context.Background())
	if err != nil {
		t.Fatalf("PrivilegeGroups: %v", err)
	}
	want := []string{"Demo-Group-A", "Demo-Group-C", "A&B"}
	if !stringSliceEqual(groups, want) {
		t.Fatalf("groups = %v, want %v", groups, want)
	}
}

func TestPrivilegeGroupsWithoutSelect(t *testing.T) {
	fs := newFake(t, fakeOptions{privilegeGroupsHTML: `<html><body>no groups here</body></html>`})
	client := newTestClient(t, fs)

	groups, err := client.PrivilegeGroups(context.Background())
	if err != nil {
		t.Fatalf("PrivilegeGroups: %v", err)
	}
	if groups == nil || len(groups) != 0 {
		t.Fatalf("expected empty non-nil slice, got %#v", groups)
	}
}

func TestPrivilegeGroupsForbidden(t *testing.T) {
	fs := newFake(t, fakeOptions{privilegeGroupsStatus: 403})
	client := newTestClient(t, fs)

	_, err := client.PrivilegeGroups(context.Background())
	if !IsKind(err, KindForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestGroupInstancesParsesRows(t *testing.T) {
	fs := newFake(t, fakeOptions{
		groupInstances: []any{
			map[string]any{"id": 3, "type": "master", "db_type": "mysql", "instance_name": "demo-order-rw"},
			map[string]any{"id": 4, "type": "slave", "db_type": "pgsql", "instance_name": "demo-pg-ro"},
		},
	})
	client := newTestClient(t, fs)

	instances, err := client.GroupInstances(context.Background(), "Demo-Group-C")
	if err != nil {
		t.Fatalf("GroupInstances: %v", err)
	}
	if len(instances) != 2 || instances[0].ID != 3 || instances[0].Name != "demo-order-rw" || instances[0].DBType != "mysql" {
		t.Fatalf("unexpected instances: %+v", instances)
	}
	if instances[1].Type != "slave" || instances[1].DBType != "pgsql" {
		t.Fatalf("unexpected second instance: %+v", instances[1])
	}

	form := fs.GroupInstancesForm()
	if form.Get("group_name") != "Demo-Group-C" || form.Get("tag_code") != "can_read" {
		t.Fatalf("unexpected group instances form: %v", form)
	}
}

func TestGroupInstancesServerError(t *testing.T) {
	fs := newFake(t, fakeOptions{groupInstancesStatus: 1, groupInstancesMsg: "资源组不存在"})
	client := newTestClient(t, fs)

	_, err := client.GroupInstances(context.Background(), "missing-group")
	if !IsKind(err, KindServer) || !strings.Contains(err.Error(), "资源组不存在") {
		t.Fatalf("expected server error with message, got %v", err)
	}
}

func TestApplyPrivilegeEncodesTableMode(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	err := client.ApplyPrivilege(context.Background(), PrivilegeApplyInput{
		Title:        "排查线上问题",
		InstanceName: "demo-order-rw",
		GroupName:    "Demo-Group-C",
		PrivType:     PrivilegeTypeTable,
		DBName:       "erp",
		TableList:    []string{"orders", "order_items"},
		ValidDate:    "2026-11-7",
		LimitNum:     500,
	})
	if err != nil {
		t.Fatalf("ApplyPrivilege: %v", err)
	}

	form := fs.PrivilegeApplyForm()
	if form.Get("title") != "排查线上问题" || form.Get("instance_name") != "demo-order-rw" {
		t.Fatalf("unexpected apply form: %v", form)
	}
	if form.Get("group_name") != "Demo-Group-C" || form.Get("priv_type") != "2" || form.Get("db_name") != "erp" {
		t.Fatalf("unexpected apply form targets: %v", form)
	}
	if got := form["table_list[]"]; len(got) != 2 || got[0] != "orders" || got[1] != "order_items" {
		t.Fatalf("table_list[] = %v", got)
	}
	if form.Get("valid_date") != "2026-11-7" || form.Get("limit_num") != "500" {
		t.Fatalf("unexpected apply form schedule: %v", form)
	}
}

func TestApplyPrivilegeEncodesDatabaseMode(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	err := client.ApplyPrivilege(context.Background(), PrivilegeApplyInput{
		Title:        "查询测试环境数据",
		InstanceName: "demo-order-rw",
		GroupName:    "Demo-Group-C",
		PrivType:     PrivilegeTypeDatabase,
		DBList:       []string{"shop", "report"},
		ValidDate:    "2027-10-31",
		LimitNum:     1000,
	})
	if err != nil {
		t.Fatalf("ApplyPrivilege: %v", err)
	}

	form := fs.PrivilegeApplyForm()
	if got := form["db_list[]"]; len(got) != 2 || got[0] != "shop" || got[1] != "report" {
		t.Fatalf("db_list[] = %v", got)
	}
	if got := form["table_list[]"]; len(got) != 0 {
		t.Fatalf("table_list[] should be empty in database mode, got %v", got)
	}
}

func TestApplyPrivilegeServerError(t *testing.T) {
	fs := newFake(t, fakeOptions{privilegeApplyStatus: 1, privilegeApplyMsg: "新建审批流失败, 请联系管理员"})
	client := newTestClient(t, fs)

	err := client.ApplyPrivilege(context.Background(), PrivilegeApplyInput{
		Title:        "查询数据",
		InstanceName: "demo-order-rw",
		GroupName:    "Demo-Group-C",
		PrivType:     PrivilegeTypeDatabase,
		DBList:       []string{"shop"},
		ValidDate:    "2026-11-7",
		LimitNum:     100,
	})
	if !IsKind(err, KindServer) || !strings.Contains(err.Error(), "新建审批流失败") {
		t.Fatalf("expected server error, got %v", err)
	}
}

func TestPrivilegeDetailDetectsReviewable(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	detail, err := client.PrivilegeDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("PrivilegeDetail: %v", err)
	}
	if !detail.CanReview {
		t.Fatalf("expected reviewable detail, got %+v", detail)
	}

	plain := newFake(t, fakeOptions{
		privilegeDetailHTML: `<html><body><h4>待审核</h4><p>你无审核权限</p></body></html>`,
	})
	other, err := newTestClient(t, plain).PrivilegeDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("PrivilegeDetail without buttons: %v", err)
	}
	if other.CanReview {
		t.Fatalf("expected non-reviewable detail, got %+v", other)
	}
}

// reviewableDetailPageHTML 按 v1.14 的 queryapplydetail.html + workflow_display.html 渲染结果构造：
// 审批流是 h5 里未闭合的节点链（真实模板同样缺 </h5>），清单在 panel-body。
const reviewableDetailPageHTML = `<html><body>
<h4 style="display: inline;">工单名称：<span>查询数据</span></h4>
<hr>
<h4>
    审批流
</h4>
<h5>
    <span class="text-success">Demo:Leader-A</span>
    <span class="glyphicon glyphicon-arrow-right" aria-hidden="true"></span>
    <u class="text-danger">
        示例审批节点(
        <span class="text-primary">张伟, 李娜</span>
        )
    </u>
    <span class="glyphicon glyphicon-arrow-right" aria-hidden="true"></span>
    Leader-A
<h4>
    其他信息
</h4>
<table class="table">
    <thead><tr><th>申请人</th><th>实例</th></tr></thead>
    <tbody><tr class="success"><td>示例用户A</td><td>demo-mysql-rw</td></tr></tbody>
</table>
<div class="panel panel-default">
    <div class="panel-heading">
        表清单
    </div>
    <div class="panel-body">
        demo_materials.t_order<br>demo_materials.t_item
    </div>
</div>
<table class="table">
    <thead><tr><th>操作信息</th></tr></thead>
    <tbody><tr><td>示例用户A 2026-10-31 10:05:00 审核通过</td></tr></tbody>
</table>
<form action="/query/privaudit/" method="post">
    <input type="hidden" name="apply_id" value="7">
    <input type="submit" id="btnPass" class="btn btn-success" value="审核通过"/>
</form>
</body></html>`

// autoPassDetailPageHTML 对应单节点自动通过的工单：workflow_display 只输出「无需审批」。
const autoPassDetailPageHTML = `<html><body>
<h4 style="display: inline;">工单名称：<span>查看数据</span></h4>
<h4>审批流</h4>
<h5>
    <span class="text-success">无需审批</span>
<h4>其他信息</h4>
<div class="panel panel-default">
    <div class="panel-heading">数据库清单</div>
    <div class="panel-body">
        demo_materials<br>demo_config
    </div>
</div>
</body></html>`

func TestPrivilegeDetailParsesReviewFlow(t *testing.T) {
	fs := newFake(t, fakeOptions{privilegeDetailHTML: reviewableDetailPageHTML})
	client := newTestClient(t, fs)

	detail, err := client.PrivilegeDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("PrivilegeDetail: %v", err)
	}
	if detail.ApplyID != 7 || detail.Title != "查询数据" {
		t.Fatalf("unexpected heading: %+v", detail)
	}
	if !detail.CanReview {
		t.Fatalf("expected reviewable detail, got %+v", detail)
	}
	want := []PrivilegeReviewNode{
		{Name: "Demo:Leader-A", State: "passed"},
		{Name: "示例审批节点", State: "current", Reviewers: []string{"张伟", "李娜"}},
		{Name: "Leader-A", State: "pending"},
	}
	if len(detail.ReviewNodes) != len(want) {
		t.Fatalf("expected %d nodes, got %+v", len(want), detail.ReviewNodes)
	}
	for i, node := range want {
		got := detail.ReviewNodes[i]
		if got.Name != node.Name || got.State != node.State {
			t.Fatalf("node %d: expected %+v, got %+v", i, node, got)
		}
		if !stringSliceEqual(got.Reviewers, node.Reviewers) {
			t.Fatalf("node %d reviewers: expected %v, got %v", i, node.Reviewers, got.Reviewers)
		}
	}
	if detail.ItemKind != "table" {
		t.Fatalf("expected table item kind, got %q", detail.ItemKind)
	}
	if !stringSliceEqual(detail.Items, []string{"demo_materials.t_order", "demo_materials.t_item"}) {
		t.Fatalf("unexpected items: %v", detail.Items)
	}
	if !strings.Contains(detail.LastOperation, "审核通过") {
		t.Fatalf("expected last operation info, got %q", detail.LastOperation)
	}
}

func TestPrivilegeDetailParsesAutoPassAndDatabaseItems(t *testing.T) {
	fs := newFake(t, fakeOptions{privilegeDetailHTML: autoPassDetailPageHTML})
	detail, err := newTestClient(t, fs).PrivilegeDetail(context.Background(), 8)
	if err != nil {
		t.Fatalf("PrivilegeDetail: %v", err)
	}
	if len(detail.ReviewNodes) != 1 {
		t.Fatalf("expected a single auto-pass node, got %+v", detail.ReviewNodes)
	}
	if node := detail.ReviewNodes[0]; node.State != "auto" || node.Name != "" {
		t.Fatalf("unexpected auto node: %+v", node)
	}
	if detail.ItemKind != "database" {
		t.Fatalf("expected database item kind, got %q", detail.ItemKind)
	}
	if !stringSliceEqual(detail.Items, []string{"demo_materials", "demo_config"}) {
		t.Fatalf("unexpected items: %v", detail.Items)
	}
	if detail.CanReview {
		t.Fatalf("expected non-reviewable detail, got %+v", detail)
	}
}

func TestPrivilegeDetailToleratesPageWithoutDetailBlocks(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	detail, err := newTestClient(t, fs).PrivilegeDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("PrivilegeDetail: %v", err)
	}
	if detail.ApplyID != 7 || detail.Title != "" || len(detail.ReviewNodes) != 0 {
		t.Fatalf("unexpected fallback detail: %+v", detail)
	}
	if detail.ItemKind != "" || len(detail.Items) != 0 || detail.LastOperation != "" {
		t.Fatalf("unexpected fallback blocks: %+v", detail)
	}
	if !detail.CanReview {
		t.Fatalf("btnPass detection must keep working: %+v", detail)
	}
}

func TestPrivilegeDetailNotFound(t *testing.T) {
	fs := newFake(t, fakeOptions{privilegeDetailStatus: 404})
	client := newTestClient(t, fs)

	_, err := client.PrivilegeDetail(context.Background(), 999)
	if !IsKind(err, KindNotFound) {
		t.Fatalf("expected not_found, got %v", err)
	}
}

func TestAuditPrivilegeSuccess(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	if err := client.AuditPrivilege(context.Background(), 7, AuditReject, "重复申请"); err != nil {
		t.Fatalf("AuditPrivilege: %v", err)
	}
	form := fs.AuditForm()
	if form.Get("apply_id") != "7" || form.Get("audit_status") != "2" || form.Get("audit_remark") != "重复申请" {
		t.Fatalf("unexpected audit form: %v", form)
	}
}

func TestAuditPrivilegeServerMessage(t *testing.T) {
	fs := newFake(t, fakeOptions{
		auditErrorHTML: `<html><body><h4>审核失败: 你无该工单的审核权限</h4></body></html>`,
	})
	client := newTestClient(t, fs)

	err := client.AuditPrivilege(context.Background(), 7, AuditPass, "")
	if err == nil {
		t.Fatal("expected audit error")
	}
	if !IsKind(err, KindServer) || !strings.Contains(err.Error(), "你无该工单的审核权限") {
		t.Fatalf("unexpected audit error: %v", err)
	}
}

func TestAuditPrivilegeForbidden(t *testing.T) {
	fs := newFake(t, fakeOptions{auditForbidden: true})
	client := newTestClient(t, fs)

	err := client.AuditPrivilege(context.Background(), 7, AuditPass, "")
	if !IsKind(err, KindForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestWorkflowLogsParsesRows(t *testing.T) {
	fs := newFake(t, fakeOptions{
		workflowLogs: map[string]any{
			"total": 2,
			"rows": []any{
				map[string]any{
					"operation_type_desc": "提交申请",
					"operator_display":    "示例用户A",
					"operation_time":      "2026-10-31 10:03:36",
					"operation_info":      "申请查询权限",
				},
				map[string]any{
					"operation_type_desc": "审核通过",
					"operator_display":    "示例用户C",
					"operation_time":      "2026-10-31 11:00:00",
					"operation_info":      "",
				},
			},
		},
	})
	client := newTestClient(t, fs)

	page, err := client.WorkflowLogs(context.Background(), 7)
	if err != nil {
		t.Fatalf("WorkflowLogs: %v", err)
	}
	if page.Total != 2 || len(page.Rows) != 2 {
		t.Fatalf("unexpected logs page: %+v", page)
	}
	if page.Rows[0].OperationType != "提交申请" || page.Rows[0].Operator != "示例用户A" || page.Rows[0].Info != "申请查询权限" {
		t.Fatalf("unexpected first log: %+v", page.Rows[0])
	}
	if page.Rows[1].OperationType != "审核通过" || page.Rows[1].Time != "2026-10-31 11:00:00" {
		t.Fatalf("unexpected second log: %+v", page.Rows[1])
	}

	form := fs.WorkflowLogForm()
	if form.Get("workflow_id") != "7" || form.Get("workflow_type") != "1" {
		t.Fatalf("unexpected workflow log form: %v", form)
	}
}

// 我的权限：Archery /query/userprivileges/ 返回 Django values() 的原始字段名，
// 后端归一化成驼峰后交给 UI；分页参数与申请列表保持一致。
func TestUserQueryPrivilegesParsesRows(t *testing.T) {
	fs := newFake(t, fakeOptions{
		userPrivilegeList: map[string]any{
			"total": 2,
			"rows": []any{
				map[string]any{
					"privilege_id":            11,
					"user_display":            "示例用户A",
					"instance__instance_name": "demo-mysql-rw",
					"db_name":                 "demo_config",
					"priv_type":               1,
					"table_name":              "",
					"limit_num":               1000,
					"valid_date":              "2027-10-31",
				},
				map[string]any{
					"privilege_id":            10,
					"user_display":            "示例用户A",
					"instance__instance_name": "demo-order-rw",
					"db_name":                 "erp",
					"priv_type":               2,
					"table_name":              "orders",
					"limit_num":               500,
					"valid_date":              "2026-11-30",
				},
			},
		},
	})
	client := newTestClient(t, fs)

	page, err := client.UserQueryPrivileges(context.Background(), 20, 20, "demo_registry")
	if err != nil {
		t.Fatalf("UserQueryPrivileges: %v", err)
	}
	if page.Total != 2 || len(page.Rows) != 2 {
		t.Fatalf("unexpected page: %+v", page)
	}
	first := page.Rows[0]
	if first.PrivilegeID != 11 || first.UserDisplay != "示例用户A" || first.InstanceName != "demo-mysql-rw" {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.DBName != "demo_config" || first.PrivType != PrivilegeTypeDatabase || first.TableName != "" {
		t.Fatalf("unexpected first row targets: %+v", first)
	}
	if first.LimitNum != 1000 || first.ValidDate != "2027-10-31" {
		t.Fatalf("unexpected first row limits: %+v", first)
	}
	second := page.Rows[1]
	if second.PrivType != PrivilegeTypeTable || second.TableName != "orders" || second.DBName != "erp" {
		t.Fatalf("unexpected second row: %+v", second)
	}

	form := fs.UserPrivilegeForm()
	if form.Get("limit") != "20" || form.Get("offset") != "20" || form.Get("search") != "demo_registry" {
		t.Fatalf("unexpected user privilege form: %v", form)
	}
}

func TestUserQueryPrivilegesEmptyPage(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	page, err := client.UserQueryPrivileges(context.Background(), 0, 0, "")
	if err != nil {
		t.Fatalf("UserQueryPrivileges: %v", err)
	}
	if page == nil || page.Rows == nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Fatalf("unexpected empty page: %+v", page)
	}
}

func TestUserQueryPrivilegesRejectsJWTMode(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs, func(cfg *Config) { cfg.AuthMode = "jwt" })

	_, err := client.UserQueryPrivileges(context.Background(), 20, 0, "")
	if !IsKind(err, KindUnsupported) {
		t.Fatalf("expected unsupported in JWT mode, got %v", err)
	}
}
