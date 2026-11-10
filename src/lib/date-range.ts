/**
 * 工单列表的日期区间：与 Archery 网页的 daterangepicker 预设保持一致。
 * 所有日期都按本地时区格式化成 YYYY-MM-DD（Archery 的 start_date / end_date 也是这个格式）。
 */
export type DateRangePreset =
  | "all"
  | "today"
  | "yesterday"
  | "last7"
  | "last30"
  | "last365"
  | "thisMonth"
  | "lastMonth"
  | "thisYear"
  | "lastYear"
  | "custom";

export interface DateRange {
  start: string;
  end: string;
}

const BUILT_IN: DateRangePreset[] = [
  "all",
  "today",
  "yesterday",
  "last7",
  "last30",
  "last365",
  "thisMonth",
  "lastMonth",
  "thisYear",
  "lastYear",
];

export function toISODate(date: Date): string {
  const month = `${date.getMonth() + 1}`.padStart(2, "0");
  const day = `${date.getDate()}`.padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
}

function shiftDays(base: Date, days: number): Date {
  const next = new Date(base.getFullYear(), base.getMonth(), base.getDate());
  next.setDate(next.getDate() + days);
  return next;
}

/**
 * 预设区间。最近 7/30 日含今天（与 Archery 一致：今天 2026-11-02 的「最近30日」
 * 是 2026-10-04 ~ 2026-11-02）。
 */
export function presetRange(
  preset: DateRangePreset,
  now: Date = new Date(),
  custom: DateRange = { start: "", end: "" },
): DateRange {
  switch (preset) {
    case "all":
      return { start: "", end: "" };
    case "today": {
      const today = toISODate(now);
      return { start: today, end: today };
    }
    case "yesterday": {
      const day = toISODate(shiftDays(now, -1));
      return { start: day, end: day };
    }
    case "last7":
      return { start: toISODate(shiftDays(now, -6)), end: toISODate(now) };
    case "last30":
      return { start: toISODate(shiftDays(now, -29)), end: toISODate(now) };
    case "last365":
      return { start: toISODate(shiftDays(now, -364)), end: toISODate(now) };
    case "thisYear":
      return {
        start: toISODate(new Date(now.getFullYear(), 0, 1)),
        end: toISODate(now),
      };
    case "lastYear":
      return {
        start: toISODate(new Date(now.getFullYear() - 1, 0, 1)),
        end: toISODate(new Date(now.getFullYear() - 1, 11, 31)),
      };
    case "thisMonth":
      return {
        start: toISODate(new Date(now.getFullYear(), now.getMonth(), 1)),
        end: toISODate(now),
      };
    case "lastMonth":
      return {
        start: toISODate(new Date(now.getFullYear(), now.getMonth() - 1, 1)),
        end: toISODate(new Date(now.getFullYear(), now.getMonth(), 0)),
      };
    default:
      return { start: custom.start, end: custom.end };
  }
}

/** 滚动区间（最近 N 日）才需要右端偏移，日历区间不受影响。 */
const ROLLING_PRESETS = new Set<DateRangePreset>(["last7", "last30", "last365"]);

/**
 * 带右端偏移的预设区间。Archery 首页的「最近 7 日」实为 [D-6, D+1]（含明天，共 8 天），
 * 传 endOffsetDays=1 即复刻该口径；列表页不传，保持 [D-6, D]。
 */
export function presetRangeWithOffset(
  preset: DateRangePreset,
  endOffsetDays = 0,
  now: Date = new Date(),
): DateRange {
  const range = presetRange(preset, now);
  if (endOffsetDays === 0 || !range.end || !ROLLING_PRESETS.has(preset)) return range;
  // 按本地年/月/日重建：直接 new Date("YYYY-MM-DD") 会被当成 UTC，负时区会差一天。
  const [year, month, day] = range.end.split("-").map(Number);
  const end = shiftDays(new Date(year, month - 1, day), endOffsetDays);
  return { start: range.start, end: toISODate(end) };
}

/** 反查当前区间属于哪个预设；两端都为空算「全部」，其余不匹配算「自定义」。 */
export function detectPreset(
  start: string,
  end: string,
  now: Date = new Date(),
): DateRangePreset {
  const from = start.trim();
  const to = end.trim();
  if (!from && !to) return "all";
  for (const preset of BUILT_IN) {
    const range = presetRange(preset, now);
    if (range.start === from && range.end === to) return preset;
  }
  return "custom";
}

/** 触发器文案：两端都有显示 "起 - 止"，都没有显示「全部」。 */
export function formatRange(start: string, end: string, allText: string): string {
  const from = start.trim();
  const to = end.trim();
  if (!from && !to) return allText;
  return `${from || "…"} - ${to || "…"}`;
}
