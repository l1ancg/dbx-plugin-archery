<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { describeError } from "../lib/errors";
import { privTypeOptions, validDatePreset, validatePrivilegeApply } from "../lib/privilege";
import type { GroupInstance, PrivilegeApplyForm, ValidDatePreset } from "../types";
import MultiSelect from "./MultiSelect.vue";
import NavIcon from "./NavIcon.vue";

const emit = defineEmits<{
  close: [];
  submitted: [];
}>();

const groups = ref<string[]>([]);
const instances = ref<GroupInstance[]>([]);
const databases = ref<string[]>([]);
const tables = ref<string[]>([]);

const title = ref("");
const groupName = ref("");
const instanceName = ref("");
const privType = ref(0);
const dbName = ref("");
const dbList = ref<string[]>([]);
const tableList = ref<string[]>([]);
const validPreset = ref<ValidDatePreset | "">("");
const limitNum = ref<number | null>(null);
const auditorsDisplay = ref("");

const loadingGroups = ref(false);
const loadingInstances = ref(false);
const loadingDatabases = ref(false);
const loadingTables = ref(false);
const submitting = ref(false);
const error = ref("");

const currentInstance = computed(() =>
  instances.value.find((item) => item.name === instanceName.value),
);
const levelOptions = computed(() => privTypeOptions(currentInstance.value?.dbType ?? ""));
const isTableLevel = computed(() => privType.value === 2);

/** 授权时间固定四档，直接用分段控件摊开，省一次展开下拉。 */
const validOptions = computed<ReadonlyArray<{ value: ValidDatePreset; label: string }>>(() => [
  { value: "day", label: t("privilege.validDay") },
  { value: "week", label: t("privilege.validWeek") },
  { value: "month", label: t("privilege.validMonth") },
  { value: "year", label: t("privilege.validYear") },
]);

/** 常用行数上限，点一下就填，免去手输。 */
const limitPresets = [100, 500, 1000];

/** 权限级别沿用 Archery 的英文枚举，前面补中文说明，避免只有 DATABASE/TABLE 的天书。 */
function levelLabel(type: number): string {
  return type === 2
    ? `${t("privilege.level.table")} · TABLE`
    : `${t("privilege.level.database")} · DATABASE`;
}

async function loadGroups(): Promise<void> {
  loadingGroups.value = true;
  error.value = "";
  try {
    groups.value = await archeryApi.privilegeOptions();
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingGroups.value = false;
  }
}

async function loadInstances(): Promise<void> {
  instances.value = [];
  instanceName.value = "";
  if (!groupName.value) return;
  loadingInstances.value = true;
  try {
    instances.value = await archeryApi.privilegeInstances({ groupName: groupName.value });
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingInstances.value = false;
  }
}

async function loadDatabases(): Promise<void> {
  databases.value = [];
  dbName.value = "";
  dbList.value = [];
  tables.value = [];
  tableList.value = [];
  if (!instanceName.value) return;
  loadingDatabases.value = true;
  try {
    const list = await archeryApi.resources({
      instanceName: instanceName.value,
      resourceType: "database",
    });
    databases.value = list.map((item) => item.name).filter(Boolean);
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingDatabases.value = false;
  }
}

async function loadTables(): Promise<void> {
  tables.value = [];
  tableList.value = [];
  if (!isTableLevel.value || !dbName.value || !instanceName.value) return;
  loadingTables.value = true;
  try {
    const list = await archeryApi.resources({
      instanceName: instanceName.value,
      dbName: dbName.value,
      resourceType: "table",
    });
    tables.value = list.map((item) => item.name).filter(Boolean);
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loadingTables.value = false;
  }
}

/**
 * 审批流与实例列表一起随资源组变化。
 * 请求带自增序号：连续切组时，先发出的响应不会覆盖后选中的组。
 */
let auditorsRequest = 0;
async function loadAuditors(name: string): Promise<void> {
  const request = ++auditorsRequest;
  auditorsDisplay.value = "";
  if (!name) return;
  try {
    const result = await archeryApi.privilegeAuditors({ groupName: name });
    if (request === auditorsRequest) auditorsDisplay.value = result.auditorsDisplay;
  } catch {
    // 资源组未配置审批流时 Archery 返回业务错误，这里只隐藏提示条，不阻断填表。
  }
}

watch(groupName, () => {
  void loadAuditors(groupName.value);
  void loadInstances();
});

watch(instanceName, () => {
  void loadDatabases();
  // Archery 对 MySQL 默认选 TABLE，其余引擎只有 DATABASE。
  const options = privTypeOptions(currentInstance.value?.dbType ?? "");
  privType.value = options.includes(2) ? 2 : 1;
});

watch(dbName, () => void loadTables());

