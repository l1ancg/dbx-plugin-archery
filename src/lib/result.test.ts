import { describe, expect, it } from "vitest";
import type { Cell } from "../types";
import {
  defaultPageSize,
  escapeDelimited,
  formatCell,
  formatElapsed,
  formatNumber,
  highlightSegments,
  matchRowIndexes,
  pageSizeChoices,
  paginate,
  toCsv,
  toMarkdown,
  toTsv,
  truncate,
} from "./result";

describe("formatCell", () => {
  it("renders null as NULL and keeps zero", () => {
    expect(formatCell(null)).toBe("NULL");
    expect(formatCell(undefined)).toBe("NULL");
    expect(formatCell(0)).toBe("0");
    expect(formatCell(false)).toBe("false");
  });
});

describe("escaped export", () => {
  it("quotes delimiters, quotes and newlines", () => {
    expect(escapeDelimited("a,b", ",")).toBe('"a,b"');
    expect(escapeDelimited('say "hi"', ",")).toBe('"say ""hi"""');
    expect(escapeDelimited("line1\nline2", ",")).toBe('"line1\nline2"');
    expect(escapeDelimited(null, ",")).toBe("");
  });

  it("builds csv with header in column order", () => {
    const csv = toCsv(["id", "name"], [
      [1, "alpha,beta"],
      [2, null],
    ]);
    expect(csv).toBe('id,name\r\n1,"alpha,beta"\r\n2,');
  });

  it("builds tsv without quoting plain values", () => {
    expect(toTsv(["id", "name"], [[1, "alpha"]])).toBe("id\tname\r\n1\talpha");
  });

  it("escapes pipes and newlines for markdown", () => {
    const markdown = toMarkdown(["a|b", "c"], [["x|y", null]]);
    expect(markdown.split("\n")[0]).toBe("| a\\|b | c |");
    expect(markdown.split("\n")[2]).toBe("| x\\|y | NULL |");
  });
});

describe("pagination", () => {
  const rows = Array.from({ length: 95 }, (_, index) => index);

  it("clamps page and reports page count", () => {
    const first = paginate(rows, 1, 50);
    expect(first.items).toHaveLength(50);
    expect(first.pageCount).toBe(2);

    const last = paginate(rows, 9, 50);
    expect(last.page).toBe(2);
    expect(last.items).toHaveLength(45);
  });

  it("handles empty result", () => {
    const empty = paginate([], 3, 50);
    expect(empty.page).toBe(1);
    expect(empty.pageCount).toBe(1);
    expect(empty.items).toHaveLength(0);
  });
});

describe("page size choices", () => {
  it("caps the options at the query row limit", () => {
    expect(pageSizeChoices(100)).toEqual([30, 50, 100]);
    expect(pageSizeChoices(500)).toEqual([30, 50, 100, 200, 500]);
    expect(pageSizeChoices(1000)).toEqual([30, 50, 100, 200, 500, 1000]);
  });

  it("keeps a custom row limit as its own step", () => {
    expect(pageSizeChoices(300)).toEqual([30, 50, 100, 200, 300]);
    expect(pageSizeChoices(20)).toEqual([20]);
    expect(pageSizeChoices(25)).toEqual([25]);
  });

  it("offers every preset when the row limit is unlimited", () => {
    expect(pageSizeChoices(-1)).toEqual([30, 50, 100, 200, 500, 1000]);
    expect(pageSizeChoices(0)).toEqual([30, 50, 100, 200, 500, 1000]);
  });

  it("defaults to 30 rows and falls back to the closest fit", () => {
    expect(defaultPageSize(1000)).toBe(30);
    expect(defaultPageSize(100)).toBe(30);
    expect(defaultPageSize(50)).toBe(30);
    expect(defaultPageSize(30)).toBe(30);
    expect(defaultPageSize(20)).toBe(20);
    expect(defaultPageSize(-1)).toBe(30);
  });
});

describe("number formatting", () => {
  it("groups thousands", () => {
    expect(formatNumber(1234567)).toBe("1,234,567");
    expect(formatNumber(-1234)).toBe("-1,234");
    expect(formatNumber(0)).toBe("0");
  });

  it("formats elapsed time", () => {
    expect(formatElapsed(31)).toBe("31 ms");
    expect(formatElapsed(1240)).toBe("1.24 s");
    expect(formatElapsed(-1)).toBe("0 ms");
  });

  it("truncates long text", () => {
    expect(truncate("abcdef", 4)).toBe("abc…");
    expect(truncate("abc", 10)).toBe("abc");
  });
});

describe("result search", () => {
  const rows: Cell[][] = [
    [1, "Alice", "paid"],
    [2, "bob", null],
    [3, "ALICE", "refunded"],
  ];

  it("matches substrings case-insensitively in the given columns", () => {
    expect(matchRowIndexes(rows, [0, 1, 2], "alice")).toEqual([0, 2]);
    expect(matchRowIndexes(rows, [2], "alice")).toEqual([]);
    expect(matchRowIndexes(rows, [0], "2")).toEqual([1]);
    expect(matchRowIndexes(rows, [2], " refund ")).toEqual([2]);
  });

  it("searches the rendered text, so NULL is findable", () => {
    expect(matchRowIndexes(rows, [2], "null")).toEqual([1]);
  });

  it("keeps every row when the query is blank", () => {
    expect(matchRowIndexes(rows, [0], "   ")).toEqual([0, 1, 2]);
  });

  it("splits text into matched and plain segments", () => {
    expect(highlightSegments("Alice and ALICE", "alice")).toEqual([
      { text: "Alice", match: true },
      { text: " and ", match: false },
      { text: "ALICE", match: true },
    ]);
  });

  it("leaves text whole without a hit and skips empty text", () => {
    expect(highlightSegments("abc", " ")).toEqual([{ text: "abc", match: false }]);
    expect(highlightSegments("abc", "z")).toEqual([{ text: "abc", match: false }]);
    expect(highlightSegments("", "z")).toEqual([]);
    expect(highlightSegments("aaa", "aa")).toEqual([
      { text: "aa", match: true },
      { text: "a", match: false },
    ]);
  });
});
