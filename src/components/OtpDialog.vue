<script setup lang="ts">
import { ref, watch } from "vue";
import { t } from "../i18n";

const props = defineProps<{
  open: boolean;
  username: string;
  method: "totp" | "sms";
  submitting: boolean;
  error: string;
}>();

const emit = defineEmits<{
  submit: [code: string];
  close: [];
}>();

const code = ref("");
const invalid = ref("");

watch(
  () => props.open,
  (open) => {
    if (open) {
      code.value = "";
      invalid.value = "";
    }
  },
);

function submit(): void {
  if (!/^\d{6}$/.test(code.value.trim())) {
    invalid.value = t("login.invalid");
    return;
  }
  invalid.value = "";
  emit("submit", code.value.trim());
}
</script>

<template>
  <div v-if="open" class="modal-mask">
    <div class="modal" role="dialog" aria-modal="true">
      <div class="modal-head">{{ t("login.title") }}</div>
      <div class="modal-body">
        <div>{{ t("login.desc", { user: username || "-" }) }}</div>
        <div class="subtle">
          {{ t("login.method") }}：{{ method === "sms" ? t("login.sms") : t("login.totp") }}
        </div>

        <label>
          <span>{{ t("login.code") }}</span>
          <input
            v-model="code"
            type="text"
            inputmode="numeric"
            maxlength="6"
            autocomplete="one-time-code"
            :placeholder="t('login.codePlaceholder')"
            @keyup.enter="submit"
          />
        </label>

        <div v-if="invalid || error" class="error-box" style="margin: 0">{{ invalid || error }}</div>
      </div>
      <div class="modal-foot">
        <button class="btn ghost" :disabled="submitting" @click="emit('close')">
          {{ t("app.cancel") }}
        </button>
        <button class="btn primary" :disabled="submitting" @click="submit">
          <span v-if="submitting" class="spinner" style="border-top-color: #fff"></span>
          {{ t("login.submit") }}
        </button>
      </div>
    </div>
  </div>
</template>
