<script setup lang="ts">
import { computed } from "vue";
import { t } from "../i18n";
import { envTypeMatrix, instanceTypeCounts, percent } from "../lib/dashboard";
import type { DashboardInstanceStats, InstanceItem } from "../types";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  instances: InstanceItem[];
  /** 首页图表数据（全库实例）；取不到时才退回按当前账号可见实例统计。 */
  stats?: DashboardInstanceStats | null;
}>();

// 图表配色比状态色低一档饱和度：仪表盘上仍能区分类型，但不再抢指标卡与表格的注意力
const PALETTE = ["#4f8ed6", "#4aab6e", "#d8a43c", "#cd7268", "#8b7ec8", "#47a6b5"];

const counts = computed(() => {
  const stats = props.stats;
  if (stats && stats.types.length > 0) {
    return stats.types.map((item) => ({ dbType: item.dbType, count: item.count }));
  }
  return instanceTypeCounts(props.instances);
});

const matrix = computed(() => {
  const stats = props.stats;
  if (stats && stats.dbTypes.length > 0 && stats.series.length > 0) {
    return {
      dbTypes: stats.dbTypes,
      envs: stats.envs,
      // 后端是「环境 → 各类型」，图表要「类型 → 各环境」，这里转置一次
      series: stats.dbTypes.map((dbType, index) => ({
        dbType,
        values: stats.series.map((item) => item.values[index] ?? 0),
      })),
    };
  }
  return envTypeMatrix(props.instances);
});

const total = computed(() => counts.value.reduce((sum, item) => sum + item.count, 0));

/** 色板按 dbType 固定分配，柱状图与饼图共用，同一类型颜色始终一致。 */
const colorMap = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {};
  counts.value.forEach((item, index) => {
    map[item.dbType] = PALETTE[index % PALETTE.length];
  });
  return map;
});

function colorFor(dbType: string): string {
  return colorMap.value[dbType] ?? PALETTE[0];
}

/** 同一根柱子内用透明度区分环境，避免引入第二套色板。 */
function envOpacity(index: number): number {
  return Math.max(0.34, 1 - index * 0.32);
}

const BAR = { width: 360, height: 246, left: 34, right: 10, top: 14, bottom: 30 };
const PIE = { size: 232, center: 116, radius: 74, stroke: 34 };

const plot = computed(() => ({
  left: BAR.left,
  top: BAR.top,
  width: BAR.width - BAR.left - BAR.right,
  height: BAR.height - BAR.top - BAR.bottom,
}));

/** 纵轴按「单根堆叠柱的总高」归一，而不是单个环境的最大值。 */
const maxValue = computed(() => {
  const { series } = matrix.value;
  const groups = series[0]?.values.length ?? 0;
  let max = 1;
  for (let index = 0; index < groups; index += 1) {
    let sum = 0;
    for (const item of series) {
      sum += item.values[index] ?? 0;
    }
    max = Math.max(max, sum);
  }
  return Math.max(4, Math.ceil(max / 4) * 4);
});

const ticks = computed(() => {
  const step = maxValue.value / 4;
  return [0, 1, 2, 3, 4].map((index) => ({
    value: Math.round(index * step),
    y: plot.value.top + plot.value.height - (index / 4) * plot.value.height,
  }));
});

interface BarSegment {
  env: string;
  value: number;
  y: number;
  height: number;
  opacity: number;
}

interface BarGroup {
  dbType: string;
  x: number;
  width: number;
  color: string;
  segments: BarSegment[];
}

/** X 轴按数据库类型分组（mysql / doris / redis），柱内按环境分段。 */
const bars = computed<BarGroup[]>(() => {
  const groups: BarGroup[] = [];
  const { dbTypes, envs, series } = matrix.value;
  if (dbTypes.length === 0) return groups;

  const slot = plot.value.width / dbTypes.length;
  const width = Math.min(64, Math.max(22, slot * 0.62));

  dbTypes.forEach((dbType, typeIndex) => {
    const x = plot.value.left + slot * typeIndex + (slot - width) / 2;
    const values = series[typeIndex]?.values ?? [];
    let cursor = plot.value.top + plot.value.height;
    const segments: BarSegment[] = [];

    envs.forEach((env, envIndex) => {
      const value = values[envIndex] ?? 0;
      if (value <= 0) return;
      const height = (value / maxValue.value) * plot.value.height;
      cursor -= height;
      segments.push({ env, value, y: cursor, height, opacity: envOpacity(envIndex) });
    });

    groups.push({ dbType, x, width, color: colorFor(dbType), segments });
  });

  return groups;
});

