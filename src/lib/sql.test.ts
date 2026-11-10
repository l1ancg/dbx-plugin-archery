import { describe, expect, it } from "vitest";
import {
  dialectFor,
  explainSql,
  formatSql,
  runSteps,
  splitStatements,
  sqlSummary,
  statementAt,
} from "./sql";

describe("splitStatements", () => {
  it("splits on semicolons and trims statement text", () => {
    const sql = "select 1;\nselect 2;";
    const statements = splitStatements(sql);
    expect(statements.map((item) => item.text)).toEqual(["select 1", "select 2"]);
    expect(statements[0]).toMatchObject({ start: 0, end: 8 });
    expect(statements[1]).toMatchObject({ start: 10, end: 18 });
  });

  it("keeps semicolons inside quoted strings and identifiers", () => {
    expect(splitStatements("select ';' as s; select 2").map((item) => item.text)).toEqual([
      "select ';' as s",
      "select 2",
    ]);
    expect(splitStatements("select 'it''s; ok' from t")).toHaveLength(1);
    expect(splitStatements('select "a;b" from t')).toHaveLength(1);
    expect(splitStatements("select `a;b` from t")).toHaveLength(1);
  });

  it("ignores semicolons inside comments", () => {
    const lineComment = splitStatements("-- note; still comment\nselect 1;");
    expect(lineComment).toHaveLength(1);
    expect(lineComment[0].text).toContain("-- note; still comment");

    const hashComment = splitStatements("select 1 # c; still\nfrom t");
    expect(hashComment).toHaveLength(1);
    expect(hashComment[0].text).toContain("# c; still");

    const blockComment = splitStatements("/* a;b */ select 1;");
    expect(blockComment).toHaveLength(1);
    expect(blockComment[0].text).toBe("/* a;b */ select 1");
  });

  it("accepts a trailing statement without semicolon", () => {
    expect(splitStatements("select 1").map((item) => item.text)).toEqual(["select 1"]);
  });

  it("drops empty statements", () => {
    expect(splitStatements("")).toEqual([]);
    expect(splitStatements("   \n\t ")).toEqual([]);
    expect(splitStatements(";;")).toEqual([]);
    expect(splitStatements("-- only comment")).toEqual([]);
    expect(splitStatements("/* only comment */")).toEqual([]);
  });
});

describe("statementAt", () => {
  const sql = "select 1;\n\nselect 2;";

  it("returns the statement under the cursor", () => {
    expect(statementAt(sql, 0)?.text).toBe("select 1");
    expect(statementAt(sql, 7)?.text).toBe("select 1");
    expect(statementAt(sql, 11)?.text).toBe("select 2");
  });

  it("falls back to the previous statement in the gap", () => {
    expect(statementAt(sql, 9)?.text).toBe("select 1");
    expect(statementAt(sql, 10)?.text).toBe("select 1");
    expect(statementAt(sql, sql.length)?.text).toBe("select 2");
  });

  it("treats a leading comment as part of its statement", () => {
    const commented = "-- c\nselect 1";
    expect(statementAt(commented, 1)?.text).toBe("-- c\nselect 1");
  });

  it("returns null when there is no statement", () => {
    expect(statementAt("", 0)).toBeNull();
    expect(statementAt("   ", 2)).toBeNull();
    expect(statementAt(";;", 1)).toBeNull();
  });
});

describe("runSteps", () => {
  const sql = "select 1;\nselect 2;\nselect 3;";

  it("runs every statement of a multi-statement selection in order", () => {
    const steps = runSteps(sql, "select 1;\nselect 2;", 0);
    expect(steps.map((step) => step.text)).toEqual(["select 1", "select 2"]);
    expect(steps.map((step) => `${step.index}/${step.total}`)).toEqual(["1/2", "2/2"]);
  });

  it("strips the trailing semicolon of a single-statement selection", () => {
    expect(runSteps(sql, "select 2;", 0)).toEqual([{ text: "select 2", index: 1, total: 1 }]);
  });

  it("keeps the script position of the statement at the cursor", () => {
    const steps = runSteps(sql, "", sql.indexOf("select 2"));
    expect(steps.map((step) => step.text)).toEqual(["select 2"]);
    expect(steps[0]).toMatchObject({ index: 2, total: 3 });
  });

  it("runs the whole script when neither selection nor cursor is available", () => {
    expect(runSteps(sql, "", undefined).map((step) => step.text)).toEqual([
      "select 1",
      "select 2",
      "select 3",
    ]);
  });

  it("returns nothing when the selection holds no statement", () => {
    expect(runSteps(sql, "-- just a note", 0)).toEqual([]);
    expect(runSteps("", "", 0)).toEqual([]);
  });
});

describe("dialectFor", () => {
  it("maps Archery db types to formatter dialects", () => {
    for (const dbType of ["mysql", "MySQL", "doris", "tidb", "mariadb"]) {
      expect(dialectFor(dbType)).toBe("mysql");
    }
    for (const dbType of ["postgresql", "PG", "postgres"]) {
      expect(dialectFor(dbType)).toBe("postgresql");
    }
    expect(dialectFor("sqlserver")).toBe("sqlserver");
    expect(dialectFor("mssql")).toBe("sqlserver");
    expect(dialectFor("oracle")).toBe("plsql");
    expect(dialectFor("hive")).toBe("hive");
  });

  it("returns null for unsupported types", () => {
    expect(dialectFor("redis")).toBeNull();
    expect(dialectFor("mongodb")).toBeNull();
    expect(dialectFor("")).toBeNull();
    expect(dialectFor("  ")).toBeNull();
  });
});

describe("formatSql", () => {
  it("uppercases keywords and breaks clauses", () => {
    const formatted = formatSql("select id,name from orders where id=1", "mysql");
    expect(formatted.startsWith("SELECT")).toBe(true);
    expect(formatted).toContain("\nFROM\n");
    expect(formatted).not.toContain("select");
  });

  it("uses the postgres dialect when asked", () => {
    const formatted = formatSql("select now()", "postgresql");
    expect(formatted.startsWith("SELECT")).toBe(true);
  });

  it("rejects unsupported dialects", () => {
    expect(() => formatSql("select 1", "redis")).toThrow(/unsupported/i);
  });
});

describe("explainSql", () => {
  it("prefixes supported dialects", () => {
    expect(explainSql("select 1", "mysql")).toBe("EXPLAIN select 1");
    expect(explainSql("  select 1\nfrom t  ", "postgresql")).toBe("EXPLAIN select 1\nfrom t");
    expect(explainSql("select 1", "doris")).toBe("EXPLAIN select 1");
  });

  it("does not double prefix an existing EXPLAIN", () => {
    expect(explainSql("explain select 1", "mysql")).toBe("explain select 1");
    expect(explainSql("EXPLAIN ANALYZE select 1", "mysql")).toBe("EXPLAIN ANALYZE select 1");
  });

  it("returns null for unsupported dialects and empty SQL", () => {
    expect(explainSql("select 1", "redis")).toBeNull();
    expect(explainSql("  ", "mysql")).toBeNull();
  });
});

describe("sqlSummary", () => {
  it("collapses whitespace into one line", () => {
    expect(sqlSummary("select id,\n  name\nfrom orders")).toBe("select id, name from orders");
  });

  it("truncates long SQL with an ellipsis", () => {
    const summary = sqlSummary(`select ${"a".repeat(100)}`, 20);
    expect(summary).toHaveLength(20);
    expect(summary.endsWith("…")).toBe(true);
  });

  it("keeps SQL that just fits", () => {
    expect(sqlSummary("select 1", 8)).toBe("select 1");
  });
});
