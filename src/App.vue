<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { archeryApi, getConnectionId, setConnectionId, setTwofaHandler } from "./api";
import { setLocale, t } from "./i18n";
import { shouldRefreshAnnouncement } from "./lib/announcement";
import { resolveConnectionId, resolveConnectionName } from "./lib/context";
import { describeError, errorKind } from "./lib/errors";
import {
  activeChild,
  activeNav,
  childRoute,
  isPlaceholder,
  metricRoute,
  routeKey,
  type AuditTab,
  type NavKey,
  type Route,
} from "./lib/nav";
import { parseRangeLabel } from "./lib/dashboard";
import { presetRangeWithOffset } from "./lib/date-range";
import {
  MESSAGE_PANEL_ID,
  appendPanel,
  closePanel,
  createDescribePanel,
  createResultPanel,
  describeTitle,
  findDescribePanel,
  isClosable,
  movePanel,
  nextResultIndex,
  targetResultPanel,
} from "./lib/panels";
import {
  explainSql,
  formatSql,
  runSteps,
  sqlSummary,
  type RunStep,
} from "./lib/sql";
import {
  SESSION_CHECK_INTERVAL_MS,
  isSessionExpiredKind,
  probeDelayForInterval,
  sessionRetryDelay,
  shouldProbeSession,
} from "./lib/session";
import { pruneTabTargets } from "./lib/state";
import {
  createTab,
  type Announcement,
  type DashboardPayload,
  type DashboardTrends,
  type DescribePanelTab,
  type InstanceItem,
  type Overview,
  type PanelTab,
  type QueryLogRow,
  type QueryTab,
  type ResourceItem,
  type ResultPanelTab,
  type UiState,
} from "./types";
import AnnouncementBar from "./components/AnnouncementBar.vue";
import AppHeader from "./components/AppHeader.vue";
import ChangePasswordDialog from "./components/ChangePasswordDialog.vue";
import DescribePanel from "./components/DescribePanel.vue";
import HistoryPanel from "./components/HistoryPanel.vue";
import InstanceTree from "./components/InstanceTree.vue";
import NavIcon from "./components/NavIcon.vue";
import OtpDialog from "./components/OtpDialog.vue";
import PickerSelect from "./components/PickerSelect.vue";
import ResultGrid from "./components/ResultGrid.vue";
import SideNav from "./components/SideNav.vue";
import SqlEditor from "./components/SqlEditor.vue";
import TablePicker from "./components/TablePicker.vue";
import TwoFactorDialog from "./components/TwoFactorDialog.vue";
import AuditOnlineView from "./views/AuditOnlineView.vue";
import TodoView from "./views/TodoView.vue";
import DashboardView from "./views/DashboardView.vue";
import DescribeView from "./views/DescribeView.vue";
import DictView from "./views/DictView.vue";
import HistoryView from "./views/HistoryView.vue";
import PlaceholderView from "./views/PlaceholderView.vue";
import PrivilegesView from "./views/PrivilegesView.vue";

const plugin = window.dbxPlugin;
const connectionName = ref("Archery");
const overview = ref<Overview | null>(null);
const instances = ref<InstanceItem[]>([]);
const tabs = ref<QueryTab[]>([createTab()]);
const activeIndex = ref(0);
const booting = ref(true);
const bootError = ref("");
/** 会话续期失败且无法自动恢复时为真：顶栏转为「会话已过期」。 */
const sessionExpired = ref(false);
const editorHeight = ref(380);
const sidebarWidth = ref(240);
/** 上次导出 Excel 保存到的目录；「另存为」默认打开这里。 */
const exportDir = ref("");
const dbCache = ref<Record<string, ResourceItem[]>>({});
const toasts = ref<Array<{ id: number; text: string; error: boolean }>>([]);
const route = ref<Route>({ page: "dashboard" });
const auditOpen = ref(false);
const dashboardPayload = ref<DashboardPayload | null>(null);
const dashboardTrends = ref<DashboardTrends | null>(null);
const dashboardLoading = ref(false);
/**
 * 首页按日统计图的时间范围。默认就是 Archery 的默认窗口口径（最近 7 日，右端含明天），
 * 不依赖页面是否能解析出区间文案——解析不到时也不会退化成「全部」。
 */
const dashboardRange = ref({ ...presetRangeWithOffset("last7", 1) });
const otp = ref({
  open: false,
  error: "",
  submitting: false,
  user: "",
  method: "totp" as "totp" | "sms",
});
/** 顶栏账号菜单里的两个安全设置弹窗。 */
const twoFactorOpen = ref(false);
const changePasswordOpen = ref(false);

/**
 * 任何接口返回 twofa_required 时都会走到这里：弹验证码框，
 * 校验通过后 api 层自动重试原请求，避免整页卡在「需要 2FA」上。
 */
setTwofaHandler(async () => {
  otpResolver?.(false);
  otp.value.error = "";
  otp.value.open = true;
  if (!otp.value.user) otp.value.user = overview.value?.username ?? "";
  return await new Promise<boolean>((resolve) => {
    otpResolver = resolve;
  });
});
/** 当前编辑器 tab 里已打开的表结构 tab（按打开顺序）。 */
const describeTabs = computed(() =>
  active.value.panels.filter((panel): panel is DescribePanelTab => panel.kind === "describe"),
);
/** 右侧「表」下拉显示的表名：取最近打开的那张。 */
const lastDescribeTable = computed(() => {
  const panels = describeTabs.value;
  return panels.length > 0 ? panels[panels.length - 1].tableName : "";
});
/** 独立「表结构」页（只有状态恢复能到达）取最近一张表的结构。 */
const describeView = computed(() => {
  const panels = describeTabs.value;
  const panel = panels[panels.length - 1];
  return {
    loading: panel?.loading ?? false,
    error: panel?.error ?? "",
    table: panel?.table ?? "",
    data: panel?.data ?? null,
    elapsed: panel?.elapsed ?? 0,
    fetchedAt: panel?.fetchedAt ?? "",
  };
});
/** 会话中途过期时等待用户补验证码的 Promise resolver。 */
let otpResolver: ((ok: boolean) => void) | null = null;
const sqlEditor = ref<{
  focus: () => void;
  getCursor: () => number;
  selectedText: () => string;
} | null>(null);
const favorites = ref<QueryLogRow[]>([]);
const favoritesLoading = ref(false);
const favoriteId = ref("");
/** 表名下拉候选：按“实例+库”缓存，避免每次聚焦都重新拉表列表。 */
const tableCache = ref<Record<string, string[]>>({});
const tableOptions = ref<string[]>([]);
const tableLoading = ref(false);
/** 实例资源树默认收起，与 Archery 网页一致；需要时从查询面板标题栏打开。 */
const resourceOpen = ref(false);
/** 每次左侧菜单导航都递增，作为路由页面的 key：重复点击同一菜单也会重新挂载并重新拉取。 */
const navEntry = ref(0);

