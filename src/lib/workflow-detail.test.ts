import { describe, expect, it } from "vitest";
import type { PrivilegeReviewNode, SqlWorkflowRow, WorkflowGridRow } from "../types";
import {
  buildCommitSql,
  buildOtherInstancePrefill,
  buildRollbackFile,
  buildSubmitPrefill,
  canRollbackWorkflow,
  ensureOptionValue,
  filterRollbackRows,
  formatSqlPreview,
  infoFieldLabelKey,
  matchOptionIdByName,
  normalizeInfoFields,
  normalizeRollbackRows,
  normalizeRunDateChange,
  normalizeWorkflowGrid,
  parseRunDateRange,
  readRunDateRange,
  resolveReviewNodes,
  resolveRollbackTarget,
  stripDatabaseQualifier,
} from "./workflow-detail";

describe("normalizeInfoFields", () => {
  it("keeps the order rendered by Archery", () => {
    const fields = normalizeInfoFields([
      { label: "发起人", value: "廖思源" },
      { label: "可执行时间范围", value: "无限制" },
      { label: "类型", value: "DDL" },
    ]);
    expect(fields).toEqual([
      { label: "发起人", value: "廖思源" },
      { label: "可执行时间范围", value: "无限制" },
      { label: "类型", value: "DDL" },
    ]);
  });

  it("accepts snake_case and trims values", () => {
    const fields = normalizeInfoFields([{ label: " 目标实例 ", value: " test-express-rw " }]);
    expect(fields).toEqual([{ label: "目标实例", value: "test-express-rw" }]);
  });

  it("drops blank rows and non arrays", () => {
    expect(normalizeInfoFields(null)).toEqual([]);
    expect(normalizeInfoFields([null, { label: "", value: "x" }, { label: "组", value: "" }])).toEqual([]);
  });
});

describe("infoFieldLabelKey", () => {
  it("maps Archery column labels to i18n keys", () => {
    expect(infoFieldLabelKey("发起人")).toBe("workflow.colApplicant");
    expect(infoFieldLabelKey("目标实例")).toBe("workflow.colInstance");
    expect(infoFieldLabelKey("数据库")).toBe("workflow.colDatabase");
    expect(infoFieldLabelKey("发起时间")).toBe("workflow.colCreateTime");
    expect(infoFieldLabelKey("可执行时间范围")).toBe("workflow.colRunDate");
    expect(infoFieldLabelKey("结束时间")).toBe("workflow.colFinishTime");
    expect(infoFieldLabelKey("备份")).toBe("workflow.colBackup");
    expect(infoFieldLabelKey("当前状态")).toBe("workflow.colCurrentStatus");
    expect(infoFieldLabelKey("组")).toBe("workflow.colGroup");
    expect(infoFieldLabelKey("类型")).toBe("workflow.colType");
  });

  it("returns an empty key for unknown labels so the raw text is shown", () => {
    expect(infoFieldLabelKey("需求链接")).toBe("");
  });
});

describe("canRollbackWorkflow", () => {
  it("only offers rollback for finished or failed tickets with backup", () => {
    expect(canRollbackWorkflow("workflow_finish", true)).toBe(true);
    expect(canRollbackWorkflow("workflow_exception", true)).toBe(true);
  });

  it("hides rollback when there is no backup", () => {
    expect(canRollbackWorkflow("workflow_finish", false)).toBe(false);
  });

  it("hides rollback before the ticket ran", () => {
    expect(canRollbackWorkflow("workflow_manreviewing", true)).toBe(false);
    expect(canRollbackWorkflow("workflow_review_pass", true)).toBe(false);
    expect(canRollbackWorkflow("workflow_abort", true)).toBe(false);
  });
});

