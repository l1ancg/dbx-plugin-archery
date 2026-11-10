<script setup lang="ts">
import { ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { syntaxTypeLabel } from "../lib/workflow";
import type { SqlWorkflowRow } from "../types";

const props = defineProps<{
  row: SqlWorkflowRow;
}>();

const emit = defineEmits<{
  close: [];
  audited: [];
  toast: [text: string];
}>();

const remark = ref("");
const submitting = ref(false);
const error = ref("");

async function submit(action: "pass" | "reject"): Promise<void> {
  error.value = "";
  // Archery 的驳回走 /cancel/，cancel_remark 实际允许空串，这里仍要求填写以便追溯。
  if (action === "reject" && !remark.value.trim()) {
    error.value = t("workflow.auditRemarkRequired");
    return;
  }
  submitting.value = true;
  try {
    await archeryApi.sqlWorkflowAudit({
      workflowId: props.row.id,
      action,
      remark: remark.value.trim(),
    });
    emit("toast", t("workflow.auditDone"));
    emit("audited");
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="modal-mask" @click.self="emit('close')">
    <div class="modal privilege-modal">
      <div class="modal-head">
        <span>{{ t("workflow.auditTitle") }}</span>
        <button class="modal-close" :title="t('app.close')" @click="emit('close')">×</button>
      </div>

      <div class="modal-body privilege-body">
        <div class="privilege-summary">
          <div class="privilege-summary-title">{{ row.workflowName }}</div>
          <dl>
            <div>
              <dt>{{ t("workflow.colApplicant") }}</dt>
              <dd>{{ row.engineerDisplay || "-" }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colInstance") }}</dt>
              <dd>{{ row.instanceName }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colDatabase") }}</dt>
              <dd>{{ row.dbName }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colType") }}</dt>
              <dd>{{ syntaxTypeLabel(row.syntaxType) || "-" }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colBackup") }}</dt>
              <dd>{{ row.isBackup ? t("workflow.backupYes") : t("workflow.backupNo") }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colGroup") }}</dt>
              <dd>{{ row.groupName }}</dd>
            </div>
          </dl>
        </div>

        <label class="privilege-field">
          <span>{{ t("workflow.auditRemark") }}</span>
          <textarea
            v-model="remark"
            class="form-control privilege-textarea"
            rows="3"
            :placeholder="t('workflow.auditRemarkPlaceholder')"
          ></textarea>
        </label>
        <div v-if="error" class="error-box">{{ error }}</div>
      </div>

      <div class="modal-foot">
        <button class="btn btn-default" :disabled="submitting" @click="emit('close')">
          {{ t("app.cancel") }}
        </button>
        <button class="btn btn-warning" :disabled="submitting" @click="submit('reject')">
          {{ t("workflow.auditReject") }}
        </button>
        <button class="btn btn-success" :disabled="submitting" @click="submit('pass')">
          {{ t("workflow.auditPass") }}
        </button>
      </div>
    </div>
  </div>
</template>
