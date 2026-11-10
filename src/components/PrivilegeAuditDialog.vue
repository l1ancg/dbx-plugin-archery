<script setup lang="ts">
import { onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { privTypeLabel, splitPrivilegeList } from "../lib/privilege";
import type { PrivilegeApplyRow } from "../types";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  row: PrivilegeApplyRow;
}>();

const emit = defineEmits<{
  close: [];
  audited: [];
  toast: [text: string];
}>();

const loading = ref(true);
const canReview = ref(false);
const detailError = ref("");
const remark = ref("");
const submitting = ref(false);
const error = ref("");

const databases = splitPrivilegeList(props.row.dbList);
const tables = splitPrivilegeList(props.row.tableList);

onMounted(async () => {
  try {
    const detail = await archeryApi.privilegeDetail({ applyId: props.row.applyId });
    canReview.value = detail.canReview;
  } catch (issue) {
    detailError.value = describeError(issue);
  } finally {
    loading.value = false;
  }
});

async function submit(status: number): Promise<void> {
  error.value = "";
  if (status === 2 && !remark.value.trim()) {
    error.value = t("privilege.auditRemarkRequired");
    return;
  }
  submitting.value = true;
  try {
    await archeryApi.privilegeAudit({
      applyId: props.row.applyId,
      auditStatus: status,
      auditRemark: remark.value.trim(),
    });
    emit("toast", t("privilege.auditDone"));
    emit("audited");
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="modal-mask pa-mask" @click.self="emit('close')">
    <div class="modal privilege-modal privilege-audit-modal pa-dialog">
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
            <rect x="5" y="4.2" width="14" height="16.6" rx="2.2" />
            <path d="M9 4.2V2.9h6v1.3" />
            <path d="M8.6 12.3l2.3 2.3 4.5-4.9" />
          </svg>
        </span>
        <div class="pa-head-text">
          <h2 class="pa-title">{{ t("privilege.auditTitle") }}</h2>
          <p class="pa-subtitle">{{ t("privilege.auditSubtitle") }}</p>
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
        <section class="pa-section">
          <div class="pa-section-head">
            <span class="pa-step">01</span>
            <h3 class="pa-section-title">{{ t("privilege.sectionTicket") }}</h3>
            <span class="pa-line"></span>
          </div>
          <div class="privilege-summary">
            <div class="privilege-summary-title">{{ row.title }}</div>
            <dl>
              <div>
                <dt>{{ t("privilege.detailApplicant") }}</dt>
                <dd>{{ row.userDisplay || "-" }}</dd>
              </div>
              <div>
                <dt>{{ t("privilege.detailInstance") }}</dt>
                <dd>{{ row.instanceName }}</dd>
              </div>
              <div>
                <dt>{{ t("privilege.colPrivType") }}</dt>
                <dd>{{ privTypeLabel(row.privType) }}</dd>
              </div>
              <div v-if="row.privType === 2">
                <dt>{{ t("privilege.detailDatabase") }}</dt>
                <dd>{{ row.dbList || "-" }}</dd>
              </div>
              <div>
                <dt>
                  {{ row.privType === 2 ? t("privilege.detailTable") : t("privilege.detailDatabase") }}
                </dt>
                <dd class="privilege-summary-list">
                  {{ (row.privType === 2 ? tables : databases).join("、") || "-" }}
                </dd>
              </div>
              <div>
                <dt>{{ t("privilege.detailLimit") }}</dt>
                <dd>{{ row.limitNum }}</dd>
              </div>
              <div>
                <dt>{{ t("privilege.detailValidDate") }}</dt>
                <dd>{{ row.validDate }}</dd>
              </div>
            </dl>
          </div>
        </section>

        <section v-if="canReview" class="pa-section">
          <div class="pa-section-head">
            <span class="pa-step">02</span>
            <h3 class="pa-section-title">{{ t("privilege.sectionReview") }}</h3>
            <span class="pa-line"></span>
          </div>
          <div class="pa-field">
            <label class="pa-label" for="pa-audit-remark">
              {{ t("privilege.auditRemark") }}
              <span class="pa-tag">{{ t("privilege.auditRemarkTag") }}</span>
            </label>
            <textarea
              id="pa-audit-remark"
              v-model="remark"
              class="privilege-textarea"
              rows="3"
              :placeholder="t('privilege.auditRemarkPlaceholder')"
            ></textarea>
            <p class="pa-note">{{ t("privilege.auditRemarkNote") }}</p>
          </div>
        </section>

        <div v-if="loading" class="pa-loading">
          <span class="spinner"></span>
          <span>{{ t("app.loading") }}</span>
        </div>
        <p v-else-if="detailError" class="pa-error" role="alert">
          <NavIcon name="info" :size="13" class="pa-hint-icon" />
          <span>{{ t("privilege.auditDetailFailed") }}：{{ detailError }}</span>
        </p>
        <p v-else-if="!canReview" class="pa-hint is-warn">
          <NavIcon name="info" :size="13" class="pa-hint-icon" />
          <span>{{ t("privilege.auditNoPermission") }}</span>
        </p>

        <p v-if="error" class="pa-error" role="alert">
          <NavIcon name="info" :size="13" class="pa-hint-icon" />
          <span>{{ error }}</span>
        </p>
      </div>

      <div class="modal-foot pa-foot">
        <p class="pa-foot-note">
          <NavIcon name="clock" :size="12" />
          {{ t("privilege.auditFootHint") }}
        </p>
        <button
          type="button"
          class="btn btn-default pa-btn"
          :disabled="submitting"
          @click="emit('close')"
        >
          {{ t("app.cancel") }}
        </button>
        <button
          v-if="canReview"
          type="button"
          class="btn btn-warning pa-btn pa-btn-danger"
          :disabled="submitting"
          @click="submit(2)"
        >
          {{ t("privilege.auditReject") }}
        </button>
        <button
          v-if="canReview"
          type="button"
          class="btn btn-success pa-btn pa-btn-primary"
          :disabled="submitting"
          @click="submit(1)"
        >
          <span v-if="submitting" class="pa-spinner"></span>
          {{ t("privilege.auditPass") }}
        </button>
      </div>
    </div>
  </div>
</template>
