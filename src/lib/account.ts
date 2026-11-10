/** 修改密码表单：旧密码 + 两次新密码。 */
export interface PasswordForm {
  username?: string;
  oldPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export type PasswordIssue =
  | "required"
  | "sameAsOld"
  | "tooShort"
  | "allNumeric"
  | "mismatch"
  | "tooSimilar";

/**
 * 与 Archery（Django AUTH_PASSWORD_VALIDATORS）对齐的前端预校验。
 * 服务端仍会再校验一次，这里只负责在提交前给出明确提示。
 */
export function validatePasswordForm(form: PasswordForm): PasswordIssue | null {
  const current = form.oldPassword.trim();
  const next = form.newPassword.trim();
  const confirm = form.confirmPassword.trim();
  if (!current || !next || !confirm) return "required";
  if (next === current) return "sameAsOld";
  if (next.length < 9) return "tooShort";
  if (/^\d+$/.test(next)) return "allNumeric";
  if (next !== confirm) return "mismatch";
  if (looksLikeUsername(next, form.username)) return "tooSimilar";
  return null;
}

/** 修改密码弹窗的实时规则勾选状态。 */
export interface PasswordChecks {
  /** 至少 9 个字符 */
  length: boolean;
  /** 不是纯数字 */
  notNumeric: boolean;
  /** 不包含账号名，也不等于旧密码 */
  notSimilar: boolean;
  /** 两次输入一致 */
  matches: boolean;
}

export function passwordChecks(form: PasswordForm): PasswordChecks {
  const next = form.newPassword.trim();
  const filled = next.length > 0;
  return {
    length: next.length >= 9,
    notNumeric: filled && !/^\d+$/.test(next),
    notSimilar:
      filled && next !== form.oldPassword.trim() && !looksLikeUsername(next, form.username),
    matches: filled && next === form.confirmPassword.trim(),
  };
}

export function passwordIssueKey(issue: PasswordIssue): string {
  switch (issue) {
    case "required":
      return "password.required";
    case "sameAsOld":
      return "password.sameAsOld";
    case "tooShort":
      return "password.ruleLength";
    case "allNumeric":
      return "password.ruleNumeric";
    case "mismatch":
      return "password.mismatch";
    case "tooSimilar":
      return "password.ruleSimilar";
  }
}

/** 两步验证开关的上下文：当前状态 + 三种锁定态。 */
export interface TwoFactorSwitchState {
  enabled: boolean;
  busy: boolean;
  enrolling: boolean;
  confirming: boolean;
}

export type TwoFactorToggleIntent = "enable" | "disable";

/**
 * 开关按钮是否应禁用。
 * 同一时刻只允许「会改变状态」的那个按钮可点：已开启只能点禁用，未开启只能点启用；
 * 请求中、绑定中、禁用确认中三个状态一律锁死，避免重复触发。
 */
export function twoFactorButtonDisabled(
  state: TwoFactorSwitchState,
  intent: TwoFactorToggleIntent,
): boolean {
  if (state.busy || state.enrolling || state.confirming) return true;
  return intent === "enable" ? state.enabled : !state.enabled;
}

/** 密码里包含账号名（忽略大小写与符号）即视为过于相似。 */
function looksLikeUsername(password: string, username?: string): boolean {
  const name = normalize(username);
  if (name.length < 3) return false;
  return normalize(password).includes(name);
}

function normalize(value?: string): string {
  return (value ?? "").toLowerCase().replace(/[^a-z0-9]/g, "");
}