describe("normalizeWorkflowGrid", () => {
  it("maps the detail_content row used by the SQL grid", () => {
    const rows = normalizeWorkflowGrid([
      {
        id: 2,
        sql: "alter table bs_vehicle_info modify column axle_number int",
        errlevel: 1,
        errormessage: "Type conversion warning",
        affected_rows: 3136,
        execute_time: "0.127",
        backup_time: "0",
        stagestatus: "Execute Successfully",
      },
    ]);
    expect(rows).toEqual([
      {
        id: 2,
        sql: "alter table bs_vehicle_info modify column axle_number int",
        errorLevel: 1,
        errorMessage: "Type conversion warning",
        affectedRows: 3136,
        executeTime: "0.127",
        backupTime: "0",
        stage: "Execute Successfully",
      },
    ]);
  });

  it("falls back to a running index when id is missing", () => {
    const rows = normalizeWorkflowGrid([
      { sql: "a", errlevel: 0 },
      { sql: "b", errlevel: 2 },
    ]);
    expect(rows.map((row) => row.id)).toEqual([1, 2]);
    expect(rows[1].errorLevel).toBe(2);
    expect(rows[0].backupTime).toBe("");
  });

  it("returns an empty list for missing payloads", () => {
    expect(normalizeWorkflowGrid(null)).toEqual([]);
    expect(normalizeWorkflowGrid([null])).toEqual([]);
  });
});

describe("normalizeRollbackRows", () => {
  it("maps the [executed, rollback] pairs returned by Archery", () => {
    expect(
      normalizeRollbackRows([
        ["CREATE TABLE `t` (`id` int)", "DROP TABLE `express_dev`.`t`;"],
        ["alter table v add column a int", "alter table v drop column a"],
      ]),
    ).toEqual([
      {
        id: 1,
        sql: "CREATE TABLE `t` (`id` int)",
        rollbackSql: "DROP TABLE `express_dev`.`t`;",
      },
      {
        id: 2,
        sql: "alter table v add column a int",
        rollbackSql: "alter table v drop column a",
      },
    ]);
  });

  it("accepts objects and keeps the id Archery sends", () => {
    expect(
      normalizeRollbackRows([
        { sql: "update t set a = 1", rollback_sql: "update t set a = 0" },
        { id: 7, sql: "delete from t", rollbackSql: "insert into t values (1)" },
      ]),
    ).toEqual([
      { id: 1, sql: "update t set a = 1", rollbackSql: "update t set a = 0" },
      { id: 7, sql: "delete from t", rollbackSql: "insert into t values (1)" },
    ]);
  });

  it("keeps rollback-only strings and drops blank rows", () => {
    expect(normalizeRollbackRows(["drop table legacy", null, "  "])).toEqual([
      { id: 1, sql: "", rollbackSql: "drop table legacy" },
    ]);
    expect(normalizeRollbackRows(null)).toEqual([]);
  });

  // 回滚语句会带着 goInception 补的 `原工单库名`.`表` 前缀；新工单已在自己的库上执行，
  // 前缀既冗余又会在换库/换实例时打错库，归一化时就按工单库名剥掉（执行语句保持原样）。
  it("strips the original database prefix from rollback statements only", () => {
    expect(
      normalizeRollbackRows(
        [
          [
            "CREATE TABLE `bs_vehicle_info` (`id` int)",
            "DROP TABLE `schedule-dev`.`bs_vehicle_info`;",
          ],
          [
            "delete from `schedule-dev`.`bs_vehicle_info` where id = 7",
            "insert into schedule-dev.bs_vehicle_info (id) values (7)",
          ],
        ],
        "schedule-dev",
      ),
    ).toEqual([
      {
        id: 1,
        sql: "CREATE TABLE `bs_vehicle_info` (`id` int)",
        rollbackSql: "DROP TABLE `bs_vehicle_info`;",
      },
      {
        id: 2,
        sql: "delete from `schedule-dev`.`bs_vehicle_info` where id = 7",
        rollbackSql: "insert into bs_vehicle_info (id) values (7)",
      },
    ]);
  });

  it("keeps the statements untouched when the ticket has no database", () => {
    expect(
      normalizeRollbackRows([["delete from t", "insert into `schedule-dev`.`t` values (1)"]], " "),
    ).toEqual([
      { id: 1, sql: "delete from t", rollbackSql: "insert into `schedule-dev`.`t` values (1)" },
    ]);
  });
});

