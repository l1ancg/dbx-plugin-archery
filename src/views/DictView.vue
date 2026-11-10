<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import {
  filterDictTables,
  groupDictTables,
  splitMatch,
  toDictTable,
  type DictTable,
} from "../lib/dict";
import type { InstanceItem, ResourceItem } from "../types";
import NavIcon from "../components/NavIcon.vue";
import TableMetaDialog from "../components/TableMetaDialog.vue";

const props = defineProps<{
  instances: InstanceItem[];
}>();

const emit = defineEmits<{
  toast: [text: string, error?: boolean];
}>();

const instanceName = ref("");
const dbName = ref("");
const keyword = ref("");
const activeLetter = ref("");
const dbs = ref<ResourceItem[]>([]);
const tables = ref<DictTable[]>([]);
const dbLoading = ref(false);
const tableLoading = ref(false);
const dbError = ref("");
const tableError = ref("");
const commentAvailable = ref(true);
const listEl = ref<HTMLElement | null>(null);
const searchEl = ref<HTMLInputElement | null>(null);
/** 非空时打开表元数据弹窗。 */
const metaTable = ref<DictTable | null>(null);

const needle = computed(() => keyword.value.trim());
const groups = computed(() => groupDictTables(filterDictTables(tables.value, keyword.value)));
const letterIndex = computed(() =>
  groups.value.map((group) => ({ letter: group.letter, count: group.tables.length })),
);
const visibleCount = computed(() =>
  groups.value.reduce((sum, group) => sum + group.tables.length, 0),
);
const loading = computed(() => dbLoading.value || tableLoading.value);
const failure = computed(() => dbError.value || tableError.value);
/** 未选库时不加载任何表，页面停在「请选择」提示。 */
const picked = computed(() => Boolean(instanceName.value && dbName.value));
const currentInstance = computed(() =>
  props.instances.find((item) => item.name === instanceName.value),
);

let scrollFrame = 0;

function resetTables(): void {
  tables.value = [];
  tableError.value = "";
  commentAvailable.value = true;
  activeLetter.value = "";
}

async function loadDbs(): Promise<void> {
  const name = instanceName.value;
  dbLoading.value = true;
  dbError.value = "";
  dbs.value = [];
  dbName.value = "";
  keyword.value = "";
  resetTables();
  if (!name) {
    dbLoading.value = false;
    return;
  }
  try {
    const instance = props.instances.find((item) => item.name === name);
    const list = await archeryApi.resources({
      instanceName: name,
      instanceId: instance?.id,
      resourceType: "database",
    });
    if (name !== instanceName.value) return;
    // 只填库候选，不预选：用户选库后才请求表清单。
    dbs.value = list;
  } catch (error) {
    if (name === instanceName.value) dbError.value = describeError(error);
  } finally {
    if (name === instanceName.value) dbLoading.value = false;
  }
}

async function loadTables(): Promise<void> {
  const instance = instanceName.value;
  const db = dbName.value;
  resetTables();
  if (!instance || !db) return;
  tableLoading.value = true;
  try {
    const item = props.instances.find((row) => row.name === instance);
    const result = await archeryApi.dictTables({
      instanceName: instance,
      instanceId: item?.id,
      dbType: item?.dbType,
      dbName: db,
    });
    if (instance !== instanceName.value || db !== dbName.value) return;
    tables.value = (result.tables ?? []).map((row) => toDictTable(row, db));
    commentAvailable.value = result.commentAvailable !== false;
    await nextTick();
    syncActiveLetter();
  } catch (error) {
    if (instance === instanceName.value && db === dbName.value) {
      tableError.value = describeError(error);
    }
  } finally {
    if (instance === instanceName.value && db === dbName.value) tableLoading.value = false;
  }
}

function refresh(): void {
  if (!instanceName.value) return;
  if (!dbName.value) {
    void loadDbs();
    return;
  }
  void loadTables();
}

function groupNode(letter: string): HTMLElement | null {
  const list = listEl.value;
  if (!list) return null;
  return list.querySelector<HTMLElement>(`[data-letter="${letter}"]`);
}

/** 滚动时把当前字母定位到最后一个已越过分组顶部的分组。 */
function syncActiveLetter(): void {
  const list = listEl.value;
  if (!list) return;
  const offset = list.scrollTop + 8;
  let current = groups.value[0]?.letter ?? "";
  for (const group of groups.value) {
    const node = groupNode(group.letter);
    if (node && node.offsetTop <= offset) current = group.letter;
  }
  activeLetter.value = current;
}