const active = computed<QueryTab>(() => tabs.value[activeIndex.value] ?? tabs.value[0]);
/** 当前激活的内层 tab；状态里指向已关闭的 tab 时退回第一条。 */
const activePanel = computed<PanelTab>(() => {
  const tab = active.value;
  return tab.panels.find((panel) => panel.id === tab.activePanel) ?? tab.panels[0];
});
/** 表结构 tab 独占整列的滚动规则，见 style.css 的 `.query-column.is-describe`。 */
const describeMode = computed(() => activePanel.value?.kind === "describe");
/** 查询历史 tab：面板高度跟当前页行数走，溢出交给整列滚动，见 `.query-column.is-history`。 */
const historyMode = computed(() => activePanel.value?.kind === "history");
/** 结果 tab：列表高度跟每页条数走，溢出同样交给整列滚动，见 `.query-column.is-result-grid`。 */
const resultGridMode = computed(() => activePanel.value?.kind === "result");
/** 模板里按 kind 取具体 tab，避免在 v-if 链上做类型收窄。 */
const resultPanel = computed(() =>
  activePanel.value.kind === "result" ? activePanel.value : null,
);
const currentDescribe = computed(() =>
  activePanel.value.kind === "describe" ? activePanel.value : null,
);

/** 内层 tab 文案：系统 tab 固定，结果带序号，表结构按库名去重后取表名。 */
function panelLabel(panel: PanelTab): string {
  switch (panel.kind) {
    case "result":
      return `${t("result.title")} ${panel.index}`;
    case "message":
      return t("message.title");
    case "history":
      return t("history.title");
    default:
      return describeTitle(active.value.panels, panel);
  }
}

function activatePanel(id: string): void {
  active.value.activePanel = id;
}

function closePanelTab(id: string): void {
  const tab = active.value;
  const next = closePanel(tab.panels, tab.activePanel, id);
  tab.panels = next.panels;
  tab.activePanel = next.activeId;
}

/** 拖拽重排：只有结果 / 表结构两类动态 tab 参与，系统 tab 钉死在前。 */
let draggingPanelId = "";
function dragPanel(id: string, event: DragEvent): void {
  draggingPanelId = id;
  event.dataTransfer?.setData("text/plain", id);
}
function dropPanel(id: string): void {
  const source = draggingPanelId;
  draggingPanelId = "";
  if (!source || source === id) return;
  const tab = active.value;
  tab.panels = movePanel(tab.panels, source, id);
}

/**
 * 把新 tab 追加进数组后取回数组里的响应式代理。
 * 直接改 createXxxPanel() 返回的原始对象不会触发重渲染（它还没被 proxy 包过）。
 */
function pushPanel<T extends PanelTab>(tab: QueryTab, panel: T): T {
  tab.panels = appendPanel(tab.panels, panel);
  return tab.panels[tab.panels.length - 1] as T;
}

/**
 * 执行 / 载入历史要落到的结果 tab：当前激活的就是结果 tab 就用它（就地刷新，不再每次执行新开一条），
 * 停在系统 / 表结构页签上时退回最后一条；全被关掉时补一条空的。
 */
function ensureResultPanel(tab: QueryTab): ResultPanelTab {
  const existing = targetResultPanel(tab.panels, tab.activePanel);
  if (existing) return existing;
  return pushPanel(tab, createResultPanel(nextResultIndex(tab.panels)));
}

/** 「+」：手动新开一条空结果 tab，并激活它。 */
function addResultTab(): void {
  const tab = active.value;
  const panel = pushPanel(tab, createResultPanel(nextResultIndex(tab.panels)));
  tab.activePanel = panel.id;
}

/** 「−」：关掉当前结果 tab；停在系统 / 表结构页签上时按钮为禁用态。 */
function closeResultTab(): void {
  if (activePanel.value.kind !== "result") return;
  closePanelTab(activePanel.value.id);
}

/** 载入历史后回到结果 tab：没有就补一条，避免停留在消息 / 查询历史页。 */
function activateLatestResult(tab: QueryTab): void {
  tab.activePanel = ensureResultPanel(tab).id;
}

/** 行数候选与 Archery 查询页保持一致；-1 表示不限制，sidecar 会映射成 limit_num=0。 */
const limitOptions = [100, 500, 1000, -1];

function limitOptionLabel(value: number): string {
  return value < 0 ? t("editor.limitMax") : String(value);
}

/** 面板标题栏上的当前查询目标：实例 / 库，未选实例时退回提示文案。 */
const targetPath = computed(() => {
  const tab = active.value;
  if (!tab?.instanceName) return t("editor.pickInstance");
  return tab.dbName ? `${tab.instanceName} / ${tab.dbName}` : tab.instanceName;
});
/** 目标完整度只影响小圆点颜色：未选 / 只选实例 / 实例+库。 */
const targetState = computed<"empty" | "partial" | "ready">(() => {
  const tab = active.value;
  if (!tab?.instanceName) return "empty";
  return tab.dbName ? "ready" : "partial";
});

/** 空串表示当前页不归属任何菜单项（待办列表）。 */
const navActive = computed<NavKey | "">(() => activeNav(route.value));
const navChild = computed(() => activeChild(route.value));
const placeholderPage = computed(() => isPlaceholder(route.value));
const auditTab = computed<AuditTab>(() =>
  route.value.page === "audit" ? route.value.tab : "online",
);

let toastSequence = 0;
let saveTimer: ReturnType<typeof setTimeout> | undefined;

function toast(text: string, isError = false): void {
  const id = ++toastSequence;
  toasts.value.push({ id, text, error: isError });
  setTimeout(() => {
    toasts.value = toasts.value.filter((item) => item.id !== id);
  }, 2800);
}

function applyTheme(): void {
  const appearance = plugin?.theme?.appearance;
  document.documentElement.dataset.theme = appearance === "light" ? "light" : "dark";
}

function tabLabel(tab: QueryTab): string {
  if (tab.title.trim()) return tab.title.trim();
  if (tab.instanceName) {
    return tab.dbName ? `${tab.instanceName}/${tab.dbName}` : tab.instanceName;
  }
  return t("nav.query");
}

const statusClass = computed<"online" | "pending" | "offline">(() => {
  if (bootError.value || sessionExpired.value) return "offline";
  if (otp.value.open) return "pending";
  return "online";
});

const statusLabel = computed(() => {
  if (bootError.value) return t("status.failed");
  if (sessionExpired.value) return t("status.sessionExpired");
  if (otp.value.open) return t("app.pending");
  return overview.value ? t("app.connected") : t("status.connecting");
});

/** 系统公告：所有页面共用一条，取自 Archery 首页的 alert 块。 */
const announcement = ref<Announcement | null>(null);
/** 上次取公告的时间戳（毫秒）；0 表示还没取过。 */
const announcementLoadedAt = ref(0);
/** 并发去重：快速切页时同一个 TTL 窗口内只发一次请求。 */
let announcementLoading = false;

