import { describe, expect, it } from "vitest";
import { pruneTabTargets } from "./state";

function savedTabs() {
  return [
    { instanceName: "test-express-rw", dbName: "schedule-dev", sql: "select 1" },
    { instanceName: "materials_test", dbName: "materials_test", sql: "select 2" },
    { instanceName: "", dbName: "", sql: "select 3" },
  ];
}

describe("pruneTabTargets", () => {
  it("保留当前环境存在的实例目标", () => {
    const tabs = savedTabs();
    const result = pruneTabTargets(tabs, ["test-express-rw", "materials_test"]);
    expect(result.dropped).toBe(0);
    expect(result.tabs).toBe(tabs);
    expect(result.tabs.map((tab) => tab.instanceName)).toEqual([
      "test-express-rw",
      "materials_test",
      "",
    ]);
  });

  it("清掉不属于当前环境的实例与库，但保留 SQL", () => {
    const result = pruneTabTargets(savedTabs(), ["materials_test"]);
    expect(result.dropped).toBe(1);
    expect(result.tabs[0]).toEqual({ instanceName: "", dbName: "", sql: "select 1" });
    expect(result.tabs[1]).toEqual({
      instanceName: "materials_test",
      dbName: "materials_test",
      sql: "select 2",
    });
  });

  it("实例清单为空时不动任何页签，避免误清", () => {
    const tabs = savedTabs();
    const result = pruneTabTargets(tabs, []);
    expect(result.dropped).toBe(0);
    expect(result.tabs).toBe(tabs);
  });

  it("没有目标的页签不计入清除数", () => {
    const tabs = [{ instanceName: "", dbName: "", sql: "select 3" }];
    const result = pruneTabTargets(tabs, ["materials_test"]);
    expect(result.dropped).toBe(0);
    expect(result.tabs).toBe(tabs);
  });
});
