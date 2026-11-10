import type {
  AnnouncementPayload,
  ConnectResult,
  DashboardPayload,
  DashboardTrends,
  DescribeResult,
  DictionaryTable,
  GroupInstance,
  InstanceItem,
  Overview,
  PrivilegeApplyPage,
  PrivilegeDetail,
  QueryLogPage,
  QueryResult,
  ResourceItem,
  SqlCheckSummary,
  SessionKeepAlive,
  SqlRollbackRow,
  SqlSubmitForm,
  SqlWorkflowDetail,
  SqlWorkflowOption,
  SqlWorkflowPage,
  TableMetadata,
  TwoFactorEnrollment,
  TwoFactorState,
  UiState,
  UserPrivilegePage,
  WorkflowLogPage,
  WorkflowTodoPage,
  WorkflowTodoTarget,
} from "./types";
import { writeClipboard } from "./lib/clipboard";

const CONNECT_TIMEOUT = 30_000;
const QUERY_TIMEOUT = 180_000;

let connectionId = "";

export function setConnectionId(id: string | undefined): void {
  connectionId = id ?? "";
}

export function getConnectionId(): string {
  return connectionId;
}

export class PluginError extends Error {
  constructor(
    public readonly kind: string,
    message: string,
  ) {
    super(message);
    this.name = "PluginError";
  }
}

/**
 * 宿主把 sidecar 的 PluginError 序列化成 "kind: message"，
 * 这里拆回分类，UI 才能区分「要重登」和「要提示」。
 */
export function normalizeError(error: unknown): PluginError {
  const message =
    typeof error === "string"
      ? error
      : error instanceof Error
        ? error.message
        : JSON.stringify(error ?? "unknown error");
  const match = /^([a-z_]+):\s*([\s\S]*)$/.exec(message.trim());
  if (match) {
    return new PluginError(match[1], match[2]);
  }
  return new PluginError("internal", message);
}

/**
 * 会话中途过期且账号开了 2FA 时，sidecar 会返回 twofa_required。
 * 这里回调 UI 弹验证码框；验证成功后自动重试原请求，页面无需退出重进。
 */
type TwofaHandler = () => Promise<boolean>;

let twofaHandler: TwofaHandler | null = null;
let twofaPending: Promise<boolean> | null = null;

export function setTwofaHandler(handler: TwofaHandler | null): void {
  twofaHandler = handler;
}

async function waitForTwofa(): Promise<boolean> {
  if (!twofaHandler) return false;
  if (!twofaPending) {
    twofaPending = twofaHandler()
      .catch(() => false)
      .finally(() => {
        twofaPending = null;
      });
  }
  return twofaPending;
}

async function invoke<T>(
  method: string,
  params: Record<string, unknown> = {},
  timeoutMs = CONNECT_TIMEOUT,
  allowTwofaRetry = true,
): Promise<T> {
  const plugin = window.dbxPlugin;
  if (!plugin) {
    throw new PluginError("host_unavailable", "DBX host bridge is not available");
  }
  await plugin.ready;
  try {
    return await plugin.invoke<T>(
      method,
      { connectionId, ...params },
      { timeoutMs },
    );
  } catch (error) {
    const normalized = normalizeError(error);
    if (allowTwofaRetry && normalized.kind === "twofa_required" && method !== "connection/connect") {
      const recovered = await waitForTwofa();
      if (recovered) {
        return invoke<T>(method, params, timeoutMs, false);
      }
    }
    throw normalized;
  }
}

