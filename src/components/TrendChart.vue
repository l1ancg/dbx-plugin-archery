<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { t } from "../i18n";
import {
  average as averageOf,
  buildAxis,
  buildDateTicks,
  columnX,
  formatAverage,
  labelAnchor,
  labelOffset,
  nearestColumn,
  seriesAxes,
  shouldLabelValue,
  smoothPath,
  toggleSeries,
  tooltipFlip,
  visibleSeries,
} from "../lib/trend";
import type { PlotBox, TrendScale, TrendSeries } from "../lib/trend";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  title: string;
  icon: string;
  mode: "line" | "bar";
  dates: string[];
  series: Array<{ name: string; values: number[] }>;
  /** Archery 首页的统计区间，例如 2026-10-29 - 2026-11-05。 */
  subtitle?: string;
}>();

const PALETTE = ["#4f8ed6", "#4aab6e", "#d8a43c"];

/** 折线图右轴要留出刻度文字的位置，柱状图只有左轴。 */
const LINE_LAYOUT = { width: 640, height: 220, left: 46, right: 46, top: 18, bottom: 30 };
const BAR_LAYOUT = { width: 640, height: 220, left: 46, right: 14, top: 18, bottom: 30 };
const TOOLTIP_WIDTH = 150;
/** 上下排列时卡片铺满一行，按宽度适度加高，避免画成 5:1 的扁图。 */
const MAX_CHART_HEIGHT = 320;

/** 卡片实际宽度（CSS px）：viewBox 按 1:1 建，字号和刻度才与容器同步。 */
const bodyRef = ref<HTMLElement | null>(null);
const measuredWidth = ref(0);

const layout = computed(() => {
  const base = props.mode === "line" ? LINE_LAYOUT : BAR_LAYOUT;
  // 首帧还没有测量值，先用基准宽度，挂载后按容器宽度重排
  const width = Math.max(320, Math.round(measuredWidth.value || base.width));
  const height =
    width > base.width
      ? Math.round(Math.min(MAX_CHART_HEIGHT, Math.max(base.height, width * 0.34)))
      : base.height;
  return { ...base, width, height };
});

// 观察 svg 自身拿不到有效的内容盒尺寸，改看外层 .trend-body 的内容宽度；
// 空态切到有数据时容器是新建的，所以用 watch 重新挂载观察目标。
const resizeObserver =
  typeof ResizeObserver === "undefined"
    ? null
    : new ResizeObserver((entries) => {
        const width = Math.round(entries[0]?.contentRect.width ?? 0);
        if (width > 0 && Math.abs(width - measuredWidth.value) >= 1) {
          measuredWidth.value = width;
        }
      });

watch(
  bodyRef,
  (node) => {
    resizeObserver?.disconnect();
    if (node) resizeObserver?.observe(node);
  },
  { immediate: true },
);

onBeforeUnmount(() => resizeObserver?.disconnect());

const plot = computed<PlotBox>(() => ({
  left: layout.value.left,
  top: layout.value.top,
  width: layout.value.width - layout.value.left - layout.value.right,
  height: layout.value.height - layout.value.top - layout.value.bottom,
}));

const isLine = computed(() => props.mode === "line");
const count = computed(() => props.dates.length);

/** 图例点掉的系列名，至少保留一条可见。 */
const hidden = ref<string[]>([]);
const hoverIndex = ref<number | null>(null);

const visible = computed<TrendSeries[]>(() =>
  isLine.value ? visibleSeries(props.series, hidden.value) : props.series,
);

/** 颜色跟原始系列走：隐藏首条后剩下那条不会串色。 */
function colorOf(name: string): string {
  const index = props.series.findIndex((item) => item.name === name);
  return PALETTE[(index >= 0 ? index : 0) % PALETTE.length];
}

/** 双轴：每个可见系列按自身量级独立刻度。 */
const axes = computed(() => (isLine.value ? seriesAxes(visible.value) : []));
const leftAxis = computed<TrendScale>(() => axes.value[0] ?? { max: 1, ticks: [0, 1] });
const rightAxis = computed<TrendScale | null>(() => axes.value[1] ?? null);

const barScale = computed(() => buildAxis(props.series[0]?.values ?? []));

/** 日期刻度按跨度与卡片宽度自适应：短区间按天、跨年长区间按自然月。 */
const xTicks = computed(() => buildDateTicks(props.dates, plot.value, props.mode));

/** 天数一多就不再逐点标数值，否则文字会糊成一片（14 天以内才标）。 */
const showValueLabels = computed(() => count.value <= 14);

interface RenderedPoint {
  x: number;
  y: number;
  value: number;
  anchor: "start" | "middle" | "end";
  labelY: number;
  labeled: boolean;
}

interface RenderedLine {
  name: string;
  color: string;
  path: string;
  points: RenderedPoint[];
  averageValue: number;
  averageY: number;
  averageLabel: string;
}

