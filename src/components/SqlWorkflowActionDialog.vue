<script setup lang="ts">
import { computed, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { normalizeRunDateChange } from "../lib/workflow-detail";
import type { SqlWorkflowRow } from "../types";

/** 执行 / 定时执行 / 终止流程 / 可执行时间变更共用一个弹窗，靠 kind 区分。 */
const props = defineProps<{
  kind: "execute" | "schedule" | "cancel" | "alterRunDate";
  row: SqlWorkflowRow;
  /** 变更可执行时间时的当前窗口（来自详情「可执行时间范围」），无限制时为空串。 */
  runDateStart?: string;
  runDateEnd?: string;
}>();

const emit = defineEmits<{
  close: [];
  done: [];
  toast: [text: string];
}>();

const mode = ref<"auto" | "manual">("auto");
const runDate = ref("");
const rangeStart = ref(props.runDateStart ?? "");
const rangeEnd = ref(props.runDateEnd ?? "");
const remark = ref("");
const submitting = ref(false);
const error = ref("");

const titleKey = computed(() => {
  switch (props.kind) {
    case "schedule":
      return "workflow.scheduleTitle";
    case "cancel":
      return "workflow.cancelTitle";
    case "alterRunDate":
      return "workflow.alterRunDateTitle";
    default:
      return "workflow.executeTitle";
  }
});

const hintKey = computed(() => {
  switch (props.kind) {
    case "schedule":
      return "workflow.scheduleHint";
    case "cancel":
      return "workflow.cancelHint";
    case "alterRunDate":
      return "workflow.alterRunDateHint";
    default:
      return "workflow.executeHint";
  }
});

async function submit(): Promise<void> {
  error.value = "";
  submitting.value = true;
  try {
    if (props.kind === "execute") {
      await archeryApi.sqlWorkflowExecute({ workflowId: props.row.id, mode: mode.value });
      emit("toast", t("workflow.executeDone"));
    } else if (props.kind === "schedule") {
      if (!runDate.value) {
        error.value = t("workflow.scheduleRequired");
        submitting.value = false;
        return;
      }
      await archeryApi.sqlWorkflowSchedule({
        workflowId: props.row.id,
        runDate: runDate.value.replace("T", " "),
      });
      emit("toast", t("workflow.scheduleDone"));
    } else if (props.kind === "alterRunDate") {
      const range = normalizeRunDateChange(rangeStart.value, rangeEnd.value);
      if (range.error === "partial") {
        error.value = t("workflow.alterRunDatePartial");
        return;
      }
      if (range.error === "order") {
        error.value = t("workflow.alterRunDateOrder");
        return;
      }
      // 两端都空即清空窗口，Archery 侧落库为 None，详情页展示「无限制」。
      await archeryApi.sqlWorkflowAlterRunDate({
        workflowId: props.row.id,
        runDateStart: range.start,
        runDateEnd: range.end,
      });
      emit("toast", t("workflow.alterRunDateDone"));
    } else {
      if (!remark.value.trim()) {
        error.value = t("workflow.cancelRemarkRequired");
        submitting.value = false;
        return;
      }
      await archeryApi.sqlWorkflowAudit({
        workflowId: props.row.id,
        action: "reject",
        remark: remark.value.trim(),
      });
      emit("toast", t("workflow.cancelDone"));
    }
    emit("done");
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
        <span>{{ t(titleKey) }}</span>
        <button class="modal-close" :title="t('app.close')" @click="emit('close')">×</button>
      </div>

      <div class="modal-body privilege-body">
        <!-- 修改可执行时间只关心时间窗，工单摘要在这里是噪音；其余动作仍需要它定位工单。 -->
        <div v-if="kind !== 'alterRunDate'" class="privilege-summary">
          <div class="privilege-summary-title">{{ row.workflowName }}</div>
          <dl>
            <div>
              <dt>{{ t("workflow.colInstance") }}</dt>
              <dd>{{ row.instanceName }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colDatabase") }}</dt>
              <dd>{{ row.dbName }}</dd>
            </div>
            <div>
              <dt>{{ t("workflow.colApplicant") }}</dt>
              <dd>{{ row.engineerDisplay || "-" }}</dd>
            </div>
          </dl>
        </div>

        <template v-if="kind === 'execute'">
          <label class="privilege-field">
            <span>{{ t("workflow.executeMode") }}</span>
            <select v-model="mode" class="form-control form-control-sm">
              <option value="auto">{{ t("workflow.executeAuto") }}</option>
              <option value="manual">{{ t("workflow.executeManual") }}</option>
            </select>
          </label>
        </template>

        <template v-else-if="kind === 'schedule'">
          <label class="privilege-field">
            <span>{{ t("workflow.scheduleTime") }}</span>
            <input v-model="runDate" class="form-control form-control-sm" type="datetime-local" />
          </label>
        </template>

        <template v-else-if="kind === 'alterRunDate'">
          <label class="privilege-field">
            <span>{{ t("workflow.runDateStart") }}</span>
            <input
              v-model="rangeStart"
              class="form-control form-control-sm"
              type="datetime-local"
            />
          </label>
          <label class="privilege-field">
            <span>{{ t("workflow.runDateEnd") }}</span>
            <input v-model="rangeEnd" class="form-control form-control-sm" type="datetime-local" />
          </label>
        </template>

        <template v-else>
          <label class="privilege-field">
            <span>{{ t("workflow.cancelRemark") }}</span>
            <textarea
              v-model="remark"
              class="form-control privilege-textarea"
              rows="3"
              :placeholder="t('workflow.cancelRemarkPlaceholder')"
            ></textarea>
          </label>
        </template>

        <p class="privilege-hint">{{ t(hintKey) }}</p>
        <div v-if="error" class="error-box">{{ error }}</div>
      </div>

      <div class="modal-foot">
        <button class="btn btn-default" :disabled="submitting" @click="emit('close')">
          {{ t("app.cancel") }}
        </button>
        <button
          class="btn"
          :class="
            kind === 'cancel' ? 'btn-warning' : kind === 'alterRunDate' ? 'btn-info' : 'btn-success'
          "
          :disabled="submitting"
          @click="submit"
        >
          <span v-if="submitting" class="spinner"></span>
          {{
            kind === "execute"
              ? t("workflow.executeNow")
              : kind === "alterRunDate"
                ? t("workflow.alterRunDateSubmit")
                : t("app.confirm")
          }}
        </button>
      </div>
    </div>
  </div>
</template>
