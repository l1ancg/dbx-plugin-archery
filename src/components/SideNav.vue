<script setup lang="ts">
import { ref, watch } from "vue";
import { t } from "../i18n";
import { navItems, type NavItem, type NavKey } from "../lib/nav";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  /** 空串表示当前页不属于任何菜单项（如顶栏铃铛进入的待办列表）。 */
  active: NavKey | "";
  activeChild: string;
  auditOpen: boolean;
  /** 兼容旧状态：是否展示控制台版本（当前 UI 不在侧栏展示）。 */
  version?: string;
}>();

const emit = defineEmits<{
  select: [key: NavKey];
  selectChild: [parent: NavKey, child: string];
  toggle: [key: NavKey, open: boolean];
}>();

/** SQL查询 默认展开，与 Archery 默认落地页一致；其余父项默认收起。 */
const open = ref<Record<string, boolean>>({
  audit: props.auditOpen,
  query: true,
  optimize: false,
  tools: false,
});

watch(
  () => props.auditOpen,
  (value) => {
    open.value = { ...open.value, audit: value };
  },
);

/** 当前页落在该菜单的二级项上：父项只做「路径」提示，强调交给二级项。 */
function isOnPath(item: NavItem): boolean {
  if (props.active !== item.key || !props.activeChild) return false;
  return !!item.children?.some((child) => child.key === props.activeChild);
}

/** 当前页就是该菜单本身（Dashboard / 相关文档 / 无二级命中的父项）。 */
function isSelected(item: NavItem): boolean {
  return props.active === item.key && !isOnPath(item);
}

function toggle(item: NavItem): void {
  if (!item.children) {
    emit("select", item.key);
    return;
  }
  const next = !open.value[item.key];
  open.value = { ...open.value, [item.key]: next };
  emit("toggle", item.key, next);
}
</script>

<template>
  <aside class="side-nav">
    <ul class="side-nav-list">
      <li
        v-for="(item, index) in navItems"
        :key="item.key"
        class="nav-group"
        :style="{ '--i': String(index) }"
      >
        <button
          class="nav-item"
          :class="{
            selected: isSelected(item),
            'on-path': isOnPath(item),
          }"
          :aria-expanded="item.children ? !!open[item.key] : undefined"
          :aria-current="isSelected(item) ? 'page' : undefined"
          @click="toggle(item)"
        >
          <span class="nav-chip">
            <NavIcon :name="item.icon" :size="15" class="nav-item-icon" />
          </span>
          <span class="nav-text">{{ t(item.label) }}</span>
          <NavIcon
            v-if="item.children"
            name="chevron"
            :size="11"
            class="nav-caret"
            :class="{ open: open[item.key] }"
          />
        </button>

        <div
          v-if="item.children"
          class="nav-children"
          :class="{ open: open[item.key] }"
          :inert="open[item.key] ? undefined : true"
        >
          <ul class="nav-children-inner">
            <li v-for="child in item.children" :key="child.key">
              <button
                class="nav-child"
                :class="{ active: active === item.key && activeChild === child.key }"
                :aria-current="
                  active === item.key && activeChild === child.key ? 'page' : undefined
                "
                @click="emit('selectChild', item.key, child.key)"
              >
                {{ t(child.label) }}
              </button>
            </li>
          </ul>
        </div>
      </li>
    </ul>
  </aside>
</template>
