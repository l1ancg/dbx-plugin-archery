import type { Cell } from "../types";

export function formatCell(value: Cell | undefined): string {
  if (value === null || value === undefined) return "NULL";
  if (typeof value === "boolean") return value ? "true" : "false";
  return String(value);
}

/** 结果搜索的命中片段：match 为 true 的片段在网格里加高亮。 */
export interface HighlightSegment {
  text: string;
  match: boolean;
}

/**
 * 结果搜索：按渲染文本做大小写不敏感的子串匹配，只扫传入的列下标（可见列）。
 * query 为空时返回全部行下标，与「没搜索」等价。
 */
export function matchRowIndexes(
  rows: readonly Cell[][],
  indexes: readonly number[],
  query: string,
): number[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return rows.map((_, index) => index);
  const hits: number[] = [];
  rows.forEach((row, rowIndex) => {
    for (const index of indexes) {
      if (formatCell(row[index]).toLowerCase().includes(needle)) {
        hits.push(rowIndex);
        return;
      }
    }
  });
  return hits;
}

/** 把文本切成命中 / 未命中片段，保留原文大小写；query 为空时整段不命中。 */
export function highlightSegments(text: string, query: string): HighlightSegment[] {
  const needle = query.trim();
  if (!needle || !text) return text ? [{ text, match: false }] : [];
  const haystack = text.toLowerCase();
  const lowered = needle.toLowerCase();
  const segments: HighlightSegment[] = [];
  let from = 0;
  let at = haystack.indexOf(lowered, from);
  while (at !== -1) {
    if (at > from) segments.push({ text: text.slice(from, at), match: false });
    segments.push({ text: text.slice(at, at + needle.length), match: true });
    from = at + needle.length;
    at = haystack.indexOf(lowered, from);
  }
  if (from < text.length) segments.push({ text: text.slice(from), match: false });
  return segments;
}

export function escapeDelimited(value: Cell | undefined, delimiter: string): string {
  const text = value === null || value === undefined ? "" : String(value);
  const needsQuote =
    text.includes(delimiter) ||
    text.includes('"') ||
    text.includes("\n") ||
    text.includes("\r");
  if (!needsQuote) return text;
  return `"${text.replace(/"/g, '""')}"`;
}

export function toDelimited(
  columns: string[],
  rows: Cell[][],
  delimiter = ",",
): string {
  const lines = [columns.map((name) => escapeDelimited(name, delimiter)).join(delimiter)];
  for (const row of rows) {
    lines.push(row.map((value) => escapeDelimited(value, delimiter)).join(delimiter));
  }
  return lines.join("\r\n");
}

export const toCsv = (columns: string[], rows: Cell[][]): string =>
  toDelimited(columns, rows, ",");

export const toTsv = (columns: string[], rows: Cell[][]): string =>
  toDelimited(columns, rows, "\t");

export function toMarkdown(columns: string[], rows: Cell[][]): string {
  const escape = (value: Cell | undefined) =>
    formatCell(value).replace(/\|/g, "\\|").replace(/\r?\n/g, "<br>");
  const lines = [
    `| ${columns.map(escape).join(" | ")} |`,
    `| ${columns.map(() => "---").join(" | ")} |`,
  ];
  for (const row of rows) {
    lines.push(`| ${row.map(escape).join(" | ")} |`);
  }
  return lines.join("\n");
}

export interface PageSlice<T> {
  items: T[];
  page: number;
  pageCount: number;
  total: number;
}

/** 结果页每页条数候选；实际可选项再按查询目标行数上限裁剪。 */
export const PAGE_SIZE_PRESETS: readonly number[] = [30, 50, 100, 200, 500, 1000];

/**
 * 结果页可选每页条数：不超过查询目标行数上限，上限本身补成一档（如 300 → 200/300）。
 * limit ≤ 0 表示不限（Archery 的 -1），此时用完整候选。
 */
export function pageSizeChoices(
  limit: number,
  presets: readonly number[] = PAGE_SIZE_PRESETS,
): number[] {
  const cap = Number.isFinite(limit) && limit > 0 ? Math.max(1, Math.floor(limit)) : 0;
  if (cap === 0) return [...presets];
  const sizes = presets.filter((size) => size <= cap);
  if (!sizes.includes(cap)) sizes.push(cap);
  return sizes.sort((a, b) => a - b);
}

/** 默认每页 30 条；上限更小时退到不超过上限的最大一档。 */
export function defaultPageSize(
  limit: number,
  preferred = 30,
  presets: readonly number[] = PAGE_SIZE_PRESETS,
): number {
  const choices = pageSizeChoices(limit, presets);
  const smaller = choices.filter((size) => size <= preferred);
  return smaller.length > 0 ? smaller[smaller.length - 1] : choices[0];
}

export function paginate<T>(items: T[], page: number, pageSize: number): PageSlice<T> {
  const size = Math.max(1, Math.floor(pageSize));
  const total = items.length;
  const pageCount = Math.max(1, Math.ceil(total / size));
  const current = Math.min(Math.max(1, Math.floor(page)), pageCount);
  const start = (current - 1) * size;
  return {
    items: items.slice(start, start + size),
    page: current,
    pageCount,
    total,
  };
}

export function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return String(value);
  const text = String(Math.trunc(value));
  const negative = text.startsWith("-");
  const digits = negative ? text.slice(1) : text;
  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return negative ? `-${grouped}` : grouped;
}

export function formatElapsed(milliseconds: number): string {
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return "0 ms";
  if (milliseconds < 1000) return `${Math.round(milliseconds)} ms`;
  return `${(milliseconds / 1000).toFixed(2)} s`;
}

export function truncate(text: string, max = 80): string {
  if (text.length <= max) return text;
  return `${text.slice(0, Math.max(0, max - 1))}…`;
}
