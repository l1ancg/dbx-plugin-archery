/**
 * 查询权限管理的纯函数：状态标签、授权时间换算、表单校验、行归一化。
 * 与 Archery 1.14 的 sql/query_privileges.py + queryapplylist.html 对齐。
 */
import type {
  PrivilegeApplyForm,
  PrivilegeApplyRow,
  PrivilegeDetail,
  PrivilegeReviewNode,
  UserPrivilegeRow,
  ValidDatePreset,
  WorkflowLogRow,
} from "../types";

export type PrivilegeTone = "warn" | "ok" | "danger" | "muted";

export interface PrivilegeStatusMeta {
  tone: PrivilegeTone;
  labelKey: string;
}

const VALID_DAYS: Record<ValidDatePreset, number> = {
  day: 1,
  week: 7,
  month: 30,
  year: 365,
};

function toNumber(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number(value.trim());
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function toText(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "";
  return String(value);
}

/** 权限级别展示：Archery 对非 1 值一律显示 TABLE。 */
export function privTypeLabel(type: number): "DATABASE" | "TABLE" {
  return type === 1 ? "DATABASE" : "TABLE";
}

/** 只有 MySQL 系实例支持限制到表级权限（Archery 申请页同样按引擎分组）。 */
export function privTypeOptions(dbType: string): number[] {
  return dbType.trim().toLowerCase() === "mysql" ? [1, 2] : [1];
}

export function privilegeStatusMeta(status: number): PrivilegeStatusMeta {
  switch (status) {
    case 0:
      return { tone: "warn", labelKey: "privilege.status.pending" };
    case 1:
      return { tone: "ok", labelKey: "privilege.status.passed" };
    case 2:
      return { tone: "danger", labelKey: "privilege.status.rejected" };
    case 3:
      return { tone: "muted", labelKey: "privilege.status.aborted" };
    default:
      return { tone: "muted", labelKey: "privilege.status.unknown" };
  }
}

/**
 * 把「一天/一周/一月/长期」换算成 Archery 需要的到期日。
 * 刻意保持 Y-M-D 不补零，与 Archery 的 addDate() 完全一致。
 */
export function validDatePreset(preset: ValidDatePreset, base: Date = new Date()): string {
  const days = VALID_DAYS[preset] ?? 1;
  const date = new Date(base.getTime());
  date.setDate(date.getDate() + days);
  return `${date.getFullYear()}-${date.getMonth() + 1}-${date.getDate()}`;
}

/** Archery 用逗号拼接库/表清单，空串表示 none。 */
export function splitPrivilegeList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

/** 兼容 Django values() 的下划线键名与后端已归一化的驼峰键名。 */
export function normalizePrivilegeApply(row: Record<string, unknown>): PrivilegeApplyRow {
  return {
    applyId: toNumber(row.applyId ?? row.apply_id),
    title: toText(row.title),
    instanceName: toText(row.instanceName ?? row.instance__instance_name),
    dbList: toText(row.dbList ?? row.db_list),
    privType: toNumber(row.privType ?? row.priv_type),
    tableList: toText(row.tableList ?? row.table_list),
    limitNum: toNumber(row.limitNum ?? row.limit_num),
    validDate: toText(row.validDate ?? row.valid_date),
    userDisplay: toText(row.userDisplay ?? row.user_display),
    status: toNumber(row.status),
    createTime: toText(row.createTime ?? row.create_time),
    groupName: toText(row.groupName ?? row.group_name),
  };
}

/** 「我的权限」行：Archery 的 values() 键名与后端驼峰键名都兼容。 */
export function normalizeUserPrivilege(row: Record<string, unknown>): UserPrivilegeRow {
  return {
    privilegeId: toNumber(row.privilegeId ?? row.privilege_id),
    userDisplay: toText(row.userDisplay ?? row.user_display),
    instanceName: toText(row.instanceName ?? row.instance__instance_name),
    dbName: toText(row.dbName ?? row.db_name),
    privType: toNumber(row.privType ?? row.priv_type),
    tableName: toText(row.tableName ?? row.table_name),
    limitNum: toNumber(row.limitNum ?? row.limit_num),
    validDate: toText(row.validDate ?? row.valid_date),
  };
}

/** 权限有效期状态：有效 / 7 天内到期 / 已过期 / 无法解析。 */
export type PrivilegeExpiryState = "active" | "soon" | "expired" | "unknown";

export interface PrivilegeExpiry {
  state: PrivilegeExpiryState;
  /** 与到期日的自然日差：0 表示今天到期，负数表示已过期。 */
  days: number;
}

/** 「我的权限」客户端筛选条件，空串表示不限。 */
export interface UserPrivilegeFilter {
  keyword?: string;
  instanceName?: string;
  privType?: number | "";
  expiry?: PrivilegeExpiryState | "";
}

export type UserPrivilegeSortKey = "instance" | "database" | "expiry";

export interface UserPrivilegeSummary {
  total: number;
  active: number;
  soon: number;
  expired: number;
  unknown: number;
  instances: number;
}

/** 临期阈值：0~7 天内到期都算 soon。 */
const SOON_DAYS = 7;

/**
 * 解析 Archery 的 valid_date（Y-M-D，可带时间）。
 * 按本地年月日重建，避免 new Date("2026-11-05") 被当成 UTC 而在负时区差一天；
 * 2026-02-31 这类非法日期会被 Date 自动进位，这里显式拒绝。
 */
function parseLocalDate(value: string): Date | null {
  const match = /^(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T].*)?$/.exec(value.trim());
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(year, month - 1, day);
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) {
    return null;
  }
  return date;
}

