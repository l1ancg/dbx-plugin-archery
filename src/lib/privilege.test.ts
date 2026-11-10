import { describe, expect, it } from "vitest";
import type {
  PrivilegeApplyForm,
  PrivilegeApplyRow,
  PrivilegeReviewNode,
  UserPrivilegeRow,
} from "../types";
import {
  canAuditPrivilege,
  detailItems,
  filterUserPrivileges,
  normalizePrivilegeApply,
  normalizePrivilegeDetail,
  normalizeUserPrivilege,
  normalizeWorkflowLog,
  privilegeCopyText,
  privilegeExpiry,
  privilegeLevelKey,
  privilegeStatusMeta,
  privTypeLabel,
  privTypeOptions,
  reviewNodeText,
  sortUserPrivileges,
  splitPrivilegeList,
  summarizeUserPrivileges,
  validDatePreset,
  validatePrivilegeApply,
} from "./privilege";

function form(patch: Partial<PrivilegeApplyForm> = {}): PrivilegeApplyForm {
  return {
    title: "排查线上问题",
    groupName: "Demo-Group-C",
    instanceName: "demo-order-rw",
    privType: 1,
    dbName: "",
    dbList: ["shop"],
    tableList: [],
    validDatePreset: "week",
    limitNum: 500,
    ...patch,
  };
}

describe("privilege labels", () => {
  it("maps priv_type to Archery labels", () => {
    expect(privTypeLabel(1)).toBe("DATABASE");
    expect(privTypeLabel(2)).toBe("TABLE");
    // Archery 的 formatter 对非 1 值一律显示 TABLE。
    expect(privTypeLabel(9)).toBe("TABLE");
  });

  it("only offers TABLE for MySQL engines", () => {
    expect(privTypeOptions("mysql")).toEqual([1, 2]);
    expect(privTypeOptions("MySQL")).toEqual([1, 2]);
    expect(privTypeOptions("pgsql")).toEqual([1]);
    expect(privTypeOptions("")).toEqual([1]);
  });

  it("maps workflow status to tone and i18n key", () => {
    expect(privilegeStatusMeta(0)).toEqual({ tone: "warn", labelKey: "privilege.status.pending" });
    expect(privilegeStatusMeta(1)).toEqual({ tone: "ok", labelKey: "privilege.status.passed" });
    expect(privilegeStatusMeta(2)).toEqual({
      tone: "danger",
      labelKey: "privilege.status.rejected",
    });
    expect(privilegeStatusMeta(3)).toEqual({ tone: "muted", labelKey: "privilege.status.aborted" });
    expect(privilegeStatusMeta(99)).toEqual({ tone: "muted", labelKey: "privilege.status.unknown" });
  });

  it("only allows auditing pending tickets", () => {
    expect(canAuditPrivilege(0)).toBe(true);
    expect(canAuditPrivilege(1)).toBe(false);
    expect(canAuditPrivilege(2)).toBe(false);
    expect(canAuditPrivilege(3)).toBe(false);
  });
});

describe("validDatePreset", () => {
  const base = new Date(2026, 9, 31);

  it("converts presets to Archery's unpadded date", () => {
    expect(validDatePreset("day", base)).toBe("2026-11-1");
    expect(validDatePreset("week", base)).toBe("2026-11-7");
    expect(validDatePreset("month", base)).toBe("2026-11-30");
    expect(validDatePreset("year", base)).toBe("2027-10-31");
  });

  it("does not mutate the base date", () => {
    validDatePreset("year", base);
    expect(base.getFullYear()).toBe(2026);
    expect(base.getMonth()).toBe(9);
  });
});

describe("splitPrivilegeList", () => {
  it("splits comma separated Archery values", () => {
    expect(splitPrivilegeList("demo_materials,demo_config")).toEqual([
      "demo_materials",
      "demo_config",
    ]);
    expect(splitPrivilegeList(" orders , order_items ,, ")).toEqual(["orders", "order_items"]);
    expect(splitPrivilegeList("")).toEqual([]);
  });
});