const lines = computed<RenderedLine[]>(() => {
  if (!isLine.value) return [];
  const band = { top: plot.value.top, bottom: plot.value.top + plot.value.height };
  return visible.value.map((item, index) => {
    const axis = axes.value[index] ?? leftAxis.value;
    const color = colorOf(item.name);
    const points = item.values.map((value, position) => {
      const x = columnX(position, count.value, plot.value, "line");
      const y = band.bottom - (value / axis.max) * plot.value.height;
      return {
        x,
        y,
        value,
        anchor: labelAnchor(x, plot.value),
        labelY: labelOffset(y, plot.value),
        labeled: shouldLabelValue(value),
      };
    });
    const averageValue = averageOf(item.values);
    return {
      name: item.name,
      color,
      path: smoothPath(points, band),
      points,
      averageValue,
      averageY: band.bottom - (averageValue / axis.max) * plot.value.height,
      averageLabel: formatAverage(averageValue),
    };
  });
});

const gridTicks = computed(() => {
  const axis = isLine.value ? leftAxis.value : barScale.value;
  if (axis.max <= 0) return [];
  return axis.ticks.map((value) => ({
    label: value.toLocaleString(),
    y: plot.value.top + plot.value.height - (value / axis.max) * plot.value.height,
  }));
});

/** 右轴只出刻度文字，网格线跟左轴走，避免两套横线交错。 */
const rightTicks = computed(() => {
  if (!rightAxis.value) return [];
  const axis = rightAxis.value;
  return axis.ticks.map((value) => ({
    label: value.toLocaleString(),
    y: plot.value.top + plot.value.height - (value / axis.max) * plot.value.height,
  }));
});

const barWidth = computed(() =>
  Math.max(1.5, Math.min(38, (plot.value.width / Math.max(1, count.value)) * 0.52)),
);

const bars = computed(() => {
  const values = props.series[0]?.values ?? [];
  return values.map((value, index) => {
    const height = (value / barScale.value.max) * plot.value.height;
    const x = columnX(index, count.value, plot.value, "bar");
    return {
      x: x - barWidth.value / 2,
      y: plot.value.top + plot.value.height - height,
      width: barWidth.value,
      height,
      value,
      labelX: x,
      labelY: plot.value.top + plot.value.height - height - 6,
    };
  });
});

/** 0 值柱子不标数字，避免轴线上一串「0」 */
const barLabels = computed(() => bars.value.filter((bar) => bar.value > 0));

interface HoveredPoint {
  name: string;
  color: string;
  value: number;
  y: number;
}

const hovered = computed<{
  x: number;
  date: string;
  items: HoveredPoint[];
  left: string;
  top: string;
  flip: boolean;
  below: boolean;
} | null>(() => {
  const index = hoverIndex.value;
  if (index === null || count.value === 0) return null;

  // 柱状图只 hover 当前柱：浮层贴柱顶，数值就是这根柱的高度
  if (!isLine.value) {
    const bar = bars.value[index];
    if (!bar) return null;
    const top = Math.max(plot.value.top, bar.y - 6);
    return {
      x: bar.labelX,
      date: props.dates[index] ?? "",
      items: [
        {
          name: props.series[0]?.name ?? "",
          color: PALETTE[0],
          value: bar.value,
          y: bar.y,
        },
      ],
      left: `${(bar.labelX / layout.value.width) * 100}%`,
      top: `${(top / layout.value.height) * 100}%`,
      flip: tooltipFlip(bar.labelX, plot.value, TOOLTIP_WIDTH),
      below: bar.y - plot.value.top < 56,
    };
  }

  const items = lines.value.map((line) => ({
    name: line.name,
    color: line.color,
    value: line.points[index]?.value ?? 0,
    y: line.points[index]?.y ?? plot.value.top + plot.value.height,
  }));
  const x = columnX(index, count.value, plot.value, "line");
  const highest = Math.min(...items.map((item) => item.y));
  return {
    x,
    date: props.dates[index] ?? "",
    items,
    left: `${(x / layout.value.width) * 100}%`,
    top: `${(Math.max(plot.value.top, highest - 6) / layout.value.height) * 100}%`,
    flip: tooltipFlip(x, plot.value, TOOLTIP_WIDTH),
    below: highest - plot.value.top < 56,
  };
});

/** 鼠标位置反推最近的一天：点数不多，直接线性扫，省掉反解公式。 */
function onPointerMove(event: PointerEvent): void {
  if (count.value === 0) return;
  const target = event.currentTarget as HTMLElement | null;
  if (!target) return;
  const rect = target.getBoundingClientRect();
  if (rect.width <= 0) return;
  const x = ((event.clientX - rect.left) / rect.width) * layout.value.width;
  hoverIndex.value = nearestColumn(x, count.value, plot.value, props.mode);
}

function onPointerLeave(): void {
  hoverIndex.value = null;
}

function onToggle(name: string): void {
  if (!isLine.value) return;
  hidden.value = toggleSeries(hidden.value, name, props.series.length);
  hoverIndex.value = null;
}
</script>