watch(privType, (value) => {
  if (value === 1) {
    tables.value = [];
    tableList.value = [];
    return;
  }
  void loadTables();
});

async function submit(): Promise<void> {
  error.value = "";
  const form: PrivilegeApplyForm = {
    title: title.value,
    groupName: groupName.value,
    instanceName: instanceName.value,
    privType: privType.value,
    dbName: dbName.value,
    dbList: dbList.value,
    tableList: tableList.value,
    validDatePreset: validPreset.value,
    limitNum: Number(limitNum.value) || 0,
  };
  const missing = validatePrivilegeApply(form);
  if (missing.length > 0) {
    error.value = t("privilege.missingFields", {
      fields: missing.map((key) => t(`privilege.field.${key}`)).join("、"),
    });
    return;
  }

  submitting.value = true;
  try {
    await archeryApi.privilegeApply({
      title: form.title.trim(),
      groupName: form.groupName,
      instanceName: form.instanceName,
      privType: form.privType,
      dbName: form.privType === 2 ? form.dbName : "",
      dbList: form.privType === 1 ? form.dbList : [],
      tableList: form.privType === 2 ? form.tableList : [],
      validDate: validDatePreset((form.validDatePreset || "day") as ValidDatePreset),
      limitNum: form.limitNum,
    });
    emit("submitted");
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    submitting.value = false;
  }
}

onMounted(loadGroups);
</script>