describe("normalizePrivilegeApply", () => {
  it("maps Django values() keys", () => {
    const row = normalizePrivilegeApply({
      apply_id: "7",
      title: "查询数据",
      instance__instance_name: "demo-mysql-rw",
      db_list: "demo_materials,demo_config",
      priv_type: "1",
      table_list: "",
      limit_num: "1000",
      valid_date: "2027-10-31",
      user_display: "示例用户A",
      status: "1",
      create_time: "2026-10-31 10:03:36",
      group_name: "Demo-Group-A",
    });
    expect(row).toEqual({
      applyId: 7,
      title: "查询数据",
      instanceName: "demo-mysql-rw",
      dbList: "demo_materials,demo_config",
      privType: 1,
      tableList: "",
      limitNum: 1000,
      validDate: "2027-10-31",
      userDisplay: "示例用户A",
      status: 1,
      createTime: "2026-10-31 10:03:36",
      groupName: "Demo-Group-A",
    });
  });

  it("accepts already normalized camelCase payloads", () => {
    const row = normalizePrivilegeApply({
      applyId: 6,
      title: "查看数据",
      instanceName: "demo-order-rw",
      dbList: "erp",
      privType: 2,
      tableList: "orders,order_items",
      limitNum: 500,
      validDate: "2026-06-08",
      userDisplay: "示例用户A",
      status: 2,
      createTime: "2026-06-01 09:00:00",
      groupName: "Demo-Group-C",
    });
    expect(row.applyId).toBe(6);
    expect(row.instanceName).toBe("demo-order-rw");
    expect(row.privType).toBe(2);
    expect(row.tableList).toBe("orders,order_items");
    expect(row.status).toBe(2);
  });
});

describe("normalizeWorkflowLog", () => {
  it("maps operation log keys", () => {
    expect(
      normalizeWorkflowLog({
        operation_type_desc: "提交申请",
        operator_display: "示例用户A",
        operation_time: "2026-10-31 10:03:36",
        operation_info: "申请查询权限",
      }),
    ).toEqual({
      operationType: "提交申请",
      operator: "示例用户A",
      time: "2026-10-31 10:03:36",
      info: "申请查询权限",
    });
  });
});

describe("validatePrivilegeApply", () => {
  it("accepts a complete database-level form", () => {
    expect(validatePrivilegeApply(form())).toEqual([]);
  });

  it("reports missing database-level fields", () => {
    const missing = validatePrivilegeApply(
      form({
        title: " ",
        groupName: "",
        instanceName: "",
        dbList: [],
        limitNum: 0,
      }),
    );
    expect(missing).toEqual(["title", "groupName", "instanceName", "dbList", "limitNum"]);
  });

  it("requires database and tables for table-level requests", () => {
    const missing = validatePrivilegeApply(
      form({ privType: 2, dbName: "", dbList: [], tableList: [] }),
    );
    expect(missing).toEqual(["dbName", "tableList"]);
  });

  it("rejects unknown privilege types", () => {
    expect(validatePrivilegeApply(form({ privType: 0 }))).toEqual(["privType"]);
  });
});

function applyRow(patch: Partial<PrivilegeApplyRow> = {}): PrivilegeApplyRow {
  return {
    applyId: 7,
    title: "查询数据",
    instanceName: "demo-mysql-rw",
    dbList: "demo_materials,demo_config",
    privType: 1,
    tableList: "",
    limitNum: 1000,
    validDate: "2027-10-31",
    userDisplay: "示例用户A",
    status: 1,
    createTime: "2026-10-31 10:03:36",
    groupName: "Demo-Group-A",
    ...patch,
  };
}