export const archeryApi = {
  /** 2FA 验证方式与手机号来自连接配置，UI 只负责提交验证码。 */
  connect(code?: string): Promise<ConnectResult> {
    const params: Record<string, unknown> = {};
    if (code) params.otp = code;
    return invoke<ConnectResult>("connection/connect", params);
  },

  async overview(): Promise<Overview> {
    return invoke<Overview>("archery/overview");
  },

  /**
   * 会话续期：sidecar 发一次无副作用的登录态 GET，让 Archery 按最后活动时间顺延会话。
   * 会话已失效时会用连接里保存的密码自动重登；2FA 账号返回 twofa_required，
   * invoke 层会弹验证码框并在验证通过后自动重试。
   */
  async sessionKeepAlive(): Promise<SessionKeepAlive> {
    const result = await invoke<{ session: SessionKeepAlive }>(
      "archery/sessionKeepAlive",
      {},
      60_000,
    );
    return result.session;
  },

  /** 当前账号两步验证的绑定状态（TOTP / 短信）。 */
  twoFactorState(): Promise<TwoFactorState> {
    return invoke<TwoFactorState>("archery/user/twoFactorState");
  },

  /**
   * 开始绑定：TOTP 返回密钥与二维码，短信给指定手机号发验证码。
   * 服务端会先用连接里保存的密码做一次账号校验。
   */
  twoFactorStart(params: {
    authType: "totp" | "sms";
    phone?: string;
  }): Promise<TwoFactorEnrollment> {
    return invoke<TwoFactorEnrollment>("archery/user/twoFactorStart", params, 60_000);
  },

  /** 校验验证码并保存绑定。 */
  twoFactorConfirm(params: {
    authType: "totp" | "sms";
    otp: string;
    secretKey?: string;
    phone?: string;
  }): Promise<void> {
    return invoke("archery/user/twoFactorConfirm", params, 60_000);
  },

  /** 关闭某个通道；Archery 会删除对应绑定记录。 */
  twoFactorDisable(authType: "totp" | "sms"): Promise<void> {
    return invoke("archery/user/twoFactorDisable", { authType }, 60_000);
  },

  /** 通过 Django admin 改密页修改密码；成功后连接里保存的旧密码会失效。 */
  changePassword(params: { oldPassword: string; newPassword: string }): Promise<void> {
    return invoke("archery/user/changePassword", params, 60_000);
  },

  async instances(): Promise<InstanceItem[]> {
    const result = await invoke<{ instances: InstanceItem[] }>("archery/instances");
    return result.instances ?? [];
  },

  async resources(params: {
    instanceName: string;
    instanceId?: number;
    resourceType: "database" | "schema" | "table" | "column";
    dbName?: string;
    schemaName?: string;
    tbName?: string;
  }): Promise<ResourceItem[]> {
    const result = await invoke<{ resources: ResourceItem[] }>(
      "archery/resources",
      params,
      60_000,
    );
    return result.resources ?? [];
  },

  /**
   * 数据字典：表名 + 中文注释。
   * 注释来自 Archery `/data_dictionary/table_list/`（instance_resource 只有表名）；
   * 账号没有该菜单权限时 commentAvailable=false，表名照常返回。
   */
  dictTables(params: {
    instanceName: string;
    instanceId?: number;
    dbName: string;
    dbType?: string;
  }): Promise<{ tables: DictionaryTable[]; commentAvailable: boolean }> {
    return invoke<{ tables: DictionaryTable[]; commentAvailable: boolean }>(
      "archery/dictTables",
      params,
      60_000,
    );
  },

  /** 表元数据：表信息 / 列信息 / 索引信息 / 建表语句。 */
  async tableMeta(params: {
    instanceName: string;
    instanceId?: number;
    dbName: string;
    tbName: string;
    dbType?: string;
  }): Promise<TableMetadata> {
    const result = await invoke<{ meta: TableMetadata }>("archery/tableMeta", params, 60_000);
    return result.meta;
  },

  async describe(params: {
    instanceName: string;
    instanceId?: number;
    dbName: string;
    tbName: string;
    schemaName?: string;
  }): Promise<DescribeResult> {
    const result = await invoke<{ describe: DescribeResult }>(
      "archery/describe",
      params,
      60_000,
    );
    return result.describe;
  },

  async query(params: {
    instanceName: string;
    dbName: string;
    sqlContent: string;
    limitNum: number;
    schemaName?: string;
    tbName?: string;
  }): Promise<QueryResult> {
    const result = await invoke<{ result: QueryResult }>(
      "archery/query",
      params,
      QUERY_TIMEOUT,
    );
    return result.result;
  },

  /**
   * 系统公告。所有页面顶部的公告条统一走这里，sidecar 只抓 /dashboard/ 的 alert 块；
   * 取不到时 announcement 为 null，UI 不显示公告条。
   */
  announcement(): Promise<AnnouncementPayload> {
    return invoke<AnnouncementPayload>("archery/announcement", {}, 60_000);
  },

  /**
   * 首页指标卡。sidecar 从 /dashboard/ 的 HTML 取值，
   * 无权限或解析失败时返回 degraded，不带 counts。
   */
  dashboard(): Promise<DashboardPayload> {
    return invoke<DashboardPayload>("archery/dashboard", {}, 60_000);
  },

  /**
   * 按时间范围重取首页两张按日统计图。
   * 页面 /dashboard/ 不认日期参数，sidecar 走的是 /dashboard/api/。
   */
  dashboardTrends(range: {
    start: string;
    end: string;
  }): Promise<{ trends: DashboardTrends | null }> {
    return invoke<{ trends: DashboardTrends | null }>(
      "archery/dashboardTrends",
      { startDate: range.start, endDate: range.end },
      60_000,
    );
  },

  async queryLogs(params: {
    limit?: number;
    offset?: number;
    search?: string;
    star?: boolean;
    queryLogId?: number;
    instanceName?: string;
    dbName?: string;
  }): Promise<QueryLogPage> {
    const result = await invoke<{ page: QueryLogPage }>(
      "archery/queryLogs",
      params,
      60_000,
    );
    return result.page;
  },

  favorite(params: {
    queryLogId: number;
    star: boolean;
    alias?: string;
  }): Promise<void> {
    return invoke("archery/favorite", params, 30_000);
  },

  /** 权限管理：申请列表（服务端分页 + 搜索）。 */
  async privileges(params: {
    limit: number;
    offset: number;
    search?: string;
  }): Promise<PrivilegeApplyPage> {
    const result = await invoke<{ page: PrivilegeApplyPage }>(
      "archery/privileges",
      params,
      60_000,
    );
    return result.page;
  },

  /**
   * 我的权限：当前账号有效的查询权限清单。
   * 数据来自 Archery /query/userprivileges/，普通账号只会拿到自己的记录。
   */
  async myPrivileges(params: {
    limit: number;
    offset: number;
    search?: string;
  }): Promise<UserPrivilegePage> {
    const result = await invoke<{ page: UserPrivilegePage }>(
      "archery/myPrivileges",
      params,
      60_000,
    );
    return result.page;
  },

  /** 申请弹窗的「组」下拉：当前用户所属资源组。 */
  async privilegeOptions(): Promise<string[]> {
    const result = await invoke<{ groups: string[] }>("archery/privilegeOptions", {}, 60_000);
    return result.groups ?? [];
  },

  /** 选定资源组后加载可申请的实例。 */
  async privilegeInstances(params: { groupName: string }): Promise<GroupInstance[]> {
    const result = await invoke<{ instances: GroupInstance[] }>(
      "archery/privilegeInstances",
      params,
      60_000,
    );
    return result.instances ?? [];
  },

  /** 资源组在查询权限工单下的审批流（申请弹窗选中组后的提示条）。 */
  async privilegeAuditors(params: { groupName: string }): Promise<{
    auditors: string;
    auditorsDisplay: string;
  }> {
    const result = await invoke<{ auditors: string; auditorsDisplay: string }>(
      "archery/privilegeAuditors",
      params,
      60_000,
    );
    return {
      auditors: result.auditors ?? "",
      auditorsDisplay: result.auditorsDisplay ?? "",
    };
  },

  privilegeApply(params: {
    title: string;
    groupName: string;
    instanceName: string;
    privType: number;
    dbName?: string;
    dbList: string[];
    tableList: string[];
    validDate: string;
    limitNum: number;
  }): Promise<void> {
    return invoke("archery/privilegeApply", params, 60_000);
  },

  /** 探测当前账号能否审核该工单（详情页是否渲染审核表单）。 */
  async privilegeDetail(params: { applyId: number }): Promise<PrivilegeDetail> {
    const result = await invoke<{ detail: PrivilegeDetail }>(
      "archery/privilegeDetail",
      params,
      60_000,
    );
    return result.detail;
  },

  privilegeAudit(params: {
    applyId: number;
    auditStatus: number;
    auditRemark: string;
  }): Promise<void> {
    return invoke("archery/privilegeAudit", params, 60_000);
  },

  /** 待办列表：Archery /workflow/list/，只返回「当前轮到我的组审核」的工单。 */
  async workflowTodos(params: {
    limit: number;
    offset: number;
    search?: string;
    /** 0 全部 / 1 查询权限 / 2 SQL 上线 / 3 数据归档。 */
    workflowType?: number;
  }): Promise<WorkflowTodoPage> {
    const result = await invoke<{ page: WorkflowTodoPage }>(
      "archery/workflowTodos",
      params,
      60_000,
    );
    return result.page;
  },

  /** 把待办（审批记录号）解析成业务工单号与类型，供审核入口跳转。 */
  async workflowTodoTarget(params: { auditId: number }): Promise<WorkflowTodoTarget> {
    const result = await invoke<{ target: WorkflowTodoTarget }>(
      "archery/workflowTodoTarget",
      params,
      60_000,
    );
    return result.target;
  },

  /** 工单操作日志（workflow_type=1 查询权限工单，2 SQL 上线工单）。 */
  async workflowLogs(params: {
    workflowId: number;
    workflowType?: number;
  }): Promise<WorkflowLogPage> {
    const result = await invoke<{ page: WorkflowLogPage }>(
      "archery/workflowLog",
      params,
      60_000,
    );
    return result.page;
  },

  /** SQL 上线工单列表；audit=true 取「待我审核」。 */
  async sqlWorkflows(params: {
    limit: number;
    offset: number;
    search?: string;
    status?: string;
    instanceId?: number;
    groupId?: number;
    startDate?: string;
    endDate?: string;
    syntaxTypes?: string[];
    audit?: boolean;
  }): Promise<SqlWorkflowPage> {
    const result = await invoke<{ page: SqlWorkflowPage }>(
      "archery/sqlWorkflows",
      params,
      60_000,
    );
    return result.page;
  },

  /** 列表页的实例与资源组筛选下拉（取自 /sqlworkflow/ 页面）。 */
  async sqlWorkflowOptions(): Promise<{
    instances: SqlWorkflowOption[];
    groups: SqlWorkflowOption[];
  }> {
    const result = await invoke<{
      instances: SqlWorkflowOption[];
      groups: SqlWorkflowOption[];
    }>("archery/sqlWorkflowOptions", {}, 60_000);
    return { instances: result.instances ?? [], groups: result.groups ?? [] };
  },

  /** 工单详情：审批流 + 操作信息 + SQL 审核/执行结果。 */
  async sqlWorkflowDetail(params: { workflowId: number }): Promise<SqlWorkflowDetail> {
    const result = await invoke<{ detail: SqlWorkflowDetail }>(
      "archery/sqlWorkflowDetail",
      params,
      60_000,
    );
    return result.detail;
  },

  /** 工单回滚 SQL：rows 为 [执行语句, 回滚语句] 明细，sql 为拼好的整段回滚文本。 */
  sqlWorkflowRollback(params: { workflowId: number }): Promise<{
    rows: SqlRollbackRow[];
    sql: string;
  }> {
    return invoke<{ rows: SqlRollbackRow[]; sql: string }>(
      "archery/sqlWorkflowRollback",
      params,
      120_000,
    );
  },

  /** 提交回滚请求：用回滚语句新建一张 SQL 上线工单，返回新工单号。 */
  sqlWorkflowRollbackSubmit(params: {
    workflowId: number;
    title: string;
    groupId: number;
    instanceId: number;
    dbName: string;
  }): Promise<{ workflowId: number }> {
    return invoke("archery/sqlWorkflowRollbackSubmit", params, 120_000);
  },

  /**
   * 把文本 / 二进制文件写到本机（DBX iframe 沙箱拦截 <a download>，落盘由 sidecar 完成）。
   * askPath=true 时由 sidecar 弹系统「另存为」对话框（等待用户操作，超时放宽到 5 分钟）。
   * 二进制内容放 contentBase64（如 Excel 导出），sidecar 优先按它解码落盘。
   */
  saveTextFile(params: {
    filename: string;
    content: string;
    contentBase64?: string;
    askPath?: boolean;
    /** 另存为对话框的初始目录；为空或不存在时由 sidecar 回退下载目录。 */
    initialDir?: string;
  }): Promise<{ path?: string; canceled?: boolean }> {
    return invoke("archery/saveTextFile", params, 300_000);
  },

  /** 审核工单：pass=审核通过，reject=驳回。 */
  sqlWorkflowAudit(params: {
    workflowId: number;
    action: "pass" | "reject";
    remark: string;
  }): Promise<void> {
    return invoke("archery/sqlWorkflowAudit", params, 60_000);
  },

  /** 执行工单：auto=平台排队执行，manual=确认线下执行完成。 */
  sqlWorkflowExecute(params: {
    workflowId: number;
    mode: "auto" | "manual";
  }): Promise<void> {
    return invoke("archery/sqlWorkflowExecute", params, 120_000);
  },

  /** 定时执行，runDate 形如 2026-11-05 03:00。 */
  sqlWorkflowSchedule(params: { workflowId: number; runDate: string }): Promise<void> {
    return invoke("archery/sqlWorkflowSchedule", params, 60_000);
  },

  /** 审核人修改可执行时间；两端留空表示清空为「无限制」。 */
  sqlWorkflowAlterRunDate(params: {
    workflowId: number;
    runDateStart: string;
    runDateEnd: string;
  }): Promise<void> {
    return invoke("archery/sqlWorkflowAlterRunDate", params, 60_000);
  },

  /** 提交上线工单，返回新工单号。 */
  async sqlWorkflowSubmit(
    params: SqlSubmitForm & { demandUrl?: string },
  ): Promise<number> {
    const result = await invoke<{ workflowId: number }>(
      "archery/sqlWorkflowSubmit",
      { ...params } as Record<string, unknown>,
      120_000,
    );
    return result.workflowId;
  },

  /** 提交前的 SQL 检测（Archery 网页同样要求先检测）。 */
  sqlWorkflowCheck(params: {
    instanceId: number;
    dbName: string;
    sql: string;
  }): Promise<SqlCheckSummary> {
    return invoke<SqlCheckSummary>("archery/sqlWorkflowCheck", params, 120_000);
  },

  /** 提交弹窗的组下拉（带 group_id）与备份开关。 */
  async sqlSubmitOptions(): Promise<{
    groups: SqlWorkflowOption[];
    backupEnabled: boolean;
  }> {
    const result = await invoke<{
      groups: SqlWorkflowOption[];
      backupEnabled: boolean;
    }>("archery/sqlSubmitOptions", {}, 60_000);
    return { groups: result.groups ?? [], backupEnabled: Boolean(result.backupEnabled) };
  },

  /** 资源组在 SQL 上线工单下的审批流（提交页蓝色提示条）。 */
  async sqlWorkflowAuditors(params: { groupName: string }): Promise<{
    auditors: string;
    auditorsDisplay: string;
  }> {
    const result = await invoke<{ auditors: string; auditorsDisplay: string }>(
      "archery/sqlWorkflowAuditors",
      params,
      60_000,
    );
    return {
      auditors: result.auditors ?? "",
      auditorsDisplay: result.auditorsDisplay ?? "",
    };
  },

  /** 资源组内「支持上线」（can_write）的实例。 */
  async sqlWorkflowTargets(params: { groupName: string }): Promise<GroupInstance[]> {
    const result = await invoke<{ instances: GroupInstance[] }>(
      "archery/sqlWorkflowTargets",
      params,
      60_000,
    );
    return result.instances ?? [];
  },

  async loadState(): Promise<UiState | null> {
    const result = await invoke<{ state: UiState | null }>("archery/uiState/get");
    return result.state ?? null;
  },

  saveState(state: UiState): Promise<void> {
    return invoke("archery/uiState/set", { state });
  },

  async copy(text: string): Promise<void> {
    // 宿主桥优先，失败再降级；全部失败时保持原有「复制失败」提示语义。
    const copied = await writeClipboard(text);
    if (!copied) throw new Error("clipboard write failed");
  },
};