describe("stripDatabaseQualifier", () => {
  it("drops the ticket database prefix in both quoting styles", () => {
    expect(stripDatabaseQualifier("DROP TABLE `schedule-dev`.`t`;", "schedule-dev")).toBe(
      "DROP TABLE `t`;",
    );
    expect(
      stripDatabaseQualifier("insert into schedule-dev.t (id) values (1)", "schedule-dev"),
    ).toBe("insert into t (id) values (1)");
  });

  it("ignores case and spaces around the dot", () => {
    expect(
      stripDatabaseQualifier("ALTER TABLE `SCHEDULE-DEV` . `t` ADD COLUMN c int", "schedule-dev"),
    ).toBe("ALTER TABLE `t` ADD COLUMN c int");
    expect(
      stripDatabaseQualifier("update `Schedule-Dev`.t set a = 1", "schedule-dev"),
    ).toBe("update t set a = 1");
  });

  it("keeps other schemas and quoted literals", () => {
    const otherSchema = "insert into other.t select * from schedule-dev2.t";
    expect(stripDatabaseQualifier(otherSchema, "schedule-dev")).toBe(otherSchema);
    const quoted = "insert into t values ('schedule-dev.x', \"schedule-dev.y\")";
    expect(stripDatabaseQualifier(quoted, "schedule-dev")).toBe(quoted);
  });

  it("does nothing without a database name", () => {
    expect(stripDatabaseQualifier("drop table `schedule-dev`.`t`", "  ")).toBe(
      "drop table `schedule-dev`.`t`",
    );
  });
});

describe("filterRollbackRows", () => {
  const rows = [
    { id: 1, sql: "create table t (id int)", rollbackSql: "drop table `db`.`t`" },
    { id: 2, sql: "alter table v add column a int", rollbackSql: "alter table v drop column a" },
  ];

  it("matches either the executed or the rollback statement", () => {
    expect(filterRollbackRows(rows, "create").map((row) => row.id)).toEqual([1]);
    expect(filterRollbackRows(rows, "DROP COLUMN").map((row) => row.id)).toEqual([2]);
    expect(filterRollbackRows(rows, "`t`").map((row) => row.id)).toEqual([1]);
  });

  it("returns every row for a blank keyword", () => {
    expect(filterRollbackRows(rows, "   ")).toEqual(rows);
  });
});

describe("buildRollbackFile", () => {
  it("writes each pair as an executed-SQL comment followed by the rollback statement", () => {
    expect(
      buildRollbackFile([
        { id: 1, sql: "create table t (id int)", rollbackSql: "drop table `db`.`t`" },
        {
          id: 2,
          sql: "alter table v add column a int",
          rollbackSql: "alter table v drop column a",
        },
      ]),
    ).toBe(
      "/*create table t (id int)*/\ndrop table `db`.`t`\n" +
        "/*alter table v add column a int*/\nalter table v drop column a\n",
    );
  });

  it("returns an empty string for no rows", () => {
    expect(buildRollbackFile([])).toBe("");
  });

  // 真实工单样本（rollback_4862.sql）：执行语句跨行、回滚语句自带分号、无 BOM、结尾单个换行。
  it("reproduces the rollback_4862.sql sample byte for byte", () => {
    expect(
      buildRollbackFile([
        {
          id: 1,
          sql: "alter table\n  bs_vehicle_info\nmodify\n  column `axle_number` int DEFAULT NULL COMMENT '轴数'",
          rollbackSql:
            "ALTER TABLE `schedule-dev`.`bs_vehicle_info` MODIFY COLUMN `axle_number` varchar(10) COMMENT '轴数，字典维护(bs_axle_number)';",
        },
      ]),
    ).toBe(
      "/*alter table\n  bs_vehicle_info\nmodify\n  column `axle_number` int DEFAULT NULL COMMENT '轴数'*/\n" +
        "ALTER TABLE `schedule-dev`.`bs_vehicle_info` MODIFY COLUMN `axle_number` varchar(10) COMMENT '轴数，字典维护(bs_axle_number)';\n",
    );
  });
});

