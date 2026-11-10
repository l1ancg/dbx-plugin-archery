import { describe, expect, it } from "vitest";
import {
  canAuditWorkflow,
  canCancelWorkflow,
  canExecuteWorkflow,
  normalizeCheckResult,
  normalizeSqlWorkflow,
  normalizeSqlWorkflowDetail,
  splitAuditMessages,
  stripImportedSql,
  syntaxTypeLabel,
  validateSqlSubmit,
  workflowScopeStatus,
  workflowStatusMeta,
} from "./workflow";
import type { SqlSubmitForm } from "../types";

const NOW = new Date("2026-11-02T10:00:00");

function form(patch: Partial<SqlSubmitForm> = {}): SqlSubmitForm {
  return {
    title: "库存盘点任务",
    groupId: 3,
    instanceId: 12,
    dbName: "materials_test",
    sql: "alter table stock add column remark varchar(64);",
    backup: true,
    runDateStart: "2026-11-02 12:00",
    runDateEnd: "2026-11-02 14:00",
    ...patch,
  };
}

describe("workflowStatusMeta", () => {
  // Archery v1.14 SqlWorkflow.status 的 9 个取值，见 sql/models.py SQL_WORKFLOW_CHOICES。
  const cases: Array<[string, string, string]> = [
    ["workflow_manreviewing", "warn", "workflow.status.manreviewing"],
    ["workflow_review_pass", "ok", "workflow.status.reviewPass"],
    ["workflow_timingtask", "warn", "workflow.status.timingtask"],
    ["workflow_queuing", "warn", "workflow.status.queuing"],
    ["workflow_executing", "warn", "workflow.status.executing"],
    ["workflow_finish", "ok", "workflow.status.finish"],
    ["workflow_abort", "muted", "workflow.status.abort"],
    ["workflow_autoreviewwrong", "danger", "workflow.status.autoreviewwrong"],
    ["workflow_exception", "danger", "workflow.status.exception"],
  ];

  it("maps every Archery workflow status", () => {
    for (const [status, tone, labelKey] of cases) {
      expect(workflowStatusMeta(status)).toEqual({ tone, labelKey });
    }
  });

  it("falls back to a muted label for unknown or empty status", () => {
    expect(workflowStatusMeta("")).toEqual({
      tone: "muted",
      labelKey: "workflow.status.unknown",
    });
    expect(workflowStatusMeta("workflow_brand_new")).toEqual({
      tone: "muted",
      labelKey: "workflow.status.unknown",
    });
  });
});

describe("syntaxTypeLabel", () => {
  it("maps Archery integer syntax_type", () => {
    expect(syntaxTypeLabel(1)).toBe("DDL");
    expect(syntaxTypeLabel(2)).toBe("DML");
    expect(syntaxTypeLabel(3)).toBe("OFFLINE");
    expect(syntaxTypeLabel(0)).toBe("");
  });

  it("accepts string and numeric-string values", () => {
    expect(syntaxTypeLabel("DDL")).toBe("DDL");
    expect(syntaxTypeLabel("dml")).toBe("DML");
    expect(syntaxTypeLabel("2")).toBe("DML");
    expect(syntaxTypeLabel(undefined)).toBe("");
    expect(syntaxTypeLabel(null)).toBe("");
    expect(syntaxTypeLabel("null")).toBe("");
  });
});

describe("normalizeSqlWorkflow", () => {
  it("maps the sqlworkflow_list row shape", () => {
    const row = normalizeSqlWorkflow({
      id: "1024",
      workflow_name: "库存盘点任务",
      engineer_display: "喻量",
      status: "workflow_finish",
      is_backup: 1,
      create_time: "2026-10-28 17:45:03",
      instance__instance_name: "materials_test",
      db_name: "materials_test",
      group_name: "Test-Materials",
      syntax_type: 2,
      export_format: null,
    });
    expect(row).toEqual({
      id: 1024,
      workflowName: "库存盘点任务",
      engineerDisplay: "喻量",
      status: "workflow_finish",
      isBackup: true,
      createTime: "2026-10-28 17:45:03",
      instanceName: "materials_test",
      dbName: "materials_test",
      groupName: "Test-Materials",
      syntaxType: 2,
      exportFormat: "",
    });
  });

  it("accepts already-normalized camelCase rows and missing fields", () => {
    const row = normalizeSqlWorkflow({
      id: 7,
      workflowName: "DDL - 车辆表结构修改",
      instanceName: "test-express-rw",
      syntaxType: "DDL",
      isBackup: "false",
    });
    expect(row.id).toBe(7);
    expect(row.workflowName).toBe("DDL - 车辆表结构修改");
    expect(row.instanceName).toBe("test-express-rw");
    expect(row.syntaxType).toBe(1);
    expect(row.isBackup).toBe(false);
    expect(row.status).toBe("");
    expect(row.dbName).toBe("");
  });
});

