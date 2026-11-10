<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { writeClipboard } from "../lib/clipboard";
import { saveTextFile } from "../lib/download";
import {
  buildRollbackFile,
  filterRollbackRows,
  formatSqlPreview,
  normalizeRollbackRows,
  resolveRollbackTarget,
} from "../lib/workflow-detail";
import type { GroupInstance, SqlRollbackRow, SqlWorkflowOption, SqlWorkflowRow } from "../types";
import NavIcon from "../components/NavIcon.vue";

const props = defineProps<{
  row: SqlWorkflowRow;
}>();

const emit = defineEmits<{
  back: [];
  submitted: [];
  toast: [text: string];
}>();

const rows = ref<SqlRollbackRow[]>([]);
const loading = ref(true);
const error = ref("");
const keyword = ref("");
const dense = ref(false);
const expanded = ref<number[]>([]);
const submitOpen = ref(false);
const submitting = ref(false);
const submitError = ref("");
const groups = ref<SqlWorkflowOption[]>([]);
const instances = ref<GroupInstance[]>([]);
/** 按名称反查原工单组/实例 id 的进行状态：解析中禁止提交。 */
const resolving = ref(false);
const loadError = ref("");

const visibleRows = computed(() => filterRollbackRows(rows.value, keyword.value));

/** 回滚工单沿用原工单的库，只读展示、不提供编辑。 */
const dbName = computed(() => props.row.dbName.trim());

/** 提交目标固定为原工单的组/实例/库，任何一项解析不到都由 missing 拦住提交。 */
const target = computed(() =>
  resolveRollbackTarget({ row: props.row, groups: groups.value, instances: instances.value }),
);

const targetIssue = computed(() => {
  if (loadError.value) return loadError.value;
  if (resolving.value) return "";
  switch (target.value.missing) {
    case "group":
      return t("workflow.rollbackGroupUnmatched", { name: props.row.groupName || "-" });
    case "instance":
      return t("workflow.rollbackInstanceUnmatched", { name: props.row.instanceName || "-" });
    case "database":
      return t("workflow.rollbackDatabaseMissing");
    default:
      return "";
  }
});

const expandedAll = computed(
  () => visibleRows.value.length > 0 && expanded.value.length >= visibleRows.value.length,
);

/** 底栏记录数：与列表页、详情页同一句话（按搜索后的条数计）。 */
const recordText = computed(() =>
  t("workflow.records", {
    from: visibleRows.value.length === 0 ? 0 : 1,
    to: visibleRows.value.length,
    total: visibleRows.value.length,
  }),
);

/** Archery 回滚工单的固定命名：【回滚工单】原工单Id:{id} ,{原工单名}。 */
const rollbackTitle = computed(
  () => `【回滚工单】原工单Id:${props.row.id} ,${props.row.workflowName}`,
);

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
  error.value = "";
  try {
    const result = await archeryApi.sqlWorkflowRollback({ workflowId: props.row.id });
    rows.value = normalizeRollbackRows(result.rows, props.row.dbName);
    expanded.value = [];
  } catch (issue) {
    error.value = describeError(issue);
    rows.value = [];
  } finally {
    loading.value = false;
  }
}

/** 行内复制回滚语句：写剪贴板失败时提示手动选中，不静默吞掉。 */
async function copyRollback(item: SqlRollbackRow): Promise<void> {
  if (!item.rollbackSql) return;
  const copied = await writeClipboard(item.rollbackSql);
  emit("toast", copied ? t("app.copied") : t("workflow.copyFailed"));
}

async function downloadRollback(): Promise<void> {
  if (rows.value.length === 0) {
    emit("toast", t("workflow.rollbackEmpty"));
    return;
  }
  const saved = await saveTextFile(buildRollbackFile(rows.value), `rollback_${props.row.id}.sql`, {
    askPath: true,
  });
  if (saved.status === "saved") {
    emit("toast", t("workflow.rollbackSavedTo", { path: saved.path }));
  } else if (saved.status === "browser") {
    emit("toast", t("workflow.rollbackSaved"));
  }
}

async function openSubmit(): Promise<void> {
  if (rows.value.length === 0) return;
  submitOpen.value = true;
  submitError.value = "";
  loadError.value = "";
  await resolveTarget();
}

/**
 * 打开弹窗就按原工单的组名解析 group_id，再按原工单的实例名解析 instance_id。
 * 解析不到时只提示，绝不换到列表里的其它组/实例（回滚语句不能落到别的库上）。
 */
async function resolveTarget(): Promise<void> {
  resolving.value = true;
  try {
    if (groups.value.length === 0) {
      const options = await archeryApi.sqlSubmitOptions();
      groups.value = options.groups ?? [];
    }
    const group = groups.value.find((item) => item.id === target.value.groupId);
    if (!group) {
      instances.value = [];
      return;
    }
    instances.value = await archeryApi.sqlWorkflowTargets({ groupName: group.name });
  } catch (issue) {
    loadError.value = describeError(issue);
  } finally {
    resolving.value = false;
  }
}