describe("parseRunDateRange", () => {
  it("treats Archery's unlimited windows as an empty range", () => {
    expect(parseRunDateRange("无限制")).toEqual({ start: "", end: "" });
    expect(parseRunDateRange("不限")).toEqual({ start: "", end: "" });
    expect(parseRunDateRange("")).toEqual({ start: "", end: "" });
    expect(parseRunDateRange("-")).toEqual({ start: "", end: "" });
  });

  it("keeps minute precision from the start ~ end text", () => {
    expect(parseRunDateRange("2026-11-02 12:00:00 ~ 2026-11-02 14:00:00")).toEqual({
      start: "2026-11-02 12:00",
      end: "2026-11-02 14:00",
    });
  });

  it("accepts dash separators, T and single-digit hours", () => {
    expect(parseRunDateRange("2026-11-02T9:00:00-2026-11-02T11:30:00")).toEqual({
      start: "2026-11-02 09:00",
      end: "2026-11-02 11:30",
    });
  });

  it("drops half-filled windows so the form stays unlimited", () => {
    expect(parseRunDateRange("2026-11-02 12:00:00")).toEqual({ start: "", end: "" });
  });
});

describe("readRunDateRange", () => {
  it("reads the current window from the detail fields", () => {
    expect(
      readRunDateRange([
        { label: "可执行时间范围", value: "2026-11-02 12:00:00 / 2026-11-02 14:00:00" },
      ]),
    ).toEqual({ start: "2026-11-02 12:00", end: "2026-11-02 14:00" });
  });

  it("returns an empty window for 无限制 or a missing field", () => {
    expect(readRunDateRange([{ label: "可执行时间范围", value: "无限制" }])).toEqual({
      start: "",
      end: "",
    });
    expect(readRunDateRange([])).toEqual({ start: "", end: "" });
  });
});

describe("normalizeRunDateChange", () => {
  it("treats an empty window as unlimited", () => {
    expect(normalizeRunDateChange("", "")).toEqual({ start: "", end: "", error: "" });
    expect(normalizeRunDateChange("   ", "  ")).toEqual({ start: "", end: "", error: "" });
  });

  it("converts datetime-local values to Archery's minute format", () => {
    expect(normalizeRunDateChange("2026-11-06T09:00", "2026-11-06T18:00")).toEqual({
      start: "2026-11-06 09:00",
      end: "2026-11-06 18:00",
      error: "",
    });
  });

  it("truncates seconds", () => {
    expect(normalizeRunDateChange("2026-11-06T09:00:30", "2026-11-06T18:00:00")).toEqual({
      start: "2026-11-06 09:00",
      end: "2026-11-06 18:00",
      error: "",
    });
  });

  it("requires both ends when only one side is filled", () => {
    expect(normalizeRunDateChange("2026-11-06T09:00", "").error).toBe("partial");
    expect(normalizeRunDateChange("", "2026-11-06T18:00").error).toBe("partial");
  });

  it("rejects an inverted window", () => {
    expect(normalizeRunDateChange("2026-11-06T18:00", "2026-11-06T09:00").error).toBe("order");
  });
});

