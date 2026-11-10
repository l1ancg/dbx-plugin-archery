/**
 * 深色主题下的 SQL 语法染色。
 *
 * CodeMirror 自带的 defaultHighlightStyle 把 token 颜色写死成浅色系（关键字 #708、
 * 注释 #940 …），铺在 #1a1e23 底上关键字几乎读不出来。语言包没有提供改写入口，
 * 这里把它注入的样式表读回来，按默认调色板把每种 token 色重定向到 `--sql-*` 变量，
 * 再用 `[data-theme="dark"]` 加一级特异性覆盖。不新增依赖，也不重建编辑器：
 * 主题切换只换 CSS 变量，语言包升级后没认出来的 token 自动退回原色。
 */

/** 默认调色板（浅色）→ 深色变量名；键统一成 #rrggbb。 */
const DARK_VAR_BY_LIGHT: Record<string, string> = {
  "#770088": "--sql-keyword", // keyword（#708）
  "#221199": "--sql-atom", // atom / bool / null（#219）
  "#116644": "--sql-number", // number / literal（#164）
  "#aa1111": "--sql-string", // string / deleted（#a11）
  "#ee4400": "--sql-string-quoted", // special(string) / regexp（#e40）
  "#008855": "--sql-type", // typeName / namespace（#085）
  "#116677": "--sql-type", // className（#167）
  "#225566": "--sql-name-special", // special(variableName) / macro（#256）
  "#0000ff": "--sql-var", // definition(variableName)（#00f）
  "#3300aa": "--sql-var", // local(variableName)（#30a）
  "#0000cc": "--sql-var", // definition(propertyName)（#00c）
  "#994400": "--sql-comment", // comment（#940）
  "#404740": "--sql-meta", // meta（#404740）
  "#ff0000": "--sql-invalid", // invalid（#f00）
};

/** CodeMirror 用 style-mod 生成的类名以 U+037C 开头（形如 `.ͼb`），只认这种单类选择器。 */
const TOKEN_SELECTOR = /^\.\u037c[\w-]+$/;

const STYLE_ID = "cm-sql-dark-highlight";

/** hex / rgb() / rgba() 统一成小写 #rrggbb；无法识别时返回 null。 */
export function normalizeColor(value: string): string | null {
  const text = value.trim().toLowerCase();
  if (!text) return null;
  const hex = text.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/);
  if (hex) {
    const body = hex[1];
    const full = body.length === 3 ? body.replace(/./g, (char) => char + char) : body;
    return `#${full}`;
  }
  const rgb = text.match(/^rgba?\(([^)]+)\)$/);
  if (!rgb) return null;
  const parts = rgb[1]
    .split(/[\s,/]+/)
    .filter(Boolean)
    .slice(0, 3)
    .map(Number);
  if (parts.length < 3 || parts.some((part) => Number.isNaN(part))) return null;
  const hexPart = parts
    .map((part) => Math.max(0, Math.min(255, Math.round(part))).toString(16).padStart(2, "0"))
    .join("");
  return `#${hexPart}`;
}

/** 是否是 CodeMirror 生成的高亮 token 类选择器。 */
export function isTokenSelector(selector: string): boolean {
  return TOKEN_SELECTOR.test(selector.trim());
}

/** 单条 token 规则 → 深色覆盖规则；不是已知 token 色时返回 null。 */
export function darkRuleFor(selector: string, color: string): string | null {
  if (!isTokenSelector(selector)) return null;
  const key = normalizeColor(color);
  const variable = key ? DARK_VAR_BY_LIGHT[key] : undefined;
  if (!variable) return null;
  return `[data-theme="dark"] ${selector.trim()}{color:var(${variable})}`;
}

/**
 * 收集样式表里所有可改写的 token 规则，写入一段独立样式；已写过则直接返回。
 * 返回 false 表示还没找到 token 规则（样式表尚未注入），可以稍后重试。
 */
export function installDarkSqlHighlight(doc: Document): boolean {
  if (doc.getElementById(STYLE_ID)) return true;
  const rules: string[] = [];
  const collect = (list: CSSRuleList | undefined | null): void => {
    if (!list) return;
    for (const rule of Array.from(list)) {
      if (rule instanceof CSSStyleRule) {
        const dark = darkRuleFor(rule.selectorText, rule.style.color);
        if (dark) rules.push(dark);
        continue;
      }
      // @media / @supports / @layer 里也可能挂着规则，向下再看一层
      const nested = (rule as CSSGroupingRule).cssRules;
      if (nested) collect(nested);
    }
  };
  for (const sheet of Array.from(doc.styleSheets)) {
    let list: CSSRuleList | null = null;
    try {
      list = sheet.cssRules;
    } catch {
      continue; // 跨域样式表拿不到规则，跳过
    }
    collect(list);
  }
  if (rules.length === 0) return false;
  const style = doc.createElement("style");
  style.id = STYLE_ID;
  style.textContent = rules.join("");
  doc.head.appendChild(style);
  return true;
}
