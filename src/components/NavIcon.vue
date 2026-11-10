<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  name: string;
  size?: number;
}>();

const px = computed(() => props.size ?? 16);
</script>

<template>
  <svg
    class="nav-icon"
    :width="px"
    :height="px"
    viewBox="0 0 16 16"
    fill="none"
    stroke="currentColor"
    stroke-width="1.35"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <g v-if="name === 'dashboard'">
      <circle cx="8" cy="8" r="5.7" />
      <path d="M8 8l3.6-3.6" />
      <path d="M8 2.3v1.3M2.3 8h1.3M12.4 8h1.3M8 13.7v-1.3" />
    </g>

    <g v-else-if="name === 'audit' || name === 'check'">
      <path d="M3.2 8.5l3.2 3.2 6.4-7.3" />
    </g>

    <g v-else-if="name === 'query' || name === 'search'">
      <circle cx="7.1" cy="7.1" r="4.2" />
      <path d="M10.3 10.3 14 14" />
    </g>

    <g v-else-if="name === 'optimize'">
      <path
        d="M10.6 2.4a3.6 3.6 0 0 0-4.3 4.6l-4 4a1.5 1.5 0 0 0 2.1 2.1l4-4a3.6 3.6 0 0 0 4.6-4.3L11.3 6.5 9.5 4.7z"
      />
    </g>

    <g v-else-if="name === 'tools'">
      <path d="M5.6 2.2v3.1M10.4 2.2v3.1" />
      <path d="M4.3 5.3h7.4v2.2a3.7 3.7 0 0 1-7.4 0z" />
      <path d="M8 11.2v2.6" />
    </g>

    <g v-else-if="name === 'docs'">
      <rect x="6.3" y="1.8" width="3.4" height="2.9" rx="0.6" />
      <rect x="1.9" y="11.3" width="3.4" height="2.9" rx="0.6" />
      <rect x="10.7" y="11.3" width="3.4" height="2.9" rx="0.6" />
      <path d="M8 4.7v3.2M3.6 11.3V7.9h8.8v3.4" />
    </g>

    <g v-else-if="name === 'bell'">
      <path d="M8 2.2a3.6 3.6 0 0 1 3.6 3.6v2.3l1 2.1H3.4l1-2.1V5.8A3.6 3.6 0 0 1 8 2.2z" />
      <path d="M6.6 12.1a1.6 1.6 0 0 0 2.8 0" />
    </g>

    <g v-else-if="name === 'question'">
      <circle cx="8" cy="8" r="5.8" />
      <path d="M6.3 6.2a1.8 1.8 0 1 1 2.5 1.7c-.5.2-.8.7-.8 1.2v.3" />
      <path d="M8 11.8v.2" />
    </g>

    <g v-else-if="name === 'gear'">
      <circle cx="8" cy="8" r="2.3" />
      <path
        d="M8 1.7v2M8 12.3v2M1.7 8h2M12.3 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4"
      />
    </g>

    <g v-else-if="name === 'lock'">
      <rect x="3.4" y="7" width="9.2" height="6.6" rx="1.2" />
      <path d="M5.6 7V5.2a2.4 2.4 0 0 1 4.8 0V7" />
    </g>

    <g v-else-if="name === 'alert'">
      <path d="M8 2.4 14 13H2z" />
      <path d="M8 6.2v3.4M8 11.4v.2" />
    </g>

    <g v-else-if="name === 'shield'">
      <path d="M8 1.9 13 3.6v4.1c0 3-2 5.3-5 6.4-3-1.1-5-3.4-5-6.4V3.6z" />
      <path d="M5.8 8.2 7.3 9.7l3-3.2" />
    </g>

    <g v-else-if="name === 'key'">
      <circle cx="5.5" cy="10.5" r="3" />
      <path d="M7.7 8.3 13.2 2.8M11.1 4.9l1.7 1.7M12.6 3.4l1.7 1.7" />
    </g>

    <g v-else-if="name === 'phone'">
      <rect x="4.6" y="1.8" width="6.8" height="12.4" rx="1.7" />
      <path d="M6.9 12.1h2.2" />
    </g>

    <g v-else-if="name === 'power'">
      <path d="M8 2v5" />
      <path d="M4.7 4.3a4.9 4.9 0 1 0 6.6 0" />
    </g>

    <g v-else-if="name === 'chevron'">
      <path d="M6 3.6 10.4 8 6 12.4" />
    </g>

    <g v-else-if="name === 'user'">
      <circle cx="8" cy="5.6" r="2.5" />
      <path d="M3.2 13.6a4.8 4.8 0 0 1 9.6 0" />
    </g>

    <g v-else-if="name === 'database'">
      <ellipse cx="8" cy="4.3" rx="4.6" ry="1.9" />
      <path d="M3.4 4.3v7.3c0 1.1 2.1 1.9 4.6 1.9s4.6-.8 4.6-1.9V4.3" />
      <path d="M3.4 8c0 1.1 2.1 1.9 4.6 1.9S12.6 9.1 12.6 8" />
    </g>

    <g v-else-if="name === 'chart'">
      <path d="M2.4 13.4h11.2" />
      <path d="M4.8 13.4V8.4M8 13.4V4.2M11.2 13.4V6.6" />
    </g>

    <g v-else-if="name === 'plus'">
      <path d="M8 3.2v9.6M3.2 8h9.6" />
    </g>

    <g v-else-if="name === 'minus'">
      <path d="M3.2 8h9.6" />
    </g>

    <g v-else-if="name === 'close'">
      <path d="M4.2 4.2l7.6 7.6M11.8 4.2l-7.6 7.6" />
    </g>

    <g v-else-if="name === 'refresh'">
      <path d="M13.1 8A5.1 5.1 0 1 1 11.5 4.2" />
      <path d="M13.1 2.6v2.9h-2.9" />
    </g>

    <g v-else-if="name === 'file'">
      <path d="M4 1.9h4.8L12.2 5.3v8.8H4z" />
      <path d="M8.6 1.9v3.6h3.6" />
    </g>

    <g v-else-if="name === 'folder'">
      <path d="M1.9 3.5h4.1l1.3 1.5h6.8v7.5H1.9z" />
    </g>

    <g v-else-if="name === 'trash'">
      <path d="M2.9 4.3h10.2" />
      <path d="M6.4 4.3V2.8h3.2v1.5" />
      <path d="M4.6 4.3l.6 8.9h5.6l.6-8.9" />
      <path d="M6.9 6.6v4.2M9.1 6.6v4.2" />
    </g>

    <g v-else-if="name === 'table'">
      <rect x="2.2" y="2.9" width="11.6" height="10.2" rx="0.8" />
      <path d="M2.2 6.3h11.6M2.2 9.7h11.6M6.2 2.9v10.2" />
    </g>

    <g v-else-if="name === 'columns'">
      <rect x="2.2" y="2.9" width="11.6" height="10.2" rx="0.8" />
      <path d="M6.1 2.9v10.2M9.9 2.9v10.2" />
    </g>

    <g v-else-if="name === 'copy'">
      <rect x="5.6" y="5.6" width="7.8" height="7.8" rx="1.1" />
      <path d="M10.6 5.6V3.7a1.1 1.1 0 0 0-1.1-1.1H3.7a1.1 1.1 0 0 0-1.1 1.1v5.8a1.1 1.1 0 0 0 1.1 1.1h1.9" />
    </g>

    <g v-else-if="name === 'star'">
      <path d="M8 2.2l1.8 3.7 4 .6-2.9 2.8.7 4-3.6-1.9-3.6 1.9.7-4L2.2 6.5l4-.6z" />
    </g>

    <g v-else-if="name === 'eye'">
      <path d="M1.8 8S4 3.8 8 3.8 14.2 8 14.2 8 12 12.2 8 12.2 1.8 8 1.8 8z" />
      <circle cx="8" cy="8" r="1.9" />
    </g>

    <g v-else-if="name === 'info'">
      <circle cx="8" cy="8" r="5.7" />
      <path d="M8 7.2v3.3M8 5.1v.2" />
    </g>

    <g v-else-if="name === 'clock'">
      <circle cx="8" cy="8" r="5.6" />
      <path d="M8 4.8v3.4l2.3 1.4" />
    </g>

    <g v-else>
      <circle cx="8" cy="8" r="5.4" />
      <path d="M8 5.4v3.2M8 10.8v.2" />
    </g>
  </svg>
</template>