describe("normalizePrivilegeDetail", () => {
  it("maps snake_case detail payloads and keeps node order", () => {
    const detail = normalizePrivilegeDetail({
      apply_id: "7",
      title: "查询数据",
      can_review: true,
      review_nodes: [
        { name: "Demo:Leader-A", state: "passed" },
        { name: " 示例审批节点 ", state: "current", reviewers: ["张伟", "李娜"] },
        { name: "Leader-A", state: "weird" },
        { name: "  ", state: "pending" },
      ],
      items: ["demo_materials", "demo_config"],
      item_kind: "database",
      last_operation: "示例用户A 2026-10-31 10:05:00 审核通过",
    });
    expect(detail).toEqual({
      applyId: 7,
      title: "查询数据",
      canReview: true,
      reviewNodes: [
        { name: "Demo:Leader-A", state: "passed", reviewers: [] },
        { name: "示例审批节点", state: "current", reviewers: ["张伟", "李娜"] },
        // 未来 Archery 新增节点状态时按「未到达」展示，不丢节点。
        { name: "Leader-A", state: "pending", reviewers: [] },
      ],
      items: ["demo_materials", "demo_config"],
      itemKind: "database",
      lastOperation: "示例用户A 2026-10-31 10:05:00 审核通过",
    });
  });

  it("keeps auto-pass nodes that carry no group name", () => {
    const detail = normalizePrivilegeDetail({
      review_nodes: [{ name: "", state: "auto" }],
    });
    expect(detail.reviewNodes).toEqual([{ name: "", state: "auto", reviewers: [] }]);
  });

  it("splits string reviewers on both comma styles", () => {
    const detail = normalizePrivilegeDetail({
      review_nodes: [{ name: "DBA", state: "current", reviewers: "张伟，李娜, 王五" }],
    });
    expect(detail.reviewNodes[0].reviewers).toEqual(["张伟", "李娜", "王五"]);
  });

  it("returns an empty detail for missing or partial payloads", () => {
    expect(normalizePrivilegeDetail(null)).toEqual({
      applyId: 0,
      title: "",
      canReview: false,
      reviewNodes: [],
      items: [],
      itemKind: "",
      lastOperation: "",
    });
    expect(
      normalizePrivilegeDetail({ items: "a, b", item_kind: "schema" }),
    ).toMatchObject({ items: ["a", "b"], itemKind: "" });
  });
});

describe("reviewNodeText", () => {
  it("replaces auto-pass nodes with the localized label", () => {
    expect(reviewNodeText({ name: "", state: "auto", reviewers: [] }, "无需审批")).toBe(
      "无需审批",
    );
  });

  it("appends reviewers of the current node", () => {
    expect(
      reviewNodeText({ name: "示例审批节点", state: "current", reviewers: ["张伟", "李娜"] }, "无需审批"),
    ).toBe("示例审批节点(张伟, 李娜)");
    expect(
      reviewNodeText({ name: "示例审批节点", state: "current", reviewers: [] }, "无需审批"),
    ).toBe("示例审批节点");
  });

  it("keeps passed and pending node names", () => {
    expect(
      reviewNodeText({ name: "Leader-A", state: "passed", reviewers: [] }, "无需审批"),
    ).toBe("Leader-A");
    expect(
      reviewNodeText({ name: "Leader-A", state: "pending", reviewers: [] }, "无需审批"),
    ).toBe("Leader-A");
  });
});

describe("detailItems", () => {
  it("prefers the detail page list when the kind matches the ticket", () => {
    const detail = normalizePrivilegeDetail({
      items: ["detail_a", "detail_b"],
      item_kind: "database",
    });
    expect(detailItems(detail, applyRow())).toEqual(["detail_a", "detail_b"]);
  });

  it("falls back to the list row when the kind mismatches", () => {
    const row = applyRow({ privType: 2, dbList: "demo_materials", tableList: "orders,order_items" });
    const detail = normalizePrivilegeDetail({ items: ["detail_a"], item_kind: "database" });
    expect(detailItems(detail, row)).toEqual(["orders", "order_items"]);
  });

  it("falls back to the list row when the detail page has no list", () => {
    expect(detailItems(normalizePrivilegeDetail({}), applyRow())).toEqual([
      "demo_materials",
      "demo_config",
    ]);
  });
});

