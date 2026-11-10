/**
 * SQL 上线工单的纯函数：状态映射、行归一化、按钮可用性与提交表单校验。
 * 与 Archery 1.14 的 sql/sql_workflow.py + sql/models.py 对齐。
 */
import type {
  PrivilegeReviewNode,
  SqlCheckSummary,
  SqlReviewRow,
  SqlSubmitForm,
  SqlWorkflowDetail,
  SqlWorkflowRow,
} from "../types";
import { normalizeReviewNode } from "./privilege";
import { normalizeInfoFields } from "./workflow-detail";

export type WorkflowTone = "ok" | "warn" | "danger" | "muted";

/**
 * 列表状态筛选口径：「待我审核」只可能是待审核状态，
 * 页面上的状态筛选对它无效；申请工单按用户选择的状态过滤。
 */
export function workflowScopeStatus(audit: boolean, status: string): string {
  return audit ? "workflow_manreviewing" : status;
}

export interface WorkflowStatusMeta {
  tone: WorkflowTone;
  labelKey: string;
}

/** Archery SqlWorkflow.status 的 9 个取值（SQL_WORKFLOW_CHOICES）。 */
const STATUS_META: Record<string, WorkflowStatusMeta> = {
  workflow_manreviewing: { tone: "warn", labelKey: "workflow.status.manreviewing" },
  workflow_review_pass: { tone: "ok", labelKey: "workflow.status.reviewPass" },
  workflow_timingtask: { tone: "warn", labelKey: "workflow.status.timingtask" },
  workflow_queuing: { tone: "warn", labelKey: "workflow.status.queuing" },
  workflow_executing: { tone: "warn", labelKey: "workflow.status.executing" },
  workflow_finish: { tone: "ok", labelKey: "workflow.status.finish" },
  workflow_abort: { tone: "muted", labelKey: "workflow.status.abort" },
  workflow_autoreviewwrong: { tone: "danger", labelKey: "workflow.status.autoreviewwrong" },
  workflow_exception: { tone: "danger", labelKey: "workflow.status.exception" },
};

/** 未知状态（新版 Archery 新增状态或空值）统一降级为灰色标签。 */
export function workflowStatusMeta(status: string): WorkflowStatusMeta {
  return STATUS_META[status.trim()] ?? { tone: "muted", labelKey: "workflow.status.unknown" };
}

/**
 * 工单类型展示：Archery 存整数（0 其他 / 1 DDL / 2 DML / 3 离线导出），
 * 部分接口会直接给 "DDL"/"DML" 字符串，两种都收。
 * OFFLINE 由 UI 换成「离线导出」，空串由 UI 显示破折号。
 */
export function syntaxTypeLabel(value: unknown): "DDL" | "DML" | "OFFLINE" | "" {
  const text =
    typeof value === "number"
      ? String(value)
      : typeof value === "string"
        ? value.trim().toUpperCase()
        : "";
  switch (text) {
    case "1":
    case "DDL":
      return "DDL";
    case "2":
    case "DML":
      return "DML";
    case "3":
    case "OFFLINE":
      return "OFFLINE";
    default:
      return "";
  }
}

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

/** 后端布尔字段可能是 bool / 1 / "true" / "True"。 */
function toBool(value: unknown): boolean {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") {
    const text = value.trim().toLowerCase();
    return text === "true" || text === "1";
  }
  return false;
}

/** 把 DDL/DML 文案与整数互转，供归一化与展示复用。 */
function syntaxTypeCode(value: unknown): number {
  switch (syntaxTypeLabel(value)) {
    case "DDL":
      return 1;
    case "DML":
      return 2;
    case "OFFLINE":
      return 3;
    default:
      return 0;
  }
}

/** 兼容 Django values() 的下划线键名与后端已归一化的驼峰键名。 */
export function normalizeSqlWorkflow(row: Record<string, unknown>): SqlWorkflowRow {
  return {
    id: toNumber(row.id),
    workflowName: toText(row.workflowName ?? row.workflow_name),
    engineerDisplay: toText(row.engineerDisplay ?? row.engineer_display),
    status: toText(row.status),
    isBackup: toBool(row.isBackup ?? row.is_backup),
    createTime: toText(row.createTime ?? row.create_time),
    instanceName: toText(row.instanceName ?? row.instance__instance_name),
    dbName: toText(row.dbName ?? row.db_name),
    groupName: toText(row.groupName ?? row.group_name),
    syntaxType: syntaxTypeCode(row.syntaxType ?? row.syntax_type),
    exportFormat: toText(row.exportFormat ?? row.export_format),
  };
}

/** 待审核的工单才展示审核入口。 */
export function canAuditWorkflow(status: string): boolean {
  return status === "workflow_manreviewing";
}

/** 审核通过（含已定时）的工单可执行。 */
export function canExecuteWorkflow(status: string): boolean {
  return status === "workflow_review_pass" || status === "workflow_timingtask";
}

/** 结束/终止/异常的工单不再允许终止流程。 */
export function canCancelWorkflow(status: string): boolean {
  return (
    status === "workflow_manreviewing" ||
    status === "workflow_review_pass" ||
    status === "workflow_timingtask" ||
    status === "workflow_queuing"
  );
}

function parseDateTime(value: string): Date | null {
  const text = value.trim();
  if (!text) return null;
  // datetime-local 给 "2026-11-02T12:00"，Archery 给 "2026-11-02 12:00"。
  const date = new Date(text.replace(" ", "T"));
  return Number.isNaN(date.getTime()) ? null : date;
}

