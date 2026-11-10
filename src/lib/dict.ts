import type { DictionaryTable } from "../types";

/**
 * 数据字典的一行：Archery 的表资源行是 [表名, 注释, 引擎...]，
 * 注释列常是 DBA 维护的中文说明。
 */
export interface DictTable {
  name: string;
  comment: string;
  dbName: string;
}

/** A–Z 分组；首字符不是字母的表统一落到 "#" 组。 */
export interface DictGroup {
  letter: string;
  tables: DictTable[];
}

export const OTHER_LETTER = "#";

/** 资源行的单元格可能是 null / number，统一转成裁剪后的文本。 */
function cellText(value: unknown): string {
  if (value === null || value === undefined) return "";
  return String(value).trim();
}

export function dictLetter(name: string): string {
  const first = name.trim().charAt(0).toLowerCase();
  return first >= "a" && first <= "z" ? first : OTHER_LETTER;
}

export function toDictTable(row: DictionaryTable, dbName: string): DictTable {
  return { name: cellText(row.name), comment: cellText(row.comment), dbName };
}

/** 一段命中了关键词的文本，供列表用 <mark> 高亮。 */
export interface MatchSegment {
  text: string;
  hit: boolean;
}

/**
 * 按关键词把文本切成命中 / 未命中的片段（不区分大小写，保留原文本大小写）。
 * 过滤与高亮共用同一个关键词，命中原因才看得见。
 */
export function splitMatch(text: string, keyword: string): MatchSegment[] {
  const needle = keyword.trim().toLowerCase();
  if (!text) return [];
  if (!needle) return [{ text, hit: false }];
  const haystack = text.toLowerCase();
  const segments: MatchSegment[] = [];
  let from = 0;
  let at = haystack.indexOf(needle);
  while (at >= 0) {
    if (at > from) segments.push({ text: text.slice(from, at), hit: false });
    segments.push({ text: text.slice(at, at + needle.length), hit: true });
    from = at + needle.length;
    at = haystack.indexOf(needle, from);
  }
  if (from < text.length) segments.push({ text: text.slice(from), hit: false });
  return segments;
}

export function filterDictTables(tables: DictTable[], keyword: string): DictTable[] {
  const needle = keyword.trim().toLowerCase();
  if (!needle) return tables;
  return tables.filter(
    (table) =>
      table.name.toLowerCase().includes(needle) ||
      table.comment.toLowerCase().includes(needle),
  );
}

/** 按首字母分组：字母升序、"#" 置尾，组内按表名升序。 */
export function groupDictTables(tables: DictTable[]): DictGroup[] {
  const buckets = new Map<string, DictTable[]>();
  for (const table of tables) {
    const letter = dictLetter(table.name);
    const bucket = buckets.get(letter);
    if (bucket) bucket.push(table);
    else buckets.set(letter, [table]);
  }
  const letters = [...buckets.keys()].sort((a, b) => {
    if (a === OTHER_LETTER) return 1;
    if (b === OTHER_LETTER) return -1;
    return a.localeCompare(b);
  });
  return letters.map((letter) => ({
    letter,
    tables: [...(buckets.get(letter) ?? [])].sort((a, b) =>
      a.name.toLowerCase().localeCompare(b.name.toLowerCase()),
    ),
  }));
}
