<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { normalizeWorkflowLog } from "../lib/privilege";
import type { WorkflowLogRow } from "../types";
import NavIcon from "./NavIcon.vue";

/**
 * 行数据同时兼容查询权限工单（applyId/title）与 SQL 上线工单（id/workflowName）；
 * workflow_type 默认 1（查询权限），SQL 上线传 2。
 */
const props = defineProps<{
  row: {
    applyId?: number;
    id?: number;
    title?: string;
    workflowName?: string;
  };
  workflowType?: number;
}>();

const emit = defineEmits<{
  close: [];
}>();

const rows = ref<WorkflowLogRow[]>([]);
const loading = ref(true);
const error = ref("");

const workflowId = computed(() => props.row.applyId ?? props.row.id ?? 0);
const title = computed(() => props.row.title ?? props.row.workflowName ?? "");

onMounted(async () => {
  try {
    const page = await archeryApi.workflowLogs({
      workflowId: workflowId.value,
      workflowType: props.workflowType ?? 1,
    });
    rows.value = (page.rows ?? []).map((item) =>
      normalizeWorkflowLog(item as unknown as Record<string, unknown>),
    );
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="modal-mask pa-mask" @click.self="emit('close')">
    <div class="modal privilege-modal privilege-log-modal pa-dialog">
      <header class="modal-head pa-head">
        <span class="pa-head-icon" aria-hidden="true">
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <circle cx="12" cy="12" r="8.2" />
            <path d="M12 7.6V12.3l3.2 1.9" />
          </svg>
        </span>
        <div class="pa-head-text">
          <h2 class="pa-title">{{ t("privilege.logTitle") }}</h2>
          <p v-if="title" class="pa-subtitle" :title="title">
            {{ t("privilege.logSubtitle", { title }) }}
          </p>
        </div>
        <button
          type="button"
          class="pa-close"
          :title="t('app.close')"
          :aria-label="t('app.close')"
          @click="emit('close')"
        >
          <NavIcon name="close" :size="14" />
        </button>
      </header>

      <div class="modal-body pa-body">
        <div v-if="loading" class="pa-log-state">
          <span class="spinner"></span>
          <span>{{ t("app.loading") }}</span>
        </div>
        <p v-else-if="error" class="pa-error" role="alert">
          <NavIcon name="info" :size="13" class="pa-hint-icon" />
          <span>{{ t("privilege.logFailed") }}：{{ error }}</span>
        </p>
        <div v-else-if="rows.length === 0" class="pa-log-state">
          {{ t("privilege.logEmpty") }}
        </div>
        <div v-else class="grid-wrap privilege-log-wrap">
          <table class="data-table privilege-log-table">
            <thead>
              <tr>
                <th>{{ t("privilege.logOperation") }}</th>
                <th>{{ t("privilege.logOperator") }}</th>
                <th>{{ t("privilege.logTime") }}</th>
                <th>{{ t("privilege.logInfo") }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(log, index) in rows" :key="index">
                <td>{{ log.operationType || "-" }}</td>
                <td>{{ log.operator || "-" }}</td>
                <td class="nowrap">{{ log.time || "-" }}</td>
                <td>{{ log.info || "-" }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="modal-foot pa-foot">
        <p v-if="!loading && !error" class="pa-foot-note">
          <NavIcon name="file" :size="12" />
          {{ t("privilege.logRecords", { n: rows.length }) }}
        </p>
        <button type="button" class="btn btn-default pa-btn" @click="emit('close')">
          {{ t("app.close") }}
        </button>
      </div>
    </div>
  </div>
</template>
