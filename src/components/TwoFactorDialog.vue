<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { archeryApi } from "../api";
import { t } from "../i18n";
import { twoFactorButtonDisabled } from "../lib/account";
import { describeError } from "../lib/errors";
import NavIcon from "./NavIcon.vue";

type AuthType = "totp" | "sms";
type ChannelStatus = "on" | "off" | "pending";

const props = defineProps<{ open: boolean }>();

const emit = defineEmits<{
  close: [];
  toast: [text: string, error?: boolean];
}>();

const state = ref({ totp: false, sms: false });
const loading = ref(false);
const busy = ref(false);
const error = ref("");
/** 正在绑定哪个通道；空串表示只展示开关。 */
const enrolling = ref<AuthType | "">("");
/** 正在等待二次确认的通道；禁用会改变登录方式，必须先确认。 */
const confirming = ref<AuthType | "">("");
const secretKey = ref("");
const qrCode = ref("");
const phone = ref("");
const otp = ref("");
const codeSent = ref(false);
const copied = ref(false);
let copyTimer: ReturnType<typeof setTimeout> | undefined;

/** 开关的视觉状态包含「正在绑定」，避免点了启用后滑块还停在禁用。 */
const visualTotp = computed(() => state.value.totp || enrolling.value === "totp");
const visualSms = computed(() => state.value.sms || enrolling.value === "sms");
const totpStatus = computed<ChannelStatus>(() =>
  enrolling.value === "totp" ? "pending" : state.value.totp ? "on" : "off",
);
const smsStatus = computed<ChannelStatus>(() =>
  enrolling.value === "sms" ? "pending" : state.value.sms ? "on" : "off",
);

/** 两个通道共用一个模板，避免卡片与二次确认各写两遍。 */
const channels = computed(() => [
  {
    type: "totp" as AuthType,
    nameKey: "twofa.totpName",
    icon: "clock",
    visual: visualTotp.value,
    status: totpStatus.value,
  },
  {
    type: "sms" as AuthType,
    nameKey: "twofa.smsName",
    icon: "phone",
    visual: visualSms.value,
    status: smsStatus.value,
  },
]);

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    resetEnrollment();
    error.value = "";
    void loadState();
  },
);

function onKeydown(event: KeyboardEvent): void {
  if (!props.open || event.key !== "Escape") return;
  event.preventDefault();
  // Esc 先撤销二次确认，再退出弹窗，避免误关整层。
  if (confirming.value) {
    confirming.value = "";
    return;
  }
  close();
}

onMounted(() => document.addEventListener("keydown", onKeydown));
onBeforeUnmount(() => {
  document.removeEventListener("keydown", onKeydown);
  if (copyTimer) clearTimeout(copyTimer);
});

function statusLabel(status: ChannelStatus): string {
  if (status === "on") return t("twofa.statusOn");
  if (status === "pending") return t("twofa.statusPending");
  return t("twofa.statusOff");
}

/** 按钮禁用与 handler 守卫共用同一套判定，避免视觉可点但点了没反应。 */
function buttonDisabled(type: AuthType, intent: "enable" | "disable"): boolean {
  return twoFactorButtonDisabled(
    {
      enabled: state.value[type],
      busy: busy.value || loading.value,
      enrolling: enrolling.value === type,
      confirming: confirming.value === type,
    },
    intent,
  );
}

function resetEnrollment(): void {
  enrolling.value = "";
  confirming.value = "";
  secretKey.value = "";
  qrCode.value = "";
  phone.value = "";
  otp.value = "";
  codeSent.value = false;
  copied.value = false;
}

async function loadState(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    state.value = await archeryApi.twoFactorState();
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    loading.value = false;
  }
}

async function enable(type: AuthType): Promise<void> {
  // 已启用、绑定中、确认中或请求中都直接忽略，避免重复申请密钥。
  if (buttonDisabled(type, "enable")) return;
  error.value = "";
  resetEnrollment();
  enrolling.value = type;
  if (type !== "totp") return;
  // TOTP 先向 Archery 申请密钥，再把二维码连同密钥一起展示给用户。
  busy.value = true;
  try {
    const result = await archeryApi.twoFactorStart({ authType: "totp" });
    secretKey.value = result.secretKey ?? "";
    qrCode.value = result.qrCode ?? "";
    if (!qrCode.value) error.value = t("twofa.qrMissing");
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    busy.value = false;
  }
}

