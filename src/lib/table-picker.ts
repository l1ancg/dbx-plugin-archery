/**
 * 表名搜索框的纯逻辑：Archery 的"查看表结构"是一个带搜索的下拉，
 * 候选来自当前库的表列表，前缀命中的排在前面。
 */
export function filterTables(tables: string[], keyword: string, limit = 200): string[] {
  const term = keyword.trim().toLowerCase();
  if (!term) {
    return [...tables]
      .sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()))
      .slice(0, limit);
  }
  const matched = tables
    .map((name) => ({ name, lower: name.toLowerCase() }))
    .filter((item) => item.lower.includes(term))
    .sort((a, b) => {
      const prefix = Number(!a.lower.startsWith(term)) - Number(!b.lower.startsWith(term));
      if (prefix !== 0) return prefix;
      const offset = a.lower.indexOf(term) - b.lower.indexOf(term);
      if (offset !== 0) return offset;
      return a.lower.localeCompare(b.lower);
    })
    .map((item) => item.name);
  return matched.slice(0, limit);
}

/**
 * 候选窗口：过滤后只渲染前 limit 条，并给出被折叠的条数。
 * 实例 / 库这类候选可能上千（原生 select 一次铺开会明显卡），
 * 这里用「只画前 N 条 + 关键字收窄」换秒开。
 */
export function optionWindow(
  options: string[],
  keyword: string,
  limit = 200,
): { visible: string[]; hidden: number } {
  const matched = filterTables(options, keyword, Number.MAX_SAFE_INTEGER);
  return {
    visible: matched.slice(0, limit),
    hidden: Math.max(0, matched.length - limit),
  };
}

/** 上下键在候选列表里移动，越界时环绕；空列表返回 -1。 */
export function moveIndex(current: number, delta: number, total: number): number {
  if (total <= 0) return -1;
  if (current < 0) return delta > 0 ? 0 : total - 1;
  return (current + delta + total) % total;
}
