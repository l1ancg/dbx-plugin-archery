<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { presetRange } from "../lib/date-range";
import {
  normalizeSqlWorkflow,
  syntaxTypeLabel,
  workflowScopeStatus,
  workflowStatusMeta,
} from "../lib/workflow";
import type {
  SqlSubmitPrefill,
  SqlWorkflowOption,
  SqlWorkflowPage,
  SqlWorkflowRow,
} from "../types";
import DateRangeFilter from "../components/DateRangeFilter.vue";
import NavIcon from "../components/NavIcon.vue";
import WorkflowLogDialog from "../components/WorkflowLogDialog.vue";
import SqlSubmitView from "./SqlSubmitView.vue";
import SqlWorkflowDetailView from "./SqlWorkflowDetailView.vue";

const emit = defineEmits<{
  toast: [text: string, error?: boolean];
}>();

const PAGE_SIZE = 20;

/** Archery 的 SQL_WORKFLOW_CHOICES，与 sql/models.py 一致。 */
const statusOptions = [
  "workflow_manreviewing",
  "workflow_review_pass",
  "workflow_timingtask",
  "workflow_queuing",
  "workflow_executing",
  "workflow_finish",
  "workflow_abort",
  "workflow_autoreviewwrong",
  "workflow_exception",
];

const rows = ref<SqlWorkflowRow[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref("");
const status = ref("");
const instanceId = ref(0);
const groupId = ref(0);
/** 与 Archery 网页一致，默认看最近 30 日（含今天）。 */
const defaultRange = presetRange("last30");
const startDate = ref(defaultRange.start);
const endDate = ref(defaultRange.end);
/** all=申请工单（Archery 按角色收敛），audit=待我审核。 */
const scope = ref<"all" | "audit">("all");
const dense = ref(false);
const loading = ref(false);
const error = ref("");
const instances = ref<SqlWorkflowOption[]>([]);
const groups = ref<SqlWorkflowOption[]>([]);
const submitOpen = ref(false);
/** 详情页「查看提交信息 / 上线其他实例」带入提交页的初始值；为空表示空白新建。 */
const submitPrefill = ref<SqlSubmitPrefill | null>(null);
const logRow = ref<SqlWorkflowRow | null>(null);
const detailRow = ref<SqlWorkflowRow | null>(null);
let debounce: ReturnType<typeof setTimeout> | undefined;

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)));
/** 页脚「显示第 X 到第 Y 条」的端点，随当前页实际条数收敛。 */
const rangeFrom = computed(() => (rows.value.length ? (page.value - 1) * PAGE_SIZE + 1 : 0));
const rangeTo = computed(() => rangeFrom.value + rows.value.length - 1);

/** 「待我审核」状态恒为待审核，下拉只做展示；切回申请工单恢复用户原来的选择。 */
const statusFilter = computed({
  get: () => workflowScopeStatus(scope.value === "audit", status.value),
  set: (value: string) => {
    status.value = value;
  },
});

function typeText(row: SqlWorkflowRow): string {
  const label = syntaxTypeLabel(row.syntaxType);
  if (label === "OFFLINE") return t("workflow.syntaxOffline");
  if (label) return label;
  return t("workflow.syntaxOther");
}

/** 类型标签的语义色：DDL 蓝、DML 紫、离线导出走中性灰。 */
function typeTone(row: SqlWorkflowRow): string {
  switch (syntaxTypeLabel(row.syntaxType)) {
    case "DDL":
      return "ddl";
    case "DML":
      return "dml";
    case "OFFLINE":
      return "offline";
    default:
      return "other";
  }
}

/** 发起时间拆成「日期 / 时刻」两行，窄列也能完整读出。 */
function splitTime(value: string): { date: string; time: string } {
  const [date = "", time = ""] = value.trim().split(/\s+/);
  return { date: date || "-", time };
}

