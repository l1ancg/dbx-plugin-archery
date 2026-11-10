import { DEFAULT_RESULT_PANEL_ID, initialPanels } from "./lib/panels";

export type Cell = string | number | boolean | null;

export interface InstanceItem {
  id: number;
  name: string;
  type: string;
  dbType: string;
}

export interface ResourceItem {
  name: string;
  values: Cell[];
}

/** 数据字典的一行：表名与注释独立成字段，不再借用资源行的位置数组。 */
export interface DictionaryTable {
  name: string;
  comment: string;
}

/** Archery 元数据里的一个表格块（列名 + 位置行）。 */
export interface TableGrid {
  columns: string[];
  rows: Cell[][];
}

/** 表元数据弹窗的数据：表信息 / 列信息 / 索引信息 / 建表语句。 */
export interface TableMetadata {
  meta: TableGrid;
  desc: TableGrid;
  index: TableGrid;
  createSql: string;
}

export interface QueryResult {
  columns: string[];
  columnTypes?: string[];
  rows: Cell[][];
  rowCount: number;
  affectedRows: number;
  elapsedMs: number;
  masked: boolean;
  maskRuleHit?: boolean;
  warning?: string;
  secondsBehindMaster?: string;
}

export interface DescribeResult {
  columns: string[];
  types?: string[];
  rows: Cell[][];
  /** SHOW CREATE TABLE 返回的表名；列清单型响应为空。 */
  table?: string;
  /** 建表语句；为空时 UI 回退到列清单表格。 */
  ddl?: string;
}

export interface QueryLogRow {
  id: number;
  instanceName: string;
  dbName: string;
  sql: string;
  effectRow: number;
  costTime: string;
  userDisplay: string;
  favorite: boolean;
  alias: string;
  createTime: string;
}

export interface QueryLogPage {
  total: number;
  rows: QueryLogRow[];
}

/** 授权时间预设，提交前换算成 Archery 需要的到期日。 */
export type ValidDatePreset = "day" | "week" | "month" | "year";

/** 查询权限申请工单（Archery /query/applylist/ 的 values 行）。 */
export interface PrivilegeApplyRow {
  applyId: number;
  title: string;
  instanceName: string;
  dbList: string;
  privType: number;
  tableList: string;
  limitNum: number;
  validDate: string;
  userDisplay: string;
  status: number;
  createTime: string;
  groupName: string;
}

export interface PrivilegeApplyPage {
  total: number;
  rows: PrivilegeApplyRow[];
}

/** 一条查询权限记录（Archery /query/userprivileges/ 的 values 行）。 */
export interface UserPrivilegeRow {
  privilegeId: number;
  userDisplay: string;
  instanceName: string;
  dbName: string;
  privType: number;
  tableName: string;
  limitNum: number;
  validDate: string;
}

export interface UserPrivilegePage {
  total: number;
  rows: UserPrivilegeRow[];
}

/** 资源组内可申请查询权限的实例。 */
export interface GroupInstance {
  id: number;
  name: string;
  type: string;
  dbType: string;
}

/** 申请表单的本地状态；提交时再换算成 Archery 的表单字段。 */
export interface PrivilegeApplyForm {
  title: string;
  groupName: string;
  instanceName: string;
  privType: number;
  dbName: string;
  dbList: string[];
  tableList: string[];
  /** 空串表示尚未选择授权时间。 */
  validDatePreset: ValidDatePreset | "";
  limitNum: number;
}

/** 审批流节点：auto=无需审批，passed=已通过，current=当前节点，pending=未到达。 */
export interface PrivilegeReviewNode {
  name: string;
  state: "auto" | "passed" | "current" | "pending";
  reviewers: string[];
}

/** 工单详情页解析结果；解析不到的区块为空，UI 用列表行兜底。 */
export interface PrivilegeDetail {
  applyId: number;
  title: string;
  canReview: boolean;
  reviewNodes: PrivilegeReviewNode[];
  /** 详情页「数据库清单 / 表清单」面板内容。 */
  items: string[];
  itemKind: "database" | "table" | "";
  lastOperation: string;
}

export interface WorkflowLogRow {
  operationType: string;
  operator: string;
  time: string;
  info: string;
}

export interface WorkflowLogPage {
  total: number;
  rows: WorkflowLogRow[];
}

/** SQL 上线工单（/sqlworkflow_list/ 的 values 行）。 */
export interface SqlWorkflowRow {
  id: number;
  workflowName: string;
  engineerDisplay: string;
  status: string;
  isBackup: boolean;
  createTime: string;
  instanceName: string;
  dbName: string;
  groupName: string;
  /** 0 其他 / 1 DDL / 2 DML / 3 离线导出。 */
  syntaxType: number;
  exportFormat: string;
}

