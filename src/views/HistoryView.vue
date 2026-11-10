<script setup lang="ts">
import { t } from "../i18n";
import HistoryPanel from "../components/HistoryPanel.vue";
import type { InstanceItem, QueryLogRow } from "../types";

defineProps<{
  instances?: InstanceItem[];
}>();

const emit = defineEmits<{
  load: [row: QueryLogRow];
  toast: [text: string];
}>();
</script>

<template>
  <div class="page page-scroll history-page">
    <section class="panel history-panel">
      <div class="panel-heading">
        <h2 class="panel-title">{{ t("history.title") }}</h2>
      </div>
      <HistoryPanel
        :instances="instances"
        @load="(row) => emit('load', row)"
        @toast="(text) => emit('toast', text)"
      />
    </section>
  </div>
</template>
