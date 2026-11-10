/**
 * 剪贴板写入的唯一出口。
 *
 * DBX 的插件 UI 跑在 `sandbox="allow-scripts"` 的 opaque-origin iframe 里：
 * 该文档没有 clipboard 权限，`navigator.clipboard.writeText` 会被 Permissions Policy 拒绝，
 * 且 copy 事件里的 `clipboardData.setData` 会被静默丢弃。所以写剪贴板必须优先走宿主桥
 * `window.dbxPlugin.copy()` / `window.dbxPlugin.clipboard.writeText()`（写入免权限）。
 * 独立浏览器预览里没有宿主桥，则依次降级到原生 API 与 textarea 兜底。
 */

/** 剪贴板写入的各条路径，按优先级排列；未注入的路径会被跳过。 */
export interface ClipboardDeps {
  /** DBX 宿主桥 `dbxPlugin.copy`。 */
  hostCopy?: (text: string) => Promise<void> | void;
  /** DBX 宿主桥 `dbxPlugin.clipboard.writeText`，与 `copy` 同一实现。 */
  hostWriteText?: (text: string) => Promise<void> | void;
  /** 普通浏览器 / 预览页的 `navigator.clipboard.writeText`。 */
  nativeWriteText?: (text: string) => Promise<void>;
  /** 最后兜底：临时 textarea + `document.execCommand("copy")`。 */
  legacyCopy?: (text: string) => boolean;
}

/** execCommand 会再次触发 copy 事件，用标记防止编辑器处理器递归调用。 */
let legacyBusy = false;

function legacyCopy(text: string): boolean {
  if (legacyBusy || typeof document === "undefined") return false;
  legacyBusy = true;
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.top = "-1000px";
  area.style.opacity = "0";
  const selection = typeof document.getSelection === "function" ? document.getSelection() : null;
  const previous = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null;
  let ok = false;
  try {
    document.body.appendChild(area);
    area.select();
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  } finally {
    area.remove();
    // execCommand 会把选区挪到 textarea，复制后还原，避免编辑器丢失光标与选区。
    if (previous && selection) {
      selection.removeAllRanges();
      selection.addRange(previous);
    }
    legacyBusy = false;
  }
  return ok;
}

/** 按当前运行环境解析各条写入路径，调用时实时读取，避免缓存过期的宿主引用。 */
export function resolveClipboardDeps(): ClipboardDeps {
  const deps: ClipboardDeps = {};
  const plugin = typeof window === "undefined" ? undefined : window.dbxPlugin;
  if (plugin) {
    if (typeof plugin.copy === "function") {
      deps.hostCopy = (text) => plugin.copy?.(text);
    }
    // `clipboard` 未写进 dbx.d.ts 的宿主契约，这里按官方文档做窄类型断言。
    const hostClipboard = (plugin as { clipboard?: { writeText?: (text: string) => Promise<void> | void } })
      .clipboard;
    if (typeof hostClipboard?.writeText === "function") {
      deps.hostWriteText = (text) => hostClipboard.writeText?.(text);
    }
  }
  if (typeof navigator !== "undefined" && typeof navigator.clipboard?.writeText === "function") {
    deps.nativeWriteText = (text) => navigator.clipboard.writeText(text);
  }
  if (typeof document !== "undefined") {
    deps.legacyCopy = legacyCopy;
  }
  return deps;
}

/**
 * 把文本写入系统剪贴板，逐级降级且永不抛错。
 * @returns 是否至少有一条路径成功。
 */
export async function writeClipboard(
  text: string,
  deps: ClipboardDeps = resolveClipboardDeps(),
): Promise<boolean> {
  const steps: Array<() => Promise<boolean>> = [
    async () => {
      if (!deps.hostCopy) return false;
      await deps.hostCopy(text);
      return true;
    },
    async () => {
      if (!deps.hostWriteText) return false;
      await deps.hostWriteText(text);
      return true;
    },
    async () => {
      if (!deps.nativeWriteText) return false;
      await deps.nativeWriteText(text);
      return true;
    },
    async () => {
      if (!deps.legacyCopy) return false;
      return deps.legacyCopy(text);
    },
  ];
  for (const step of steps) {
    try {
      if (await step()) return true;
    } catch {
      // 该路径不可用（缺权限 / 沙箱拦截），继续降级。
    }
  }
  return false;
}
