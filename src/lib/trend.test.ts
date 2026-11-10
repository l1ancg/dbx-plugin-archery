import { describe, expect, it } from "vitest";
import {
  average,
  buildAxis,
  buildDateTicks,
  columnX,
  DATE_LABEL_CHAR,
  dateSpanDays,
  formatAverage,
  labelAnchor,
  labelOffset,
  nearestColumn,
  seriesAxes,
  shouldLabelValue,
  shouldStackTrendPanels,
  smoothPath,
  toggleSeries,
  tooltipFlip,
  visibleSeries,
} from "./trend";
import type { DateTick, PlotBox, TrendSeries } from "./trend";

/** 与 TrendChart 的 viewBox 一致：640x220，左 46 右 14 上 18 下 30。 */
const PLOT: PlotBox = { left: 46, top: 18, width: 580, height: 172 };
/** 折线图右侧要给纵轴刻度留位置，与 TrendChart 的 LINE_LAYOUT 一致。 */
const LINE_PLOT: PlotBox = { left: 46, top: 18, width: 548, height: 172 };

/** Archery 首页 2026-10-29 ~ 2026-11-05 的真实数据。 */
const ROWS = [1870, 2435, 167, 0, 1943, 3246, 1066, 0];
const COUNTS = [37, 95, 10, 0, 78, 230, 52, 0];

const SERIES: TrendSeries[] = [
  { name: "检索行数", values: ROWS },
  { name: "检索次数", values: COUNTS },
];

/** 从路径串里取出所有 x 或 y 坐标，用于断言平滑曲线不越界。 */
function pathNumbers(path: string, axis: "x" | "y"): number[] {
  const values: number[] = [];
  const pattern = /(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)/g;
  let match: RegExpExecArray | null = pattern.exec(path);
  while (match) {
    values.push(Number(match[axis === "x" ? 1 : 2]));
    match = pattern.exec(path);
  }
  return values;
}

describe("buildAxis", () => {
  it("picks a tick count that fills the height for the row series", () => {
    const axis = buildAxis(ROWS);
    expect(axis.max).toBe(3500);
    expect(axis.ticks).toEqual([0, 500, 1000, 1500, 2000, 2500, 3000, 3500]);
  });

  it("keeps the small series readable on its own axis", () => {
    // 单轴时 230 只有 3246 的 7%，双轴后应铺满 0~250
    const axis = buildAxis(COUNTS);
    expect(axis.max).toBe(250);
    expect(axis.ticks).toEqual([0, 50, 100, 150, 200, 250]);
  });

  it("falls back to 0..1 when every value is zero", () => {
    expect(buildAxis([0, 0, 0]).ticks).toEqual([0, 1]);
  });
});

describe("seriesAxes", () => {
  it("puts the first visible series on the left axis and the second on the right", () => {
    const axes = seriesAxes(SERIES);
    expect(axes.map((axis) => axis.side)).toEqual(["left", "right"]);
    expect(axes.map((axis) => axis.max)).toEqual([3500, 250]);
  });

  it("keeps a lone survivor on the left axis instead of an empty right axis", () => {
    expect(seriesAxes([SERIES[1]]).map((axis) => [axis.side, axis.max])).toEqual([["left", 250]]);
  });
});

describe("visibleSeries", () => {
  it("drops the unchecked series and keeps declaration order", () => {
    expect(visibleSeries(SERIES, []).map((item) => item.name)).toEqual(["检索行数", "检索次数"]);
    expect(visibleSeries(SERIES, ["检索次数"]).map((item) => item.name)).toEqual(["检索行数"]);
  });

  it("ignores names that are not in the series list", () => {
    expect(visibleSeries(SERIES, ["上线数量"]).map((item) => item.name)).toEqual([
      "检索行数",
      "检索次数",
    ]);
  });
});

describe("toggleSeries", () => {
  it("hides on first click and restores on second", () => {
    const hidden = toggleSeries([], "检索次数");
    expect(hidden).toEqual(["检索次数"]);
    expect(toggleSeries(hidden, "检索次数")).toEqual([]);
  });

  it("refuses to hide the last visible series", () => {
    // 两条都点掉会得到一张空图，图例退化为无法恢复
    expect(toggleSeries(["检索行数"], "检索次数")).toEqual(["检索行数"]);
  });
});

describe("smoothPath", () => {
  it("emits one cubic segment per gap", () => {
    const path = smoothPath(
      [
        { x: 0, y: 10 },
        { x: 10, y: 0 },
        { x: 20, y: 10 },
      ],
      { top: -20, bottom: 20 },
    );
    expect(path.startsWith("M")).toBe(true);
    expect(path.match(/C/g)).toHaveLength(2);
  });

  it("keeps every control point inside the plot band", () => {
    // 尖峰两侧陡降时 Catmull-Rom 会过冲到 100 以上，必须钳制
    const path = smoothPath(
      [
        { x: 0, y: 0 },
        { x: 10, y: 100 },
        { x: 20, y: 0 },
      ],
      { top: 0, bottom: 100 },
    );
    const ys = pathNumbers(path, "y");
    expect(ys.length).toBeGreaterThan(0);
    expect(Math.min(...ys)).toBeGreaterThanOrEqual(0);
    expect(Math.max(...ys)).toBeLessThanOrEqual(100);
  });

  it("collapses a single point to a move command", () => {
    expect(smoothPath([{ x: 5, y: 5 }], { top: 0, bottom: 10 })).toBe("M5.0 5.0");
  });
});

