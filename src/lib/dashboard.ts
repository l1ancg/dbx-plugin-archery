import type { InstanceItem } from "../types";

export interface TypeCount {
  dbType: string;
  count: number;
}

export interface EnvTypeSeries {
  dbType: string;
  values: number[];
}

export interface EnvTypeMatrix {
  envs: string[];
  dbTypes: string[];
  series: EnvTypeSeries[];
}

export const UNKNOWN_TYPE = "unknown";

const CELL_SEPARATOR = "\u0000";

function normalizeType(value: string | undefined): string {
  const text = (value ?? "").trim().toLowerCase();
  return text === "" ? UNKNOWN_TYPE : text;
}

function compareName(left: string, right: string): number {
  if (left === right) return 0;
  return left < right ? -1 : 1;
}

/** 实例按数据库类型汇总，数量降序、同数量按类型名升序，保证图表顺序稳定。 */
export function instanceTypeCounts(instances: InstanceItem[]): TypeCount[] {
  const counter = new Map<string, number>();
  for (const instance of instances) {
    const key = normalizeType(instance.dbType);
    counter.set(key, (counter.get(key) ?? 0) + 1);
  }
  return [...counter.entries()]
    .map(([dbType, count]) => ({ dbType, count }))
    .sort((left, right) => right.count - left.count || compareName(left.dbType, right.dbType));
}

/** 环境 × 数据库类型的堆叠柱数据；envs 保持首次出现顺序，dbTypes 固定按名称排序。 */
export function envTypeMatrix(instances: InstanceItem[]): EnvTypeMatrix {
  const envs: string[] = [];
  const dbTypes = new Set<string>();
  const cells = new Map<string, number>();

  for (const instance of instances) {
    const env = normalizeType(instance.type);
    const dbType = normalizeType(instance.dbType);
    if (!envs.includes(env)) {
      envs.push(env);
    }
    dbTypes.add(dbType);
    const key = `${env}${CELL_SEPARATOR}${dbType}`;
    cells.set(key, (cells.get(key) ?? 0) + 1);
  }

  const orderedDbTypes = [...dbTypes].sort(compareName);
  return {
    envs,
    dbTypes: orderedDbTypes,
    series: orderedDbTypes.map((dbType) => ({
      dbType,
      values: envs.map((env) => cells.get(`${env}${CELL_SEPARATOR}${dbType}`) ?? 0),
    })),
  };
}

export function percent(part: number, total: number): number {
  if (!Number.isFinite(total) || total <= 0) return 0;
  const value = Math.round((part / total) * 100);
  return Math.min(100, Math.max(0, value));
}

/** 指标值缺失时统一显示破折号，避免把「取不到」和「0」混为一谈。 */
export function formatMetric(value: number | null | undefined): string {
  return value === null || value === undefined ? "—" : String(value);
}

/**
 * 解析 Archery 首页回显的统计区间，例如 "2026-10-29 - 2026-11-05"。
 * 用它作唯一事实来源：请求参数没被服务端接受时，选择器会跟着回到实际窗口。
 */
export function parseRangeLabel(label: string): { start: string; end: string } | null {
  const match = /^(\d{4}-\d{2}-\d{2})\s*[-~]\s*(\d{4}-\d{2}-\d{2})$/.exec(label.trim());
  if (!match) return null;
  return { start: match[1], end: match[2] };
}
