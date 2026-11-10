import { describe, expect, it } from "vitest";
import { darkRuleFor, isTokenSelector, normalizeColor } from "./sql-highlight";

describe("normalizeColor", () => {
  it("把三位 hex 补成六位", () => {
    expect(normalizeColor("#708")).toBe("#770088");
    expect(normalizeColor("#A11")).toBe("#aa1111");
  });

  it("把 rgb()/rgba() 统一成 hex", () => {
    expect(normalizeColor("rgb(119, 0, 136)")).toBe("#770088");
    expect(normalizeColor("rgba(170, 17, 17, 0.6)")).toBe("#aa1111");
    expect(normalizeColor("rgb(255 0 0 / 50%)")).toBe("#ff0000");
  });

  it("认不出的写法返回 null，不抛异常", () => {
    expect(normalizeColor("")).toBeNull();
    expect(normalizeColor("inherit")).toBeNull();
    expect(normalizeColor("currentColor")).toBeNull();
    expect(normalizeColor("rgb(1, 2)")).toBeNull();
  });
});

describe("isTokenSelector", () => {
  it("只认 CodeMirror 生成的单类选择器", () => {
    expect(isTokenSelector(".ͼb")).toBe(true);
    expect(isTokenSelector(".ͼ1a")).toBe(true);
  });

  it("复合选择器与普通类名都不算 token", () => {
    expect(isTokenSelector(".ͼ1 .cm-line")).toBe(false);
    expect(isTokenSelector(".cm-content")).toBe(false);
    expect(isTokenSelector("")).toBe(false);
  });
});

describe("darkRuleFor", () => {
  it("默认调色板里的 token 色重定向到深色变量", () => {
    expect(darkRuleFor(".ͼb", "#708")).toBe('[data-theme="dark"] .ͼb{color:var(--sql-keyword)}');
    expect(darkRuleFor(".ͼm", "rgb(153, 68, 0)")).toBe(
      '[data-theme="dark"] .ͼm{color:var(--sql-comment)}',
    );
  });

  it("未知颜色与非 token 选择器都不生成规则", () => {
    expect(darkRuleFor(".ͼb", "#123456")).toBeNull();
    expect(darkRuleFor(".cm-line", "#708")).toBeNull();
    expect(darkRuleFor(".ͼb", "")).toBeNull();
  });
});
