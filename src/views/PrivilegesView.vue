<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { writeClipboard } from "../lib/clipboard";
import { toISODate } from "../lib/date-range";
import { saveTextFile } from "../lib/download";
import { describeError } from "../lib/errors";
import {
  canAuditPrivilege,
  filterUserPrivileges,
  normalizePrivilegeApply,
  normalizeUserPrivilege,
  privilegeCopyText,
  privilegeExpiry,
  privilegeLevelKey,
  privilegeStatusMeta,
  sortUserPrivileges,
  splitPrivilegeList,
  summarizeUserPrivileges,
} from "../lib/privilege";
import { toCsv } from "../lib/result";
import type { PrivilegeExpiry, PrivilegeExpiryState, UserPrivilegeSortKey } from "../lib/privilege";
import type { PrivilegeApplyRow, UserPrivilegeRow } from "../types";
import NavIcon from "../components/NavIcon.vue";
import PrivilegeApplyDialog from "../components/PrivilegeApplyDialog.vue";
import PrivilegeAuditDialog from "../components/PrivilegeAuditDialog.vue";
import WorkflowLogDialog from "../components/WorkflowLogDialog.vue";
import PrivilegeDetailView from "./PrivilegeDetailView.vue";

const emit = defineEmits<{
  toast: [text: string, error?: boolean];
}>();

const PAGE_SIZE = 20;

/** 页签：我的权限（当前账号的有效权限）与申请工单（工单列表）。进入页面默认选中申请工单。 */
type PrivilegeTab = "mine" | "applies";
const tab = ref<PrivilegeTab>("applies");

// ---- 我的权限 ----
/** 服务端上限 200 条；在该范围内做客户端筛选、排序、汇总与分页。 */
const MINE_FETCH_LIMIT = 200;
const mineAllRows = ref<UserPrivilegeRow[]>([]);
const mineTotal = ref(0);
const minePage = ref(1);
const mineKeyword = ref("");
const mineInstance = ref("");
const mineLevel = ref<number | "">("");
const mineExpiry = ref<PrivilegeExpiryState | "">("");
const mineSortKey = ref<UserPrivilegeSortKey>("instance");
const mineSortDir = ref<"asc" | "desc">("asc");
const mineLoading = ref(false);
const mineError = ref("");
/** 到期状态按加载时刻计算，避免每次渲染都用新的 Date。 */
const mineToday = ref(new Date());

// ---- 申请工单 ----
const rows = ref<PrivilegeApplyRow[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref("");
const dense = ref(false);
const loading = ref(false);
const error = ref("");
const expandedId = ref(0);
const applyOpen = ref(false);
const auditRow = ref<PrivilegeApplyRow | null>(null);
const logRow = ref<PrivilegeApplyRow | null>(null);
/** 非空时进入工单详情页，返回后重新拉列表。 */
const detailRow = ref<PrivilegeApplyRow | null>(null);
let mineDebounce: ReturnType<typeof setTimeout> | undefined;
let debounce: ReturnType<typeof setTimeout> | undefined;

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)));

const mineFiltered = computed(() =>
  sortUserPrivileges(
    filterUserPrivileges(
      mineAllRows.value,
      {
        keyword: mineKeyword.value,
        instanceName: mineInstance.value,
        privType: mineLevel.value,
        expiry: mineExpiry.value,
      },
      mineToday.value,
    ),
    mineSortKey.value,
    mineSortDir.value,
    mineToday.value,
  ),
);
const minePageCount = computed(() =>
  Math.max(1, Math.ceil(mineFiltered.value.length / PAGE_SIZE)),
);
const mineRows = computed(() =>
  mineFiltered.value.slice((minePage.value - 1) * PAGE_SIZE, minePage.value * PAGE_SIZE),
);
const mineSummary = computed(() => summarizeUserPrivileges(mineFiltered.value, mineToday.value));
const mineInstanceOptions = computed(() =>
  [...new Set(mineAllRows.value.map((row) => row.instanceName.trim()).filter(Boolean))].sort(
    (a, b) => a.localeCompare(b),
  ),
);
const mineFilteredActive = computed(
  () =>
    mineKeyword.value.trim() !== "" ||
    mineInstance.value !== "" ||
    mineLevel.value !== "" ||
    mineExpiry.value !== "",
);
/** 服务端总数超过本次加载条数时提示搜索收窄。 */
const mineTruncated = computed(() => mineTotal.value > mineAllRows.value.length);

