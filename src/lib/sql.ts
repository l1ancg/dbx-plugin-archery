import { format, type SqlLanguage } from "sql-formatter";

/**
 * SQL 文本工具：语句切分、光标定位、方言映射、格式化与执行计划前缀。
 * 纯函数，便于单测；编辑器与查询流程共用。
 */
export type SqlDialect =
  | "mysql"
  | "mariadb"
  | "tidb"
  | "postgresql"
  | "sqlserver"
  | "plsql"
  | "hive"
  | "spark"
  | "sqlite"
  | "bigquery"
  | "redshift"
  | "trino"
  | "db2";

export interface SqlStatement {
  /** 语句正文，去掉首尾空白，保留语句内的注释。 */
  text: string;
  /** 语句在原文中的起始下标（首个非空白字符，含注释起始）。 */
  start: number;
  /** 语句在原文中的结束下标（不含结尾分号）。 */
  end: number;
}

const DIALECTS: Record<string, SqlDialect> = {
  mysql: "mysql",
  mariadb: "mysql",
  tidb: "mysql",
  doris: "mysql",
  starrocks: "mysql",
  postgres: "postgresql",
  postgresql: "postgresql",
  pg: "postgresql",
  greenplum: "postgresql",
  sqlserver: "sqlserver",
  mssql: "sqlserver",
  oracle: "plsql",
  plsql: "plsql",
  hive: "hive",
  spark: "spark",
  sqlite: "sqlite",
  bigquery: "bigquery",
  redshift: "redshift",
  trino: "trino",
  presto: "trino",
  db2: "db2",
};

/** sql-formatter 用 `transactsql` 表示 SQL Server。 */
function formatterLanguage(dialect: SqlDialect): SqlLanguage {
  return dialect === "sqlserver" ? "transactsql" : dialect;
}

function isSpace(char: string | undefined): boolean {
  return char === undefined || /\s/.test(char);
}

/** `--` 只有后面跟空白或行尾才算注释，避免把 `1--2` 当成注释。 */
function isLineCommentStart(sql: string, index: number): boolean {
  return sql[index] === "-" && sql[index + 1] === "-" && isSpace(sql[index + 2]);
}

function skipQuoted(sql: string, index: number, quote: string): number {
  let cursor = index + 1;
  while (cursor < sql.length) {
    const char = sql[cursor];
    if (char === "\\" && quote !== "`") {
      cursor += 2;
      continue;
    }
    if (char === quote) {
      if (sql[cursor + 1] === quote) {
        cursor += 2;
        continue;
      }
      return cursor + 1;
    }
    cursor += 1;
  }
  return sql.length;
}

function skipLineComment(sql: string, index: number): number {
  let cursor = index;
  while (cursor < sql.length && sql[cursor] !== "\n") {
    cursor += 1;
  }
  return cursor;
}

function skipBlockComment(sql: string, index: number): number {
  let cursor = index + 2;
  while (cursor < sql.length && !(sql[cursor] === "*" && sql[cursor + 1] === "/")) {
    cursor += 1;
  }
  return Math.min(cursor + 2, sql.length);
}

/**
 * 按分号切分语句：分号出现在字符串、标识符或注释里时不切分。
 * 纯注释段与空段不会产生语句。
 */
export function splitStatements(sql: string): SqlStatement[] {
  const statements: SqlStatement[] = [];
  let start = -1;
  let hasCode = false;

  const flush = (end: number): void => {
    if (start >= 0 && hasCode) {
      const text = sql.slice(start, end).trim();
      if (text) statements.push({ text, start, end });
    }
    start = -1;
    hasCode = false;
  };

  let cursor = 0;
  while (cursor < sql.length) {
    const char = sql[cursor];
    if (char === "'" || char === '"' || char === "`") {
      if (start < 0) start = cursor;
      hasCode = true;
      cursor = skipQuoted(sql, cursor, char);
      continue;
    }
    if (isLineCommentStart(sql, cursor) || char === "#") {
      if (start < 0) start = cursor;
      cursor = skipLineComment(sql, cursor);
      continue;
    }
    if (char === "/" && sql[cursor + 1] === "*") {
      if (start < 0) start = cursor;
      cursor = skipBlockComment(sql, cursor);
      continue;
    }
    if (char === ";") {
      flush(cursor);
      cursor += 1;
      continue;
    }
    if (!/\s/.test(char)) {
      if (start < 0) start = cursor;
      hasCode = true;
    }
    cursor += 1;
  }
  flush(sql.length);
  return statements;
}

/** 取光标所在语句：落在语句之间的空白或分号上时回退到前一条，首条之前取第一条。 */
function statementIn(statements: SqlStatement[], sql: string, cursor: number): SqlStatement {
  const position = Math.min(Math.max(cursor, 0), sql.length);
  let chosen = statements[0];
  for (const statement of statements) {
    if (position >= statement.start) chosen = statement;
  }
  return chosen;
}

export function statementAt(sql: string, cursor: number): SqlStatement | null {
  const statements = splitStatements(sql);
  if (statements.length === 0) return null;
  return statementIn(statements, sql, cursor);
}

/** 一次执行要跑的一条语句：正文已去掉结尾分号，「第 index / total 条」用于提示。 */
export interface RunStep {
  text: string;
  index: number;
  total: number;
}

/**
 * 执行计划：有选区就把选区按分号切成多条（逐条顺序执行），
 * 否则取光标所在语句；连光标都拿不到时兜底整段。
 * 无论哪条路径，返回的每条语句都已去掉结尾分号，可以直接发给 Archery。
 */
export function runSteps(sql: string, selection: string, cursor?: number): RunStep[] {
  if (selection.trim()) {
    const picked = splitStatements(selection);
    return picked.map((statement, at) => ({
      text: statement.text,
      index: at + 1,
      total: picked.length,
    }));
  }

  const statements = splitStatements(sql);
  if (statements.length === 0) return [];

  if (cursor === undefined) {
    return statements.map((statement, at) => ({
      text: statement.text,
      index: at + 1,
      total: statements.length,
    }));
  }

  const chosen = statementIn(statements, sql, cursor);
  return [{ text: chosen.text, index: statements.indexOf(chosen) + 1, total: statements.length }];
}

export function dialectFor(dbType: string): SqlDialect | null {
  return DIALECTS[dbType.trim().toLowerCase()] ?? null;
}

/** 格式化 SQL；方言不支持时抛错，由调用方降级提示。 */
export function formatSql(sql: string, dbType: string): string {
  const dialect = dialectFor(dbType);
  if (!dialect) throw new Error(`unsupported dialect: ${dbType}`);
  return format(sql, {
    language: formatterLanguage(dialect),
    keywordCase: "upper",
    tabWidth: 2,
  }).trim();
}

/** 生成执行计划语句；方言不支持或 SQL 为空时返回 null。 */
export function explainSql(sql: string, dbType: string): string | null {
  const text = sql.trim();
  if (!text || !dialectFor(dbType)) return null;
  if (/^explain\b/i.test(text)) return text;
  return `EXPLAIN ${text}`;
}

/** 单行摘要，用于「常用查询」下拉等窄区域。 */
export function sqlSummary(sql: string, max = 80): string {
  const text = sql.replace(/\s+/g, " ").trim();
  if (text.length <= max) return text;
  return `${text.slice(0, Math.max(0, max - 1))}…`;
}
