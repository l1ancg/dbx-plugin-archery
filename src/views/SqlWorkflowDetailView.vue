<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { normalizeWorkflowLog } from "../lib/privilege";
import {
  canAuditWorkflow,
  canCancelWorkflow,
  canExecuteWorkflow,
  normalizeSqlWorkflowDetail,
  syntaxTypeLabel,
  workflowStatusMeta,
} from "../lib/workflow";
import {
  buildOtherInstancePrefill,
  buildRollbackFile,
  buildSubmitPrefill,
  canRollbackWorkflow,
  infoFieldLabelKey,
  normalizeRollbackRows,
  normalizeWorkflowGrid,
  readRunDateRange,
  resolveReviewNodes,
} from "../lib/workflow-detail";
import { saveTextFile } from "../lib/download";
import { writeClipboard } from "../lib/clipboard";
import type {
  SqlSubmitPrefill,
  SqlWorkflowRow,
  WorkflowGridRow,
  WorkflowInfoField,
  WorkflowLogRow,
} from "../types";
import NavIcon from "../components/NavIcon.vue";
import SqlWorkflowActionDialog from "../components/SqlWorkflowActionDialog.vue";
import SqlWorkflowAuditDialog from "../components/SqlWorkflowAuditDialog.vue";
import SqlWorkflowRollbackView from "./SqlWorkflowRollbackView.vue";

const props = defineProps<{
  row: SqlWorkflowRow;
}>();

const emit = defineEmits<{
  back: [];
  updated: [];
  toast: [text: string];
  /** 「查看提交信息 / 上线其他实例」：把本工单的提交内容带给提交页，由上层切换到整页表单。 */
  commit: [prefill: SqlSubmitPrefill];
}>();

const detail = ref(normalizeSqlWorkflowDetail(null));
const grid = ref<WorkflowGridRow[]>([]);
const loading = ref(true);
const detailError = ref("");
const tab = ref<"detail" | "log">("detail");
const sqlKeyword = ref("");
const dense = ref(false);
const expanded = ref<number[]>([]);
const auditOpen = ref(false);
const actionKind = ref<"" | "execute" | "schedule" | "cancel" | "alterRunDate">("");
const rollbackOpen = ref(false);
const rollbackDownloading = ref(false);
const logRows = ref<WorkflowLogRow[]>([]);
const logKeyword = ref("");
const logLoading = ref(false);
const logError = ref("");

const status = computed(() => detail.value.status || props.row.status);
/** 状态胶囊：与 SQL 上线列表页共用 privilege-status 的语义色。 */
const statusMeta = computed(() => workflowStatusMeta(status.value));
const typeText = computed(() => {
  const label = syntaxTypeLabel(props.row.syntaxType);
  if (label === "OFFLINE") return t("workflow.syntaxOffline");
  return label || t("workflow.syntaxOther");
});
const canAudit = computed(() => canAuditWorkflow(props.row.status));
const canExecute = computed(() => canExecuteWorkflow(props.row.status));
const canCancel = computed(() => canCancelWorkflow(props.row.status));
const rollbackVisible = computed(() =>
  canRollbackWorkflow(props.row.status, props.row.isBackup),
);

/** 详情页解析失败时用列表行兜底，保证关键信息仍然可读（缺的列宁可不显示也不编造）。 */
const fallbackFields = computed<WorkflowInfoField[]>(() => [
  { label: "发起人", value: props.row.engineerDisplay || "-" },
  { label: "目标实例", value: props.row.instanceName },
  { label: "数据库", value: props.row.dbName },
  { label: "发起时间", value: props.row.createTime },
  { label: "备份", value: props.row.isBackup ? t("workflow.backupYes") : t("workflow.backupNo") },
  { label: "当前状态", value: t(workflowStatusMeta(props.row.status).labelKey) },
  { label: "组", value: props.row.groupName },
  { label: "类型", value: typeText.value },
]);

const infoFields = computed<WorkflowInfoField[]>(() =>
  detail.value.infoFields.length > 0 ? detail.value.infoFields : fallbackFields.value,
);

