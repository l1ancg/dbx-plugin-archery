import type { QueryLogRow } from "../types";

/**
 * 查询历史的展示与筛选逻辑：
 * 表格列定义、本地兜底筛选、筛选项去重。
 */
export const historyColumns: readonly string[] = [
  "history.colOperation",
  "history.colUser",
  "history.colInstance",
  "history.colDatabase",
  "history.colTime",
  "history.colSql",
  "history.colEffectRow",
  "history.colCost",
];

export interface HistoryFilter {
  instance: string;
  db: string;
  keyword: string;
}

export function filterLogs(rows: QueryLogRow[], filter: HistoryFilter): QueryLogRow[] {
  const keyword = filter.keyword.trim().toLowerCase();
  return rows.filter((row) => {
    if (filter.instance && row.instanceName !== filter.instance) return false;
    if (filter.db && row.dbName !== filter.db) return false;
    if (!keyword) return true;
    const haystack = [row.sql, row.alias, row.userDisplay, row.instanceName, row.dbName]
      .join("\n")
      .toLowerCase();
    return haystack.includes(keyword);
  });
}

/** 页码条里的一项：数字页码或省略号占位。 */
export type PageItem = number | "gap";

/**
 * 页码窗口：当前页居中取 span 个连续页码，首末页常驻。
 * 与首/末页相邻时不出省略号（1 2 3 … 而不是 1 … 3），窄宽度下也不会撑爆页脚。
 */
export function pageWindow(page: number, pageCount: number, span = 5): PageItem[] {
  const pages = Math.max(1, Math.floor(pageCount) || 1);
  const size = Math.max(1, Math.floor(span) || 1);
  const current = Math.min(Math.max(1, Math.floor(page) || 1), pages);
  if (pages <= size + 2) {
    return Array.from({ length: pages }, (_, index) => index + 1);
  }
  let start = Math.max(1, current - Math.floor(size / 2));
  const end = Math.min(pages, start + size - 1);
  start = Math.max(1, end - size + 1);
  const items: PageItem[] = [];
  if (start > 1) {
    items.push(1);
    if (start > 2) items.push("gap");
  }
  for (let value = start; value <= end; value += 1) items.push(value);
  if (end < pages) {
    if (end < pages - 1) items.push("gap");
    items.push(pages);
  }
  return items;
}

/** 页脚「显示第 X 到第 Y 条记录」的区间；空结果显示 0/0，越界页码收敛到末页。 */
export function logRowRange(
  page: number,
  pageSize: number,
  total: number,
): { from: number; to: number } {
  if (total <= 0 || pageSize <= 0) return { from: 0, to: 0 };
  const lastPage = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(1, Math.floor(page) || 1), lastPage);
  return {
    from: (safePage - 1) * pageSize + 1,
    to: Math.min(safePage * pageSize, total),
  };
}

/** 查询时间拆成日期与时刻：同一格里做两级字重，不额外占行高。 */
export function splitTimestamp(value: string): { date: string; time: string } {
  const text = value.trim();
  if (!text) return { date: "", time: "" };
  const parts = text.split(/[ T]/);
  return { date: parts[0] ?? "", time: parts[1] ?? "" };
}

export type CostTone = "normal" | "warn" | "slow";

/** 耗时分级：≥1s 记为慢查询、≥0.3s 记为偏慢，列表里按色阶提示。 */
export function costTone(value: string): CostTone {
  const seconds = Number.parseFloat(value);
  if (!Number.isFinite(seconds)) return "normal";
  if (seconds >= 1) return "slow";
  if (seconds >= 0.3) return "warn";
  return "normal";
}

export type SqlKind = "read" | "write" | "ddl" | "other";

const READ_KEYWORDS = new Set(["select", "show", "desc", "describe", "explain", "with"]);
const WRITE_KEYWORDS = new Set(["insert", "update", "delete", "replace", "merge"]);
const DDL_KEYWORDS = new Set(["create", "alter", "drop", "truncate", "rename"]);

/** 语句类型：列表左侧色条与提示按读 / 写 / DDL 分色，未知语句归 other。 */
export function sqlKind(sql: string): SqlKind {
  const lead = /^\s*(\/\*[\s\S]*?\*\/|--[^\n]*(?:\n|$)|#[^\n]*(?:\n|$))/;
  let text = sql;
  while (lead.test(text)) text = text.replace(lead, "");
  const keyword = /^[a-z]+/i.exec(text.trim())?.[0].toLowerCase() ?? "";
  if (READ_KEYWORDS.has(keyword)) return "read";
  if (WRITE_KEYWORDS.has(keyword)) return "write";
  if (DDL_KEYWORDS.has(keyword)) return "ddl";
  return "other";
}

/** 行数加千分位，扫读时量级一眼可比。 */
export function formatCount(value: number): string {
  if (!Number.isFinite(value)) return "-";
  return String(Math.trunc(value)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

export function facets(rows: QueryLogRow[]): { instances: string[]; databases: string[] } {
  const instances = new Set<string>();
  const databases = new Set<string>();
  for (const row of rows) {
    if (row.instanceName) instances.add(row.instanceName);
    if (row.dbName) databases.add(row.dbName);
  }
  return {
    instances: [...instances].sort(),
    databases: [...databases].sort(),
  };
}
