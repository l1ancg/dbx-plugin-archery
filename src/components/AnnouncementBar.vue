<script setup lang="ts">
import type { Announcement } from "../types";
import NavIcon from "./NavIcon.vue";

defineProps<{
  /** Archery 首页公告；取不到时不渲染公告条。 */
  announcement?: Announcement | null;
}>();
</script>

<template>
  <div v-if="announcement" class="alert alert-info app-announcement" :class="announcement.level">
    <NavIcon name="info" :size="15" class="alert-icon" />
    <div class="alert-body">
      <strong v-if="announcement.title">{{ announcement.title }}</strong>
      <span>{{ announcement.content }}</span>
    </div>
  </div>
</template>
