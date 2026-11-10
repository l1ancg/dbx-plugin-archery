import { describe, expect, it } from "vitest";
import type { DescribePanelTab, PanelTab, ResultPanelTab } from "../types";
import {
  appendPanel,
  closePanel,
  createDescribePanel,
  createResultPanel,
  describeTitle,
  findDescribePanel,
  initialPanels,
  isClosable,
  movePanel,
  nextResultIndex,
  systemPanels,
  targetResultPanel,
  type DescribeTarget,
} from "./panels";

function target(dbName: string, tableName: string, instanceName = "demo-mysql"): DescribeTarget {
  return { instanceName, dbName, tableName };
}

const kinds = (panels: PanelTab[]) => panels.map((panel) => panel.kind);
const ids = (panels: PanelTab[]) => panels.map((panel) => panel.id);

function describeTab(panels: PanelTab[], dbName: string, tableName: string): DescribePanelTab {
  const found = findDescribePanel(panels, target(dbName, tableName));
  if (!found) throw new Error(`missing describe tab ${dbName}.${tableName}`);
  return found;
}

describe("initialPanels", () => {
  it("puts the system tabs first and starts with one empty result tab", () => {
    const panels = initialPanels();
    expect(kinds(panels)).toEqual(["message", "history", "result"]);
    expect((panels[2] as ResultPanelTab).index).toBe(1);
    expect((panels[2] as ResultPanelTab).result).toBeNull();
  });

  it("keeps the system tabs stable and unclosable", () => {
    const panels = systemPanels();
    expect(kinds(panels)).toEqual(["message", "history"]);
    expect(panels.every((panel) => !isClosable(panel))).toBe(true);
  });
});

describe("result tabs", () => {
  it("numbers new result tabs after the highest open one", () => {
    const panels = initialPanels();
    expect(nextResultIndex(panels)).toBe(2);
    const withSecond = appendPanel(panels, createResultPanel(2));
    expect(nextResultIndex(withSecond)).toBe(3);
  });

  it("reuses a freed number only after its tab is closed", () => {
    const panels = appendPanel(initialPanels(), createResultPanel(2));
    const closed = closePanel(panels, "panel-result-2", "panel-result-2").panels;
    expect(nextResultIndex(closed)).toBe(2);
  });

  it("keeps result tabs closable", () => {
    expect(isClosable(createResultPanel(1))).toBe(true);
  });

  it("runs into the active result tab instead of opening another one", () => {
    const panels = appendPanel(appendPanel(initialPanels(), createResultPanel(2)), createResultPanel(3));
    expect(targetResultPanel(panels, "panel-result-2")?.index).toBe(2);
    expect(targetResultPanel(panels, "panel-result-3")?.index).toBe(3);
  });

  it("falls back to the last result tab when a system or describe tab is active", () => {
    const panels = appendPanel(
      appendPanel(initialPanels(), createResultPanel(2)),
      createDescribePanel(target("demo_shop", "orders")),
    );
    expect(targetResultPanel(panels, "panel-message")?.index).toBe(2);
    expect(targetResultPanel(panels, "panel-history")?.index).toBe(2);
    expect(
      targetResultPanel(panels, "panel-describe-demo-mysql-demo_shop-orders")?.index,
    ).toBe(2);
  });

  it("has nothing to run into once every result tab is closed", () => {
    const panels = closePanel(initialPanels(), "panel-result-1", "panel-result-1").panels;
    expect(targetResultPanel(panels, "panel-message")).toBeUndefined();
  });
});

describe("appendPanel", () => {
  it("appends dynamic tabs after the fixed system tabs", () => {
    const panels = appendPanel(initialPanels(), createDescribePanel(target("demo_shop", "orders")));
    expect(kinds(panels)).toEqual(["message", "history", "result", "describe"]);
    expect(ids(panels).slice(0, 2)).toEqual(["panel-message", "panel-history"]);
  });

  it("keeps the opening order of dynamic tabs", () => {
    let panels = initialPanels();
    panels = appendPanel(panels, createDescribePanel(target("demo_shop", "orders")));
    panels = appendPanel(panels, createResultPanel(2));
    panels = appendPanel(panels, createDescribePanel(target("demo_shop", "routes")));
    expect(ids(panels)).toEqual([
      "panel-message",
      "panel-history",
      "panel-result-1",
      "panel-describe-demo-mysql-demo_shop-orders",
      "panel-result-2",
      "panel-describe-demo-mysql-demo_shop-routes",
    ]);
  });

  it("does not mutate the input array", () => {
    const panels = initialPanels();
    appendPanel(panels, createResultPanel(2));
    expect(kinds(panels)).toEqual(["message", "history", "result"]);
  });
});