describe("normalizeUserPrivilege", () => {
  it("maps Archery /query/userprivileges/ values() keys", () => {
    expect(
      normalizeUserPrivilege({
        privilege_id: "11",
        user_display: "示例用户A",
        instance__instance_name: "demo-mysql-rw",
        db_name: "demo_config",
        priv_type: "1",
        table_name: "",
        limit_num: "1000",
        valid_date: "2027-10-31",
      }),
    ).toEqual({
      privilegeId: 11,
      userDisplay: "示例用户A",
      instanceName: "demo-mysql-rw",
      dbName: "demo_config",
      privType: 1,
      tableName: "",
      limitNum: 1000,
      validDate: "2027-10-31",
    });
  });

  it("accepts already normalized camelCase payloads", () => {
    expect(
      normalizeUserPrivilege({
        privilegeId: 10,
        userDisplay: "示例用户A",
        instanceName: "demo-order-rw",
        dbName: "erp",
        privType: 2,
        tableName: "orders",
        limitNum: 500,
        validDate: "2026-11-30",
      }),
    ).toEqual({
      privilegeId: 10,
      userDisplay: "示例用户A",
      instanceName: "demo-order-rw",
      dbName: "erp",
      privType: 2,
      tableName: "orders",
      limitNum: 500,
      validDate: "2026-11-30",
    });
  });

  it("falls back to empty values for missing fields", () => {
    expect(normalizeUserPrivilege({})).toEqual({
      privilegeId: 0,
      userDisplay: "",
      instanceName: "",
      dbName: "",
      privType: 0,
      tableName: "",
      limitNum: 0,
      validDate: "",
    });
  });
});

function userPrivilege(patch: Partial<UserPrivilegeRow> = {}): UserPrivilegeRow {
  return {
    privilegeId: 1,
    userDisplay: "示例用户A",
    instanceName: "demo-order-rw",
    dbName: "demo_biz",
    privType: 1,
    tableName: "",
    limitNum: 1000,
    validDate: "2026-12-31",
    ...patch,
  };
}

function privilegeIds(rows: UserPrivilegeRow[]): number[] {
  return rows.map((row) => row.privilegeId);
}

// 固定「今天」为 2026-11-05，避免用例随时间漂移。
const TODAY = new Date(2026, 10, 5);

describe("privilegeExpiry", () => {
  it("treats today as expiring soon rather than expired", () => {
    expect(privilegeExpiry("2026-11-05", TODAY)).toEqual({ state: "soon", days: 0 });
  });

  it("flags the last day of the soon window", () => {
    expect(privilegeExpiry("2026-11-12", TODAY)).toEqual({ state: "soon", days: 7 });
  });

  it("keeps permissions valid beyond the soon window", () => {
    expect(privilegeExpiry("2026-11-13", TODAY)).toEqual({ state: "active", days: 8 });
  });

  it("marks past dates as expired with negative days", () => {
    expect(privilegeExpiry("2026-11-04", TODAY)).toEqual({ state: "expired", days: -1 });
    expect(privilegeExpiry("2025-11-05", TODAY)).toEqual({ state: "expired", days: -365 });
  });

  it("parses unpadded dates and ignores the time part", () => {
    expect(privilegeExpiry("2027-1-5", TODAY)).toEqual({ state: "active", days: 61 });
    expect(privilegeExpiry("2026-11-12 23:59:59", TODAY)).toEqual({
      state: "soon",
      days: 7,
    });
  });

  it("compares calendar days instead of elapsed milliseconds", () => {
    const lateToday = new Date(2026, 10, 5, 23, 59);
    expect(privilegeExpiry("2026-11-06", lateToday)).toEqual({ state: "soon", days: 1 });
  });

  it("returns unknown for empty or malformed input", () => {
    expect(privilegeExpiry("", TODAY)).toEqual({ state: "unknown", days: 0 });
    expect(privilegeExpiry("长期", TODAY)).toEqual({ state: "unknown", days: 0 });
    expect(privilegeExpiry("2026-13-01", TODAY)).toEqual({ state: "unknown", days: 0 });
    // 2 月没有 31 日，不能让 Date 自动进位成 3 月。
    expect(privilegeExpiry("2026-02-31", TODAY)).toEqual({ state: "unknown", days: 0 });
  });
});

