import { describe, expect, it } from "vitest";
import { metaPairs, nodeText, rowTexts } from "./table-meta";

describe("nodeText", () => {
  it("renders null, undefined and blank values as a dash", () => {
    expect(nodeText(null)).toBe("-");
    expect(nodeText(undefined)).toBe("-");
    expect(nodeText("   ")).toBe("-");
  });

  it("keeps numbers and trims strings", () => {
    expect(nodeText(0)).toBe("0");
    expect(nodeText(20)).toBe("20");
    expect(nodeText("  20 ")).toBe("20");
  });
});

describe("metaPairs", () => {
  it("pairs meta columns with the single row and orders known fields", () => {
    const pairs = metaPairs({
      columns: ["table_comment", "table_name", "engine", "unknown_field"],
      rows: [["班组信息表", "biz_team", "InnoDB", "x"]],
    });
    expect(pairs.map((pair) => pair.key)).toEqual([
      "table_name",
      "engine",
      "table_comment",
      "unknown_field",
    ]);
    expect(pairs[0].value).toBe("biz_team");
    expect(pairs[2].value).toBe("班组信息表");
  });

  it("returns an empty list for an empty grid", () => {
    expect(metaPairs({ columns: [], rows: [] })).toEqual([]);
  });

  it("renders missing cells as dashes", () => {
    const pairs = metaPairs({
      columns: ["table_name", "auto_increment"],
      rows: [["biz_team"]],
    });
    expect(pairs[1].value).toBe("-");
  });
});

describe("rowTexts", () => {
  it("maps every column and pads missing cells", () => {
    expect(rowTexts({ columns: ["列名", "列类型", "列说明"], rows: [] }, ["id", "bigint"])).toEqual([
      "id",
      "bigint",
      "-",
    ]);
  });
});
