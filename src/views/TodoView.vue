<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import {
  normalizeTodo,
  todoRowRange,
  todoStatusMeta,
  todoToPrivilegeRow,
  todoToSqlWorkflowRow,
  todoTypeLabelKey,
  todoTypeOptions,
} from "../lib/todo";
import type { PrivilegeApplyRow, SqlWorkflowRow, WorkflowTodoRow } from "../types";
import NavIcon from "../components/NavIcon.vue";
import PrivilegeDetailView from "./PrivilegeDetailView.vue";
import SqlWorkflowDetailView from "./SqlWorkflowDetailView.vue";

const emit = defineEmits<{
  toast: [text: string, error?: boolean];
  /** 每次拉到列表后回传总数，顶栏铃铛据此同步（审核完立刻跟着掉）。 */
  total: [count: number];
}>();

/** 与 Archery workflow.html 的 bootstrap-table 一致，每页 20 条。 */
const PAGE_SIZE = 20;

const rows = ref<WorkflowTodoRow[]>([]);
const total = ref(0);
const page = ref(1);
/** 0=全部，1=查询权限，2=SQL 上线，3=数据归档（Archery workflow_type）。 */
const workflowType = ref(0);
const keyword = ref("");
const dense = ref(false);
const loading = ref(false);
const error = ref("");
/** 审核入口解析出的详情目标：为空表示停留在列表。 */
const sqlDetail = ref<SqlWorkflowRow | null>(null);
const privilegeDetail = ref<PrivilegeApplyRow | null>(null);
/** 正在解析详情的 auditId，用来禁用对应行的审核按钮。 */
const opening = ref(0);
let debounce: ReturnType<typeof setTimeout> | undefined;

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)));
const range = computed(() => todoRowRange(page.value, PAGE_SIZE, total.value));

async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    const result = await archeryApi.workflowTodos({
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE,
      search: keyword.value.trim(),
      workflowType: workflowType.value,
    });
    rows.value = (result.rows ?? []).map((row) =>
      normalizeTodo(row as unknown as Record<string, unknown>),
    );
    total.value = result.total ?? 0;
    emit("total", total.value);
    // 审核完当前页最后一条后总数会缩小，页码越界时退回最后一页再拉一次。
    const lastPage = Math.max(1, Math.ceil(total.value / PAGE_SIZE));
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

function go(delta: number): void {
  const next = page.value + delta;
  if (next < 1 || next > pageCount.value) return;
  page.value = next;
  void load();
}

/**
 * 审核：待办行只有 audit_id，先让 sidecar 解析出业务工单号，
 * 再进入插件里已实现的工单详情页完成审核（与 Archery 网页的 302 跳转一致）。
 */
async function openTodo(row: WorkflowTodoRow): Promise<void> {
  if (opening.value) return;
  opening.value = row.auditId;
  try {
    const target = await archeryApi.workflowTodoTarget({ auditId: row.auditId });
    if (target.workflowType === 2) {
      sqlDetail.value = todoToSqlWorkflowRow(row, target.workflowId);
      return;
    }
    if (target.workflowType === 1) {
      privilegeDetail.value = todoToPrivilegeRow(row, target.workflowId);
      return;
    }
    // 数据归档工单插件内没有详情页，避免给一个走不通的入口。
    emit("toast", t("todo.archiveUnsupported"), true);
  } catch (issue) {
    emit("toast", `${t("todo.openFailed")}：${describeError(issue)}`, true);
  } finally {
    opening.value = 0;
  }
}

/** 审核完成或从详情返回：该工单可能已不在待办里，回列表重新拉一次。 */
function onDetailClosed(): void {
  sqlDetail.value = null;
  privilegeDetail.value = null;
  void load();
}

onMounted(load);
</script>