/** 用 UTC 日序号做差，避开夏令时造成的 23/25 小时天。 */
function dayIndex(date: Date): number {
  return Math.floor(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / 86_400_000);
}

/** 计算权限到期状态；空值与非法日期归为 unknown。 */
export function privilegeExpiry(validDate: string, today: Date = new Date()): PrivilegeExpiry {
  const valid = parseLocalDate(validDate);
  if (!valid) return { state: "unknown", days: 0 };
  const days = dayIndex(valid) - dayIndex(today);
  if (days < 0) return { state: "expired", days };
  if (days <= SOON_DAYS) return { state: "soon", days };
  return { state: "active", days };
}

/** 复制内容：表级用「库.表」，库级只用库名。 */
export function privilegeCopyText(row: Pick<UserPrivilegeRow, "dbName" | "tableName">): string {
  const dbName = row.dbName.trim();
  const tableName = row.tableName.trim();
  if (dbName && tableName) return `${dbName}.${tableName}`;
  return dbName || tableName;
}

/** 权限级别的本地化 key；Archery 对非 1 值一律显示 TABLE。 */
export function privilegeLevelKey(
  privType: number,
): "privilege.level.database" | "privilege.level.table" {
  return privType === 1 ? "privilege.level.database" : "privilege.level.table";
}

/** 按关键字、实例、级别与到期状态过滤，返回新数组。 */
export function filterUserPrivileges(
  rows: UserPrivilegeRow[],
  filter: UserPrivilegeFilter,
  today: Date = new Date(),
): UserPrivilegeRow[] {
  const keyword = (filter.keyword ?? "").trim().toLowerCase();
  const instanceName = (filter.instanceName ?? "").trim();
  const privType = filter.privType ?? "";
  const expiry = filter.expiry ?? "";
  return rows.filter((row) => {
    if (instanceName && row.instanceName !== instanceName) return false;
    if (privType !== "" && privTypeLabel(row.privType) !== privTypeLabel(privType)) return false;
    if (expiry && privilegeExpiry(row.validDate, today).state !== expiry) return false;
    if (!keyword) return true;
    return [row.userDisplay, row.instanceName, row.dbName, row.tableName].some((value) =>
      value.toLowerCase().includes(keyword),
    );
  });
}

function compareText(a: string, b: string): number {
  const left = a.trim().toLowerCase();
  const right = b.trim().toLowerCase();
  if (left === right) return 0;
  return left < right ? -1 : 1;
}

/** 主键相同时的固定次序，保证排序结果可复现。 */
function compareTie(a: UserPrivilegeRow, b: UserPrivilegeRow): number {
  return (
    compareText(a.instanceName, b.instanceName) ||
    compareText(a.dbName, b.dbName) ||
    compareText(a.tableName, b.tableName) ||
    compareText(a.userDisplay, b.userDisplay) ||
    a.privilegeId - b.privilegeId
  );
}

/** 稳定排序：到期日排序时 unknown 恒排最后，不受升降序影响。 */
export function sortUserPrivileges(
  rows: UserPrivilegeRow[],
  key: UserPrivilegeSortKey,
  direction: "asc" | "desc",
  today: Date = new Date(),
): UserPrivilegeRow[] {
  const sign = direction === "desc" ? -1 : 1;
  return [...rows].sort((a, b) => {
    let primary = 0;
    if (key === "instance") {
      primary = compareText(a.instanceName, b.instanceName);
    } else if (key === "database") {
      primary = compareText(a.dbName, b.dbName);
    } else {
      const left = privilegeExpiry(a.validDate, today);
      const right = privilegeExpiry(b.validDate, today);
      const leftUnknown = left.state === "unknown";
      const rightUnknown = right.state === "unknown";
      if (leftUnknown !== rightUnknown) return leftUnknown ? 1 : -1;
      primary = left.days - right.days;
    }
    if (primary !== 0) return primary * sign;
    return compareTie(a, b);
  });
}