describe("status gates", () => {
  it("only allows audit while waiting for review", () => {
    expect(canAuditWorkflow("workflow_manreviewing")).toBe(true);
    expect(canAuditWorkflow("workflow_review_pass")).toBe(false);
    expect(canAuditWorkflow("workflow_finish")).toBe(false);
  });

  it("allows execute after the review passed", () => {
    expect(canExecuteWorkflow("workflow_review_pass")).toBe(true);
    expect(canExecuteWorkflow("workflow_timingtask")).toBe(true);
    expect(canExecuteWorkflow("workflow_manreviewing")).toBe(false);
    expect(canExecuteWorkflow("workflow_finish")).toBe(false);
  });

  it("allows abort before the ticket is finished", () => {
    expect(canCancelWorkflow("workflow_manreviewing")).toBe(true);
    expect(canCancelWorkflow("workflow_review_pass")).toBe(true);
    expect(canCancelWorkflow("workflow_queuing")).toBe(true);
    expect(canCancelWorkflow("workflow_finish")).toBe(false);
    expect(canCancelWorkflow("workflow_abort")).toBe(false);
  });
});

describe("validateSqlSubmit", () => {
  it("accepts a complete form", () => {
    expect(validateSqlSubmit(form(), NOW)).toEqual([]);
  });

  it("reports every missing field in form order", () => {
    expect(
      validateSqlSubmit(
        form({ title: " ", groupId: 0, instanceId: 0, dbName: "", sql: "  " }),
        NOW,
      ),
    ).toEqual(["title", "groupId", "instanceId", "dbName", "sql"]);
  });

  it("rejects a title longer than 50 characters", () => {
    expect(validateSqlSubmit(form({ title: "x".repeat(51) }), NOW)).toEqual(["titleLength"]);
  });

  it("requires an execution window of at least 60 minutes", () => {
    expect(
      validateSqlSubmit(
        form({ runDateStart: "2026-11-02 12:00", runDateEnd: "2026-11-02 12:30" }),
        NOW,
      ),
    ).toEqual(["runDateRange"]);
  });

  it("rejects an execution window that already started", () => {
    expect(
      validateSqlSubmit(
        form({ runDateStart: "2026-10-01 08:00", runDateEnd: "2026-10-01 12:00" }),
        NOW,
      ),
    ).toEqual(["runDatePast"]);
  });

  it("skips the window check when no range is given", () => {
    expect(validateSqlSubmit(form({ runDateStart: "", runDateEnd: "" }), NOW)).toEqual([]);
  });
});

describe("normalizeSqlWorkflowDetail", () => {
  it("maps review rows from /sqlworkflow/detail_content/", () => {
    const detail = normalizeSqlWorkflowDetail({
      workflowId: 9,
      title: "库存盘点任务",
      status: "workflow_finish",
      reviewNodes: [{ name: "Test-Materials:Leader", state: "passed", reviewers: [] }],
      lastOperation: "喻量 2026-10-28 17:45:03 提交工单",
      reviewRows: [
        {
          sequence: "0_0_00000000",
          stage: "阶段1",
          stagestatus: "Audit completed",
          errlevel: 2,
          errormessage: "syntax error",
          sql: "alter table stock add column remark varchar(64)",
          affected_rows: 0,
          backup_dbname: "",
          execute_time: "0.01",
        },
      ],
    });
    expect(detail.workflowId).toBe(9);
    expect(detail.title).toBe("库存盘点任务");
    expect(detail.status).toBe("workflow_finish");
    expect(detail.reviewNodes).toHaveLength(1);
    expect(detail.lastOperation).toContain("提交工单");
    expect(detail.reviewRows).toEqual([
      {
        sequence: "0_0_00000000",
        stage: "阶段1",
        status: "Audit completed",
        errorLevel: 2,
        errorMessage: "syntax error",
        sql: "alter table stock add column remark varchar(64)",
        affectedRows: 0,
        backupDbName: "",
        executeTime: "0.01",
      },
    ]);
  });

  it("degrades to empty values when blocks are missing", () => {
    const detail = normalizeSqlWorkflowDetail(null);
    expect(detail.workflowId).toBe(0);
    expect(detail.reviewNodes).toEqual([]);
    expect(detail.reviewRows).toEqual([]);
    expect(detail.lastOperation).toBe("");
  });

  it("accepts snake_case detail payloads", () => {
    const detail = normalizeSqlWorkflowDetail({
      workflow_id: 11,
      review_nodes: [{ name: "DBA", state: "current", reviewers: ["审核人A"] }],
      review_rows: [{ errlevel: "1", errormessage: "warn" }],
    });
    expect(detail.workflowId).toBe(11);
    expect(detail.reviewNodes[0].reviewers).toEqual(["审核人A"]);
    expect(detail.reviewRows[0].errorLevel).toBe(1);
  });
});