/**
 * 拉取系统公告。60 秒内复用上次结果，避免每次切页都打一次 Archery 首页；
 * 取不到（只读连接 / 无 Dashboard 权限 / 旧版 sidecar）时保持不显示，不阻塞页面。
 */
async function loadAnnouncement(): Promise<void> {
  if (!shouldRefreshAnnouncement(announcementLoadedAt.value, Date.now())) return;
  if (announcementLoading) return;
  announcementLoading = true;
  try {
    const payload = await archeryApi.announcement();
    announcement.value = payload.announcement ?? null;
  } catch {
    // 公告不是关键路径：保留上一次结果，60 秒后再试。
  } finally {
    // 失败也记时间，避免接口不可用时每次切页都重试。
    announcementLoadedAt.value = Date.now();
    announcementLoading = false;
  }
}

/** 顶栏铃铛右侧的待办总数（Archery base.html 的 `{{ todo }}`）；null 表示取不到，不显示。 */
const todoCount = ref<number | null>(null);

async function loadTodoCount(): Promise<void> {
  try {
    // 只取 1 条，用 total 当计数：与待办列表同一接口，不额外加端点。
    const page = await archeryApi.workflowTodos({ limit: 1, offset: 0 });
    todoCount.value = page.total ?? 0;
  } catch {
    // 只读连接、无审核权限、旧版 sidecar 都取不到，铃铛保持无数字。
    todoCount.value = null;
  }
}

const dbOptions = computed<ResourceItem[]>(() => {
  const name = active.value?.instanceName;
  if (!name) return [];
  return dbCache.value[name] ?? [];
});
/** 实例 / 库下拉的候选：只取名字，交给 PickerSelect 过滤与窗口化渲染。 */
const instanceNames = computed(() => instances.value.map((item) => item.name));
const dbNames = computed(() => dbOptions.value.map((item) => item.name));
/** 正在拉库清单的实例：下拉里显示加载态，避免打开时是空的。 */
const dbPending = ref<Record<string, boolean>>({});
const dbLoading = computed(() => Boolean(dbPending.value[active.value?.instanceName ?? ""]));
const instancesLoading = ref(false);

function parseRouteKey(value: string): Route | null {
  switch (value) {
    case "dashboard":
      return { page: "dashboard" };
    case "todo":
      return { page: "todo" };
    case "query":
      return { page: "query" };
    case "history":
      return { page: "history" };
    case "describe":
      return { page: "describe" };
    case "dict":
      return { page: "dict" };
    case "grant":
      return { page: "grant" };
    case "audit:online":
      return { page: "audit", tab: "online" };
    case "audit:analyze":
      return { page: "audit", tab: "analyze" };
    case "optimize":
      return { page: "optimize" };
    case "tools":
      return { page: "tools" };
    case "docs":
      return { page: "docs" };
    default:
      return null;
  }
}

function go(next: Route): void {
  navEntry.value += 1;
  route.value = next;
  if (next.page === "query") {
    void ensureDbOptions(active.value.instanceName);
  }
  if (next.page === "query" || next.page === "history") {
    void loadFavorites();
  }
  if (next.page === "dashboard") {
    void loadDashboard();
  }
  persist();
}

/** 只有不带二级菜单的项（Dashboard / 相关文档）直接导航，父项交给面板展开。 */
function onNavSelect(key: NavKey): void {
  switch (key) {
    case "dashboard":
      go({ page: "dashboard" });
      break;
    case "docs":
      go({ page: "docs" });
      break;
    default:
      go({ page: "query" });
      break;
  }
}

function onSelectChild(parent: NavKey, child: string): void {
  go(childRoute(parent, child));
}

function onNavToggle(key: NavKey, open: boolean): void {
  if (key === "audit") auditOpen.value = open;
  persist();
}

function onMetricMore(key: string): void {
  const next = metricRoute(key);
  if (!next) {
    toast(t("toast.noNotifications"));
    return;
  }
  if (next.page === "audit") auditOpen.value = true;
  go(next);
}

async function loadDashboard(): Promise<void> {
  if (dashboardLoading.value) return;
  dashboardLoading.value = true;
  try {
    const payload = await archeryApi.dashboard();
    dashboardPayload.value = payload;
    dashboardTrends.value = payload.trends ?? null;
    // 区间以选择器为准（默认最近 7 日、右端含明天，与 Archery 首页同口径），
    // 不依赖页面能否解析出区间文案；页面的区间只用于省一次请求。
    const current = dashboardRange.value;
    const pageRange = parseRangeLabel(payload.trends?.rangeLabel ?? "");
    if (pageRange && pageRange.start === current.start && pageRange.end === current.end) {
      return;
    }
    const scoped = await archeryApi.dashboardTrends(current).catch(() => null);
    if (scoped?.trends) {
      dashboardTrends.value = scoped.trends;
    }
  } catch {
    // 旧版 sidecar / 无权限都走降级，首页仍要能看实例分布。
    dashboardPayload.value = { degraded: true, reason: "unavailable" };
  } finally {
    dashboardLoading.value = false;
  }
}

/**
 * 统计时间范围变化：只重取两张按日统计图。
 * 四张指标卡是 Archery 的全量计数，不随区间变化，因此不重新拉整页。
 */
async function changeDashboardRange(start: string, end: string): Promise<void> {
  if (dashboardRange.value.start === start && dashboardRange.value.end === end) return;
  dashboardRange.value = { start, end };
  if (!start || !end) {
    await loadDashboard();
    return;
  }
  dashboardLoading.value = true;
  try {
    const result = await archeryApi.dashboardTrends({ start, end });
    dashboardTrends.value = result.trends ?? dashboardTrends.value;
  } catch (error) {
    toast(`${t("dashboard.trendFailed")}：${describeError(error)}`, true);
  } finally {
    dashboardLoading.value = false;
  }
}

/** 会话续期定时器与探测状态：连上后启动，卸载、连接失败或确认过期时清理。 */
let sessionTimer: ReturnType<typeof setTimeout> | undefined;
let sessionKeepAliveActive = false;
let sessionProbing = false;
let sessionProbeFailures = 0;
let lastSessionProbeAt = 0;

function clearSessionTimer(): void {
  if (sessionTimer) clearTimeout(sessionTimer);
  sessionTimer = undefined;
}

function stopSessionKeepAlive(): void {
  sessionKeepAliveActive = false;
  clearSessionTimer();
}

function scheduleSessionProbe(delay: number = SESSION_CHECK_INTERVAL_MS): void {
  clearSessionTimer();
  if (!sessionKeepAliveActive) return;
  sessionTimer = setTimeout(() => {
    void probeSession();
  }, delay);
}

/**
 * 后台静默续期：Archery 按最后活动时间计算会话有效期，长时间不操作就会失效。
 * 这里定期触发 sidecar 的一个无害登录态请求；会话已失效时 sidecar 会用保存的密码自动重登。
 * 网络类失败不打扰用户，只做指数退避重试。
 */