/** 「可执行时间变更」的预填值：取详情「可执行时间范围」，「无限制」时两端为空。 */
const currentRunDate = computed(() => readRunDateRange(infoFields.value));

/** 头部元信息行只取这 5 个字段：目标上下文（实例/库/组）+ 发起人/时间。 */
const META_LABELS = ["目标实例", "数据库", "组", "发起人", "发起时间"];

const metaItems = computed(() => {
  const byLabel = new Map(infoFields.value.map((field) => [field.label.trim(), field]));
  return META_LABELS.flatMap((label) => {
    const field = byLabel.get(label);
    return field ? [{ key: label, label: fieldLabel(field), value: field.value }] : [];
  });
});

/** 底栏只在真有可用操作时渲染，避免空条占位。 */
const hasActions = computed(
  () => canAudit.value || canCancel.value || canExecute.value || rollbackVisible.value,
);

/** 审批流：详情页解析不到节点状态时，已结束的工单整条按「已通过」渲染。 */
const reviewNodes = computed(() => resolveReviewNodes(detail.value.reviewNodes, status.value));

/** 卡片列数优先 5 列（10 个字段正好 5×2），字段数少时退到能整除的列数，别留空槽。 */
const INFO_COLUMN_CHOICES = [5, 4, 3, 2];

const infoColumns = computed(() => {
  const total = infoFields.value.length;
  if (total <= 1) return 1;
  return INFO_COLUMN_CHOICES.find((cols) => total % cols === 0) ?? 5;
});

const infoGridStyle = computed<Record<string, string>>(() => ({
  "--info-cols": String(infoColumns.value),
}));

const visibleRows = computed(() => {
  const keyword = sqlKeyword.value.trim().toLowerCase();
  if (!keyword) return grid.value;
  return grid.value.filter((item) =>
    [item.sql, item.errorMessage, item.stage].some((text) =>
      text.toLowerCase().includes(keyword),
    ),
  );
});

const expandedAll = computed(
  () => visibleRows.value.length > 0 && expanded.value.length >= visibleRows.value.length,
);

/** 记录数右侧的小结：不通过的行数，长工单不必逐行扫状态列。 */
const warnCount = computed(
  () => visibleRows.value.filter((item) => item.errorLevel === 1).length,
);
const errorCount = computed(
  () => visibleRows.value.filter((item) => item.errorLevel === 2).length,
);

/** 日志按操作时间倒序（最新在上，与 Archery 网页一致），时间相同或缺失时保持服务端顺序。 */
const visibleLogs = computed(() => {
  const keyword = logKeyword.value.trim().toLowerCase();
  return logRows.value
    .filter((item) =>
      [item.operationType, item.operator, item.time, item.info].some((text) =>
        text.toLowerCase().includes(keyword),
      ),
    )
    .sort((left, right) =>
      left.time === right.time ? 0 : left.time < right.time ? 1 : -1,
    );
});

function fieldLabel(field: WorkflowInfoField): string {
  const key = infoFieldLabelKey(field.label);
  return key ? t(key) : field.label;
}

/** 「当前状态」在卡片里用状态胶囊展示，避免和普通文本混在一起。 */
function isStatusField(field: WorkflowInfoField): boolean {
  return field.label.trim() === "当前状态";
}

/** SQL 列表行内复制：写剪贴板失败时提示手动选中，不静默吞掉。 */
async function copySql(item: WorkflowGridRow): Promise<void> {
  const copied = await writeClipboard(item.sql);
  emit("toast", copied ? t("app.copied") : t("workflow.copyFailed"));
}

function levelLabel(level: number): string {
  if (level === 2) return t("workflow.level.error");
  if (level === 1) return t("workflow.level.warn");
  return t("workflow.level.ok");
}

function levelTone(level: number): string {
  if (level === 2) return "is-danger";
  if (level === 1) return "is-warn";
  return "is-ok";
}

function rowClass(item: WorkflowGridRow): string {
  if (item.errorLevel === 2) return "is-danger";
  if (item.errorLevel === 1) return "is-warn";
  return "";
}

function isExpanded(id: number): boolean {
  return expanded.value.includes(id);
}

