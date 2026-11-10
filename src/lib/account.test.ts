import { describe, expect, it } from "vitest";
import {
  passwordChecks,
  passwordIssueKey,
  twoFactorButtonDisabled,
  validatePasswordForm,
} from "./account";

const base = {
  username: "tester",
  oldPassword: "secret",
  newPassword: "newpassword123",
  confirmPassword: "newpassword123",
};

describe("validatePasswordForm", () => {
  it("accepts a valid form", () => {
    expect(validatePasswordForm(base)).toBeNull();
  });

  it("requires all three fields", () => {
    expect(validatePasswordForm({ ...base, oldPassword: "  " })).toBe("required");
    expect(validatePasswordForm({ ...base, newPassword: "" })).toBe("required");
    expect(validatePasswordForm({ ...base, confirmPassword: "" })).toBe("required");
  });

  it("rejects passwords shorter than 9 characters", () => {
    expect(validatePasswordForm({ ...base, newPassword: "short123", confirmPassword: "short123" })).toBe(
      "tooShort",
    );
  });

  it("rejects all-numeric passwords", () => {
    expect(
      validatePasswordForm({ ...base, newPassword: "123456789", confirmPassword: "123456789" }),
    ).toBe("allNumeric");
  });

  it("rejects mismatched confirmation", () => {
    expect(validatePasswordForm({ ...base, confirmPassword: "newpassword124" })).toBe("mismatch");
  });

  it("rejects reusing the old password", () => {
    expect(
      validatePasswordForm({ ...base, newPassword: "secret", confirmPassword: "secret" }),
    ).toBe("sameAsOld");
  });

  it("rejects passwords built from the username", () => {
    expect(
      validatePasswordForm({
        ...base,
        username: "tester",
        newPassword: "tester-tester",
        confirmPassword: "tester-tester",
      }),
    ).toBe("tooSimilar");
  });

  it("maps issues to i18n keys", () => {
    expect(passwordIssueKey("required")).toBe("password.required");
    expect(passwordIssueKey("tooShort")).toBe("password.ruleLength");
    expect(passwordIssueKey("allNumeric")).toBe("password.ruleNumeric");
    expect(passwordIssueKey("mismatch")).toBe("password.mismatch");
    expect(passwordIssueKey("sameAsOld")).toBe("password.sameAsOld");
    expect(passwordIssueKey("tooSimilar")).toBe("password.ruleSimilar");
  });
});

describe("passwordChecks", () => {
  it("marks every rule when the form is valid", () => {
    expect(passwordChecks(base)).toEqual({
      length: true,
      notNumeric: true,
      notSimilar: true,
      matches: true,
    });
  });

  it("keeps short passwords from satisfying the length rule", () => {
    const checks = passwordChecks({ ...base, newPassword: "short", confirmPassword: "short" });
    expect(checks.length).toBe(false);
    expect(checks.notNumeric).toBe(true);
    expect(checks.matches).toBe(true);
  });

  it("rejects all-numeric passwords", () => {
    const checks = passwordChecks({
      ...base,
      newPassword: "123456789",
      confirmPassword: "123456789",
    });
    expect(checks.notNumeric).toBe(false);
  });

  it("rejects passwords too similar to the account", () => {
    const checks = passwordChecks({
      ...base,
      newPassword: "tester-tester",
      confirmPassword: "tester-tester",
    });
    expect(checks.notSimilar).toBe(false);
  });

  it("treats reusing the old password as too similar", () => {
    const checks = passwordChecks({ ...base, newPassword: "secret", confirmPassword: "secret" });
    expect(checks.notSimilar).toBe(false);
  });

  it("requires matching confirmation", () => {
    const checks = passwordChecks({ ...base, confirmPassword: "newpassword124" });
    expect(checks.matches).toBe(false);
  });

  it("marks nothing for an empty form", () => {
    expect(
      passwordChecks({ ...base, oldPassword: "", newPassword: "", confirmPassword: "" }),
    ).toEqual({ length: false, notNumeric: false, notSimilar: false, matches: false });
  });
});

describe("twoFactorButtonDisabled", () => {
  const enabled = { enabled: true, busy: false, enrolling: false, confirming: false };

  it("keeps only the opposite action clickable", () => {
    expect(twoFactorButtonDisabled(enabled, "enable")).toBe(true);
    expect(twoFactorButtonDisabled(enabled, "disable")).toBe(false);
    expect(twoFactorButtonDisabled({ ...enabled, enabled: false }, "enable")).toBe(false);
    expect(twoFactorButtonDisabled({ ...enabled, enabled: false }, "disable")).toBe(true);
  });

  it("locks both buttons while a request is running", () => {
    expect(twoFactorButtonDisabled({ ...enabled, busy: true }, "enable")).toBe(true);
    expect(twoFactorButtonDisabled({ ...enabled, busy: true }, "disable")).toBe(true);
  });

  it("locks both buttons while binding", () => {
    expect(twoFactorButtonDisabled({ ...enabled, enrolling: true }, "enable")).toBe(true);
    expect(twoFactorButtonDisabled({ ...enabled, enrolling: true }, "disable")).toBe(true);
  });

  it("locks both buttons while the disable confirmation is open", () => {
    expect(twoFactorButtonDisabled({ ...enabled, confirming: true }, "enable")).toBe(true);
    expect(twoFactorButtonDisabled({ ...enabled, confirming: true }, "disable")).toBe(true);
  });
});