function shorten(value: string, max: number): string {
  return value.length > max ? `${value.slice(0, max)}...` : value;
}

/** 结果集行数加千分位，量级一眼可比。 */
function formatLimit(value: number): string {
  if (!Number.isFinite(value)) return "-";
  return String(Math.trunc(value)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/** 申请时间拆成日期与时刻两行，窄列也读得清。 */
function datePart(value: string): string {
  return value.trim().split(/\s+/)[0] ?? "";
}

function timePart(value: string): string {
  return value.trim().split(/\s+/)[1] ?? "";
}

/** 展开行里的清单：表级工单取表名，库级工单取库名。 */
function listItems(row: PrivilegeApplyRow): string[] {
  return splitPrivilegeList(row.privType === 2 ? row.tableList : row.dbList);
}

async function loadMine(): Promise<void> {
  mineLoading.value = true;
  mineError.value = "";
  mineToday.value = new Date();
  try {
    const result = await archeryApi.myPrivileges({
      limit: MINE_FETCH_LIMIT,
      offset: 0,
      search: mineKeyword.value.trim(),
    });
    mineAllRows.value = (result.rows ?? []).map((row) =>
      normalizeUserPrivilege(row as unknown as Record<string, unknown>),
    );
    mineTotal.value = result.total ?? 0;
    if (minePage.value > minePageCount.value) minePage.value = minePageCount.value;
  } catch (issue) {
    mineError.value = describeError(issue);
    mineAllRows.value = [];
    mineTotal.value = 0;
  } finally {
    mineLoading.value = false;
  }
}

async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    const result = await archeryApi.privileges({
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE,
      search: keyword.value.trim(),
    });
    rows.value = (result.rows ?? []).map((row) =>
      normalizePrivilegeApply(row as unknown as Record<string, unknown>),
    );
    total.value = result.total ?? 0;
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

function onMineSearch(): void {
  if (mineDebounce) clearTimeout(mineDebounce);
  mineDebounce = setTimeout(() => {
    minePage.value = 1;
    void loadMine();
  }, 300);
}

/** 实例/级别/到期状态都是客户端筛选，不重新请求。 */
function changeMineFilter(): void {
  minePage.value = 1;
}

function clearMineFilters(): void {
  const hadKeyword = mineKeyword.value.trim() !== "";
  // 取消尚未触发的搜索防抖，避免清空后又被旧关键字拉一次。
  if (mineDebounce) clearTimeout(mineDebounce);
  mineKeyword.value = "";
  mineInstance.value = "";
  mineLevel.value = "";
  mineExpiry.value = "";
  minePage.value = 1;
  // 关键字参与服务端搜索，清空后需要重新拉一次全量。
  if (hadKeyword) void loadMine();
}

function toggleMineSort(key: UserPrivilegeSortKey): void {
  if (mineSortKey.value === key) {
    mineSortDir.value = mineSortDir.value === "asc" ? "desc" : "asc";
  } else {
    mineSortKey.value = key;
    mineSortDir.value = "asc";
  }
  minePage.value = 1;
}

function sortMark(key: UserPrivilegeSortKey): string {
  if (mineSortKey.value !== key) return "";
  return mineSortDir.value === "asc" ? "↑" : "↓";
}

function expiryOf(row: UserPrivilegeRow): PrivilegeExpiry {
  return privilegeExpiry(row.validDate, mineToday.value);
}

function expiryTone(expiry: PrivilegeExpiry): "ok" | "urgent" | "expired" | "unknown" {
  switch (expiry.state) {
    case "active":
      return "ok";
    case "soon":
      return "urgent";
    case "expired":
      return "expired";
    default:
      return "unknown";
  }
}

function expiryLabel(expiry: PrivilegeExpiry): string {
  switch (expiry.state) {
    case "active":
      return t("privilege.expiry.active");
    case "soon":
      return t("privilege.expiry.soon", { n: expiry.days });
    case "expired":
      return t("privilege.expiry.expired");
    default:
      return t("privilege.expiry.unknown");
  }
}

async function copyMineRow(row: UserPrivilegeRow): Promise<void> {
  const text = privilegeCopyText(row);
  if (!text) return;
  const copied = await writeClipboard(text);
  emit("toast", copied ? t("app.copied") : t("privilege.copyFailed"), !copied);
}

/** 导出当前筛选结果；CSV 加 BOM，Excel 打开不乱码。 */
async function exportMine(): Promise<void> {
  if (mineFiltered.value.length === 0) return;
  const columns = [
    t("privilege.colUser"),
    t("privilege.colInstance"),
    t("privilege.colDatabase"),
    t("privilege.colPrivType"),
    t("privilege.colTable"),
    t("privilege.colLimit"),
    t("privilege.colValidDate"),
  ];
  const csv = toCsv(
    columns,
    mineFiltered.value.map((row) => [
      row.userDisplay,
      row.instanceName,
      row.dbName,
      t(privilegeLevelKey(row.privType)),
      row.tableName,
      row.limitNum,
      row.validDate,
    ]),
  );
  const result = await saveTextFile(`\uFEFF${csv}`, `privileges-${toISODate(new Date())}.csv`, {
    mime: "text/csv;charset=utf-8",
  });
  if (result.status === "saved") {
    emit("toast", t("privilege.exported", { path: result.path }));
  } else if (result.status === "browser") {
    emit("toast", t("privilege.exportStarted"));
  }
}

function go(delta: number): void {
  const next = page.value + delta;
  if (next < 1 || next > pageCount.value) return;
  page.value = next;
  void load();
}

function goMine(delta: number): void {
  const next = minePage.value + delta;
  if (next < 1 || next > minePageCount.value) return;
  minePage.value = next;
}

function switchTab(next: PrivilegeTab): void {
  if (tab.value === next) return;
  tab.value = next;
  if (next === "mine" && mineAllRows.value.length === 0) void loadMine();
  if (next === "applies" && rows.value.length === 0) void load();
}

/** 刷新按钮跟随当前页签。 */
function refresh(): void {
  if (tab.value === "mine") void loadMine();
  else void load();
}

function toggleExpand(row: PrivilegeApplyRow): void {
  expandedId.value = expandedId.value === row.applyId ? 0 : row.applyId;
}

function onSubmitted(): void {
  applyOpen.value = false;
  emit("toast", t("privilege.submitted"));
  tab.value = "applies";
  page.value = 1;
  void load();
}

function onAudited(): void {
  auditRow.value = null;
  void load();
}

/** 详情页审核成功后回到列表并刷新状态。 */
function onDetailAudited(): void {
  detailRow.value = null;
  void load();
}

onMounted(load);
</script>

<template>
  <PrivilegeDetailView
    v-if="detailRow"
    :row="detailRow"
    @back="detailRow = null"
    @audited="onDetailAudited"
    @toast="(text) => emit('toast', text)"
  />
  <div v-else class="page page-fill privilege-page">
    <header class="privilege-head">
      <h2 class="page-title">{{ t("nav.queryGrant") }}</h2>
      <p class="privilege-head-desc">{{ t("privilege.headDesc") }}</p>
    </header>

    <section class="panel card-fill privilege-panel">
      <div class="panel-heading privilege-toolbar">
        <div class="privilege-tabs" role="tablist">
          <button
            class="btn btn-default btn-xs"
            role="tab"
            :aria-selected="tab === 'mine'"
            :class="{ 'is-on': tab === 'mine' }"
            @click="switchTab('mine')"
          >
            <span>{{ t("privilege.mine") }}</span>
            <span v-if="mineAllRows.length > 0" class="privilege-tab-count">
              {{ mineAllRows.length }}
            </span>
          </button>
          <button
            class="btn btn-default btn-xs"
            role="tab"
            :aria-selected="tab === 'applies'"
            :class="{ 'is-on': tab === 'applies' }"
            @click="switchTab('applies')"
          >
            <span>{{ t("privilege.applies") }}</span>
            <span v-if="total > 0" class="privilege-tab-count">{{ total }}</span>
          </button>
        </div>

        <span class="privilege-toolbar-sep" aria-hidden="true"></span>
        <span class="spacer"></span>

        <label class="privilege-search-box">
          <NavIcon name="search" :size="13" class="privilege-search-icon" />
          <input
            v-if="tab === 'mine'"
            v-model="mineKeyword"
            class="form-control form-control-sm privilege-search"
            type="search"
            :aria-label="t('privilege.mineSearchPlaceholder')"
            :placeholder="t('privilege.mineSearchPlaceholder')"
            @input="onMineSearch"
          />
          <input
            v-else
            v-model="keyword"
            class="form-control form-control-sm privilege-search"
            type="search"
            :aria-label="t('privilege.searchPlaceholder')"
            :placeholder="t('privilege.searchPlaceholder')"
            @input="onSearch"
          />
        </label>

        <button class="tool-btn" :title="t('app.refresh')" @click="refresh">
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
        <button class="btn btn-success btn-xs privilege-apply-btn" @click="applyOpen = true">
          <NavIcon name="plus" :size="12" />
          <span>{{ t("privilege.apply") }}</span>
        </button>
      </div>

      <div class="panel-body privilege-list-body">
        <template v-if="tab === 'mine'">
          <div v-if="mineError" class="error-box">
            <span>{{ t("privilege.loadFailed") }}：{{ mineError }}</span>
            <button class="btn btn-default btn-xs" @click="loadMine">
              {{ t("privilege.retry") }}
            </button>
          </div>
          <div v-else-if="mineLoading" class="empty">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="mineAllRows.length === 0" class="empty privilege-empty">
            <span class="privilege-empty-icon">
              <NavIcon name="user" :size="22" />
            </span>
            <strong>{{ t("privilege.mineEmpty") }}</strong>
            <span class="privilege-empty-hint">{{ t("privilege.emptyHint") }}</span>
          </div>

          <template v-else>
            <div class="privilege-mine-summary">
              <div class="privilege-summary-metrics">
                <span class="privilege-summary-item is-ok">
                  <span class="privilege-summary-dot"></span>
                  <span class="privilege-summary-label">{{ t("privilege.summary.active") }}</span>
                  <strong>{{ mineSummary.active }}</strong>
                </span>
                <span class="privilege-summary-item is-urgent">
                  <span class="privilege-summary-dot"></span>
                  <span class="privilege-summary-label">{{ t("privilege.summary.soon") }}</span>
                  <strong>{{ mineSummary.soon }}</strong>
                </span>
                <span class="privilege-summary-item is-expired">
                  <span class="privilege-summary-dot"></span>
                  <span class="privilege-summary-label">{{ t("privilege.summary.expired") }}</span>
                  <strong>{{ mineSummary.expired }}</strong>
                </span>
                <span v-if="mineSummary.unknown > 0" class="privilege-summary-item is-unknown">
                  <span class="privilege-summary-dot"></span>
                  <span class="privilege-summary-label">{{ t("privilege.summary.unknown") }}</span>
                  <strong>{{ mineSummary.unknown }}</strong>
                </span>
                <span class="privilege-summary-divider"></span>
                <span class="privilege-summary-item">
                  <span class="privilege-summary-label">{{ t("privilege.summary.instances") }}</span>
                  <strong>{{ mineSummary.instances }}</strong>
                </span>
              </div>

              <span class="spacer"></span>
              <span v-if="mineFilteredActive" class="subtle privilege-summary-count">
                {{ t("privilege.recordsFiltered", { shown: mineFiltered.length, total: mineTotal }) }}
              </span>

              <!-- 到期构成：一条极细的比例条，扫一眼就知道权限有没有在悄悄过期 -->
              <div class="privilege-health" aria-hidden="true">
                <span
                  v-if="mineSummary.active > 0"
                  class="is-ok"
                  :style="{ flexGrow: mineSummary.active }"
                ></span>
                <span
                  v-if="mineSummary.soon > 0"
                  class="is-urgent"
                  :style="{ flexGrow: mineSummary.soon }"
                ></span>
                <span
                  v-if="mineSummary.expired > 0"
                  class="is-expired"
                  :style="{ flexGrow: mineSummary.expired }"
                ></span>
                <span
                  v-if="mineSummary.unknown > 0"
                  class="is-unknown"
                  :style="{ flexGrow: mineSummary.unknown }"
                ></span>
              </div>
            </div>

            <div v-if="mineTruncated" class="privilege-truncated">
              <NavIcon name="info" :size="13" />
              <span>{{ t("privilege.truncated", { total: mineTotal, loaded: mineAllRows.length }) }}</span>
            </div>

            <div class="privilege-filters">
              <span class="privilege-filters-label">{{ t("privilege.filterLabel") }}</span>
              <select
                v-model="mineInstance"
                class="form-control form-control-sm"
                :aria-label="t('privilege.colInstance')"
                @change="changeMineFilter"
              >
                <option value="">{{ t("privilege.filterInstance") }}</option>
                <option v-for="name in mineInstanceOptions" :key="name" :value="name">
                  {{ name }}
                </option>
              </select>
              <select
                v-model.number="mineLevel"
                class="form-control form-control-sm"
                :aria-label="t('privilege.colPrivType')"
                @change="changeMineFilter"
              >
                <option value="">{{ t("privilege.filterLevel") }}</option>
                <option :value="1">{{ t("privilege.level.database") }}</option>
                <option :value="2">{{ t("privilege.level.table") }}</option>
              </select>
              <select
                v-model="mineExpiry"
                class="form-control form-control-sm"
                :aria-label="t('privilege.colValidDate')"
                @change="changeMineFilter"
              >
                <option value="">{{ t("privilege.filterExpiry") }}</option>
                <option value="active">{{ t("privilege.expiry.active") }}</option>
                <option value="soon">{{ t("privilege.filterExpirySoon") }}</option>
                <option value="expired">{{ t("privilege.expiry.expired") }}</option>
              </select>
              <button
                v-if="mineFilteredActive"
                class="btn btn-quiet btn-xs privilege-filter-clear"
                @click="clearMineFilters"
              >
                {{ t("privilege.filterClear") }}
              </button>
              <span class="spacer"></span>
              <button
                class="btn btn-default btn-xs privilege-export-btn"
                :disabled="mineFiltered.length === 0"
                @click="exportMine"
              >
                <NavIcon name="file" :size="12" />
                <span>{{ t("privilege.exportCsv") }}</span>
              </button>
            </div>

            <div v-if="mineFiltered.length === 0" class="empty">
              <span>{{ t("privilege.mineEmptyFiltered") }}</span>
              <button class="btn btn-default btn-xs" @click="clearMineFilters">
                {{ t("privilege.filterClear") }}
              </button>
            </div>

            <template v-else>
              <div class="grid-wrap">
                <table
                  class="data-table privilege-table privilege-mine-table"
                  :class="{ compact: dense }"
                >
                  <thead>
                    <tr>
                      <th>{{ t("privilege.colUser") }}</th>
                      <th
                        class="is-sortable"
                        data-sort="instance"
                        :aria-sort="
                          mineSortKey === 'instance'
                            ? mineSortDir === 'asc'
                              ? 'ascending'
                              : 'descending'
                            : 'none'
                        "
                        @click="toggleMineSort('instance')"
                      >
                        {{ t("privilege.colInstance") }}
                        <span class="privilege-sort-mark">{{ sortMark("instance") }}</span>
                      </th>
                      <th
                        class="is-sortable"
                        data-sort="database"
                        :aria-sort="
                          mineSortKey === 'database'
                            ? mineSortDir === 'asc'
                              ? 'ascending'
                              : 'descending'
                            : 'none'
                        "
                        @click="toggleMineSort('database')"
                      >
                        {{ t("privilege.colDatabase") }}
                        <span class="privilege-sort-mark">{{ sortMark("database") }}</span>
                      </th>
                      <th>{{ t("privilege.colPrivType") }}</th>
                      <th>{{ t("privilege.colTable") }}</th>
                      <th class="is-num">{{ t("privilege.colLimit") }}</th>
                      <th
                        class="is-sortable"
                        data-sort="expiry"
                        :aria-sort="
                          mineSortKey === 'expiry'
                            ? mineSortDir === 'asc'
                              ? 'ascending'
                              : 'descending'
                            : 'none'
                        "
                        @click="toggleMineSort('expiry')"
                      >
                        {{ t("privilege.colValidDate") }}
                        <span class="privilege-sort-mark">{{ sortMark("expiry") }}</span>
                      </th>
                      <th>{{ t("privilege.colOperation") }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="row in mineRows"
                      :key="row.privilegeId"
                      :class="{
                        'is-expired': expiryOf(row).state === 'expired',
                        'is-soon': expiryOf(row).state === 'soon',
                      }"
                    >
                      <td class="privilege-user-cell" :title="row.userDisplay">
                        {{ row.userDisplay || "-" }}
                      </td>
                      <td class="privilege-ident" :title="row.instanceName">
                        {{ row.instanceName }}
                      </td>
                      <td class="privilege-ident is-key" :title="row.dbName">
                        {{ row.dbName || "-" }}
                      </td>
                      <td>
                        <span
                          class="privilege-level"
                          :class="row.privType === 1 ? 'is-database' : 'is-table'"
                        >
                          {{ t(privilegeLevelKey(row.privType)) }}
                        </span>
                      </td>
                      <td class="privilege-ident" :title="row.tableName">
                        {{ row.tableName || "-" }}
                      </td>
                      <td class="privilege-num">{{ formatLimit(row.limitNum) }}</td>
                      <td class="nowrap privilege-valid">
                        <span class="privilege-valid-date">{{ row.validDate || "-" }}</span>
                        <span
                          class="privilege-expiry"
                          :class="`is-${expiryTone(expiryOf(row))}`"
                        >
                          {{ expiryLabel(expiryOf(row)) }}
                        </span>
                      </td>
                      <td class="privilege-ops">
                        <button
                          class="btn btn-quiet btn-xs privilege-copy-btn"
                          :title="t('privilege.copyName')"
                          @click="copyMineRow(row)"
                        >
                          <NavIcon name="copy" :size="12" />
                          <span>{{ t("app.copy") }}</span>
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>

              <div class="pager privilege-pager">
                <span class="privilege-pager-count">
                  {{ t("privilege.records", { n: mineFiltered.length }) }}
                </span>
                <span class="spacer"></span>
                <span class="privilege-page-indicator">
                  {{ t("result.page", { page: minePage, pages: minePageCount }) }}
                </span>
                <button
                  class="btn ghost privilege-page-btn"
                  :disabled="minePage <= 1"
                  @click="goMine(-1)"
                >
                  <NavIcon name="chevron" :size="12" class="privilege-page-prev" />
                  {{ t("result.prev") }}
                </button>
                <button
                  class="btn ghost privilege-page-btn"
                  :disabled="minePage >= minePageCount"
                  @click="goMine(1)"
                >
                  {{ t("result.next") }}
                  <NavIcon name="chevron" :size="12" />
                </button>
              </div>
            </template>
          </template>
        </template>

        <template v-else>
          <div v-if="error" class="error-box">{{ t("privilege.loadFailed") }}：{{ error }}</div>
          <div v-else-if="loading" class="empty">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="rows.length === 0" class="empty privilege-empty">
            <span class="privilege-empty-icon">
              <NavIcon name="file" :size="22" />
            </span>
            <strong>{{ t("privilege.empty") }}</strong>
            <span class="privilege-empty-hint">{{ t("privilege.emptyHint") }}</span>
          </div>

          <template v-else>
            <div class="grid-wrap">
              <table
                class="data-table privilege-table privilege-apply-table"
                :class="{ compact: dense }"
              >
                <thead>
                  <tr>
                    <th>{{ t("privilege.colTitle") }}</th>
                    <th>{{ t("privilege.colInstance") }}</th>
                    <th>{{ t("privilege.colDatabase") }}</th>
                    <th>{{ t("privilege.colPrivType") }}</th>
                    <th class="is-num">{{ t("privilege.colLimit") }}</th>
                    <th>{{ t("privilege.colValidDate") }}</th>
                    <th>{{ t("privilege.colStatus") }}</th>
                    <th>{{ t("privilege.colApplicant") }}</th>
                    <th>{{ t("privilege.colCreateTime") }}</th>
                    <th>{{ t("privilege.colGroup") }}</th>
                    <th>{{ t("privilege.colOperation") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <template v-for="row in rows" :key="row.applyId">
                    <tr>
                      <td class="privilege-title">
                        <button
                          class="op-btn privilege-caret"
                          :title="
                            expandedId === row.applyId
                              ? t('privilege.collapse')
                              : t('privilege.expand')
                          "
                          @click="toggleExpand(row)"
                        >
                          <NavIcon
                            :name="expandedId === row.applyId ? 'minus' : 'plus'"
                            :size="12"
                          />
                        </button>
                        <button
                          class="privilege-title-link"
                          :title="row.title"
                          @click="detailRow = row"
                        >
                          {{ row.title }}
                        </button>
                      </td>
                      <td class="privilege-ident" :title="row.instanceName">
                        {{ row.instanceName }}
                      </td>
                      <td class="privilege-ident is-key" :title="row.dbList">
                        {{ shorten(row.dbList, 30) }}
                      </td>
                      <td>
                        <span
                          class="privilege-level"
                          :class="row.privType === 1 ? 'is-database' : 'is-table'"
                        >
                          {{ t(privilegeLevelKey(row.privType)) }}
                        </span>
                      </td>
                      <td class="privilege-num">{{ formatLimit(row.limitNum) }}</td>
                      <td class="nowrap privilege-valid-date">{{ row.validDate }}</td>
                      <td>
                        <span
                          class="privilege-status"
                          :class="`is-${privilegeStatusMeta(row.status).tone}`"
                        >
                          {{ t(privilegeStatusMeta(row.status).labelKey) }}
                        </span>
                      </td>
                      <td class="privilege-user-cell" :title="row.userDisplay">
                        {{ row.userDisplay }}
                      </td>
                      <td class="nowrap" :title="row.createTime">
                        <span class="privilege-when">
                          <span class="privilege-when-date">{{ datePart(row.createTime) }}</span>
                          <span v-if="timePart(row.createTime)" class="privilege-when-time">
                            {{ timePart(row.createTime) }}
                          </span>
                        </span>
                      </td>
                      <td>
                        <span class="privilege-group" :title="row.groupName">
                          {{ row.groupName }}
                        </span>
                      </td>
                      <td class="privilege-ops">
                        <button class="btn btn-quiet btn-xs privilege-log-btn" @click="logRow = row">
                          {{ t("privilege.log") }}
                        </button>
                        <button
                          v-if="canAuditPrivilege(row.status)"
                          class="btn btn-warning btn-xs"
                          @click="auditRow = row"
                        >
                          {{ t("privilege.audit") }}
                        </button>
                      </td>
                    </tr>
                    <tr v-if="expandedId === row.applyId" class="privilege-detail-row">
                      <td colspan="11">
                        <div class="privilege-detail">
                          <strong class="privilege-detail-label">{{
                            row.privType === 2 ? t("privilege.tableList") : t("privilege.dbList")
                          }}</strong>
                          <div class="privilege-detail-chips">
                            <span
                              v-for="item in listItems(row)"
                              :key="item"
                              class="privilege-detail-chip"
                            >
                              {{ item }}
                            </span>
                            <span v-if="listItems(row).length === 0" class="subtle">
                              {{ t("privilege.itemsEmpty") }}
                            </span>
                          </div>
                        </div>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>

            <div class="pager privilege-pager">
              <span class="privilege-pager-count">{{ t("privilege.records", { n: total }) }}</span>
              <span class="spacer"></span>
              <span class="privilege-page-indicator">
                {{ t("result.page", { page, pages: pageCount }) }}
              </span>
              <button class="btn ghost privilege-page-btn" :disabled="page <= 1" @click="go(-1)">
                <NavIcon name="chevron" :size="12" class="privilege-page-prev" />
                {{ t("result.prev") }}
              </button>
              <button
                class="btn ghost privilege-page-btn"
                :disabled="page >= pageCount"
                @click="go(1)"
              >
                {{ t("result.next") }}
                <NavIcon name="chevron" :size="12" />
              </button>
            </div>
          </template>
        </template>
      </div>
    </section>

    <PrivilegeApplyDialog v-if="applyOpen" @close="applyOpen = false" @submitted="onSubmitted" />
    <PrivilegeAuditDialog
      v-if="auditRow"
      :row="auditRow"
      @close="auditRow = null"
      @audited="onAudited"
      @toast="(text) => emit('toast', text)"
    />
    <WorkflowLogDialog v-if="logRow" :row="logRow" @close="logRow = null" />
  </div>
</template>
