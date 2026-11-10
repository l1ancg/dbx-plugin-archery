<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { t } from "../i18n";
import { formatMetric } from "../lib/dashboard";
import type { DateRangePreset } from "../lib/date-range";
import { shouldStackTrendPanels } from "../lib/trend";
import { metricRoute } from "../lib/nav";
import type { DashboardPayload, DashboardTrends, InstanceItem, Overview } from "../types";
import DateRangeFilter from "../components/DateRangeFilter.vue";
import InstanceTypeChart from "../components/InstanceTypeChart.vue";
import NavIcon from "../components/NavIcon.vue";
import StatCard from "../components/StatCard.vue";
import TrendChart from "../components/TrendChart.vue";

const props = defineProps<{
  overview: Overview | null;
  instances: InstanceItem[];
  payload: DashboardPayload | null;
  /** 按日统计图的数据（随统计区间变化，与整页 payload 分开维护）。 */
  trends: DashboardTrends | null;
  loading: boolean;
  /** 按日统计图的统计区间（Archery 首页回显的值）。 */
  rangeStart: string;
  rangeEnd: string;
}>();

const emit = defineEmits<{
  more: [key: string];
  refresh: [];
  "range-change": [start: string, end: string];
}>();

/** 与 Archery 首页一致：没有「全部」，默认最近 7 日。 */
const TREND_PRESETS: DateRangePreset[] = [
  "last7",
  "last30",
  "last365",
  "thisMonth",
  "thisYear",
  "lastMonth",
  "lastYear",
  "custom",
];

const draftStart = ref(props.rangeStart);
const draftEnd = ref(props.rangeEnd);

watch(
  () => [props.rangeStart, props.rangeEnd],
  ([start, end]) => {
    draftStart.value = start;
    draftEnd.value = end;
  },
);

function onRangeChange(): void {
  emit("range-change", draftStart.value, draftEnd.value);
}

interface MetricView {
  key: string;
  tone: "blue" | "green" | "orange" | "red";
  icon: string;
  value: string;
  label: string;
  /** 有目标列表页才给「更多」；没有的（有效平台用户、实例数量）不留无效入口。 */
  more: boolean;
}

const metrics = computed<MetricView[]>(() => {
  const counts = props.payload?.counts;
  const cards: Array<Omit<MetricView, "more">> = [
    {
      key: "workflow",
      tone: "blue",
      icon: "check",
      value: formatMetric(counts?.sqlWorkflow ?? null),
      label: t("dashboard.metricWorkflow"),
    },
    {
      key: "query",
      tone: "green",
      icon: "search",
      value: formatMetric(counts?.sqlQuery ?? null),
      label: t("dashboard.metricQuery"),
    },
    {
      key: "users",
      tone: "orange",
      icon: "user",
      value: formatMetric(counts?.activeUsers ?? null),
      label: t("dashboard.metricUsers"),
    },
    {
      key: "instances",
      tone: "red",
      icon: "database",
      value: formatMetric(counts?.instances ?? props.instances.length),
      label: t("dashboard.metricInstances"),
    },
  ];
  return cards.map((card) => ({ ...card, more: metricRoute(card.key) !== null }));
});

const degradedText = computed(() => {
  if (!props.payload?.degraded) return "";
  return t(`dashboard.reason.${props.payload.reason ?? "unavailable"}`);
});

/** 首页两张按日趋势图的数据；Archery 没渲染时为空，卡片保留占位说明。 */
const trends = computed(() => props.trends);
const trendDates = computed(() => trends.value?.dates ?? []);
const trendRange = computed(() => trends.value?.rangeLabel ?? "");
const releaseSeries = computed(() => [
  { name: t("dashboard.workflowStat"), values: trends.value?.release ?? [] },
]);

/** 区间超过 30 天时两张图改上下排列，各占满一行，日期刻度才排得开。 */
const stackedPanels = computed(() => shouldStackTrendPanels(trendDates.value));

</script>

<template>
  <div class="page page-scroll">
    <div v-if="degradedText" class="notice notice-warn">
      <NavIcon name="clock" :size="14" />
      <span>{{ degradedText }}</span>
    </div>

    <!-- 口径说明：四个数字取自 Archery 首页全量统计，实例图按当前账号可见实例实时汇总。
         原来的「统计时间范围」控件既不参与取数也不过滤图表，属于误导，已换成这条说明。 -->
    <div class="range-bar">
      <span class="spacer"></span>
      <button class="btn ghost" :disabled="loading" @click="emit('refresh')">
        {{ t("app.refresh") }}
      </button>
    </div>

    <div class="stat-grid">
      <StatCard
        v-for="metric in metrics"
        :key="metric.key"
        :tone="metric.tone"
        :icon="metric.icon"
        :value="metric.value"
        :label="metric.label"
        :more="metric.more"
        :loading="loading"
        @more="emit('more', metric.key)"
      />
    </div>

    <InstanceTypeChart :instances="instances" :stats="payload?.instanceStats ?? null" />

    <!-- 统计区间只作用于下面两张按日统计图，与 Archery 首页的取数方式一致 -->
    <div class="trend-toolbar">
      <span class="trend-toolbar-label">{{ t("dashboard.trendRange") }}</span>
      <DateRangeFilter
        v-model:start="draftStart"
        v-model:end="draftEnd"
        :presets="TREND_PRESETS"
        :end-offset-days="1"
        @change="onRangeChange"
      />
      <span class="spacer"></span>
    </div>

    <div class="stat-panels" :class="{ 'is-stacked': stackedPanels }">
      <TrendChart
        mode="line"
        icon="query"
        :title="t('dashboard.queryStat')"
        :subtitle="trendRange"
        :dates="trendDates"
        :series="trends?.query ?? []"
      />
      <TrendChart
        mode="bar"
        icon="audit"
        :title="t('dashboard.workflowStat')"
        :subtitle="trendRange"
        :dates="trendDates"
        :series="releaseSeries"
      />
    </div>
  </div>
</template>