describe("privilegeCopyText", () => {
  it("copies the database name for database-level records", () => {
    expect(privilegeCopyText({ dbName: "demo_config", tableName: "" })).toBe("demo_config");
  });

  it("copies database.table for table-level records", () => {
    expect(privilegeCopyText({ dbName: "erp", tableName: "orders" })).toBe("erp.orders");
  });

  it("trims both parts and skips empty halves", () => {
    expect(privilegeCopyText({ dbName: " erp ", tableName: " orders " })).toBe("erp.orders");
    expect(privilegeCopyText({ dbName: "", tableName: "orders" })).toBe("orders");
    expect(privilegeCopyText({ dbName: "", tableName: "" })).toBe("");
  });
});

describe("privilegeLevelKey", () => {
  it("localizes DATABASE only for priv_type 1", () => {
    expect(privilegeLevelKey(1)).toBe("privilege.level.database");
    expect(privilegeLevelKey(2)).toBe("privilege.level.table");
    // 与 privTypeLabel 一致：Archery 对非 1 值一律按 TABLE 展示。
    expect(privilegeLevelKey(9)).toBe("privilege.level.table");
    expect(privilegeLevelKey(0)).toBe("privilege.level.table");
  });
});

function filterRows(): UserPrivilegeRow[] {
  return [
    userPrivilege({
      privilegeId: 1,
      instanceName: "demo-order-rw",
      dbName: "demo_biz",
      validDate: "2026-11-04",
    }),
    userPrivilege({
      privilegeId: 2,
      instanceName: "demo-order-rw",
      dbName: "demo_mq",
      validDate: "2026-11-10",
    }),
    userPrivilege({
      privilegeId: 3,
      instanceName: "demo-mysql-rw",
      dbName: "demo_config",
      validDate: "2027-10-31",
    }),
    userPrivilege({
      privilegeId: 4,
      userDisplay: "示例用户B",
      instanceName: "demo-mysql-rw",
      dbName: "erp",
      privType: 2,
      tableName: "orders",
      validDate: "",
    }),
  ];
}

describe("filterUserPrivileges", () => {
  it("returns every row without filters, as a new array", () => {
    const rows = filterRows();
    const filtered = filterUserPrivileges(rows, {}, TODAY);
    expect(privilegeIds(filtered)).toEqual([1, 2, 3, 4]);
    expect(filtered).not.toBe(rows);
  });

  it("matches the keyword across user, instance, database and table", () => {
    const rows = filterRows();
    expect(privilegeIds(filterUserPrivileges(rows, { keyword: "示例用户B" }, TODAY))).toEqual([4]);
    expect(privilegeIds(filterUserPrivileges(rows, { keyword: "MYSQL" }, TODAY))).toEqual([3, 4]);
    expect(privilegeIds(filterUserPrivileges(rows, { keyword: "orders" }, TODAY))).toEqual([4]);
    expect(privilegeIds(filterUserPrivileges(rows, { keyword: " demo_config " }, TODAY))).toEqual([
      3,
    ]);
    expect(privilegeIds(filterUserPrivileges(rows, { keyword: "no-match" }, TODAY))).toEqual([]);
  });

  it("filters by instance and privilege level", () => {
    const rows = filterRows();
    expect(
      privilegeIds(filterUserPrivileges(rows, { instanceName: "demo-mysql-rw" }, TODAY)),
    ).toEqual([3, 4]);
    expect(privilegeIds(filterUserPrivileges(rows, { privType: 1 }, TODAY))).toEqual([1, 2, 3]);
    expect(privilegeIds(filterUserPrivileges(rows, { privType: 2 }, TODAY))).toEqual([4]);
  });

  it("filters by expiry state", () => {
    const rows = filterRows();
    expect(privilegeIds(filterUserPrivileges(rows, { expiry: "expired" }, TODAY))).toEqual([1]);
    expect(privilegeIds(filterUserPrivileges(rows, { expiry: "soon" }, TODAY))).toEqual([2]);
    expect(privilegeIds(filterUserPrivileges(rows, { expiry: "active" }, TODAY))).toEqual([3]);
    expect(privilegeIds(filterUserPrivileges(rows, { expiry: "unknown" }, TODAY))).toEqual([4]);
  });

  it("combines conditions with AND", () => {
    const rows = filterRows();
    expect(
      privilegeIds(
        filterUserPrivileges(
          rows,
          { instanceName: "demo-order-rw", privType: 1, expiry: "soon" },
          TODAY,
        ),
      ),
    ).toEqual([2]);
  });
});