function toggleRow(id: number): void {
  expanded.value = isExpanded(id)
    ? expanded.value.filter((item) => item !== id)
    : [...expanded.value, id];
}

function toggleAll(): void {
  expanded.value = expandedAll.value ? [] : visibleRows.value.map((item) => item.id);
}

async function load(): Promise<void> {
  loading.value = true;
  detailError.value = "";
  try {
    const payload = await archeryApi.sqlWorkflowDetail({ workflowId: props.row.id });
    const record = (payload ?? {}) as unknown as Record<string, unknown>;
    detail.value = normalizeSqlWorkflowDetail(payload);
    grid.value = normalizeWorkflowGrid(record.reviewRows ?? record.review_rows);
    expanded.value = [];
  } catch (issue) {
    detailError.value = describeError(issue);
    detail.value = normalizeSqlWorkflowDetail(null);
    grid.value = [];
  } finally {
    loading.value = false;
  }
}

async function openLogs(): Promise<void> {
  tab.value = "log";
  if (logRows.value.length > 0 || logLoading.value) return;
  logLoading.value = true;
  logError.value = "";
  try {
    const page = await archeryApi.workflowLogs({ workflowId: props.row.id, workflowType: 2 });
    logRows.value = (page.rows ?? []).map((item) =>
      normalizeWorkflowLog(item as unknown as Record<string, unknown>),
    );
  } catch (issue) {
    logError.value = describeError(issue);
  } finally {
    logLoading.value = false;
  }
}

/** 回滚子页提交了回滚工单后回到详情并刷新状态。 */
function onRollbackSubmitted(): void {
  rollbackOpen.value = false;
  void load();
}

/** 下载回滚 SQL：不进子页，弹「另存为」对话框自选路径，按 Archery 的下载格式落盘。 */
async function downloadRollback(): Promise<void> {
  rollbackDownloading.value = true;
  try {
    const result = await archeryApi.sqlWorkflowRollback({ workflowId: props.row.id });
    const rows = normalizeRollbackRows(result.rows, props.row.dbName);
    if (rows.length === 0) {
      emit("toast", t("workflow.rollbackEmpty"));
      return;
    }
    const saved = await saveTextFile(buildRollbackFile(rows), `rollback_${props.row.id}.sql`, {
      askPath: true,
    });
    if (saved.status === "saved") {
      emit("toast", t("workflow.rollbackSavedTo", { path: saved.path }));
    } else if (saved.status === "browser") {
      emit("toast", t("workflow.rollbackSaved"));
    }
  } catch (issue) {
    emit("toast", describeError(issue));
  } finally {
    rollbackDownloading.value = false;
  }
}

function onActionDone(): void {
  const kind = actionKind.value;
  actionKind.value = "";
  // 改时间窗不改变工单状态，留在详情页刷新即可；其余动作会让工单离开当前节点。
  if (kind === "alterRunDate") {
    void load();
    return;
  }
  emit("updated");
}

function onAudited(): void {
  auditOpen.value = false;
  emit("updated");
}

/** 两个预填按钮共用的取值来源：详情标题 + 其他信息 + SQL 列表，缺失时用列表行兜底。 */
const prefillSource = computed(() => ({
  row: props.row,
  title: detail.value.title,
  infoFields: infoFields.value,
  rows: grid.value,
}));

/** 「查看提交信息」：SQL 原文 + 组/实例/库/备份/执行时间窗。 */
const submitPrefill = computed<SqlSubmitPrefill>(() => buildSubmitPrefill(prefillSource.value));

/** 「上线其他实例」：同上，只是实例与库留空。 */
const otherInstancePrefill = computed<SqlSubmitPrefill>(() =>
  buildOtherInstancePrefill(prefillSource.value),
);

function openCommit(): void {
  emit("commit", submitPrefill.value);
}

function openOtherInstance(): void {
  emit("commit", otherInstancePrefill.value);
}

onMounted(load);
</script>

