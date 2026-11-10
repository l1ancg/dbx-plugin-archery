/**
 * 「SQL 查询统计」双轴折线的纯计算层，从 TrendChart.vue 抽出来便于单测。
 * 视图层只负责把这里的坐标与路径画成 SVG。
 */

/** 一条按日折线：name 作图例名，values 与 dates 一一对应。 */
export interface TrendSeries {
  name: string;
  values: number[];
}

/** 一根纵轴的刻度：max 为轴上限，ticks 自 0 到 max 等距。 */
export interface TrendScale {
  max: number;
  ticks: number[];
}

/** 轴与所在侧：首个可见系列在左轴，第二个在右轴。 */
export interface TrendAxis extends TrendScale {
  side: "left" | "right";
}

export interface PlotBox {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface PlotPoint {
  x: number;
  y: number;
}

export interface PlotBand {
  top: number;
  bottom: number;
}

export type ChartMode = "line" | "bar";

/** 一根日期刻度：index 对应 dates 下标，x 为绘图坐标。 */
export interface DateTick {
  index: number;
  x: number;
  label: string;
  anchor: "start" | "middle" | "end";
}

/** 数值标签与数据点的间距。 */
const LABEL_GAP = 7;
/** 贴顶时标签翻到点下方的距离。 */
const LABEL_FLIP = 10;
/** 标签向内收边的阈值。 */
const LABEL_EDGE = 18;
/** 候选刻度档数：档数越少刻度越疏，按填充率挑最贴合数据的那档。 */
const SPLIT_CANDIDATES = [4, 5, 6, 7];
/** 图表最多两条折线，用于「至少保留一条可见」的判定。 */
const MAX_SERIES = 2;

function niceStep(value: number): number {
  if (!Number.isFinite(value) || value <= 1) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const normalized = value / magnitude;
  const nice = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return nice * magnitude;
}

function ticksFor(max: number, step: number): number[] {
  const count = Math.max(1, Math.round(max / step));
  return Array.from({ length: count + 1 }, (_, index) => Math.round(step * index * 1e6) / 1e6);
}

/**
 * 按数据自身量级挑刻度：候选 4~7 档，取「曲线占满高度」的那一档。
 * 行数 3246 → 0~3500（step 500，填充 93%）；次数 230 → 0~250（step 50，填充 92%）。
 */
export function buildAxis(values: number[]): TrendScale {
  const peak = values.reduce((max, value) => {
    if (!Number.isFinite(value)) return max;
    return value > max ? value : max;
  }, 0);
  if (peak <= 0) return { max: 1, ticks: [0, 1] };

  let best: TrendScale = { max: 1, ticks: [0, 1] };
  let bestFill = -1;
  for (const split of SPLIT_CANDIDATES) {
    const step = niceStep(peak / split);
    const max = Math.ceil(peak / step) * step;
    const ticks = ticksFor(max, step);
    const fill = peak / max;
    // 填充率相同时选刻度更密的那档，否则 822 这种量级只出 0/500/1,000 三根线
    const denser = Math.abs(fill - bestFill) <= 1e-9 && ticks.length > best.ticks.length;
    if (fill > bestFill + 1e-9 || denser) {
      bestFill = fill;
      best = { max, ticks };
    }
  }
  return best;
}

/** 每个可见系列一根独立纵轴，量级差 10 倍以上也能各自铺开。 */
export function seriesAxes(series: TrendSeries[]): TrendAxis[] {
  return series.map((item, index) => ({
    ...buildAxis(item.values),
    side: index === 0 ? "left" : "right",
  }));
}

/** 图例点选后剩下要画的系列，保持声明顺序。 */
export function visibleSeries(series: TrendSeries[], hidden: string[]): TrendSeries[] {
  return series.filter((item) => !hidden.includes(item.name));
}

/** 切换某系列的显隐；至少保留一条可见，否则图例会退化成无法恢复。 */
export function toggleSeries(hidden: string[], name: string, total = MAX_SERIES): string[] {
  if (hidden.includes(name)) {
    return hidden.filter((item) => item !== name);
  }
  if (hidden.length >= Math.max(1, total - 1)) {
    return hidden;
  }
  return [...hidden, name];
}

function clampTo(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function coordinate(value: number): string {
  return value.toFixed(1);
}

/**
 * Catmull-Rom 转三次贝塞尔平滑路径。
 * 控制点钳制在绘图带内——贝塞尔曲线落在控制点凸包内，因此不会过冲出边界。
 */
export function smoothPath(points: PlotPoint[], band: PlotBand): string {
  if (points.length === 0) return "";
  const top = Math.min(band.top, band.bottom);
  const bottom = Math.max(band.top, band.bottom);
  const clampY = (value: number) => clampTo(value, top, bottom);

  const first = points[0];
  let path = `M${coordinate(first.x)} ${coordinate(clampY(first.y))}`;
  if (points.length === 1) return path;

  const minX = points[0].x;
  const maxX = points[points.length - 1].x;
  const clampX = (value: number) => clampTo(value, minX, maxX);

  for (let index = 0; index < points.length - 1; index += 1) {
    const before = points[index - 1] ?? points[index];
    const start = points[index];
    const end = points[index + 1];
    const after = points[index + 2] ?? end;
    const control1X = clampX(start.x + (end.x - before.x) / 6);
    const control1Y = clampY(start.y + (end.y - before.y) / 6);
    const control2X = clampX(end.x - (after.x - start.x) / 6);
    const control2Y = clampY(end.y - (after.y - start.y) / 6);
    path +=
      ` C${coordinate(control1X)} ${coordinate(control1Y)}` +
      ` ${coordinate(control2X)} ${coordinate(control2Y)}` +
      ` ${coordinate(end.x)} ${coordinate(clampY(end.y))}`;
  }
  return path;
}

/** 算术平均，0 值日也计入分母。 */
export function average(values: number[]): number {
  if (values.length === 0) return 0;
  const sum = values.reduce((total, value) => total + (Number.isFinite(value) ? value : 0), 0);
  return sum / values.length;
}

/** 均值参考线文案：整数不带小数点，其余保留两位。 */
export function formatAverage(value: number): string {
  if (!Number.isFinite(value)) return "0";
  const rounded = Math.round(value * 100) / 100;
  return String(rounded);
}

/** 数值标签的锚点：贴边时朝内，防裁切。 */
export function labelAnchor(x: number, plot: PlotBox): "start" | "middle" | "end" {
  if (x < plot.left + LABEL_EDGE) return "start";
  if (x > plot.left + plot.width - LABEL_EDGE) return "end";
  return "middle";
}

/** 数值标签的 y：顶部的点改标在点下方，不越出绘图区。 */
export function labelOffset(y: number, plot: PlotBox): number {
  if (y - LABEL_GAP < plot.top) return y + LABEL_FLIP;
  return y - LABEL_GAP;
}

/** 0 值不标数字，否则轴线上会挂一串「0」。 */
export function shouldLabelValue(value: number): boolean {
  return Number.isFinite(value) && value > 0;
}

/** 悬停浮层是否翻到点的左侧：按整块浮层宽度预留，半宽会在卡片边缘被裁掉。 */
export function tooltipFlip(x: number, plot: PlotBox, boxWidth: number): boolean {
  return x + boxWidth > plot.left + plot.width;
}

/** 折线首尾贴边、柱状取格心，单点时居中。 */
export function columnX(index: number, count: number, plot: PlotBox, mode: ChartMode): number {
  if (mode === "bar") {
    return plot.left + (plot.width / Math.max(1, count)) * (index + 0.5);
  }
  if (count <= 1) return plot.left + plot.width / 2;
  if (index <= 0) return plot.left;
  if (index >= count - 1) return plot.left + plot.width;
  return plot.left + (plot.width / (count - 1)) * index;
}

/**
 * 指针位置反推最近的一列：折线取最近点、柱状取所在格心。
 * 落在绘图区外（纵轴刻度、卡片留白）返回 null，避免误触发第一根/最后一根。
 */
export function nearestColumn(
  x: number,
  count: number,
  plot: PlotBox,
  mode: ChartMode,
): number | null {
  if (count <= 0) return null;
  if (x < plot.left || x > plot.left + plot.width) return null;

  let nearest = 0;
  let distance = Number.POSITIVE_INFINITY;
  for (let index = 0; index < count; index += 1) {
    const gap = Math.abs(columnX(index, count, plot, mode) - x);
    if (gap < distance) {
      distance = gap;
      nearest = index;
    }
  }
  return nearest;
}

/** 跨度超过两个月且跨年时，MM-DD 会出现重复文案，改用自然月刻度。 */
const MONTH_MODE_MIN_DAYS = 62;
/** 一条轴最多排几个日期标签：再多就是「一堵字」，读不出趋势。 */
const MAX_DATE_TICKS = 8;
/** 10.5px 数字标签的估算字宽（viewBox 单位），实测 MM-DD 约 25 单位。 */
export const DATE_LABEL_CHAR = 5.2;
/** 相邻两个日期标签之间至少留出的空隙。 */
const DATE_LABEL_GAP = 12;

/**
 * 首尾日期相差的天数：2026-10-30 ~ 2026-11-06 为 7。
 * 不足两天或日期不合法时按点数兜底，避免布局判断被脏数据带偏。
 */
export function dateSpanDays(dates: string[]): number {
  if (dates.length < 2) return 0;
  const first = Date.parse(`${dates[0]}T00:00:00`);
  const last = Date.parse(`${dates[dates.length - 1]}T00:00:00`);
  if (!Number.isFinite(first) || !Number.isFinite(last)) return dates.length - 1;
  return Math.max(0, Math.round((last - first) / 86_400_000));
}

/** 区间含首尾的天数：供「跨两个月」这类按天数判定使用。 */
function spanDays(dates: string[]): number {
  if (dates.length < 2) return dates.length;
  return dateSpanDays(dates) + 1;
}

/** 统计区间超过 30 天时两张按日图改上下排列：半宽会挤掉日期刻度。 */
export function shouldStackTrendPanels(dates: string[]): boolean {
  return dateSpanDays(dates) > 30;
}

function crossesYear(dates: string[]): boolean {
  const first = dates[0] ?? "";
  const last = dates[dates.length - 1] ?? "";
  return first.length >= 10 && last.length >= 10 && first.slice(0, 4) !== last.slice(0, 4);
}

/** 短标签：MM-DD，日期结构不认识时原样返回。 */
function dayLabel(date: string): string {
  return date.length >= 10 ? date.slice(5) : date;
}

/** 长跨度标签：YYYY-MM，跨年时不会出现两个一模一样的「11-06」。 */
function monthLabel(date: string): string {
  return date.length >= 7 ? date.slice(0, 7) : date;
}

/** 按字宽算这一轴能排几个标签，上限 MAX_DATE_TICKS。 */
function tickCapacity(plot: PlotBox, labelChars: number): number {
  const perTick = labelChars * DATE_LABEL_CHAR + DATE_LABEL_GAP;
  const fit = Math.floor((Math.max(0, plot.width) + DATE_LABEL_GAP) / perTick) + 1;
  return Math.max(2, Math.min(MAX_DATE_TICKS, fit));
}

function anchorAt(position: number, total: number): DateTick["anchor"] {
  if (total <= 1) return "middle";
  if (position === 0) return "start";
  if (position === total - 1) return "end";
  return "middle";
}

/** 按天等距抽稀：首尾必留，尾部不足一个步长时用末点替换，避免落单贴在一起。 */
function dayTicks(dates: string[], plot: PlotBox, mode: ChartMode): DateTick[] {
  const count = dates.length;
  const capacity = tickCapacity(plot, dayLabel(dates[0]).length);
  const step = Math.max(1, Math.ceil((count - 1) / (capacity - 1)));
  const indices: number[] = [];
  for (let index = 0; index < count; index += step) indices.push(index);

  const last = count - 1;
  if (indices[indices.length - 1] !== last) {
    const tail = last - indices[indices.length - 1];
    if (indices.length >= capacity || tail < step) {
      indices[indices.length - 1] = last;
    } else {
      indices.push(last);
    }
  }

  return indices.map((index, position) => ({
    index,
    x: columnX(index, count, plot, mode),
    label: dayLabel(dates[index]),
    anchor: anchorAt(position, indices.length),
  }));
}

/** 按自然月等距抽稀：月首数量超过容量时每 N 个月取一个。 */
function monthTicks(dates: string[], plot: PlotBox, mode: ChartMode): DateTick[] {
  const starts: number[] = [];
  dates.forEach((date, index) => {
    if (date.length >= 10 && date.slice(8, 10) === "01") starts.push(index);
  });
  if (starts.length < 2) return [];

  const capacity = tickCapacity(plot, monthLabel(dates[starts[0]]).length);
  const step = Math.max(1, Math.ceil(starts.length / capacity));
  return starts
    .filter((_, position) => position % step === 0)
    .map((index) => ({
      index,
      x: columnX(index, dates.length, plot, mode),
      label: monthLabel(dates[index]),
      anchor: "middle" as const,
    }));
}

/**
 * x 轴日期刻度：跨度小按天抽稀、跨年长跨度按自然月，标签数随绘图区宽度收敛，
 * 首尾始终保留并朝内对齐，任何区间都不会把日期挤成一排。
 */
export function buildDateTicks(dates: string[], plot: PlotBox, mode: ChartMode): DateTick[] {
  if (dates.length === 0) return [];
  if (dates.length === 1) {
    return [
      { index: 0, x: columnX(0, 1, plot, mode), label: dayLabel(dates[0]), anchor: "middle" },
    ];
  }

  if (crossesYear(dates) && spanDays(dates) > MONTH_MODE_MIN_DAYS) {
    const months = monthTicks(dates, plot, mode);
    if (months.length >= 2) return months;
  }
  return dayTicks(dates, plot, mode);
}
