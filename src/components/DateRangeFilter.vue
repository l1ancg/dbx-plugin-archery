<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { t } from "../i18n";
import {
  formatRange,
  presetRangeWithOffset,
  type DateRangePreset,
} from "../lib/date-range";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  start: string;
  end: string;
  /** 只显示这些预设；不传则用列表页那套完整预设。 */
  presets?: DateRangePreset[];
  /**
   * 滚动区间（最近 N 日）的右端往后推几天。
   * Archery 首页的「最近 7 日」实为 [D-6, D+1]，含端点 8 天，这里用 1 复刻。
   */
  endOffsetDays?: number;
}>();

const emit = defineEmits<{
  "update:start": [value: string];
  "update:end": [value: string];
  change: [];
}>();

const open = ref(false);
const customMode = ref(false);
const customStart = ref(props.start);
const customEnd = ref(props.end);
const root = ref<HTMLElement | null>(null);

/**
 * 预设对应的实际区间。endOffsetDays 只作用于滚动区间：
 * 首页「最近 7 日」= [D-6, D+1]，与 Archery 默认窗口（含明天）一致。
 */
function rangeFor(preset: DateRangePreset): { start: string; end: string } {
  return presetRangeWithOffset(preset, props.endOffsetDays ?? 0);
}

const label = computed(() => formatRange(props.start, props.end, t("workflow.dateAll")));

const PRESET_LABELS: Record<DateRangePreset, string> = {
  all: "workflow.dateAll",
  today: "workflow.dateToday",
  yesterday: "workflow.dateYesterday",
  last7: "workflow.dateLast7",
  last30: "workflow.dateLast30",
  last365: "workflow.dateLast365",
  thisMonth: "workflow.dateThisMonth",
  lastMonth: "workflow.dateLastMonth",
  thisYear: "workflow.dateThisYear",
  lastYear: "workflow.dateLastYear",
  custom: "workflow.dateCustom",
};

const DEFAULT_PRESETS: DateRangePreset[] = [
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
  "custom",
];

const presets = computed<Array<{ key: DateRangePreset; labelKey: string }>>(() =>
  (props.presets ?? DEFAULT_PRESETS).map((key) => ({ key, labelKey: PRESET_LABELS[key] })),
);

/** 反查当前区间属于哪个预设；要按 rangeFor 的口径比对（含 endOffsetDays）。 */
const active = computed<DateRangePreset>(() => {
  const from = props.start.trim();
  const to = props.end.trim();
  if (!from && !to) return "all";
  for (const item of presets.value) {
    const range = rangeFor(item.key);
    if (range.start === from && range.end === to) return item.key;
  }
  return "custom";
});

watch(
  () => [props.start, props.end],
  ([start, end]) => {
    customStart.value = start ?? "";
    customEnd.value = end ?? "";
  },
);

function apply(start: string, end: string): void {
  emit("update:start", start);
  emit("update:end", end);
  open.value = false;
  customMode.value = false;
  emit("change");
}

function choose(preset: DateRangePreset): void {
  if (preset === "custom") {
    customMode.value = true;
    return;
  }
  const range = rangeFor(preset);
  apply(range.start, range.end);
}

function applyCustom(): void {
  const from = customStart.value.trim();
  const to = customEnd.value.trim();
  if (!from || !to) return;
  // 起止反了就自动交换，避免直接给出空结果。
  const [start, end] = from <= to ? [from, to] : [to, from];
  apply(start, end);
}

function toggle(): void {
  open.value = !open.value;
  if (open.value) {
    customStart.value = props.start;
    customEnd.value = props.end;
    customMode.value = active.value === "custom";
  }
}

function onDocumentMouseDown(event: MouseEvent): void {
  if (!open.value) return;
  if (root.value && !root.value.contains(event.target as Node)) {
    open.value = false;
    customMode.value = false;
  }
}

onMounted(() => document.addEventListener("mousedown", onDocumentMouseDown));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocumentMouseDown));
</script>

<template>
  <div ref="root" class="date-range">
    <button
      type="button"
      class="form-control form-control-sm date-range-trigger"
      :title="label"
      @click="toggle"
    >
      <NavIcon name="clock" :size="13" />
      <span class="date-range-label">{{ label }}</span>
      <NavIcon name="chevron" :size="10" class="date-range-caret" />
    </button>

    <div v-if="open" class="date-range-menu">
      <button
        v-for="item in presets"
        :key="item.key"
        type="button"
        class="date-range-item"
        :class="{
          active: item.key === 'custom' ? customMode : !customMode && active === item.key,
        }"
        @click="choose(item.key)"
      >
        {{ t(item.labelKey) }}
      </button>

      <div v-if="customMode" class="date-range-custom">
        <input
          v-model="customStart"
          class="form-control form-control-sm"
          type="date"
          :aria-label="t('workflow.dateStart')"
        />
        <span class="date-range-dash">-</span>
        <input
          v-model="customEnd"
          class="form-control form-control-sm"
          type="date"
          :aria-label="t('workflow.dateEnd')"
        />
        <button
          class="btn btn-primary btn-xs"
          :disabled="!customStart || !customEnd"
          @click="applyCustom"
        >
          {{ t("workflow.dateApply") }}
        </button>
      </div>
    </div>
  </div>
</template>
