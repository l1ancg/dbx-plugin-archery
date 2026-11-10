<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { t } from "../i18n";
import { allVisible, pickColumns, toggleVisible, visibleIndexes } from "../lib/column-picker";
import { parentDir, saveBinaryFile } from "../lib/download";
import { describeError } from "../lib/errors";
import { logRowRange, pageWindow } from "../lib/history";
import ColumnPickerPanel from "./ColumnPickerPanel.vue";
import NavIcon from "./NavIcon.vue";
import {
  defaultPageSize,
  formatCell,
  formatElapsed,
  formatNumber,
  highlightSegments,
  matchRowIndexes,
  pageSizeChoices,
  paginate,
  toCsv,
  toMarkdown,
  toTsv,
} from "../lib/result";
import { buildXlsx, exportFilename, XLSX_MIME, XlsxTooLargeError } from "../lib/xlsx";
import type { Cell, QueryResult } from "../types";

const props = defineProps<{
  result: QueryResult | null;
  loading: boolean;
  error: string;
  info: string;
  /** 查询目标行数上限（-1 表示不限）：每页条数的上限跟着它走。 */
  rowLimit: number;
  /** 上次导出成功后的目录；「另存为」默认打开这里。 */
  exportDir: string;
}>();

const emit = defineEmits<{
  copy: [text: string, label: string];
  toast: [text: string, error?: boolean];
  /** 导出成功后回填目录，由父级持久化到 uiState。 */
  saveDir: [dir: string];
}>();

const page = ref(1);
/** 生成 + 落盘期间禁用按钮，避免重复点击把同一个文件写多份。 */
const exporting = ref(false);
const pageSize = ref(defaultPageSize(props.rowLimit));
/** 分页最大值随查询目标行数上限变化，上限本身也留一档。 */
const sizeChoices = computed(() => pageSizeChoices(props.rowLimit));
const widths = ref<Record<number, number>>({});

/** 工具栏两个下拉：复制格式与列配置。 */
const copyOpen = ref(false);
const pickerOpen = ref(false);
/** 列配置是 fixed 浮层：横向贴按钮左缘，纵向贴按钮下方，空间不够时翻向另一侧。 */
const pickerAnchor = ref<{
  top?: number;
  bottom?: number;
  left?: number;
  right?: number;
  maxHeight: number;
}>({
  right: 8,
  maxHeight: 380,
});
const pickerButton = ref<HTMLElement | null>(null);
/** 每列是否显示；换结果时恢复全部显示（最后一列由 toggleVisible 兜住）。 */
const visible = ref<boolean[]>(allVisible(props.result?.columns.length ?? 0));
const visibleIdx = computed(() => visibleIndexes(visible.value));

/** 结果搜索：输入即按可见列过滤，匹配内容在单元格内高亮。 */
const search = ref("");
const searchInput = ref<HTMLInputElement | null>(null);
const needle = computed(() => search.value.trim());
const searching = computed(() => needle.value.length > 0);
/** 命中行的原始下标；没有搜索词时就是全部行。 */
const matchedIndexes = computed(() =>
  matchRowIndexes(props.result?.rows ?? [], visibleIdx.value, needle.value),
);
/** 网格、分页、复制与导出共用一份视图：搜索时只保留命中行，并带原始行号。 */
const viewRows = computed(() =>
  matchedIndexes.value.map((index) => ({
    row: props.result?.rows[index] ?? [],
    index,
  })),
);
const noMatches = computed(() => searching.value && matchedIndexes.value.length === 0);
const exportHint = computed(() =>
  searching.value
    ? t("result.exportFilteredHint", { n: formatNumber(matchedIndexes.value.length) })
    : t("result.exportExcelHint"),
);

/** 与 style.css 的 .column-picker-panel 宽度保持一致。 */
const PICKER_WIDTH = 300;
const PICKER_EDGE = 8;