/** 每行只算一次展示值，模板里不再重复调用格式化函数。 */
const displayRows = computed(() =>
  rows.value.map((row) => ({
    row,
    type: typeText(row),
    tone: typeTone(row),
    when: splitTime(row.createTime),
  })),
);

async function loadOptions(): Promise<void> {
  try {
    const options = await archeryApi.sqlWorkflowOptions();
    instances.value = options.instances;
    groups.value = options.groups;
  } catch {
    // 下拉拿不到不影响列表本身，筛选退回「全部」。
    instances.value = [];
    groups.value = [];
  }
}

/** 列表参数；「待我审核」只可能是待审核状态，状态筛选被忽略。 */
function listParams(audit: boolean) {
  return {
    limit: PAGE_SIZE,
    offset: (page.value - 1) * PAGE_SIZE,
    search: keyword.value.trim(),
    status: workflowScopeStatus(audit, status.value),
    instanceId: instanceId.value,
    groupId: groupId.value,
    startDate: startDate.value,
    endDate: endDate.value,
    audit,
  };
}

function applyPage(result: SqlWorkflowPage): void {
  rows.value = (result.rows ?? []).map((row) =>
    normalizeSqlWorkflow(row as unknown as Record<string, unknown>),
  );
  total.value = result.total ?? 0;
}

function showFailure(issue: unknown): void {
  error.value = describeError(issue);
  rows.value = [];
  total.value = 0;
}

/**
 * 申请工单直接取 /sqlworkflow_list/；
 * 「待我审核」由 sidecar 用 /workflow/list/（current_audit 命中我的审核组）
 * 与待审核工单求交集，只返回当前轮到我的工单。
 */