/**
 * 返回缺失/不合法的字段名，顺序与表单自上而下一致，UI 直接据此提示。
 * 执行时间窗与 Archery 网页一致：最短 60 分钟，且不能早于当前时间。
 */
export function validateSqlSubmit(form: SqlSubmitForm, now: Date = new Date()): string[] {
  const issues: string[] = [];
  const title = form.title.trim();
  if (!title) {
    issues.push("title");
  } else if (title.length > 50) {
    issues.push("titleLength");
  }
  if (!(form.groupId > 0)) issues.push("groupId");
  if (!(form.instanceId > 0)) issues.push("instanceId");
  if (!form.dbName.trim()) issues.push("dbName");
  if (!form.sql.trim()) issues.push("sql");

  const start = parseDateTime(form.runDateStart);
  const end = parseDateTime(form.runDateEnd);
  if (start && end) {
    if (end.getTime() - start.getTime() < 60 * 60 * 1000) {
      issues.push("runDateRange");
    } else if (start.getTime() < now.getTime()) {
      issues.push("runDatePast");
    }
  }
  return issues;
}

/** 统一解析 detail_content 的审核/执行结果行。 */
export function normalizeReviewRow(raw: unknown): SqlReviewRow {
  const row =
    typeof raw === "object" && raw !== null ? (raw as Record<string, unknown>) : {};
  return {
    sequence: toText(row.sequence),
    stage: toText(row.stage),
    status: toText(row.status ?? row.stagestatus),
    errorLevel: toNumber(row.errorLevel ?? row.errlevel),
    errorMessage: toText(row.errorMessage ?? row.errormessage),
    sql: toText(row.sql),
    affectedRows: toNumber(row.affectedRows ?? row.affected_rows),
    backupDbName: toText(row.backupDbName ?? row.backup_dbname),
    executeTime: toText(row.executeTime ?? row.execute_time),
  };
}

function normalizeReviewRows(value: unknown): SqlReviewRow[] {
  return Array.isArray(value) ? value.map(normalizeReviewRow) : [];
}

function normalizeReviewNodes(value: unknown): PrivilegeReviewNode[] {
  if (!Array.isArray(value)) return [];
  return value
    .map(normalizeReviewNode)
    .filter((node): node is PrivilegeReviewNode => node !== null);
}

/** 详情归一化：缺字段一律降级为空，避免详情页整页报错。 */
export function normalizeSqlWorkflowDetail(raw: unknown): SqlWorkflowDetail {
  const record =
    typeof raw === "object" && raw !== null ? (raw as Record<string, unknown>) : {};
  return {
    workflowId: toNumber(record.workflowId ?? record.workflow_id),
    title: toText(record.title),
    status: toText(record.status),
    reviewNodes: normalizeReviewNodes(record.reviewNodes ?? record.review_nodes),
    infoFields: normalizeInfoFields(record.infoFields ?? record.info_fields),
    lastOperation: toText(record.lastOperation ?? record.last_operation),
    reviewRows: normalizeReviewRows(
      record.reviewRows ?? record.review_rows ?? record.rows,
    ),
  };
}

function countOr(value: unknown, fallback: number): number {
  if (value === null || value === undefined || value === "") return fallback;
  return toNumber(value);
}

/** SQL 检测汇总：优先用接口给的总数，缺失时按 errlevel 自行统计。 */
export function normalizeCheckResult(raw: unknown): SqlCheckSummary {
  const record =
    typeof raw === "object" && raw !== null ? (raw as Record<string, unknown>) : {};
  const rows = normalizeReviewRows(record.rows);
  const errors = rows.filter((row) => row.errorLevel === 2).length;
  const warnings = rows.filter((row) => row.errorLevel === 1).length;
  return {
    errorCount: countOr(record.errorCount ?? record.error_count, errors),
    warningCount: countOr(record.warningCount ?? record.warning_count, warnings),
    rows,
  };
}

/** 执行时间窗展示：两端都填时才拼 "起 ~ 止"。 */
export function runDateRangeText(start: string, end: string): string {
  const from = start.trim();
  const to = end.trim();
  if (!from && !to) return "";
  return `${from || "-"} ~ ${to || "-"}`;
}

/**
 * Archery 把同一语句的多条检查结果用 ". " 拼进 errormessage，
 * 这里按句子边界拆开，供界面逐行展示每条错误/警告。
 * 只在「句号后跟空白」处断行，避免拆坏 v1.2、3.14、'a.b' 这类写法。
 */
export function splitAuditMessages(message: string | null | undefined): string[] {
  if (!message) return [];
  return message
    .split(/\r?\n/)
    .flatMap((line) => line.split(/(?<=[.。])\s+/))
    .map((item) => item.trim())
    .filter(Boolean);
}

/**
 * 从编辑器内容里摘掉文件导入的那段 SQL，恢复导入前的文本。
 * 优先匹配最后一处（同一文件多次导入时逐次回退）；
 * removed=false 说明片段已被改写，调用方只清文件名标记即可。
 */
export function stripImportedSql(
  current: string,
  imported: string,
): { sql: string; removed: boolean } {
  if (!imported.trim()) return { sql: current, removed: false };
  const index = current.lastIndexOf(imported);
  if (index < 0) return { sql: current, removed: false };
  const merged = `${current.slice(0, index)}${current.slice(index + imported.length)}`.trimEnd();
  return { sql: merged.trim() ? merged : "", removed: true };
}
