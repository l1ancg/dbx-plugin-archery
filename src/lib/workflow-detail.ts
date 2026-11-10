/**
 * SQL 上线工单详情页的纯函数：其他信息字段、SQL 列表行、可执行时间范围与回滚入口。
 * 对齐 Archery 1.14 的 sql/templates/detail.html。
 */
import type {
  PrivilegeReviewNode,
  SqlRollbackRow,
  SqlSubmitPrefill,
  SqlWorkflowRow,
  WorkflowGridRow,
  WorkflowInfoField,
} from "../types";

/** Archery 详情页「其他信息」的列名 → 本地化 key；未知列名返回空串，UI 原样展示。 */
const FIELD_LABEL_KEYS: Record<string, string> = {
  发起人: "workflow.colApplicant",
  目标实例: "workflow.colInstance",
  数据库: "workflow.colDatabase",
  发起时间: "workflow.colCreateTime",
  可执行时间范围: "workflow.colRunDate",
  结束时间: "workflow.colFinishTime",
  备份: "workflow.colBackup",
  当前状态: "workflow.colCurrentStatus",
  组: "workflow.colGroup",
  类型: "workflow.colType",
};

export function infoFieldLabelKey(label: string): string {
  return FIELD_LABEL_KEYS[label.trim()] ?? "";
}

function toText(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "";
  return String(value);
}

