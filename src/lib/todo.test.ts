import { describe, expect, it } from "vitest";
import {
  normalizeTodo,
  todoRowRange,
  todoStatusMeta,
  todoToPrivilegeRow,
  todoToSqlWorkflowRow,
  todoTypeLabelKey,
  todoTypeOptions,
} from "./todo";
import type { WorkflowTodoRow } from "../types";

const sqlTodo: WorkflowTodoRow = {
  auditId: 118,
  workflowType: 2,
  title: "DDL - 车辆表结构修改",
  applicant: "廖思源",
  createTime: "2026-11-05 10:13:08",
  status: 0,
  groupName: "Test-Express",
  currentAudit: "2",
  auditGroups: "1,2",
};

describe("normalizeTodo", () => {
  it("reads the Django values() keys from /workflow/list/", () => {
    expect(
      normalizeTodo({
        audit_id: 118,
        workflow_type: 2,
        workflow_title: "DDL - 车辆表结构修改",
        create_user_display: "廖思源",
        create_time: "2026-11-05 10:13:08",
        current_status: 0,
        audit_auth_groups: "1,2",
        current_audit: "2",
        group_name: "Test-Express",
      }),
    ).toEqual(sqlTodo);
  });

  it("accepts the sidecar camelCase shape and string numbers", () => {
    expect(
      normalizeTodo({
        auditId: "7",
        workflowType: "1",
        title: "库表权限申请",
        applicant: "示例用户",
        createTime: "2026-11-05 09:00:00",
        status: "0",
        groupName: "Demo-Group",
      }),
    ).toEqual({
      auditId: 7,
      workflowType: 1,
      title: "库表权限申请",
      applicant: "示例用户",
      createTime: "2026-11-05 09:00:00",
      status: 0,
      groupName: "Demo-Group",
      currentAudit: "",
      auditGroups: "",
    });
  });

  it("keeps empty cells readable instead of printing undefined", () => {
    const row = normalizeTodo({});
    expect(row.title).toBe("");
    expect(row.applicant).toBe("");
    expect(row.auditId).toBe(0);
  });
});

describe("todoTypeLabelKey", () => {
  it("maps the three Archery workflow types", () => {
    expect(todoTypeLabelKey(1)).toBe("todo.typeQuery");
    expect(todoTypeLabelKey(2)).toBe("todo.typeSql");
    expect(todoTypeLabelKey(3)).toBe("todo.typeArchive");
  });

  it("falls back for unknown types", () => {
    expect(todoTypeLabelKey(9)).toBe("todo.typeUnknown");
    expect(todoTypeLabelKey(0)).toBe("todo.typeUnknown");
  });
});

describe("todoTypeOptions", () => {
  it("puts the all-types entry first, then the three types", () => {
    expect(todoTypeOptions.map((item) => item.value)).toEqual([0, 1, 2, 3]);
    expect(todoTypeOptions[0].labelKey).toBe("todo.typeAll");
  });
});

describe("todoStatusMeta", () => {
  it("maps waiting / pass / reject", () => {
    expect(todoStatusMeta(0)).toEqual({ tone: "info", labelKey: "todo.statusWaiting" });
    expect(todoStatusMeta(1)).toEqual({ tone: "ok", labelKey: "todo.statusPass" });
    expect(todoStatusMeta(2)).toEqual({ tone: "danger", labelKey: "todo.statusReject" });
  });

  it("degrades unknown status to a muted label", () => {
    expect(todoStatusMeta(9)).toEqual({ tone: "muted", labelKey: "todo.statusUnknown" });
  });
});

describe("todoRowRange", () => {
  it("returns the visible slice for a full page", () => {
    expect(todoRowRange(1, 20, 40)).toEqual({ from: 1, to: 20 });
    expect(todoRowRange(2, 20, 40)).toEqual({ from: 21, to: 40 });
  });

  it("clamps the last page to the record count", () => {
    expect(todoRowRange(3, 20, 41)).toEqual({ from: 41, to: 41 });
  });

  it("stays empty when there is no record", () => {
    expect(todoRowRange(1, 20, 0)).toEqual({ from: 0, to: 0 });
  });
});

describe("todoToSqlWorkflowRow", () => {
  it("builds a pending SQL release row for the detail view", () => {
    const row = todoToSqlWorkflowRow(sqlTodo, 5329);
    expect(row.id).toBe(5329);
    expect(row.workflowName).toBe("DDL - 车辆表结构修改");
    // 待办列表只返回待审核工单，详情页据此显示审核入口。
    expect(row.status).toBe("workflow_manreviewing");
    expect(row.engineerDisplay).toBe("廖思源");
    expect(row.groupName).toBe("Test-Express");
    expect(row.createTime).toBe("2026-11-05 10:13:08");
  });
});

describe("todoToPrivilegeRow", () => {
  it("builds a pending privilege row for the detail view", () => {
    const row = todoToPrivilegeRow({ ...sqlTodo, workflowType: 1 }, 40);
    expect(row.applyId).toBe(40);
    expect(row.title).toBe("DDL - 车辆表结构修改");
    expect(row.status).toBe(0);
    expect(row.userDisplay).toBe("廖思源");
    expect(row.groupName).toBe("Test-Express");
    expect(row.createTime).toBe("2026-11-05 10:13:08");
  });
});
