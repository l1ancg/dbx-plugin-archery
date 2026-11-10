<script setup lang="ts">
import { t } from "../i18n";
import DescribePanel from "../components/DescribePanel.vue";
import type { DescribeResult } from "../types";

defineProps<{
  loading: boolean;
  error: string;
  table: string;
  data: DescribeResult | null;
  elapsed?: number;
  fetchedAt?: string;
}>();

const emit = defineEmits<{
  copy: [text: string, label: string];
}>();
</script>

<template>
  <div class="page page-fill">
    <section class="panel card-fill">
      <div class="panel-heading">
        <h2 class="panel-title">{{ t("describe.title") }}</h2>
        <span class="spacer"></span>
        <span v-if="table" class="subtle">{{ table }}</span>
      </div>
      <DescribePanel
        :loading="loading"
        :error="error"
        :table="table"
        :data="data"
        :elapsed="elapsed"
        :fetched-at="fetchedAt"
        @copy="(text, label) => emit('copy', text, label)"
      />
    </section>
  </div>
</template>
