<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { t } from "../i18n";
import { hiddenCount, matchColumns, type ColumnMatchMode } from "../lib/column-picker";
import NavIcon from "./NavIcon.vue";

/**
 * 浮层定位：横向给 left 就贴按钮左缘，否则用 right 贴按钮右缘；
 * 纵向给 top 就向下开，否则用 bottom 向上开。
 */
interface PickerAnchor {
  top?: number;
  bottom?: number;
  left?: number;
  right?: number;
  maxHeight: number;
}

const props = defineProps<{
  columns: string[];
  columnTypes?: string[];
  /** 与 columns 等长：true 表示该列显示。 */
  visible: boolean[];
  anchor: PickerAnchor;
}>();

const emit = defineEmits<{
  toggle: [index: number];
  showAll: [];
  close: [];
}>();

const query = ref("");
const mode = ref<ColumnMatchMode>("fuzzy");
/** 键盘当前停在过滤结果的第几项。 */
const active = ref(0);
const inputEl = ref<HTMLInputElement | null>(null);
const listEl = ref<HTMLElement | null>(null);

const matches = computed(() =>
  matchColumns(props.columns, props.columnTypes, query.value, mode.value),
);
const hidden = computed(() => hiddenCount(props.visible));
const visibleCount = computed(() => props.visible.filter(Boolean).length);

/** 只剩这一列可见时不允许取消，与 toggleVisible 的规则保持一致。 */
function locked(index: number): boolean {
  return visibleCount.value === 1 && props.visible[index];
}

function toggle(index: number): void {
  if (locked(index)) return;
  emit("toggle", index);
}

watch(matches, () => {
  active.value = 0;
});

watch(active, async () => {
  await nextTick();
  listEl.value
    ?.querySelector<HTMLElement>(".column-picker-item.is-active")
    ?.scrollIntoView({ block: "nearest" });
});

onMounted(() => {
  inputEl.value?.focus();
});

function step(delta: number): void {
  const total = matches.value.length;
  if (total === 0) return;
  active.value = (active.value + delta + total) % total;
}

/** 输入框内接管方向键与回车（回车切换显隐）；Esc 在任何位置都收起面板。 */
function onKeydown(event: KeyboardEvent): void {
  if (event.key === "ArrowDown") {
    event.preventDefault();
    step(1);
  } else if (event.key === "ArrowUp") {
    event.preventDefault();
    step(-1);
  } else if (event.key === "Enter") {
    event.preventDefault();
    const picked = matches.value[active.value];
    if (picked) toggle(picked.index);
  } else if (event.key === "Escape") {
    event.preventDefault();
    emit("close");
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      class="column-picker-panel"
      role="dialog"
      :aria-label="t('result.columnPicker')"
      :style="{
        ...(anchor.top === undefined ? { bottom: `${anchor.bottom}px` } : { top: `${anchor.top}px` }),
        ...(anchor.left === undefined
          ? { right: `${anchor.right}px` }
          : { left: `${anchor.left}px` }),
        maxHeight: `${anchor.maxHeight}px`,
      }"
    >
      <header class="column-picker-head">
        <span class="column-picker-title">
          <NavIcon name="columns" :size="13" />
          {{ t("result.columnPicker") }}
        </span>
        <button
          class="column-picker-close"
          type="button"
          :title="t('result.close')"
          :aria-label="t('result.close')"
          @click="emit('close')"
        >
          <NavIcon name="close" :size="12" />
        </button>
      </header>

      <div class="column-picker-search">
        <NavIcon name="search" :size="13" class="column-picker-search-icon" />
        <input
          ref="inputEl"
          v-model="query"
          type="text"
          spellcheck="false"
          :placeholder="t('result.searchColumnPlaceholder')"
          @keydown="onKeydown"
        />
        <select v-model="mode" :aria-label="t('result.matchMode')" @keydown.esc="emit('close')">
          <option value="fuzzy">{{ t("result.matchFuzzy") }}</option>
          <option value="prefix">{{ t("result.matchPrefix") }}</option>
          <option value="exact">{{ t("result.matchExact") }}</option>
        </select>
      </div>

      <ul ref="listEl" class="column-picker-list">
        <li
          v-for="(item, at) in matches"
          :key="item.index"
          class="column-picker-item"
          :class="{
            'is-active': at === active,
            'is-off': !visible[item.index],
            'is-locked': locked(item.index),
          }"
          role="checkbox"
          :aria-checked="visible[item.index]"
          :aria-disabled="locked(item.index)"
          :title="locked(item.index) ? t('result.keepOneColumn') : undefined"
          @mousemove="active = at"
          @click="toggle(item.index)"
        >
          <span class="column-picker-check" :class="{ on: visible[item.index] }">
            <NavIcon v-if="visible[item.index]" name="check" :size="10" />
          </span>
          <span class="column-picker-text">
            <span class="column-picker-name">{{ item.name }}</span>
            <span v-if="item.note" class="column-picker-note">{{ item.note }}</span>
          </span>
          <span class="column-picker-index">#{{ item.index + 1 }}</span>
        </li>
        <li v-if="matches.length === 0" class="column-picker-empty">
          {{ t("result.columnNoMatch") }}
        </li>
      </ul>

      <footer class="column-picker-foot">
        <span>{{ t("result.columnsHidden", { n: hidden }) }}</span>
        <button
          class="column-picker-reset"
          type="button"
          :disabled="hidden === 0"
          @click="emit('showAll')"
        >
          {{ t("result.showAllColumns") }}
        </button>
      </footer>
    </div>
  </Teleport>
</template>
