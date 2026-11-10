<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import {
  canAuditPrivilege,
  detailItems,
  normalizePrivilegeDetail,
  privilegeLevelKey,
  privilegeStatusMeta,
  reviewNodeText,
} from "../lib/privilege";
import type { PrivilegeApplyRow } from "../types";
import NavIcon from "../components/NavIcon.vue";
import PrivilegeAuditDialog from "../components/PrivilegeAuditDialog.vue";
import WorkflowLogDialog from "../components/WorkflowLogDialog.vue";

const props = defineProps<{
  row: PrivilegeApplyRow;
}>();

const emit = defineEmits<{
  back: [];
  audited: [];
  toast: [text: string];
}>();

const detail = ref(normalizePrivilegeDetail(null));
const loading = ref(true);
const detailError = ref("");
const auditOpen = ref(false);
const logOpen = ref(false);

/** 详情页面板是权威清单，类型不一致或缺清单时回退列表行。 */
const items = computed(() => detailItems(detail.value, props.row));
const status = computed(() => privilegeStatusMeta(props.row.status));

/** 结果集行数加千分位，与列表页的展示口径一致。 */
function formatLimit(value: number): string {
  if (!Number.isFinite(value)) return "-";
  return String(Math.trunc(value)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

async function load(): Promise<void> {
  loading.value = true;
  detailError.value = "";
  try {
    detail.value = normalizePrivilegeDetail(
      await archeryApi.privilegeDetail({ applyId: props.row.applyId }),
    );
  } catch (issue) {
    detailError.value = describeError(issue);
  } finally {
    loading.value = false;
  }
}

function onAudited(): void {
  auditOpen.value = false;
  // 状态已变化，回列表由父组件刷新，避免详情页展示过期状态。
  emit("audited");
}

onMounted(load);
</script>

<template>
  <div class="page page-fill privilege-page privilege-detail-page">
    <section class="panel card-fill privilege-detail-panel">
      <div class="panel-heading privilege-toolbar privilege-detail-top">
        <button
          class="tool-btn privilege-back"
          :title="t('privilege.back')"
          @click="emit('back')"
        >
          <NavIcon name="chevron" :size="14" />
        </button>
        <span class="privilege-detail-crumb">{{ t("nav.queryGrant") }}</span>
        <NavIcon name="chevron" :size="11" class="privilege-crumb-sep" />
        <span class="privilege-detail-crumb is-current">{{ t("privilege.detailTitle") }}</span>
        <span class="spacer"></span>
        <button class="tool-btn" :title="t('app.refresh')" @click="load">
          <NavIcon name="refresh" :size="14" />
        </button>
        <button class="btn btn-default btn-xs" @click="logOpen = true">
          {{ t("privilege.log") }}
        </button>
        <button
          v-if="canAuditPrivilege(row.status)"
          class="btn btn-warning btn-xs"
          @click="auditOpen = true"
        >
          {{ t("privilege.audit") }}
        </button>
      </div>

      <div class="panel-body privilege-detail-body">
        <header class="privilege-detail-hero">
          <h3 class="privilege-detail-heading">
            <span class="privilege-detail-kicker">{{ t("privilege.ticketTitle") }}<span class="privilege-detail-name">{{ row.title }}</span></span>
            <span class="privilege-status" :class="`is-${status.tone}`">
              {{ t(status.labelKey) }}
            </span>
          </h3>
          <div class="privilege-detail-meta">
            <span class="privilege-detail-meta-item">
              <span class="privilege-detail-meta-label">{{ t("privilege.colApplicant") }}</span>
              <span class="privilege-detail-meta-value">{{ row.userDisplay || "-" }}</span>
            </span>
            <span class="privilege-detail-meta-item">
              <span class="privilege-detail-meta-label">{{ t("privilege.colInstance") }}</span>
              <span class="privilege-detail-meta-value is-mono">{{ row.instanceName || "-" }}</span>
            </span>
            <span class="privilege-detail-meta-item">
              <span class="privilege-detail-meta-label">{{ t("privilege.colPrivType") }}</span>
              <span class="privilege-detail-meta-value">
                {{ t(privilegeLevelKey(row.privType)) }}
              </span>
            </span>
            <span class="privilege-detail-meta-item">
              <span class="privilege-detail-meta-label">{{ t("privilege.colValidDate") }}</span>
              <span class="privilege-detail-meta-value is-mono">{{ row.validDate || "-" }}</span>
            </span>
            <span class="privilege-detail-meta-item">
              <span class="privilege-detail-meta-label">{{ t("privilege.colCreateTime") }}</span>
              <span class="privilege-detail-meta-value is-mono">{{ row.createTime || "-" }}</span>
            </span>
          </div>
        </header>

        <section class="privilege-detail-block">
          <h4>{{ t("privilege.reviewFlow") }}</h4>
          <div v-if="detailError" class="alert-inline">
            {{ t("privilege.auditDetailFailed") }}：{{ detailError }}
          </div>
          <div v-else-if="loading" class="subtle privilege-detail-loading">
            <span class="spinner"></span>
            <span>{{ t("app.loading") }}</span>
          </div>
          <div v-else-if="detail.reviewNodes.length === 0" class="subtle">
            {{ t("privilege.reviewFlowEmpty") }}
          </div>
          <div v-else class="privilege-flow">
            <template v-for="(node, index) in detail.reviewNodes" :key="`${index}-${node.name}`">
              <span v-if="index > 0" class="privilege-flow-arrow">→</span>
              <span class="privilege-flow-node" :class="`is-${node.state}`">
                {{ reviewNodeText(node, t("privilege.autoPass")) }}
              </span>
            </template>
          </div>
        </section>

        <section class="privilege-detail-block">
          <h4>{{ t("privilege.otherInfo") }}</h4>
          <div class="grid-wrap privilege-detail-table-wrap">
            <table class="data-table privilege-detail-table">
              <thead>
                <tr>
                  <th>{{ t("privilege.colApplicant") }}</th>
                  <th>{{ t("privilege.colInstance") }}</th>
                  <th>{{ t("privilege.colPrivType") }}</th>
                  <th>{{ t("privilege.colDatabase") }}</th>
                  <th>{{ t("privilege.colLimit") }}</th>
                  <th>{{ t("privilege.colValidDate") }}</th>
                  <th>{{ t("privilege.colCreateTime") }}</th>
                  <th>{{ t("privilege.colStatus") }}</th>
                  <th>{{ t("privilege.colGroup") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>{{ row.userDisplay || "-" }}</td>
                  <td class="privilege-ident">{{ row.instanceName || "-" }}</td>
                  <td>
                    <span
                      class="privilege-level"
                      :class="row.privType === 1 ? 'is-database' : 'is-table'"
                    >
                      {{ t(privilegeLevelKey(row.privType)) }}
                    </span>
                  </td>
                  <td class="privilege-detail-db privilege-ident is-key" :title="row.dbList">
                    {{ row.dbList || "-" }}
                  </td>
                  <td class="privilege-num">{{ formatLimit(row.limitNum) }}</td>
                  <td class="nowrap privilege-valid-date">{{ row.validDate || "-" }}</td>
                  <td class="nowrap privilege-valid-date">{{ row.createTime || "-" }}</td>
                  <td>
                    <span class="privilege-status" :class="`is-${status.tone}`">
                      {{ t(status.labelKey) }}
                    </span>
                  </td>
                  <td>
                    <span class="privilege-group" :title="row.groupName">
                      {{ row.groupName || "-" }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="privilege-detail-block">
          <h4>{{ row.privType === 2 ? t("privilege.tableList") : t("privilege.dbList") }}</h4>
          <div class="privilege-detail-items">
            <div v-for="item in items" :key="item" class="privilege-detail-item">{{ item }}</div>
            <div v-if="items.length === 0" class="subtle">{{ t("privilege.itemsEmpty") }}</div>
          </div>
        </section>

        <section v-if="detail.lastOperation" class="privilege-detail-block">
          <h4>{{ t("privilege.lastOperation") }}</h4>
          <div class="privilege-last-operation">{{ detail.lastOperation }}</div>
        </section>
      </div>
    </section>

    <PrivilegeAuditDialog
      v-if="auditOpen"
      :row="row"
      @close="auditOpen = false"
      @audited="onAudited"
      @toast="(text) => emit('toast', text)"
    />
    <WorkflowLogDialog v-if="logOpen" :row="row" @close="logOpen = false" />
  </div>
</template>