async function probeSession(): Promise<void> {
  if (sessionProbing) return;
  sessionProbing = true;
  // 无论是定时到点还是切回页面手动触发，都先清掉待执行的定时器，保证同一时刻只有一条链。
  clearSessionTimer();
  let delay = SESSION_CHECK_INTERVAL_MS;
  let restart = true;
  try {
    const result = await archeryApi.sessionKeepAlive();
    lastSessionProbeAt = Date.now();
    if (!result.enabled) {
      // 连接配置关掉了自动续期（keep_alive_secs=0），不再排下一轮。
      restart = false;
      return;
    }
    sessionProbeFailures = 0;
    // 检查节奏跟随连接配置的 keep_alive_secs，配置改小/改大都能真正生效。
    delay = probeDelayForInterval(result.intervalSec);
    if (sessionExpired.value) {
      sessionExpired.value = false;
      toast(t("session.recovered"));
    }
  } catch (error) {
    lastSessionProbeAt = Date.now();
    const kind = errorKind(error);
    if (isSessionExpiredKind(kind)) {
      // 已确认需要人工验证（2FA 由 api 层弹框）：停掉心跳，避免反复弹框；
      // 用户完成验证后 loadWorkspace 会重新启动心跳。
      restart = false;
      sessionProbeFailures = 0;
      sessionKeepAliveActive = false;
      if (!sessionExpired.value) {
        sessionExpired.value = true;
        toast(t("session.expired"), true);
      }
    } else {
      sessionProbeFailures += 1;
      delay = sessionRetryDelay(sessionProbeFailures);
    }
  } finally {
    sessionProbing = false;
    if (restart) {
      scheduleSessionProbe(delay);
    } else {
      clearSessionTimer();
    }
  }
}

function startSessionKeepAlive(): void {
  sessionKeepAliveActive = true;
  sessionProbeFailures = 0;
  sessionExpired.value = false;
  // 立即探一次：把会话活动时间推到最新，顺便确认配置是否启用自动续期。
  scheduleSessionProbe(0);
}

/** 标签页从后台切回来时补一次：后台定时器会被浏览器节流，切回后立刻续期更稳。 */
function onVisibilityChange(): void {
  if (document.visibilityState !== "visible") return;
  if (!sessionKeepAliveActive) return;
  if (shouldProbeSession(lastSessionProbeAt, Date.now())) {
    void probeSession();
  }
}

async function loadWorkspace(): Promise<void> {
  booting.value = true;
  try {
    overview.value = await archeryApi.overview();
    otp.value.user = overview.value?.username ?? "";
    instances.value = await archeryApi.instances();
    bootError.value = "";
    await restoreState();
    await ensureDbOptions(active.value.instanceName);
    void loadAnnouncement();
    void loadDashboard();
    void loadFavorites();
    void loadTodoCount();
    startSessionKeepAlive();
  } catch (error) {
    handleFailure(error);
  } finally {
    booting.value = false;
  }
}

async function bootstrap(): Promise<void> {
  booting.value = true;
  bootError.value = "";
  try {
    const result = await archeryApi.connect();
    // 宿主 context 没有 connectionId 时，用 sidecar 回传的 id 兜底。
    if (!getConnectionId() && result.connectionId) {
      setConnectionId(result.connectionId);
    }
    if (result.pendingTwofa) {
      otp.value.method = result.twofaType ?? "totp";
      otp.value.user = result.username ?? otp.value.user;
      otp.value.open = true;
    } else {
      booting.value = false;
      await loadWorkspace();
    }
  } catch (error) {
    const kind = errorKind(error);
    if (kind === "twofa_required") {
      otp.value.open = true;
    } else {
      bootError.value = describeError(error);
    }
  } finally {
    booting.value = false;
  }
}

function handleFailure(error: unknown): void {
  const kind = errorKind(error);
  bootError.value = describeError(error);
  stopSessionKeepAlive();
  if (kind === "twofa_required" || kind === "auth_required") {
    otp.value.open = true;
    otp.value.error = "";
    return;
  }
  toast(describeError(error), true);
}

async function ensureDbOptions(instanceName: string): Promise<void> {
  if (!instanceName || dbCache.value[instanceName] || dbPending.value[instanceName]) return;
  dbPending.value[instanceName] = true;
  try {
    const instance = instances.value.find((item) => item.name === instanceName);
    dbCache.value[instanceName] = await archeryApi.resources({
      instanceName,
      instanceId: instance?.id,
      resourceType: "database",
    });
  } catch (error) {
    toast(describeError(error), true);
  } finally {
    dbPending.value[instanceName] = false;
  }
}

/** 打开「库」下拉时补一次候选：实例刚切换、启动时没拉到都能在这里补上。 */
function openDbOptions(): void {
  void ensureDbOptions(active.value.instanceName);
}

/** 打开「实例」下拉时若还没拉到实例清单，补一次（失败只提示，不影响页面）。 */
async function ensureInstances(): Promise<void> {
  if (instances.value.length > 0 || instancesLoading.value) return;
  instancesLoading.value = true;
  try {
    instances.value = await archeryApi.instances();
  } catch (error) {
    toast(describeError(error), true);
  } finally {
    instancesLoading.value = false;
  }
}

async function submitOtp(code: string): Promise<void> {
  otp.value.submitting = true;
  otp.value.error = "";
  try {
    const result = await archeryApi.connect(code);
    if (result.pendingTwofa) {
      otp.value.error = t("login.invalid");
      return;
    }
    otp.value.open = false;
    otpResolver?.(true);
    otpResolver = null;
    await loadWorkspace();
  } catch (error) {
    otp.value.error = describeError(error);
  } finally {
    otp.value.submitting = false;
  }
}

function closeOtp(): void {
  otp.value.open = false;
  otpResolver?.(false);
  otpResolver = null;
  if (!overview.value) {
    bootError.value = t("status.failed");
  }
}

function instanceOf(name: string): InstanceItem | undefined {
  return instances.value.find((item) => item.name === name);
}

/** 当前选区 + 光标：所有执行入口（按钮 / 编辑器快捷键 / 全局快捷键）都读同一份。 */
function editorSelection(): { selection: string; cursor?: number } {
  return {
    selection: sqlEditor.value?.selectedText() ?? "",
    cursor: sqlEditor.value?.getCursor(),
  };
}

/**
 * 执行一条语句；`fresh` 为真时新开一条结果页签（逐条执行的第 2 条起）。
 * 返回是否成功：批量执行时失败即停，不再往下跑。
 */