/** 禁用会移除一条登录保护，先收起绑定区并展开二次确认。 */
function requestDisable(type: AuthType): void {
  // 未开启、绑定中、确认中或请求中都直接忽略，确认框不会反复重开。
  if (buttonDisabled(type, "disable")) return;
  error.value = "";
  resetEnrollment();
  confirming.value = type;
}

async function disable(type: AuthType): Promise<void> {
  if (busy.value || confirming.value !== type) return;
  busy.value = true;
  error.value = "";
  try {
    await archeryApi.twoFactorDisable(type);
    resetEnrollment();
    await loadState();
    emit("toast", t("twofa.disabledToast"));
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    busy.value = false;
  }
}

async function copySecret(): Promise<void> {
  if (!secretKey.value) return;
  try {
    await archeryApi.copy(secretKey.value);
    copied.value = true;
    if (copyTimer) clearTimeout(copyTimer);
    copyTimer = setTimeout(() => {
      copied.value = false;
    }, 1600);
  } catch {
    emit("toast", t("twofa.copyFailed"), true);
  }
}

async function sendCode(): Promise<void> {
  if (busy.value) return;
  const value = phone.value.trim();
  if (!/^\d{6,20}$/.test(value)) {
    error.value = t("twofa.invalidPhone");
    return;
  }
  error.value = "";
  busy.value = true;
  try {
    await archeryApi.twoFactorStart({ authType: "sms", phone: value });
    codeSent.value = true;
    emit("toast", t("twofa.sentToast"));
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    busy.value = false;
  }
}

async function confirm(): Promise<void> {
  if (busy.value || !enrolling.value) return;
  const code = otp.value.trim();
  if (!/^\d{6}$/.test(code)) {
    error.value = t("twofa.invalidCode");
    return;
  }
  error.value = "";
  busy.value = true;
  try {
    await archeryApi.twoFactorConfirm({
      authType: enrolling.value,
      otp: code,
      secretKey: enrolling.value === "totp" ? secretKey.value : undefined,
      phone: enrolling.value === "sms" ? phone.value.trim() : undefined,
    });
    resetEnrollment();
    await loadState();
    emit("toast", t("twofa.boundToast"));
  } catch (issue) {
    error.value = describeError(issue);
  } finally {
    busy.value = false;
  }
}

function close(): void {
  resetEnrollment();
  emit("close");
}
</script>

