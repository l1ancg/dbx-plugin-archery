<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { t } from "../i18n";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  modelValue: string[];
  options: string[];
  placeholder?: string;
  loading?: boolean;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string[]];
}>();

const root = ref<HTMLElement | null>(null);
const open = ref(false);
const keyword = ref("");

const filtered = computed(() => {
  const key = keyword.value.trim().toLowerCase();
  if (!key) return props.options;
  return props.options.filter((option) => option.toLowerCase().includes(key));
});

function toggle(option: string): void {
  const next = props.modelValue.includes(option)
    ? props.modelValue.filter((item) => item !== option)
    : [...props.modelValue, option];
  emit("update:modelValue", next);
}

function onDocumentClick(event: MouseEvent): void {
  if (!root.value) return;
  if (!root.value.contains(event.target as Node)) open.value = false;
}

onMounted(() => document.addEventListener("mousedown", onDocumentClick));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocumentClick));
</script>

<template>
  <div ref="root" class="multi-select" :class="{ 'is-open': open, 'is-disabled': disabled }">
    <button
      type="button"
      class="form-control multi-select-trigger"
      :disabled="disabled"
      @click="open = !open"
    >
      <span v-if="modelValue.length" class="multi-select-value" :title="modelValue.join(', ')">
        {{ modelValue.join(", ") }}
      </span>
      <span v-else class="multi-select-placeholder">{{ placeholder ?? "" }}</span>
      <NavIcon name="chevron" :size="12" class="multi-select-arrow" />
    </button>

    <div v-if="open" class="multi-select-panel">
      <input
        v-model="keyword"
        class="form-control form-control-sm"
        type="search"
        :placeholder="t('app.search')"
      />
      <div class="multi-select-options">
        <label v-for="option in filtered" :key="option" class="multi-select-option">
          <input type="checkbox" :checked="modelValue.includes(option)" @change="toggle(option)" />
          <span>{{ option }}</span>
        </label>
        <div v-if="loading" class="multi-select-empty">{{ t("app.loading") }}</div>
        <div v-else-if="filtered.length === 0" class="multi-select-empty">{{ t("app.empty") }}</div>
      </div>
      <div class="multi-select-foot">
        <span class="subtle">{{ t("privilege.selected", { n: modelValue.length }) }}</span>
        <button type="button" class="btn ghost btn-xs" @click="emit('update:modelValue', [])">
          {{ t("privilege.clear") }}
        </button>
      </div>
    </div>
  </div>
</template>
