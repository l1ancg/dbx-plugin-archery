import { describe, expect, it } from "vitest";
import {
  filterByName,
  matchesNode,
  mergeUnique,
  resourceDetail,
  treeKey,
  visibleDbs,
  visibleInstances,
  visibleTables,
} from "./tree";

describe("filterByName", () => {
  const items = [{ name: "orders" }, { name: "Users" }, { name: "order_items" }];

  it("filters case-insensitively", () => {
    expect(filterByName(items, "ORDER").map((item) => item.name)).toEqual([
      "orders",
      "order_items",
    ]);
  });

  it("returns all items for empty keyword", () => {
    expect(filterByName(items, "   ")).toHaveLength(3);
  });
});

describe("mergeUnique", () => {
  it("keeps existing order and skips duplicates", () => {
    const merged = mergeUnique(
      [{ name: "a" }, { name: "b" }],
      [{ name: "b" }, { name: "c" }],
    );
    expect(merged.map((item) => item.name)).toEqual(["a", "b", "c"]);
  });
});

describe("resourceDetail", () => {
  it("uses the second column as readable detail", () => {
    expect(resourceDetail(["orders", "订单表", "InnoDB"])).toBe("订单表");
    expect(resourceDetail(["orders"])).toBe("");
    expect(resourceDetail([null, null])).toBe("");
  });
});

const instance = { id: 1, name: "demo-mysql", type: "master", dbType: "mysql" };
const dbShop = { name: "demo_shop", values: ["demo_shop", "订单主库"] };
const dbReport = { name: "demo_report", values: ["demo_report", "报表库"] };
const tbOrders = { name: "orders", values: ["orders", "订单表"] };
const tbUsers = { name: "users", values: ["users", "用户表"] };
const tbRoutes = { name: "routes", values: ["routes", "线路表"] };
const dbsByName = { "demo-mysql": [dbShop, dbReport] };
const tablesByKey = {
  [treeKey("demo-mysql", "demo_shop")]: [tbOrders, tbUsers],
  [treeKey("demo-mysql", "demo_report")]: [tbRoutes],
};

describe("matchesNode", () => {
  it("matches name case-insensitively and also the comment column", () => {
    expect(matchesNode(dbShop, "SHOP")).toBe(true);
    expect(matchesNode(dbShop, "订单")).toBe(true);
    expect(matchesNode(tbOrders, "订单表")).toBe(true);
  });

  it("misses when neither name nor comment contains the keyword", () => {
    expect(matchesNode(dbShop, "invoice")).toBe(false);
  });

  it("treats a blank keyword as a match so the tree stays complete", () => {
    expect(matchesNode(dbShop, "   ")).toBe(true);
  });
});

describe("visibleInstances", () => {
  it("keeps an instance when only a descendant database matches", () => {
    expect(visibleInstances([instance], dbsByName, tablesByKey, "report")).toEqual([instance]);
  });

  it("keeps an instance when only a descendant table matches", () => {
    expect(visibleInstances([instance], dbsByName, tablesByKey, "routes")).toEqual([instance]);
  });

  it("keeps an instance whose own name matches even without loaded children", () => {
    expect(visibleInstances([instance], {}, {}, "demo-mysql")).toEqual([instance]);
  });

  it("returns nothing when no level matches", () => {
    expect(visibleInstances([instance], dbsByName, tablesByKey, "invoice")).toEqual([]);
  });

  it("returns every instance for a blank keyword", () => {
    expect(visibleInstances([instance], {}, {}, "  ")).toEqual([instance]);
  });
});

describe("visibleDbs", () => {
  it("keeps only the database whose table matches", () => {
    expect(visibleDbs(instance, dbsByName["demo-mysql"], tablesByKey, "orders")).toEqual([
      dbShop,
    ]);
  });

  it("keeps the database whose own name matches", () => {
    expect(visibleDbs(instance, dbsByName["demo-mysql"], tablesByKey, "REPORT")).toEqual([
      dbReport,
    ]);
  });

  it("shows all databases when the instance name itself matches", () => {
    expect(visibleDbs(instance, dbsByName["demo-mysql"], tablesByKey, "demo-mysql")).toEqual([
      dbShop,
      dbReport,
    ]);
  });

  it("returns nothing when neither the database nor its tables match", () => {
    expect(visibleDbs(instance, dbsByName["demo-mysql"], tablesByKey, "invoice")).toEqual([]);
  });
});

describe("visibleTables", () => {
  it("keeps only tables matching the keyword", () => {
    expect(visibleTables(instance, dbShop, [tbOrders, tbUsers], "ORDER")).toEqual([tbOrders]);
  });

  it("shows all tables when the database name itself matches", () => {
    expect(visibleTables(instance, dbReport, [tbRoutes], "report")).toEqual([tbRoutes]);
  });

  it("shows all tables when the instance name itself matches", () => {
    expect(
      visibleTables(instance, dbShop, [tbOrders, tbUsers], "demo-mysql"),
    ).toEqual([tbOrders, tbUsers]);
  });

  it("returns nothing when no table matches", () => {
    expect(visibleTables(instance, dbReport, [tbRoutes], "orders")).toEqual([]);
  });
});