describe("average", () => {
  it("counts zero days in the denominator", () => {
    expect(average(COUNTS)).toBe(62.75);
    expect(average(ROWS)).toBe(1340.875);
  });
});

describe("formatAverage", () => {
  it("trims trailing zeros and caps at two decimals", () => {
    expect([formatAverage(230), formatAverage(62.75), formatAverage(58.751)]).toEqual([
      "230",
      "62.75",
      "58.75",
    ]);
  });
});

describe("labelAnchor", () => {
  it("anchors edge labels inside the plot so they are not clipped", () => {
    expect([
      labelAnchor(PLOT.left, PLOT),
      labelAnchor(PLOT.left + PLOT.width / 2, PLOT),
      labelAnchor(PLOT.left + PLOT.width, PLOT),
    ]).toEqual(["start", "middle", "end"]);
  });
});

describe("labelOffset", () => {
  it("drops the label below a point that sits near the top edge", () => {
    // 峰值点 y=19 时标签若仍向上偏移会被裁掉
    expect(labelOffset(19, PLOT)).toBe(29);
    expect(labelOffset(100, PLOT)).toBe(93);
  });
});

describe("shouldLabelValue", () => {
  it("skips zeros but labels every positive value", () => {
    expect([shouldLabelValue(0), shouldLabelValue(1), shouldLabelValue(3246)]).toEqual([
      false,
      true,
      true,
    ]);
  });
});

describe("tooltipFlip", () => {
  it("flips the tooltip to the left near the right edge", () => {
    expect([
      tooltipFlip(PLOT.left + PLOT.width, PLOT, 120),
      tooltipFlip(PLOT.left + 100, PLOT, 120),
    ]).toEqual([true, false]);
  });
});

describe("columnX", () => {
  it("sticks line points to both edges", () => {
    expect([columnX(0, 8, PLOT, "line"), columnX(7, 8, PLOT, "line")]).toEqual([46, 626]);
  });

  it("centers bars in their own cell", () => {
    expect(columnX(0, 8, PLOT, "bar")).toBeCloseTo(82.25, 5);
  });

  it("centers a single point instead of dividing by zero", () => {
    expect(columnX(3, 1, PLOT, "line")).toBe(336);
  });
});

/** 生成连续日期，用来覆盖 7 天 / 30 天 / 一年等真实区间。 */
function datesFrom(start: string, days: number): string[] {
  const base = new Date(`${start}T00:00:00`);
  return Array.from({ length: days }, (_, offset) => {
    const day = new Date(base);
    day.setDate(base.getDate() + offset);
    return [
      day.getFullYear(),
      String(day.getMonth() + 1).padStart(2, "0"),
      String(day.getDate()).padStart(2, "0"),
    ].join("-");
  });
}

/** 按锚点算出标签占位，相邻两个相交即视为挤在一起。 */
function tickBounds(tick: DateTick): { left: number; right: number } {
  const width = tick.label.length * DATE_LABEL_CHAR;
  if (tick.anchor === "start") return { left: tick.x, right: tick.x + width };
  if (tick.anchor === "end") return { left: tick.x - width, right: tick.x };
  return { left: tick.x - width / 2, right: tick.x + width / 2 };
}

function expectNoOverlap(ticks: DateTick[]): void {
  const sorted = [...ticks].sort((left, right) => left.x - right.x);
  for (let index = 1; index < sorted.length; index += 1) {
    expect(tickBounds(sorted[index]).left).toBeGreaterThanOrEqual(tickBounds(sorted[index - 1]).right);
  }
}