async function execute(tab: QueryTab, sql: string, info = "", fresh = false): Promise<boolean> {
  // 就地刷新当前结果页签：想保留上一次结果，用页签右侧的「+」先开一条空的。
  const panel = fresh
    ? pushPanel(tab, createResultPanel(nextResultIndex(tab.panels)))
    : ensureResultPanel(tab);
  panel.sql = sql;
  panel.limitNum = tab.limitNum;
  panel.running = true;
  panel.result = null;
  panel.error = "";
  panel.errorKind = "";
  panel.info = "";
  tab.activePanel = panel.id;
  tab.error = "";
  tab.errorKind = "";
  tab.info = "";
  try {
    const result = await archeryApi.query({
      instanceName: tab.instanceName,
      dbName: tab.dbName,
      sqlContent: sql,
      limitNum: tab.limitNum,
    });
    panel.result = result;
    if (result.columns.length === 0) {
      panel.info = t("result.noColumns");
    } else if (info) {
      panel.info = info;
    } else if (result.warning) {
      panel.info = result.warning;
    }
    tab.info = panel.info;
    return true;
  } catch (error) {
    // 失败的那次留在自己的 tab 里（可关闭），同时回到「消息」看错误详情。
    panel.error = describeError(error);
    panel.errorKind = errorKind(error);
    tab.error = panel.error;
    tab.errorKind = panel.errorKind;
    tab.activePanel = MESSAGE_PANEL_ID;
    if (panel.errorKind === "twofa_required" || panel.errorKind === "auth_required") {
      otp.value.open = true;
    }
    return false;
  } finally {
    panel.running = false;
  }
}

/** 整批执行期间保持「运行中」：逐条之间不给按钮 / 快捷键重入的机会。 */
async function withRunning(tab: QueryTab, task: () => Promise<void>): Promise<void> {
  tab.running = true;
  try {
    await task();
  } finally {
    tab.running = false;
  }
}

/** 逐条顺序执行：第 1 条落当前结果页签，其余各开一条；中途失败即停。 */
async function runAllSteps(tab: QueryTab, steps: RunStep[]): Promise<void> {
  await withRunning(tab, async () => {
    for (const [at, step] of steps.entries()) {
      const info =
        step.total > 1
          ? t("editor.statementHint", { index: step.index, total: step.total })
          : "";
      const ok = await execute(tab, step.text, info, at > 0);
      if (!ok) return;
    }
  });
}

async function run(selection = "", cursor?: number): Promise<void> {
  const tab = active.value;
  if (tab.running) return;
  if (!tab.instanceName || !tab.dbName) {
    toast(t("editor.needInstance"), true);
    return;
  }
  const picked = editorSelection();
  const steps = runSteps(tab.sql, selection || picked.selection, cursor ?? picked.cursor);
  if (steps.length === 0) {
    toast(t("editor.needSql"), true);
    return;
  }
  await runAllSteps(tab, steps);
}

async function explain(): Promise<void> {
  const tab = active.value;
  if (tab.running) return;
  if (!tab.instanceName || !tab.dbName) {
    toast(t("editor.needInstance"), true);
    return;
  }
  const picked = editorSelection();
  // 执行计划只解释一条：选区优先取第 1 条，否则取光标所在语句。
  const sql = runSteps(tab.sql, picked.selection, picked.cursor)[0]?.text ?? "";
  if (!sql.trim()) {
    toast(t("editor.needSql"), true);
    return;
  }
  const plan = explainSql(sql, instanceOf(tab.instanceName)?.dbType ?? "");
  if (!plan) {
    toast(t("editor.explainUnsupported"), true);
    return;
  }
  await withRunning(tab, async () => {
    await execute(tab, plan, t("editor.explainDone"));
  });
}

function beautify(): void {
  const tab = active.value;
  if (!tab.sql.trim()) {
    toast(t("editor.needSql"), true);
    return;
  }
  try {
    tab.sql = formatSql(tab.sql, instanceOf(tab.instanceName)?.dbType ?? "");
    persist();
    toast(t("editor.formatted"));
  } catch {
    toast(t("editor.formatUnsupported"), true);
  }
}

async function loadFavorites(): Promise<void> {
  if (favoritesLoading.value) return;
  favoritesLoading.value = true;
  try {
    const page = await archeryApi.queryLogs({ limit: 50, star: true });
    favorites.value = page.rows ?? [];
  } catch {
    // 常用查询只是快捷入口，拉不到时不影响主流程。
  } finally {
    favoritesLoading.value = false;
  }
}

function applyFavorite(): void {
  const row = favorites.value.find((item) => item.id === Number(favoriteId.value));
  favoriteId.value = "";
  if (!row) return;
  const tab = active.value;
  tab.sql = row.sql;
  if (row.alias) tab.title = row.alias;
  if (row.instanceName) tab.instanceName = row.instanceName;
  if (row.dbName) tab.dbName = row.dbName;
  activateLatestResult(tab);
  void ensureDbOptions(tab.instanceName);
  persist();
  toast(t("toast.loadedHistory"));
}

/** 表名下拉候选：打开下拉时按当前实例+库懒加载，命中缓存直接复用。 */
async function loadTableOptions(): Promise<void> {
  const tab = active.value;
  if (!tab.instanceName || !tab.dbName) {
    tableOptions.value = [];
    return;
  }
  const key = `${tab.instanceName}\u0000${tab.dbName}`;
  const cached = tableCache.value[key];
  if (cached) {
    tableOptions.value = cached;
    return;
  }
  tableLoading.value = true;
  try {
    const instance = instanceOf(tab.instanceName);
    const list = await archeryApi.resources({
      instanceName: tab.instanceName,
      instanceId: instance?.id,
      resourceType: "table",
      dbName: tab.dbName,
    });
    tableCache.value[key] = list.map((item) => item.name);
    tableOptions.value = tableCache.value[key];
  } catch (error) {
    toast(describeError(error), true);
  } finally {
    tableLoading.value = false;
  }
}

function applyTarget(instance: InstanceItem, db: string): void {
  const tab = active.value;
  tab.instanceName = instance.name;
  tab.dbName = db;
  void ensureDbOptions(instance.name);
  void loadTableOptions();
  persist();
}

function pickTable(payload: { instance: InstanceItem; db: string; table: string }): void {
  applyTarget(payload.instance, payload.db);
  void selectTable(payload.table);
}

/** 选中表后开一条表结构 tab；同一张表再点一次就原地刷新，不重复开。 */
async function selectTable(name: string): Promise<void> {
  const tab = active.value;
  const instance = instanceOf(tab.instanceName);
  if (!instance || !tab.dbName || !name) {
    toast(t("editor.needInstance"), true);
    return;
  }
  const target = { instanceName: instance.name, dbName: tab.dbName, tableName: name };
  const existing = findDescribePanel(tab.panels, target);
  const panel = existing ?? pushPanel(tab, createDescribePanel(target));
  tab.activePanel = panel.id;
  await fetchDescribe(instance, panel);
}

async function fetchDescribe(instance: InstanceItem, panel: DescribePanelTab): Promise<void> {
  const started = performance.now();
  panel.loading = true;
  panel.error = "";
  panel.data = null;
  panel.elapsed = 0;
  panel.fetchedAt = "";
  try {
    panel.data = await archeryApi.describe({
      instanceName: instance.name,
      instanceId: instance.id,
      dbName: panel.dbName,
      tbName: panel.tableName,
    });
  } catch (error) {
    panel.error = describeError(error);
  } finally {
    panel.loading = false;
    panel.elapsed = (performance.now() - started) / 1000;
    panel.fetchedAt = new Date().toLocaleTimeString();
  }
}

