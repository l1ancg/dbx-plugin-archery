<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { formatSql } from "../lib/sql";
import {
  normalizeCheckResult,
  splitAuditMessages,
  stripImportedSql,
  validateSqlSubmit,
} from "../lib/workflow";
import {
  ensureOptionValue,
  matchOptionIdByName,
  normalizeWorkflowGrid,
} from "../lib/workflow-detail";
import type {
  GroupInstance,
  SqlCheckSummary,
  SqlSubmitForm,
  SqlSubmitPrefill,
  SqlWorkflowOption,
  WorkflowGridRow,
} from "../types";
import DateTimeRangeField from "../components/DateTimeRangeField.vue";
import NavIcon from "../components/NavIcon.vue";
import SqlEditor from "../components/SqlEditor.vue";

const props = defineProps<{
  /** 工单详情「查看提交信息 / 上线其他实例」带入的初始值；为空表示新建空白工单。 */
  prefill?: SqlSubmitPrefill | null;
}>();

const emit = defineEmits<{
  back: [];
  submitted: [workflowId: number];
  toast: [text: string, isError?: boolean];
}>();

const MAX_FILE_BYTES = 10 * 1024 * 1024;

const groups = ref<SqlWorkflowOption[]>([]);
const instances = ref<GroupInstance[]>([]);
const databases = ref<string[]>([]);
const backupEnabled = ref(false);

const title = ref(props.prefill?.title ?? "");
const demandUrl = ref("");
const groupId = ref(0);
const instanceId = ref(0);
const dbName = ref("");
const sql = ref(props.prefill?.sql ?? "");
/** 已导入的 SQL 文件名，空串表示还没选过文件。 */
const fileName = ref("");
/** 文件导入时追加进编辑器的原始片段，移除文件时按它回退。 */
const importedSql = ref("");
const backup = ref(props.prefill?.backup ?? true);
const runDateStart = ref(props.prefill?.runDateStart ?? "");
const runDateEnd = ref(props.prefill?.runDateEnd ?? "");

const loadingGroups = ref(true);
const loadingInstances = ref(false);
const loadingDatabases = ref(false);
const checking = ref(false);
const submitting = ref(false);
const error = ref("");
const checkError = ref("");
const checkedSql = ref("");
const checkSummary = ref<SqlCheckSummary | null>(null);
const checkRows = ref<WorkflowGridRow[]>([]);
const checkKeyword = ref("");
const expandedCheck = ref<number[]>([]);
const auditorsDisplay = ref("");
const confirmPending = ref(false);

const currentInstance = computed(() =>
  instances.value.find((item) => item.id === instanceId.value),
);
/** 页头来源提示：换实例上线时要额外提醒重新选实例与库。 */
const prefillNotice = computed(() => {
  const source = props.prefill;
  if (!source) return "";
  const key =
    source.mode === "other" ? "workflow.otherInstanceNotice" : "workflow.prefillNotice";
  return t(key, { id: source.workflowId, title: source.title });
});
/** 库下拉选项：原工单的库不在资源清单里时补到首位，避免预填值显示为空。 */
const dbOptions = computed(() => ensureOptionValue(databases.value, dbName.value));
const checked = computed(() => checkedSql.value !== "" && checkedSql.value === sql.value.trim());
/** 与 Archery 一致：检测通过（无 error）后才允许提交。 */
const canSubmit = computed(
  () => checked.value && checkSummary.value !== null && checkSummary.value.errorCount === 0,
);
const visibleCheckRows = computed(() => {
  const keyword = checkKeyword.value.trim().toLowerCase();
  if (!keyword) return checkRows.value;
  return checkRows.value.filter((row) =>
    [row.sql, row.errorMessage].some((text) => text.toLowerCase().includes(keyword)),
  );
});
/** 编辑器工具条上的行数/字符数，空内容显示 0。 */
const sqlStats = computed(() => {
  const text = sql.value;
  return {
    lines: text.trim() ? text.split("\n").length : 0,
    chars: text.length,
  };
});

