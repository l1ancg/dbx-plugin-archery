/** 恢复页签时只关心这两个字段；其余状态原样保留。 */
export interface TabTarget {
  instanceName: string;
  dbName: string;
}

/**
 * 丢弃当前环境不存在的实例目标。
 *
 * 跨环境共用的状态文件（0.2.59 及以前只有一份）、实例改名或下线时，
 * 恢复出来的页签会带着别的环境的实例名，看着像「页签被缓存了」。
 * 这里只清 instanceName / dbName，保留 SQL —— 用户重新选目标就能继续跑。
 *
 * `instanceNames` 为空表示实例清单没取到，此时一律不动，避免把全部目标误清。
 */
export function pruneTabTargets<T extends TabTarget>(
  tabs: T[],
  instanceNames: string[],
): { tabs: T[]; dropped: number } {
  if (instanceNames.length === 0) return { tabs, dropped: 0 };
  const known = new Set(instanceNames);
  let dropped = 0;
  const next = tabs.map((tab) => {
    if (!tab.instanceName || known.has(tab.instanceName)) return tab;
    dropped += 1;
    return { ...tab, instanceName: "", dbName: "" };
  });
  return dropped === 0 ? { tabs, dropped } : { tabs: next, dropped };
}
