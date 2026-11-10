<script setup lang="ts">
import { t } from "../i18n";
import NavIcon from "./NavIcon.vue";

const props = withDefaults(
  defineProps<{
    tone: "blue" | "green" | "orange" | "red";
    icon: string;
    value: string;
    label: string;
    loading?: boolean;
    /** 没有目标列表页的指标不展示底栏「更多」。 */
    more?: boolean;
  }>(),
  { loading: false, more: true },
);

const emit = defineEmits<{
  more: [];
}>();
</script>

<template>
  <article class="stat-card" :class="`tone-${tone}`">
    <div class="stat-icon">
      <span class="stat-icon-chip">
        <NavIcon :name="icon" :size="22" />
      </span>
    </div>
    <div class="stat-body">
      <div class="stat-value" :class="{ loading }">{{ loading ? "…" : value }}</div>
      <div class="stat-label">{{ label }}</div>
    </div>
    <button v-if="props.more" class="stat-more" @click="emit('more')">
      <span>{{ t("dashboard.more") }}</span>
      <span class="stat-more-arrow">›</span>
    </button>
  </article>
</template>
