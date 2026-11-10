import type { Cell } from "../types";

export interface NamedItem {
  name: string;
  values?: Cell[];
}

export function filterByName<T extends NamedItem>(items: T[], keyword: string): T[] {
  const needle = keyword.trim().toLowerCase();
  if (!needle) return items;
  return items.filter((item) => item.name.toLowerCase().includes(needle));
}

/** 资源树里「实例 + 库」的复合键，避免同名库在不同实例下互相覆盖。 */
export function treeKey(instanceName: string, dbName: string): string {
  return `${instanceName}\u0000${dbName}`;
}

/**
 * 单节点是否命中：名称或备注列包含关键字（大小写不敏感）。
 * 空关键字视为命中，保证不搜索时整棵树保持完整。
 */
export function matchesNode(item: NamedItem, keyword: string): boolean {
  const needle = keyword.trim().toLowerCase();
  if (!needle) return true;
  if (item.name.toLowerCase().includes(needle)) return true;
  return resourceDetail(item.values).toLowerCase().includes(needle);
}

function dbHit(
  instance: NamedItem,
  db: NamedItem,
  tables: NamedItem[],
  keyword: string,
): boolean {
  if (matchesNode(instance, keyword)) return true;
  if (matchesNode(db, keyword)) return true;
  return tables.some((table) => matchesNode(table, keyword));
}

/**
 * 实例是否保留：自身命中，或任一已加载的库/表命中。
 * 只按实例名过滤会让「搜库/搜表」永远搜不到，这是搜索失效的根因。
 */
export function visibleInstances<T extends NamedItem>(
  instances: T[],
  dbsByName: Record<string, NamedItem[]>,
  tablesByKey: Record<string, NamedItem[]>,
  keyword: string,
): T[] {
  if (!keyword.trim()) return instances;
  return instances.filter(
    (instance) =>
      matchesNode(instance, keyword) ||
      (dbsByName[instance.name] ?? []).some((db) =>
        dbHit(
          instance,
          db,
          tablesByKey[treeKey(instance.name, db.name)] ?? [],
          keyword,
        ),
      ),
  );
}

/**
 * 库是否保留：实例自身命中时显示全部库（父级命中展示完整子树）；
 * 否则只保留库自身命中、或库下已有表命中的库。
 */
export function visibleDbs<T extends NamedItem>(
  instance: NamedItem,
  dbs: T[],
  tablesByKey: Record<string, NamedItem[]>,
  keyword: string,
): T[] {
  if (!keyword.trim()) return dbs;
  return dbs.filter((db) =>
    dbHit(instance, db, tablesByKey[treeKey(instance.name, db.name)] ?? [], keyword),
  );
}

/**
 * 表是否保留：实例或库自身命中时显示全部表，否则只保留表自身命中的表。
 */
export function visibleTables<T extends NamedItem>(
  instance: NamedItem,
  db: NamedItem,
  tables: T[],
  keyword: string,
): T[] {
  if (!keyword.trim()) return tables;
  if (matchesNode(instance, keyword) || matchesNode(db, keyword)) return tables;
  return tables.filter((table) => matchesNode(table, keyword));
}

export function mergeUnique<T extends NamedItem>(current: T[], incoming: T[]): T[] {
  const seen = new Set(current.map((item) => item.name));
  const merged = [...current];
  for (const item of incoming) {
    if (!seen.has(item.name)) {
      seen.add(item.name);
      merged.push(item);
    }
  }
  return merged;
}

/**
 * Archery 的资源行可能是 [表名, 备注, 引擎...]，第二列常是可读备注。
 */
export function resourceDetail(values: Cell[] | undefined): string {
  if (!values || values.length < 2) return "";
  const detail = values[1];
  if (detail === null || detail === undefined) return "";
  return String(detail);
}
