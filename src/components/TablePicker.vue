<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { t } from "../i18n";
import { filterTables, moveIndex } from "../lib/table-picker";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  /** 当前已查看的表名，用于在下拉里打勾。 */
  value: string;
  tables: string[];
  loading?: boolean;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  select: [name: string];
  open: [];
}>();

const keyword = ref("");
const opened = ref(false);
const cursor = ref(-1);
const root = ref<HTMLElement | null>(null);

const filtered = computed(() => filterTables(props.tables, keyword.value));

/**
 * 打开候选列表。点输入框（mousedown / click）与聚焦都会走到这里：
 * 只靠 focus 时，输入框一直保持聚焦（选完一张表想再选）或宿主 iframe 首次点击
 * 都不会再触发 focus，表现为「点一下没反应，得先点别处再点回来」。
 */
function openOptions(): void {
  if (props.disabled) return;
  if (opened.value) {
    // 已经打开但列表还是空的（上次加载失败 / 刚切库）：补一次加载，避免一直空着。
    if (props.tables.length === 0 && !props.loading) emit("open");
    return;
  }
  opened.value = true;
  const picked = filtered.value.findIndex((name) => name === props.value);
  cursor.value = picked >= 0 ? picked : filtered.value.length > 0 ? 0 : -1;
  emit("open");
}

function onInput(): void {
  opened.value = true;
  cursor.value = filtered.value.length > 0 ? 0 : -1;
  emit("open");
}

function pick(name: string | undefined): void {
  if (!name) return;
  opened.value = false;
  keyword.value = "";
  cursor.value = -1;
  emit("select", name);
}

function onKeydown(event: KeyboardEvent): void {
  switch (event.key) {
    case "ArrowDown":
      event.preventDefault();
      if (!opened.value) {
        openOptions();
        return;
      }
      cursor.value = moveIndex(cursor.value, 1, filtered.value.length);
      break;
    case "ArrowUp":
      event.preventDefault();
      if (!opened.value) {
        openOptions();
        return;
      }
      cursor.value = moveIndex(cursor.value, -1, filtered.value.length);
      break;
    case "Enter":
      event.preventDefault();
      pick(filtered.value[cursor.value] ?? filtered.value[0]);
      break;
    case "Escape":
      opened.value = false;
      keyword.value = "";
      break;
  }
}

function onDocumentClick(event: MouseEvent): void {
  if (!opened.value) return;
  if (root.value && !root.value.contains(event.target as Node)) {
    opened.value = false;
  }
}

onMounted(() => document.addEventListener("mousedown", onDocumentClick));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocumentClick));
</script>

<template>
  <div ref="root" class="table-picker">
    <NavIcon name="search" :size="13" class="table-picker-icon" />
    <input
      v-model="keyword"
      class="form-control form-control-sm table-picker-input"
      type="text"
      :placeholder="t('describe.searchPlaceholder')"
      :disabled="disabled"
      :aria-label="t('describe.searchPlaceholder')"
      @mousedown="openOptions"
      @click="openOptions"
      @focus="openOptions"
      @input="onInput"
      @keydown="onKeydown"
    />
    <NavIcon name="chevron" :size="12" class="table-picker-caret" :class="{ 'is-open': opened }" />

    <ul v-if="opened" class="table-options">
      <li v-if="loading" class="table-option muted">
        <span class="spinner"></span>
        {{ t("app.loading") }}
      </li>
      <li v-else-if="filtered.length === 0" class="table-option muted">
        {{ t("describe.noTableMatched") }}
      </li>
      <template v-else>
        <li
          v-for="(name, index) in filtered"
          :key="name"
          class="table-option"
          :class="{ active: index === cursor, picked: name === value }"
          @mouseenter="cursor = index"
          @mousedown.prevent="pick(name)"
        >
          <span class="table-option-name">{{ name }}</span>
          <NavIcon
            v-if="name === value || (!value && index === cursor)"
            name="check"
            :size="12"
            class="table-option-check"
          />
        </li>
      </template>
    </ul>
  </div>
</template>