function toNumber(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number(value.trim());
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/** 其他信息：保留 Archery 的列顺序，空标题或空值直接丢弃。 */
export function normalizeInfoFields(value: unknown): WorkflowInfoField[] {
  if (!Array.isArray(value)) return [];
  const fields: WorkflowInfoField[] = [];
  for (const item of value) {
    if (!isRecord(item)) continue;
    const label = toText(item.label ?? item.name ?? item.key).trim();
    const text = toText(item.value).trim();
    if (!label || !text) continue;
    fields.push({ label, value: text });
  }
  return fields;
}

/** 回滚入口：工单执行结束（正常/异常）且选择了备份。 */
export function canRollbackWorkflow(status: string, isBackup: boolean): boolean {
  if (!isBackup) return false;
  return status === "workflow_finish" || status === "workflow_exception";
}

/** SQL 列表行：id 缺失时用 1 起的序号兜底，字段兼容驼峰与 Django 下划线键。 */
export function normalizeWorkflowGrid(value: unknown): WorkflowGridRow[] {
  if (!Array.isArray(value)) return [];
  return value.filter(isRecord).map((row, index) => ({
    id: toNumber(row.id) || index + 1,
    sql: toText(row.sql),
    errorLevel: toNumber(row.errorLevel ?? row.errlevel),
    errorMessage: toText(row.errorMessage ?? row.errormessage),
    affectedRows: toNumber(row.affectedRows ?? row.affected_rows),
    executeTime: toText(row.executeTime ?? row.execute_time),
    backupTime: toText(row.backupTime ?? row.backup_time),
    stage: toText(row.stage ?? row.stagestatus),
  }));
}

/** SQL 标识符里的字符：命中后不能再当作库名的开头/结尾。 */
const IDENT_CHAR = /[0-9A-Za-z_$.]/;

function skipSpaces(text: string, from: number): number {
  let index = from;
  while (index < text.length && /\s/.test(text[index])) index += 1;
  return index;
}

/**
 * 去掉回滚语句里与原工单库同名的库名前缀：goInception 会把回滚语句补成
 * 「`schedule-dev`.`bs_vehicle_info`」或「schedule-dev.bs_vehicle_info」。
 * 回滚工单本来就落在原工单的库上，前缀既冗余，换库/换实例后又会打错库。
 *
 * 只剥离完整的库名 token（前后不接标识符字符，点后可跟空白），字符串字面量
 * （'…' / "…"，含反斜杠转义）与别的库名（schedule-dev2、other 等）原样保留。
 */
export function stripDatabaseQualifier(sql: string, dbName: string): string {
  const db = dbName.trim();
  if (!db || !sql) return sql;
  const lower = db.toLowerCase();
  const matchesAt = (from: number): boolean =>
    sql.slice(from, from + db.length).toLowerCase() === lower;
  let out = "";
  let quote = "";
  for (let index = 0; index < sql.length; index += 1) {
    const char = sql[index];
    if (quote) {
      out += char;
      if (char === "\\" && index + 1 < sql.length) {
        out += sql[index + 1];
        index += 1;
      } else if (char === quote) {
        quote = "";
      }
      continue;
    }
    if (char === "'" || char === '"') {
      quote = char;
      out += char;
      continue;
    }
    // 反引号标识符：`库名` + 点 才剥，其余整段照抄。
    if (char === "`") {
      const close = sql.indexOf("`", index + 1);
      if (close > index) {
        const after = skipSpaces(sql, close + 1);
        if (
          sql.slice(index + 1, close).toLowerCase() === lower &&
          sql[after] === "." &&
          skipSpaces(sql, after + 1) < sql.length
        ) {
          index = skipSpaces(sql, after + 1) - 1;
          continue;
        }
        out += sql.slice(index, close + 1);
        index = close;
        continue;
      }
    }
    if (
      (index === 0 || !IDENT_CHAR.test(sql[index - 1])) &&
      matchesAt(index) &&
      sql[skipSpaces(sql, index + db.length)] === "."
    ) {
      const table = skipSpaces(sql, skipSpaces(sql, index + db.length) + 1);
      if (table < sql.length) {
        index = table - 1;
        continue;
      }
    }
    out += char;
  }
  return out;
}

/**
 * 回滚明细行：Archery 的 backup_sql 返回 [执行语句, 回滚语句] 二元数组
 * （goInception 引擎，按执行顺序倒序）；兼容 {sql, rollback_sql} 对象与纯回滚字符串。
 * id 缺失时用 1 起的序号兜底；回滚语句按工单库名剥掉库名前缀（执行语句保持原样）。
 */
export function normalizeRollbackRows(value: unknown, dbName = ""): SqlRollbackRow[] {
  if (!Array.isArray(value)) return [];
  const rows: SqlRollbackRow[] = [];
  for (const item of value) {
    let sql = "";
    let rollbackSql = "";
    let id = 0;
    if (Array.isArray(item)) {
      sql = toText(item[0]).trim();
      rollbackSql = stripDatabaseQualifier(toText(item[1]).trim(), dbName);
    } else if (isRecord(item)) {
      sql = toText(item.sql).trim();
      rollbackSql = stripDatabaseQualifier(
        (toText(item.rollbackSql) || toText(item.rollback_sql)).trim(),
        dbName,
      );
      id = toNumber(item.id);
    } else {
      rollbackSql = stripDatabaseQualifier(toText(item).trim(), dbName);
    }
    if (!sql && !rollbackSql) continue;
    rows.push({ id: id || rows.length + 1, sql, rollbackSql });
  }
  return rows;
}

/**
 * 弹窗预览用的单行化：goInception 会把 DDL 重排成 "alter table\n  t1\nmodify\n  column ..."，
 * 直接按原文渲染会在窄弹窗里出现锯齿缩进。这里把引号外的连续空白（含换行）压成单个空格，
 * 字符串字面量（'…' / "…" / `…`，含反斜杠转义）原样保留，避免改动显示内容。
 */
export function formatSqlPreview(sql: string): string {
  const text = sql.trim();
  if (!text) return "";
  let out = "";
  let quote = "";
  let pendingSpace = false;
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (quote) {
      out += char;
      if (char === "\\" && index + 1 < text.length) {
        out += text[index + 1];
        index += 1;
      } else if (char === quote) {
        quote = "";
      }
      continue;
    }
    if (char === "'" || char === '"' || char === "`") {
      if (pendingSpace) out += " ";
      pendingSpace = false;
      quote = char;
      out += char;
      continue;
    }
    if (/\s/.test(char)) {
      pendingSpace = out.length > 0;
      continue;
    }
    if (pendingSpace) {
      out += " ";
      pendingSpace = false;
    }
    out += char;
  }
  return out;
}

/** 回滚明细搜索：执行语句或回滚语句命中即可。 */
export function filterRollbackRows(rows: SqlRollbackRow[], keyword: string): SqlRollbackRow[] {
  const text = keyword.trim().toLowerCase();
  if (!text) return rows;
  return rows.filter((row) =>
    [row.sql, row.rollbackSql].some((value) => value.toLowerCase().includes(text)),
  );
}

/**
 * 下载回滚 SQL 的文件内容，对齐 Archery `views.rollback` 的下载格式：
 * 每段写成「/*执行语句*／\n回滚语句\n」，回滚顺序即接口返回顺序（执行顺序倒序）。
 */
