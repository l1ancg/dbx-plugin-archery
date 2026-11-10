<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { t } from "../i18n";
import { moveIndex, optionWindow } from "../lib/table-picker";
import NavIcon from "./NavIcon.vue";

/**
 * 单值下拉选择器，替代实例 / 库这两个原生 <select>：
 * 原生 select 在宿主 iframe 里首次点击常常只拿到焦点、不弹面板，候选上百上千时一次铺开也会明显卡；
 * 这里点触发器就开（JS 驱动），候选只画前 limit 条，其余靠关键字收窄。
 */
const props = defineProps<{
  modelValue: string;
  options: string[];
  placeholder: string;
  loading?: boolean;
  disabled?: boolean;
  /** 允许清空：有值时触发器上出现 ×。 */
  clearable?: boolean;
  /** 候选超过 limit 条时只渲染前 limit 条。 */
  limit?: number;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  open: [];
}>();

const keyword = ref("");
const opened = ref(false);
const cursor = ref(-1);
const root = ref<HTMLElement | null>(null);
const search = ref<HTMLInputElement | null>(null);

const window_ = computed(() => optionWindow(props.options, keyword.value, props.limit ?? 200));
const visible = computed(() => window_.value.visible);
const hidden = computed(() => window_.value.hidden);

function toggle(): void {
  if (props.disabled) return;
  if (opened.value) {
    close();
    return;
  }
  opened.value = true;
  keyword.value = "";
  const picked = visible.value.findIndex((name) => name === props.modelValue);
  cursor.value = picked >= 0 ? picked : visible.value.length > 0 ? 0 : -1;
  emit("open");
  void nextTick(() => search.value?.focus());
}

function close(): void {
  opened.value = false;
  keyword.value = "";
  cursor.value = -1;
}

function pick(value: string | undefined): void {
  if (value === undefined) return;
  close();
  emit("update:modelValue", value);
}

function clear(): void {
  close();
  emit("update:modelValue", "");
}

function onInput(): void {
  cursor.value = visible.value.length > 0 ? 0 : -1;
  emit("open");
}

function onKeydown(event: KeyboardEvent): void {
  switch (event.key) {
    case "ArrowDown":
      event.preventDefault();
      cursor.value = moveIndex(cursor.value, 1, visible.value.length);
      break;
    case "ArrowUp":
      event.preventDefault();
      cursor.value = moveIndex(cursor.value, -1, visible.value.length);
      break;
    case "Enter":
      event.preventDefault();
      pick(visible.value[cursor.value] ?? visible.value[0]);
      break;
    case "Escape":
      event.preventDefault();
      close();
      break;
  }
}

function onDocumentClick(event: MouseEvent): void {
  if (!opened.value) return;
  if (root.value && !root.value.contains(event.target as Node)) close();
}

onMounted(() => document.addEventListener("mousedown", onDocumentClick));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocumentClick));
</script>

<template>
  <div
    ref="root"
    class="picker"
    :class="{ 'is-open': opened, 'is-disabled': disabled, 'has-clear': clearable && modelValue }"
  >
    <div
      class="picker-trigger"
      role="combobox"
      :aria-expanded="opened"
      :aria-disabled="disabled"
      :tabindex="disabled ? -1 : 0"
      :title="modelValue || placeholder"
      @mousedown.prevent="toggle"
      @keydown.enter.prevent="toggle"
      @keydown.space.prevent="toggle"
    >
      <span v-if="modelValue" class="picker-value">{{ modelValue }}</span>
      <span v-else class="picker-placeholder">{{ placeholder }}</span>
      <span
        v-if="clearable && modelValue && !disabled"
        class="picker-clear"
        :title="t('picker.clear')"
        @mousedown.prevent.stop="clear"
        @click.prevent.stop="clear"
      >
        <NavIcon name="close" :size="10" />
      </span>
      <NavIcon name="chevron" :size="12" class="picker-caret" />
    </div>

    <div v-if="opened" class="picker-pop">
      <label class="picker-search">
        <NavIcon name="search" :size="12" />
        <input
          ref="search"
          v-model="keyword"
          class="picker-search-input"
          type="text"
          :placeholder="t('picker.search')"
          :aria-label="t('picker.search')"
          @input="onInput"
          @keydown="onKeydown"
        />
      </label>

      <div v-if="loading" class="picker-note">
        <span class="spinner"></span>
        {{ t("app.loading") }}
      </div>
      <ul v-else-if="visible.length > 0" class="picker-list">
        <li
          v-for="(name, index) in visible"
          :key="name"
          class="picker-item"
          :class="{ active: index === cursor, picked: name === modelValue }"
          @mouseenter="cursor = index"
          @mousedown.prevent="pick(name)"
        >
          <span class="picker-item-name">{{ name }}</span>
          <NavIcon v-if="name === modelValue" name="check" :size="12" class="picker-item-check" />
        </li>
      </ul>
      <div v-else class="picker-note">{{ t("picker.empty") }}</div>
      <div v-if="hidden > 0" class="picker-note">{{ t("picker.more", { n: hidden }) }}</div>
    </div>
  </div>
</template>