<template>
  <div v-if="open" class="modal-mask account-mask" @click.self="close">
    <div class="modal account-modal twofa-modal" role="dialog" aria-modal="true" aria-labelledby="twofa-title">
      <header class="account-head">
        <span class="account-badge"><NavIcon name="shield" :size="19" /></span>
        <div class="account-head-text">
          <h2 id="twofa-title">{{ t("twofa.title") }}</h2>
          <p>{{ t("twofa.subtitle") }}</p>
        </div>
        <button class="modal-close" :title="t('app.close')" @click="close">×</button>
      </header>

      <div class="modal-body account-body">
        <div v-if="loading" class="account-loading">
          <span class="spinner"></span>
          <span>{{ t("twofa.loading") }}</span>
        </div>

        <template v-else>
          <section
            v-for="channel in channels"
            :key="channel.type"
            class="twofa-channel"
            :class="[`is-${channel.status}`, { 'is-confirming': confirming === channel.type }]"
          >
            <div class="twofa-channel-row">
              <div class="twofa-channel-main">
                <span class="twofa-channel-icon"><NavIcon :name="channel.icon" :size="16" /></span>
                <div class="twofa-channel-text">
                  <span class="twofa-channel-name">{{ t(channel.nameKey) }}</span>
                  <span class="twofa-channel-state">
                    <i class="state-dot"></i>
                    {{ statusLabel(channel.status) }}
                  </span>
                </div>
              </div>
              <div
                class="twofa-switch"
                :class="{ on: channel.visual }"
                role="group"
                :aria-label="t(channel.nameKey)"
              >
                <span class="twofa-thumb"></span>
                <button
                  class="twofa-option"
                  :class="{ active: channel.visual }"
                  :aria-pressed="channel.visual"
                  :disabled="buttonDisabled(channel.type, 'enable')"
                  @click="enable(channel.type)"
                >
                  {{ t("twofa.enable") }}
                </button>
                <button
                  class="twofa-option"
                  :class="{ active: !channel.visual }"
                  :aria-pressed="!channel.visual"
                  :disabled="buttonDisabled(channel.type, 'disable')"
                  @click="requestDisable(channel.type)"
                >
                  {{ t("twofa.disable") }}
                </button>
              </div>
            </div>

            <Transition name="enroll">
              <div v-if="confirming === channel.type" class="twofa-confirm">
                <div class="twofa-confirm-head">
                  <span class="twofa-confirm-icon"><NavIcon name="alert" :size="15" /></span>
                  <div class="twofa-confirm-text">
                    <strong>{{ t("twofa.confirmDisableTitle", { name: t(channel.nameKey) }) }}</strong>
                    <p>{{ t("twofa.confirmDisableDesc") }}</p>
                  </div>
                </div>
                <div class="twofa-confirm-actions">
                  <button class="btn ghost" :disabled="busy" @click="confirming = ''">
                    {{ t("app.cancel") }}
                  </button>
                  <button class="btn danger" :disabled="busy" @click="disable(channel.type)">
                    <span v-if="busy" class="spinner" style="border-top-color: #fff"></span>
                    {{ t("twofa.confirmDisable") }}
                  </button>
                </div>
              </div>
            </Transition>
          </section>

          <Transition name="enroll">
            <section v-if="enrolling === 'totp'" class="twofa-enroll">
              <div class="twofa-enroll-title">{{ t("twofa.scanTitle") }}</div>
              <p class="twofa-scan-hint">{{ t("twofa.qrHint") }}</p>
              <div class="twofa-qr-frame">
                <img v-if="qrCode" class="twofa-qr" :src="qrCode" :alt="t('twofa.qrAlt')" />
              </div>
              <div class="twofa-secret">
                <code>{{ secretKey || "—" }}</code>
                <button class="twofa-copy" :class="{ done: copied }" @click="copySecret">
                  <NavIcon :name="copied ? 'check' : 'copy'" :size="13" />
                  {{ copied ? t("twofa.copied") : t("twofa.copySecret") }}
                </button>
              </div>
              <label class="account-field">
                <span>{{ t("twofa.code") }}</span>
                <input
                  v-model="otp"
                  class="account-input otp-input"
                  type="text"
                  inputmode="numeric"
                  maxlength="6"
                  autocomplete="one-time-code"
                  :placeholder="t('twofa.codePlaceholder')"
                  @keyup.enter="confirm"
                />
                <small>{{ t("twofa.codeHint") }}</small>
              </label>
              <div class="account-actions">
                <button class="btn ghost" :disabled="busy" @click="resetEnrollment">
                  {{ t("app.cancel") }}
                </button>
                <button class="btn primary" :disabled="busy" @click="confirm">
                  <span v-if="busy" class="spinner" style="border-top-color: #fff"></span>
                  {{ t("twofa.bind") }}
                </button>
              </div>
            </section>
          </Transition>

          <Transition name="enroll">
            <section v-if="enrolling === 'sms'" class="twofa-enroll">
              <div class="twofa-enroll-title">{{ t("twofa.smsTitle") }}</div>
              <div class="twofa-phone-row">
                <label class="account-field">
                  <span>{{ t("twofa.phone") }}</span>
                  <input
                    v-model="phone"
                    class="account-input"
                    type="text"
                    inputmode="numeric"
                    maxlength="20"
                    :placeholder="t('twofa.phonePlaceholder')"
                  />
                </label>
                <button class="btn twofa-send" :disabled="busy" @click="sendCode">
                  {{ codeSent ? t("twofa.resendCode") : t("twofa.sendCode") }}
                </button>
              </div>
              <label class="account-field">
                <span>{{ t("twofa.code") }}</span>
                <input
                  v-model="otp"
                  class="account-input otp-input"
                  type="text"
                  inputmode="numeric"
                  maxlength="6"
                  autocomplete="one-time-code"
                  :placeholder="t('twofa.codePlaceholder')"
                  @keyup.enter="confirm"
                />
                <small>{{ t("twofa.smsCodeHint") }}</small>
              </label>
              <div class="account-actions">
                <button class="btn ghost" :disabled="busy" @click="resetEnrollment">
                  {{ t("app.cancel") }}
                </button>
                <button class="btn primary" :disabled="busy" @click="confirm">
                  <span v-if="busy" class="spinner" style="border-top-color: #fff"></span>
                  {{ t("twofa.bind") }}
                </button>
              </div>
            </section>
          </Transition>
        </template>

        <div v-if="error" class="error-box account-error">{{ error }}</div>
      </div>

      <footer class="modal-foot account-foot">
        <span class="account-hint"><kbd>Esc</kbd>{{ t("app.escClose") }}</span>
        <button class="btn ghost" @click="close">{{ t("app.close") }}</button>
      </footer>
    </div>
  </div>
</template>