describe("describe tabs", () => {
  it("starts in the loading state with the full path", () => {
    const panel = createDescribePanel(target("demo_shop", "orders"));
    expect(panel.kind).toBe("describe");
    expect(panel.loading).toBe(true);
    expect(panel.data).toBeNull();
    expect(panel.table).toBe("demo-mysql / demo_shop / orders");
  });

  it("matches an already opened table by instance, database and name", () => {
    const panels = appendPanel(initialPanels(), createDescribePanel(target("demo_shop", "orders")));
    expect(describeTab(panels, "demo_shop", "orders").tableName).toBe("orders");
    expect(findDescribePanel(panels, target("demo_shop", "routes"))).toBeUndefined();
    expect(findDescribePanel(panels, target("demo_schedule", "orders"))).toBeUndefined();
    expect(findDescribePanel(panels, target("demo_shop", "orders", "other-mysql"))).toBeUndefined();
  });

  it("uses the bare table name until another database opens the same name", () => {
    const first = appendPanel(initialPanels(), createDescribePanel(target("demo_shop", "orders")));
    expect(describeTitle(first, target("demo_shop", "orders"))).toBe("orders");

    const both = appendPanel(first, createDescribePanel(target("demo_schedule", "orders")));
    expect(describeTitle(both, target("demo_shop", "orders"))).toBe("demo_shop.orders");
    expect(describeTitle(both, target("demo_schedule", "orders"))).toBe("demo_schedule.orders");
    expect(describeTitle(both, target("demo_schedule", "addresses"))).toBe("addresses");
  });

  it("prefixes the instance too when the database name is not enough", () => {
    let panels = appendPanel(initialPanels(), createDescribePanel(target("demo_shop", "orders", "a")));
    panels = appendPanel(panels, createDescribePanel(target("demo_shop", "orders", "b")));
    expect(describeTitle(panels, target("demo_shop", "orders", "a"))).toBe("a.demo_shop.orders");
    expect(describeTitle(panels, target("demo_shop", "orders", "b"))).toBe("b.demo_shop.orders");
  });
});

describe("closePanel", () => {
  it("never closes a system tab", () => {
    const panels = initialPanels();
    const result = closePanel(panels, "panel-result-1", "panel-message");
    expect(ids(result.panels)).toEqual(ids(panels));
    expect(result.activeId).toBe("panel-result-1");
  });

  it("keeps the active tab when another tab is closed", () => {
    const panels = appendPanel(initialPanels(), createResultPanel(2));
    const result = closePanel(panels, "panel-result-1", "panel-result-2");
    expect(ids(result.panels)).toEqual(["panel-message", "panel-history", "panel-result-1"]);
    expect(result.activeId).toBe("panel-result-1");
  });

  it("activates the right neighbour, or the left one at the end", () => {
    const panels = appendPanel(appendPanel(initialPanels(), createResultPanel(2)), createResultPanel(3));
    expect(closePanel(panels, "panel-result-2", "panel-result-2").activeId).toBe("panel-result-3");
    expect(closePanel(panels, "panel-result-3", "panel-result-3").activeId).toBe("panel-result-2");
  });
});

describe("movePanel", () => {
  it("reorders dynamic tabs", () => {
    let panels = appendPanel(initialPanels(), createDescribePanel(target("demo_shop", "orders")));
    panels = appendPanel(panels, createResultPanel(2));
    const moved = movePanel(panels, "panel-result-2", "panel-describe-demo-mysql-demo_shop-orders");
    expect(ids(moved).slice(2)).toEqual([
      "panel-result-1",
      "panel-result-2",
      "panel-describe-demo-mysql-demo_shop-orders",
    ]);
  });

  it("keeps the system tabs pinned", () => {
    const panels = appendPanel(initialPanels(), createResultPanel(2));
    expect(ids(movePanel(panels, "panel-result-2", "panel-message")).slice(0, 2)).toEqual([
      "panel-message",
      "panel-history",
    ]);
    expect(ids(movePanel(panels, "panel-message", "panel-result-2"))).toEqual(ids(panels));
  });
});
