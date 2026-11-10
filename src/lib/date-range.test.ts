import { describe, expect, it } from "vitest";
import {
  detectPreset,
  formatRange,
  presetRange,
  presetRangeWithOffset,
  toISODate,
  type DateRangePreset,
} from "./date-range";

const NOW = new Date(2026, 10, 2, 17, 43, 6); // 2026-11-02 17:43:06 本地时间

describe("toISODate", () => {
  it("formats local dates with zero padding", () => {
    expect(toISODate(new Date(2026, 0, 5))).toBe("2026-01-05");
    expect(toISODate(new Date(2026, 11, 31))).toBe("2026-12-31");
  });
});

describe("presetRangeWithOffset", () => {
  // 2026-11-02（周一）
  it("pushes rolling ranges one day past today, matching the Archery dashboard window", () => {
    expect(presetRangeWithOffset("last7", 1, NOW)).toEqual({
      start: "2026-10-27",
      end: "2026-11-03",
    });
    expect(presetRangeWithOffset("last30", 1, NOW)).toEqual({
      start: "2026-10-04",
      end: "2026-11-03",
    });
    expect(presetRangeWithOffset("last365", 1, NOW).end).toBe("2026-11-03");
  });

  it("crosses a month boundary without drifting", () => {
    const endOfMonth = new Date(2026, 10, 30, 9, 0, 0); // 2026-11-30
    expect(presetRangeWithOffset("last7", 1, endOfMonth)).toEqual({
      start: "2026-11-24",
      end: "2026-12-01",
    });
  });

  it("leaves calendar ranges untouched", () => {
    expect(presetRangeWithOffset("thisMonth", 1, NOW)).toEqual({
      start: "2026-11-01",
      end: "2026-11-02",
    });
    expect(presetRangeWithOffset("lastMonth", 1, NOW)).toEqual({
      start: "2026-10-01",
      end: "2026-10-31",
    });
  });

  it("matches the plain preset when no offset is requested", () => {
    expect(presetRangeWithOffset("last7", 0, NOW)).toEqual(presetRange("last7", NOW));
  });
});

describe("presetRange", () => {
  it("clears both ends for 全部", () => {
    expect(presetRange("all", NOW)).toEqual({ start: "", end: "" });
  });

  it("handles 今日 and 昨日", () => {
    expect(presetRange("today", NOW)).toEqual({ start: "2026-11-02", end: "2026-11-02" });
    expect(presetRange("yesterday", NOW)).toEqual({
      start: "2026-11-01",
      end: "2026-11-01",
    });
  });

  it("counts 最近 7 日 and 最近 30 日 inclusive of today", () => {
    expect(presetRange("last7", NOW)).toEqual({ start: "2026-10-27", end: "2026-11-02" });
    expect(presetRange("last30", NOW)).toEqual({ start: "2026-10-04", end: "2026-11-02" });
  });

  it("covers 本月 and 上月 across a year boundary", () => {
    expect(presetRange("thisMonth", NOW)).toEqual({
      start: "2026-11-01",
      end: "2026-11-02",
    });
    const january = new Date(2026, 0, 15);
    expect(presetRange("lastMonth", january)).toEqual({
      start: "2025-12-01",
      end: "2025-12-31",
    });
    const march = new Date(2026, 2, 31);
    expect(presetRange("lastMonth", march)).toEqual({
      start: "2026-02-01",
      end: "2026-02-28",
    });
  });

  it("returns the given custom values untouched", () => {
    expect(presetRange("custom", NOW, { start: "2026-01-02", end: "2026-01-09" })).toEqual({
      start: "2026-01-02",
      end: "2026-01-09",
    });
  });
});

describe("detectPreset", () => {
  const cases: Array<[DateRangePreset, string, string]> = [
    ["all", "", ""],
    ["today", "2026-11-02", "2026-11-02"],
    ["yesterday", "2026-11-01", "2026-11-01"],
    ["last7", "2026-10-27", "2026-11-02"],
    ["last30", "2026-10-04", "2026-11-02"],
    ["thisMonth", "2026-11-01", "2026-11-02"],
  ];

  it("recognises every built-in preset from its dates", () => {
    for (const [preset, start, end] of cases) {
      expect(detectPreset(start, end, NOW)).toBe(preset);
    }
  });

  it("falls back to 自定义 for other ranges", () => {
    expect(detectPreset("2026-10-01", "2026-10-03", NOW)).toBe("custom");
    // 单边日期不参与预设匹配
    expect(detectPreset("2026-10-04", "", NOW)).toBe("custom");
  });
});

describe("formatRange", () => {
  it("joins both ends with a dash", () => {
    expect(formatRange("2026-10-04", "2026-11-02", "全部")).toBe("2026-10-04 - 2026-11-02");
  });

  it("shows the placeholder when the range is empty", () => {
    expect(formatRange("", "", "全部")).toBe("全部");
    expect(formatRange("  ", " ", "全部")).toBe("全部");
  });

  it("keeps a single side readable", () => {
    expect(formatRange("2026-10-04", "", "全部")).toBe("2026-10-04 - …");
  });
});
