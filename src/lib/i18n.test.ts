import { describe, expect, it } from "vitest";
import { messages, t } from "../i18n";

function flatten(input: Record<string, unknown>, prefix = ""): string[] {
  const keys: string[] = [];
  for (const [key, value] of Object.entries(input)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === "object") {
      keys.push(...flatten(value as Record<string, unknown>, path));
    } else {
      keys.push(path);
    }
  }
  return keys.sort();
}

describe("i18n", () => {
  it("keeps zh-CN and en key sets identical", () => {
    expect(flatten(messages.en)).toEqual(flatten(messages["zh-CN"]));
  });

  it("interpolates parameters", () => {
    expect(t("result.rows", { n: 3 })).toContain("3");
  });

  it("falls back to the key when missing", () => {
    expect(t("missing.key" as never)).toBe("missing.key");
  });
});
