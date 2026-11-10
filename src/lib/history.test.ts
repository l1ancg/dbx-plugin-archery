import { describe, expect, it } from "vitest";
import {
  costTone,
  facets,
  filterLogs,
  formatCount,
  historyColumns,
  logRowRange,
  pageWindow,
  splitTimestamp,
  sqlKind,
} from "./history";
import type { QueryLogRow } from "../types";

function row(patch: Partial<QueryLogRow>): QueryLogRow {
  return {
    id: 1,
    instanceName: "",
    dbName: "",
    sql: "",
    effectRow: 0,
    costTime: "0",
    userDisplay: "",
    favorite: false,
    alias: "",
    createTime: "",
    ...patch,
  };
}

const rows: QueryLogRow[] = [
  row({
    id: 1,
    instanceName: "demo-mysql",
    dbName: "shop",
    sql: "select * from orders",
    userDisplay: "demo_user",
    costTime: "0.023",
  }),
  row({
    id: 2,
    instanceName: "demo-pg",
    dbName: "warehouse",
    sql: "select count(*) from items",
    alias: "每日巡检",
    costTime: "0.120",
  }),
  row({
    id: 3,
    instanceName: "demo-mysql",
    dbName: "report",
    sql: "select 1",
    userDisplay: "alice",
  }),
];

describe("historyColumns", () => {
  it("lists the eight Archery history columns in order", () => {
    expect(historyColumns).toEqual([
      "history.colOperation",
      "history.colUser",
      "history.colInstance",
      "history.colDatabase",
      "history.colTime",
      "history.colSql",
      "history.colEffectRow",
      "history.colCost",
    ]);
  });
});

describe("filterLogs", () => {
  it("returns everything when the filter is empty", () => {
    expect(filterLogs(rows, { instance: "", db: "", keyword: "  " })).toHaveLength(3);
  });

  it("filters by instance and database", () => {
    expect(
      filterLogs(rows, { instance: "demo-mysql", db: "", keyword: "" }).map((item) => item.id),
    ).toEqual([1, 3]);
    expect(
      filterLogs(rows, { instance: "", db: "warehouse", keyword: "" }).map((item) => item.id),
    ).toEqual([2]);
  });

  it("matches the keyword across SQL, alias and user, ignoring case", () => {
    expect(filterLogs(rows, { instance: "", db: "", keyword: "ORDERS" })).toHaveLength(1);
    expect(filterLogs(rows, { instance: "", db: "", keyword: "每日" })[0]?.id).toBe(2);
    expect(filterLogs(rows, { instance: "", db: "", keyword: "ALICE" })[0]?.id).toBe(3);
  });

  it("combines instance and keyword filters", () => {
    expect(
      filterLogs(rows, { instance: "demo-mysql", db: "", keyword: "select 1" }).map(
        (item) => item.id,
      ),
    ).toEqual([3]);
  });
});

describe("facets", () => {
  it("collects sorted, de-duplicated instance and database options", () => {
    const result = facets(rows);
    expect(result.instances).toEqual(["demo-mysql", "demo-pg"]);
    expect(result.databases).toEqual(["report", "shop", "warehouse"]);
  });

  it("skips empty values", () => {
    const result = facets([row({ instanceName: "", dbName: "" })]);
    expect(result).toEqual({ instances: [], databases: [] });
  });
});

