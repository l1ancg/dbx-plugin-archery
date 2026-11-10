<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { t } from "../i18n";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  connectionName: string;
  /** Archery 侧动态品牌；取不到时退回连接名。 */
  brand?: string;
  brandSuffix?: string;
  username: string;
  version: string;
  readOnly: boolean;
  status: "online" | "pending" | "offline";
  statusLabel: string;
  /** 待办总数；null / undefined 表示取不到（无审核权限、只读连接、旧版 sidecar）。 */
  todoCount?: number | null;
}>();

const emit = defineEmits<{
  home: [];
  notifications: [];
  settings: [];
  "two-factor": [];
  "change-password": [];
  logout: [];
}>();

/** 账号下拉菜单：与 Archery 网页一致，收拢两步验证 / 修改密码 / 退出。 */
const menuOpen = ref(false);
const menuEl = ref<HTMLElement | null>(null);

function onDocumentClick(event: MouseEvent): void {
  if (!menuOpen.value) return;
  if (menuEl.value && !menuEl.value.contains(event.target as Node)) {
    menuOpen.value = false;
  }
}

onMounted(() => document.addEventListener("click", onDocumentClick));
onBeforeUnmount(() => document.removeEventListener("click", onDocumentClick));

function pick(action: "two-factor" | "change-password" | "logout"): void {
  menuOpen.value = false;
  if (action === "two-factor") emit("two-factor");
  else if (action === "change-password") emit("change-password");
  else emit("logout");
}

/** 品牌优先取 Archery 导航栏（Archery + custom_title_suffix），取不到再用连接名兜底。 */
const brandName = computed(() => props.brand?.trim() || "Archery");

const suffix = computed(() => {
  // Archery 的 custom_title_suffix 自带括号（如 (广州跨境&其他测试环境)），原样输出。
  const fromPlatform = props.brandSuffix?.trim();
  if (fromPlatform) return fromPlatform;
  // 连接名兜底：去掉前缀与已有括号，统一补一对半角括号。
  const name = props.connectionName.trim();
  if (!name) return "";
  const text = name
    .replace(/^archery\s*[（(]?/i, "")
    .replace(/[)）]\s*$/, "")
    .trim();
  return text ? `(${text})` : "";
});

const greeting = computed(() => t("header.greeting", { user: props.username || "-" }));

/** 铃铛右侧的待办总数，与 Archery base.html 的 `{{ todo }}` 一致：0 也照实显示。 */
const bellCount = computed(() =>
  props.todoCount === null || props.todoCount === undefined ? null : props.todoCount,
);
</script>

<template>
  <header class="app-header">
    <button class="app-brand" :title="version ? `${brandName} v${version}` : brandName" @click="emit('home')">
      <span class="brand-name">{{ brandName }}</span>
      <span v-if="suffix" class="brand-suffix">{{ suffix }}</span>
    </button>

    <div class="app-header-right">
      <span class="status-dot" :class="status" :title="statusLabel"></span>
      <span class="app-greeting">{{ greeting }}</span>
      <button
        class="header-icon header-bell"
        :class="{ 'has-count': bellCount !== null }"
        :title="t('header.notifications')"
        @click="emit('notifications')"
      >
        <NavIcon name="bell" :size="16" />
        <span v-if="bellCount !== null" class="header-bell-count">{{ bellCount }}</span>
      </button>
      <button class="header-icon" :title="t('header.help')" @click="emit('settings')">
        <NavIcon name="question" :size="16" />
      </button>
      <div ref="menuEl" class="header-menu">
        <button
          class="header-icon avatar"
          :class="{ open: menuOpen }"
          :title="username ? `${t('header.account')}: ${username}` : t('header.avatar')"
          @click.stop="menuOpen = !menuOpen"
        >
          <NavIcon name="user" :size="15" />
        </button>
        <div v-if="menuOpen" class="header-dropdown">
          <button class="header-menu-item" @click="pick('two-factor')">
            <NavIcon name="lock" :size="14" />
            <span>{{ t("header.twoFactor") }}</span>
          </button>
          <button class="header-menu-item" @click="pick('change-password')">
            <NavIcon name="user" :size="14" />
            <span>{{ t("header.changePassword") }}</span>
          </button>
          <button class="header-menu-item" @click="pick('logout')">
            <NavIcon name="power" :size="14" />
            <span>{{ t("header.signOut") }}</span>
          </button>
        </div>
      </div>
    </div>
  </header>
</template>
