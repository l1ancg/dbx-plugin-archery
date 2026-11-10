/**
 * 单文件产物不能引 vue-router（iframe 里没有 URL 语义），
 * 导航状态用可序列化的 Route 联合类型表达。
 *
 * 菜单结构对齐 Archery 1.14 的 base.html（SB Admin 2 侧栏）：
 * Dashboard / SQL审核 / SQL查询 / SQL优化 / 工具插件 / 相关文档，
 * 前四项带二级菜单，相关文档是直接链接。
 */
export type NavKey = "dashboard" | "audit" | "query" | "optimize" | "tools" | "docs";

export type AuditTab = "online" | "analyze";

export type Route =
  | { page: "dashboard" }
  /** 顶栏铃铛进入的待办列表，不占左侧菜单项。 */
  | { page: "todo" }
  | { page: "query" }
  | { page: "history" }
  | { page: "describe" }
  | { page: "dict" }
  | { page: "grant" }
  | { page: "audit"; tab: AuditTab }
  | { page: "optimize" }
  | { page: "tools" }
  | { page: "docs" };

export interface NavChild {
  key: string;
  label: string;
}

export interface NavItem {
  key: NavKey;
  label: string;
  icon: string;
  children?: NavChild[];
}

export const navItems: NavItem[] = [
  { key: "dashboard", label: "nav.dashboard", icon: "dashboard" },
  {
    key: "audit",
    label: "nav.audit",
    icon: "audit",
    children: [
      { key: "online", label: "nav.auditOnline" },
      { key: "analyze", label: "nav.auditAnalyze" },
    ],
  },
  {
    key: "query",
    label: "nav.query",
    icon: "query",
    children: [
      { key: "online", label: "nav.queryOnline" },
      { key: "dict", label: "nav.queryDict" },
      { key: "grant", label: "nav.queryGrant" },
    ],
  },
  {
    key: "optimize",
    label: "nav.optimize",
    icon: "optimize",
    children: [
      { key: "advisor", label: "nav.optimizeAdvisor" },
      { key: "slow", label: "nav.optimizeSlow" },
    ],
  },
  {
    key: "tools",
    label: "nav.tools",
    icon: "tools",
    children: [
      { key: "archive", label: "nav.toolsArchive" },
      { key: "my2sql", label: "nav.toolsMy2sql" },
      { key: "schemasync", label: "nav.toolsSchemasync" },
    ],
  },
  { key: "docs", label: "nav.docs", icon: "docs" },
];

export function routeKey(route: Route): string {
  if (route.page === "audit") {
    return `audit:${route.tab}`;
  }
  return route.page;
}

/** 待办列表在 Archery 里不是菜单项，返回空串让侧栏保持无选中。 */
export function activeNav(route: Route): NavKey | "" {
  switch (route.page) {
    case "history":
    case "describe":
    case "dict":
    case "grant":
      return "query";
    case "todo":
      return "";
    default:
      return route.page;
  }
}

/** 返回当前路由命中的二级菜单 key，未命中时为空串。 */
export function activeChild(route: Route): string {
  switch (route.page) {
    case "query":
      return "online";
    case "dict":
      return "dict";
    case "grant":
      return "grant";
    case "audit":
      return route.tab;
    default:
      return "";
  }
}

export function childRoute(parent: NavKey, child: string): Route {
  switch (parent) {
    case "audit":
      return { page: "audit", tab: child === "analyze" ? "analyze" : "online" };
    case "query":
      if (child === "dict") return { page: "dict" };
      if (child === "grant") return { page: "grant" };
      return { page: "query" };
    case "optimize":
      return { page: "optimize" };
    case "tools":
      return { page: "tools" };
    case "dashboard":
      return { page: "dashboard" };
    default:
      return { page: "docs" };
  }
}

/**
 * Dashboard 指标卡「更多」的目标页：
 * 上线工单进 SQL 上线列表，查询工单进权限管理的申请工单列表；
 * 有效平台用户与实例数量没有对应列表页，返回 null（卡片不展示「更多」）。
 */
export function metricRoute(metric: string): Route | null {
  switch (metric) {
    case "workflow":
      return { page: "audit", tab: "online" };
    case "query":
      return { page: "grant" };
    default:
      return null;
  }
}

export function breadcrumbs(route: Route): string[] {
  switch (route.page) {
    case "todo":
      return ["todo.title"];
    case "history":
      return ["nav.query", "nav.history"];
    case "describe":
      return ["nav.query", "nav.describe"];
    case "dict":
      return ["nav.query", "nav.queryDict"];
    case "grant":
      return ["nav.query", "nav.queryGrant"];
    case "audit":
      return ["nav.audit", route.tab === "online" ? "nav.auditOnline" : "nav.auditAnalyze"];
    default:
      return [`nav.${route.page}`];
  }
}

export function isPlaceholder(route: Route): boolean {
  // SQL 上线已实现；SQL 分析仍是占位页。
  if (route.page === "audit") return route.tab === "analyze";
  return route.page === "optimize" || route.page === "tools" || route.page === "docs";
}
