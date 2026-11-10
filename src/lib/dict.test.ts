import { describe, expect, it } from "vitest";
import {
  dictLetter,
  filterDictTables,
  groupDictTables,
  OTHER_LETTER,
  splitMatch,
  toDictTable,
  type DictTable,
} from "./dict";

describe("dictLetter", () => {
  it("uses the first character lowercased", () => {
    expect(dictLetter("biz_customer")).toBe("b");
    expect(dictLetter("Addr_geo_location")).toBe("a");
  });

  it("ignores leading whitespace", () => {
    expect(dictLetter("  orders")).toBe("o");
  });

  it("falls back to the other bucket for non-letters", () => {
    expect(dictLetter("123_table")).toBe(OTHER_LETTER);
    expect(dictLetter("_private")).toBe(OTHER_LETTER);
    expect(dictLetter("")).toBe(OTHER_LETTER);
  });
});

describe("toDictTable", () => {
  it("keeps name and comment as independent fields", () => {
    expect(
      toDictTable({ name: "customers", comment: "客户信息" }, "demo_schedule"),
    ).toEqual({ name: "customers", comment: "客户信息", dbName: "demo_schedule" });
  });

  it("trims the comment and tolerates an empty one", () => {
    expect(toDictTable({ name: "log_2026", comment: "  归档表  " }, "shop")).toEqual({
      name: "log_2026",
      comment: "归档表",
      dbName: "shop",
    });
    expect(toDictTable({ name: "orders", comment: "" }, "shop").comment).toBe("");
  });
});

describe("splitMatch", () => {
  it("returns the whole text when the keyword is blank", () => {
    expect(splitMatch("customers", "  ")).toEqual([{ text: "customers", hit: false }]);
    expect(splitMatch("", "")).toEqual([]);
  });

  it("splits every occurrence and keeps the original case", () => {
    expect(splitMatch("Biz_Customer_biz", "biz")).toEqual([
      { text: "Biz", hit: true },
      { text: "_Customer_", hit: false },
      { text: "biz", hit: true },
    ]);
  });

  it("marks the hit at the very start and at the very end", () => {
    expect(splitMatch("addresses", "add")).toEqual([
      { text: "add", hit: true },
      { text: "resses", hit: false },
    ]);
    expect(splitMatch("addresses", "ses")).toEqual([
      { text: "addres", hit: false },
      { text: "ses", hit: true },
    ]);
  });

  it("returns one plain segment when nothing matches", () => {
    expect(splitMatch("订单表", "客户")).toEqual([{ text: "订单表", hit: false }]);
  });

  it("matches CJK comments the same way as names", () => {
    expect(splitMatch("地址服务_地理位置", "地理")).toEqual([
      { text: "地址服务_", hit: false },
      { text: "地理", hit: true },
      { text: "位置", hit: false },
    ]);
  });
});

describe("filterDictTables", () => {
  const tables: DictTable[] = [
    { name: "customers", comment: "客户信息", dbName: "demo_schedule" },
    { name: "drivers", comment: "司机信息表", dbName: "demo_schedule" },
    { name: "add_geo_location", comment: "地址服务_地理位置信息表", dbName: "demo_schedule" },
  ];

  it("matches table names case-insensitively", () => {
    expect(filterDictTables(tables, "DRIVER").map((item) => item.name)).toEqual([
      "drivers",
    ]);
  });

  it("matches comments too", () => {
    expect(filterDictTables(tables, "客户").map((item) => item.name)).toEqual([
      "customers",
    ]);
  });

  it("returns everything for a blank keyword", () => {
    expect(filterDictTables(tables, "   ")).toHaveLength(3);
  });

  it("returns nothing when no table matches", () => {
    expect(filterDictTables(tables, "not-exists")).toEqual([]);
  });

  it("skips whitespace inside long comments when matching", () => {
    expect(filterDictTables(tables, "地理位置")).toHaveLength(1);
  });
});

describe("groupDictTables", () => {
  it("groups by letter, sorting letters and tables inside each group", () => {
    const tables: DictTable[] = [
      { name: "biz_team", comment: "班组信息表", dbName: "demo_schedule" },
      { name: "add_geo_location", comment: "地址服务_地理位置信息表", dbName: "demo_schedule" },
      { name: "biz_customer", comment: "客户信息", dbName: "demo_schedule" },
    ];
    const groups = groupDictTables(tables);
    expect(groups.map((group) => group.letter)).toEqual(["a", "b"]);
    expect(groups[1].tables.map((item) => item.name)).toEqual(["biz_customer", "biz_team"]);
    expect(groups[0].tables.map((item) => item.name)).toEqual(["add_geo_location"]);
  });

  it("pushes the other bucket to the end", () => {
    const groups = groupDictTables([
      { name: "123_table", comment: "", dbName: "shop" },
      { name: "orders", comment: "订单表", dbName: "shop" },
    ]);
    expect(groups.map((group) => group.letter)).toEqual(["o", OTHER_LETTER]);
  });

  it("returns an empty list for no tables", () => {
    expect(groupDictTables([])).toEqual([]);
  });
});