function onScroll(): void {
  if (scrollFrame) return;
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0;
    syncActiveLetter();
  });
}

function jumpTo(letter: string): void {
  const list = listEl.value;
  const node = groupNode(letter);
  if (!list || !node) return;
  list.scrollTop = node.offsetTop;
  activeLetter.value = letter;
}

function openMeta(table: DictTable): void {
  metaTable.value = table;
}

function clearKeyword(): void {
  keyword.value = "";
  searchEl.value?.focus();
}

/** 行 title 只在有注释时拼分隔符，避免尾随空白。 */
function rowTitle(table: DictTable): string {
  return table.comment ? `${table.name} · ${table.comment}` : table.name;
}

/** 「/」聚焦搜索：焦点已在输入控件或弹窗内时让出去，不抢宿主快捷键。 */
function onSlashKey(event: KeyboardEvent): void {
  if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey) return;
  if (!picked.value || document.querySelector(".modal-mask")) return;
  const target = event.target as HTMLElement | null;
  if (target?.closest("input, textarea, select, [contenteditable='true'], .cm-editor")) return;
  event.preventDefault();
  searchEl.value?.focus();
}

onMounted(() => window.addEventListener("keydown", onSlashKey));
onBeforeUnmount(() => window.removeEventListener("keydown", onSlashKey));

/** 骨架屏行数：够铺满常见高度的首屏即可，不做真实分页估算。 */
const skeletonRows = [0, 1, 2, 3, 4, 5, 6, 7];

/** 过滤后当前高亮字母可能已不存在，回退到第一个分组。 */
watch(groups, (list) => {
  if (!list.some((group) => group.letter === activeLetter.value)) {
    activeLetter.value = list[0]?.letter ?? "";
  }
});
</script>