export interface SqlWorkflowPage {
  total: number;
  rows: SqlWorkflowRow[];
}

/**
 * 待办列表的一行（Archery /workflow/list/ 的 values 行）。
 * 只含审批记录本身的字段，不含业务工单号，审核时需要再解析一次详情去向。
 */
export interface WorkflowTodoRow {
  auditId: number;
  /** 1 查询权限 / 2 SQL 上线 / 3 数据归档。 */
  workflowType: number;
  title: string;
  applicant: string;
  createTime: string;
  /** 0 待审核 / 1 审核通过 / 2 审核不通过。 */
  status: number;
  groupName: string;
  currentAudit: string;
  auditGroups: string;
}

export interface WorkflowTodoPage {
  total: number;
  rows: WorkflowTodoRow[];
}

/** 待办详情定位：Archery 的 /workflow/<audit_id>/ 会 302 到对应工单详情页。 */
export interface WorkflowTodoTarget {
  auditId: number;
  workflowId: number;
  workflowType: number;
}

/** 筛选下拉项：id 对应 instance_id / group_id。 */
export interface SqlWorkflowOption {
  id: number;
  name: string;
}

/** 工单内容里的一条 SQL 审核/执行结果。 */
export interface SqlReviewRow {
  sequence: string;
  stage: string;
  status: string;
  /** 0 通过 / 1 警告 / 2 错误。 */
  errorLevel: number;
  errorMessage: string;
  sql: string;
  affectedRows: number;
  backupDbName: string;
  executeTime: string;
}

/** 详情页「其他信息」的一列：列名与顺序由 Archery 决定。 */
export interface WorkflowInfoField {
  label: string;
  value: string;
}

/** 一条回滚明细：执行语句与对应的回滚语句（goInception 备份，按执行顺序倒序）。 */
export interface SqlRollbackRow {
  id: number;
  sql: string;
  rollbackSql: string;
}

/** 详情页 SQL 列表的一行（含 Archery 的 id 与备份耗时）。 */
export interface WorkflowGridRow {
  id: number;
  sql: string;
  errorLevel: number;
  errorMessage: string;
  affectedRows: number;
  executeTime: string;
  backupTime: string;
  stage: string;
}

export interface SqlWorkflowDetail {
  workflowId: number;
  title: string;
  status: string;
  reviewNodes: PrivilegeReviewNode[];
  infoFields: WorkflowInfoField[];
  lastOperation: string;
  reviewRows: SqlReviewRow[];
}

/** 提交前的 SQL 检测汇总。 */
export interface SqlCheckSummary {
  errorCount: number;
  warningCount: number;
  rows: SqlReviewRow[];
}

/**
 * 提交页的带入来源：commit=原样重新提交（实例与库一起带入）；
 * other=上线其他实例（实例与库留空，由用户重新选择）。
 */
export type SqlSubmitMode = "commit" | "other";

/**
 * 从工单详情带入提交页的初始值：组 / 实例 / 库按名称匹配下拉选项，
 * 匹配不到的项保留原文，由提交页兜底展示给用户确认。
 */
export interface SqlSubmitPrefill {
  mode: SqlSubmitMode;
  /** 来源工单号，用于提交页提示条。 */
  workflowId: number;
  title: string;
  groupName: string;
  instanceName: string;
  dbName: string;
  sql: string;
  backup: boolean;
  runDateStart: string;
  runDateEnd: string;
}

/** 提交 SQL 上线工单的本地表单状态。 */
export interface SqlSubmitForm {
  title: string;
  groupId: number;
  instanceId: number;
  dbName: string;
  sql: string;
  backup: boolean;
  /** 执行时间窗，datetime-local 或 YYYY-MM-DD HH:mm，空串表示不限。 */
  runDateStart: string;
  runDateEnd: string;
}

export interface Overview {
  version: string;
  username: string;
  readOnly: boolean;
  authMode: "session" | "jwt";
  instanceCount: number;
  baseURL: string;
}

export interface ConnectResult {
  success: boolean;
  connectionId?: string;
  pendingTwofa: boolean;
  twofaType?: "totp" | "sms";
  username?: string;
  message: string;
  version?: string;
  readOnly?: boolean;
}

/** 两步验证：两个通道是否已在 Archery 绑定。 */
export interface TwoFactorState {
  totp: boolean;
  sms: boolean;
}

/** 开启 TOTP 返回密钥与二维码；短信只回手机号。 */
export interface TwoFactorEnrollment {
  authType: "totp" | "sms";
  secretKey?: string;
  qrCode?: string;
  phone?: string;
}

/** Archery 首页四个指标卡；接口拿不到时为 null，UI 显示破折号。 */
export interface DashboardCounts {
  sqlWorkflow: number;
  sqlQuery: number;
  activeUsers: number;
  instances: number;
}