describe("buildDateTicks", () => {
  it("labels every day when a week fits the axis", () => {
    const ticks = buildDateTicks(datesFrom("2026-10-30", 8), LINE_PLOT, "line");
    expect(ticks.map((tick) => tick.label)).toEqual([
      "10-30",
      "10-31",
      "11-01",
      "11-02",
      "11-03",
      "11-04",
      "11-05",
      "11-06",
    ]);
    // 折线首尾贴边，标签朝内对齐才不会左右被裁
    expect([ticks[0].x, ticks[ticks.length - 1].x]).toEqual([46, 594]);
    expect([ticks[0].anchor, ticks[ticks.length - 1].anchor]).toEqual(["start", "end"]);
  });

  it("thins a month-long window instead of stacking 30 labels", () => {
    const ticks = buildDateTicks(datesFrom("2026-10-08", 30), LINE_PLOT, "line");
    expect(ticks.length).toBeLessThanOrEqual(8);
    expect(ticks.length).toBeGreaterThanOrEqual(4);
    expect([ticks[0].label, ticks[ticks.length - 1].label]).toEqual(["10-08", "11-06"]);
    expectNoOverlap(ticks);
  });

  it("switches to month labels when the window crosses a year", () => {
    // 2025-11-06 ~ 2026-11-06：沿用 MM-DD 会出现两个无法区分的 11-06
    const ticks = buildDateTicks(datesFrom("2025-11-06", 366), LINE_PLOT, "line");
    expect(ticks.length).toBeLessThanOrEqual(8);
    expect(ticks.length).toBeGreaterThanOrEqual(4);
    ticks.forEach((tick) => expect(tick.label).toMatch(/^\d{4}-\d{2}$/));
    expect(new Set(ticks.map((tick) => tick.label)).size).toBe(ticks.length);
    expectNoOverlap(ticks);
  });

  it("keeps mm-dd labels for a year inside one calendar year", () => {
    const ticks = buildDateTicks(datesFrom("2026-01-01", 300), LINE_PLOT, "line");
    expect(ticks.length).toBeLessThanOrEqual(8);
    ticks.forEach((tick) => expect(tick.label).toMatch(/^\d{2}-\d{2}$/));
    expectNoOverlap(ticks);
  });

  it("always shows the last day without crowding the label before it", () => {
    // 20 天窗口按 2 天抽稀时，末点 01-20 紧跟 01-19 会挤成一个字块
    const ticks = buildDateTicks(datesFrom("2026-01-01", 20), LINE_PLOT, "line");
    expect(ticks[ticks.length - 1].label).toBe("01-20");
    expectNoOverlap(ticks);
  });

  it("drops labels as the card narrows", () => {
    const narrow: PlotBox = { ...LINE_PLOT, width: 200 };
    const ticks = buildDateTicks(datesFrom("2026-10-01", 30), narrow, "line");
    expect(ticks.length).toBeLessThanOrEqual(6);
    expect(ticks.length).toBeGreaterThanOrEqual(2);
    expectNoOverlap(ticks);
  });

  it("centers a single day instead of pinning it to an edge", () => {
    expect(buildDateTicks(["2026-11-05"], LINE_PLOT, "line")).toEqual([
      { index: 0, x: 320, label: "11-05", anchor: "middle" },
    ]);
  });

  it("centers bars in their own cell", () => {
    const ticks = buildDateTicks(datesFrom("2026-10-30", 8), PLOT, "bar");
    expect(ticks[0].x).toBeCloseTo(82.25, 5);
  });
});

describe("dateSpanDays", () => {
  it("measures the window from the first to the last day", () => {
    expect(dateSpanDays(datesFrom("2026-10-30", 8))).toBe(7);
    expect(dateSpanDays(datesFrom("2026-10-06", 31))).toBe(30);
    expect(dateSpanDays(datesFrom("2025-11-06", 366))).toBe(365);
  });

  it("returns zero when there is no range to measure", () => {
    expect([dateSpanDays([]), dateSpanDays(["2026-11-05"])]).toEqual([0, 0]);
  });
});

describe("shouldStackTrendPanels", () => {
  it("keeps the two charts side by side up to a 30-day window", () => {
    expect(shouldStackTrendPanels(datesFrom("2026-10-08", 30))).toBe(false);
    // 「最近30日」实际是 31 个点（左端 30 天前、右端到明天），跨度仍是 30 天
    expect(shouldStackTrendPanels(datesFrom("2026-10-06", 31))).toBe(false);
  });

  it("stacks them once the window is longer than 30 days", () => {
    expect(shouldStackTrendPanels(datesFrom("2026-10-05", 32))).toBe(true);
    expect(shouldStackTrendPanels(datesFrom("2025-11-06", 366))).toBe(true);
  });

  it("does not stack a window without a range", () => {
    expect(shouldStackTrendPanels([])).toBe(false);
    expect(shouldStackTrendPanels(["2026-11-05"])).toBe(false);
  });
});

describe("nearestColumn", () => {
  it("snaps to the closest line point", () => {
    expect([
      nearestColumn(46, 8, PLOT, "line"),
      nearestColumn(120, 8, PLOT, "line"),
      nearestColumn(626, 8, PLOT, "line"),
    ]).toEqual([0, 1, 7]);
  });

  it("snaps to the bar cell that contains the pointer", () => {
    // 8 根柱、每格 72.5：第 2 格覆盖 [118.5, 191)，格心 154.75
    expect([
      nearestColumn(82, 8, PLOT, "bar"),
      nearestColumn(155, 8, PLOT, "bar"),
      nearestColumn(620, 8, PLOT, "bar"),
    ]).toEqual([0, 1, 7]);
  });

  it("ignores pointers outside the plot area", () => {
    expect([
      nearestColumn(45, 8, PLOT, "line"),
      nearestColumn(627, 8, PLOT, "line"),
      nearestColumn(100, 0, PLOT, "bar"),
    ]).toEqual([null, null, null]);
  });
});