describe("buildCommitSql", () => {
  it("joins statements with one trailing semicolon each", () => {
    expect(
      buildCommitSql([
        { ...gridRow(), sql: "alter table bs_vehicle_info add column axle_number int" },
        { ...gridRow(), sql: "update bs_vehicle_info set axle_number = 2;" },
      ]),
    ).toBe(
      "alter table bs_vehicle_info add column axle_number int;\n" +
        "update bs_vehicle_info set axle_number = 2;",
    );
  });

  it("skips blank rows", () => {
    expect(buildCommitSql([{ ...gridRow(), sql: "   " }, { ...gridRow(), sql: "select 1" }])).toBe(
      "select 1;",
    );
    expect(buildCommitSql([])).toBe("");
  });

  // detail_content 里引擎会塞一条「use 库名」，带到新工单只会改错库上下文。
  it("drops USE statements that switch the current database", () => {
    expect(
      buildCommitSql([
        { ...gridRow(), sql: "use `schedule-dev`" },
        { ...gridRow(), sql: " USE schedule-dev;" },
        { ...gridRow(), sql: "use\n  `express_dev`;" },
        { ...gridRow(), sql: "alter table bs_vehicle_info add column axle_number int" },
      ]),
    ).toBe("alter table bs_vehicle_info add column axle_number int;");
  });

  it("keeps statements that merely mention a database", () => {
    const sql =
      "CREATE TABLE `schedule-dev`.`use_log` (`id` int) COMMENT '切换库记录'";
    expect(buildCommitSql([{ ...gridRow(), sql }])).toBe(`${sql};`);
  });
});

describe("matchOptionIdByName", () => {
  const options = [
    { id: 3, name: "Test-Express" },
    { id: 7, name: "Test-Materials" },
  ];

  it("matches trimmed names ignoring case", () => {
    expect(matchOptionIdByName(options, " test-express ")).toBe(3);
    expect(matchOptionIdByName(options, "TEST-MATERIALS")).toBe(7);
  });

  it("returns 0 when the name is unknown or blank", () => {
    expect(matchOptionIdByName(options, "Other")).toBe(0);
    expect(matchOptionIdByName(options, "")).toBe(0);
    expect(matchOptionIdByName([], "Test-Express")).toBe(0);
  });
});

describe("ensureOptionValue", () => {
  it("prepends an unknown value so the select can still show it", () => {
    expect(ensureOptionValue(["demo_schedule", "demo_shop"], "schedule-dev")).toEqual([
      "schedule-dev",
      "demo_schedule",
      "demo_shop",
    ]);
  });

  it("keeps the list when the value already exists or is blank", () => {
    expect(ensureOptionValue(["demo_schedule"], "DEMO_SCHEDULE")).toEqual(["demo_schedule"]);
    expect(ensureOptionValue(["demo_schedule"], "  ")).toEqual(["demo_schedule"]);
  });
});

describe("buildSubmitPrefill", () => {
  it("rebuilds the submit form from the ticket detail", () => {
    expect(
      buildSubmitPrefill({
        row: workflowRow(),
        title: "DDL - 车辆表结构修改",
        infoFields: [
          { label: "目标实例", value: "test-express-rw" },
          { label: "数据库", value: "schedule-dev" },
          { label: "可执行时间范围", value: "2026-11-02 12:00:00 ~ 2026-11-02 14:00:00" },
          { label: "备份", value: "是" },
          { label: "组", value: "Test-Express" },
        ],
        rows: [
          { ...gridRow(), sql: "alter table bs_vehicle_info add column axle_number int" },
          { ...gridRow(), sql: "update bs_vehicle_info set axle_number = 2;" },
        ],
      }),
    ).toEqual({
      mode: "commit",
      workflowId: 5338,
      title: "DDL - 车辆表结构修改",
      groupName: "Test-Express",
      instanceName: "test-express-rw",
      dbName: "schedule-dev",
      sql: "alter table bs_vehicle_info add column axle_number int;\nupdate bs_vehicle_info set axle_number = 2;",
      backup: true,
      runDateStart: "2026-11-02 12:00",
      runDateEnd: "2026-11-02 14:00",
    });
  });

  it("falls back to the list row when the detail page has no info fields", () => {
    const prefill = buildSubmitPrefill({
      row: { ...workflowRow(), isBackup: false },
      title: "",
      infoFields: [],
      rows: [],
    });
    expect(prefill).toMatchObject({
      workflowId: 5338,
      title: "DDL - 车辆表结构修改",
      groupName: "Test-Express",
      instanceName: "test-express-rw",
      dbName: "schedule-dev",
      backup: false,
      runDateStart: "",
      runDateEnd: "",
      sql: "",
    });
  });

  it("reads the backup flag from the info field when present", () => {
    const prefill = buildSubmitPrefill({
      row: { ...workflowRow(), isBackup: true },
      title: "",
      infoFields: [{ label: "备份", value: "否" }],
      rows: [],
    });
    expect(prefill.backup).toBe(false);
  });
});