function form(): SqlSubmitForm {
  return {
    title: title.value,
    groupId: groupId.value,
    instanceId: instanceId.value,
    dbName: dbName.value,
    sql: sql.value,
    backup: backup.value,
    runDateStart: runDateStart.value,
    runDateEnd: runDateEnd.value,
  };
}

async function loadGroups(): Promise<void> {
  loadingGroups.value = true;
  try {
    const options = await archeryApi.sqlSubmitOptions();
    groups.value = options.groups ?? [];
    backupEnabled.value = options.backupEnabled;
    if (!backupEnabled.value) backup.value = false;
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingGroups.value = false;
  }
}

async function loadInstances(): Promise<void> {
  instances.value = [];
  instanceId.value = 0;
  databases.value = [];
  dbName.value = "";
  auditorsDisplay.value = "";
  const group = groups.value.find((item) => item.id === groupId.value);
  if (!group) return;
  loadingInstances.value = true;
  void loadAuditors(group.name);
  try {
    instances.value = await archeryApi.sqlWorkflowTargets({ groupName: group.name });
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingInstances.value = false;
  }
}

/** 审批流程与实例下拉并行拉取，失败不阻断表单填写。 */
async function loadAuditors(groupName: string): Promise<void> {
  try {
    const result = await archeryApi.sqlWorkflowAuditors({ groupName });
    if (groupName === groups.value.find((item) => item.id === groupId.value)?.name) {
      auditorsDisplay.value = result.auditorsDisplay;
    }
  } catch (issue) {
    emit("toast", describeError(issue), true);
  }
}

async function loadDatabases(): Promise<void> {
  databases.value = [];
  dbName.value = "";
  const instance = currentInstance.value;
  if (!instance) return;
  loadingDatabases.value = true;
  try {
    const list = await archeryApi.resources({
      instanceName: instance.name,
      instanceId: instance.id,
      resourceType: "database",
    });
    databases.value = list.map((item) => item.name).filter(Boolean);
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingDatabases.value = false;
  }
}

/** 下拉显式 change：预填链路需要 await 实例/库加载，不能再依赖 watch 的异步回调。 */
async function onGroupChange(): Promise<void> {
  await loadInstances();
}

async function onInstanceChange(): Promise<void> {
  await loadDatabases();
}

watch(sql, () => {
  // SQL 改了，之前的检测结论作废（提交按钮随之禁用）。
  if (checkedSql.value && sql.value.trim() !== checkedSql.value) {
    checkSummary.value = null;
    checkRows.value = [];
    confirmPending.value = false;
  }
});

async function onFileChange(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  // 立即清空 value，保证再次选择同一个文件也能触发 change。
  input.value = "";
  if (!file) return;
  if (file.size > MAX_FILE_BYTES) {
    emit("toast", t("workflow.sqlFileTooLarge"), true);
    return;
  }
  try {
    const text = await file.text();
    // 与 Archery 一致：导入内容追加到编辑器末尾，而不是覆盖已有 SQL。
    sql.value = sql.value.trim() ? `${sql.value}\n${text}` : text;
    fileName.value = file.name;
    importedSql.value = text;
  } catch (issue) {
    emit("toast", `${t("workflow.sqlFileError")}：${describeError(issue)}`, true);
  }
}

/** 移除文件：连它追加进编辑器的 SQL 一起摘掉，恢复导入前的文本。 */
function clearFile(): void {
  const { sql: rest, removed } = stripImportedSql(sql.value, importedSql.value);
  if (removed) {
    sql.value = rest;
  } else if (importedSql.value) {
    // 内容被手工改过就定位不到原文，只清标记并提示。
    emit("toast", t("workflow.sqlFileRemoveMissed"), true);
  }
  fileName.value = "";
  importedSql.value = "";
}

function beautify(): void {
  const dbType = currentInstance.value?.dbType ?? "";
  try {
    sql.value = formatSql(sql.value, dbType);
  } catch {
    emit("toast", t("workflow.beautifyFailed"), true);
  }
}