<template>
  <SqlWorkflowRollbackView
    v-if="rollbackOpen"
    :row="row"
    @back="rollbackOpen = false"
    @submitted="onRollbackSubmitted"
    @toast="(text) => emit('toast', text)"
  />
  <div v-else class="page page-fill workflow-detail-page">
    <section class="panel card-fill workflow-detail-panel">
      <div class="panel-heading workflow-detail-top">
        <button class="tool-btn privilege-back" :title="t('privilege.back')" @click="emit('back')">
          <NavIcon name="chevron" :size="14" />
        </button>
        <span class="workflow-detail-crumb">{{ t("nav.auditOnline") }}</span>
        <span class="spacer"></span>
        <button class="tool-btn" :title="t('app.refresh')" @click="load">
          <NavIcon name="refresh" :size="14" />
        </button>
      </div>

      <div class="panel-body workflow-detail-scroll">
        <div v-if="detailError" class="error-box workflow-detail-error">
          {{ t("workflow.loadDetailFailed") }}：{{ detailError }}
        </div>

        <header class="workflow-detail-head" :class="`is-${statusMeta.tone}`">
          <div class="workflow-title-row">
            <h3 class="workflow-title">{{ detail.title || row.workflowName }}</h3>
            <span class="privilege-status workflow-status-pill" :class="`is-${statusMeta.tone}`">
              {{ t(statusMeta.labelKey) }}
            </span>
            <span class="workflow-type-chip">{{ typeText }}</span>
            <span class="spacer"></span>
            <div class="workflow-title-actions">
              <button class="btn btn-default btn-xs" @click="openOtherInstance">
                {{ t("workflow.deployOtherInstance") }}
              </button>
              <button class="btn btn-default btn-xs" @click="openCommit">
                {{ t("workflow.viewCommit") }}
              </button>
            </div>
          </div>
          <div v-if="metaItems.length > 0" class="workflow-meta">
            <span v-for="item in metaItems" :key="item.key" class="workflow-meta-item">
              <span class="workflow-meta-label">{{ item.label }}</span>
              <span class="workflow-meta-value" :title="item.value">{{ item.value }}</span>
            </span>
          </div>
        </header>

        <section class="workflow-block">
          <h4>{{ t("privilege.reviewFlow") }}</h4>
          <div v-if="loading && detail.reviewNodes.length === 0" class="workflow-flow-loading">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="detail.reviewNodes.length === 0" class="subtle">
            {{ t("privilege.reviewFlowEmpty") }}
          </div>
          <ol v-else class="workflow-flow">
            <li
              v-for="(node, index) in reviewNodes"
              :key="`${index}-${node.name}`"
              class="workflow-flow-node"
              :class="`is-${node.state}`"
            >
              <span class="workflow-flow-mark">
                <NavIcon
                  v-if="node.state === 'passed' || node.state === 'auto'"
                  name="check"
                  :size="11"
                />
                <template v-else>{{ index + 1 }}</template>
              </span>
              <span class="workflow-flow-body">
                <span class="workflow-flow-label">
                  {{ node.state === "auto" ? t("privilege.autoPass") : node.name }}
                </span>
                <span
                  v-if="node.state === 'current' && node.reviewers.length > 0"
                  class="workflow-flow-reviewers"
                >
                  {{ t("workflow.reviewerPrefix") }}{{ node.reviewers.join("、") }}
                </span>
              </span>
            </li>
          </ol>
        </section>

        <section class="workflow-block">
          <h4>{{ t("workflow.otherInfo") }}</h4>
          <div class="workflow-info-grid" :style="infoGridStyle">
            <div v-for="field in infoFields" :key="field.label" class="workflow-info-item">
              <span class="workflow-info-label">{{ fieldLabel(field) }}</span>
              <span class="workflow-info-value" :title="field.value || '-'">
                <span
                  v-if="isStatusField(field)"
                  class="privilege-status"
                  :class="`is-${statusMeta.tone}`"
                >
                  {{ field.value }}
                </span>
                <template v-else>{{ field.value || "-" }}</template>
              </span>
            </div>
          </div>
        </section>

        <div class="workflow-tabs">
          <button
            class="workflow-tab"
            :class="{ active: tab === 'detail' }"
            @click="tab = 'detail'"
          >
            {{ t("workflow.tabDetail") }}
            <span v-if="grid.length > 0" class="workflow-tab-count">{{ grid.length }}</span>
          </button>
          <button class="workflow-tab" :class="{ active: tab === 'log' }" @click="openLogs">
            {{ t("workflow.tabLog") }}
            <span v-if="logRows.length > 0" class="workflow-tab-count">{{ logRows.length }}</span>
          </button>
          <span class="spacer"></span>
          <input
            v-if="tab === 'log'"
            v-model="logKeyword"
            class="form-control workflow-tab-search workflow-log-search"
            type="search"
            :placeholder="t('workflow.logSearch')"
          />
          <input
            v-else
            v-model="sqlKeyword"
            class="form-control workflow-tab-search workflow-sql-search"
            type="search"
            :placeholder="t('workflow.searchSql')"
          />
        </div>

        <template v-if="tab === 'log'">
          <div v-if="logLoading" class="empty">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="logError" class="error-box">
            {{ t("privilege.logFailed") }}：{{ logError }}
          </div>
          <div v-else-if="logRows.length === 0" class="empty">{{ t("privilege.logEmpty") }}</div>
          <template v-else>
            <div v-if="visibleLogs.length === 0" class="empty">{{ t("workflow.logNoMatch") }}</div>
            <div v-else class="grid-wrap workflow-log-wrap">
              <table class="data-table privilege-log-table workflow-log-table">
                <thead>
                  <tr>
                    <th>{{ t("privilege.logOperation") }}</th>
                    <th>{{ t("privilege.logOperator") }}</th>
                    <th>{{ t("privilege.logTime") }}</th>
                    <th>{{ t("privilege.logInfo") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(log, index) in visibleLogs" :key="`${log.time}-${index}`">
                    <td>{{ log.operationType || "-" }}</td>
                    <td>{{ log.operator || "-" }}</td>
                    <td class="nowrap">{{ log.time || "-" }}</td>
                    <td :title="log.info">{{ log.info || "-" }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="workflow-record-count">
              {{
                t("workflow.records", {
                  from: visibleLogs.length === 0 ? 0 : 1,
                  to: visibleLogs.length,
                  total: visibleLogs.length,
                })
              }}
            </div>
          </template>
        </template>

        <template v-else>
          <div class="workflow-grid-toolbar">
            <button class="btn btn-default btn-xs" @click="toggleAll">
              {{ expandedAll ? t("workflow.collapseAll") : t("workflow.expandAll") }}
            </button>
            <span class="spacer"></span>
            <button
              class="tool-btn"
              :class="{ 'is-on': dense }"
              :title="t('workflow.density')"
              @click="dense = !dense"
            >
              <NavIcon name="table" :size="14" />
            </button>
          </div>

          <div v-if="loading && grid.length === 0" class="empty">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="visibleRows.length === 0" class="empty">
            {{ t("workflow.reviewRowsEmpty") }}
          </div>
          <div v-else class="grid-wrap workflow-grid-wrap">
            <table class="data-table workflow-grid" :class="{ compact: dense }">
              <thead>
                <tr>
                  <th class="workflow-grid-caret"></th>
                  <th>{{ t("workflow.colId") }}</th>
                  <th>{{ t("workflow.colSqlContent") }}</th>
                  <th>{{ t("workflow.colReviewStatus") }}</th>
                  <th>{{ t("workflow.colReviewInfo") }}</th>
                  <th>{{ t("workflow.colAffectedRowsText") }}</th>
                  <th>{{ t("workflow.colExecuteTime") }}</th>
                  <th>{{ t("workflow.colBackupTime") }}</th>
                  <th>{{ t("workflow.colCurrentStage") }}</th>
                  <th>{{ t("workflow.colOperation") }}</th>
                </tr>
              </thead>
              <tbody>
                <template v-for="item in visibleRows" :key="item.id">
                  <tr :class="rowClass(item)">
                    <td class="workflow-grid-caret">
                      <button
                        class="op-btn"
                        :title="isExpanded(item.id) ? t('workflow.collapseRow') : t('workflow.expandRow')"
                        @click="toggleRow(item.id)"
                      >
                        <NavIcon :name="isExpanded(item.id) ? 'minus' : 'plus'" :size="11" />
                      </button>
                    </td>
                    <td class="workflow-grid-id">{{ item.id }}</td>
                    <td class="workflow-grid-sql" :title="item.sql">{{ item.sql || "-" }}</td>
                    <td>
                      <span class="privilege-status" :class="levelTone(item.errorLevel)">
                        {{ levelLabel(item.errorLevel) }}
                      </span>
                    </td>
                    <td class="workflow-grid-message" :class="{ 'is-empty': !item.errorMessage }">
                      {{ item.errorMessage || "-" }}
                    </td>
                    <td>{{ item.affectedRows }}</td>
                    <td :class="{ 'is-empty': !item.executeTime }">{{ item.executeTime || "-" }}</td>
                    <td :class="{ 'is-empty': !item.backupTime }">{{ item.backupTime || "-" }}</td>
                    <td class="workflow-grid-stage" :class="{ 'is-empty': !item.stage }">
                      {{ item.stage || "-" }}
                    </td>
                    <td class="workflow-grid-ops">
                      <button
                        class="workflow-copy-sql"
                        :disabled="!item.sql"
                        :title="t('workflow.copySql')"
                        :aria-label="t('workflow.copySql')"
                        @click="copySql(item)"
                      >
                        <NavIcon name="copy" :size="13" />
                      </button>
                    </td>
                  </tr>
                  <tr v-if="isExpanded(item.id)" class="workflow-grid-expanded">
                    <td colspan="10">
                      <pre class="workflow-grid-sql-full">{{ item.sql }}</pre>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <div class="workflow-record-count">
            <span>
              {{
                t("workflow.records", {
                  from: visibleRows.length === 0 ? 0 : 1,
                  to: visibleRows.length,
                  total: visibleRows.length,
                })
              }}
            </span>
            <span v-if="warnCount > 0" class="workflow-count-chip">
              {{ t("workflow.warnCount", { count: warnCount }) }}
            </span>
            <span v-if="errorCount > 0" class="workflow-count-chip is-danger">
              {{ t("workflow.errorCount", { count: errorCount }) }}
            </span>
          </div>

          <section v-if="detail.lastOperation" class="workflow-block">
            <h4>{{ t("privilege.lastOperation") }}</h4>
            <div class="privilege-last-operation">{{ detail.lastOperation }}</div>
          </section>
        </template>

      </div>

      <div v-if="hasActions" class="workflow-actions workflow-detail-actions">
        <button v-if="canAudit" class="btn btn-info" @click="actionKind = 'alterRunDate'">
          {{ t("workflow.alterRunDate") }}
        </button>
        <button v-if="canAudit" class="btn btn-success" @click="auditOpen = true">
          {{ t("workflow.auditPass") }}
        </button>
        <button v-if="canCancel" class="btn btn-default" @click="actionKind = 'cancel'">
          {{ t("workflow.cancel") }}
        </button>
        <button v-if="canExecute" class="btn btn-success" @click="actionKind = 'execute'">
          {{ t("workflow.execute") }}
        </button>
        <button v-if="canExecute" class="btn btn-primary" @click="actionKind = 'schedule'">
          {{ t("workflow.schedule") }}
        </button>
        <span class="spacer"></span>
        <button v-if="rollbackVisible" class="btn btn-default" @click="rollbackOpen = true">
          {{ t("workflow.rollbackView") }}
        </button>
        <button
          v-if="rollbackVisible"
          class="btn btn-info"
          :disabled="rollbackDownloading"
          @click="downloadRollback"
        >
          <span v-if="rollbackDownloading" class="spinner"></span>
          {{ t("workflow.rollbackDownload") }}
        </button>
      </div>
    </section>

    <SqlWorkflowAuditDialog
      v-if="auditOpen"
      :row="row"
      @close="auditOpen = false"
      @audited="onAudited"
      @toast="(text) => emit('toast', text)"
    />
    <SqlWorkflowActionDialog
      v-if="actionKind"
      :kind="actionKind"
      :row="row"
      :run-date-start="currentRunDate.start"
      :run-date-end="currentRunDate.end"
      @close="actionKind = ''"
      @done="onActionDone"
      @toast="(text) => emit('toast', text)"
    />
  </div>
</template>
