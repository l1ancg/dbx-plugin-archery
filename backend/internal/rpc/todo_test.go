package rpc

import (
	"testing"
)

// 待办列表：字段取自 Archery common/workflow.py::lists 的 values()。
func TestWorkflowTodosParams(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/workflowTodos", map[string]any{
		"limit":        20,
		"offset":       0,
		"search":       "  车辆 ",
		"workflowType": 2,
	})

	page := wfObject(t, payload["page"])
	if wfNumber(t, page["total"]) != 1 {
		t.Fatalf("total = %v", page["total"])
	}
	rows := wfList(t, page["rows"])
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	row := wfObject(t, rows[0])
	if wfNumber(t, row["auditId"]) != 118 {
		t.Fatalf("auditId = %v", row["auditId"])
	}
	if wfNumber(t, row["workflowType"]) != 2 {
		t.Fatalf("workflowType = %v", row["workflowType"])
	}
	if row["title"] != "DDL - 车辆表结构修改" {
		t.Fatalf("title = %v", row["title"])
	}
	if row["applicant"] != "廖思源" {
		t.Fatalf("applicant = %v", row["applicant"])
	}
	if row["createTime"] != "2026-11-05 10:13:08" {
		t.Fatalf("createTime = %v", row["createTime"])
	}
	if wfNumber(t, row["status"]) != 0 {
		t.Fatalf("status = %v", row["status"])
	}
	if row["groupName"] != "Test-Express" {
		t.Fatalf("groupName = %v", row["groupName"])
	}

	form := fake.Form("/workflow/list/")
	if form == nil {
		t.Fatal("expected /workflow/list/ to be called")
	}
	if form.Get("limit") != "20" || form.Get("offset") != "0" {
		t.Fatalf("paging = %v", form)
	}
	if form.Get("workflow_type") != "2" {
		t.Fatalf("workflow_type = %q", form.Get("workflow_type"))
	}
	if form.Get("search") != "车辆" {
		t.Fatalf("search should be trimmed, got %q", form.Get("search"))
	}
}

func TestWorkflowTodosLimitRules(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	wfCall(t, service, "archery/workflowTodos", map[string]any{"limit": 500, "offset": -5})
	form := fake.Form("/workflow/list/")
	if form.Get("limit") != "200" {
		t.Fatalf("limit should clamp to 200, got %q", form.Get("limit"))
	}
	if form.Get("offset") != "0" {
		t.Fatalf("offset should clamp to 0, got %q", form.Get("offset"))
	}
	// 不传类型等价于「全部」，且必须显式带 workflow_type=0（Archery 直接 int() 转换）。
	if form.Get("workflow_type") != "0" {
		t.Fatalf("workflow_type should default to 0, got %q", form.Get("workflow_type"))
	}
}

func TestWorkflowTodoTargetResolvesSqlWorkflow(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	payload := wfCall(t, service, "archery/workflowTodoTarget", map[string]any{"auditId": 118})
	target := wfObject(t, payload["target"])
	if wfNumber(t, target["auditId"]) != 118 {
		t.Fatalf("auditId = %v", target["auditId"])
	}
	if wfNumber(t, target["workflowId"]) != 5329 {
		t.Fatalf("workflowId = %v", target["workflowId"])
	}
	if wfNumber(t, target["workflowType"]) != 2 {
		t.Fatalf("workflowType = %v", target["workflowType"])
	}
}

func TestWorkflowTodoTargetResolvesPrivilegeAndArchive(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	cases := []struct {
		auditID  int
		wantID   float64
		wantType float64
	}{
		{auditID: 117, wantID: 40, wantType: 1},
		{auditID: 116, wantID: 9, wantType: 3},
	}
	for _, tc := range cases {
		payload := wfCall(t, service, "archery/workflowTodoTarget", map[string]any{"auditId": tc.auditID})
		target := wfObject(t, payload["target"])
		if wfNumber(t, target["workflowId"]) != tc.wantID {
			t.Fatalf("audit %d workflowId = %v, want %v", tc.auditID, target["workflowId"], tc.wantID)
		}
		if wfNumber(t, target["workflowType"]) != tc.wantType {
			t.Fatalf("audit %d workflowType = %v, want %v", tc.auditID, target["workflowType"], tc.wantType)
		}
	}
}

func TestWorkflowTodoTargetUnresolved(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, false)

	if err := wfCallErr(t, service, "archery/workflowTodoTarget", map[string]any{"auditId": 0}); err.Code != -32602 {
		t.Fatalf("expected auditId validation error, got %+v", err)
	}
	err := wfCallErr(t, service, "archery/workflowTodoTarget", map[string]any{"auditId": 999})
	if err.Code != -32000 {
		t.Fatalf("expected server error for missing workflow, got %+v", err)
	}
}

// 待办列表只读：不写任何数据，只读连接必须放行。
func TestWorkflowTodosAllowedInReadOnly(t *testing.T) {
	fake := newWorkflowFake(t)
	service := wfConnect(t, fake, true)

	wfCall(t, service, "archery/workflowTodos", map[string]any{"limit": 20})
	wfCall(t, service, "archery/workflowTodoTarget", map[string]any{"auditId": 118})
}