describe("buildOtherInstancePrefill", () => {
  it("keeps everything except the target instance and database", () => {
    expect(
      buildOtherInstancePrefill({
        row: workflowRow(),
        title: "DDL - 车辆表结构修改",
        infoFields: [
          { label: "目标实例", value: "test-express-rw" },
          { label: "数据库", value: "schedule-dev" },
          { label: "可执行时间范围", value: "2026-11-02 12:00:00 ~ 2026-11-02 14:00:00" },
          { label: "备份", value: "是" },
          { label: "组", value: "Test-Express" },
        ],
        rows: [{ ...gridRow(), sql: "alter table bs_vehicle_info add column axle_number int" }],
      }),
    ).toEqual({
      mode: "other",
      workflowId: 5338,
      title: "DDL - 车辆表结构修改",
      groupName: "Test-Express",
      instanceName: "",
      dbName: "",
      sql: "alter table bs_vehicle_info add column axle_number int;",
      backup: true,
      runDateStart: "2026-11-02 12:00",
      runDateEnd: "2026-11-02 14:00",
    });
  });

  it("still falls back to the list row for the other fields", () => {
    expect(
      buildOtherInstancePrefill({
        row: { ...workflowRow(), isBackup: false },
        infoFields: [],
        rows: [],
      }),
    ).toMatchObject({
      mode: "other",
      title: "DDL - 车辆表结构修改",
      groupName: "Test-Express",
      instanceName: "",
      dbName: "",
      backup: false,
      sql: "",
    });
  });
});

describe("formatSqlPreview", () => {
  it("flattens the multi-line DDL that goInception rewrites", () => {
    expect(
      formatSqlPreview(
        "alter table\n  bs_vehicle_info\nmodify\n  column `axle_number` int DEFAULT NULL COMMENT '轴数'",
      ),
    ).toBe(
      "alter table bs_vehicle_info modify column `axle_number` int DEFAULT NULL COMMENT '轴数'",
    );
  });

  it("keeps whitespace inside string literals", () => {
    expect(formatSqlPreview("insert into t (a) values ('a  b')")).toBe(
      "insert into t (a) values ('a  b')",
    );
  });

  it("keeps escaped quotes inside literals", () => {
    expect(formatSqlPreview("update t set c = 'it\\'s  ok'")).toBe("update t set c = 'it\\'s  ok'");
  });

  it("trims the outer whitespace and blank lines", () => {
    expect(formatSqlPreview("\n\n  delete from t where id = 7 \n")).toBe(
      "delete from t where id = 7",
    );
  });

  it("returns an empty string for blank input", () => {
    expect(formatSqlPreview("   \n ")).toBe("");
  });
});