function sortRows(): UserPrivilegeRow[] {
  return [
    userPrivilege({
      privilegeId: 1,
      instanceName: "b-instance",
      dbName: "alpha",
      validDate: "2026-12-05",
    }),
    userPrivilege({
      privilegeId: 2,
      instanceName: "a-instance",
      dbName: "zeta",
      validDate: "2026-11-08",
    }),
    userPrivilege({
      privilegeId: 3,
      instanceName: "a-instance",
      dbName: "alpha",
      tableName: "users",
      validDate: "2026-11-04",
    }),
    userPrivilege({
      privilegeId: 4,
      instanceName: "a-instance",
      dbName: "alpha",
      tableName: "orders",
      validDate: "2026-11-05",
    }),
    userPrivilege({
      privilegeId: 5,
      instanceName: "c-instance",
      dbName: "alpha",
      validDate: "",
    }),
  ];
}

describe("sortUserPrivileges", () => {
  it("sorts by instance then database and table", () => {
    expect(privilegeIds(sortUserPrivileges(sortRows(), "instance", "asc", TODAY))).toEqual([
      4, 3, 2, 1, 5,
    ]);
    expect(privilegeIds(sortUserPrivileges(sortRows(), "instance", "desc", TODAY))).toEqual([
      5, 1, 4, 3, 2,
    ]);
  });

  it("sorts by database then instance", () => {
    expect(privilegeIds(sortUserPrivileges(sortRows(), "database", "asc", TODAY))).toEqual([
      4, 3, 1, 5, 2,
    ]);
  });

  it("sorts by expiry and keeps unknown rows last in both directions", () => {
    expect(privilegeIds(sortUserPrivileges(sortRows(), "expiry", "asc", TODAY))).toEqual([
      3, 4, 2, 1, 5,
    ]);
    expect(privilegeIds(sortUserPrivileges(sortRows(), "expiry", "desc", TODAY))).toEqual([
      1, 2, 4, 3, 5,
    ]);
  });

  it("keeps the original order for equal keys and never mutates the input", () => {
    const rows = sortRows();
    const before = privilegeIds(rows);
    const doubled = [
      userPrivilege({ privilegeId: 6, instanceName: "a-instance", dbName: "same" }),
      userPrivilege({ privilegeId: 7, instanceName: "a-instance", dbName: "same" }),
    ];
    expect(privilegeIds(sortUserPrivileges(doubled, "database", "asc", TODAY))).toEqual([6, 7]);
    sortUserPrivileges(rows, "expiry", "desc", TODAY);
    expect(privilegeIds(rows)).toEqual(before);
  });
});

describe("summarizeUserPrivileges", () => {
  it("counts each expiry state and unique instances", () => {
    expect(summarizeUserPrivileges(filterRows(), TODAY)).toEqual({
      total: 4,
      active: 1,
      soon: 1,
      expired: 1,
      unknown: 1,
      instances: 2,
    });
  });

  it("returns zeros for an empty list", () => {
    expect(summarizeUserPrivileges([], TODAY)).toEqual({
      total: 0,
      active: 0,
      soon: 0,
      expired: 0,
      unknown: 0,
      instances: 0,
    });
  });
});
