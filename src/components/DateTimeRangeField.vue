<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { t } from "../i18n";
import NavIcon from "./NavIcon.vue";

/** 可执行时间范围：单入口 + 弹层里两个 datetime-local，值统一成 "YYYY-MM-DD HH:mm"。 */
const props = defineProps<{
  start: string;
  end: string;
}>();

const emit = defineEmits<{
  "update:start": [value: string];
  "update:end": [value: string];
}>();

const open = ref(false);
const draftStart = ref("");
const draftEnd = ref("");
const root = ref<HTMLElement | null>(null);

function toInput(value: string): string {
  return value.trim().replace(" ", "T");
}

function toArchery(value: string): string {
  return value.trim().replace("T", " ");
}

const label = computed(() => {
  const from = props.start.trim();
  const to = props.end.trim();
  if (!from && !to) return t("workflow.pickRunDate");
  return `${from || "…"} - ${to || "…"}`;
});

watch(
  () => [props.start, props.end],
  ([start, end]) => {
    draftStart.value = toInput(start ?? "");
    draftEnd.value = toInput(end ?? "");
  },
);

function toggle(): void {
  open.value = !open.value;
  if (open.value) {
    draftStart.value = toInput(props.start);
    draftEnd.value = toInput(props.end);
  }
}

function apply(): void {
  const from = toArchery(draftStart.value);
  const to = toArchery(draftEnd.value);
  if (!from || !to) return;
  // 起止反了自动交换，避免提交出无效窗口。
  const start = from <= to ? from : to;
  const end = from <= to ? to : from;
  emit("update:start", start);
  emit("update:end", end);
  open.value = false;
}

function clear(): void {
  draftStart.value = "";
  draftEnd.value = "";
  emit("update:start", "");
  emit("update:end", "");
  open.value = false;
}

function onDocumentMouseDown(event: MouseEvent): void {
  if (!open.value) return;
  if (root.value && !root.value.contains(event.target as Node)) {
    open.value = false;
  }
}

onMounted(() => document.addEventListener("mousedown", onDocumentMouseDown));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocumentMouseDown));
</script>

<template>
  <div ref="root" class="date-time-range">
    <button
      type="button"
      class="form-control form-control-sm date-time-range-trigger"
      :class="{ 'is-empty': !start && !end }"
      :title="label"
      @click="toggle"
    >
      <span class="date-time-range-label">{{ label }}</span>
      <NavIcon name="clock" :size="13" class="date-time-range-icon" />
    </button>

    <div v-if="open" class="date-time-range-menu">
      <input
        v-model="draftStart"
        class="form-control form-control-sm"
        type="datetime-local"
        :aria-label="t('workflow.runDateStart')"
      />
      <span class="date-time-range-dash">-</span>
      <input
        v-model="draftEnd"
        class="form-control form-control-sm"
        type="datetime-local"
        :aria-label="t('workflow.runDateEnd')"
      />
      <div class="date-time-range-actions">
        <button class="btn btn-default btn-xs" @click="clear">{{ t("workflow.runDateClear") }}</button>
        <button
          class="btn btn-primary btn-xs"
          :disabled="!draftStart || !draftEnd"
          @click="apply"
        >
          {{ t("workflow.dateApply") }}
        </button>
      </div>
    </div>
  </div>
</template>