<template>
  <div class="page page-fill dict-page">
    <section class="panel card-fill">
      <div class="panel-heading dict-toolbar">
        <label class="dict-field">
          <span class="dict-label">{{ t("dict.instance") }}</span>
          <select
            v-model="instanceName"
            class="form-control form-control-sm dict-select"
            @change="loadDbs"
          >
            <option value="">{{ t("dict.selectInstance") }}</option>
            <option v-for="item in instances" :key="item.name" :value="item.name">
              {{ item.name }}
            </option>
          </select>
        </label>

        <label class="dict-field">
          <span class="dict-label">{{ t("dict.database") }}</span>
          <select
            v-model="dbName"
            class="form-control form-control-sm dict-select"
            :disabled="!instanceName || dbLoading || dbs.length === 0"
            @change="loadTables"
          >
            <option value="">{{ t("dict.selectDatabase") }}</option>
            <option v-for="db in dbs" :key="db.name" :value="db.name">{{ db.name }}</option>
          </select>
        </label>

        <span class="spacer"></span>

        <span
          v-if="picked"
          class="dict-count"
          :class="{ 'is-filtering': needle }"
          role="status"
          aria-live="polite"
        >
          {{
            needle
              ? t("dict.filteredCount", { n: visibleCount, total: tables.length })
              : t("dict.count", { n: tables.length })
          }}
        </span>

        <div class="dict-search-wrap">
          <NavIcon name="search" :size="13" class="dict-search-icon" />
          <input
            ref="searchEl"
            v-model="keyword"
            class="form-control form-control-sm dict-search"
            type="search"
            :placeholder="t('dict.searchPlaceholder')"
            :aria-label="t('dict.searchPlaceholder')"
            :disabled="!picked"
            :title="t('dict.searchHint')"
            @keydown.esc="keyword = ''"
          />
          <button
            v-if="keyword"
            type="button"
            class="dict-search-clear"
            :title="t('dict.clearSearch')"
            :aria-label="t('dict.clearSearch')"
            @click="clearKeyword"
          >
            <NavIcon name="close" :size="11" />
          </button>
          <kbd v-else class="dict-search-kbd">/</kbd>
        </div>

        <button class="tool-btn" :title="t('app.refresh')" @click="refresh">
          <NavIcon name="refresh" :size="14" />
        </button>
      </div>

      <nav v-if="letterIndex.length > 0" class="dict-index" :aria-label="t('dict.indexLabel')">
        <button
          v-for="item in letterIndex"
          :key="item.letter"
          class="dict-letter"
          :class="{ 'is-on': item.letter === activeLetter }"
          :data-index-letter="item.letter"
          :aria-current="item.letter === activeLetter ? 'true' : undefined"
          @click="jumpTo(item.letter)"
        >
          <span class="dict-letter-char">{{ item.letter }}</span>
          <span class="dict-letter-count">{{ item.count }}</span>
        </button>
      </nav>

      <div v-if="picked && !commentAvailable && !loading" class="dict-warn">
        <span class="dict-warn-icon"><NavIcon name="info" :size="12" /></span>
        <span>{{ t("dict.commentUnavailable") }}</span>
      </div>

      <div ref="listEl" class="dict-list" @scroll="onScroll">
        <div v-if="failure" class="error-box">{{ t("dict.loadFailed") }}：{{ failure }}</div>

        <div v-else-if="loading" class="dict-skeleton" role="status" :aria-label="t('dict.loading')">
          <div v-for="row in skeletonRows" :key="row" class="dict-skeleton-row">
            <span class="dict-skeleton-bar is-name"></span>
            <span class="dict-skeleton-bar is-comment"></span>
          </div>
        </div>

        <div v-else-if="!instanceName" class="empty" role="status">
          <span class="dict-empty-icon"><NavIcon name="database" :size="22" /></span>
          <strong>{{ t("dict.pickHint") }}</strong>
          <span class="dict-empty-hint">{{ t("dict.pickHintDetail") }}</span>
        </div>

        <div v-else-if="!dbs.length" class="empty" role="status">
          <span class="dict-empty-icon"><NavIcon name="database" :size="22" /></span>
          <strong>{{ t("dict.noDatabase") }}</strong>
          <span class="dict-empty-hint">{{ t("dict.noDatabaseDetail") }}</span>
        </div>

        <div v-else-if="!dbName" class="empty" role="status">
          <span class="dict-empty-icon"><NavIcon name="database" :size="22" /></span>
          <strong>{{ t("dict.pickDbTitle") }}</strong>
          <span class="dict-empty-hint">{{ t("dict.pickDbDetail") }}</span>
        </div>

        <div v-else-if="groups.length === 0" class="empty" role="status">
          <span class="dict-empty-icon">
            <NavIcon :name="tables.length > 0 ? 'search' : 'table'" :size="22" />
          </span>
          <strong>{{ tables.length > 0 ? t("dict.noMatch") : t("dict.empty") }}</strong>
          <span class="dict-empty-hint">
            {{ tables.length > 0 ? t("dict.noMatchHint") : t("dict.emptyHint") }}
          </span>
          <button
            v-if="tables.length > 0"
            class="btn btn-default btn-sm dict-empty-action"
            @click="clearKeyword"
          >
            {{ t("dict.clearSearch") }}
          </button>
        </div>

        <template v-else>
          <div
            v-for="group in groups"
            :key="group.letter"
            class="dict-group"
            :data-letter="group.letter"
          >
            <div class="dict-group-head">
              <span class="dict-group-plate">{{ group.letter }}</span>
              <span class="dict-group-count">{{ group.tables.length }}</span>
              <span class="dict-group-rule"></span>
            </div>
            <div
              v-for="table in group.tables"
              :key="table.name"
              class="dict-row"
              :title="rowTitle(table)"
            >
              <!-- 只有表名可点击复制，注释与空白区域不响应 -->
              <button class="dict-name" :title="t('dict.openMetaHint')" @click="openMeta(table)">
                <span class="dict-name-text">
                  <template v-for="(part, index) in splitMatch(table.name, needle)" :key="index">
                    <mark v-if="part.hit" class="dict-hit">{{ part.text }}</mark>
                    <template v-else>{{ part.text }}</template>
                  </template>
                </span>
              </button>
              <span class="dict-comment" :class="{ 'is-empty': !table.comment }">
                <template v-if="table.comment">
                  <template
                    v-for="(part, index) in splitMatch(table.comment, needle)"
                    :key="index"
                  >
                    <mark v-if="part.hit" class="dict-hit">{{ part.text }}</mark>
                    <template v-else>{{ part.text }}</template>
                  </template>
                </template>
                <template v-else>—</template>
              </span>
              <span class="dict-open" aria-hidden="true">
                <NavIcon name="chevron" :size="11" />
              </span>
            </div>
          </div>
        </template>
      </div>
    </section>

    <TableMetaDialog
      v-if="metaTable"
      :instance-name="instanceName"
      :instance-id="currentInstance?.id"
      :db-name="dbName"
      :db-type="currentInstance?.dbType"
      :table-name="metaTable.name"
      @close="metaTable = null"
      @toast="(text, isError) => emit('toast', text, isError)"
    />
  </div>
</template>