<template>
  <section class="card chart-card trend-card">
    <header class="card-head">
      <h2 class="card-title">
        <NavIcon :name="icon" :size="14" />
        <span>{{ title }}</span>
      </h2>
      <span v-if="subtitle" class="card-subtitle">{{ subtitle }}</span>
      <div v-if="mode === 'line' && series.length > 1" class="chart-legend">
        <button
          v-for="(item, index) in series"
          :key="item.name"
          type="button"
          class="legend-item legend-toggle"
          :class="{ 'is-off': hidden.includes(item.name) }"
          :aria-pressed="!hidden.includes(item.name)"
          @click="onToggle(item.name)"
        >
          <i class="legend-dot" :style="{ background: PALETTE[index % PALETTE.length] }"></i>
          <span>{{ item.name }}</span>
        </button>
      </div>
    </header>

    <div v-if="dates.length === 0" class="card-empty">
      <NavIcon name="chart" :size="20" />
      <span>{{ t("dashboard.trendEmpty") }}</span>
    </div>

    <div v-else ref="bodyRef" class="chart-body trend-body">
      <svg
        :viewBox="`0 0 ${layout.width} ${layout.height}`"
        class="chart-svg trend-svg"
        role="img"
        @pointermove="onPointerMove"
        @pointerleave="onPointerLeave"
      >
        <g class="chart-grid">
          <template v-for="tick in gridTicks" :key="tick.y">
            <line :x1="plot.left" :x2="plot.left + plot.width" :y1="tick.y" :y2="tick.y" />
            <text :x="plot.left - 8" :y="tick.y + 4" text-anchor="end">{{ tick.label }}</text>
          </template>
        </g>

        <template v-if="mode === 'line'">
          <g v-for="line in lines" :key="line.name">
            <path
              :d="line.path"
              fill="none"
              :stroke="line.color"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <template v-if="line.averageValue > 0">
              <line
                class="trend-average"
                :x1="plot.left"
                :x2="plot.left + plot.width"
                :y1="line.averageY"
                :y2="line.averageY"
                :stroke="line.color"
              />
              <text
                class="trend-average-label"
                :x="plot.left + plot.width - 2"
                :y="line.averageY - 4"
                text-anchor="end"
                :fill="line.color"
              >
                {{ t("dashboard.average") }} {{ line.averageLabel }}
              </text>
            </template>
            <circle
              v-for="(point, index) in line.points"
              :key="`dot-${index}`"
              :cx="point.x"
              :cy="point.y"
              r="2.6"
              :fill="line.color"
            />
            <template v-if="showValueLabels">
              <text
                v-for="(point, index) in line.points"
                v-show="point.labeled"
                :key="`label-${index}`"
                :x="point.x"
                :y="point.labelY"
                :text-anchor="point.anchor"
                :fill="line.color"
                class="trend-value-label"
              >
                {{ point.value.toLocaleString() }}
              </text>
            </template>
          </g>

          <g v-if="rightTicks.length" class="chart-grid trend-right-axis">
            <text
              v-for="tick in rightTicks"
              :key="`right-${tick.y}`"
              :x="plot.left + plot.width + 8"
              :y="tick.y + 4"
              text-anchor="start"
            >
              {{ tick.label }}
            </text>
          </g>

          <g v-if="hovered" class="trend-hover">
            <line
              :x1="hovered.x"
              :x2="hovered.x"
              :y1="plot.top"
              :y2="plot.top + plot.height"
            />
            <circle
              v-for="item in hovered.items"
              :key="`hover-${item.name}`"
              :cx="hovered.x"
              :cy="item.y"
              r="3.8"
              fill="var(--card-bg)"
              :stroke="item.color"
              stroke-width="2"
            />
          </g>
        </template>

        <template v-else>
          <rect
            v-for="(bar, index) in bars"
            :key="index"
            :x="bar.x"
            :y="bar.y"
            :width="bar.width"
            :height="bar.height"
            rx="2"
            class="trend-bar"
            :class="{ 'is-active': hoverIndex === index }"
            :fill="PALETTE[0]"
          />
          <text
            v-for="(bar, index) in barLabels"
            v-show="showValueLabels"
            :key="`bar-label-${index}`"
            :x="bar.labelX"
            :y="bar.labelY"
            text-anchor="middle"
            class="trend-value-label"
          >
            {{ bar.value }}
          </text>
        </template>

        <g class="chart-grid">
          <text
            v-for="tick in xTicks"
            :key="`x-${tick.index}`"
            :x="tick.x"
            :y="layout.height - 10"
            :text-anchor="tick.anchor"
            class="axis-label"
          >
            {{ tick.label }}
          </text>
        </g>
      </svg>

      <div
        v-if="hovered"
        class="trend-tooltip"
        :class="{ 'is-flip': hovered.flip, 'is-below': hovered.below }"
        :style="{ left: hovered.left, top: hovered.top }"
      >
        <div class="trend-tooltip-date">{{ hovered.date }}</div>
        <div v-for="item in hovered.items" :key="`tip-${item.name}`" class="trend-tooltip-row">
          <i class="legend-dot" :style="{ background: item.color }"></i>
          <span class="trend-tooltip-name">{{ item.name }}</span>
          <span class="trend-tooltip-value">{{ item.value.toLocaleString() }}</span>
        </div>
      </div>
    </div>
  </section>
</template>
