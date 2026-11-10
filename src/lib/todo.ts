/**
 * 待办列表的纯函数：行归一化、申请类型/审核状态映射、分页区间，
 * 以及把待办行转换成既有详情页需要的行数据。
 * 字段口径对齐 Archery 1.14 的 common/workflow.py::lists 与 sql/models.py::WorkflowAudit。
 */
import type {
  PrivilegeApplyRow,
  SqlWorkflowRow,
  WorkflowTodoRow,
} from "../types";

export type TodoTone = "ok" | "info" | "warn" | "danger" | "muted";

export interface TodoStatusMeta {
  tone: TodoTone;
  labelKey: string;
}

/** Archery WorkflowStatus：0 待审核 / 1 审核通过 / 2 审核不通过。 */
const STATUS_META: Record<number, TodoStatusMeta> = {
  0: { tone: "info", labelKey: "todo.statusWaiting" },
  1: { tone: "ok", labelKey: "todo.statusPass" },
  2: { tone: "danger", labelKey: "todo.statusReject" },
};

/** 未知状态（新版新增或空值）统一降级为灰色标签。 */
export function todoStatusMeta(status: number): TodoStatusMeta {
  return STATUS_META[status] ?? { tone: "muted", labelKey: "todo.statusUnknown" };
}

/** Archery WorkflowType：1 查询权限 / 2 SQL 上线 / 3 数据归档。 */
export function todoTypeLabelKey(type: number): string {
  switch (type) {
    case 1:
      return "todo.typeQuery";
    case 2:
      return "todo.typeSql";
    case 3:
      return "todo.typeArchive";
    default:
      return "todo.typeUnknown";
  }
}

/** 列表页「申请类型」下拉：0 为不过滤，与 Archery workflow.html 的取值一致。 */
export const todoTypeOptions: Array<{ value: number; labelKey: string }> = [
  { value: 0, labelKey: "todo.typeAll" },
  { value: 1, labelKey: "todo.typeQuery" },
  { value: 2, labelKey: "todo.typeSql" },
  { value: 3, labelKey: "todo.typeArchive" },
];

function toText(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "";
  return String(value);
}

function toNumber(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number(value.trim());
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

/** 兼容 Django values() 的下划线键名与 sidecar 已归一化的驼峰键名。 */
export function normalizeTodo(row: Record<string, unknown>): WorkflowTodoRow {
  return {
    auditId: toNumber(row.auditId ?? row.audit_id),
    workflowType: toNumber(row.workflowType ?? row.workflow_type),
    title: toText(row.title ?? row.workflowTitle ?? row.workflow_title),
    applicant: toText(row.applicant ?? row.createUserDisplay ?? row.create_user_display),
    createTime: toText(row.createTime ?? row.create_time),
    status: toNumber(row.status ?? row.currentStatus ?? row.current_status),
    groupName: toText(row.groupName ?? row.group_name),
    currentAudit: toText(row.currentAudit ?? row.current_audit),
    auditGroups: toText(row.auditGroups ?? row.audit_auth_groups),
  };
}

/**
 * 页脚「显示第 X 到第 Y 条记录」的区间；越界页码收敛到最后一条，
 * 空列表返回 0/0，由 UI 决定是否渲染页脚。
 */
export function todoRowRange(
  page: number,
  pageSize: number,
  total: number,
): { from: number; to: number } {
  if (total <= 0 || pageSize <= 0) return { from: 0, to: 0 };
  const safePage = Math.max(1, page);
  const from = Math.min((safePage - 1) * pageSize + 1, total);
  const to = Math.min(safePage * pageSize, total);
  return { from, to };
}

/**
 * 待办 → SQL 上线详情页的行数据：列表只给出审批记录，
 * 实例/库/类型等由详情页重新拉取，这里只兜底标题、发起人与组。
 */
export function todoToSqlWorkflowRow(todo: WorkflowTodoRow, workflowId: number): SqlWorkflowRow {
  return {
    id: workflowId,
    workflowName: todo.title,
    engineerDisplay: todo.applicant,
    // 待办列表本身只返回待审核工单，详情页据此展示审核入口。
    status: "workflow_manreviewing",
    isBackup: false,
    createTime: todo.createTime,
    instanceName: "",
    dbName: "",
    groupName: todo.groupName,
    syntaxType: 0,
    exportFormat: "",
  };
}

/** 待办 → 权限申请详情页的行数据，字段同样以详情页解析结果为准。 */
export function todoToPrivilegeRow(todo: WorkflowTodoRow, applyId: number): PrivilegeApplyRow {
  return {
    applyId,
    title: todo.title,
    instanceName: "",
    dbList: "",
    privType: 0,
    tableList: "",
    limitNum: 0,
    validDate: "",
    userDisplay: todo.applicant,
    status: 0,
    createTime: todo.createTime,
    groupName: todo.groupName,
  };
}