/** 汇总条数据：各状态条数与覆盖实例数。 */
export function summarizeUserPrivileges(
  rows: UserPrivilegeRow[],
  today: Date = new Date(),
): UserPrivilegeSummary {
  const summary: UserPrivilegeSummary = {
    total: rows.length,
    active: 0,
    soon: 0,
    expired: 0,
    unknown: 0,
    instances: 0,
  };
  const instances = new Set<string>();
  for (const row of rows) {
    summary[privilegeExpiry(row.validDate, today).state] += 1;
    const name = row.instanceName.trim();
    if (name) instances.add(name);
  }
  summary.instances = instances.size;
  return summary;
}

export function normalizeWorkflowLog(row: Record<string, unknown>): WorkflowLogRow {
  return {
    operationType: toText(row.operationType ?? row.operation_type_desc),
    operator: toText(row.operator ?? row.operator_display),
    time: toText(row.time ?? row.operation_time),
    info: toText(row.info ?? row.operation_info),
  };
}

/**
 * 返回缺失字段名，顺序与表单自上而下一致，UI 直接据此提示。
 * 库级权限要求 dbList；表级权限要求 dbName + tableList。
 */
export function validatePrivilegeApply(form: PrivilegeApplyForm): string[] {
  const missing: string[] = [];
  if (!form.title.trim()) missing.push("title");
  if (!form.groupName.trim()) missing.push("groupName");
  if (!form.instanceName.trim()) missing.push("instanceName");
  if (form.privType === 1) {
    if (form.dbList.filter((item) => item.trim()).length === 0) missing.push("dbList");
  } else if (form.privType === 2) {
    if (!form.dbName.trim()) missing.push("dbName");
    if (form.tableList.filter((item) => item.trim()).length === 0) missing.push("tableList");
  } else {
    missing.push("privType");
  }
  if (!form.validDatePreset) missing.push("validDate");
  if (!(form.limitNum > 0)) missing.push("limitNum");
  return missing;
}

/** 只有待审核工单需要展示审核入口。 */
export function canAuditPrivilege(status: number): boolean {
  return status === 0;
}

const REVIEW_STATES: ReadonlyArray<PrivilegeReviewNode["state"]> = [
  "auto",
  "passed",
  "current",
  "pending",
];

/** 后端布尔字段可能是 bool / 1 / "true"，统一收口。 */
function toBool(value: unknown): boolean {
  return value === true || value === 1 || value === "1" || value === "true";
}

/** 数组与逗号串都接受；审批人里会出现中文逗号。 */
function toStringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => toText(item).trim()).filter(Boolean);
  }
  return splitPrivilegeList(toText(value).replace(/，/g, ","));
}

export function normalizeReviewNode(raw: unknown): PrivilegeReviewNode | null {
  if (typeof raw !== "object" || raw === null) return null;
  const node = raw as Record<string, unknown>;
  const stateRaw = toText(node.state).trim().toLowerCase();
  const state = (REVIEW_STATES as readonly string[]).includes(stateRaw)
    ? (stateRaw as PrivilegeReviewNode["state"])
    : "pending";
  const name = toText(node.name).trim();
  // 自动通过节点在 Archery 侧只有「无需审批」文案，没有组名。
  if (!name && state !== "auto") return null;
  return { name, state, reviewers: toStringList(node.reviewers) };
}

/** 详情页解析结果归一化；缺字段一律降级为空，避免详情页整页报错。 */
export function normalizePrivilegeDetail(raw: unknown): PrivilegeDetail {
  const record =
    typeof raw === "object" && raw !== null ? (raw as Record<string, unknown>) : {};
  const nodes = record.reviewNodes ?? record.review_nodes;
  const itemKind = toText(record.itemKind ?? record.item_kind).trim().toLowerCase();
  return {
    applyId: toNumber(record.applyId ?? record.apply_id),
    title: toText(record.title),
    canReview: toBool(record.canReview ?? record.can_review),
    reviewNodes: (Array.isArray(nodes) ? nodes : [])
      .map(normalizeReviewNode)
      .filter((node): node is PrivilegeReviewNode => node !== null),
    items: toStringList(record.items),
    itemKind: itemKind === "database" || itemKind === "table" ? itemKind : "",
    lastOperation: toText(record.lastOperation ?? record.last_operation),
  };
}

/** 审批流节点文案：自动通过用本地化文案，当前节点带出审批人。 */
export function reviewNodeText(node: PrivilegeReviewNode, autoLabel: string): string {
  if (node.state === "auto") return autoLabel;
  if (node.state === "current" && node.reviewers.length > 0) {
    return `${node.name}(${node.reviewers.join(", ")})`;
  }
  return node.name;
}

/**
 * 清单来源：详情页面板是权威值，但只有类型与工单一致时才采用；
 * 类型不一致或缺清单（老版本页面/解析失败）回退列表行的 db_list / table_list。
 */
export function detailItems(detail: PrivilegeDetail, row: PrivilegeApplyRow): string[] {
  const expected = row.privType === 2 ? "table" : "database";
  if (detail.items.length > 0 && detail.itemKind === expected) {
    return detail.items;
  }
  return splitPrivilegeList(row.privType === 2 ? row.tableList : row.dbList);
}