async function runCheck(): Promise<void> {
  checkError.value = "";
  error.value = "";
  if (!instanceId.value || !dbName.value || !sql.value.trim()) {
    checkError.value = t("workflow.needCheckInput");
    return;
  }
  checking.value = true;
  try {
    const payload = await archeryApi.sqlWorkflowCheck({
      instanceId: instanceId.value,
      dbName: dbName.value,
      sql: sql.value,
    });
    const record = (payload ?? {}) as unknown as Record<string, unknown>;
    checkSummary.value = normalizeCheckResult(payload);
    checkRows.value = normalizeWorkflowGrid(record.rows);
    checkKeyword.value = "";
    expandedCheck.value = [];
    checkedSql.value = sql.value.trim();
    confirmPending.value = false;
  } catch (issue) {
    checkError.value = describeError(issue);
    checkSummary.value = null;
    checkRows.value = [];
    checkedSql.value = "";
  } finally {
    checking.value = false;
  }
}

function toggleCheckRow(id: number): void {
  expandedCheck.value = expandedCheck.value.includes(id)
    ? expandedCheck.value.filter((item) => item !== id)
    : [...expandedCheck.value, id];
}

/** 审核状态按 Archery 原样显示 pass / warning / error。 */
function checkStatusText(level: number): string {
  if (level === 2) return "error";
  if (level === 1) return "warning";
  return "pass";
}

function checkRowClass(level: number): string {
  if (level === 2) return "is-danger";
  if (level === 1) return "is-warn";
  return "";
}

/** 表格单元格里每条错误/警告各占一行，交给 CSS 的 pre-line 渲染换行。 */
function auditMessageText(message: string | null | undefined): string {
  return splitAuditMessages(message).join("\n");
}

/** 状态标签沿用权限模块的 pill 配色：pass=绿、warning=黄、error=红。 */
function checkStatusTone(level: number): string {
  if (level === 2) return "is-danger";
  if (level === 1) return "is-warn";
  return "is-ok";
}

async function submit(force = false): Promise<void> {
  error.value = "";
  const missing = validateSqlSubmit(form());
  if (missing.length > 0) {
    error.value = t("workflow.missingFields", {
      fields: missing.map((key) => t(`workflow.field.${key}`)).join("、"),
    });
    return;
  }
  if (!checked.value || !checkSummary.value) {
    error.value = t("workflow.needCheck");
    return;
  }
  if (checkSummary.value.errorCount > 0) {
    error.value = t("workflow.checkBlocked");
    return;
  }
  if (checkSummary.value.warningCount > 0 && !force) {
    confirmPending.value = true;
    return;
  }

  submitting.value = true;
  try {
    const workflowId = await archeryApi.sqlWorkflowSubmit({
      ...form(),
      title: title.value.trim(),
      sql: sql.value.trim(),
      demandUrl: demandUrl.value.trim(),
    });
    emit("toast", t("workflow.submitted", { id: workflowId }));
    emit("submitted", workflowId);
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    submitting.value = false;
  }
}

/** 把带入的组 / 实例 / 库按名称落到下拉上；匹配不到的保留原值，交给用户确认。 */
async function applyPrefill(): Promise<void> {
  const source = props.prefill;
  if (!source) return;
  const matchedGroup = matchOptionIdByName(groups.value, source.groupName);
  if (!matchedGroup) return;
  groupId.value = matchedGroup;
  await loadInstances();
  const matchedInstance = matchOptionIdByName(instances.value, source.instanceName);
  if (!matchedInstance) return;
  instanceId.value = matchedInstance;
  await loadDatabases();
  dbName.value = source.dbName.trim();
}

onMounted(async () => {
  await loadGroups();
  await applyPrefill();
});
</script>