describe("normalizeCheckResult", () => {
  it("counts error and warning rows by errlevel", () => {
    const result = normalizeCheckResult({
      rows: [
        { stage: "阶段1", errlevel: 2, errormessage: "syntax error", sql: "a" },
        { stage: "阶段2", errlevel: 1, errormessage: "no where", sql: "b" },
        { stage: "阶段3", errlevel: 0, errormessage: "", sql: "c" },
      ],
    });
    expect(result.errorCount).toBe(1);
    expect(result.warningCount).toBe(1);
    expect(result.rows).toHaveLength(3);
    expect(result.rows[0].errorMessage).toBe("syntax error");
  });

  it("prefers explicit counters from the API", () => {
    const result = normalizeCheckResult({ error_count: 3, warning_count: 4, rows: [] });
    expect(result.errorCount).toBe(3);
    expect(result.warningCount).toBe(4);
  });

  it("treats a missing payload as clean", () => {
    expect(normalizeCheckResult(null)).toEqual({ errorCount: 0, warningCount: 0, rows: [] });
  });
});

describe("splitAuditMessages", () => {
  it("splits the '. '-joined messages Archery returns for one statement", () => {
    expect(
      splitAuditMessages(
        "Column 'sm_user.version' not existed. Column 'sm_user.deleted' not existed. Column count doesn't match value count at row 1.",
      ),
    ).toEqual([
      "Column 'sm_user.version' not existed.",
      "Column 'sm_user.deleted' not existed.",
      "Column count doesn't match value count at row 1.",
    ]);
  });

  it("keeps a single sentence on one line", () => {
    expect(splitAuditMessages("建议补充 WHERE 条件，避免全表更新")).toEqual([
      "建议补充 WHERE 条件，避免全表更新",
    ]);
  });

  it("splits on line breaks and drops blank lines", () => {
    expect(splitAuditMessages("第一行\n第二行\n\n")).toEqual(["第一行", "第二行"]);
  });

  it("does not split decimals or dotted identifiers", () => {
    expect(splitAuditMessages("Column v1.2 not found. Please check 3.14 value")).toEqual([
      "Column v1.2 not found.",
      "Please check 3.14 value",
    ]);
  });

  it("returns no lines for empty input", () => {
    expect(splitAuditMessages("")).toEqual([]);
    expect(splitAuditMessages("   ")).toEqual([]);
    expect(splitAuditMessages(null)).toEqual([]);
  });
});

describe("stripImportedSql", () => {
  // 移除文件 = 把当初追加进去的那段 SQL 再摘出来，保留导入前手写的内容。
  it("restores the SQL that existed before the file was appended", () => {
    const before = "alter table a add column b int;";
    const imported = "insert into a values (1);";
    expect(stripImportedSql(`${before}\n${imported}`, imported)).toEqual({
      sql: before,
      removed: true,
    });
  });

  it("clears the editor when it only held the imported file", () => {
    const text = "insert into a values (1);";
    expect(stripImportedSql(text, text)).toEqual({ sql: "", removed: true });
  });

  it("keeps the SQL untouched when the imported text was edited", () => {
    const current = "insert into a values (2);";
    expect(stripImportedSql(current, "insert into a values (1);")).toEqual({
      sql: current,
      removed: false,
    });
  });

  it("removes only the last copy when the same file was appended twice", () => {
    const text = "truncate table a;";
    expect(stripImportedSql(`${text}\n${text}`, text)).toEqual({ sql: text, removed: true });
  });

  it("reports nothing removed for empty imported text", () => {
    expect(stripImportedSql("select 1", "")).toEqual({ sql: "select 1", removed: false });
  });
});

describe("workflowScopeStatus", () => {
  // 「待我审核」只可能是待审核状态；申请工单才按页面上的状态筛选走。
  it("locks the audit scope to workflow_manreviewing", () => {
    expect(workflowScopeStatus(true, "")).toBe("workflow_manreviewing");
    expect(workflowScopeStatus(true, "workflow_finish")).toBe("workflow_manreviewing");
  });

  it("keeps the selected status for the normal scope", () => {
    expect(workflowScopeStatus(false, "workflow_finish")).toBe("workflow_finish");
    expect(workflowScopeStatus(false, "")).toBe("");
  });
});
