import { describe, expect, it, vi } from "vitest";
import { parentDir, saveBinaryFile, toBase64, type BinarySaveDeps } from "./download";
import { XLSX_MIME } from "./xlsx";

const bytes = new Uint8Array([0x50, 0x4b, 0x03, 0x04]);

function deps(patch: Partial<BinarySaveDeps> = {}): BinarySaveDeps {
  return {
    saveFile: vi.fn(async () => ({ path: "/tmp/query-result.xlsx" })),
    saveByAnchor: vi.fn(),
    ...patch,
  };
}

describe("toBase64", () => {
  it("按标准 base64 编码并补齐 padding", () => {
    expect(toBase64(new Uint8Array())).toBe("");
    expect(toBase64(new Uint8Array([0x41]))).toBe("QQ==");
    expect(toBase64(new Uint8Array([0x41, 0x42]))).toBe("QUI=");
    expect(toBase64(new Uint8Array([0x41, 0x42, 0x43]))).toBe("QUJD");
    expect(toBase64(bytes)).toBe("UEsDBA==");
  });
});

describe("saveBinaryFile", () => {
  it("走 sidecar 桥时把字节转成 contentBase64", async () => {
    const spy = deps();
    await expect(
      saveBinaryFile(bytes, "query-result.xlsx", { mime: XLSX_MIME }, spy),
    ).resolves.toEqual({ status: "saved", path: "/tmp/query-result.xlsx" });
    expect(spy.saveFile).toHaveBeenCalledWith({
      filename: "query-result.xlsx",
      content: "",
      contentBase64: "UEsDBA==",
    });
    expect(spy.saveByAnchor).not.toHaveBeenCalled();
  });

  it("用户取消保存时不回退浏览器下载", async () => {
    const spy = deps({ saveFile: vi.fn(async () => ({ canceled: true })) });
    await expect(saveBinaryFile(bytes, "query-result.xlsx", {}, spy)).resolves.toEqual({
      status: "canceled",
    });
    expect(spy.saveByAnchor).not.toHaveBeenCalled();
  });

  it("invoke 不可用时回退浏览器下载并带上 xlsx MIME", async () => {
    const spy = deps({
      saveFile: vi.fn(async () => {
        throw new Error("no bridge");
      }),
    });
    await expect(
      saveBinaryFile(bytes, "query-result.xlsx", { mime: XLSX_MIME }, spy),
    ).resolves.toEqual({ status: "browser" });
    expect(spy.saveByAnchor).toHaveBeenCalledWith(bytes, "query-result.xlsx", XLSX_MIME);
  });

  it("另存为参数（askPath / initialDir）原样透传给 sidecar", async () => {
    const spy = deps();
    await saveBinaryFile(
      bytes,
      "query-result.xlsx",
      { mime: XLSX_MIME, askPath: true, initialDir: "D:\\exports" },
      spy,
    );
    expect(spy.saveFile).toHaveBeenCalledWith({
      filename: "query-result.xlsx",
      content: "",
      contentBase64: "UEsDBA==",
      askPath: true,
      initialDir: "D:\\exports",
    });
  });
});

describe("parentDir", () => {
  it("取落盘路径的所在目录", () => {
    expect(parentDir("C:\\Users\\demo\\Downloads\\query-result.xlsx")).toBe(
      "C:\\Users\\demo\\Downloads",
    );
    expect(parentDir("/home/demo/out/query-result.xlsx")).toBe("/home/demo/out");
    expect(parentDir("C:\\query-result.xlsx")).toBe("C:");
    expect(parentDir("query-result.xlsx")).toBe("");
  });
});
