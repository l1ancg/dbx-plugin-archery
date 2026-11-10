package rpc

import "testing"

// 「待我审核」只认 /workflow/list/ 的审批队列。
//
// Archery v1.14 的 /sqlworkflow_list_audit/ 只是给 /sqlworkflow_list/ 加了
// sql.audit_user 权限装饰器，查询条件与普通列表完全一致，不能用来判断
// 「当前审核节点是否轮到我的审核组」；真正按 current_audit 过滤的是
// common/workflow.py::lists。两边的行都没有对方的主键，因此按
// 「工单名 + 资源组」求交集，并用发起人排除同名不同人的工单。

func auditTodoRow(auditID int, title, applicant, group, createTime string) map[string]any {
	return map[string]any{
		"audit_id":            auditID,
		"workflow_type":       2,
		"workflow_title":      title,
		"create_user_display": applicant,
		"create_time":         createTime,
		"current_status":      0,
		"audit_auth_groups":   "1,2",
		"current_audit":       "2",
		"group_name":          group,
	}
}

func auditPendingRow(id int, name, applicant, group, status, createTime string) map[string]any {
	return map[string]any{
		"id":                      id,
		"workflow_name":           name,
		"engineer_display":        applicant,
		"status":                  status,
		"is_backup":               0,
		"create_time":             createTime,
		"instance__instance_name": "test-express-rw",
		"db_name":                 "schedule-dev",
		"group_name":              group,
		"syntax_type":             1,
		"export_format":           "",
	}
}

func TestSqlWorkflowsAuditOnlyCurrentReviewer(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	// 待审核工单里只有第一条与审批队列同名同组同发起人。
	fake.setPendingRows([]any{
		auditPendingRow(5338, "DDL - 车辆表结构修改", "廖思源", "Test-Express", "workflow_manreviewing", "2026-11-05 10:13:08"),
		auditPendingRow(5337, "DDL - 车辆表结构修改", "喻量", "Test-Express", "workflow_manreviewing", "2026-11-04 09:12:30"),
		auditPendingRow(5336, "数据订正 - 车辆", "廖思源", "Test-Express", "workflow_manreviewing", "2026-11-03 18:00:00"),
	})

	payload := wfCall(t, service, "archery/sqlWorkflows", map[string]any{"audit": true, "limit": 20})
	page := wfObject(t, payload["page"])
	if wfNumber(t, page["total"]) != 1 {
		t.Fatalf("total = %v, want 1", page["total"])
	}
	rows := wfList(t, page["rows"])
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := wfObject(t, rows[0])
	if wfNumber(t, row["id"]) != 5338 {
		t.Fatalf("id = %v, want 5338", row["id"])
	}

	if fake.Calls("/sqlworkflow_list_audit/") != 0 {
		t.Fatal("/sqlworkflow_list_audit/ 与普通列表同源，不应作为待我审核的数据源")
	}
	auditForm := fake.Form("/workflow/list/")
	if auditForm == nil {
		t.Fatal("expected /workflow/list/ to be called")
	}
	if auditForm.Get("workflow_type") != "2" {
		t.Fatalf("workflow_type = %q, want 2", auditForm.Get("workflow_type"))
	}
	listForm := fake.Form("/sqlworkflow_list/")
	if listForm == nil {
		t.Fatal("expected /sqlworkflow_list/ to be called")
	}
	if listForm.Get("navStatus") != "workflow_manreviewing" {
		t.Fatalf("navStatus = %q, want workflow_manreviewing", listForm.Get("navStatus"))
	}
}

func TestSqlWorkflowsAuditEmptyQueueSkipsList(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)
	fake.setAuditRows([]any{})

	payload := wfCall(t, service, "archery/sqlWorkflows", map[string]any{"audit": true, "limit": 20})
	page := wfObject(t, payload["page"])
	if wfNumber(t, page["total"]) != 0 {
		t.Fatalf("total = %v, want 0", page["total"])
	}
	if rows := wfList(t, page["rows"]); len(rows) != 0 {
		t.Fatalf("rows = %v, want empty", rows)
	}
	if fake.Calls("/sqlworkflow_list/") != 0 {
		t.Fatal("审批队列为空时不应再拉待审核工单")
	}
}

func TestSqlWorkflowsAuditPagesAfterIntersection(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	fake.setAuditRows([]any{
		auditTodoRow(301, "工单A", "廖思源", "Test-Express", "2026-11-05 10:00:03"),
		auditTodoRow(302, "工单B", "廖思源", "Test-Express", "2026-11-05 10:00:02"),
		auditTodoRow(303, "工单C", "廖思源", "Test-Express", "2026-11-05 10:00:01"),
	})
	// 列表按创建时间倒序，交集中的 3 条夹着一条别人的待审工单。
	fake.setPendingRows([]any{
		auditPendingRow(5333, "工单C", "廖思源", "Test-Express", "workflow_manreviewing", "2026-11-05 10:00:03"),
		auditPendingRow(5332, "工单B", "廖思源", "Test-Express", "workflow_manreviewing", "2026-11-05 10:00:02"),
		auditPendingRow(5331, "别人的工单", "喻量", "Test-Express", "workflow_manreviewing", "2026-11-05 10:00:01"),
		auditPendingRow(5330, "工单A", "廖思源", "Test-Express", "workflow_manreviewing", "2026-11-05 10:00:00"),
	})

	payload := wfCall(t, service, "archery/sqlWorkflows", map[string]any{
		"audit": true, "limit": 2, "offset": 2,
	})
	page := wfObject(t, payload["page"])
	if wfNumber(t, page["total"]) != 3 {
		t.Fatalf("total = %v, want 3", page["total"])
	}
	rows := wfList(t, page["rows"])
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if id := wfNumber(t, wfObject(t, rows[0])["id"]); id != 5330 {
		t.Fatalf("id = %v, want 5330", id)
	}
}

func TestSqlWorkflowsAuditKeepsFiltersAndForcesPendingStatus(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/sqlWorkflows", map[string]any{
		"audit":       true,
		"limit":       20,
		"search":      "  车辆 ",
		"status":      "workflow_finish",
		"instanceId":  12,
		"groupId":     3,
		"startDate":   "2026-10-01",
		"endDate":     "2026-10-31",
		"syntaxTypes": []string{"1", "2"},
	})

	auditForm := fake.Form("/workflow/list/")
	if auditForm == nil {
		t.Fatal("expected /workflow/list/ to be called")
	}
	// 审批队列不按 search 收窄：/workflow/list/ 只搜标题，会漏掉按发起人搜的工单。
	if auditForm.Get("search") != "" {
		t.Fatalf("audit search = %q, want empty", auditForm.Get("search"))
	}
	if auditForm.Get("limit") != "200" || auditForm.Get("offset") != "0" {
		t.Fatalf("audit paging = %v", auditForm)
	}

	form := fake.Form("/sqlworkflow_list/")
	if form == nil {
		t.Fatal("expected /sqlworkflow_list/ to be called")
	}
	// 待我审核只可能是待审核状态，页面上的状态筛选必须被忽略。
	if form.Get("navStatus") != "workflow_manreviewing" {
		t.Fatalf("navStatus = %q, want workflow_manreviewing", form.Get("navStatus"))
	}
	if form.Get("search") != "车辆" {
		t.Fatalf("search = %q", form.Get("search"))
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
