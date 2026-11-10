<script setup lang="ts">
import { computed } from "vue";
import { t } from "../i18n";
import type { Route } from "../lib/nav";
import NavIcon from "../components/NavIcon.vue";

const props = defineProps<{ route: Route }>();

const emit = defineEmits<{
  back: [];
}>();

const icon = computed(() => {
  switch (props.route.page) {
    case "audit":
      return "audit";
    case "optimize":
      return "optimize";
    case "tools":
      return "tools";
    case "dict":
      return "table";
    case "grant":
      return "user";
    default:
      return "docs";
  }
});

const descKey = computed(() => {
  if (props.route.page === "audit") {
    return props.route.tab === "online" ? "placeholder.auditOnline" : "placeholder.auditAnalyze";
  }
  switch (props.route.page) {
    case "optimize":
      return "placeholder.optimize";
    case "tools":
      return "placeholder.tools";
    case "docs":
      return "placeholder.docs";
    case "dict":
      return "placeholder.dict";
    case "grant":
      return "placeholder.grant";
    default:
      return "placeholder.desc";
  }
});
</script>

<template>
  <div class="page page-fill">
    <section class="panel placeholder-card">
      <div class="placeholder-icon">
        <NavIcon :name="icon" :size="34" />
      </div>
      <h2>{{ t("placeholder.title") }}</h2>
      <p>{{ t(descKey) }}</p>
      <button class="btn btn-primary" @click="emit('back')">{{ t("placeholder.back") }}</button>
    </section>
  </div>
</template>
