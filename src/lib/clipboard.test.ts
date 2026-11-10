import { describe, expect, it, vi } from "vitest";
import { writeClipboard } from "./clipboard";
import type { ClipboardDeps } from "./clipboard";

function fullDeps(patch: Partial<ClipboardDeps> = {}): Required<ClipboardDeps> {
  return {
    hostCopy: vi.fn(() => undefined),
    hostWriteText: vi.fn(async () => undefined),
    nativeWriteText: vi.fn(async () => undefined),
    legacyCopy: vi.fn(() => true),
    ...patch,
  };
}

describe("writeClipboard", () => {
  it("优先走宿主 copy 桥，且不再触发其他路径", async () => {
    const deps = fullDeps();
    await expect(writeClipboard("select 1", deps)).resolves.toBe(true);
    expect(deps.hostCopy).toHaveBeenCalledWith("select 1");
    expect(deps.hostWriteText).not.toHaveBeenCalled();
    expect(deps.nativeWriteText).not.toHaveBeenCalled();
    expect(deps.legacyCopy).not.toHaveBeenCalled();
  });

  it("宿主 copy 抛错时降级到 clipboard.writeText", async () => {
    const deps = fullDeps({
      hostCopy: vi.fn(() => {
        throw new Error("copy bridge unavailable");
      }),
    });
    await expect(writeClipboard("select 2", deps)).resolves.toBe(true);
    expect(deps.hostWriteText).toHaveBeenCalledWith("select 2");
    expect(deps.nativeWriteText).not.toHaveBeenCalled();
  });

  it("宿主桥缺失时降级到 navigator.clipboard", async () => {
    const nativeWriteText = vi.fn(async () => undefined);
    const deps: ClipboardDeps = { nativeWriteText, legacyCopy: vi.fn(() => true) };
    await expect(writeClipboard("select 3", deps)).resolves.toBe(true);
    expect(nativeWriteText).toHaveBeenCalledWith("select 3");
    expect(deps.legacyCopy).not.toHaveBeenCalled();
  });

  it("原生写入被沙箱拒绝时降级到 textarea 兜底", async () => {
    const legacyCopy = vi.fn(() => true);
    const deps: ClipboardDeps = {
      nativeWriteText: vi.fn(async () => {
        throw new Error("blocked by permissions policy");
      }),
      legacyCopy,
    };
    await expect(writeClipboard("select 4", deps)).resolves.toBe(true);
    expect(legacyCopy).toHaveBeenCalledWith("select 4");
  });

  it("所有路径都失败时返回 false 且不抛异常", async () => {
    const deps: ClipboardDeps = {
      hostCopy: vi.fn(() => {
        throw new Error("no bridge");
      }),
      hostWriteText: vi.fn(async () => {
        throw new Error("no bridge");
      }),
      nativeWriteText: vi.fn(async () => {
        throw new Error("blocked");
      }),
      legacyCopy: vi.fn(() => false),
    };
    await expect(writeClipboard("select 5", deps)).resolves.toBe(false);
  });

  it("未被注入的路径直接跳过，不产生副作用", async () => {
    const legacyCopy = vi.fn(() => false);
    await expect(writeClipboard("select 6", { legacyCopy })).resolves.toBe(false);
    expect(legacyCopy).toHaveBeenCalledTimes(1);
  });
});
