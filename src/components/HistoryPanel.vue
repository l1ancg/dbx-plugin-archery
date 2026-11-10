<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import {
  costTone,
  facets,
  filterLogs,
  formatCount,
  historyColumns,
  logRowRange,
  pageWindow,
  splitTimestamp,
  sqlKind,
} from "../lib/history";
import type { InstanceItem, QueryLogRow } from "../types";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  instances?: InstanceItem[];
}>();

const emit = defineEmits<{
  load: [row: QueryLogRow];
  toast: [text: string];
}>();

const rows = ref<QueryLogRow[]>([]);
const total = ref(0);
const page = ref(1);
/** 每页条数默认 10 条，切换后回第 1 页重新拉取。 */
const pageSize = ref(10);
const pageSizes = [10, 20, 30, 50, 100];
const keyword = ref("");
const instance = ref("");
const database = ref("");
const starOnly = ref(false);
const dense = ref(false);
const loading = ref(false);
const error = ref("");
const expandedId = ref(0);
let debounce: ReturnType<typeof setTimeout> | undefined;

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const pageItems = computed(() => pageWindow(page.value, pageCount.value));
const range = computed(() => logRowRange(page.value, pageSize.value, total.value));
/** 翻页 / 切每页条数时的后台请求：表格不卸载，只在工具栏转圈提示。 */
const paging = computed(() => loading.value && rows.value.length > 0);
const instanceOptions = computed(() => {
  const names = new Set<string>();
  for (const item of props.instances ?? []) names.add(item.name);
  for (const name of facets(rows.value).instances) names.add(name);
  return [...names].sort();
});
const databaseOptions = computed(() => facets(rows.value).databases);
const visibleRows = computed(() =>
  filterLogs(rows.value, {
    instance: instance.value,
    db: database.value,
    keyword: keyword.value,
  }),
);

/**
 * 列表展示项：语句类型决定左侧色条、耗时决定色阶、时间拆成日期 + 时刻。
 * 都在这里算好，模板只负责摆放，避免每行重复解析。
 */
const viewRows = computed(() =>
  visibleRows.value.map((row) => {
    const when = splitTimestamp(row.createTime);
    return {
      row,
      kind: sqlKind(row.sql),
      date: when.date,
      clock: when.time,
      cost: costTone(row.costTime),
      rowsText: formatCount(row.effectRow),
      initial: (row.userDisplay || "?").trim().charAt(0).toUpperCase(),
    };
  }),
);

