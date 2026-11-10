<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import {
  matchesNode,
  resourceDetail,
  treeKey,
  visibleDbs,
  visibleInstances,
  visibleTables,
} from "../lib/tree";
import type { InstanceItem, ResourceItem } from "../types";

const props = defineProps<{
  instances: InstanceItem[];
  activeInstance: string;
  activeDb: string;
}>();

const emit = defineEmits<{
  pickDb: [payload: { instance: InstanceItem; db: string }];
  pickTable: [payload: { instance: InstanceItem; db: string; table: string }];
  refresh: [];
}>();

const keyword = ref("");
const expanded = ref<Record<string, boolean>>({});
const openDb = ref<Record<string, boolean>>({});
const dbs = ref<Record<string, ResourceItem[]>>({});
const tables = ref<Record<string, ResourceItem[]>>({});
const dbLoading = ref<Record<string, boolean>>({});
const tableLoading = ref<Record<string, boolean>>({});
const dbError = ref<Record<string, string>>({});
const tableError = ref<Record<string, string>>({});

const searchTerm = computed(() => keyword.value.trim());
const searchActive = computed(() => searchTerm.value.length > 0);
/** 搜索期间正在补拉库/表：先给「搜索中」，避免误报「无匹配」。 */
const searching = ref(false);

const shownInstances = computed(() =>
  visibleInstances(props.instances, dbs.value, tables.value, searchTerm.value),
);

function shownDbs(instance: InstanceItem): ResourceItem[] {
  return visibleDbs(instance, dbs.value[instance.name] ?? [], tables.value, searchTerm.value);
}

function shownTables(instance: InstanceItem, db: ResourceItem): ResourceItem[] {
  return visibleTables(
    instance,
    db,
    tables.value[treeKey(instance.name, db.name)] ?? [],
    searchTerm.value,
  );
}

/** 搜索时自动展开「子级命中」的实例；实例自身命中则尊重用户原本的展开状态。 */
function isExpanded(instance: InstanceItem): boolean {
  if (expanded.value[instance.name]) return true;
  if (!searchActive.value || matchesNode(instance, searchTerm.value)) return false;
  return shownDbs(instance).length > 0;
}

/** 搜索时自动展开「库名命中」或「库下有表命中」的库，让命中项可见。 */
function isDbOpen(instance: InstanceItem, db: ResourceItem): boolean {
  if (openDb.value[treeKey(instance.name, db.name)]) return true;
  if (!searchActive.value) return false;
  if (matchesNode(db, searchTerm.value)) return true;
  return (tables.value[treeKey(instance.name, db.name)] ?? []).some((table) =>
    matchesNode(table, searchTerm.value),
  );
}

async function loadDbs(instance: InstanceItem): Promise<void> {
  const key = instance.name;
  if (dbs.value[key] || dbLoading.value[key]) return;
  dbLoading.value[key] = true;
  dbError.value[key] = "";
  try {
    dbs.value[key] = await archeryApi.resources({
      instanceName: instance.name,
      instanceId: instance.id,
      resourceType: "database",
    });
  } catch (error) {
    dbError.value[key] = describeError(error);
  } finally {
    dbLoading.value[key] = false;
  }
}

async function loadTables(instance: InstanceItem, db: string): Promise<void> {
  const key = treeKey(instance.name, db);
  if (tables.value[key] || tableLoading.value[key]) return;
  tableLoading.value[key] = true;
  tableError.value[key] = "";
  try {
    tables.value[key] = await archeryApi.resources({
      instanceName: instance.name,
      instanceId: instance.id,
      resourceType: "table",
      dbName: db,
    });
  } catch (error) {
    tableError.value[key] = describeError(error);
  } finally {
    tableLoading.value[key] = false;
  }
}

async function toggleInstance(instance: InstanceItem): Promise<void> {
  const key = instance.name;
  expanded.value[key] = !expanded.value[key];
  if (expanded.value[key]) {
    await loadDbs(instance);
  }
}

async function toggleDb(instance: InstanceItem, db: string): Promise<void> {
  const key = treeKey(instance.name, db);
  openDb.value[key] = !openDb.value[key];
  emit("pickDb", { instance, db });
  if (openDb.value[key]) {
    await loadTables(instance, db);
  }
}

function dbRowActive(instance: InstanceItem, db: string): boolean {
  return props.activeInstance === instance.name && props.activeDb === db;
}

function reset(): void {
  searchRun += 1;
  searching.value = false;
  expanded.value = {};
  openDb.value = {};
  dbs.value = {};
  tables.value = {};
  dbError.value = {};
  tableError.value = {};
}

let searchTimer: ReturnType<typeof setTimeout> | null = null;
/** 每次搜索自增，慢请求回来时据此丢弃过期结果。 */
let searchRun = 0;

/** 分批并发，避免一次给 Archery 打出上百个资源请求。 */
async function inBatches<T>(
  items: T[],
  size: number,
  task: (item: T) => Promise<void>,
): Promise<void> {
  for (let index = 0; index < items.length; index += size) {
    await Promise.all(items.slice(index, index + size).map((item) => task(item)));
  }
}

/**
 * 搜索前补齐「库 → 表」：子级是惰性加载的，
 * 不预取的话没展开过的库/表永远搜不到。
 */