<template>
  <div class="page page-fill workflow-submit-page">
    <section class="panel card-fill workflow-submit-panel">
      <div class="panel-heading workflow-submit-head">
        <span class="workflow-submit-mark">
          <NavIcon name="audit" :size="12" />
        </span>
        <h2 class="panel-title">{{ t("workflow.submitPageTitle") }}</h2>
        <span class="spacer"></span>
        <button
          class="btn btn-default btn-xs workflow-back-btn"
          :title="t('privilege.back')"
          @click="emit('back')"
        >
          <NavIcon name="chevron" :size="12" class="workflow-back-icon" />
          <span>{{ t("privilege.back") }}</span>
        </button>
      </div>

      <div class="panel-body workflow-submit-body">
        <div v-if="prefill" class="alert alert-info workflow-notice workflow-prefill">
          <NavIcon name="info" :size="15" class="alert-icon" />
          <div class="alert-body">
            <span>{{ prefillNotice }}</span>
          </div>
        </div>

        <div class="workflow-submit-main">
          <div class="workflow-submit-editor">
            <div class="workflow-editor-bar">
              <span class="workflow-editor-bar-title">
                <NavIcon name="query" :size="12" />
                <span>{{ t("workflow.editorTitle") }}</span>
              </span>
              <span class="spacer"></span>
              <span class="workflow-editor-stat">
                {{ t("workflow.editorLines", { n: sqlStats.lines }) }}
              </span>
              <span class="workflow-editor-sep">·</span>
              <span class="workflow-editor-stat">
                {{ t("workflow.editorChars", { n: sqlStats.chars }) }}
              </span>
            </div>
            <SqlEditor v-model="sql" :placeholder="t('workflow.sqlPlaceholder')" />
          </div>

          <aside class="workflow-submit-side">
            <div class="workflow-submit-file">
              <span
                class="workflow-file-name"
                :class="{ 'is-empty': !fileName }"
                :title="fileName || t('workflow.sqlFileHint')"
              >
                <NavIcon :name="fileName ? 'file' : 'info'" :size="12" />
                <span class="workflow-file-label">
                  {{ fileName || t("workflow.sqlFileHint") }}
                </span>
              </span>
              <button
                v-if="fileName"
                type="button"
                class="btn btn-default btn-xs workflow-file-remove"
                :title="t('workflow.sqlFileRemove')"
                @click="clearFile"
              >
                <NavIcon name="trash" :size="12" />
                <span>{{ t("workflow.sqlFileRemove") }}</span>
              </button>
              <label class="btn btn-primary btn-xs workflow-file-btn">
                <NavIcon name="folder" :size="12" />
                <span>{{ t("workflow.sqlFilePick") }}</span>
                <input type="file" accept=".sql" @change="onFileChange" />
              </label>
            </div>

            <div class="workflow-field-group">
              <p class="workflow-field-legend">{{ t("workflow.groupBasic") }}</p>
              <input
                v-model="title"
                class="form-control form-control-sm"
                type="text"
                maxlength="50"
                :placeholder="t('workflow.submitNamePlaceholder')"
              />
              <input
                v-model="demandUrl"
                class="form-control form-control-sm"
                type="text"
                :placeholder="t('workflow.demandUrlPlaceholder')"
              />
            </div>

            <div class="workflow-field-group">
              <p class="workflow-field-legend">{{ t("workflow.groupTarget") }}</p>
              <select
                v-model.number="groupId"
                class="form-control form-control-sm"
                :aria-label="t('workflow.pickGroup')"
                :disabled="loadingGroups"
                @change="onGroupChange"
              >
                <option :value="0" disabled>{{ t("workflow.pickGroup") }}</option>
                <option v-for="group in groups" :key="group.id" :value="group.id">
                  {{ group.name }}
                </option>
              </select>
              <div v-if="!loadingGroups && groups.length === 0" class="subtle">
                {{ t("workflow.groupEmpty") }}
              </div>

              <select
                v-model.number="instanceId"
                class="form-control form-control-sm"
                :aria-label="t('workflow.pickInstance')"
                :disabled="!groupId || loadingInstances"
                @change="onInstanceChange"
              >
                <option :value="0" disabled>{{ t("workflow.pickInstance") }}</option>
                <option v-for="item in instances" :key="item.id" :value="item.id">
                  {{ item.name }}
                </option>
              </select>
              <div v-if="groupId && !loadingInstances && instances.length === 0" class="subtle">
                {{ t("workflow.instanceEmpty") }}
              </div>

              <select
                v-model="dbName"
                class="form-control form-control-sm"
                :aria-label="t('workflow.pickDatabase')"
                :disabled="!instanceId || loadingDatabases"
              >
                <option value="" disabled>{{ t("workflow.pickDatabase") }}</option>
                <option v-for="db in dbOptions" :key="db" :value="db">{{ db }}</option>
              </select>
            </div>

            <div class="workflow-field-group">
              <p class="workflow-field-legend">{{ t("workflow.groupExec") }}</p>
              <DateTimeRangeField
                v-model:start="runDateStart"
                v-model:end="runDateEnd"
              />
              <select
                v-if="backupEnabled"
                v-model="backup"
                class="form-control form-control-sm"
                :aria-label="t('workflow.backup')"
              >
                <option :value="true">{{ t("workflow.backupOn") }}</option>
                <option :value="false">{{ t("workflow.backupOff") }}</option>
              </select>
              <p v-if="auditorsDisplay" class="workflow-auditors">
                <NavIcon name="check" :size="12" class="workflow-auditors-icon" />
                <span class="workflow-auditors-label">{{ t("workflow.auditors") }}</span>
                <b>{{ auditorsDisplay }}</b>
              </p>
            </div>

            <div class="workflow-submit-actions">
              <button
                class="btn btn-sm workflow-btn-quiet"
                :disabled="submitting"
                @click="beautify"
              >
                {{ t("workflow.beautify") }}
              </button>
              <button
                class="btn btn-danger btn-sm workflow-btn-check"
                :disabled="checking"
                @click="runCheck"
              >
                <span v-if="checking" class="spinner"></span>
                <NavIcon v-else name="check" :size="13" />
                <span>{{ t("workflow.check") }}</span>
              </button>
              <button
                class="btn btn-success btn-sm"
                :disabled="submitting || !canSubmit"
                @click="submit()"
              >
                <span v-if="submitting" class="spinner"></span>
                <NavIcon v-else name="chevron" :size="13" />
                <span>{{ t("workflow.submitButton") }}</span>
              </button>
            </div>
          </aside>
        </div>

        <div v-if="error" class="error-box workflow-submit-error">{{ error }}</div>

        <div v-if="confirmPending" class="alert alert-warning workflow-confirm">
          <span>
            {{
              t("workflow.checkConfirmBody", {
                errors: checkSummary?.errorCount ?? 0,
                warnings: checkSummary?.warningCount ?? 0,
              })
            }}
          </span>
          <button class="btn btn-warning btn-xs" :disabled="submitting" @click="submit(true)">
            {{ t("workflow.checkConfirmAction") }}
          </button>
        </div>

        <section class="panel workflow-check-panel">
          <div class="panel-heading workflow-check-head">
            <span class="workflow-check-mark">
              <NavIcon name="check" :size="12" />
            </span>
            <h2 class="panel-title">{{ t("workflow.checkPanel") }}</h2>
            <span class="spacer"></span>
            <span v-if="checkSummary" class="workflow-check-summary" :class="{
              'is-bad': checkSummary.errorCount > 0,
              'is-warn': checkSummary.errorCount === 0 && checkSummary.warningCount > 0,
            }">
              <NavIcon
                :name="checkSummary.errorCount > 0 || checkSummary.warningCount > 0 ? 'info' : 'check'"
                :size="12"
              />
              <span>
                {{
                  t("workflow.checkSummary", {
                    errors: checkSummary.errorCount,
                    warnings: checkSummary.warningCount,
                  })
                }}
              </span>
            </span>
          </div>
          <div class="panel-body workflow-check-body">
            <div v-if="checkError" class="error-box">{{ checkError }}</div>
            <div v-else-if="checking" class="empty">
              <span class="spinner"></span>
              <span>{{ t("workflow.checking") }}</span>
            </div>
            <template v-else-if="checkRows.length > 0">
              <div class="workflow-check-toolbar">
                <label class="workflow-check-search">
                  <NavIcon name="search" :size="12" class="workflow-check-search-icon" />
                  <input
                    v-model="checkKeyword"
                    class="form-control form-control-sm workflow-sql-search"
                    type="search"
                    :placeholder="t('app.search')"
                  />
                </label>
                <span class="spacer"></span>
              </div>
              <div class="grid-wrap">
                <table class="data-table compact workflow-check-table">
                  <thead>
                    <tr>
                      <th class="workflow-grid-caret"></th>
                      <th>{{ t("workflow.colId") }}</th>
                      <th>{{ t("workflow.colSqlStatement") }}</th>
                      <th>{{ t("workflow.colAffectedRowsText") }}</th>
                      <th>{{ t("workflow.colAuditStatus") }}</th>
                      <th>{{ t("workflow.colAuditInfo") }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <template v-for="row in visibleCheckRows" :key="row.id">
                      <tr :class="checkRowClass(row.errorLevel)">
                        <td class="workflow-grid-caret">
                          <button
                            class="op-btn workflow-caret-btn"
                            :title="
                              expandedCheck.includes(row.id)
                                ? t('workflow.collapseRow')
                                : t('workflow.expandRow')
                            "
                            @click="toggleCheckRow(row.id)"
                          >
                            <NavIcon
                              :name="expandedCheck.includes(row.id) ? 'minus' : 'plus'"
                              :size="11"
                            />
                          </button>
                        </td>
                        <td class="workflow-check-id">{{ row.id }}</td>
                        <td class="workflow-grid-sql" :title="row.sql">{{ row.sql || "-" }}</td>
                        <td class="workflow-check-rows">{{ row.affectedRows }}</td>
                        <td>
                          <span
                            class="privilege-status workflow-check-status"
                            :class="checkStatusTone(row.errorLevel)"
                          >
                            {{ checkStatusText(row.errorLevel) }}
                          </span>
                        </td>
                        <td class="workflow-check-message">
                          <div class="workflow-check-message-text" :title="row.errorMessage || ''">
                            {{ auditMessageText(row.errorMessage) }}
                          </div>
                        </td>
                      </tr>
                      <tr v-if="expandedCheck.includes(row.id)" class="workflow-grid-expanded">
                        <td colspan="6">
                          <pre class="workflow-grid-sql-full">{{ row.sql }}</pre>
                          <div v-if="row.errorMessage" class="workflow-check-message-full">
                            <span class="workflow-check-message-full-label">
                              {{ t("workflow.colAuditInfo") }}
                            </span>
                            <ul class="workflow-check-message-list">
                              <li
                                v-for="(item, index) in splitAuditMessages(row.errorMessage)"
                                :key="index"
                              >
                                {{ item }}
                              </li>
                            </ul>
                          </div>
                        </td>
                      </tr>
                    </template>
                  </tbody>
                </table>
              </div>
              <div class="workflow-record-count">
                {{
                  t("workflow.records", {
                    from: visibleCheckRows.length === 0 ? 0 : 1,
                    to: visibleCheckRows.length,
                    total: visibleCheckRows.length,
                  })
                }}
              </div>
            </template>
            <div v-else class="empty workflow-check-empty">
              <span class="workflow-check-empty-icon">
                <NavIcon name="check" :size="16" />
              </span>
              <span class="workflow-check-empty-text">
                <strong>{{ t("workflow.checkEmpty") }}</strong>
                <span>{{ t("workflow.checkEmptyHint") }}</span>
              </span>
            </div>
          </div>
        </section>
      </div>
    </section>
  </div>
</template>