<template>
  <div class="modal-mask pa-mask" @click.self="emit('close')">
    <div class="modal privilege-modal privilege-apply-modal pa-dialog">
      <header class="modal-head pa-head">
        <span class="pa-head-icon" aria-hidden="true">
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M12 3l7 2.9v6.2c0 4.2-2.9 7.4-7 8.9-4.1-1.5-7-4.7-7-8.9V5.9z" />
            <path d="M8.9 12.1l2.2 2.2 4.1-4.6" />
          </svg>
        </span>
        <div class="pa-head-text">
          <h2 class="pa-title">{{ t("privilege.applyTitle") }}</h2>
          <p class="pa-subtitle">{{ t("privilege.applySubtitle") }}</p>
        </div>
        <button
          type="button"
          class="pa-close"
          :title="t('app.close')"
          :aria-label="t('app.close')"
          @click="emit('close')"
        >
          <NavIcon name="close" :size="14" />
        </button>
      </header>

      <div class="modal-body pa-body">
        <section class="pa-section">
          <div class="pa-section-head">
            <span class="pa-step">01</span>
            <h3 class="pa-section-title">{{ t("privilege.sectionPurpose") }}</h3>
            <span class="pa-line"></span>
          </div>
          <div class="pa-field">
            <label class="pa-label" for="pa-title">
              {{ t("privilege.labelTitle") }}
              <i class="pa-req" aria-hidden="true">*</i>
            </label>
            <input
              id="pa-title"
              v-model="title"
              class="pa-control"
              type="text"
              maxlength="100"
              autocomplete="off"
              :placeholder="t('privilege.applyPurpose')"
            />
            <p class="pa-note">{{ t("privilege.noteTitle") }}</p>
          </div>
        </section>

        <section class="pa-section">
          <div class="pa-section-head">
            <span class="pa-step">02</span>
            <h3 class="pa-section-title">{{ t("privilege.sectionScope") }}</h3>
            <span class="pa-line"></span>
          </div>
          <div class="pa-grid">
            <div class="pa-field">
              <label class="pa-label" for="pa-group">
                {{ t("privilege.labelGroup") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </label>
              <div class="pa-select" :class="{ 'is-loading': loadingGroups }">
                <select id="pa-group" v-model="groupName" :disabled="loadingGroups">
                  <option value="">{{ t("privilege.pickGroup") }}</option>
                  <option v-for="group in groups" :key="group" :value="group">{{ group }}</option>
                </select>
              </div>
              <p v-if="!loadingGroups && groups.length === 0" class="pa-note is-warn">
                {{ t("privilege.groupEmpty") }}
              </p>
            </div>

            <div class="pa-field">
              <label class="pa-label" for="pa-instance">
                {{ t("privilege.labelInstance") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </label>
              <div class="pa-select" :class="{ 'is-loading': loadingInstances }">
                <select
                  id="pa-instance"
                  v-model="instanceName"
                  :disabled="!groupName || loadingInstances"
                >
                  <option value="">{{ t("privilege.pickInstance") }}</option>
                  <option v-for="item in instances" :key="item.id" :value="item.name">
                    {{ item.name }}
                  </option>
                </select>
              </div>
              <p
                v-if="groupName && !loadingInstances && instances.length === 0"
                class="pa-note is-warn"
              >
                {{ t("privilege.instanceEmpty") }}
              </p>
            </div>

            <div class="pa-field">
              <label class="pa-label" for="pa-level">
                {{ t("privilege.labelLevel") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </label>
              <div class="pa-select">
                <select id="pa-level" v-model.number="privType" :disabled="!instanceName">
                  <option :value="0" disabled>{{ t("privilege.pickPrivType") }}</option>
                  <option v-for="option in levelOptions" :key="option" :value="option">
                    {{ levelLabel(option) }}
                  </option>
                </select>
              </div>
            </div>

            <div class="pa-field">
              <span class="pa-label">
                {{ isTableLevel ? t("privilege.labelDatabase") : t("privilege.labelDatabases") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </span>
              <div
                v-if="isTableLevel"
                class="pa-select"
                :class="{ 'is-loading': loadingDatabases }"
              >
                <select id="pa-database" v-model="dbName" :disabled="!instanceName">
                  <option value="">{{ t("privilege.pickDatabase") }}</option>
                  <option v-for="db in databases" :key="db" :value="db">{{ db }}</option>
                </select>
              </div>
              <MultiSelect
                v-else
                v-model="dbList"
                :options="databases"
                :loading="loadingDatabases"
                :disabled="!instanceName"
                :placeholder="t('privilege.pickDatabases')"
              />
            </div>

            <div v-if="isTableLevel" class="pa-field is-wide">
              <span class="pa-label">
                {{ t("privilege.labelTables") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </span>
              <MultiSelect
                v-model="tableList"
                :options="tables"
                :loading="loadingTables"
                :disabled="!dbName"
                :placeholder="t('privilege.pickTable')"
              />
            </div>
          </div>
        </section>

        <section class="pa-section">
          <div class="pa-section-head">
            <span class="pa-step">03</span>
            <h3 class="pa-section-title">{{ t("privilege.sectionDuration") }}</h3>
            <span class="pa-line"></span>
          </div>
          <div class="pa-grid pa-grid-duration">
            <div class="pa-field">
              <span class="pa-label">
                {{ t("privilege.labelValidDate") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </span>
              <div
                class="pa-segmented"
                role="radiogroup"
                :aria-label="t('privilege.labelValidDate')"
              >
                <button
                  v-for="option in validOptions"
                  :key="option.value"
                  type="button"
                  role="radio"
                  class="pa-segment"
                  :class="{ 'is-on': validPreset === option.value }"
                  :aria-checked="validPreset === option.value"
                  @click="validPreset = option.value"
                >
                  {{ option.label }}
                </button>
              </div>
            </div>

            <div class="pa-field">
              <label class="pa-label" for="pa-limit">
                {{ t("privilege.labelLimit") }}
                <i class="pa-req" aria-hidden="true">*</i>
              </label>
              <div class="pa-affix">
                <input
                  id="pa-limit"
                  v-model.number="limitNum"
                  class="pa-control"
                  type="number"
                  min="1"
                  max="10000"
                  :placeholder="t('privilege.limitPlaceholder')"
                />
                <span class="pa-affix-text">{{ t("privilege.unitRow") }}</span>
              </div>
              <div class="pa-chips">
                <button
                  v-for="preset in limitPresets"
                  :key="preset"
                  type="button"
                  class="pa-chip"
                  :class="{ 'is-on': limitNum === preset }"
                  @click="limitNum = preset"
                >
                  {{ preset }}
                </button>
              </div>
            </div>
          </div>
        </section>

        <p v-if="auditorsDisplay" class="privilege-auditors">
          <NavIcon name="check" :size="12" class="privilege-auditors-icon" />
          <span class="privilege-auditors-label">{{ t("privilege.auditors") }}</span>
          <b>{{ auditorsDisplay }}</b>
        </p>

        <p class="pa-hint">
          <NavIcon name="info" :size="13" class="pa-hint-icon" />
          <span>{{ t("privilege.applyHint") }}</span>
        </p>

        <p v-if="error" class="pa-error" role="alert">
          <NavIcon name="info" :size="13" class="pa-hint-icon" />
          <span>{{ error }}</span>
        </p>
      </div>

      <div class="modal-foot pa-foot">
        <p class="pa-foot-note">
          <NavIcon name="clock" :size="12" />
          {{ t("privilege.footHint") }}
        </p>
        <button
          type="button"
          class="btn btn-default pa-btn"
          :disabled="submitting"
          @click="emit('close')"
        >
          {{ t("app.cancel") }}
        </button>
        <button
          type="button"
          class="btn btn-success pa-btn pa-btn-primary"
          :disabled="submitting"
          @click="submit"
        >
          <span v-if="submitting" class="pa-spinner"></span>
          {{ t("privilege.submit") }}
        </button>
      </div>
    </div>
  </div>
</template>