async function prefetch(term: string): Promise<void> {
  const run = ++searchRun;
  const stale = (): boolean => run !== searchRun;
  searching.value = true;
  try {
    await inBatches(props.instances, 4, async (instance) => {
      if (!stale()) await loadDbs(instance);
    });
    if (stale()) return;

    const jobs: { instance: InstanceItem; db: ResourceItem }[] = [];
    for (const instance of props.instances) {
      for (const db of dbs.value[instance.name] ?? []) {
        jobs.push({ instance, db });
      }
    }
    // 库名先命中的先拉，命中结果能更早显示
    jobs.sort((a, b) => Number(matchesNode(b.db, term)) - Number(matchesNode(a.db, term)));
    await inBatches(jobs, 4, async (job) => {
      if (!stale()) await loadTables(job.instance, job.db.name);
    });
  } finally {
    if (!stale()) searching.value = false;
  }
}

watch(searchTerm, (term) => {
  if (searchTimer) {
    clearTimeout(searchTimer);
    searchTimer = null;
  }
  if (!term) {
    searchRun += 1;
    searching.value = false;
    return;
  }
  searchTimer = setTimeout(() => {
    searchTimer = null;
    void prefetch(term);
  }, 250);
});

onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer);
  searchRun += 1;
});

watch(
  () => props.instances,
  (list) => {
    reset();
    const first = list[0];
    if (first) {
      expanded.value[first.name] = true;
      void loadDbs(first);
    }
    if (searchTerm.value) void prefetch(searchTerm.value);
  },
  { immediate: true },
);
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-head">
      <div class="sidebar-title">
        <span>{{ t("sidebar.instances") }}</span>
        <span class="subtle">
          {{ instances.length }}
          <button class="btn ghost" style="height: 18px; padding: 0 4px" @click="emit('refresh')">
            ⟳
          </button>
        </span>
      </div>
      <input
        v-model="keyword"
        class="search"
        type="search"
        :placeholder="t('sidebar.searchPlaceholder')"
      />
    </div>

    <div class="tree">
      <div v-if="instances.length === 0" class="tree-state">{{ t("sidebar.empty") }}</div>

      <template v-else>
        <div v-if="searchActive && searching" class="tree-state">
          <span class="spinner"></span> {{ t("sidebar.searching") }}
        </div>
        <div
          v-if="searchActive && !searching && shownInstances.length === 0"
          class="tree-state"
        >
          {{ t("sidebar.noMatch") }}
        </div>

        <template v-for="instance in shownInstances" :key="instance.name">
          <div
            class="tree-row"
            :class="{ active: activeInstance === instance.name && !activeDb }"
            :title="instance.name"
            @click="toggleInstance(instance)"
          >
            <span class="tree-caret" :class="{ open: isExpanded(instance) }">▶</span>
            <span class="tree-label">{{ instance.name }}</span>
            <span class="tree-detail"></span>
            <span class="tree-kind">{{ instance.dbType }}</span>
          </div>

          <template v-if="isExpanded(instance)">
            <div v-if="dbLoading[instance.name]" class="tree-state">
              <span class="spinner"></span> {{ t("sidebar.loading") }}
            </div>
            <div v-else-if="dbError[instance.name] && !searchActive" class="tree-state error">
              {{ dbError[instance.name] }}
            </div>
            <div
              v-else-if="shownDbs(instance).length === 0 && !searchActive"
              class="tree-state"
            >
              {{ t("sidebar.noDatabase") }}
            </div>

            <template v-for="db in shownDbs(instance)" :key="`${instance.name}/${db.name}`">
              <div
                class="tree-row"
                :class="{ active: dbRowActive(instance, db.name) }"
                :style="{ paddingLeft: '22px' }"
                :title="db.name"
                @click.stop="toggleDb(instance, db.name)"
              >
                <span class="tree-caret" :class="{ open: isDbOpen(instance, db) }">▶</span>
                <span class="tree-label">{{ db.name }}</span>
                <span class="tree-detail">{{ resourceDetail(db.values) }}</span>
              </div>

              <template v-if="isDbOpen(instance, db)">
                <div
                  v-if="tableLoading[treeKey(instance.name, db.name)]"
                  class="tree-state"
                  :style="{ paddingLeft: '40px' }"
                >
                  <span class="spinner"></span> {{ t("sidebar.loading") }}
                </div>
                <div
                  v-else-if="tableError[treeKey(instance.name, db.name)] && !searchActive"
                  class="tree-state error"
                  :style="{ paddingLeft: '40px' }"
                >
                  {{ tableError[treeKey(instance.name, db.name)] }}
                </div>
                <div
                  v-else-if="shownTables(instance, db).length === 0 && !searchActive"
                  class="tree-state"
                  :style="{ paddingLeft: '40px' }"
                >
                  {{ t("sidebar.noTable") }}
                </div>

                <div
                  v-for="table in shownTables(instance, db)"
                  :key="`${instance.name}/${db.name}/${table.name}`"
                  class="tree-row"
                  :style="{ paddingLeft: '40px' }"
                  :title="`${table.name} ${resourceDetail(table.values)}`"
                  @dblclick="
                    emit('pickTable', { instance, db: db.name, table: table.name })
                  "
                  @click.stop="emit('pickDb', { instance, db: db.name })"
                >
                  <span class="tree-label">{{ table.name }}</span>
                  <span class="tree-detail">{{ resourceDetail(table.values) }}</span>
                </div>
              </template>
            </template>
          </template>
        </template>
      </template>
    </div>
  </aside>
</template>
