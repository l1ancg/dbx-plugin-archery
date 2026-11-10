package rpc

import (
	"context"
	"strings"
	"testing"

	"github.com/nixing178/dbx-plugin-archery/internal/archery"
)

func connectFake(t *testing.T, fake *rpcFake, mutate func(map[string]any)) *Service {
	t.Helper()
	service := NewService()
	if _, err := service.Handle(context.Background(), "connection/connect", connectParams(t, fake, mutate)); err != nil {
		t.Fatalf("connect: %+v", err)
	}
	return service
}

func TestPrivilegeListLimitRules(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/privileges", simpleParams(t, map[string]any{
		"limit":  500,
		"offset": -3,
		"search": "查询",
	}))
	if err != nil {
		t.Fatalf("privileges: %+v", err)
	}
	page, ok := result.(map[string]any)["page"].(*archery.PrivilegeApplyPage)
	if !ok {
		t.Fatalf("unexpected payload: %+v", result)
	}
	if page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].ApplyID != 7 {
		t.Fatalf("unexpected page: %+v", page)
	}
	form := fake.PrivListForm()
	if form.Get("limit") != "200" {
		t.Fatalf("limit should be capped at 200, got %v", form.Get("limit"))
	}
	if form.Get("offset") != "0" {
		t.Fatalf("negative offset should clamp to 0, got %v", form.Get("offset"))
	}
}

// 我的权限与申请列表共用分页规则：limit 默认 20、上限 200，offset 下限 0。
func TestMyPrivilegesLimitRules(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/myPrivileges", simpleParams(t, map[string]any{
		"limit":  500,
		"offset": -3,
		"search": " demo_registry ",
	}))
	if err != nil {
		t.Fatalf("myPrivileges: %+v", err)
	}
	page, ok := result.(map[string]any)["page"].(*archery.UserPrivilegePage)
	if !ok {
		t.Fatalf("unexpected payload: %+v", result)
	}
	if page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].PrivilegeID != 11 {
		t.Fatalf("unexpected page: %+v", page)
	}
	form := fake.MyPrivilegesForm()
	if form.Get("limit") != "200" {
		t.Fatalf("limit should be capped at 200, got %v", form.Get("limit"))
	}
	if form.Get("offset") != "0" {
		t.Fatalf("negative offset should clamp to 0, got %v", form.Get("offset"))
	}
	if form.Get("search") != "demo_registry" {
		t.Fatalf("search should be trimmed, got %q", form.Get("search"))
	}
}

func TestPrivilegeOptionsReturnsGroups(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/privilegeOptions", simpleParams(t, nil))
	if err != nil {
		t.Fatalf("privilegeOptions: %+v", err)
	}
	groups, ok := result.(map[string]any)["groups"].([]string)
	if !ok {
		t.Fatalf("unexpected groups payload: %+v", result)
	}
	if len(groups) != 1 || groups[0] != "Demo-Group-C" {
		t.Fatalf("unexpected groups: %v", groups)
	}
}

func TestPrivilegeInstancesRequiresGroup(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	_, err := service.Handle(context.Background(), "archery/privilegeInstances", simpleParams(t, nil))
	if err == nil || err.Code != -32602 || err.Kind != "invalid_params" {
		t.Fatalf("expected invalid params, got %+v", err)
	}
}

func TestPrivilegeInstancesReturnsRows(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/privilegeInstances", simpleParams(t, map[string]any{
		"groupName": "Demo-Group-C",
	}))
	if err != nil {
		t.Fatalf("privilegeInstances: %+v", err)
	}
	list, ok := result.(map[string]any)["instances"].([]archery.GroupInstance)
	if !ok || len(list) != 1 || list[0].Name != "demo-order-rw" || list[0].DBType != "mysql" {
		t.Fatalf("unexpected instances: %+v", result)
	}
}

// 申请弹窗选中资源组后要显示「审批流程：TEST-Leader-Express」，
// 走查询权限工单（workflow_type=1），不能复用 SQL 上线的审批流。
func TestPrivilegeAuditors(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/privilegeAuditors", simpleParams(t, map[string]any{
		"groupName": " Demo-Group-C ",
	}))
	if err != nil {
		t.Fatalf("privilegeAuditors: %+v", err)
	}
	payload := result.(map[string]any)
	if payload["auditorsDisplay"] != "TEST-Leader-Express" {
		t.Fatalf("unexpected auditors display: %+v", payload)
	}
	form := fake.AuditorsForm()
	if form.Get("group_name") != "Demo-Group-C" || form.Get("workflow_type") != "1" {
		t.Fatalf("unexpected auditors form: %v", form)
	}
}

func TestPrivilegeAuditorsRequiresGroup(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	_, err := service.Handle(context.Background(), "archery/privilegeAuditors", simpleParams(t, nil))
	if err == nil || err.Code != -32602 || !strings.Contains(err.Message, "groupName") {
		t.Fatalf("expected invalid groupName, got %+v", err)
	}
}