async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    const result = await archeryApi.queryLogs({
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
      search: keyword.value.trim(),
      star: starOnly.value,
      instanceName: instance.value,
      dbName: database.value,
    });
    rows.value = result.rows ?? [];
    total.value = result.total ?? 0;
    // 收紧筛选或调小每页条数后总数会缩小，页码越界时退回最后一页再拉一次。
    const lastPage = Math.max(1, Math.ceil(total.value / pageSize.value));
    if (page.value > lastPage) {
      page.value = lastPage;
      await load();
      return;
    }
  } catch (issue) {
    error.value = describeError(issue);
    rows.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

function changeFilter(): void {
  page.value = 1;
  void load();
}

function onSearch(): void {
  if (debounce) clearTimeout(debounce);
  debounce = setTimeout(changeFilter, 300);
}

function toggleStarFilter(): void {
  starOnly.value = !starOnly.value;
  changeFilter();
}

function goTo(target: number): void {
  if (target === page.value || target < 1 || target > pageCount.value) return;
  page.value = target;
  void load();
}

function changePageSize(): void {
  page.value = 1;
  void load();
}

function toggleExpand(row: QueryLogRow): void {
  expandedId.value = expandedId.value === row.id ? 0 : row.id;
}

async function toggleStar(row: QueryLogRow): Promise<void> {
  try {
    await archeryApi.favorite({
      queryLogId: row.id,
      star: !row.favorite,
      alias: row.alias,
    });
    row.favorite = !row.favorite;
    emit("toast", t("history.favoriteDone"));
  } catch (issue) {
    emit("toast", describeError(issue));
  }
}

onMounted(load);
</script>

<template>
  <div class="panel-body history-body">
    <div class="panel-toolbar history-toolbar">
      <select
        v-model="instance"
        class="form-control form-control-sm history-filter"
        @change="changeFilter"
      >
        <option value="">{{ t("history.filterAll") }}</option>
        <option v-for="name in instanceOptions" :key="name" :value="name">{{ name }}</option>
      </select>
      <select
        v-model="database"
        class="form-control form-control-sm history-filter"
        @change="changeFilter"
      >
        <option value="">{{ t("history.filterAll") }}</option>
        <option v-for="name in databaseOptions" :key="name" :value="name">{{ name }}</option>
      </select>
      <button
        class="tool-btn"
        :class="{ 'is-on': starOnly }"
        :title="t('history.onlyStar')"
        @click="toggleStarFilter"
      >
        <NavIcon name="star" :size="13" />
      </button>
      <span class="spacer"></span>
      <input
        v-model="keyword"
        class="form-control form-control-sm history-search"
        type="search"
        :placeholder="t('history.searchPlaceholder')"
        @input="onSearch"
      />
      <span
        class="spinner history-paging"
        :class="{ 'is-idle': !paging }"
        :aria-hidden="!paging"
        :title="paging ? t('app.loading') : undefined"
      ></span>
      <button class="tool-btn" :title="t('app.refresh')" @click="load">
        <NavIcon name="refresh" :size="14" />
      </button>
      <button
        class="tool-btn"
        :class="{ 'is-on': dense }"
        :title="t('history.dense')"
        @click="dense = !dense"
      >
        <NavIcon name="table" :size="14" />
      </button>
    </div>

    <div v-if="error" class="error-box">{{ error }}</div>
    <!-- 翻页时保留表格挂载：内容高度不塌陷，滚动位置才不会被浏览器夹回顶部 -->
    <div v-else-if="loading && rows.length === 0" class="empty">
      <span class="spinner"></span>
      <span>{{ t("app.loading") }}</span>
    </div>
    <div v-else-if="visibleRows.length === 0" class="empty history-empty">
      <span class="history-empty-icon">
        <NavIcon name="search" :size="20" />
      </span>
      <strong>{{ t("history.empty") }}</strong>
      <span class="history-empty-hint">{{ t("history.emptyHint") }}</span>
    </div>

    <template v-else>
      <div class="grid-wrap">
        <table class="data-table history-table" :class="{ compact: dense }">
          <thead>
            <tr>
              <th v-for="key in historyColumns" :key="key">{{ t(key) }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="(item, index) in viewRows" :key="item.row.id">
              <tr
                class="history-row"
                :class="[
                  `is-${item.kind}`,
                  { 'is-alt': index % 2 === 1, 'is-expanded': expandedId === item.row.id },
                ]"
              >
                <td class="history-ops">
                  <button
                    class="op-btn"
                    :title="t('history.expand')"
                    @click="toggleExpand(item.row)"
                  >
                    <NavIcon
                      :name="expandedId === item.row.id ? 'minus' : 'plus'"
                      :size="12"
                    />
                  </button>
                  <button
                    class="op-btn"
                    :class="{ starred: item.row.favorite }"
                    :title="item.row.favorite ? t('history.unstar') : t('history.star')"
                    @click="toggleStar(item.row)"
                  >
                    <NavIcon name="star" :size="12" />
                  </button>
                  <button class="op-btn" :title="t('history.load')" @click="emit('load', item.row)">
                    <NavIcon name="eye" :size="12" />
                  </button>
                </td>
                <td>
                  <div class="history-user">
                    <span class="history-avatar" aria-hidden="true">{{ item.initial }}</span>
                    <span class="history-user-name" :title="item.row.userDisplay">
                      {{ item.row.userDisplay || "-" }}
                    </span>
                  </div>
                </td>
                <td class="history-target" :title="item.row.instanceName">
                  {{ item.row.alias || item.row.instanceName }}
                </td>
                <td class="history-db-cell">
                  <span class="history-db" :title="item.row.dbName">{{ item.row.dbName || "-" }}</span>
                </td>
                <td class="history-when">
                  <span class="history-date">{{ item.date || "-" }}</span>
                  <span v-if="item.clock" class="history-clock">{{ item.clock }}</span>
                </td>
                <td class="history-sql-cell" :title="item.row.sql">
                  <div class="history-sql">
                    <span
                      class="history-kind-mark"
                      :title="t(`history.kind.${item.kind}`)"
                      aria-hidden="true"
                    ></span>
                    <span class="history-sql-text">{{ item.row.sql }}</span>
                  </div>
                </td>
                <td class="history-count">{{ item.rowsText }}</td>
                <td class="history-cost" :class="`is-${item.cost}`">
                  <span class="history-cost-value">{{ item.row.costTime }}</span>
                  <span class="history-cost-unit">s</span>
                </td>
              </tr>
              <tr v-if="expandedId === item.row.id" class="history-detail">
                <td colspan="8">
                  <div class="history-detail-card">
                    <div class="history-detail-meta">
                      <span class="history-meta-item">
                        <span class="history-meta-label">{{ t("history.colInstance") }}</span>
                        <span class="history-meta-value">{{ item.row.instanceName || "-" }}</span>
                      </span>
                      <span class="history-meta-item">
                        <span class="history-meta-label">{{ t("history.alias") }}</span>
                        <span class="history-meta-value">{{ item.row.alias || "-" }}</span>
                      </span>
                      <span class="history-meta-item">
                        <span class="history-meta-label">{{ t("history.colCost") }}</span>
                        <span class="history-meta-value" :class="`is-${item.cost}`">
                          {{ item.row.costTime }}
                        </span>
                      </span>
                      <span class="history-meta-item">
                        <span class="history-meta-label">{{ t("history.colEffectRow") }}</span>
                        <span class="history-meta-value">{{ item.rowsText }}</span>
                      </span>
                      <span class="history-meta-item">
                        <span class="history-meta-label">{{ t("history.colTime") }}</span>
                        <span class="history-meta-value">{{ item.row.createTime }}</span>
                      </span>
                    </div>
                    <div class="history-detail-sql">
                      <span class="history-kind-badge" :class="`is-${item.kind}`">
                        {{ t(`history.kind.${item.kind}`) }}
                      </span>
                      <pre class="history-sql-full">{{ item.row.sql }}</pre>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <div class="pager history-pager">
        <span class="history-pager-info">
          {{ t("history.recordsRange", { from: range.from, to: range.to, total }) }}
        </span>
        <label class="history-pager-size">
          <span>{{ t("history.pageSizePrefix") }}</span>
          <select
            v-model.number="pageSize"
            :aria-label="t('result.pageSize')"
            @change="changePageSize"
          >
            <option v-for="size in pageSizes" :key="size" :value="size">{{ size }}</option>
          </select>
          <span>{{ t("history.pageSizeSuffix") }}</span>
        </label>
        <span class="spacer"></span>
        <div class="history-pages">
          <button
            class="history-page-step"
            :disabled="page <= 1"
            :title="t('result.prev')"
            :aria-label="t('result.prev')"
            @click="goTo(page - 1)"
          >
            <NavIcon name="chevron" :size="12" class="history-page-prev" />
          </button>
          <template v-for="(item, index) in pageItems" :key="`${item}-${index}`">
            <span v-if="item === 'gap'" class="history-page-gap" aria-hidden="true">…</span>
            <button
              v-else
              class="history-page-num"
              :class="{ 'is-active': Number(item) === page }"
              :aria-current="Number(item) === page ? 'page' : undefined"
              :aria-label="t('history.pageAria', { page: Number(item) })"
              @click="goTo(Number(item))"
            >
              {{ item }}
            </button>
          </template>
          <button
            class="history-page-step"
            :disabled="page >= pageCount"
            :title="t('result.next')"
            :aria-label="t('result.next')"
            @click="goTo(page + 1)"
          >
            <NavIcon name="chevron" :size="12" />
          </button>
        </div>
      </div>
    </template>
  </div>
</template>
