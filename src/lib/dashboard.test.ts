import { describe, expect, it } from "vitest";
import {
  envTypeMatrix,
  formatMetric,
  instanceTypeCounts,
  parseRangeLabel,
  percent,
} from "./dashboard";
import type { InstanceItem } from "../types";

function instance(id: number, dbType: string, type = "master"): InstanceItem {
  return { id, name: `ins-${id}`, type, dbType };
}

describe("instanceTypeCounts", () => {
  it("aggregates by dbType, sorting by count desc then name asc", () => {
    const counts = instanceTypeCounts([
      instance(1, "mysql"),
      instance(2, "mysql"),
      instance(3, "doris", "slave"),
      instance(4, "redis"),
      instance(5, "doris"),
    ]);
    expect(counts).toEqual([
      { dbType: "doris", count: 2 },
      { dbType: "mysql", count: 2 },
      { dbType: "redis", count: 1 },
    ]);
  });

  it("normalizes unknown or empty dbType", () => {
    expect(instanceTypeCounts([instance(1, ""), instance(2, "  ")])).toEqual([
      { dbType: "unknown", count: 2 },
    ]);
  });

  it("returns an empty list for no instances", () => {
    expect(instanceTypeCounts([])).toEqual([]);
  });
});

describe("envTypeMatrix", () => {
  it("builds a stable env x dbType matrix", () => {
    const matrix = envTypeMatrix([
      instance(1, "mysql", "master"),
      instance(2, "mysql", "master"),
      instance(3, "doris", "slave"),
      instance(4, "doris", "master"),
    ]);
    expect(matrix.envs).toEqual(["master", "slave"]);
    expect(matrix.dbTypes).toEqual(["doris", "mysql"]);
    expect(matrix.series).toEqual([
      { dbType: "doris", values: [1, 1] },
      { dbType: "mysql", values: [2, 0] },
    ]);
  });

  it("falls back to unknown for missing env", () => {
    const matrix = envTypeMatrix([instance(1, "mysql", "")]);
    expect(matrix.envs).toEqual(["unknown"]);
    expect(matrix.series).toEqual([{ dbType: "mysql", values: [1] }]);
  });

  it("returns empty structures for no instances", () => {
    expect(envTypeMatrix([])).toEqual({ envs: [], dbTypes: [], series: [] });
  });
});

describe("percent", () => {
  it("rounds to a whole percentage", () => {
    expect(percent(1, 3)).toBe(33);
    expect(percent(2, 2)).toBe(100);
  });

  it("guards a zero total", () => {
    expect(percent(0, 0)).toBe(0);
    expect(percent(5, 0)).toBe(0);
  });
});

describe("parseRangeLabel", () => {
  it("reads the range Archery echoes back on the dashboard", () => {
    expect(parseRangeLabel("2026-10-29 - 2026-11-05")).toEqual({
      start: "2026-10-29",
      end: "2026-11-05",
    });
  });

  it("tolerates surrounding spaces and a tilde separator", () => {
    expect(parseRangeLabel("  2026-10-29 ~ 2026-11-05 ")).toEqual({
      start: "2026-10-29",
      end: "2026-11-05",
    });
  });

  it("returns null for anything that is not a date range", () => {
    expect(parseRangeLabel("")).toBeNull();
    expect(parseRangeLabel("最近7日")).toBeNull();
    expect(parseRangeLabel("2026-10-29")).toBeNull();
  });
});

describe("formatMetric", () => {
  it("renders an em dash for missing values", () => {
    expect(formatMetric(null)).toBe("—");
    expect(formatMetric(undefined)).toBe("—");
  });

  it("keeps real numbers verbatim", () => {
    expect(formatMetric(0)).toBe("0");
    expect(formatMetric(2200)).toBe("2200");
  });
});


