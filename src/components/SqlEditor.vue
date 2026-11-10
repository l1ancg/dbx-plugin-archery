<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { basicSetup } from "codemirror";
import { EditorState, Prec } from "@codemirror/state";
import { EditorView, keymap, placeholder as placeholderExt } from "@codemirror/view";
import { sql } from "@codemirror/lang-sql";
import { t } from "../i18n";
import { writeClipboard } from "../lib/clipboard";
import { installDarkSqlHighlight } from "../lib/sql-highlight";

const props = defineProps<{
  modelValue: string;
  placeholder?: string;
  /**
   * 同一个编辑器实例被多个查询页签复用时传入页签 id：
   * 每个页签各自保存一份 EditorState（文档 / 光标 / 选区 / 撤销历史），
   * 切页签只换状态、不做整篇替换 —— 撤销历史不会跨页签串台。
   */
  tabId?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  run: [payload: { selection: string; cursor: number }];
}>();

const host = ref<HTMLDivElement | null>(null);
let view: EditorView | null = null;

const theme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    color: "var(--fg)",
    height: "100%",
  },
  ".cm-content": {
    fontFamily: "var(--mono)",
    padding: "8px 0",
    caretColor: "var(--accent)",
  },
  ".cm-line": { padding: "0 12px" },
  ".cm-gutters": {
    backgroundColor: "var(--bg-inset)",
    color: "var(--fg-subtle)",
    border: "none",
    borderRight: "1px solid var(--border)",
    paddingLeft: "4px",
  },
  ".cm-activeLine": { backgroundColor: "var(--editor-active-line)" },
  // 有选区时让出当前行底色：选区层在负 z-index，会被整行底色整条盖住。
  "&:has(.cm-selectionLayer .cm-selectionBackground) .cm-activeLine": {
    backgroundColor: "transparent",
  },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--fg-muted)" },
  // drawSelection 在基础主题里用「.cm-editor.cm-focused > .cm-scroller > .cm-selectionLayer
  // .cm-selectionBackground」这条高特异性选择器，必须写成同样的形状才能覆盖掉默认紫。
  ".cm-selectionBackground": { backgroundColor: "var(--editor-selection)" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground": {
    backgroundColor: "var(--editor-selection)",
  },
  "& ::selection": { backgroundColor: "var(--editor-selection)" },
  ".cm-cursor": { borderLeftColor: "var(--accent)" },
  ".cm-tooltip": {
    backgroundColor: "var(--bg-elevated)",
    border: "1px solid var(--border-strong)",
    borderRadius: "6px",
    color: "var(--fg)",
  },
  ".cm-tooltip-autocomplete ul li[aria-selected]": {
    backgroundColor: "var(--selected)",
    color: "var(--fg)",
  },
});

function selectionText(instance: EditorView): string {
  const ranges = instance.state.selection.ranges.filter((range) => !range.empty);
  if (ranges.length === 0) return "";
  return ranges
    .map((range) => instance.state.sliceDoc(range.from, range.to))
    .join("\n");
}

/** 每个查询页签各自的编辑器状态；页签关掉后内容仍以 App 的 sql 为准，这里只丢撤销历史。 */
const states = new Map<string, EditorState>();
/** 状态缓存上限：页签数量极端多时丢掉最早的一份，避免历史状态无限堆积。 */
const MAX_STATES = 20;

function createState(doc: string): EditorState {
  return EditorState.create({
    doc,
    extensions: [
      basicSetup,
      sql(),
      theme,
      EditorView.lineWrapping,
      placeholderExt(props.placeholder ?? t("editor.placeholder")),
      // 必须压过 basicSetup 里的 defaultKeymap：那里把 Mod-Enter 绑成了 insertBlankLine，
      // 不提高优先级时 Ctrl+Enter 只会插一个空行，执行键位永远轮不到。
      Prec.highest(
        keymap.of([
          {
            key: "Mod-Enter",
            preventDefault: true,
            run: (instance) => {
              emit("run", {
                selection: selectionText(instance),
                cursor: instance.state.selection.main.head,
              });
              return true;
            },
          },
        ]),
      ),
      EditorView.domEventHandlers({
        copy: (event, instance) => {
          const text = selectionText(instance);
          if (!text) return false;
          // 沙箱内原生写剪贴板会被静默丢弃，必须同时走宿主桥；
          // setData 让普通浏览器（预览）保持同步写入，不依赖异步 API。
          event.clipboardData?.setData("text/plain", text);
          event.preventDefault();
          void writeClipboard(text);
          return true;
        },
      }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          emit("update:modelValue", update.state.doc.toString());
        }
      }),
    ],
  });
}

/** 存一份页签状态：重插到队尾，超上限时丢掉最早的那份。 */
function saveState(id: string, state: EditorState): void {
  states.delete(id);
  states.set(id, state);
  while (states.size > MAX_STATES) {
    const oldest = states.keys().next().value;
    if (oldest === undefined) break;
    states.delete(oldest);
  }
}

onMounted(() => {
  if (!host.value) return;
  view = new EditorView({
    parent: host.value,
    state: createState(props.modelValue),
  });
  // CodeMirror 在创建视图时才注入高亮样式表，紧跟其后把浅色 token 色改写成深色变量；
  // 万一这次还没注入成功，下一帧再试一次（已写入过会直接返回）。
  if (!installDarkSqlHighlight(document)) {
    requestAnimationFrame(() => installDarkSqlHighlight(document));
  }
});

watch(
  () => [props.tabId, props.modelValue] as const,
  ([id, value], [previousId]) => {
    if (!view) return;
    // 换页签：把当前状态存回上一个页签，再整份换成新页签的状态。
    // 不能沿用整篇 dispatch —— 那会把「切页签」记进撤销历史，
    // 用户在新页签里按 Ctrl+Z 就能撤出上一个页签的 SQL。
    if (id !== previousId) {
      if (previousId) saveState(previousId, view.state);
      const saved = id ? states.get(id) : undefined;
      view.setState(saved ?? createState(value));
      if (view.state.doc.toString() !== value) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: value },
        });
      }
      return;
    }
    // 同一页签内的外部改动（载入历史 / 美化 / 清空）：整篇替换，保留撤销历史。
    const current = view.state.doc.toString();
    if (current === value) return;
    view.dispatch({
      changes: { from: 0, to: current.length, insert: value },
    });
  },
);

onBeforeUnmount(() => {
  view?.destroy();
  view = null;
});

defineExpose({
  focus(): void {
    view?.focus();
  },
  getCursor(): number {
    return view?.state.selection.main.head ?? 0;
  },
  selectedText(): string {
    return view ? selectionText(view) : "";
  },
});
</script>

<template>
  <div ref="host" class="editor-host"></div>
</template>
