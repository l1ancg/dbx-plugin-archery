import { describe, expect, it } from "vitest";
import {
  allVisible,
  hiddenCount,
  matchColumns,
  pickColumns,
  toggleVisible,
  visibleIndexes,
} from "./column-picker";

const COLUMNS = ["id", "name_zh", "parent_id", "level"];
const TYPES = ["bigint", "varchar(64)", "bigint", "tinyint"];
const COMMENTS = ["主键", "中文名称", "父级ID", "层级(1-6)"];

const picked = (rows: ReturnType<typeof matchColumns>): string[] => rows.map((row) => row.name);

describe("allVisible", () => {
  it("starts with every column shown", () => {
    expect(allVisible(0)).toEqual([]);
    expect(allVisible(1)).toEqual([true]);
    expect(allVisible(3)).toEqual([true, true, true]);
  });
});

describe("toggleVisible", () => {
  it("hides and shows a column again", () => {
    expect(toggleVisible([true, true, true], 1)).toEqual([true, false, true]);
    expect(toggleVisible([true, false, true], 1)).toEqual([true, true, true]);
  });

  it("never hides the last visible column", () => {
    expect(toggleVisible([true], 0)).toEqual([true]);
    expect(toggleVisible([true, false], 0)).toEqual([true, false]);
    expect(toggleVisible([false, true], 1)).toEqual([false, true]);
  });

  it("ignores out-of-range indexes", () => {
    expect(toggleVisible([true, true], -1)).toEqual([true, true]);
    expect(toggleVisible([true, true], 2)).toEqual([true, true]);
    expect(toggleVisible([], 0)).toEqual([]);
  });
});

describe("visibleIndexes", () => {
  it("keeps the original column order", () => {
    expect(visibleIndexes([true, false, true])).toEqual([0, 2]);
    expect(visibleIndexes([false, false, true])).toEqual([2]);
    expect(visibleIndexes([true, true])).toEqual([0, 1]);
  });
});

describe("hiddenCount", () => {
  it("counts the columns turned off", () => {
    expect(hiddenCount([])).toBe(0);
    expect(hiddenCount([true, true])).toBe(0);
    expect(hiddenCount([true, false, false, true])).toBe(2);
  });
});

describe("pickColumns", () => {
  it("picks names and row cells by visible indexes", () => {
    const indexes = visibleIndexes([true, false, true]);
    expect(pickColumns(COLUMNS, indexes)).toEqual(["id", "parent_id"]);
    expect(pickColumns([1, "alpha", null, 3], indexes)).toEqual([1, null]);
  });

  it("returns an empty list without visible indexes", () => {
    expect(pickColumns(COLUMNS, [])).toEqual([]);
  });
});

describe("matchColumns", () => {
  it("returns every column in original order when the query is blank", () => {
    expect(matchColumns(COLUMNS, TYPES, "", "fuzzy")).toEqual([
      { index: 0, name: "id", note: "bigint" },
      { index: 1, name: "name_zh", note: "varchar(64)" },
      { index: 2, name: "parent_id", note: "bigint" },
      { index: 3, name: "level", note: "tinyint" },
    ]);
    expect(matchColumns(COLUMNS, TYPES, "   ", "exact")).toHaveLength(4);
  });

  it("matches column names by substring, ignoring case", () => {
    expect(picked(matchColumns(COLUMNS, TYPES, "NAME", "fuzzy"))).toEqual(["name_zh"]);
    expect(picked(matchColumns(COLUMNS, TYPES, "id", "fuzzy"))).toEqual(["id", "parent_id"]);
  });

  it("matches types by substring in fuzzy mode", () => {
    expect(picked(matchColumns(COLUMNS, COMMENTS, "名称", "fuzzy"))).toEqual(["name_zh"]);
    expect(picked(matchColumns(COLUMNS, COMMENTS, "父级", "fuzzy"))).toEqual(["parent_id"]);
    expect(picked(matchColumns(COLUMNS, COMMENTS, "varchar", "fuzzy"))).toEqual([]);
  });

  it("matches only the start of names and notes in prefix mode", () => {
    expect(picked(matchColumns(COLUMNS, TYPES, "par", "prefix"))).toEqual(["parent_id"]);
    expect(picked(matchColumns(COLUMNS, COMMENTS, "中文", "prefix"))).toEqual(["name_zh"]);
    expect(picked(matchColumns(COLUMNS, TYPES, "arent", "prefix"))).toEqual([]);
  });

  it("matches whole names and notes in exact mode", () => {
    expect(picked(matchColumns(COLUMNS, TYPES, "LEVEL", "exact"))).toEqual(["level"]);
    expect(picked(matchColumns(COLUMNS, TYPES, "lev", "exact"))).toEqual([]);
    expect(picked(matchColumns(COLUMNS, COMMENTS, "父级id", "exact"))).toEqual(["parent_id"]);
  });

  it("keeps original indexes and falls back to an empty note without types", () => {
    expect(matchColumns(COLUMNS, undefined, "level", "fuzzy")).toEqual([
      { index: 3, name: "level", note: "" },
    ]);
    expect(matchColumns(["id", "id"], ["", " "], "id", "fuzzy")).toEqual([
      { index: 0, name: "id", note: "" },
      { index: 1, name: "id", note: "" },
    ]);
  });

  it("returns an empty list when nothing matches", () => {
    expect(matchColumns(COLUMNS, TYPES, "nothing-here", "fuzzy")).toEqual([]);
  });
});