const circumference = computed(() => 2 * Math.PI * PIE.radius);

const slices = computed(() => {
  let offset = 0;
  return counts.value.map((item) => {
    const length = total.value > 0 ? (item.count / total.value) * circumference.value : 0;
    const slice = {
      dbType: item.dbType,
      count: item.count,
      percent: percent(item.count, total.value),
      color: colorFor(item.dbType),
      dash: `${Math.max(0, length - 2)} ${circumference.value}`,
      offset,
    };
    offset += length;
    return slice;
  });
});
</script>

<template>
  <section class="card chart-card">
    <header class="card-head">
      <h2 class="card-title">
        <NavIcon name="chart" :size="14" />
        <span>{{ t("dashboard.instanceType") }}</span>
      </h2>
      <div v-if="matrix.envs.length > 1" class="chart-legend">
        <span class="legend-label">{{ t("dashboard.envLegend") }}</span>
        <span v-for="(env, index) in matrix.envs" :key="env" class="legend-item">
          <i
            class="legend-dot"
            :style="{ background: 'var(--tone-blue)', opacity: envOpacity(index) }"
          ></i>
          <span>{{ env }}</span>
        </span>
      </div>
    </header>

    <div v-if="counts.length === 0" class="chart-empty">{{ t("dashboard.chartEmpty") }}</div>

    <div v-else class="chart-body">
      <div class="chart-bars">
        <svg :viewBox="`0 0 ${BAR.width} ${BAR.height}`" class="chart-svg" role="img">
          <g class="chart-grid">
            <template v-for="tick in ticks" :key="tick.y">
              <line :x1="plot.left" :x2="plot.left + plot.width" :y1="tick.y" :y2="tick.y" />
              <text :x="plot.left - 8" :y="tick.y + 4" text-anchor="end">{{ tick.value }}</text>
            </template>
          </g>

          <g v-for="bar in bars" :key="bar.dbType">
            <rect
              v-for="segment in bar.segments"
              :key="segment.env"
              :x="bar.x"
              :y="segment.y"
              :width="bar.width"
              :height="segment.height"
              :fill="bar.color"
              :fill-opacity="segment.opacity"
              rx="2"
            >
              <title>{{ segment.env }} / {{ bar.dbType }}: {{ segment.value }}</title>
            </rect>
            <text
              :x="bar.x + bar.width / 2"
              :y="BAR.height - 10"
              text-anchor="middle"
              class="axis-label"
            >
              {{ bar.dbType }}
            </text>
          </g>
        </svg>
      </div>

      <div class="chart-pie">
        <svg :viewBox="`0 0 ${PIE.size} ${PIE.size}`" class="chart-svg pie-svg" role="img">
          <circle
            :cx="PIE.center"
            :cy="PIE.center"
            :r="PIE.radius"
            fill="none"
            stroke="var(--bg-inset)"
            :stroke-width="PIE.stroke"
          />
          <circle
            v-for="slice in slices"
            :key="slice.dbType"
            :cx="PIE.center"
            :cy="PIE.center"
            :r="PIE.radius"
            fill="none"
            :stroke="slice.color"
            :stroke-width="PIE.stroke"
            :stroke-dasharray="slice.dash"
            :stroke-dashoffset="-slice.offset"
            :transform="`rotate(-90 ${PIE.center} ${PIE.center})`"
          />
          <text :x="PIE.center" :y="PIE.center - 1" text-anchor="middle" class="pie-total">
            {{ total }}
          </text>
          <text
            :x="PIE.center"
            :y="PIE.center + 17"
            text-anchor="middle"
            class="pie-total-label"
          >
            {{ t("dashboard.metricInstances") }}
          </text>
        </svg>

        <ul class="pie-legend">
          <li v-for="slice in slices" :key="slice.dbType">
            <span class="pie-legend-head">
              <i class="legend-dot" :style="{ background: slice.color }"></i>
              <span class="pie-legend-name">{{ slice.dbType }}</span>
              <span class="pie-legend-value">{{ slice.count }}</span>
              <span class="pie-legend-percent">{{ slice.percent }}%</span>
            </span>
            <span class="pie-legend-bar">
              <i :style="{ width: `${slice.percent}%`, background: slice.color }"></i>
            </span>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>