export function buildRollbackFile(rows: SqlRollbackRow[]): string {
  return rows.map((row) => `/*${row.sql}*/\n${row.rollbackSql}\n`).join("");
}

/**
 * Archery「可执行时间范围」的展示值 → 提交页的起止时间（"YYYY-MM-DD HH:mm"）。
 * 展示值可能是「无限制」，也可能是 "2026-11-02 12:00:00 ~ 2026-11-02 14:00:00"；
 * 只取到分钟，缺一端或没有时间戳时整段丢弃，交给提交页按「不限」处理。
 */
export function parseRunDateRange(text: string): { start: string; end: string } {
  const [start, end] = text.match(/\d{4}-\d{2}-\d{2}[ T]\d{1,2}:\d{2}(?::\d{2})?/g) ?? [];
  if (!start || !end) return { start: "", end: "" };
  const toMinute = (value: string) =>
    value.replace("T", " ").replace(/ (\d):/, " 0$1:").slice(0, 16);
  return { start: toMinute(start), end: toMinute(end) };
}

/** 「可执行时间变更」的校验结果：error 为空表示可以提交。 */
export type RunDateChangeError = "" | "partial" | "order";

export interface RunDateChange {
  start: string;
  end: string;
  error: RunDateChangeError;
}

/**
 * 变更弹窗的提交值：两端都空表示清空为「无限制」（Archery 落库为 None），
 * 只填一端或起止倒置一律拦下；datetime-local 的 T 与秒在这里归一成分钟。
 */
export function normalizeRunDateChange(start: string, end: string): RunDateChange {
  const from = start.trim().replace("T", " ").slice(0, 16);
  const to = end.trim().replace("T", " ").slice(0, 16);
  if (!from && !to) return { start: "", end: "", error: "" };
  if (!from || !to) return { start: from, end: to, error: "partial" };
  if (from > to) return { start: from, end: to, error: "order" };
  return { start: from, end: to, error: "" };
}

/** 详情页「可执行时间范围」→ 变更弹窗的预填值；「无限制」或缺失时返回空串。 */
export function readRunDateRange(fields: WorkflowInfoField[]): { start: string; end: string } {
  return parseRunDateRange(fieldValue(fields, "可执行时间范围"));
}

/**
 * detail_content 里引擎会插入一条「use 库名」来切库，它只属于原工单的执行上下文：
 * 带到提交页会改掉新工单的默认库，因此预填时整条丢弃。
 */
