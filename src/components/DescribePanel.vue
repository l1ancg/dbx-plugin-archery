<script setup lang="ts">
import { computed } from "vue";
import { t } from "../i18n";
import { formatCell } from "../lib/result";
import type { DescribeResult } from "../types";

const props = defineProps<{
  loading: boolean;
  error: string;
  table: string;
  data: DescribeResult | null;
  /** 本次 describe 请求耗时（秒）。 */
  elapsed?: number;
  /** 请求完成时刻，格式化为本地时间字符串。 */
  fetchedAt?: string;
}>();

const emit = defineEmits<{
  copy: [text: string, label: string];
}>();

/** Archery 的 describe 视图会把引擎异常变成 500，补一句可执行的替代路径。 */
const httpHint = computed(() => (/HTTP 5\d\d/.test(props.error) ? t("describe.http500Hint") : ""));

function copyDdl(): void {
  if (!props.data?.ddl) return;
  emit("copy", props.data.ddl, t("describe.copyDdl"));
}
</script>

<template>
  <div class="panel-body">
    <div class="panel-toolbar">
      <span v-if="table" class="mono">{{ table }}</span>
      <span v-if="elapsed !== undefined" class="subtle">
        {{ t("describe.queryTime", { time: elapsed.toFixed(2) }) }}
      </span>
      <span v-if="fetchedAt" class="subtle">{{ t("describe.fetchTime", { time: fetchedAt }) }}</span>
      <span class="spacer"></span>
      <button v-if="data?.ddl" class="btn btn-default btn-xs" @click="copyDdl">
        {{ t("describe.copyDdl") }}
      </button>
    </div>

    <div v-if="loading" class="empty">
      <span class="spinner"></span>
      <span>{{ t("describe.loading") }}</span>
    </div>
    <div v-else-if="error" class="error-box">
      {{ error }}
      <div v-if="httpHint" class="error-hint">{{ httpHint }}</div>
    </div>
    <div v-else-if="!data" class="empty">{{ t("describe.empty") }}</div>

    <div v-else-if="data.ddl" class="ddl-wrap">
      <pre class="ddl-body">{{ data.ddl }}</pre>
    </div>

    <div v-else-if="data.columns.length === 0" class="empty">{{ t("result.noColumns") }}</div>

    <template v-else>
      <div class="ddl-hint">{{ t("describe.noDdl") }}</div>
      <div class="grid-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th v-for="(column, index) in data.columns" :key="`${column}-${index}`">
                {{ column }}
                <span v-if="data.types?.[index]" class="col-type">{{ data.types[index] }}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, rowIndex) in data.rows" :key="rowIndex">
              <td
                v-for="(value, cellIndex) in row"
                :key="cellIndex"
                :class="{ null: value === null || value === undefined }"
                :title="String(value ?? 'NULL')"
              >
                {{ formatCell(value) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
