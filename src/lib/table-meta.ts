import type { Cell, TableGrid } from "../types";

/** 表信息字段的展示顺序，与 Archery 页面一致；未知字段排在末尾。 */
export const META_FIELD_ORDER = [
  "table_name",
  "engine",
  "row_format",
  "table_rows",
  "avg_row_length",
  "max_data_length",
  "data_length",
  "index_length",
  "data_total",
  "data_free",
  "auto_increment",
  "table_collation",
  "create_time",
  "update_time",
  "check_time",
  "table_comment",
];

/** 空值统一显示为 "-"，与 Archery 页面一致。 */
export function nodeText(value: Cell | undefined): string {
  if (value === null || value === undefined) return "-";
  const text = String(value).trim();
  return text === "" ? "-" : text;
}

/** 表信息是「列名 + 单行值」，按固定字段顺序转成键值对。 */
export function metaPairs(grid: TableGrid): Array<{ key: string; value: string }> {
  const columns = grid.columns ?? [];
  if (columns.length === 0) return [];
  const row = grid.rows?.[0] ?? [];
  const rank = (key: string): number => {
    const index = META_FIELD_ORDER.indexOf(key);
    return index === -1 ? META_FIELD_ORDER.length : index;
  };
  return columns
    .map((key, index) => ({ key, value: nodeText(row[index]), index }))
    .sort((a, b) => rank(a.key) - rank(b.key) || a.index - b.index)
    .map(({ key, value }) => ({ key, value }));
}

export function rowTexts(grid: TableGrid, row: Cell[]): string[] {
  return (grid.columns ?? []).map((_, index) => nodeText(row[index]));
}
