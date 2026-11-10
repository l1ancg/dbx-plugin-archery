import { describe, expect, it } from "vitest";
import { resolveConnectionId, resolveConnectionName } from "./context";

describe("resolveConnectionId", () => {
  it("prefers the top-level connectionId", () => {
    expect(resolveConnectionId({ connectionId: "c-1", connection: { id: "c-2" } })).toBe("c-1");
  });

  it("falls back to connection.id", () => {
    expect(resolveConnectionId({ connection: { id: "c-2" } })).toBe("c-2");
  });

  it("falls back to id", () => {
    expect(resolveConnectionId({ id: "c-3" })).toBe("c-3");
  });

  it("returns empty for missing or malformed context", () => {
    expect(resolveConnectionId(undefined)).toBe("");
    expect(resolveConnectionId({})).toBe("");
    expect(resolveConnectionId({ connectionId: "   " })).toBe("");
    expect(resolveConnectionId({ connection: {} })).toBe("");
  });
});

describe("resolveConnectionName", () => {
  it("reads connectionName, name and connection.name", () => {
    expect(resolveConnectionName({ connectionName: "生产" })).toBe("生产");
    expect(resolveConnectionName({ name: "预发" })).toBe("预发");
    expect(resolveConnectionName({ connection: { name: "测试" } })).toBe("测试");
  });

  it("keeps the fallback when context is empty", () => {
    expect(resolveConnectionName(undefined)).toBe("Archery");
    expect(resolveConnectionName({}, "自定义")).toBe("自定义");
  });
});