function loadHistory(row: QueryLogRow): void {
  const tab = active.value;
  tab.sql = row.sql;
  if (row.instanceName) tab.instanceName = row.instanceName;
  if (row.dbName) tab.dbName = row.dbName;
  activateLatestResult(tab);
  void ensureDbOptions(tab.instanceName);
  go({ page: "query" });
  toast(t("toast.loadedHistory"));
}

async function copy(text: string, label: string): Promise<void> {
  try {
    await archeryApi.copy(text);
    toast(`${label} · ${t("app.copied")}`);
  } catch (error) {
    toast(describeError(error), true);
  }
}

function addTab(): void {
  const tab = createTab({
    instanceName: active.value.instanceName,
    dbName: active.value.dbName,
  });
  tabs.value.push(tab);
  activeIndex.value = tabs.value.length - 1;
  persist();
}

function closeTab(index: number): void {
  if (tabs.value.length === 1) {
    tabs.value = [createTab()];
    activeIndex.value = 0;
  } else {
    tabs.value.splice(index, 1);
    activeIndex.value = Math.min(activeIndex.value, tabs.value.length - 1);
  }
  persist();
}

function selectTab(index: number): void {
  activeIndex.value = index;
  void ensureDbOptions(active.value.instanceName);
  void loadTableOptions();
}

/** 换实例：库必须重选，顺带把库清单拉回来。 */
function onInstancePick(name: string): void {
  const tab = active.value;
  if (tab.instanceName === name) return;
  tab.instanceName = name;
  tab.dbName = "";
  void ensureDbOptions(name);
  persist();
}

function onDbPick(name: string): void {
  const tab = active.value;
  if (tab.dbName === name) return;
  tab.dbName = name;
  persist();
}

async function restoreState(): Promise<void> {
  try {
    const state = await archeryApi.loadState();
    if (!state) return;
    if (Array.isArray(state.tabs) && state.tabs.length > 0) {
      const restored = state.tabs.map((saved) =>
        createTab({
          ...saved,
          // 老状态里可能没有/是脏行数，退回默认值，避免下拉选不中任何一项
          limitNum: Number.isFinite(saved.limitNum) ? saved.limitNum : 100,
        }),
      );
      // 别的环境遗留 / 本环境已下线的实例：清掉目标但保留 SQL，避免页签指向不存在的实例。
      const pruned = pruneTabTargets(restored, instances.value.map((item) => item.name));
      tabs.value = pruned.tabs;
      activeIndex.value = Math.min(Math.max(0, state.activeIndex ?? 0), tabs.value.length - 1);
      if (pruned.dropped > 0) toast(t("toast.foreignTarget", { n: pruned.dropped }));
    }
    if (state.editorHeight) editorHeight.value = state.editorHeight;
    if (state.sidebarWidth) sidebarWidth.value = state.sidebarWidth;
    if (typeof state.exportDir === "string") exportDir.value = state.exportDir;
    if (typeof state.auditOpen === "boolean") auditOpen.value = state.auditOpen;
    if (typeof state.route === "string") {
      const restored = parseRouteKey(state.route);
      if (restored) route.value = restored;
    }
  } catch {
    // 状态只是锦上添花，读取失败不影响使用。
  }
}

/** 导出成功后记住目录：下次「另存为」默认打开它，并随 uiState 落盘。 */
function onExportDir(dir: string): void {
  if (!dir || dir === exportDir.value) return;
  exportDir.value = dir;
  persist();
}

function persist(): void {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    const state: UiState = {
      tabs: tabs.value.map((tab) => ({
        title: tab.title,
        instanceName: tab.instanceName,
        dbName: tab.dbName,
        sql: tab.sql,
        limitNum: tab.limitNum,
      })),
      activeIndex: activeIndex.value,
      editorHeight: editorHeight.value,
      sidebarWidth: sidebarWidth.value,
      exportDir: exportDir.value,
      route: routeKey(route.value),
      auditOpen: auditOpen.value,
    };
    void archeryApi.saveState(state).catch(() => undefined);
  }, 600);
}

function dragEditor(event: MouseEvent): void {
  event.preventDefault();
  const startY = event.clientY;
  const startHeight = editorHeight.value;
  const maxHeight = Math.max(180, window.innerHeight - 260);
  const move = (moveEvent: MouseEvent) => {
    editorHeight.value = Math.min(
      Math.max(240, startHeight + moveEvent.clientY - startY),
      maxHeight,
    );
  };
  const up = () => {
    window.removeEventListener("mousemove", move);
    window.removeEventListener("mouseup", up);
    persist();
  };
  window.addEventListener("mousemove", move);
  window.addEventListener("mouseup", up);
}

function dragSidebar(event: MouseEvent): void {
  event.preventDefault();
  const startX = event.clientX;
  const startWidth = sidebarWidth.value;
  const move = (moveEvent: MouseEvent) => {
    sidebarWidth.value = Math.min(
      Math.max(200, startWidth + moveEvent.clientX - startX),
      420,
    );
  };
  const up = () => {
    window.removeEventListener("mousemove", move);
    window.removeEventListener("mouseup", up);
    persist();
  };
  window.addEventListener("mousemove", move);
  window.addEventListener("mouseup", up);
}

function onKeydown(event: KeyboardEvent): void {
  if (!(event.ctrlKey || event.metaKey) || event.key !== "Enter") return;
  if (route.value.page !== "query") return;
  const target = event.target as HTMLElement | null;
  if (target?.closest(".cm-editor")) return;
  event.preventDefault();
  void run();
}

onMounted(async () => {
  window.addEventListener("keydown", onKeydown);
  document.addEventListener("visibilitychange", onVisibilityChange);
  // 宿主在 ready 之后才填充 context，必须先 await，否则 connectionId 读到空值。
  await plugin?.ready;
  const context = plugin?.context as Record<string, unknown> | undefined;
  setConnectionId(resolveConnectionId(context));
  connectionName.value = resolveConnectionName(context);
  setLocale(plugin?.locale);
  applyTheme();
  document.title = t("app.title");
  await bootstrap();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown);
  document.removeEventListener("visibilitychange", onVisibilityChange);
  stopSessionKeepAlive();
  if (saveTimer) clearTimeout(saveTimer);
});

watch(tabs, persist, { deep: true });
/**
 * 切页时检查一次公告：所有页面共用同一份数据，
 * 60 秒 TTL 内直接复用，超过才重新请求 Archery 首页。
 */
watch(
  () => routeKey(route.value),
  () => {
    void loadAnnouncement();
  },
);
/** 离开待办页时补一次计数：审核后直接跳到别的页，铃铛数字也要跟着更新。 */
watch(
  () => route.value.page,
  (page, prev) => {
    if (prev === "todo" && page !== "todo") void loadTodoCount();
  },
);
watch(
  () => [active.value?.instanceName, active.value?.dbName] as const,
  ([name, db]) => {
    tableOptions.value = [];
    if (name) void ensureDbOptions(name);
    if (name && db) void loadTableOptions();
  },
);
</script>

