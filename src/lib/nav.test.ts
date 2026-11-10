import { describe, expect, it } from "vitest";
import {
  activeChild,
  activeNav,
  breadcrumbs,
  isPlaceholder,
  metricRoute,
  navItems,
  routeKey,
  type Route,
} from "./nav";

describe("navItems", () => {
  it("exposes the six Archery console entries in fixed order", () => {
    expect(navItems.map((item) => item.key)).toEqual([
      "dashboard",
      "audit",
      "query",
      "optimize",
      "tools",
      "docs",
    ]);
  });

  it("gives SQL audit its two sub entries", () => {
    const audit = navItems.find((item) => item.key === "audit");
    expect(audit?.children?.map((child) => child.key)).toEqual(["online", "analyze"]);
  });

  it("keeps every menu label as an i18n key", () => {
    for (const item of navItems) {
      expect(item.label.startsWith("nav.")).toBe(true);
      for (const child of item.children ?? []) {
        expect(child.label.startsWith("nav.")).toBe(true);
      }
    }
  });
});

describe("routeKey", () => {
  it("distinguishes sub pages", () => {
    const routes: Route[] = [
      { page: "dashboard" },
      { page: "audit", tab: "online" },
      { page: "audit", tab: "analyze" },
      { page: "query" },
      { page: "history" },
      { page: "describe" },
    ];
    const keys = routes.map(routeKey);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it("keeps the key stable for the same route", () => {
    expect(routeKey({ page: "audit", tab: "online" })).toBe(
      routeKey({ page: "audit", tab: "online" }),
    );
  });
});

describe("activeNav", () => {
  it("highlights the owning top level entry", () => {
    expect(activeNav({ page: "dashboard" })).toBe("dashboard");
    expect(activeNav({ page: "query" })).toBe("query");
    expect(activeNav({ page: "history" })).toBe("query");
    expect(activeNav({ page: "describe" })).toBe("query");
    expect(activeNav({ page: "audit", tab: "online" })).toBe("audit");
    expect(activeNav({ page: "audit", tab: "analyze" })).toBe("audit");
    expect(activeNav({ page: "optimize" })).toBe("optimize");
    expect(activeNav({ page: "tools" })).toBe("tools");
    expect(activeNav({ page: "docs" })).toBe("docs");
  });

  // 待办列表由顶栏铃铛进入，不属于左侧任何菜单项。
  it("leaves the sidebar unselected on the todo list", () => {
    expect(activeNav({ page: "todo" })).toBe("");
    expect(activeChild({ page: "todo" })).toBe("");
  });
});

describe("breadcrumbs", () => {
  it("renders one label for top level pages", () => {
    expect(breadcrumbs({ page: "dashboard" })).toEqual(["nav.dashboard"]);
    expect(breadcrumbs({ page: "query" })).toEqual(["nav.query"]);
    expect(breadcrumbs({ page: "todo" })).toEqual(["todo.title"]);
  });

  it("renders parent plus child for nested pages", () => {
    expect(breadcrumbs({ page: "history" })).toEqual(["nav.query", "nav.history"]);
    expect(breadcrumbs({ page: "describe" })).toEqual(["nav.query", "nav.describe"]);
    expect(breadcrumbs({ page: "grant" })).toEqual(["nav.query", "nav.queryGrant"]);
    expect(breadcrumbs({ page: "audit", tab: "online" })).toEqual([
      "nav.audit",
      "nav.auditOnline",
    ]);
    expect(breadcrumbs({ page: "audit", tab: "analyze" })).toEqual([
      "nav.audit",
      "nav.auditAnalyze",
    ]);
  });
});

describe("metricRoute", () => {
  // 首页「SQL 查询工单」统计的就是权限申请工单，更多要落到权限管理的申请工单列表。
  it("sends the query ticket metric to the privilege apply list", () => {
    expect(metricRoute("query")).toEqual({ page: "grant" });
  });

  it("keeps the SQL release jump", () => {
    expect(metricRoute("workflow")).toEqual({ page: "audit", tab: "online" });
  });

  // 有效平台用户与实例数量没有对应列表页，卡片上不展示「更多」，点了也不该跳转。
  it("returns nothing for metrics without a list page", () => {
    expect(metricRoute("users")).toBeNull();
    expect(metricRoute("instances")).toBeNull();
  });
});

describe("isPlaceholder", () => {
  it("marks entries that are not implemented yet", () => {
    expect(isPlaceholder({ page: "audit", tab: "analyze" })).toBe(true);
    expect(isPlaceholder({ page: "optimize" })).toBe(true);
    expect(isPlaceholder({ page: "tools" })).toBe(true);
    expect(isPlaceholder({ page: "docs" })).toBe(true);
  });

  it("treats SQL release as an implemented page", () => {
    expect(isPlaceholder({ page: "audit", tab: "online" })).toBe(false);
  });

  it("keeps implemented pages out of the placeholder state", () => {
    expect(isPlaceholder({ page: "dashboard" })).toBe(false);
    expect(isPlaceholder({ page: "query" })).toBe(false);
    expect(isPlaceholder({ page: "history" })).toBe(false);
    expect(isPlaceholder({ page: "describe" })).toBe(false);
  });
});
