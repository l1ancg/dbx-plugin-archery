import { archeryApi } from "../api";

export type SaveTextResult =
  | { status: "saved"; path: string }
  | { status: "canceled" }
  | { status: "browser" };

/**
 * 保存文本文件。DBX 的插件 UI 跑在 opaque-origin iframe 沙箱里，
 * `<a download>` 会被宿主拦截，因此由 sidecar 完成：
 * - askPath=true：弹系统「另存为」对话框，用户自选路径（取消返回 canceled）；
 * - 否则直写本机下载目录；
 * - invoke 不可用时（独立浏览器预览等）回退浏览器下载，返回 browser。
 */
export async function saveTextFile(
  content: string,
  filename: string,
  options: { askPath?: boolean; mime?: string } = {},
): Promise<SaveTextResult> {
  const mime = options.mime ?? "application/sql;charset=utf-8";
  try {
    const result = await archeryApi.saveTextFile({
      filename,
      content,
      askPath: options.askPath,
    });
    if (result.canceled) return { status: "canceled" };
    if (result.path) return { status: "saved", path: result.path };
    return { status: "canceled" };
  } catch {
    saveByAnchor(content, filename, mime);
    return { status: "browser" };
  }
}

function saveByAnchor(content: string, filename: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

/** 二进制落盘的依赖，测试直接注入，不 mock 模块。 */
export interface BinarySaveDeps {
  /** 交给 sidecar 落盘：RPC 只收文本，文件字节按标准 base64 放 contentBase64。 */
  saveFile: (params: {
    filename: string;
    content: string;
    contentBase64: string;
    askPath?: boolean;
    initialDir?: string;
  }) => Promise<{ path?: string; canceled?: boolean }>;
  /** invoke 不可用时（独立浏览器预览等）回退浏览器下载。 */
  saveByAnchor: (bytes: Uint8Array, filename: string, mime: string) => void;
}

const BASE64_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/** 标准 base64（带 padding）。 */
export function toBase64(bytes: Uint8Array): string {
  let text = "";
  for (let index = 0; index < bytes.length; index += 3) {
    const first = bytes[index];
    const second = index + 1 < bytes.length ? bytes[index + 1] : 0;
    const third = index + 2 < bytes.length ? bytes[index + 2] : 0;
    text += BASE64_ALPHABET[first >> 2];
    text += BASE64_ALPHABET[((first & 0x03) << 4) | (second >> 4)];
    text += index + 1 < bytes.length ? BASE64_ALPHABET[((second & 0x0f) << 2) | (third >> 6)] : "=";
    text += index + 2 < bytes.length ? BASE64_ALPHABET[third & 0x3f] : "=";
  }
  return text;
}

const defaultBinaryDeps: BinarySaveDeps = {
  saveFile: (params) =>
    archeryApi.saveTextFile({
      filename: params.filename,
      content: params.content,
      contentBase64: params.contentBase64,
      askPath: params.askPath,
      initialDir: params.initialDir,
    }),
  saveByAnchor: saveBytesByAnchor,
};

/**
 * 保存二进制文件（Excel 导出）。与 saveTextFile 同一套降级：
 * sidecar 落盘 → 取消 → invoke 不可用时回退浏览器下载。
 */
export async function saveBinaryFile(
  bytes: Uint8Array,
  filename: string,
  options: { mime?: string; askPath?: boolean; initialDir?: string } = {},
  deps: BinarySaveDeps = defaultBinaryDeps,
): Promise<SaveTextResult> {
  const mime = options.mime ?? "application/octet-stream";
  const params: {
    filename: string;
    content: string;
    contentBase64: string;
    askPath?: boolean;
    initialDir?: string;
  } = { filename, content: "", contentBase64: toBase64(bytes) };
  // 可选参数按需写入：不传时不改变既有调用形状。
  if (options.askPath) params.askPath = true;
  if (options.initialDir) params.initialDir = options.initialDir;
  try {
    const result = await deps.saveFile(params);
    if (result.canceled) return { status: "canceled" };
    if (result.path) return { status: "saved", path: result.path };
    return { status: "canceled" };
  } catch {
    deps.saveByAnchor(bytes, filename, mime);
    return { status: "browser" };
  }
}

function saveBytesByAnchor(bytes: Uint8Array, filename: string, mime: string): void {
  const blob = new Blob([bytes], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

/** 取落盘路径的所在目录（兼容 `\` 与 `/`）；没有分隔符时返回空串。 */
export function parentDir(path: string): string {
  const at = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
  return at > 0 ? path.slice(0, at) : "";
}