<template>
  <div class="app-shell">
    <AppHeader
      :connection-name="connectionName"
      :brand="dashboardPayload?.brand ?? ''"
      :brand-suffix="dashboardPayload?.brandSuffix ?? ''"
      :username="overview?.username ?? ''"
      :version="overview?.version ?? ''"
      :read-only="overview?.readOnly ?? false"
      :status="statusClass"
      :status-label="statusLabel"
      :todo-count="todoCount"
      @home="go({ page: 'dashboard' })"
      @notifications="go({ page: 'todo' })"
      @settings="toast(t('toast.settingsHint'))"
      @two-factor="twoFactorOpen = true"
      @change-password="changePasswordOpen = true"
      @logout="toast(t('toast.logoutHint'))"
    />

    <div v-if="booting && !instances.length && !bootError" class="boot-screen">
      <span class="spinner"></span>
      <span>{{ t("status.connecting") }}</span>
    </div>

    <div v-else-if="bootError && !instances.length" class="boot-screen">
      <strong>{{ t("status.failed") }}</strong>
      <span class="boot-detail">{{ bootError }}</span>
      <button class="btn" @click="bootstrap">{{ t("app.retry") }}</button>
    </div>

    <div v-else class="app-body">
      <SideNav
        :active="navActive"
        :active-child="navChild"
        :audit-open="auditOpen"
        :version="overview?.version ?? ''"
        @select="onNavSelect"
        @select-child="onSelectChild"
        @toggle="onNavToggle"
      />

      <main class="app-main">
        <AnnouncementBar :announcement="announcement" />

        <DashboardView
          v-if="route.page === 'dashboard'"
          :overview="overview"
          :instances="instances"
          :payload="dashboardPayload"
          :trends="dashboardTrends"
          :loading="dashboardLoading"
          :range-start="dashboardRange.start"
          :range-end="dashboardRange.end"
          @more="onMetricMore"
          @refresh="loadWorkspace"
          @range-change="changeDashboardRange"
        />

        <section v-else-if="route.page === 'query'" class="query-page">
          <InstanceTree
            v-if="resourceOpen"
            class="instance-panel"
            :instances="instances"
            :active-instance="active.instanceName"
            :active-db="active.dbName"
            :style="{ width: `${sidebarWidth}px` }"
            @pick-db="(payload) => applyTarget(payload.instance, payload.db)"
            @pick-table="pickTable"
            @refresh="loadWorkspace"
          />
          <div v-if="resourceOpen" class="splitter-v" @mousedown="dragSidebar"></div>

          <div
            class="query-column"
            :class="{
              'is-describe': describeMode,
              'is-history': historyMode,
              'is-result-grid': resultGridMode,
            }"
          >
            <section class="panel query-panel">
              <div class="panel-heading panel-heading-tabs">
                <div class="panel-head-tabs">
                  <div
                    v-for="(tab, index) in tabs"
                    :key="tab.id"
                    class="panel-head-tab"
                    :class="{ active: index === activeIndex }"
                    @click="selectTab(index)"
                  >
                    <span class="tab-name">{{ tabLabel(tab) }}</span>
                    <button
                      class="tab-close"
                      :title="t('tabs.close')"
                      @click.stop="closeTab(index)"
                    >
                      ×
                    </button>
                  </div>
                </div>
                <button class="panel-head-add" :title="t('tabs.new')" @click="addTab">
                  <NavIcon name="plus" :size="12" />
                </button>

                <span class="spacer"></span>

                <span
                  class="query-target"
                  :class="`is-${targetState}`"
                  :title="targetPath"
                >
                  <span class="query-target-dot"></span>
                  <span class="query-target-text">{{ targetPath }}</span>
                </span>

                <span v-if="overview?.readOnly" class="query-flag">
                  {{ t("app.readOnly") }}
                </span>

                <button
                  class="btn btn-default btn-xs query-tool"
                  :class="{ 'is-on': resourceOpen }"
                  :title="t('nav.resources')"
                  @click="resourceOpen = !resourceOpen"
                >
                  <NavIcon name="database" :size="12" />
                  <span>{{ t("nav.resources") }}</span>
                </button>

                <label class="head-select" :title="t('editor.favorites')">
                  <NavIcon name="star" :size="12" class="head-select-icon" />
                  <select
                    v-model="favoriteId"
                    class="head-select-input"
                    :aria-label="t('editor.favorites')"
                    :disabled="favorites.length === 0"
                    @change="applyFavorite"
                  >
                    <option value="">
                      {{
                        favoritesLoading
                          ? t("app.loading")
                          : favorites.length
                            ? t("editor.favorites")
                            : t("editor.favoritesEmpty")
                      }}
                    </option>
                    <option v-for="row in favorites" :key="row.id" :value="String(row.id)">
                      {{ row.alias || sqlSummary(row.sql, 64) }}
                    </option>
                  </select>
                </label>

                <button
                  class="btn btn-default btn-xs query-tool"
                  :title="t('editor.clear')"
                  @click="active.sql = ''"
                >
                  <NavIcon name="trash" :size="12" />
                  <span>{{ t("editor.clear") }}</span>
                </button>
              </div>

              <div class="panel-body query-body" :style="{ height: `${editorHeight}px` }">
                <div class="editor-wrap">
                  <SqlEditor
                    ref="sqlEditor"
                    v-model="active.sql"
                    :tab-id="active.id"
                    @run="(payload) => run(payload.selection, payload.cursor)"
                  />
                </div>

                <aside class="query-config">
                  <div class="config-section">
                    <div class="config-section-head">
                      <span class="config-section-title">{{ t("query.sectionTarget") }}</span>
                    </div>

                    <div class="config-field">
                      <span class="config-field-label">{{ t("editor.instance") }}</span>
                      <PickerSelect
                        :model-value="active.instanceName"
                        :options="instanceNames"
                        :placeholder="t('editor.pickInstance')"
                        :loading="instancesLoading"
                        clearable
                        @update:model-value="onInstancePick"
                        @open="ensureInstances"
                      />
                    </div>

                    <div class="config-field">
                      <span class="config-field-label">{{ t("editor.database") }}</span>
                      <PickerSelect
                        :model-value="active.dbName"
                        :options="dbNames"
                        :placeholder="t('editor.pickDatabase')"
                        :loading="dbLoading"
                        :disabled="!active.instanceName"
                        clearable
                        @update:model-value="onDbPick"
                        @open="openDbOptions"
                      />
                    </div>

                    <div class="config-field">
                      <span class="config-field-label">{{ t("query.fieldTable") }}</span>
                      <TablePicker
                        :value="lastDescribeTable"
                        :tables="tableOptions"
                        :loading="tableLoading"
                        :disabled="!active.instanceName || !active.dbName"
                        @open="loadTableOptions"
                        @select="selectTable"
                      />
                    </div>

                    <!-- 行数跟着查询目标走：与实例 / 库 / 表同组，底部执行区只留动作 -->
                    <label class="config-field config-field-limit">
                      <span class="config-field-label">{{ t("editor.limit") }}</span>
                      <select
                        v-model.number="active.limitNum"
                        class="form-control form-control-sm"
                        :title="t('editor.limit')"
                        :aria-label="t('editor.limit')"
                      >
                        <!-- 旧版本存下来的自定义行数不在候选里，单独留一项，别让它显示成空白 -->
                        <option v-if="!limitOptions.includes(active.limitNum)" :value="active.limitNum">
                          {{ active.limitNum }}
                        </option>
                        <option v-for="value in limitOptions" :key="value" :value="value">
                          {{ limitOptionLabel(value) }}
                        </option>
                      </select>
                    </label>
                  </div>

                  <div class="config-run">
                    <div class="config-actions">
                      <button class="btn btn-default" @click="beautify">
                        {{ t("editor.beautify") }}
                      </button>
                      <button class="btn btn-default" @click="explain">
                        {{ t("editor.explain") }}
                      </button>
                      <button class="btn btn-success" :disabled="active.running" @click="run()">
                        <span v-if="active.running" class="spinner"></span>
                        <span>{{ t("editor.run") }}</span>
                        <span class="kbd">Ctrl ↵</span>
                      </button>
                    </div>

                    <ul class="config-tips">
                      <li :title="t('query.tipRun')">{{ t("query.tipRun") }}</li>
                      <li :title="t('query.tipLimit')">{{ t("query.tipLimit") }}</li>
                    </ul>
                  </div>
                </aside>
              </div>
            </section>

            <div class="splitter-h" @mousedown="dragEditor"></div>

            <section class="panel result-panel">
              <div class="panel-heading result-heading">
                <h2 class="panel-title">{{ t("result.panelTitle") }}</h2>

                <ul class="nav-tabs result-tabs">
                  <li v-for="panel in active.panels" :key="panel.id">
                    <button
                      class="nav-tab"
                      :class="{
                        active: active.activePanel === panel.id,
                        'is-dynamic': isClosable(panel),
                      }"
                      :draggable="isClosable(panel)"
                      @click="activatePanel(panel.id)"
                      @dragstart="dragPanel(panel.id, $event)"
                      @dragover.prevent
                      @drop="dropPanel(panel.id)"
                    >
                      {{ panelLabel(panel) }}
                      <span v-if="panel.kind === 'message' && active.error" class="badge warn"
                        >!</span
                      >
                      <span
                        v-if="isClosable(panel)"
                        class="nav-tab-close"
                        :title="t('app.close')"
                        @click.stop="closePanelTab(panel.id)"
                      >
                        <NavIcon name="close" :size="11" />
                      </span>
                    </button>
                  </li>
                </ul>

                <span class="spacer"></span>
                <span v-if="active.running" class="result-running">
                  <span class="spinner"></span>
                  {{ t("app.loading") }}
                </span>
                <div class="result-tab-actions">
                  <button
                    class="tab-action"
                    type="button"
                    :title="t('result.addTab')"
                    :aria-label="t('result.addTab')"
                    @click="addResultTab"
                  >
                    +
                  </button>
                  <button
                    class="tab-action"
                    type="button"
                    :title="t('result.closeTab')"
                    :aria-label="t('result.closeTab')"
                    :disabled="activePanel.kind !== 'result'"
                    @click="closeResultTab"
                  >
                    −
                  </button>
                </div>
              </div>

              <div class="panel-body result-body">
                <ResultGrid
                  v-if="resultPanel"
                  :result="resultPanel.result"
                  :loading="resultPanel.running"
                  :error="resultPanel.error"
                  :info="resultPanel.info"
                  :row-limit="resultPanel.limitNum"
                  :export-dir="exportDir"
                  @copy="copy"
                  @toast="(text, isError) => toast(text, isError)"
                  @save-dir="onExportDir"
                />

                <div v-else-if="activePanel.kind === 'message'" class="panel-body">
                  <div v-if="active.error" class="alert alert-danger">{{ active.error }}</div>
                  <div v-else-if="active.info" class="alert alert-info">{{ active.info }}</div>
                  <div v-else class="empty">{{ t("message.empty") }}</div>
                </div>

                <HistoryPanel
                  v-else-if="activePanel.kind === 'history'"
                  :instances="instances"
                  @load="loadHistory"
                  @toast="(text) => toast(text)"
                />

                <DescribePanel
                  v-else-if="currentDescribe"
                  :loading="currentDescribe.loading"
                  :error="currentDescribe.error"
                  :table="currentDescribe.table"
                  :data="currentDescribe.data"
                  :elapsed="currentDescribe.elapsed"
                  :fetched-at="currentDescribe.fetchedAt"
                  @copy="copy"
                />
              </div>
            </section>
          </div>
        </section>

        <HistoryView
          v-else-if="route.page === 'history'"
          :instances="instances"
          @load="loadHistory"
          @toast="(text) => toast(text)"
        />

        <DescribeView
          v-else-if="route.page === 'describe'"
          :loading="describeView.loading"
          :error="describeView.error"
          :table="describeView.table"
          :data="describeView.data"
          :elapsed="describeView.elapsed"
          :fetched-at="describeView.fetchedAt"
          @copy="copy"
        />

        <DictView
          v-else-if="route.page === 'dict'"
          :key="navEntry"
          :instances="instances"
          @toast="(text, isError) => toast(text, isError)"
        />

        <PrivilegesView
          v-else-if="route.page === 'grant'"
          :key="navEntry"
          @toast="(text, isError) => toast(text, isError)"
        />

        <TodoView
          v-else-if="route.page === 'todo'"
          :key="navEntry"
          @total="(count) => (todoCount = count)"
          @toast="(text, isError) => toast(text, isError)"
        />

        <AuditOnlineView
          v-else-if="route.page === 'audit' && route.tab === 'online'"
          :key="navEntry"
          @toast="(text, isError) => toast(text, isError)"
        />

        <PlaceholderView
          v-else-if="placeholderPage"
          :route="route"
          @back="go({ page: 'query' })"
        />
      </main>
    </div>

    <OtpDialog
      :open="otp.open"
      :username="otp.user"
      :method="otp.method"
      :submitting="otp.submitting"
      :error="otp.error"
      @submit="submitOtp"
      @close="closeOtp"
    />

    <TwoFactorDialog
      :open="twoFactorOpen"
      @close="twoFactorOpen = false"
      @toast="(text, isError) => toast(text, isError)"
    />

    <ChangePasswordDialog
      :open="changePasswordOpen"
      :username="overview?.username ?? ''"
      @close="changePasswordOpen = false"
      @toast="(text, isError) => toast(text, isError)"
    />

    <div class="toasts">
      <div v-for="item in toasts" :key="item.id" class="toast" :class="{ error: item.error }">
        {{ item.text }}
      </div>
    </div>
  </div>
</template>
