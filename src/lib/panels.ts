import type { DescribePanelTab, PanelTab, ResultPanelTab } from "../types";

/** 打开表结构的目标：复用键就是这三项。 */
export interface DescribeTarget {
  instanceName: string;
  dbName: string;
  tableName: string;
}

/** 系统 tab 的 id 固定，便于直接激活/判断。 */
export const MESSAGE_PANEL_ID = "panel-message";
export const HISTORY_PANEL_ID = "panel-history";
/** 新建编辑器 tab 时默认激活的那条结果 tab。 */
export const DEFAULT_RESULT_PANEL_ID = "panel-result-1";

export function describePanelId(target: DescribeTarget): string {
  return `panel-describe-${target.instanceName}-${target.dbName}-${target.tableName}`;
}

/** 系统 tab（消息 / 查询历史）：固定在最前，不可关闭、不可拖动。 */
export function systemPanels(): PanelTab[] {
  return [
    { id: MESSAGE_PANEL_ID, kind: "message" },
    { id: HISTORY_PANEL_ID, kind: "history" },
  ];
}

/** 新建一条结果 tab；index 是「结果 N」的 N。 */
export function createResultPanel(index: number): ResultPanelTab {
  return {
    id: `panel-result-${index}`,
    kind: "result",
    index,
    sql: "",
    limitNum: 100,
    running: false,
    result: null,
    error: "",
    errorKind: "",
    info: "",
  };
}

export function createDescribePanel(target: DescribeTarget): DescribePanelTab {
  return {
    id: describePanelId(target),
    kind: "describe",
    instanceName: target.instanceName,
    dbName: target.dbName,
    tableName: target.tableName,
    table: `${target.instanceName} / ${target.dbName} / ${target.tableName}`,
    loading: true,
    error: "",
    data: null,
    elapsed: 0,
    fetchedAt: "",
  };
}

/** 编辑器 tab 的初始面板：系统 tab + 第一条空结果 tab。 */
export function initialPanels(): PanelTab[] {
  return [...systemPanels(), createResultPanel(1)];
}

/** 下一个结果序号：现有结果 tab 的最大序号 + 1（关掉的那条序号会被回收，但不会与在册的重名）。 */
export function nextResultIndex(panels: PanelTab[]): number {
  const highest = panels.reduce(
    (max, panel) => (panel.kind === "result" ? Math.max(max, panel.index) : max),
    0,
  );
  return highest + 1;
}

/** 动态 tab 追加到末尾；系统 tab 永远保持在前。 */
export function appendPanel(panels: PanelTab[], panel: PanelTab): PanelTab[] {
  return [...panels, panel];
}

/**
 * 执行 SQL 时要落到的结果 tab：优先当前激活的那条，否则退回最后一条；
 * 全被关掉（用户按下「−」）时返回 undefined，由调用方新建一条。
 */
export function targetResultPanel(
  panels: PanelTab[],
  activeId: string,
): ResultPanelTab | undefined {
  const current = panels.find((panel) => panel.id === activeId);
  if (current?.kind === "result") return current;
  const results = panels.filter((panel): panel is ResultPanelTab => panel.kind === "result");
  return results[results.length - 1];
}

/** 同一张表已经打开过：返回命中的 tab，由调用方决定原地刷新。 */
export function findDescribePanel(
  panels: PanelTab[],
  target: DescribeTarget,
): DescribePanelTab | undefined {
  return panels.find(
    (panel): panel is DescribePanelTab =>
      panel.kind === "describe" &&
      panel.instanceName === target.instanceName &&
      panel.dbName === target.dbName &&
      panel.tableName === target.tableName,
  );
}

/**
 * 表结构 tab 的标题：默认只用表名；与别的库重名时补库名，
 * 库名也撞（同库同表、不同实例）时再补实例名。
 */
export function describeTitle(panels: PanelTab[], target: DescribeTarget): string {
  const sameName = panels.filter(
    (panel): panel is DescribePanelTab =>
      panel.kind === "describe" &&
      panel.tableName === target.tableName &&
      !(panel.instanceName === target.instanceName && panel.dbName === target.dbName),
  );
  if (sameName.length === 0) return target.tableName;
  if (sameName.some((panel) => panel.dbName === target.dbName)) {
    return `${target.instanceName}.${target.dbName}.${target.tableName}`;
  }
  return `${target.dbName}.${target.tableName}`;
}

/** 只有动态 tab（结果 / 表结构）可关闭、可拖动。 */
export function isClosable(panel: PanelTab): panel is ResultPanelTab | DescribePanelTab {
  return panel.kind === "result" || panel.kind === "describe";
}

/** 关闭一条 tab：关掉激活项时激活右侧邻居，没有右侧就退回左侧。 */
export function closePanel(
  panels: PanelTab[],
  activeId: string,
  id: string,
): { panels: PanelTab[]; activeId: string } {
  const index = panels.findIndex((panel) => panel.id === id);
  if (index < 0 || !isClosable(panels[index])) return { panels, activeId };

  const next = panels.filter((panel) => panel.id !== id);
  if (id !== activeId) return { panels: next, activeId };

  const fallback = next[index] ?? next[index - 1] ?? next[0];
  return { panels: next, activeId: fallback?.id ?? "" };
}

/** 拖拽重排：只允许动态 tab 之间互调，系统 tab 位置不动。 */
export function movePanel(panels: PanelTab[], sourceId: string, targetId: string): PanelTab[] {
  const source = panels.find((panel) => panel.id === sourceId);
  const target = panels.find((panel) => panel.id === targetId);
  if (!source || !target || sourceId === targetId) return panels;
  if (!isClosable(source) || !isClosable(target)) return panels;

  const next = panels.filter((panel) => panel.id !== sourceId);
  const at = next.findIndex((panel) => panel.id === targetId);
  next.splice(at < 0 ? next.length : at, 0, source);
  return next;
}