describe("resolveRollbackTarget", () => {
  const groups = [
    { id: 3, name: "Test-Express" },
    { id: 7, name: "Test-Materials" },
  ];
  const instances = [
    { id: 12, name: "test-express-rw" },
    { id: 13, name: "materials_test" },
  ];

  it("resolves the original group and instance and keeps the original database", () => {
    expect(
      resolveRollbackTarget({ row: workflowRow(), groups, instances }),
    ).toEqual({ groupId: 3, instanceId: 12, dbName: "schedule-dev", missing: "" });
  });

  it("ignores case and surrounding spaces when matching names", () => {
    const target = resolveRollbackTarget({
      row: { ...workflowRow(), groupName: " test-express ", instanceName: "TEST-EXPRESS-RW" },
      groups,
      instances,
    });
    expect(target).toMatchObject({ groupId: 3, instanceId: 12, missing: "" });
  });

  it("never falls back to another group when the original group is gone", () => {
    const target = resolveRollbackTarget({
      row: { ...workflowRow(), groupName: "Test-Legacy" },
      groups,
      instances,
    });
    expect(target).toEqual({ groupId: 0, instanceId: 0, dbName: "schedule-dev", missing: "group" });
  });

  it("never falls back to another instance when the original instance is gone", () => {
    const target = resolveRollbackTarget({
      row: { ...workflowRow(), instanceName: "test-express-ro" },
      groups,
      instances,
    });
    expect(target).toEqual({ groupId: 3, instanceId: 0, dbName: "schedule-dev", missing: "instance" });
  });

  it("reports a missing database when the original ticket has none", () => {
    const target = resolveRollbackTarget({
      row: { ...workflowRow(), dbName: " " },
      groups,
      instances,
    });
    expect(target).toEqual({ groupId: 3, instanceId: 12, dbName: "", missing: "database" });
  });

  it("reports the group first when the original ticket has no target at all", () => {
    const target = resolveRollbackTarget({
      row: { ...workflowRow(), groupName: "", instanceName: "", dbName: "" },
      groups,
      instances,
    });
    expect(target).toEqual({ groupId: 0, instanceId: 0, dbName: "", missing: "group" });
  });
});

describe("resolveReviewNodes", () => {
  const node = (name: string, state: PrivilegeReviewNode["state"]): PrivilegeReviewNode => ({
    name,
    state,
    reviewers: [],
  });

  it("marks every node passed when a finished workflow lost its node states", () => {
    const nodes = [node("TEST-Leader-Express", "pending"), node("DBA", "pending")];
    expect(resolveReviewNodes(nodes, "workflow_finish").map((item) => item.state)).toEqual([
      "passed",
      "passed",
    ]);
  });

  it("also fills in an exceptionally finished workflow", () => {
    expect(resolveReviewNodes([node("DBA", "pending")], "workflow_exception")).toEqual([
      { name: "DBA", state: "passed", reviewers: [] },
    ]);
  });

  it("keeps whatever progress the detail page did parse", () => {
    const nodes = [node("A", "passed"), node("B", "current")];
    expect(resolveReviewNodes(nodes, "workflow_finish")).toBe(nodes);
    expect(resolveReviewNodes([node("A", "auto")], "workflow_finish")[0].state).toBe("auto");
  });

  it("never invents progress for a workflow that is still running", () => {
    const nodes = [node("A", "pending"), node("B", "pending")];
    expect(resolveReviewNodes(nodes, "workflow_manreviewing")).toBe(nodes);
    expect(resolveReviewNodes(nodes, "workflow_abort")).toBe(nodes);
  });

  it("returns an empty flow untouched", () => {
    expect(resolveReviewNodes([], "workflow_finish")).toEqual([]);
  });
});

function workflowRow(): SqlWorkflowRow {
  return {
    id: 5338,
    workflowName: "DDL - 车辆表结构修改",
    engineerDisplay: "廖思源",
    status: "workflow_finish",
    isBackup: true,
    createTime: "2026-11-02 10:31:10",
    instanceName: "test-express-rw",
    dbName: "schedule-dev",
    groupName: "Test-Express",
    syntaxType: 1,
    exportFormat: "",
  };
}

function gridRow(): WorkflowGridRow {
  return {
    id: 1,
    sql: "",
    errorLevel: 0,
    errorMessage: "",
    affectedRows: 0,
    executeTime: "0",
    backupTime: "0",
    stage: "Execute Successfully",
  };
}