export interface Announcement {
  level: "info" | "warning" | "danger" | "success" | string;
  title: string;
  content: string;
}

/** 公告接口返回：首页取不到时 degraded=true 且 announcement 为 null。 */
export interface AnnouncementPayload {
  announcement: Announcement | null;
  degraded: boolean;
  reason?: string;
}

/** 首页「数据库实例类型统计」的数据：全库实例，与当前账号可见实例清单无关。 */
export interface DashboardInstanceStats {
  dbTypes: string[];
  envs: string[];
  series: Array<{ env: string; values: number[] }>;
  types: Array<{ dbType: string; count: number }>;
}

/** 首页两张按日趋势图：检索行数/次数与上线数量，Dates 与 Values 一一对应。 */
export interface DashboardTrends {
  dates: string[];
  query: Array<{ name: string; values: number[] }>;
  release: number[];
  rangeLabel: string;
}

export interface DashboardPayload {
  degraded: boolean;
  reason?: string;
  counts?: DashboardCounts;
  instanceStats?: DashboardInstanceStats | null;
  trends?: DashboardTrends | null;
  announcement?: Announcement | null;
  /** Archery 导航栏品牌（Archery + custom_title_suffix），取不到时为空。 */
  brand?: string;
  brandSuffix?: string;
}

export interface QueryTab {
  id: string;
  title: string;
  instanceName: string;
  dbName: string;
  sql: string;
  limitNum: number;
  running: boolean;
  /** 结果面板的内层 tab：系统 tab 在前，结果 / 表结构按打开顺序排在后面。 */
  panels: PanelTab[];
  /** 当前激活的内层 tab id。 */
  activePanel: string;
  /** 「消息」tab 的内容：最近一次执行的错误 / 信息摘要。 */
  error: string;
  errorKind: string;
  info: string;
}

export type PanelKey = "result" | "message" | "history" | "describe";

/**
 * 结果面板里的内层 tab：系统 tab（消息 / 查询历史）固定在前且不可关闭，
 * 结果与表结构可多开。标题在渲染时按 kind + 序号 / 表名生成，不落进状态。
 */
export interface ResultPanelTab {
  id: string;
  kind: "result";
  /** 「结果 N」里的 N；关闭后不回收，避免重名。 */
  index: number;
  sql: string;
  limitNum: number;
  running: boolean;
  result: QueryResult | null;
  error: string;
  errorKind: string;
  info: string;
}

export interface DescribePanelTab {
  id: string;
  kind: "describe";
  instanceName: string;
  dbName: string;
  tableName: string;
  /** 工具条上的完整路径：实例 / 库 / 表。 */
  table: string;
  loading: boolean;
  error: string;
  data: DescribeResult | null;
  elapsed: number;
  fetchedAt: string;
}

export interface SystemPanelTab {
  id: string;
  kind: "message" | "history";
}

export type PanelTab = ResultPanelTab | DescribePanelTab | SystemPanelTab;

/** 会话续期探测结果（sidecar `archery/sessionKeepAlive`）。 */
export interface SessionKeepAlive {
  /** false 表示连接配置关闭了自动续期（keep_alive_secs=0）。 */
  enabled: boolean;
  /** 探测后本地是否持有可用会话；自动重登成功也算有效。 */
  alive: boolean;
  /** 距上次心跳不足间隔，本次直接复用了上次结果。 */
  skipped: boolean;
  /** 探测期间会话失效，已用连接里保存的密码自动重登。 */
  relogged: boolean;
  /** 本次会话来自落盘快照恢复（sidecar 重启后的首次请求）。 */
  restored: boolean;
  username: string;
  /** sidecar 侧真正的心跳间隔，UI 只做展示与对齐。 */
  intervalSec: number;
  checkedAt: string;
}

export interface UiState {
  tabs: Array<Pick<QueryTab, "title" | "instanceName" | "dbName" | "sql" | "limitNum">>;
  activeIndex: number;
  sidebarWidth?: number;
  editorHeight?: number;
  /** 上次导出 Excel 的所在目录，用作「另存为」的初始位置。 */
  exportDir?: string;
  /** 导航位置（routeKey 形式，如 "dashboard" / "audit:online"）。 */
  route?: string;
  auditOpen?: boolean;
}

export function createTab(patch: Partial<QueryTab> = {}): QueryTab {
  const index = Math.floor(Math.random() * 1e6);
  return {
    id: `tab-${index}`,
    title: "",
    instanceName: "",
    dbName: "",
    sql: "",
    limitNum: 100,
    running: false,
    panels: initialPanels(),
    activePanel: DEFAULT_RESULT_PANEL_ID,
    error: "",
    errorKind: "",
    info: "",
    ...patch,
  };
}