describe("pageWindow", () => {
  it("lists every page when the count fits the window", () => {
    expect(pageWindow(1, 5)).toEqual([1, 2, 3, 4, 5]);
    expect(pageWindow(3, 7)).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it("keeps the current page centered and elides both ends", () => {
    expect(pageWindow(10, 28)).toEqual([1, "gap", 8, 9, 10, 11, 12, "gap", 28]);
  });

  it("drops the leading ellipsis when the window touches the first page", () => {
    expect(pageWindow(2, 28)).toEqual([1, 2, 3, 4, 5, "gap", 28]);
    expect(pageWindow(4, 28)).toEqual([1, 2, 3, 4, 5, 6, "gap", 28]);
  });

  it("drops the trailing ellipsis when the window touches the last page", () => {
    expect(pageWindow(27, 28)).toEqual([1, "gap", 24, 25, 26, 27, 28]);
    expect(pageWindow(25, 28)).toEqual([1, "gap", 23, 24, 25, 26, 27, 28]);
  });

  it("clamps out-of-range pages and page counts", () => {
    expect(pageWindow(0, 3)).toEqual([1, 2, 3]);
    expect(pageWindow(99, 3)).toEqual([1, 2, 3]);
    expect(pageWindow(1, 0)).toEqual([1]);
  });
});

describe("splitTimestamp", () => {
  it("splits the Archery timestamp into date and clock parts", () => {
    expect(splitTimestamp("2026-11-07 09:59:05")).toEqual({
      date: "2026-11-07",
      time: "09:59:05",
    });
    expect(splitTimestamp("2026-11-07T09:59:05")).toEqual({
      date: "2026-11-07",
      time: "09:59:05",
    });
  });

  it("keeps a date-only value in the date part", () => {
    expect(splitTimestamp("2026-11-07")).toEqual({ date: "2026-11-07", time: "" });
    expect(splitTimestamp("")).toEqual({ date: "", time: "" });
  });
});

describe("costTone", () => {
  it("grades slow and warning queries by seconds", () => {
    expect(costTone("0.023")).toBe("normal");
    expect(costTone("0.35")).toBe("warn");
    expect(costTone("1.339243")).toBe("slow");
    expect(costTone("1")).toBe("slow");
  });

  it("falls back to normal for unparsable values", () => {
    expect(costTone("")).toBe("normal");
    expect(costTone("-")).toBe("normal");
    expect(costTone("abc")).toBe("normal");
  });
});

describe("sqlKind", () => {
  it("classifies reads, writes and DDL by the leading keyword", () => {
    expect(sqlKind("select * from orders")).toBe("read");
    expect(sqlKind("  EXPLAIN select 1")).toBe("read");
    expect(sqlKind("show tables")).toBe("read");
    expect(sqlKind("with t as (select 1) select * from t")).toBe("read");
    expect(sqlKind("insert into orders values (1)")).toBe("write");
    expect(sqlKind("update orders set a = 1")).toBe("write");
    expect(sqlKind("delete from orders")).toBe("write");
    expect(sqlKind("replace into orders values (1)")).toBe("write");
    expect(sqlKind("create table t (id int)")).toBe("ddl");
    expect(sqlKind("alter table t add column a int")).toBe("ddl");
    expect(sqlKind("drop table t")).toBe("ddl");
    expect(sqlKind("truncate table t")).toBe("ddl");
  });

  it("ignores leading comments and treats unknown input as other", () => {
    expect(sqlKind("-- 巡检\nselect 1")).toBe("read");
    expect(sqlKind("/* note */ select 1")).toBe("read");
    expect(sqlKind("")).toBe("other");
    expect(sqlKind("call p_demo()")).toBe("other");
  });
});

describe("formatCount", () => {
  it("groups thousands for scanability", () => {
    expect(formatCount(0)).toBe("0");
    expect(formatCount(999)).toBe("999");
    expect(formatCount(1234)).toBe("1,234");
    expect(formatCount(1234567)).toBe("1,234,567");
  });
});

describe("logRowRange", () => {
  it("returns 0/0 for an empty result set", () => {
    expect(logRowRange(1, 20, 0)).toEqual({ from: 0, to: 0 });
  });

  it("maps the page window onto record numbers", () => {
    expect(logRowRange(1, 20, 553)).toEqual({ from: 1, to: 20 });
    expect(logRowRange(2, 20, 553)).toEqual({ from: 21, to: 40 });
    expect(logRowRange(3, 30, 553)).toEqual({ from: 61, to: 90 });
  });

  it("stops at the last record on a partial page", () => {
    expect(logRowRange(28, 20, 553)).toEqual({ from: 541, to: 553 });
  });

  it("clamps out-of-range pages", () => {
    expect(logRowRange(99, 20, 45)).toEqual({ from: 41, to: 45 });
    expect(logRowRange(0, 20, 45)).toEqual({ from: 1, to: 20 });
  });
});