const USE_STATEMENT = /^use\s+[`"']?[\w$-]+[`"']?\s*;?$/i;

/** 提交页的 SQL 内容：空行与 USE 语句丢弃，其余每条补分号后按行拼接（Archery 要求语句以分号结尾）。 */
export function buildCommitSql(rows: WorkflowGridRow[]): string {
  return rows
    .map((row) => row.sql.trim())
    .filter((sql) => sql !== "" && !USE_STATEMENT.test(sql))
    .map((sql) => (sql.endsWith(";") ? sql : `${sql};`))
    .join("\n");
}

/** 组 / 实例下拉按名称反查 id：忽略首尾空格与大小写，匹配不到返回 0。 */
export function matchOptionIdByName(
  options: Array<{ id: number; name: string }>,
  name: string,
): number {
  const target = name.trim().toLowerCase();
  if (!target) return 0;
  const matched = options.find((item) => item.name.trim().toLowerCase() === target);
  return matched?.id ?? 0;
}

/**
 * 库下拉的兜底：原工单的库可能不在当前账号的资源清单里（改名、权限收敛），
 * 这时把原值补到列表首位，避免 select 选不中而被清空。
 */
export function ensureOptionValue(options: string[], value: string): string[] {
  const target = value.trim();
  if (!target) return options;
  const exists = options.some((item) => item.trim().toLowerCase() === target.toLowerCase());
  return exists ? options : [target, ...options];
}

/** 回滚工单三类目标里第一个缺失的项，空串表示三项都取到了原工单的值。 */
export type RollbackTargetMissing = "" | "group" | "instance" | "database";

/** 回滚工单的目标：组/实例/库必须与原工单一致。 */
export interface RollbackTarget {
  groupId: number;
  instanceId: number;
  dbName: string;
  missing: RollbackTargetMissing;
}

/**
 * 「提交回滚请求」的目标必须锁死成原工单的组/实例/库：组与实例按名称反查 id，
 * 只在同名的唯一选项上命中，绝不退到列表里的其它组/实例；库直接沿用原值。
 * 任一项取不到就把 missing 交给 UI 拦截提交，避免回滚语句落到别的库上。
 */
export function resolveRollbackTarget(input: {
  row: Pick<SqlWorkflowRow, "groupName" | "instanceName" | "dbName">;
  groups: Array<{ id: number; name: string }>;
  instances: Array<{ id: number; name: string }>;
}): RollbackTarget {
  const dbName = input.row.dbName.trim();
  const groupId = matchOptionIdByName(input.groups, input.row.groupName);
  const instanceId = groupId > 0 ? matchOptionIdByName(input.instances, input.row.instanceName) : 0;
  let missing: RollbackTargetMissing = "";
  if (groupId === 0) {
    missing = "group";
  } else if (instanceId === 0) {
    missing = "instance";
  } else if (!dbName) {
    missing = "database";
  }
  return { groupId, instanceId, dbName, missing };
}

const YES_TEXT = new Set(["是", "yes", "true", "1"]);
const NO_TEXT = new Set(["否", "no", "false", "0"]);

function fieldValue(fields: WorkflowInfoField[], label: string): string {
  return fields.find((field) => field.label.trim() === label)?.value.trim() ?? "";
}

/** 详情页两种预填共用的取值来源：标题、其他信息与 SQL 列表。 */
export interface SubmitPrefillSource {
  row: SqlWorkflowRow;
  title?: string;
  infoFields: WorkflowInfoField[];
  rows: WorkflowGridRow[];
}

/**
 * 「查看提交信息」→ 提交页的预填数据：优先用详情页「其他信息」（列名是中文标签），
 * 详情解析失败时退回列表行，保证预填尽可能完整。
 */
export function buildSubmitPrefill(input: SubmitPrefillSource): SqlSubmitPrefill {
  const fields = input.infoFields;
  const backupText = fieldValue(fields, "备份").toLowerCase();
  const backup = YES_TEXT.has(backupText)
    ? true
    : NO_TEXT.has(backupText)
      ? false
      : input.row.isBackup;
  const range = parseRunDateRange(fieldValue(fields, "可执行时间范围"));
  return {
    mode: "commit",
    workflowId: input.row.id,
    title: (input.title ?? "").trim() || input.row.workflowName,
    groupName: fieldValue(fields, "组") || input.row.groupName,
    instanceName: fieldValue(fields, "目标实例") || input.row.instanceName,
    dbName: fieldValue(fields, "数据库") || input.row.dbName,
    sql: buildCommitSql(input.rows),
    backup,
    runDateStart: range.start,
    runDateEnd: range.end,
  };
}

/**
 * 「上线其他实例」→ 提交页的预填数据：除了目标实例与库，其余内容照原工单带入，
 * 实例和库留空交给用户在新目标上重新选择并检测（换库后原审核结论也不可复用）。
 */
export function buildOtherInstancePrefill(input: SubmitPrefillSource): SqlSubmitPrefill {
  return { ...buildSubmitPrefill(input), mode: "other", instanceName: "", dbName: "" };
}

/** 走完审批才能到达的终态：正常结束与执行异常。 */
const FINISHED_STATUSES = new Set(["workflow_finish", "workflow_exception"]);

/**
 * 审批流兜底：详情页的节点状态靠 HTML class 区分（text-success / text-danger / <u>），
 * 部分 Archery 版本或定制模板不渲染这些 class，已结束的工单会整条显示成「未到达」。
 * 已结束的工单必然走完全部审批节点，因此只在「一个已通过 / 当前节点都没解析到」时整条按已通过渲染；
 * 解析到任何进度就原样返回，不覆盖详情页的结论，也不给仍在流转的工单编造进度。
 */
export function resolveReviewNodes(
  nodes: PrivilegeReviewNode[],
  status: string,
): PrivilegeReviewNode[] {
  if (nodes.length === 0 || !FINISHED_STATUSES.has(status.trim())) return nodes;
  if (nodes.some((node) => node.state !== "pending")) return nodes;
  return nodes.map((node) => ({ ...node, state: "passed" as const }));
}