/** 计算锚点：左缘对齐按钮正下方；右侧放不下才改成右对齐，纵向同理。 */
function pickerAnchorFor(rect: DOMRect): typeof pickerAnchor.value {
  const width = Math.min(PICKER_WIDTH, window.innerWidth - PICKER_EDGE * 2);
  const horizontal =
    rect.left + width + PICKER_EDGE <= window.innerWidth
      ? { left: Math.max(PICKER_EDGE, rect.left) }
      : { right: Math.max(PICKER_EDGE, window.innerWidth - rect.right) };
  const spaceBelow = window.innerHeight - rect.bottom - 12;
  const spaceAbove = rect.top - 12;
  const below = spaceBelow >= 240 || spaceBelow >= spaceAbove;
  const maxHeight = Math.min(380, Math.max(180, below ? spaceBelow : spaceAbove));
  return below
    ? { ...horizontal, top: rect.bottom + 6, maxHeight }
    : { ...horizontal, bottom: window.innerHeight - rect.top + 6, maxHeight };
}

function togglePicker(): void {
  if (pickerOpen.value) {
    pickerOpen.value = false;
    return;
  }
  const rect = pickerButton.value?.getBoundingClientRect();
  if (!rect) return;
  pickerAnchor.value = pickerAnchorFor(rect);
  pickerOpen.value = true;
}

/** 面板要一直贴在按钮下方：窗口缩放或任意容器滚动时重算锚点。 */
function repositionPicker(): void {
  if (!pickerOpen.value) return;
  const rect = pickerButton.value?.getBoundingClientRect();
  if (!rect) return;
  pickerAnchor.value = pickerAnchorFor(rect);
}

/** 切换一列显隐；「最后一列不可隐藏」由 toggleVisible 保证。 */
function toggleColumn(index: number): void {
  visible.value = toggleVisible(visible.value, index);
}

function showAllColumns(): void {
  visible.value = allVisible(props.result?.columns.length ?? 0);
}

/** 导出与复制共用：只取可见列与当前匹配行，做到所见即所得。 */
function visibleTable(): { columns: string[]; rows: Cell[][] } {
  const result = props.result;
  if (!result) return { columns: [], rows: [] };
  const indexes = visibleIdx.value;
  return {
    columns: pickColumns(result.columns, indexes),
    rows: viewRows.value.map((entry) => pickColumns(entry.row, indexes)),
  };
}

function pickCopy(kind: "csv" | "tsv" | "markdown"): void {
  copyOpen.value = false;
  copyAll(kind);
}

/** 点空白处收起两个下拉；列配置面板 Teleport 到 body，因此按 class 判断归属。 */
function onDocumentMouseDown(event: MouseEvent): void {
  const target = event.target as HTMLElement | null;
  if (!target) return;
  if (!target.closest(".copy-menu-wrap")) copyOpen.value = false;
  if (!target.closest(".column-picker-wrap") && !target.closest(".column-picker-panel")) {
    pickerOpen.value = false;
  }
}

/** Ctrl/⌘+F 聚焦结果搜索（编辑器里让给 CodeMirror 自己的查找）；Esc 收起下拉。 */
function onDocumentKeyDown(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
    if (!props.result || props.result.columns.length === 0) return;
    const target = event.target;
    if (
      target instanceof HTMLElement &&
      target.closest(".cm-editor, input, textarea, select, [contenteditable='true']")
    ) {
      return;
    }
    event.preventDefault();
    searchInput.value?.focus();
    searchInput.value?.select();
    return;
  }
  if (event.key !== "Escape") return;
  copyOpen.value = false;
  pickerOpen.value = false;
}

/** Esc：有搜索词先清空，已经空了再让出焦点。 */
function clearSearch(): void {
  if (search.value) {
    search.value = "";
    searchInput.value?.focus();
    return;
  }
  searchInput.value?.blur();
}

onMounted(() => {
  document.addEventListener("mousedown", onDocumentMouseDown);
  document.addEventListener("keydown", onDocumentKeyDown);
  window.addEventListener("resize", repositionPicker);
  // 捕获阶段：结果区的滚动容器不冒泡到 window，但会经过这里。
  window.addEventListener("scroll", repositionPicker, true);
});
onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocumentMouseDown);
  document.removeEventListener("keydown", onDocumentKeyDown);
  window.removeEventListener("resize", repositionPicker);
  window.removeEventListener("scroll", repositionPicker, true);
});