async function submitRollback(): Promise<void> {
  submitError.value = "";
  const current = target.value;
  if (resolving.value) return;
  if (current.missing) {
    submitError.value = targetIssue.value;
    return;
  }
  submitting.value = true;
  try {
    const result = await archeryApi.sqlWorkflowRollbackSubmit({
      workflowId: props.row.id,
      title: rollbackTitle.value,
      groupId: current.groupId,
      instanceId: current.instanceId,
      dbName: current.dbName,
    });
    submitOpen.value = false;
    emit("toast", t("workflow.rollbackSubmitDone", { id: result.workflowId }));
    emit("submitted");
  } catch (issue) {
    submitError.value = describeError(issue);
  } finally {
    submitting.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="page page-fill workflow-detail-page rollback-page">
    <section class="panel card-fill workflow-detail-panel">
      <div class="panel-heading workflow-detail-top">
        <button class="tool-btn privilege-back" :title="t('privilege.back')" @click="emit('back')">
          <NavIcon name="chevron" :size="14" />
        </button>
        <span class="workflow-detail-crumb">
          {{ t("nav.auditOnline") }} / {{ t("workflow.rollbackTitle") }}
        </span>
        <span class="spacer"></span>
        <button class="tool-btn" :title="t('app.refresh')" @click="load">
          <NavIcon name="refresh" :size="14" />
        </button>
      </div>

      <div class="panel-body workflow-detail-scroll">
        <header class="rollback-head">
          <div class="workflow-title-row">
            <h3 class="workflow-title">{{ row.workflowName }}</h3>
            <span class="rollback-count-chip">
              {{ t("workflow.rollbackStatements", { n: rows.length }) }}
            </span>
            <span class="spacer"></span>
            <span class="rollback-order-hint">{{ t("workflow.rollbackOrderHint") }}</span>
          </div>
          <div class="workflow-meta">
            <span class="workflow-meta-item">
              <span class="workflow-meta-label">{{ t("workflow.colGroup") }}</span>
              <span class="workflow-meta-value" :title="row.groupName || '-'">
                {{ row.groupName || "-" }}
              </span>
            </span>
            <span class="workflow-meta-item">
              <span class="workflow-meta-label">{{ t("workflow.colInstance") }}</span>
              <span class="workflow-meta-value" :title="row.instanceName">{{ row.instanceName }}</span>
            </span>
            <span class="workflow-meta-item">
              <span class="workflow-meta-label">{{ t("workflow.colDatabase") }}</span>
              <span class="workflow-meta-value" :title="dbName || '-'">{{ dbName || "-" }}</span>
            </span>
          </div>
        </header>

        <div class="workflow-grid-toolbar">
          <button class="btn btn-default btn-xs" @click="toggleAll">
            {{ expandedAll ? t("workflow.collapseAll") : t("workflow.expandAll") }}
          </button>
          <span class="spacer"></span>
          <input
            v-model="keyword"
            class="form-control form-control-sm workflow-sql-search"
            type="search"
            :placeholder="t('workflow.searchSql')"
          />
          <button
            class="tool-btn"
            :class="{ 'is-on': dense }"
            :title="t('workflow.density')"
            @click="dense = !dense"
          >
            <NavIcon name="table" :size="14" />
          </button>
        </div>

        <div v-if="loading" class="empty">
          <span class="spinner"></span>
          <span>{{ t("app.loading") }}</span>
        </div>
        <div v-else-if="error" class="error-box">
          {{ t("workflow.rollbackFailed") }}：{{ error }}
        </div>
        <div v-else-if="visibleRows.length === 0" class="empty">
          {{ t("workflow.rollbackEmpty") }}
        </div>
        <div v-else class="grid-wrap workflow-grid-wrap">
          <table class="data-table workflow-grid rollback-grid" :class="{ compact: dense }">
            <thead>
              <tr>
                <th class="workflow-grid-caret"></th>
                <th class="rollback-grid-index">{{ t("workflow.rollbackColIndex") }}</th>
                <th>{{ t("workflow.rollbackColSql") }}</th>
                <th class="rollback-grid-rollback">{{ t("workflow.rollbackColRollback") }}</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="(item, index) in visibleRows" :key="item.id">
                <tr>
                  <td class="workflow-grid-caret">
                    <button
                      class="op-btn"
                      :title="
                        isExpanded(item.id) ? t('workflow.collapseRow') : t('workflow.expandRow')
                      "
                      @click="toggleRow(item.id)"
                    >
                      <NavIcon :name="isExpanded(item.id) ? 'minus' : 'plus'" :size="11" />
                    </button>
                  </td>
                  <td class="workflow-grid-id rollback-grid-index">#{{ index + 1 }}</td>
                  <td class="workflow-grid-sql">
                    <span
                      class="rollback-sql-clamp"
                      :class="{ 'is-empty': !item.sql }"
                      :title="item.sql"
                    >
                      {{ item.sql || "-" }}
                    </span>
                  </td>
                  <td class="workflow-grid-sql rollback-grid-rollback">
                    <span
                      class="rollback-sql-clamp"
                      :class="{ 'is-empty': !item.rollbackSql }"
                      :title="item.rollbackSql"
                    >
                      {{ item.rollbackSql || "-" }}
                    </span>
                    <button
                      class="workflow-copy-sql rollback-copy"
                      :title="t('workflow.rollbackCopy')"
                      :disabled="!item.rollbackSql"
                      @click="copyRollback(item)"
                    >
                      <NavIcon name="copy" :size="12" />
                    </button>
                  </td>
                </tr>
                <tr v-if="isExpanded(item.id)" class="workflow-grid-expanded">
                  <td colspan="4">
                    <div class="rollback-expanded-block">
                      <span class="rollback-expanded-label">
                        {{ t("workflow.rollbackColSql") }}
                      </span>
                      <pre class="workflow-grid-sql-full">{{ item.sql || "-" }}</pre>
                    </div>
                    <div class="rollback-expanded-block">
                      <span class="rollback-expanded-label">
                        {{ t("workflow.rollbackColRollback") }}
                      </span>
                      <pre class="workflow-grid-sql-full">{{ item.rollbackSql || "-" }}</pre>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>

      <div class="workflow-actions workflow-detail-actions rollback-actions">
        <span class="workflow-record-count">{{ recordText }}</span>
        <span class="spacer"></span>
        <button class="btn btn-default" :disabled="loading || rows.length === 0" @click="downloadRollback">
          {{ t("workflow.rollbackDownload") }}
        </button>
        <button class="btn btn-warning" :disabled="loading || rows.length === 0" @click="openSubmit">
          {{ t("workflow.rollbackSubmit") }}
        </button>
      </div>
    </section>

    <div v-if="submitOpen" class="modal-mask" @click.self="submitOpen = false">
      <div class="modal privilege-modal rollback-submit-modal">
        <div class="modal-head">
          <span>{{ t("workflow.rollbackSubmit") }}</span>
          <button class="modal-close" :title="t('app.close')" @click="submitOpen = false">×</button>
        </div>
        <div class="modal-body privilege-body">
          <div class="privilege-summary">
            <div class="privilege-summary-title">{{ rollbackTitle }}</div>
            <dl>
              <div>
                <dt>{{ t("workflow.colGroup") }}</dt>
                <dd>{{ row.groupName || "-" }}</dd>
              </div>
              <div>
                <dt>{{ t("workflow.colInstance") }}</dt>
                <dd>{{ row.instanceName }}</dd>
              </div>
              <div>
                <dt>{{ t("workflow.colDatabase") }}</dt>
                <dd>{{ dbName || "-" }}</dd>
              </div>
              <div>
                <dt>{{ t("workflow.rollbackColRollback") }}</dt>
                <dd>{{ t("workflow.rollbackStatements", { n: rows.length }) }}</dd>
              </div>
            </dl>
            <p class="subtle rollback-fixed-note">{{ t("workflow.rollbackTargetFixed") }}</p>
          </div>

          <div v-if="rows.length > 0" class="rollback-submit-list">
            <div v-for="(item, index) in rows" :key="item.id" class="rollback-submit-item">
              <div class="rollback-submit-block">
                <span class="rollback-submit-label">
                  <span class="rollback-submit-index">#{{ index + 1 }}</span>
                  {{ t("workflow.rollbackColSql") }}
                </span>
                <pre class="rollback-submit-sql">{{ formatSqlPreview(item.sql) || "-" }}</pre>
              </div>
              <div class="rollback-submit-block is-rollback">
                <span class="rollback-submit-label">
                  {{ t("workflow.rollbackColRollback") }}
                </span>
                <pre class="rollback-submit-sql">{{ formatSqlPreview(item.rollbackSql) || "-" }}</pre>
              </div>
            </div>
          </div>

          <div v-if="resolving" class="empty">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="targetIssue" class="error-box">{{ targetIssue }}</div>

          <p class="privilege-hint">{{ t("workflow.rollbackSubmitHint") }}</p>
          <div v-if="submitError" class="error-box">{{ submitError }}</div>
        </div>
        <div class="modal-foot">
          <button class="btn btn-default" :disabled="submitting" @click="submitOpen = false">
            {{ t("app.cancel") }}
          </button>
          <button
            class="btn btn-warning"
            :disabled="submitting || resolving || Boolean(targetIssue)"
            @click="submitRollback"
          >
            <span v-if="submitting" class="spinner"></span>
            {{ t("app.confirm") }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