async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    applyPage(await archeryApi.sqlWorkflows(listParams(scope.value === "audit")));
  } catch (issue) {
    showFailure(issue);
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

function switchScope(next: "all" | "audit"): void {
  if (scope.value === next) return;
  scope.value = next;
  changeFilter();
}

function go(delta: number): void {
  const next = page.value + delta;
  if (next < 1 || next > pageCount.value) return;
  page.value = next;
  void load();
}

/** 列表页的「提交SQL」：空白新建，不带任何工单信息。 */
function openSubmit(): void {
  submitPrefill.value = null;
  submitOpen.value = true;
}

/** 详情页的「查看提交信息 / 上线其他实例」：整页切到提交表单并预填原工单内容。 */
function onCommit(prefill: SqlSubmitPrefill): void {
  submitPrefill.value = prefill;
  submitOpen.value = true;
}

/** 提交页返回：有来源工单时回到详情，否则回列表。 */
function closeSubmit(): void {
  submitOpen.value = false;
  submitPrefill.value = null;
}

function onSubmitted(): void {
  submitOpen.value = false;
  submitPrefill.value = null;
  detailRow.value = null;
  scope.value = "all";
  page.value = 1;
  void load();
}

/** 详情页的动作改变了工单状态：详情页留在原地，列表数据在后台同步。 */
async function onDetailUpdated(): Promise<void> {
  await load();
  const fresh = rows.value.find((row) => row.id === detailRow.value?.id);
  if (fresh) detailRow.value = fresh;
}

onMounted(() => {
  void loadOptions();
  void load();
});
</script>

<template>
  <SqlSubmitView
    v-if="submitOpen"
    :prefill="submitPrefill"
    @back="closeSubmit"
    @submitted="onSubmitted"
    @toast="(text, isError) => emit('toast', text, isError)"
  />

  <SqlWorkflowDetailView
    v-else-if="detailRow"
    :row="detailRow"
    @back="detailRow = null"
    @commit="onCommit"
    @updated="onDetailUpdated"
    @toast="(text) => emit('toast', text)"
  />

  <div v-else class="page page-fill workflow-page">
    <header class="workflow-head">
      <h2 class="page-title">{{ t("nav.auditOnline") }}</h2>
      <p class="workflow-head-desc">{{ t("workflow.listDesc") }}</p>
    </header>

    <section class="panel card-fill workflow-panel">
      <div class="panel-heading privilege-toolbar workflow-toolbar">
        <span class="workflow-scope">
          <button
            class="btn btn-default btn-xs"
            :class="{ 'is-on': scope === 'all' }"
            @click="switchScope('all')"
          >
            {{ t("workflow.scope.all") }}
          </button>
          <button
            class="btn btn-default btn-xs"
            :class="{ 'is-on': scope === 'audit' }"
            @click="switchScope('audit')"
          >
            {{ t("workflow.scope.audit") }}
          </button>
        </span>

        <span class="workflow-toolbar-sep" aria-hidden="true"></span>

        <select
          v-model="statusFilter"
          class="form-control form-control-sm workflow-filter"
          :aria-label="t('workflow.colStatus')"
          :disabled="scope === 'audit'"
          @change="changeFilter"
        >
          <option value="">{{ t("workflow.colStatus") }}</option>
          <option v-for="item in statusOptions" :key="item" :value="item">
            {{ t(workflowStatusMeta(item).labelKey) }}
          </option>
        </select>

        <select
          v-model.number="instanceId"
          class="form-control form-control-sm workflow-filter"
          :aria-label="t('workflow.colInstance')"
          @change="changeFilter"
        >
          <option :value="0">{{ t("workflow.colInstance") }}</option>
          <option v-for="item in instances" :key="item.id" :value="item.id">
            {{ item.name }}
          </option>
        </select>

        <select
          v-model.number="groupId"
          class="form-control form-control-sm workflow-filter"
          :aria-label="t('workflow.colGroup')"
          @change="changeFilter"
        >
          <option :value="0">{{ t("workflow.colGroup") }}</option>
          <option v-for="item in groups" :key="item.id" :value="item.id">
            {{ item.name }}
          </option>
        </select>

        <DateRangeFilter
          v-model:start="startDate"
          v-model:end="endDate"
          @change="changeFilter"
        />

        <span class="spacer"></span>

        <label class="workflow-search">
          <NavIcon name="search" :size="13" class="workflow-search-icon" />
          <input
            v-model="keyword"
            class="form-control form-control-sm privilege-search"
            type="search"
            :aria-label="t('workflow.searchPlaceholder')"
            :placeholder="t('workflow.searchPlaceholder')"
            @input="onSearch"
          />
        </label>

        <button class="tool-btn workflow-refresh" :title="t('app.refresh')" @click="load">
          <NavIcon name="refresh" :size="14" />
        </button>

        <button
          class="btn btn-success btn-xs privilege-apply-btn workflow-submit"
          @click="openSubmit"
        >
          <NavIcon name="plus" :size="12" />
          <span>{{ t("workflow.submit") }}</span>
        </button>
      </div>

      <div class="panel-body privilege-list-body">
        <div v-if="error" class="error-box workflow-error">
          {{ t("workflow.loadFailed") }}：{{ error }}
        </div>
        <div
          v-else-if="loading"
          class="workflow-skeleton"
          role="status"
          :aria-label="t('app.loading')"
        >
          <span v-for="n in 7" :key="n" class="workflow-skeleton-row"></span>
        </div>
        <div v-else-if="rows.length === 0" class="workflow-empty">
          <span class="workflow-empty-icon">
            <NavIcon name="file" :size="22" />
          </span>
          <strong>{{ t("workflow.empty") }}</strong>
          <span class="workflow-empty-hint">{{ t("workflow.emptyHint") }}</span>
          <button class="btn btn-default btn-xs" @click="openSubmit">
            <NavIcon name="plus" :size="12" />
            <span>{{ t("workflow.submit") }}</span>
          </button>
        </div>

        <template v-else>
          <div class="grid-wrap">
            <table class="data-table privilege-table workflow-table" :class="{ compact: dense }">
              <thead>
                <tr>
                  <th>{{ t("workflow.colTitle") }}</th>
                  <th>{{ t("workflow.colType") }}</th>
                  <th>{{ t("workflow.colApplicant") }}</th>
                  <th>{{ t("workflow.colStatus") }}</th>
                  <th>{{ t("workflow.colBackup") }}</th>
                  <th>{{ t("workflow.colCreateTime") }}</th>
                  <th>{{ t("workflow.colInstance") }}</th>
                  <th>{{ t("workflow.colDatabase") }}</th>
                  <th>{{ t("workflow.colGroup") }}</th>
                  <th>{{ t("workflow.colOperation") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(item, index) in displayRows"
                  :key="item.row.id"
                  class="is-clickable"
                  :style="{ animationDelay: `${Math.min(index, 9) * 24}ms` }"
                  @click="detailRow = item.row"
                >
                  <td class="privilege-title">
                    <button
                      class="privilege-title-link"
                      :title="item.row.workflowName"
                    >
                      {{ item.row.workflowName }}
                    </button>
                  </td>
                  <td>
                    <span class="workflow-type" :class="`is-${item.tone}`">{{ item.type }}</span>
                  </td>
                  <td :title="item.row.engineerDisplay">
                    <span class="workflow-owner">{{ item.row.engineerDisplay || "-" }}</span>
                  </td>
                  <td>
                    <span
                      class="privilege-status"
                      :class="`is-${workflowStatusMeta(item.row.status).tone}`"
                      :title="t(workflowStatusMeta(item.row.status).labelKey)"
                    >
                      {{ t(workflowStatusMeta(item.row.status).labelKey) }}
                    </span>
                  </td>
                  <td>
                    <span class="workflow-backup" :class="{ 'is-on': item.row.isBackup }">
                      <NavIcon :name="item.row.isBackup ? 'check' : 'minus'" :size="11" />
                      <span>
                        {{
                          item.row.isBackup ? t("workflow.backupYes") : t("workflow.backupNo")
                        }}
                      </span>
                    </span>
                  </td>
                  <td class="nowrap">
                    <span class="workflow-when">
                      <span class="workflow-date">{{ item.when.date }}</span>
                      <span v-if="item.when.time" class="workflow-clock">{{ item.when.time }}</span>
                    </span>
                  </td>
                  <td class="workflow-ident" :title="item.row.instanceName">
                    {{ item.row.instanceName }}
                  </td>
                  <td class="workflow-ident" :title="item.row.dbName">{{ item.row.dbName }}</td>
                  <td :title="item.row.groupName">{{ item.row.groupName }}</td>
                  <!-- 操作列内部的点击不触发行跳转 -->
                  <td class="privilege-ops" @click.stop>
                    <button class="btn btn-quiet btn-xs workflow-log" @click="logRow = item.row">
                      <NavIcon name="clock" :size="12" />
                      <span>{{ t("privilege.log") }}</span>
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager workflow-pager">
            <span class="workflow-pager-count">
              {{ t("workflow.records", { from: rangeFrom, to: rangeTo, total }) }}
            </span>
            <span class="spacer"></span>
            <button
              class="btn btn-default btn-xs workflow-page-btn"
              :disabled="page <= 1"
              @click="go(-1)"
            >
              <NavIcon name="chevron" :size="10" class="workflow-page-prev" />
              <span>{{ t("result.prev") }}</span>
            </button>
            <span class="workflow-page-indicator">
              {{ t("result.page", { page, pages: pageCount }) }}
            </span>
            <button
              class="btn btn-default btn-xs workflow-page-btn"
              :disabled="page >= pageCount"
              @click="go(1)"
            >
              <span>{{ t("result.next") }}</span>
              <NavIcon name="chevron" :size="10" />
            </button>
          </div>
        </template>
      </div>
    </section>

    <WorkflowLogDialog v-if="logRow" :row="logRow" :workflow-type="2" @close="logRow = null" />
  </div>
</template>