<template>
  <SqlWorkflowDetailView
    v-if="sqlDetail"
    :row="sqlDetail"
    @back="onDetailClosed"
    @updated="onDetailClosed"
    @toast="(text) => emit('toast', text)"
  />

  <PrivilegeDetailView
    v-else-if="privilegeDetail"
    :row="privilegeDetail"
    @back="onDetailClosed"
    @audited="onDetailClosed"
    @toast="(text) => emit('toast', text)"
  />

  <div v-else class="page page-fill">
    <h2 class="page-title todo-title">{{ t("todo.title") }}</h2>

    <section class="panel card-fill">
      <div class="panel-heading privilege-toolbar workflow-toolbar">
        <select
          v-model.number="workflowType"
          class="form-control form-control-sm workflow-filter todo-type-filter"
          :aria-label="t('todo.colType')"
          @change="changeFilter"
        >
          <option v-for="item in todoTypeOptions" :key="item.value" :value="item.value">
            {{ t(item.labelKey) }}
          </option>
        </select>

        <span class="workflow-toolbar-end">
          <input
            v-model="keyword"
            class="form-control form-control-sm privilege-search"
            type="search"
            :placeholder="t('todo.searchPlaceholder')"
            @input="onSearch"
          />
          <button class="tool-btn" :title="t('app.refresh')" @click="load">
            <NavIcon name="refresh" :size="14" />
          </button>
          <button
            class="tool-btn"
            :class="{ 'is-on': dense }"
            :title="t('todo.dense')"
            @click="dense = !dense"
          >
            <NavIcon name="table" :size="14" />
          </button>
        </span>
      </div>

      <div class="panel-body privilege-list-body">
        <div v-if="error" class="error-box">{{ t("todo.loadFailed") }}：{{ error }}</div>
        <div v-else-if="loading" class="empty">
          <span class="spinner"></span>
          <span>{{ t("app.loading") }}</span>
        </div>
        <div v-else-if="rows.length === 0" class="empty">{{ t("todo.empty") }}</div>

        <template v-else>
          <div class="grid-wrap">
            <table class="data-table todo-table" :class="{ compact: dense }">
              <thead>
                <tr>
                  <th>{{ t("todo.colTitle") }}</th>
                  <th>{{ t("todo.colGroup") }}</th>
                  <th>{{ t("todo.colType") }}</th>
                  <th>{{ t("todo.colApplicant") }}</th>
                  <th>{{ t("todo.colCreateTime") }}</th>
                  <th>{{ t("todo.colStatus") }}</th>
                  <th>{{ t("todo.colOperation") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in rows" :key="row.auditId">
                  <td class="todo-title-cell">
                    <button
                      class="privilege-title-link"
                      :title="row.title"
                      @click="openTodo(row)"
                    >
                      {{ row.title || "-" }}
                    </button>
                  </td>
                  <td>{{ row.groupName || "-" }}</td>
                  <td>{{ t(todoTypeLabelKey(row.workflowType)) }}</td>
                  <td>{{ row.applicant || "-" }}</td>
                  <td class="nowrap">{{ row.createTime || "-" }}</td>
                  <td>
                    <span
                      class="privilege-status"
                      :class="`is-${todoStatusMeta(row.status).tone}`"
                    >
                      {{ t(todoStatusMeta(row.status).labelKey) }}
                    </span>
                  </td>
                  <td class="privilege-ops">
                    <button
                      class="btn btn-quiet btn-xs"
                      :disabled="opening === row.auditId"
                      @click="openTodo(row)"
                    >
                      {{ t("todo.audit") }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager">
            <span>{{ t("workflow.records", { from: range.from, to: range.to, total }) }}</span>
            <span class="spacer"></span>
            <span class="subtle">{{ t("result.page", { page, pages: pageCount }) }}</span>
            <button class="btn ghost" :disabled="page <= 1" @click="go(-1)">
              {{ t("result.prev") }}
            </button>
            <button class="btn ghost" :disabled="page >= pageCount" @click="go(1)">
              {{ t("result.next") }}
            </button>
          </div>
        </template>
      </div>
    </section>
  </div>
</template>
