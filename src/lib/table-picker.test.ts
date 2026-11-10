import { describe, expect, it } from "vitest";
import { filterTables, moveIndex, optionWindow } from "./table-picker";

const tables = ["biz_customer", "addr_geo_location", "biz_drivers", "Addr_Log", "orders"];

describe("filterTables", () => {
  it("sorts the full list case-insensitively when the keyword is empty", () => {
    expect(filterTables(tables, "")).toEqual([
      "addr_geo_location",
      "Addr_Log",
      "biz_customer",
      "biz_drivers",
      "orders",
    ]);
  });

  it("matches regardless of case and ranks prefix hits first", () => {
    expect(filterTables(tables, "addr")).toEqual(["addr_geo_location", "Addr_Log"]);
    expect(filterTables(tables, "GEO")).toEqual(["addr_geo_location"]);
  });

  it("returns nothing when no table matches", () => {
    expect(filterTables(tables, "missing")).toEqual([]);
  });

  it("caps the candidate list", () => {
    expect(filterTables(tables, "", 2)).toHaveLength(2);
  });
});

describe("optionWindow", () => {
  const many = Array.from({ length: 250 }, (_, index) => `db_${String(index).padStart(3, "0")}`);

  it("renders the first N and reports how many were folded away", () => {
    const { visible, hidden } = optionWindow(many, "", 200);
    expect(visible).toHaveLength(200);
    expect(hidden).toBe(50);
    expect(visible[0]).toBe("db_000");
  });

  it("reports nothing folded when the keyword narrows below the cap", () => {
    const { visible, hidden } = optionWindow(many, "db_01", 200);
    expect(visible.length).toBe(10);
    expect(hidden).toBe(0);
  });

  it("keeps prefix matches first so typed keywords land on top", () => {
    const { visible } = optionWindow(many, "db_249", 200);
    expect(visible).toEqual(["db_249"]);
  });
});

describe("moveIndex", () => {
  it("wraps around the candidate list", () => {
    expect(moveIndex(0, 1, 3)).toBe(1);
    expect(moveIndex(2, 1, 3)).toBe(0);
    expect(moveIndex(0, -1, 3)).toBe(2);
  });

  it("starts at the first or last entry when nothing is selected", () => {
    expect(moveIndex(-1, 1, 3)).toBe(0);
    expect(moveIndex(-1, -1, 3)).toBe(2);
  });

  it("returns -1 for an empty list", () => {
    expect(moveIndex(0, 1, 0)).toBe(-1);
  });
});
