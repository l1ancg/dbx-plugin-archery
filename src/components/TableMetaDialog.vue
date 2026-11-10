<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { metaPairs, rowTexts } from "../lib/table-meta";
import type { TableMetadata } from "../types";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  instanceName: string;
  instanceId?: number;
  dbName: string;
  dbType?: string;
  tableName: string;
}>();

const emit = defineEmits<{
  close: [];
  toast: [text: string, error?: boolean];
}>();

const data = ref<TableMetadata | null>(null);
const loading = ref(true);
const error = ref("");
const dialogEl = ref<HTMLElement | null>(null);

const metaRows = computed(() => (data.value ? metaPairs(data.value.meta) : []));
const descColumns = computed(() => data.value?.desc.columns ?? []);
const descRows = computed(() =>
  (data.value?.desc.rows ?? []).map((row) => rowTexts(data.value!.desc, row)),
);
const indexColumns = computed(() => data.value?.index.columns ?? []);
const indexRows = computed(() =>
  (data.value?.index.rows ?? []).map((row) => rowTexts(data.value!.index, row)),
);

/** 未知字段直接显示原始列名，避免出现 meta.field.xxx 这类裸 key。 */
function fieldLabel(key: string): string {
  const label = t(`meta.field.${key}`);
  return label.startsWith("meta.field.") ? key : label;
}

async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    data.value = await archeryApi.tableMeta({
      instanceName: props.instanceName,
      instanceId: props.instanceId,
      dbName: props.dbName,
      tbName: props.tableName,
      dbType: props.dbType,
    });
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loading.value = false;
  }
}

async function copyName(): Promise<void> {
  try {
    await archeryApi.copy(props.tableName);
    emit("toast", `${t("dict.copied")} ${props.tableName}`);
  } catch (issue) {
    emit("toast", describeError(issue), true);
  }
}

/** 列名走等宽字；索引标记（PRI/MUL/UNI）按值点亮，MySQL / PG 都成立。 */
function cellClass(cell: string, index: number): Record<string, boolean> {
  return { "is-name": index === 0, "is-key": /^(PRI|MUL|UNI)$/.test(cell) };
}

async function copyDdl(): Promise<void> {
  const sql = data.value?.createSql;
  if (!sql) return;
  try {
    await archeryApi.copy(sql);
    emit("toast", t("meta.ddlCopied"));
  } catch (issue) {
    emit("toast", describeError(issue), true);
  }
}

/** Esc 关闭：宿主页面没有全局 Esc 处理，弹窗自己收口。 */
function onKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape") emit("close");
}

onMounted(() => {
  window.addEventListener("keydown", onKeydown);
  void load();
  dialogEl.value?.focus();
});

onBeforeUnmount(() => window.removeEventListener("keydown", onKeydown));
</script>

<template>
  <div class="modal-mask" @click.self="emit('close')">
    <div
      ref="dialogEl"
      class="modal meta-modal"
      role="dialog"
      aria-modal="true"
      :aria-label="`${t('meta.title')} ${tableName}`"
      tabindex="-1"
    >
      <div class="modal-head meta-head">
        <div class="meta-head-main">
          <span class="meta-head-icon"><NavIcon name="table" :size="13" /></span>
          <div class="meta-head-text">
            <div class="meta-title">{{ t("meta.title") }}</div>
            <div class="meta-path">
              <span class="meta-table-name">{{ tableName }}</span>
              <template v-if="instanceName || dbName">
                <span class="meta-path-dot">·</span>
                <span v-if="instanceName">{{ instanceName }}</span>
                <span v-if="dbName" class="meta-path-dot">/</span>
                <span v-if="dbName">{{ dbName }}</span>
              </template>
            </div>
          </div>
        </div>
        <button
          class="modal-close"
          :title="t('app.close')"
          :aria-label="t('app.close')"
          @click="emit('close')"
        >
          ×
        </button>
      </div>

      <div class="modal-body meta-body">
        <div v-if="loading" class="meta-skeleton" aria-hidden="true">
          <div class="meta-skeleton-grid">
            <span v-for="cell in 12" :key="cell" class="meta-skeleton-cell"></span>
          </div>
          <div class="meta-skeleton-lines">
            <span v-for="line in 6" :key="line" class="meta-skeleton-line"></span>
          </div>
        </div>
        <div v-else-if="error" class="error-box">{{ t("meta.loadFailed") }}：{{ error }}</div>
        <div v-else-if="!data" class="empty">{{ t("meta.empty") }}</div>

        <template v-else>
          <section class="meta-block">
            <div class="meta-block-head">
              <span class="meta-block-title">{{ t("meta.tableInfo") }}</span>
              <span class="meta-block-rule"></span>
            </div>
            <div class="meta-grid">
              <div v-for="pair in metaRows" :key="pair.key" class="meta-pair">
                <span class="meta-label" :title="fieldLabel(pair.key)">{{
                  fieldLabel(pair.key)
                }}</span>
                <span class="meta-value" :title="pair.value">{{ pair.value }}</span>
              </div>
            </div>
          </section>

          <section class="meta-block">
            <div class="meta-block-head">
              <span class="meta-block-title">{{ t("meta.columns") }}</span>
              <span class="meta-block-rule"></span>
              <span class="meta-block-note">{{
                t("meta.columnCount", { n: descRows.length })
              }}</span>
            </div>
            <div class="meta-table-wrap">
              <table class="data-table meta-table">
                <thead>
                  <tr>
                    <th v-for="column in descColumns" :key="column">{{ column }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, rowIndex) in descRows" :key="rowIndex">
                    <td
                      v-for="(cell, cellIndex) in row"
                      :key="cellIndex"
                      :class="cellClass(cell, cellIndex)"
                    >
                      {{ cell }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="meta-block">
            <div class="meta-block-head">
              <span class="meta-block-title">{{ t("meta.indexes") }}</span>
              <span class="meta-block-rule"></span>
              <span class="meta-block-note">{{
                t("meta.indexCount", { n: indexRows.length })
              }}</span>
            </div>
            <div v-if="indexRows.length > 0" class="meta-table-wrap">
              <table class="data-table meta-table">
                <thead>
                  <tr>
                    <th v-for="column in indexColumns" :key="column">{{ column }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, rowIndex) in indexRows" :key="rowIndex">
                    <td
                      v-for="(cell, cellIndex) in row"
                      :key="cellIndex"
                      :class="cellClass(cell, cellIndex)"
                    >
                      {{ cell }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else class="meta-empty">{{ t("meta.noIndexes") }}</div>
          </section>

          <section class="meta-block">
            <div class="meta-block-head">
              <span class="meta-block-title">{{ t("meta.ddl") }}</span>
              <span class="meta-block-rule"></span>
              <button
                v-if="data.createSql"
                class="meta-copy"
                :title="t('meta.copyDdl')"
                @click="copyDdl"
              >
                <NavIcon name="copy" :size="11" />
                <span>{{ t("meta.copyDdl") }}</span>
              </button>
            </div>
            <pre v-if="data.createSql" class="meta-ddl">{{ data.createSql }}</pre>
            <div v-else class="meta-empty">{{ t("meta.noDdl") }}</div>
          </section>
        </template>
      </div>

      <div class="modal-foot">
        <button class="btn btn-default" @click="copyName">{{ t("meta.copyName") }}</button>
        <button class="btn" @click="emit('close')">{{ t("app.close") }}</button>
      </div>
    </div>
  </div>
</template>