const sliced = computed(() => paginate(viewRows.value, page.value, pageSize.value));
const pageItems = computed(() => pageWindow(sliced.value.page, sliced.value.pageCount));
const range = computed(() =>
  logRowRange(sliced.value.page, pageSize.value, sliced.value.total),
);

watch(
  () => props.result,
  () => {
    page.value = 1;
    widths.value = {};
    copyOpen.value = false;
    pickerOpen.value = false;
    search.value = "";
    visible.value = allVisible(props.result?.columns.length ?? 0);
  },
);

/** 搜索词或可见列变化会改变结果集大小，页码回到第一页。 */
watch([needle, visibleIdx], () => {
  page.value = 1;
});

/** 行数上限变了（重新执行或切换查询目标）：每页条数收敛回候选，页码回第一页。 */
watch(sizeChoices, (choices) => {
  page.value = 1;
  if (!choices.includes(pageSize.value)) pageSize.value = defaultPageSize(props.rowLimit);
});

/** 拖拽表头右缘调整列宽，最小 60px；双击不做处理，清空结果后复位。 */
function startResize(index: number, event: MouseEvent): void {
  event.preventDefault();
  event.stopPropagation();
  const header = (event.currentTarget as HTMLElement | null)?.closest("th");
  const startWidth = header?.offsetWidth ?? 0;
  const startX = event.clientX;
  const move = (moveEvent: MouseEvent) => {
    widths.value = {
      ...widths.value,
      [index]: Math.max(60, startWidth + moveEvent.clientX - startX),
    };
  };
  const up = () => {
    window.removeEventListener("mousemove", move);
    window.removeEventListener("mouseup", up);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  };
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";
  window.addEventListener("mousemove", move);
  window.addEventListener("mouseup", up);
}

function cellClass(value: Cell): string {
  return value === null || value === undefined ? "null" : "";
}

function copyAll(kind: "csv" | "tsv" | "markdown"): void {
  const result = props.result;
  if (!result) return;
  const table = visibleTable();
  const text =
    kind === "csv"
      ? toCsv(table.columns, table.rows)
      : kind === "tsv"
        ? toTsv(table.columns, table.rows)
        : toMarkdown(table.columns, table.rows);
  const label = t(
    kind === "csv" ? "result.copyCsv" : kind === "tsv" ? "result.copyTsv" : "result.copyMarkdown",
  );
  emit("copy", text, label);
}

/**
 * 导出当前结果页签的全部行（忽略前端分页），落盘交给 sidecar。
 * 每次弹系统「另存为」，默认目录用上次保存位置；
 * 结果过大时 sidecar 的 8MB 消息上限会撑爆，因此构建后先按 5MB 拦截。
 */
async function exportExcel(): Promise<void> {
  const result = props.result;
  const table = visibleTable();
  if (!result || table.columns.length === 0 || exporting.value) return;
  exporting.value = true;
  try {
    const bytes = buildXlsx(table);
    const saved = await saveBinaryFile(bytes, exportFilename(), {
      mime: XLSX_MIME,
      askPath: true,
      initialDir: props.exportDir,
    });
    if (saved.status === "saved") {
      const dir = parentDir(saved.path);
      if (dir) emit("saveDir", dir);
      emit("toast", t("result.exported", { path: saved.path }));
    } else if (saved.status === "browser") {
      emit("toast", t("result.exportStarted"));
    }
  } catch (issue) {
    if (issue instanceof XlsxTooLargeError) {
      emit(
        "toast",
        t("result.exportTooLarge", {
          size: (issue.bytes / (1024 * 1024)).toFixed(1),
          rows: formatNumber(table.rows.length),
          cols: table.columns.length,
        }),
        true,
      );
      return;
    }
    emit("toast", `${t("result.exportFailed")}：${describeError(issue)}`, true);
  } finally {
    exporting.value = false;
  }
}

/** 页码只在客户端切片内跳转，不重新查询。 */
function goTo(target: number): void {
  if (target < 1 || target > sliced.value.pageCount || target === page.value) return;
  page.value = target;
}

function changePageSize(): void {
  page.value = 1;
}
</script>