func TestPrivilegeApplyValidation(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{
			name:   "missing title",
			params: map[string]any{"groupName": "g", "instanceName": "i", "privType": 1, "dbList": []string{"a"}, "validDate": "2026-11-7", "limitNum": 100},
			want:   "title",
		},
		{
			name:   "missing group",
			params: map[string]any{"title": "t", "instanceName": "i", "privType": 1, "dbList": []string{"a"}, "validDate": "2026-11-7", "limitNum": 100},
			want:   "groupName",
		},
		{
			name:   "missing databases",
			params: map[string]any{"title": "t", "groupName": "g", "instanceName": "i", "privType": 1, "validDate": "2026-11-7", "limitNum": 100},
			want:   "dbList",
		},
		{
			name:   "missing table list",
			params: map[string]any{"title": "t", "groupName": "g", "instanceName": "i", "privType": 2, "dbName": "erp", "validDate": "2026-11-7", "limitNum": 100},
			want:   "tableList",
		},
		{
			name:   "missing valid date",
			params: map[string]any{"title": "t", "groupName": "g", "instanceName": "i", "privType": 1, "dbList": []string{"a"}, "limitNum": 100},
			want:   "validDate",
		},
		{
			name:   "invalid limit",
			params: map[string]any{"title": "t", "groupName": "g", "instanceName": "i", "privType": 1, "dbList": []string{"a"}, "validDate": "2026-11-7"},
			want:   "limitNum",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Handle(context.Background(), "archery/privilegeApply", simpleParams(t, tc.params))
			if err == nil || err.Code != -32602 || err.Kind != "invalid_params" {
				t.Fatalf("expected invalid params, got %+v", err)
			}
			if !strings.Contains(err.Message, tc.want) {
				t.Fatalf("error should mention %q, got %q", tc.want, err.Message)
			}
		})
	}
}

func TestPrivilegeApplyAllowedInReadOnly(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, func(payload map[string]any) {
		config := payload["connection"].(map[string]any)["external_config"].(map[string]any)
		config["read_only"] = true
	})

	if _, err := service.Handle(context.Background(), "archery/privilegeApply", simpleParams(t, map[string]any{
		"title":        "排查线上问题",
		"groupName":    "Demo-Group-C",
		"instanceName": "demo-order-rw",
		"privType":     2,
		"dbName":       "erp",
		"tableList":    []string{"orders"},
		"validDate":    "2026-11-7",
		"limitNum":     500,
	})); err != nil {
		t.Fatalf("privilege apply should be allowed in read-only mode: %+v", err)
	}
	form := fake.PrivApplyForm()
	if form.Get("title") != "排查线上问题" || form.Get("priv_type") != "2" {
		t.Fatalf("unexpected apply form: %v", form)
	}
	if got := form["table_list[]"]; len(got) != 1 || got[0] != "orders" {
		t.Fatalf("table_list[] = %v", got)
	}
}

func TestPrivilegeAuditValidation(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	if _, err := service.Handle(context.Background(), "archery/privilegeAudit", simpleParams(t, map[string]any{
		"auditStatus": 1,
	})); err == nil || err.Code != -32602 {
		t.Fatalf("expected invalid params for missing applyId, got %+v", err)
	}
	if _, err := service.Handle(context.Background(), "archery/privilegeAudit", simpleParams(t, map[string]any{
		"applyId":     7,
		"auditStatus": 9,
	})); err == nil || err.Code != -32602 || !strings.Contains(err.Message, "auditStatus") {
		t.Fatalf("expected invalid auditStatus, got %+v", err)
	}
}

func TestPrivilegeAuditSucceeds(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/privilegeAudit", simpleParams(t, map[string]any{
		"applyId":     7,
		"auditStatus": 2,
		"auditRemark": "重复申请",
	}))
	if err != nil {
		t.Fatalf("privilegeAudit: %+v", err)
	}
	if result.(map[string]any)["success"] != true {
		t.Fatalf("unexpected audit result: %+v", result)
	}
	form := fake.AuditForm()
	if form.Get("apply_id") != "7" || form.Get("audit_status") != "2" || form.Get("audit_remark") != "重复申请" {
		t.Fatalf("unexpected audit form: %v", form)
	}
}

func TestWorkflowLogDefaults(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/workflowLog", simpleParams(t, map[string]any{
		"workflowId": 7,
	}))
	if err != nil {
		t.Fatalf("workflowLog: %+v", err)
	}
	page, ok := result.(map[string]any)["page"].(*archery.WorkflowLogPage)
	if !ok || page.Total != 1 || len(page.Rows) != 1 {
		t.Fatalf("unexpected workflow log page: %+v", result)
	}
	if page.Rows[0].OperationType != "提交申请" || page.Rows[0].Operator != "示例用户A" {
		t.Fatalf("unexpected log row: %+v", page.Rows[0])
	}
	form := fake.WorkflowLogForm()
	if form.Get("workflow_id") != "7" || form.Get("workflow_type") != "1" {
		t.Fatalf("workflow_type should default to 1, got %v", form)
	}
}

func TestWorkflowLogRequiresWorkflowID(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	_, err := service.Handle(context.Background(), "archery/workflowLog", simpleParams(t, nil))
	if err == nil || err.Code != -32602 || !strings.Contains(err.Message, "workflowId") {
		t.Fatalf("expected invalid workflowId, got %+v", err)
	}
}
