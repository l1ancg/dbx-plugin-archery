/**
 * 「列配置」面板的纯函数层：列显隐状态 + 列搜索。
 * 表格渲染与浮层定位留在组件里，这里只处理不可变状态与匹配规则，便于单测。
 */
export type ColumnMatchMode = "fuzzy" | "prefix" | "exact";

export interface PickableColumn {
  /** 原始列序（0 基）；展示时用 index + 1 渲染 #N。 */
  index: number;
  name: string;
  /** 第二行说明：列类型；没有类型时为空串。 */
  note: string;
}

/** 默认所有列可见。 */
export function allVisible(count: number): boolean[] {
  const size = Number.isFinite(count) ? Math.max(0, Math.floor(count)) : 0;
  return Array.from({ length: size }, () => true);
}

/** 切换一列显隐；只剩最后一列可见时拒绝隐藏。 */
export function toggleVisible(visible: readonly boolean[], index: number): boolean[] {
  const next = [...visible];
  if (index < 0 || index >= next.length) return next;
  if (next[index] && next.filter(Boolean).length === 1) return next;
  next[index] = !next[index];
  return next;
}

/** 可见列的下标（保持原始列序）：表头、单元格与导出共用。 */
export function visibleIndexes(visible: readonly boolean[]): number[] {
  const indexes: number[] = [];
  visible.forEach((on, index) => {
    if (on) indexes.push(index);
  });
  return indexes;
}

export function hiddenCount(visible: readonly boolean[]): number {
  return visible.filter((on) => !on).length;
}

/** 按可见下标挑出列名或一行数据。 */
export function pickColumns<T>(items: readonly T[], indexes: readonly number[]): T[] {
  return indexes.map((index) => items[index]);
}

function hit(field: string, needle: string, mode: ColumnMatchMode): boolean {
  if (!field) return false;
  if (mode === "prefix") return field.startsWith(needle);
  if (mode === "exact") return field === needle;
  return field.includes(needle);
}

/**
 * 过滤列清单：query 为空时返回全部列并保留原始列序。
 * 列名与列类型都参与匹配，三种模式统一按同一份 query 比较（忽略大小写）。
 */
export function matchColumns(
  columns: readonly string[],
  types: readonly (string | undefined)[] | undefined,
  query: string,
  mode: ColumnMatchMode,
): PickableColumn[] {
  const needle = query.trim().toLowerCase();
  const rows: PickableColumn[] = columns.map((name, index) => ({
    index,
    name,
    note: (types?.[index] ?? "").trim(),
  }));
  if (!needle) return rows;
  return rows.filter(
    (column) =>
      hit(column.name.toLowerCase(), needle, mode) ||
      hit(column.note.toLowerCase(), needle, mode),
  );
}