<template>
  <div class="panel-body">
    <div v-if="result || loading || error || info" class="panel-toolbar">
      <template v-if="result">
        <span>{{ t("result.rows", { n: formatNumber(result.rowCount) }) }}</span>
        <span class="subtle">·</span>
        <span>{{ t("result.elapsed", { time: formatElapsed(result.elapsedMs) }) }}</span>
        <span v-if="result.affectedRows > 0" class="subtle">
          · {{ t("result.affected", { n: formatNumber(result.affectedRows) }) }}
        </span>
        <span v-if="result.masked" class="badge warn">{{ t("result.masked") }}</span>
        <span v-if="result.secondsBehindMaster" class="badge">
          {{ t("result.replicaDelay", { value: result.secondsBehindMaster }) }}
        </span>
        <span v-if="result.warning" class="badge warn" :title="result.warning">
          {{ t("result.warning") }}
        </span>
        <span v-if="info" class="subtle" :title="info">{{ info }}</span>
      </template>
      <span class="spacer"></span>
      <template v-if="result && result.columns.length > 0">
        <div class="result-search-group">
          <div class="result-search-wrap">
            <NavIcon name="search" :size="13" class="result-search-icon" />
            <input
              ref="searchInput"
              v-model="search"
              type="search"
              class="result-search"
              spellcheck="false"
              :placeholder="t('result.searchPlaceholder')"
              :aria-label="t('result.searchPlaceholder')"
              :title="t('result.searchHint')"
              @keydown.esc="clearSearch"
            />
            <button
              v-if="search"
              type="button"
              class="result-search-clear"
              :title="t('result.searchClear')"
              :aria-label="t('result.searchClear')"
              @click="clearSearch"
            >
              <NavIcon name="close" :size="10" />
            </button>
          </div>
          <span
            v-if="searching"
            class="result-search-count"
            :class="{ 'is-empty': noMatches }"
            role="status"
            aria-live="polite"
          >
            {{
              t("result.searchMatches", {
                n: formatNumber(matchedIndexes.length),
                total: formatNumber(result.rows.length),
              })
            }}
          </span>
        </div>
        <button
          class="btn ghost"
          :disabled="exporting || noMatches"
          :title="exportHint"
          @click="exportExcel"
        >
          {{ t("result.exportExcel") }}
        </button>
        <div class="copy-menu-wrap">
          <button
            class="btn ghost"
            :disabled="noMatches"
            :aria-expanded="copyOpen"
            :title="t('result.copyMenuHint')"
            @click="copyOpen = !copyOpen"
          >
            {{ t("result.copy") }}
            <NavIcon name="chevron" :size="11" class="menu-caret" />
          </button>
          <div v-if="copyOpen" class="copy-menu" role="menu">
            <button type="button" role="menuitem" @click="pickCopy('csv')">
              {{ t("result.copyCsv") }}
            </button>
            <button type="button" role="menuitem" @click="pickCopy('tsv')">
              {{ t("result.copyTsv") }}
            </button>
            <button type="button" role="menuitem" @click="pickCopy('markdown')">
              {{ t("result.copyMarkdown") }}
            </button>
          </div>
        </div>
        <div class="column-picker-wrap">
          <button
            ref="pickerButton"
            class="btn ghost"
            :aria-expanded="pickerOpen"
            :title="t('result.columnPickerHint')"
            @click="togglePicker"
          >
            <NavIcon name="columns" :size="12" />
            {{ t("result.columnPicker") }}
          </button>
        </div>
      </template>
    </div>

    <div v-if="loading" class="empty">
      <span class="spinner"></span>
      <span>{{ t("app.loading") }}</span>
    </div>
    <div v-else-if="error" class="error-box">{{ error }}</div>
    <div v-else-if="!result" class="empty result-empty">
      <span class="result-empty-icon">
        <NavIcon name="table" :size="22" />
      </span>
      <strong>{{ t("result.empty") }}</strong>
      <span class="result-empty-hint">{{ t("result.emptyHint") }}</span>
      <span class="result-empty-keys">
        <kbd>Ctrl</kbd>
        <span>+</span>
        <kbd>Enter</kbd>
        <span>{{ t("result.emptyRun") }}</span>
      </span>
    </div>
    <div v-else-if="result.columns.length === 0" class="empty">
      <strong>{{ t("result.noColumns") }}</strong>
      <span>{{ info }}</span>
    </div>

    <template v-else>
      <div class="grid-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th class="row-index">#</th>
              <th
                v-for="index in visibleIdx"
                :key="`${result.columns[index]}-${index}`"
                :style="widths[index] ? { width: `${widths[index]}px` } : undefined"
              >
                {{ result.columns[index] }}
                <span v-if="result.columnTypes?.[index]" class="col-type">
                  {{ result.columnTypes[index] }}
                </span>
                <span class="col-resize" @mousedown="startResize(index, $event)"></span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in sliced.items" :key="entry.index">
              <td
                class="row-index"
                :title="searching ? t('result.originalRow', { n: entry.index + 1 }) : undefined"
              >
                {{ entry.index + 1 }}
              </td>
              <td
                v-for="index in visibleIdx"
                :key="index"
                :class="cellClass(entry.row[index])"
                :title="
                  entry.row[index] === null || entry.row[index] === undefined
                    ? 'NULL'
                    : String(entry.row[index])
                "
              >
                <template v-if="searching">
                  <template
                    v-for="(piece, at) in highlightSegments(formatCell(entry.row[index]), needle)"
                    :key="at"
                  >
                    <mark v-if="piece.match" class="result-match">{{ piece.text }}</mark>
                    <template v-else>{{ piece.text }}</template>
                  </template>
                </template>
                <template v-else>{{ formatCell(entry.row[index]) }}</template>
              </td>
            </tr>
            <tr v-if="noMatches">
              <td class="result-no-match" :colspan="visibleIdx.length + 1">
                {{ t("result.searchNoMatch") }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页沿用查询历史页签的版式：记录区间 + 每页条数 + 页码窗口 -->
      <div class="pager history-pager result-pager">
        <span class="history-pager-info">
          {{
            searching
              ? t("result.searchRange", { from: range.from, to: range.to, total: sliced.total })
              : t("result.recordsRange", { from: range.from, to: range.to, total: sliced.total })
          }}
        </span>
        <label class="history-pager-size">
          <span>{{ t("result.pageSizePrefix") }}</span>
          <select
            v-model.number="pageSize"
            :aria-label="t('result.pageSize')"
            @change="changePageSize"
          >
            <option v-for="size in sizeChoices" :key="size" :value="size">{{ size }}</option>
          </select>
          <span>{{ t("result.pageSizeSuffix") }}</span>
        </label>
        <span class="spacer"></span>
        <div class="history-pages">
          <button
            class="history-page-step"
            :disabled="sliced.page <= 1"
            :title="t('result.prev')"
            :aria-label="t('result.prev')"
            @click="goTo(sliced.page - 1)"
          >
            <NavIcon name="chevron" :size="12" class="history-page-prev" />
          </button>
          <template v-for="(item, index) in pageItems" :key="`${item}-${index}`">
            <span v-if="item === 'gap'" class="history-page-gap" aria-hidden="true">…</span>
            <button
              v-else
              class="history-page-num"
              :class="{ 'is-active': Number(item) === sliced.page }"
              :aria-current="Number(item) === sliced.page ? 'page' : undefined"
              :aria-label="t('result.pageAria', { page: Number(item) })"
              @click="goTo(Number(item))"
            >
              {{ item }}
            </button>
          </template>
          <button
            class="history-page-step"
            :disabled="sliced.page >= sliced.pageCount"
            :title="t('result.next')"
            :aria-label="t('result.next')"
            @click="goTo(sliced.page + 1)"
          >
            <NavIcon name="chevron" :size="12" />
          </button>
        </div>
      </div>
    </template>

    <ColumnPickerPanel
      v-if="pickerOpen && result && result.columns.length > 0"
      :columns="result?.columns ?? []"
      :column-types="result?.columnTypes"
      :visible="visible"
      :anchor="pickerAnchor"
      @toggle="toggleColumn"
      @show-all="showAllColumns"
      @close="pickerOpen = false"
    />
  </div>
</template>
