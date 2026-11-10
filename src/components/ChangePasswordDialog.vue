<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { passwordChecks, passwordIssueKey, validatePasswordForm } from "../lib/account";
import { describeError } from "../lib/errors";
import NavIcon from "./NavIcon.vue";

const props = defineProps<{
  open: boolean;
  username: string;
}>();

const emit = defineEmits<{
  close: [];
  toast: [text: string, error?: boolean];
}>();

const oldInput = ref<HTMLInputElement | null>(null);
const oldPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");
const submitting = ref(false);
const invalid = ref("");
const error = ref("");

const checks = computed(() =>
  passwordChecks({
    username: props.username,
    oldPassword: oldPassword.value,
    newPassword: newPassword.value,
    confirmPassword: confirmPassword.value,
  }),
);
const passedCount = computed(() => Object.values(checks.value).filter(Boolean).length);
const rules = computed(() => [
  { key: "password.ruleLength", ok: checks.value.length },
  { key: "password.ruleNumeric", ok: checks.value.notNumeric },
  { key: "password.ruleSimilar", ok: checks.value.notSimilar },
  { key: "password.ruleMatch", ok: checks.value.matches },
]);
const strengthClass = computed(() => {
  if (passedCount.value >= 4) return "strength-strong";
  if (passedCount.value >= 2) return "strength-mid";
  return "strength-weak";
});

watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    reset();
    await nextTick();
    oldInput.value?.focus();
  },
);

function onKeydown(event: KeyboardEvent): void {
  if (!props.open || event.key !== "Escape") return;
  event.preventDefault();
  close();
}

onMounted(() => document.addEventListener("keydown", onKeydown));
onBeforeUnmount(() => document.removeEventListener("keydown", onKeydown));

function reset(): void {
  oldPassword.value = "";
  newPassword.value = "";
  confirmPassword.value = "";
  invalid.value = "";
  error.value = "";
}

function close(): void {
  reset();
  emit("close");
}

async function submit(): Promise<void> {
  if (submitting.value) return;
  const issue = validatePasswordForm({
    username: props.username,
    oldPassword: oldPassword.value,
    newPassword: newPassword.value,
    confirmPassword: confirmPassword.value,
  });
  if (issue) {
    invalid.value = t(passwordIssueKey(issue));
    return;
  }
  invalid.value = "";
  error.value = "";
  submitting.value = true;
  try {
    await archeryApi.changePassword({
      oldPassword: oldPassword.value.trim(),
      newPassword: newPassword.value.trim(),
    });
    emit("toast", t("password.changedHint"));
    reset();
    emit("close");
  } catch (err) {
    error.value = describeError(err);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div v-if="open" class="modal-mask account-mask" @click.self="close">
    <div
      class="modal account-modal password-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="password-title"
    >
      <header class="account-head">
        <span class="account-badge"><NavIcon name="key" :size="19" /></span>
        <div class="account-head-text">
          <h2 id="password-title">{{ t("password.title") }}</h2>
          <p>{{ t("password.subtitle") }}</p>
        </div>
        <button class="modal-close" :title="t('app.close')" @click="close">×</button>
      </header>

      <div class="modal-body account-body">
        <p class="account-intro">{{ t("password.desc") }}</p>

        <label class="account-field">
          <span>{{ t("password.old") }}</span>
          <input
            ref="oldInput"
            v-model="oldPassword"
            class="account-input"
            type="password"
            autocomplete="current-password"
            @keyup.enter="submit"
          />
        </label>

        <div class="account-divider"><span>{{ t("password.section") }}</span></div>

        <div class="password-grid">
          <label class="account-field">
            <span>{{ t("password.new") }}</span>
            <input
              v-model="newPassword"
              class="account-input"
              type="password"
              autocomplete="new-password"
              @keyup.enter="submit"
            />
          </label>
          <label class="account-field">
            <span>{{ t("password.confirm") }}</span>
            <input
              v-model="confirmPassword"
              class="account-input"
              type="password"
              autocomplete="new-password"
              @keyup.enter="submit"
            />
          </label>
        </div>

        <section class="password-checks" :class="strengthClass">
          <div class="password-checks-head">
            <span>{{ t("password.rulesTitle") }}</span>
            <span class="password-count" aria-live="polite">{{ passedCount }}/{{ rules.length }}</span>
          </div>
          <div class="password-strength" aria-hidden="true">
            <i v-for="step in rules.length" :key="step" :class="{ on: step <= passedCount }"></i>
          </div>
          <ul>
            <li v-for="rule in rules" :key="rule.key" :class="{ ok: rule.ok }">
              <span class="rule-mark"><NavIcon name="check" :size="10" /></span>
              {{ t(rule.key) }}
            </li>
          </ul>
          <p class="password-common">{{ t("password.ruleCommon") }}</p>
        </section>

        <div v-if="invalid || error" class="error-box account-error">{{ invalid || error }}</div>
      </div>

      <footer class="modal-foot account-foot">
        <span class="account-hint"><kbd>Esc</kbd>{{ t("app.escClose") }}</span>
        <button class="btn ghost" :disabled="submitting" @click="close">
          {{ t("app.cancel") }}
        </button>
        <button class="btn primary" :disabled="submitting" @click="submit">
          <span v-if="submitting" class="spinner" style="border-top-color: #fff"></span>
          {{ t("password.submit") }}
        </button>
      </footer>
    </div>
  </div>
</template>
