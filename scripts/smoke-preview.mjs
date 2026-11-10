import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { chromium } from "playwright-core";

const preview = resolve("preview/index.html");
if (!existsSync(preview)) {
  console.error("preview/index.html 不存在，先运行 pnpm build && node scripts/make-preview.mjs");
  process.exit(1);
}

const candidates = [
  "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
  "C:/Program Files/Microsoft/Edge/Application/msedge.exe",
  "C:/Program Files/Google/Chrome/Application/chrome.exe",
];
const executablePath = candidates.find((path) => existsSync(path));
if (!executablePath) {
  console.error("找不到 Edge/Chrome 可执行文件");
  process.exit(1);
}

const browser = await chromium.launch({ executablePath, headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const errors = [];
page.on("pageerror", (error) => errors.push(`pageerror: ${error.message}`));
page.on("console", (message) => {
  if (message.type() === "error") errors.push(`console: ${message.text()}`);
});

const checks = {};

await page.goto(pathToFileURL(preview).href, { waitUntil: "load" });
await page.waitForTimeout(1400);

// 首屏 = Archery 控制台 Dashboard
const dashboardText = await page.locator(".app-main").innerText();
checks.dashboardCards = (await page.locator(".stat-card").count()) === 4;
// 公告条挂在 app-main 顶部，所有页面共用同一条动态公告（首页也要有）
checks.dashboardAnnouncement =
  (await page.locator(".app-announcement").count()) === 1 &&
  (await page.locator(".app-announcement").innerText()).includes("演示用公告");
checks.metricWorkflow = dashboardText.includes("SQL 上线工单") && dashboardText.includes("2200");
checks.metricQuery = dashboardText.includes("396");
checks.metricInstances = dashboardText.includes("19");
// 「更多」只出现在有目标列表的两张工单卡上；用户数与实例数没有列表页，不留无效入口
checks.dashboardMoreCards =
  (await page.locator(".stat-card .stat-more").count()) === 2 &&
  (await page.locator(".stat-card", { hasText: "有效平台用户" }).locator(".stat-more").count()) === 0 &&
  (await page.locator(".stat-card", { hasText: "实例数量" }).locator(".stat-more").count()) === 0;
// 少了底栏也不能破坏四张卡等高
checks.dashboardMoreCardsEqualHeight = await page.evaluate(() => {
  const cards = Array.from(document.querySelectorAll(".stat-card"));
  const heights = cards.map((card) => Math.round(card.getBoundingClientRect().height));
  return heights.length === 4 && new Set(heights).size === 1;
});
checks.chartSeries = (await page.locator(".chart-card .chart-svg").count()) >= 2;
// 两张按日统计卡：不再是「等待接口开放」的空态，而是首页同源数据画出的折线/柱状
checks.dashboardTrendPlaceholder = (await page.locator(".card-empty").count()) === 0;
checks.dashboardTrendQuery = await page.evaluate(() => {
  const card = document.querySelectorAll(".trend-card")[0];
  if (!card) return false;
  const labels = Array.from(card.querySelectorAll(".trend-value-label")).map((node) =>
    node.textContent.trim(),
  );
  return (
    (card.textContent || "").includes("检索行数") &&
    labels.length >= 7 &&
    labels.includes("3,246")
  );
});
checks.dashboardTrendRelease = await page.evaluate(() => {
  const card = document.querySelectorAll(".trend-card")[1];
  if (!card) return false;
  const labels = Array.from(card.querySelectorAll(".trend-value-label")).map((node) =>
    node.textContent.trim(),
  );
  return (
    card.querySelectorAll("rect").length >= 7 && labels.includes("16") && labels.includes("11")
  );
});
// 图表用 Archery 首页统计（16/2/1）而不是当前账号可见实例清单（14/3/1/1）
checks.dashboardUsesHomepageStats = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll(".pie-legend li")).map((item) => {
    const name = item.querySelector(".pie-legend-name")?.textContent?.trim() ?? "";
    const value = item.querySelector(".pie-legend-value")?.textContent?.trim() ?? "";
    return `${name}=${value}`;
  });
  const total = document.querySelector(".pie-total")?.textContent?.trim();
  return rows.join(",") === "mysql=16,redis=2,doris=1" && total === "19";
});
checks.sideNav = (await page.locator(".side-nav .nav-item").count()) >= 6;
// 品牌取 Archery 侧 navbar-brand（mock 的 custom_title_suffix），而不是 DBX 连接名；
// 逐字比对，避免 custom_title_suffix 自带括号时再包一层出现双括号。
checks.headerTitle =
  (await page.locator(".app-brand").innerText()).replace(/\s+/g, "") ===
  "Archery(预览动态环境)";
checks.greeting = dashboardText.includes("你好") || (await page.locator(".app-greeting").count()) > 0;
await page.screenshot({ path: "preview/smoke-dashboard.png", fullPage: true });

// 统计时间范围：默认显示 Archery 回显的窗口，切换预设后重新取数重画
const rangeTrigger = page.locator(".trend-toolbar .date-range-trigger");
checks.dashboardRangeControl =
  (await page.locator(".trend-toolbar .date-range").count()) === 1 &&
  (await rangeTrigger.count()) === 1 &&
  (await rangeTrigger.innerText()).includes("2026-10-29") &&
  // 页面解析不到区间文案时也不能退化成「全部」
  !(await rangeTrigger.innerText()).includes("全部");
checks.dashboardRangePresets = false;
checks.dashboardRangeApplied = false;
if (checks.dashboardRangeControl) {
  const ticksBefore = await page.locator(".trend-card").first().locator(".axis-label").count();
  const labelsBefore = (
    await page.locator(".trend-card").first().locator(".axis-label").allTextContents()
  ).join(",");
  const labelBefore = (await rangeTrigger.innerText()).trim();
  await rangeTrigger.click();
  // 与 Archery 首页一致：没有「全部」，默认停在最近 7 日
  const rangeItems = (await page.locator(".trend-toolbar .date-range-item").allInnerTexts()).map(
    (text) => text.trim(),
  );
  const activeItem = (await page.locator(".trend-toolbar .date-range-item.active").innerText()).trim();
  checks.dashboardRangePresets =
    rangeItems.length === 8 && !rangeItems.includes("全部") && activeItem === "最近7日";
  await page.locator(".trend-toolbar .date-range-item", { hasText: "最近30日" }).click();
  await page.waitForTimeout(700);
  const ticksAfter = await page.locator(".trend-card").first().locator(".axis-label").count();
  const labelsAfter = (
    await page.locator(".trend-card").first().locator(".axis-label").allTextContents()
  ).join(",");
  const labelAfter = (await rangeTrigger.innerText()).trim();
  // 刻度按跨度抽稀后不再「一天一个」：只要求两档都在上限内且标签整体变了。
  // 卡片副标题必须等于所选区间：证明数据是按区间重新取回来的，而不只是刻度变多
  const subtitleAfter = (await page.locator(".trend-card .card-subtitle").first().innerText()).trim();
  checks.dashboardRangeApplied =
    ticksBefore <= 8 &&
    ticksAfter <= 8 &&
    labelsAfter !== labelsBefore &&
    labelAfter !== labelBefore &&
    subtitleAfter === labelAfter;
  await page.screenshot({ path: "preview/smoke-dashboard-range.png", fullPage: true });
}

// 统计区间超过 30 天：两张按日图改上下排列，各铺满一行（mock 上限 62 天，足够越过阈值）
checks.dashboardTrendStacked = false;
await rangeTrigger.click();
await page.locator(".trend-toolbar .date-range-item", { hasText: "最近365日" }).click();
await page.waitForTimeout(700);
checks.dashboardTrendStacked = await page.evaluate(() => {
  const panels = document.querySelector(".stat-panels");
  const cards = Array.from(document.querySelectorAll(".trend-card")).map((card) =>
    card.getBoundingClientRect(),
  );
  return Boolean(
    panels?.classList.contains("is-stacked") &&
      cards.length === 2 &&
      Math.round(cards[0].left) === Math.round(cards[1].left) &&
      Math.round(cards[0].width) === Math.round(cards[1].width) &&
      cards[1].top > cards[0].top,
  );
});
await page.screenshot({ path: "preview/smoke-dashboard-stacked.png", fullPage: true });

// 铃铛右侧显示待办总数（Archery base.html 的 {{ todo }}），数字排在图标右侧
checks.headerBellCount =
  (await page.locator(".header-bell-count").count()) === 1 &&
  (await page.locator(".header-bell-count").innerText()).trim() === "2";
checks.headerBellCountRight = await page.evaluate(() => {
  const icon = document.querySelector(".header-bell .nav-icon");
  const count = document.querySelector(".header-bell-count");
  if (!icon || !count) return false;
  const box = icon.getBoundingClientRect();
  const text = count.getBoundingClientRect();
  return text.left >= box.right - 1 && text.top < box.bottom && text.bottom > box.top;
});

// 顶栏铃铛 → 待办列表（Archery /workflow/ 页面）：7 列表格、申请类型筛选与审核入口
await page.locator(".app-header-right .header-icon").first().click();
await page.waitForSelector(".todo-table", { timeout: 8000 });
checks.todoTitle = (await page.locator(".todo-title").innerText()).trim() === "待办列表";
checks.todoColumns = (await page.locator(".todo-table thead th").count()) === 7;
checks.todoRows = (await page.locator(".todo-table tbody tr").count()) === 2;
checks.todoStatus =
  (await page.locator(".todo-table .privilege-status.is-info").count()) === 2 &&
  (await page.locator(".todo-table tbody").innerText()).includes("待审核");
checks.todoRecords = (await page.locator(".pager").innerText()).includes(
  "显示第 1 到第 2 条记录，总共 2 条记录",
);
checks.todoTypeOptions = (await page.locator(".todo-type-filter option").count()) === 4;
await page.screenshot({ path: "preview/smoke-todo.png", fullPage: true });

// 申请类型筛选：只看 SQL 上线申请
await page.locator(".todo-type-filter").selectOption("2");
await page.waitForTimeout(500);
checks.todoTypeFilter =
  (await page.locator(".todo-table tbody tr").count()) === 1 &&
  (await page.locator(".todo-table tbody").innerText()).includes("SQL上线申请");

// 审核入口：audit_id → 业务工单号，进入插件内的 SQL 上线详情页
await page.locator(".todo-table tbody tr").first().locator(".btn-quiet").click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
checks.todoAuditDetail =
  (await page.locator(".workflow-detail-panel").innerText()).includes("DDL - 车辆表结构修改") &&
  (await page.locator(".workflow-actions .btn-success").count()) === 1;
await page.locator(".privilege-back").click();
await page.waitForSelector(".todo-table", { timeout: 8000 });
checks.todoBack = (await page.locator(".todo-table thead th").count()) === 7;

// 查询权限待办：同样解析出业务工单号后进插件内的权限工单详情页
await page.locator(".todo-type-filter").selectOption("0");
await page.waitForTimeout(500);
await page
  .locator(".todo-table tbody tr", { hasText: "查询数据" })
  .locator(".btn-quiet")
  .click();
await page.waitForSelector(".privilege-detail-panel", { timeout: 8000 });
checks.todoPrivilegeDetail = (
  await page.locator(".privilege-detail-panel").innerText()
).includes("查询数据");
await page.locator(".privilege-back").click();
await page.waitForSelector(".todo-table", { timeout: 8000 });

// 重复点击铃铛：停在筛选态时再点一次，重新进入待办首页（筛选与页码归位）
await page.locator(".todo-type-filter").selectOption("2");
await page.waitForTimeout(500);
await page.locator(".app-header-right .header-icon").first().click();
await page.waitForSelector(".todo-table", { timeout: 3000 }).catch(() => null);
await page.waitForTimeout(400);
checks.todoBellReenter =
  (await page.locator(".todo-table tbody tr").count()) === 2 &&
  (await page.locator(".todo-table tbody").innerText()).includes("查询权限申请");

// 停在详情页时点铃铛：同样回到待办列表而不是留在工单详情
await page.locator(".todo-table tbody tr").first().locator(".btn-quiet").click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
await page.locator(".app-header-right .header-icon").first().click();
await page.waitForSelector(".todo-table", { timeout: 3000 }).catch(() => null);
await page.waitForTimeout(400);
checks.todoBellFromDetail = (await page.locator(".workflow-grid").count()) === 0;
// 兜底回到列表，避免后续步骤依赖失败现场
if ((await page.locator(".todo-table").count()) === 0) {
  await page.locator(".privilege-back").click();
  await page.waitForSelector(".todo-table", { timeout: 8000 });
}

// 切到 SQL 查询工作台：父项只展开，二级菜单才导航
checks.sidebarChildren = (await page.locator(".side-nav .nav-child").count()) === 3;
await page.locator(".side-nav .nav-child", { hasText: "在线查询" }).click();
await page.waitForTimeout(600);
// 记录每次 archery/query 真正发出去的 SQL：用于核对选区 / 光标 / 快捷键三条入口
await page.evaluate(() => {
  window.__ranSql = [];
  const plugin = window.dbxPlugin;
  const original = plugin.invoke;
  plugin.invoke = (method, params) => {
    if (method === "archery/query") window.__ranSql.push(params?.sqlContent ?? "");
    return original(method, params);
  };
});
checks.editor = (await page.locator(".cm-editor").count()) > 0;
checks.instanceTreeHidden = (await page.locator(".tree-row").count()) === 0;
checks.configPane = (await page.locator(".query-config").count()) === 1;
checks.configActions = (await page.locator(".config-actions .btn").count()) === 3;

// 实例 / 库改成可搜索下拉（原生 select 在宿主 iframe 里首次点击常常只拿到焦点不弹面板，
// 候选多时一次铺开也会卡）：点一下就开、输入收窄、选中后库跟着可用
const targetPickers = page.locator(".query-config .picker-trigger");
checks.targetPickers = (await targetPickers.count()) === 2;
await targetPickers.nth(0).click();
await page.waitForTimeout(400);
checks.instancePickerOpens = (await page.locator(".picker-pop").count()) === 1;
checks.instancePickerOptions = (await page.locator(".picker-item").count()) > 1;
await page.locator(".picker-search-input").fill("demo-mysql");
await page.waitForTimeout(300);
checks.instancePickerFilter = await page.evaluate(() => {
  const names = Array.from(document.querySelectorAll(".picker-item-name")).map((node) =>
    node.textContent.trim(),
  );
  return names.length > 0 && names.every((name) => name.includes("demo-mysql"));
});
await page.locator(".picker-item").first().click();
await page.waitForTimeout(900);
checks.instancePicked = (await targetPickers.nth(0).innerText()).includes("demo-mysql");
await targetPickers.nth(1).click();
await page.waitForTimeout(700);
checks.dbPickerOptions = (await page.locator(".picker-item").count()) > 0;
await page.locator(".picker-item", { hasText: "demo_shop" }).first().click();
await page.waitForTimeout(900);
checks.dbPicked = (await targetPickers.nth(1).innerText()).includes("demo_shop");
checks.tableReadyAfterPick = await page.locator(".table-picker-input").isEnabled();
await page.screenshot({ path: "preview/smoke-target-picker.png", fullPage: true });
// 「行数」跟实例 / 库 / 表同组，排在表下面；底部执行区只剩按钮与提示
checks.configLimitGrouped = await page.evaluate(() => {
  const section = document.querySelector(".query-config .config-section");
  const run = document.querySelector(".query-config .config-run");
  const limit = document.querySelector(".query-config .config-field-limit");
  const picker = section?.querySelector(".table-picker");
  if (!section || !run || !limit || !picker) return false;
  const afterPicker = !!(picker.compareDocumentPosition(limit) & Node.DOCUMENT_POSITION_FOLLOWING);
  return section.contains(limit) && !run.contains(limit) && afterPicker;
});
checks.favorites = (await page.locator(".panel-heading select option").count()) > 1;
checks.announcement =
  (await page.locator(".app-announcement").count()) === 1 &&
  (await page.locator(".app-announcement").innerText()).includes("演示用公告");
await page.screenshot({ path: "preview/smoke-query.png", fullPage: true });

// 实例资源抽屉：默认收起，按需打开后仍可展开库 → 表
await page.locator(".panel-heading .btn", { hasText: "实例资源" }).click();
await page.waitForTimeout(600);
checks.instanceTree = (await page.locator(".tree-row").count()) > 0;
await page.locator(".tree-row", { hasText: "demo_shop" }).first().click();
await page.waitForTimeout(800);
checks.tables = (await page.locator(".tree-row", { hasText: "orders" }).count()) > 0;

// 实例树搜索：库名、表名、无命中三级都要生效（旧实现只按实例名过滤，搜库/搜表恒为空）
const treeSearch = page.locator(".sidebar .search");
await treeSearch.fill("demo_shop");
await page.waitForTimeout(1200);
checks.sidebarSearchDb =
  (await page.locator('.tree-row[title="demo-mysql"]').count()) === 1 &&
  (await page.locator('.tree-row[title="demo_shop"]').count()) > 0 &&
  (await page.locator('.tree-row[title="demo_report"]').count()) === 0;
await treeSearch.fill("orders");
await page.waitForTimeout(1500);
checks.sidebarSearchTable = (await page.locator('.tree-row[title^="orders "]').count()) > 0;
await treeSearch.fill("zzzz-no-such");
await page.waitForTimeout(1200);
checks.sidebarSearchEmpty = (await page.locator(".sidebar .tree").innerText()).includes(
  "没有匹配的实例 / 库 / 表",
);
await page.screenshot({ path: "preview/smoke-sidebar-search.png" });
await treeSearch.fill("");
await page.waitForTimeout(500);

// 输入 SQL 并执行
await page.locator(".cm-content").click();
await page.keyboard.type("select * from orders order by id desc limit 100");
await page.locator(".config-actions .btn-success").click();
await page.waitForSelector(".data-table tbody tr", { timeout: 8000 });
checks.rows = (await page.locator(".data-table tbody tr").count()) > 0;
checks.resultText = (await page.locator(".data-table tbody tr").first().innerText()).includes("SO2026");

// 编辑器复制必须走宿主剪贴板桥：DBX 插件 UI 在 sandbox=allow-scripts 的 opaque-origin
// iframe 里，原生复制会被静默丢弃，只有 dbxPlugin.copy 能真正写入系统剪贴板。
// 这里断言 copy 桥被调用且收到完整选区，防止组件退回「只依赖原生复制」的写法。
await page.evaluate(() => {
  window.__copyCalls = [];
  const plugin = window.dbxPlugin;
  const original = plugin.copy;
  window.__restoreCopy = () => {
    plugin.copy = original;
  };
  plugin.copy = (text) => {
    window.__copyCalls.push(text);
    return Promise.resolve();
  };
});
// 执行按钮会抢走焦点，复制前必须重新聚焦编辑器。
await page.locator(".cm-content").click();
await page.keyboard.press("Control+A");
await page.keyboard.press("Control+C");
await page.waitForTimeout(200);
// 折叠选区后再触发一次 copy：空选区不应调用宿主桥，保持原生行为。
await page.locator(".cm-content").click();
const emptySelectionCopyCalls = await page.evaluate(() => {
  const before = window.__copyCalls.length;
  document
    .querySelector(".cm-content")
    .dispatchEvent(new ClipboardEvent("copy", { clipboardData: new DataTransfer(), bubbles: true }));
  return window.__copyCalls.length - before;
});
const editorCopyCalls = await page.evaluate(() => {
  window.__restoreCopy?.();
  return window.__copyCalls.slice();
});
checks.editorCopyUsesHostBridge =
  editorCopyCalls.length === 1 &&
  editorCopyCalls[0] === "select * from orders order by id desc limit 100";
checks.editorCopyEmptySelectionSkipsBridge = emptySelectionCopyCalls === 0;

// 列宽拖拽：向右拖第 2 个数据列的表头手柄
const resizedHeader = page.locator(".result-panel .data-table thead th").nth(2);
const widthBefore = (await resizedHeader.boundingBox()).width;
const resizeHandle = page.locator(".result-panel .data-table .col-resize").nth(1);
const handleBox = await resizeHandle.boundingBox();
await page.mouse.move(handleBox.x + 2, handleBox.y + handleBox.height / 2);
await page.mouse.down();
await page.mouse.move(handleBox.x + 80, handleBox.y + handleBox.height / 2, { steps: 4 });
await page.mouse.up();
await page.waitForTimeout(200);
const widthAfter = (await resizedHeader.boundingBox()).width;
checks.columnResize = widthAfter > widthBefore + 40;
await page.screenshot({ path: "preview/smoke-result.png", fullPage: true });

// 美化：关键字大写并换行
await page.locator(".config-actions .btn", { hasText: "美化" }).click();
await page.waitForTimeout(400);
const beautified = await page.locator(".cm-content").innerText();
checks.beautify = beautified.includes("SELECT") && beautified.includes("\n");

// 执行计划：结果面板给出提示
await page.locator(".config-actions .btn", { hasText: "执行计划" }).click();
await page.waitForSelector(".data-table tbody tr", { timeout: 8000 });
checks.explain = (await page.locator(".result-panel").innerText()).includes("执行计划已生成");
await page.screenshot({ path: "preview/smoke-config.png", fullPage: true });

// 结果页签不再随执行自动新增：查询与执行计划都落回同一条「结果 1」，系统页签钉在最前且不可关
const resultTabLabels = await page.locator(".result-tabs .nav-tab").allInnerTexts();
checks.resultTabsNoAutoAdd =
  resultTabLabels.filter((text) => text.includes("结果")).length === 1 &&
  resultTabLabels[0].includes("消息") &&
  resultTabLabels[1].includes("查询历史");
checks.resultTabClosable =
  (await page.locator(".result-tabs .nav-tab .nav-tab-close").count()) === 1;

// 页签右侧的 +/−：手动开一条空结果、再关掉；停在系统页签上时「−」为禁用态
const resultTabActions = page.locator(".result-tab-actions .tab-action");
checks.resultTabActions = (await resultTabActions.count()) === 2;
await resultTabActions.first().click();
await page.waitForTimeout(250);
checks.resultTabAdd =
  (await page.locator(".result-tabs .nav-tab", { hasText: "结果 2" }).count()) === 1 &&
  (await page.locator(".result-tabs .nav-tab.active").innerText()).includes("结果 2");
await resultTabActions.last().click();
await page.waitForTimeout(250);
checks.resultTabRemove =
  (await page.locator(".result-tabs .nav-tab", { hasText: "结果 2" }).count()) === 0 &&
  (await page.locator(".result-tabs .nav-tab.active").innerText()).includes("结果 1");
await page.locator(".result-tabs .nav-tab", { hasText: "消息" }).click();
await page.waitForTimeout(200);
checks.resultTabRemoveDisabled = await resultTabActions.last().isDisabled();
await page.locator(".result-tabs .nav-tab", { hasText: "结果 1" }).click();
await page.waitForTimeout(200);

// 查看表结构：表名搜索下拉 → 结果面板直接出 DDL
await page.locator(".table-picker-input").click();
await page.waitForTimeout(600);
checks.tableOptions = (await page.locator(".table-option").count()) > 0;
checks.tableDropdown =
  (await page.locator(".table-options").boundingBox()).height > 100 &&
  (await page.locator(".table-option.active").count()) === 1;
await page.screenshot({ path: "preview/smoke-tables.png", fullPage: true });
await page.locator(".table-option", { hasText: "orders" }).first().click();
await page.waitForSelector(".ddl-body", { timeout: 6000 });
checks.structurePanel = (await page.locator(".result-panel").innerText()).includes("CREATE TABLE");
checks.ddlTab = (await page.locator(".nav-tab", { hasText: "orders" }).count()) > 0;
checks.ddlMeta = (await page.locator(".result-panel").innerText()).includes("查询时间");

// 表下拉：点一下就开。选完表 / Escape 关闭后输入框仍是焦点态，再点一次必须能重新打开
// （旧实现只监听 focus，这两种情况下 focus 不会再触发，表现为「得先点别处再点回来」）
checks.tableCaret = (await page.locator(".table-picker-caret").count()) === 1;
const caretClosed = await page
  .locator(".table-picker-caret")
  .evaluate((element) => getComputedStyle(element).transform);
await page.locator(".table-picker-input").click();
await page.waitForTimeout(500);
checks.tableReopenAfterPick =
  (await page.locator(".table-options").count()) === 1 &&
  (await page.locator(".table-option").count()) > 0;
const caretOpen = await page
  .locator(".table-picker-caret")
  .evaluate((element) => getComputedStyle(element).transform);
checks.tableCaretRotates = caretClosed !== caretOpen;
await page.keyboard.press("Escape");
await page.waitForTimeout(250);
checks.tableEscapeCloses = (await page.locator(".table-options").count()) === 0;
await page.locator(".table-picker-input").click();
await page.waitForTimeout(400);
checks.tableReopenAfterEscape = (await page.locator(".table-options").count()) === 1;
await page.keyboard.press("Escape");
await page.waitForTimeout(250);

// 查询历史：8 列表格 + 实例/库筛选
await page.locator(".nav-tab", { hasText: "查询历史" }).click();
await page.waitForTimeout(600);
checks.historyColumns = (await page.locator(".history-table thead th").count()) === 8;
checks.historyRows = (await page.locator(".history-table tbody tr").count()) > 0;
checks.historyFilters = (await page.locator(".history-filter").count()) === 2;
// 行内视觉：语句类型色条、用户头像、数据库标签都按行渲染
checks.historyRowKind =
  (await page.locator(".history-row.is-read").count()) === 10 &&
  (await page.locator(".history-kind-mark").count()) === 10;
checks.historyCellChips =
  (await page.locator(".history-table .history-db").count()) === 10 &&
  (await page.locator(".history-table .history-avatar").count()) === 10;

// 分页：Archery 口径的记录区间 + 每页条数 + 页码窗口
const historyRowCount = () =>
  page.locator(".history-table tbody tr:not(.history-detail)").count();
const historyPanelHeight = () =>
  page
    .locator(".result-panel")
    .evaluate((element) => Math.round(element.getBoundingClientRect().height));
// 默认每页 10 条：记录区间、页数、下拉选中项都按 10 走
checks.historyPageSizeDefault =
  (await page.locator(".history-pager-size select").inputValue()) === "10";
checks.historyPagerInfo = (await page.locator(".history-pager-info").innerText()).includes(
  "显示第 1 到第 10 条记录，总共 26 条记录",
);
checks.historyPageSizeOptions = (
  await page.locator(".history-pager-size option").allInnerTexts()
).map((text) => text.trim()).join(",") === "10,20,30,50,100";
checks.historyPageNumbers = (
  await page.locator(".history-page-num").allInnerTexts()
).map((text) => text.trim()).join(",") === "1,2,3";
checks.historyPageActive =
  (await page.locator(".history-page-num.is-active").innerText()).trim() === "1";
checks.historyPageSizeRows = (await historyRowCount()) === 10;
// 切到每页 20 条：行数与页数跟着变，面板高度随行数变高（不是固定拉满）
const historyHeightAt10 = await historyPanelHeight();
await page.locator(".history-pager-size select").selectOption("20");
await page.waitForTimeout(600);
checks.historyPageSizeRowsSwitch = (await historyRowCount()) === 20;
checks.historyPageSizePages = (await page.locator(".history-page-num").count()) === 2;
checks.historyHeightFollowsRows = (await historyPanelHeight()) >= historyHeightAt10 + 100;
// 翻页：第 2 页取回剩余记录，记录区间与高亮页码同步
await page.locator(".history-page-num", { hasText: "2" }).first().click();
await page.waitForTimeout(600);
checks.historyPageNav =
  (await page.locator(".history-pager-info").innerText()).includes("第 21 到第 26 条") &&
  (await page.locator(".history-page-num.is-active").innerText()).trim() === "2";
await page.locator(".history-pager-size select").selectOption("10");
await page.waitForTimeout(600);
checks.historyPageReset = (await historyRowCount()) === 10;

// 翻页不复原滚动条：切页时表格保持挂载，滚动位置原地不动（旧实现被 loading 占位挤回顶部）
await page.locator(".query-column").evaluate((el) => {
  el.scrollTop = 140;
});
await page.waitForTimeout(200);
const historyScrollBefore = await page
  .locator(".query-column")
  .evaluate((el) => Math.round(el.scrollTop));
await page.evaluate(() => document.querySelectorAll(".history-page-num")[1].click());
await page.waitForTimeout(700);
checks.historyPageKeepsScroll = await page
  .locator(".query-column")
  .evaluate((el, before) => Math.round(el.scrollTop) === before, historyScrollBefore);
checks.historyPageKeepsScrollActive =
  (await page.locator(".history-page-num.is-active").innerText()).trim() === "2";
await page.evaluate(() => document.querySelectorAll(".history-page-num")[0].click());
await page.waitForTimeout(600);
await page.screenshot({ path: "preview/smoke-history-pager.png", fullPage: true });

// 行内展开
await page.locator(".history-table .op-btn").first().click();
await page.waitForTimeout(200);
checks.historyExpand = (await page.locator(".history-detail").count()) === 1;
checks.historyDetailCard =
  (await page.locator(".history-detail-card .history-kind-badge").count()) === 1 &&
  (await page.locator(".history-detail-meta .history-meta-item").count()) === 5;
await page.screenshot({ path: "preview/smoke-history.png", fullPage: true });

// 双击资源树的表名：在当前查询页打开该表的 DDL tab
await page.locator(".tree-row", { hasText: "addresses" }).first().dblclick();
await page.waitForTimeout(800);
checks.describeDdl = (await page.locator(".ddl-body").innerText()).includes("CREATE TABLE");
checks.describeTab =
  (await page.locator(".nav-tab", { hasText: "addresses" }).count()) === 1;
// DDL 上方不再挂「Create Table」小标题，直接给建表语句
checks.describeNoDdlTitle = (await page.locator(".ddl-title").count()) === 0;
await page.screenshot({ path: "preview/smoke-describe.png", fullPage: true });

// 表结构 tab 多开：orders 与 addresses 各一条；同一张表再点一次原地刷新，不新增
checks.describeTabsMulti =
  (await page.locator(".result-tabs .nav-tab", { hasText: "orders" }).count()) === 1 &&
  (await page.locator(".result-tabs .nav-tab", { hasText: "addresses" }).count()) === 1;
await page.locator('.tree-row[title^="addresses "]').first().dblclick();
await page.waitForTimeout(700);
checks.describeTabReuse =
  (await page.locator(".result-tabs .nav-tab", { hasText: "addresses" }).count()) === 1 &&
  (await page.locator(".result-tabs .nav-tab", { hasText: "orders" }).count()) === 1;

// 表结构 tab：面板高度由内容决定 —— 短 DDL 就地收缩不拉满，长 DDL 继续长高、滚动交给整个查询列
const describeLayout = () =>
  page.evaluate(() => {
    const box = (selector) => {
      const el = document.querySelector(selector);
      if (!el) return null;
      const rect = el.getBoundingClientRect();
      return {
        top: Math.round(rect.top),
        bottom: Math.round(rect.bottom),
        height: Math.round(rect.height),
        clientHeight: el.clientHeight,
        scrollHeight: el.scrollHeight,
      };
    };
    const column = document.querySelector(".query-column");
    const columnRect = column.getBoundingClientRect();
    return {
      column: {
        top: Math.round(columnRect.top),
        bottom: Math.round(columnRect.bottom),
        clientHeight: column.clientHeight,
        scrollHeight: column.scrollHeight,
      },
      editor: box(".query-panel"),
      panel: box(".result-panel"),
      wrap: box(".ddl-wrap"),
      ddl: box(".ddl-body"),
    };
  });

// 4 列的 routes：内容明显矮于结果面板，面板应收缩，底部空白交给列容器
await page.locator('.tree-row[title^="routes "]').first().dblclick();
await page.waitForTimeout(700);
const shortDescribe = await describeLayout();
checks.describeShortNotStretched =
  shortDescribe.panel.bottom <= shortDescribe.column.bottom - 20 &&
  shortDescribe.column.scrollHeight <= shortDescribe.column.clientHeight + 1;
checks.describeShortNoInnerScroll =
  shortDescribe.wrap.scrollHeight <= shortDescribe.wrap.clientHeight + 1;
checks.describeShortDdlFullyVisible =
  shortDescribe.ddl.scrollHeight <= shortDescribe.ddl.clientHeight + 1;

// 30 列的 bs_vehicle_info：内容高于可视区，面板继续长高，溢出交给整列滚动
await page.locator('.tree-row[title^="bs_vehicle_info "]').first().dblclick();
await page.waitForTimeout(700);
const longDescribe = await describeLayout();
checks.describeLongPanelGrows = longDescribe.panel.height > shortDescribe.panel.height + 200;
checks.describeLongColumnScrolls =
  longDescribe.column.scrollHeight > longDescribe.column.clientHeight + 200;
checks.describeLongNoInnerScroll =
  longDescribe.wrap.scrollHeight <= longDescribe.wrap.clientHeight + 1;
checks.describeLongEditorIntact =
  Math.abs(longDescribe.editor.height - shortDescribe.editor.height) <= 1;
await page.screenshot({ path: "preview/smoke-describe-tall.png", fullPage: true });

// 无 DDL 的表：回退到列清单表格，高度同样跟随内容
await page.locator('.tree-row[title^="legacy_import_log "]').first().dblclick();
await page.waitForTimeout(700);
const fallbackDescribe = await page.evaluate(() => {
  const wrap = document.querySelector(".result-body .grid-wrap");
  const panel = document.querySelector(".result-panel");
  const column = document.querySelector(".query-column");
  return {
    rows: wrap ? wrap.querySelectorAll("tbody tr").length : 0,
    fits: wrap ? wrap.scrollHeight <= wrap.clientHeight + 1 : false,
    notStretched:
      Math.round(panel.getBoundingClientRect().bottom) <=
      Math.round(column.getBoundingClientRect().bottom) - 20,
  };
});
checks.describeFallbackGridFits =
  fallbackDescribe.rows === 4 && fallbackDescribe.fits && fallbackDescribe.notStretched;

// 关闭动态 tab：× 只出现在结果 / 表结构上，关掉 orders 后不再出现
const ordersTab = page.locator(".result-tabs .nav-tab", { hasText: "orders" }).first();
await ordersTab.locator(".nav-tab-close").click();
await page.waitForTimeout(300);
checks.describeTabClose =
  (await page.locator(".result-tabs .nav-tab", { hasText: "orders" }).count()) === 0;

// 拖拽重排：把最后一条动态 tab 拖到最前，系统 tab 仍钉在原位
const dynamicBefore = await page.locator(".result-tabs .nav-tab.is-dynamic").allInnerTexts();
await page.evaluate(() => {
  const tabs = Array.from(document.querySelectorAll(".result-tabs .nav-tab.is-dynamic"));
  const source = tabs[tabs.length - 1];
  const target = tabs[0];
  const data = new DataTransfer();
  source.dispatchEvent(new DragEvent("dragstart", { dataTransfer: data, bubbles: true }));
  target.dispatchEvent(
    new DragEvent("dragover", { dataTransfer: data, bubbles: true, cancelable: true }),
  );
  target.dispatchEvent(new DragEvent("drop", { dataTransfer: data, bubbles: true }));
});
await page.waitForTimeout(300);
const dynamicAfter = await page.locator(".result-tabs .nav-tab.is-dynamic").allInnerTexts();
const systemAfter = await page.locator(".result-tabs .nav-tab:not(.is-dynamic)").allInnerTexts();
checks.panelTabReorder =
  dynamicAfter.length === dynamicBefore.length &&
  dynamicAfter[0] === dynamicBefore[dynamicBefore.length - 1] &&
  systemAfter[0].includes("消息") &&
  systemAfter[1].includes("查询历史");

// 切回结果 tab：列表高度跟每页条数走，溢出交给整个查询列滚动（与查询历史页签同口径）
await page.locator(".result-tabs .nav-tab", { hasText: "结果 1" }).click();
await page.waitForTimeout(400);
const backToResult = await page.evaluate(() => {
  const column = document.querySelector(".query-column");
  const panel = document.querySelector(".result-panel");
  return {
    overflow: getComputedStyle(column).overflowY,
    scrollTop: column.scrollTop,
    fills:
      Math.abs(panel.getBoundingClientRect().bottom - column.getBoundingClientRect().bottom) <= 1,
    pager: Boolean(document.querySelector(".result-pager")),
  };
});
checks.describeBackToResult =
  backToResult.overflow === "auto" && backToResult.pager && !backToResult.fills;

// 结果分页：候选上限跟着查询目标行数（默认 100），版式与查询历史页签一致
const resultRowCount = () => page.locator(".result-panel .data-table tbody tr").count();
const resultPanelHeight = () =>
  page
    .locator(".result-panel")
    .evaluate((element) => Math.round(element.getBoundingClientRect().height));
checks.resultPagerOptions =
  (await page.locator(".result-pager option").allInnerTexts())
    .map((text) => text.trim())
    .join(",") === "30,50,100";
checks.resultPagerInfo = (await page.locator(".result-pager .history-pager-info").innerText()).includes(
  "显示第 1 到第 30 条记录，总共 137 条记录",
);
checks.resultPagerRows = (await resultRowCount()) === 30;
checks.resultPagerPages = (await page.locator(".result-pager .history-page-num").count()) === 5;
const resultHeightAt30 = await resultPanelHeight();
await page.locator(".result-pager select").selectOption("50");
await page.waitForTimeout(300);
checks.resultPageSizeRows = (await resultRowCount()) === 50;
checks.resultHeightFollowsPageSize = (await resultPanelHeight()) > resultHeightAt30 + 500;
// 翻页：第 2 页取回剩余记录，记录区间与高亮页码同步
await page.locator(".result-pager .history-page-num", { hasText: "2" }).click();
await page.waitForTimeout(300);
checks.resultPageNav =
  (await page.locator(".result-pager .history-pager-info").innerText()).includes(
    "显示第 51 到第 100 条记录",
  ) && (await page.locator(".result-pager .history-page-num.is-active").innerText()).trim() === "2";
await page.locator(".result-pager select").selectOption("30");
await page.waitForTimeout(300);

// 结果搜索：输入即按可见列过滤，命中片段高亮、计数与原始行号保留；Esc 清空
const resultSearch = page.locator(".result-search");
checks.resultSearchBox = (await resultSearch.count()) === 1;
await resultSearch.fill("paid");
await page.waitForTimeout(300);
checks.resultSearchFilters = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll(".result-panel .data-table tbody tr"));
  return (
    rows.length === 30 &&
    rows.every((row) => (row.textContent || "").toLowerCase().includes("paid"))
  );
});
const resultSearchCount = await page.locator(".result-search-count").innerText();
checks.resultSearchCount = resultSearchCount.includes("34") && resultSearchCount.includes("137");
checks.resultSearchHighlight = (await page.locator(".result-panel mark.result-match").count()) > 0;
checks.resultSearchRowNumber =
  (await page.locator(".result-panel .data-table tbody .row-index").first().innerText()).trim() ===
  "4";
checks.resultSearchPager = (
  await page.locator(".result-pager .history-pager-info").innerText()
).includes("匹配记录");
await page.screenshot({ path: "preview/smoke-result-search.png", fullPage: true });

await resultSearch.fill("zzz-no-hit");
await page.waitForTimeout(300);
checks.resultSearchEmpty = (await page.locator(".result-no-match").count()) === 1;
checks.resultSearchExportDisabled = await page
  .locator(".panel-toolbar .btn", { hasText: "导出 Excel" })
  .isDisabled();
await resultSearch.press("Escape");
await page.waitForTimeout(300);
checks.resultSearchEscape =
  (await resultSearch.inputValue()) === "" &&
  (await page.locator(".result-panel .data-table tbody tr").count()) === 30;

await page.locator(".result-pager .history-pager-info").click();
await page.keyboard.press("Control+f");
await page.waitForTimeout(150);
checks.resultSearchShortcut = await resultSearch.evaluate(
  (element) => document.activeElement === element,
);
await resultSearch.press("Escape");
await page.waitForTimeout(150);

// 选区执行（v0.2.40）：Ctrl+Enter 真正执行；选中多条逐条顺序跑，各占一条结果页签
await page.evaluate(() => window.__ranSql.splice(0));
await page.locator(".cm-content").click();
await page.keyboard.press("Control+A");
await page.keyboard.type("select * from orders limit 1;\nselect * from addresses limit 1;");
await page.waitForTimeout(300);
const tabsBeforeSelection = await page.locator(".result-tabs .nav-tab").count();
await page.keyboard.press("Control+Home");
await page.keyboard.press("Shift+ArrowDown");
await page.keyboard.press("Shift+ArrowDown");
await page.keyboard.press("Control+Enter");
await page.waitForTimeout(1800);
const ranSelection = await page.evaluate(() => window.__ranSql.splice(0));
checks.selectionRunsStatements =
  ranSelection.length === 2 &&
  ranSelection[0] === "select * from orders limit 1" &&
  ranSelection[1] === "select * from addresses limit 1";
checks.ctrlEnterNoBlankLine =
  (await page.locator(".cm-content").innerText()).split("\n").length === 2;
checks.selectionTabPerStatement =
  (await page.locator(".result-tabs .nav-tab").count()) === tabsBeforeSelection + 1;
checks.selectionHintPerTab = (
  await page.locator(".result-panel").innerText()
).includes("已执行第 2 / 2 条语句");
await page.screenshot({ path: "preview/smoke-selection.png", fullPage: true });

// 无选区：仍是「光标所在语句」一条，并给出第 N / M 条提示
await page.evaluate(() => window.__ranSql.splice(0));
await page.locator(".cm-content").click();
await page.keyboard.press("Control+Home");
await page.keyboard.press("End");
await page.keyboard.press("Control+Enter");
await page.waitForTimeout(900);
const ranCursor = await page.evaluate(() => window.__ranSql.splice(0));
checks.ctrlEnterCursorStatement =
  ranCursor.length === 1 && ranCursor[0] === "select * from orders limit 1";
checks.cursorStatementHint = (
  await page.locator(".result-panel").innerText()
).includes("已执行第 1 / 2 条语句");

// 编辑器页签不共享撤销历史：新建 tab2 后连续撤销，绝不能把 tab1 的 SQL 撤进 tab2
await page.locator(".panel-head-add").click();
await page.waitForTimeout(350);
await page.locator(".cm-content").click();
await page.keyboard.type("select 42 as tab2_only");
await page.waitForTimeout(300);
for (let i = 0; i < 3; i += 1) {
  await page.keyboard.press("Control+Z");
  await page.waitForTimeout(150);
}
const tab2AfterUndo = (await page.locator(".cm-content").innerText()).trim();
checks.tabUndoIsolated =
  !tab2AfterUndo.includes("select * from orders") &&
  !tab2AfterUndo.includes("select * from addresses");
checks.tabUndoKeepsOwnSql = !tab2AfterUndo.includes("tab2_only");
// 切回 tab1：编辑器内容仍是原来那两条语句
await page.locator(".panel-head-tabs .panel-head-tab").nth(0).click();
await page.waitForTimeout(400);
checks.tabSwitchKeepsSql = (await page.locator(".cm-content").innerText()).includes(
  "select * from orders limit 1",
);
// 关掉临时 tab2，恢复单页签现场
await page.locator(".panel-head-tabs .panel-head-tab").nth(1).locator(".tab-close").click();
await page.waitForTimeout(300);
checks.tabClosedBack = (await page.locator(".panel-head-tabs .panel-head-tab").count()) === 1;

// 深色主题：结构一致，仅验证可渲染
const darkPage = await browser.newPage({ viewport: { width: 1440, height: 900 } });
darkPage.on("pageerror", (error) => errors.push(`dark pageerror: ${error.message}`));
await darkPage.goto(`${pathToFileURL(preview).href}?theme=dark`, { waitUntil: "load" });
await darkPage.waitForTimeout(1200);
// 深色主题的仪表盘：指标卡、图表与分布列表都要能渲染
checks.darkDashboard =
  (await darkPage.locator(".stat-card").count()) === 4 &&
  (await darkPage.locator(".chart-card .chart-svg").count()) >= 2 &&
  (await darkPage.locator(".pie-legend-bar").count()) > 0;
await darkPage.screenshot({ path: "preview/smoke-dark-dashboard.png", fullPage: true });
await darkPage.locator(".side-nav .nav-child", { hasText: "在线查询" }).click();
await darkPage.waitForTimeout(700);
checks.darkTheme = (await darkPage.locator(".query-config").count()) === 1;
await darkPage.screenshot({ path: "preview/smoke-dark.png", fullPage: true });

// 深色主题的数据字典（需先选实例与库）
await darkPage.locator(".side-nav .nav-child", { hasText: "数据字典" }).click();
await darkPage.waitForTimeout(500);
await darkPage.locator(".dict-select").nth(0).selectOption("demo-mysql");
await darkPage.waitForTimeout(600);
await darkPage.locator(".dict-select").nth(1).selectOption("demo_schedule");
await darkPage.waitForSelector(".dict-row", { timeout: 8000 });
checks.darkDict = (await darkPage.locator(".dict-row").count()) > 0;
await darkPage.screenshot({ path: "preview/smoke-dark-dict.png", fullPage: true });

// 深色主题的表元数据弹窗
await darkPage.locator(".dict-name").first().click();
await darkPage.waitForSelector(".meta-modal", { timeout: 8000 });
await darkPage.waitForTimeout(300);
checks.darkMeta = (await darkPage.locator(".meta-pair").count()) === 16;
await darkPage.screenshot({ path: "preview/smoke-dark-meta.png", fullPage: true });
await darkPage.locator(".meta-modal .modal-close").click();

// 数据字典：默认不预选实例与库
await page.locator(".side-nav .nav-child", { hasText: "数据字典" }).click();
await page.waitForTimeout(500);
// 字典页顶部同样是 Archery 动态公告；原来的库表设计规范硬编码文案已删除。
checks.dictAnnouncement =
  (await page.locator(".app-announcement").count()) === 1 &&
  (await page.locator(".app-announcement").innerText()).includes("演示用公告");
checks.dictDefaultEmpty =
  (await page.locator(".dict-row").count()) === 0 &&
  (await page.locator(".dict-list .empty").innerText()).includes("请选择实例与数据库");
await page.screenshot({ path: "preview/smoke-dict-empty.png", fullPage: true });

// 选实例后仍不预选库
await page.locator(".dict-select").nth(0).selectOption("demo-mysql");
await page.waitForTimeout(600);
checks.dictNoDefaultDb = (await page.locator(".dict-row").count()) === 0;
await page.locator(".dict-select").nth(1).selectOption("demo_schedule");
await page.waitForSelector(".dict-row", { timeout: 8000 });
checks.dictLetters = (await page.locator(".dict-letter").count()) >= 2;
checks.dictRows = (await page.locator(".dict-row").count()) > 20;
checks.dictGroups = (await page.locator(".dict-group-head").count()) >= 2;
checks.dictRowColumns = (await page.locator(".dict-row").first().innerText()).includes(
  "addresses",
);

// 可点击区域只覆盖表名：表名按钮明显窄于整行，点注释不会打开弹窗
const nameBox = await page.locator(".dict-name").first().boundingBox();
const rowBox = await page.locator(".dict-row").first().boundingBox();
checks.dictNameOnlyClickable = nameBox.width < rowBox.width - 60;
await page.locator(".dict-comment").first().click();
await page.waitForTimeout(250);
checks.dictCommentNotClickable = (await page.locator(".meta-modal").count()) === 0;

await page.screenshot({ path: "preview/smoke-dict.png", fullPage: true });

// 点击表名打开表元数据弹窗：表信息 / 列信息 / 索引信息 / 建表语句
await page.locator(".dict-name").first().click();
await page.waitForSelector(".meta-modal", { timeout: 8000 });
await page.waitForTimeout(300);
checks.metaTitle = (await page.locator(".meta-modal .modal-head").innerText()).includes(
  "表元数据展示",
);
checks.metaSections = (await page.locator(".meta-block-title").count()) === 4;
checks.metaBlockCounts =
  (await page.locator(".meta-block-note").allInnerTexts()).join("|") === "13 列|3 个索引";
checks.metaPairs = (await page.locator(".meta-pair").count()) === 16;
checks.metaMetaComment = (
  await page.locator(".meta-pair", { hasText: "表说明" }).innerText()
).includes("地址服务");
checks.metaDescRows = (await page.locator(".meta-table").nth(0).locator("tbody tr").count()) === 13;
checks.metaIndexRows = (await page.locator(".meta-table").nth(1).locator("tbody tr").count()) === 3;
checks.metaDdl = (await page.locator(".meta-ddl").innerText()).includes("CREATE TABLE");

// 表信息值不截断、建表语句不出现内部纵向滚动
checks.metaValueNotTruncated = await page
  .locator(".meta-pair", { hasText: "表说明" })
  .locator(".meta-value")
  .evaluate(
    (element) =>
      element.scrollWidth <= element.clientWidth + 1 &&
      getComputedStyle(element).textOverflow !== "ellipsis",
  );
checks.metaDdlAutoHeight = await page
  .locator(".meta-ddl")
  .evaluate((element) => element.scrollHeight <= element.clientHeight + 1);

await page.screenshot({ path: "preview/smoke-meta.png", fullPage: true });

// 滚到底部核对建表语句区块
await page.locator(".meta-body").evaluate((element) => {
  element.scrollTop = element.scrollHeight;
});
await page.waitForTimeout(300);
checks.metaDdlVisible = (await page.locator(".meta-ddl").boundingBox()).height > 40;
await page.screenshot({ path: "preview/smoke-meta-ddl.png", fullPage: true });

await page.locator(".meta-modal .modal-close").click();
await page.waitForTimeout(300);
checks.metaClosed = (await page.locator(".meta-modal").count()) === 0;

// 弹窗打开即接管焦点，Esc 能直接收起（宿主页面没有全局 Esc）
await page.locator(".dict-name").first().click();
await page.waitForSelector(".meta-modal", { timeout: 8000 });
await page.waitForTimeout(300);
checks.metaFocus = await page.evaluate(
  () => document.activeElement === document.querySelector(".meta-modal"),
);
await page.keyboard.press("Escape");
await page.waitForTimeout(400);
checks.metaEscClosed = (await page.locator(".meta-modal").count()) === 0;

// 搜索命中注释：customer*/receivers/senders 四行，字母索引同步收窄为 c/r/s 三组
await page.locator(".dict-search").fill("客户");
await page.waitForTimeout(400);
checks.dictSearch = (await page.locator(".dict-row").count()) === 4;
checks.dictSearchIndex = (await page.locator(".dict-letter").count()) === 3;
await page.screenshot({ path: "preview/smoke-dict-search.png", fullPage: true });
await page.locator(".dict-search").fill("");
await page.waitForTimeout(400);

// 字母索引跳转并高亮
await page.locator('.dict-letter[data-index-letter="c"]').click();
await page.waitForTimeout(500);
checks.dictJump = (await page.locator('.dict-letter[data-index-letter="c"].is-on').count()) === 1;

// 跳转后当前字母段的分组头吸在列表顶部，且只有一枚
checks.dictStickyHead = await page.evaluate(() => {
  const list = document.querySelector(".dict-list").getBoundingClientRect();
  const stuck = Array.from(document.querySelectorAll(".dict-group-head")).filter(
    (head) => Math.abs(head.getBoundingClientRect().top - list.top) <= 2,
  );
  return stuck.length === 1;
});

// 索引与计数的可访问性约定：当前字母 aria-current、计数走 live region
checks.dictAriaCurrent = (await page.locator(".dict-letter[aria-current='true']").count()) === 1;
checks.dictA11y =
  (await page.locator(".dict-index").getAttribute("aria-label")) === "字母索引" &&
  (await page.locator(".dict-count").getAttribute("aria-live")) === "polite";

// 「/」把焦点交给搜索框；已在输入框里时照常输入字符
await page.locator(".dict-name").first().focus();
await page.keyboard.press("/");
checks.dictSlashFocus = await page.evaluate(
  () => document.activeElement === document.querySelector(".dict-search"),
);
await page.keyboard.press("/");
checks.dictSlashTyping = (await page.locator(".dict-search").inputValue()) === "/";
await page.locator(".dict-search").fill("");
await page.waitForTimeout(400);

// 切库后表清单跟着变
await page.locator(".dict-select").nth(1).selectOption("demo_shop");
await page.waitForTimeout(700);
checks.dictSwitchDb = (await page.locator(".dict-row", { hasText: "orders" }).count()) === 1;
await page.screenshot({ path: "preview/smoke-dict-db.png", fullPage: true });

// 重复点击「数据字典」：重新进入回到未预选实例与库的状态
await page.locator(".side-nav .nav-child", { hasText: "数据字典" }).click();
await page.waitForTimeout(700);
checks.dictReenterEmpty =
  (await page.locator(".dict-row").count()) === 0 &&
  (await page.locator(".dict-list .empty").innerText()).includes("请选择实例与数据库");
await page.screenshot({ path: "preview/smoke-dict-reenter.png", fullPage: true });

// 宿主面板拖窄到 1000px：工具条换行而不是把右侧按钮裁掉
await page.setViewportSize({ width: 1000, height: 900 });
await page.locator(".dict-select").nth(0).selectOption("demo-mysql");
await page.waitForTimeout(600);
await page.locator(".dict-select").nth(1).selectOption("demo_schedule");
await page.waitForSelector(".dict-row", { timeout: 8000 });
checks.dictNarrowToolbar = await page.evaluate(() => {
  const bar = document.querySelector(".dict-toolbar");
  return (
    bar.scrollWidth <= bar.clientWidth + 1 &&
    bar.getBoundingClientRect().height > 50 &&
    bar.getBoundingClientRect().right <= window.innerWidth
  );
});
await page.screenshot({ path: "preview/smoke-dict-narrow.png", fullPage: true });
await page.setViewportSize({ width: 1440, height: 900 });
await page.waitForTimeout(300);

// 申请工单：进入权限管理默认选中该页签，11 列工单列表 + 状态标签
await page.locator(".side-nav .nav-child", { hasText: "权限管理" }).click();
await page.waitForTimeout(700);
checks.privilegeDefaultTab = (
  await page.locator(".privilege-toolbar .btn-default.is-on").innerText()
).includes("申请工单");
checks.privilegeAppliesColumns =
  (await page.locator(".privilege-table:not(.privilege-mine-table) thead th").count()) === 11;
checks.privilegeAppliesRows =
  (await page.locator(".privilege-table:not(.privilege-mine-table) tbody tr").count()) > 0;
checks.privilegeStatus = (await page.locator(".privilege-status").count()) > 0;
await page.screenshot({ path: "preview/smoke-privilege-applies.png", fullPage: true });

// 我的权限：切到 8 列表格（用户/实例/数据库/权限级别/表/结果集/有效时间/操作）
await page.locator(".privilege-toolbar .btn-default", { hasText: "我的权限" }).click();
await page.waitForTimeout(700);
checks.privilegeColumns = (await page.locator(".privilege-mine-table thead th").count()) === 8;
checks.privilegeRows = (await page.locator(".privilege-mine-table tbody tr").count()) > 0;
checks.privilegeScope = (await page.locator(".privilege-mine-table").innerText()).includes(
  "demo-mysql-rw",
);

// 汇总条 / 筛选 / 到期徽标 / 复制 / 导出 / 记录数
checks.privilegeMineSummary =
  (await page.locator(".privilege-mine-summary").count()) === 1 &&
  (await page.locator(".privilege-mine-summary").innerText()).includes("覆盖实例");
checks.privilegeMineFilters = (await page.locator(".privilege-filters select").count()) === 3;
checks.privilegeMineExpiry =
  (await page.locator(".privilege-mine-table .privilege-expiry.is-urgent").count()) >= 1 &&
  (await page.locator(".privilege-mine-table .privilege-expiry.is-expired").count()) >= 1;
checks.privilegeMineLevel = (await page.locator(".privilege-mine-table").innerText()).includes(
  "库级",
);
checks.privilegeMineCopy =
  (await page.locator(".privilege-mine-table tbody tr .privilege-copy-btn").count()) > 0;
checks.privilegeMineExport = (await page.locator(".privilege-export-btn").count()) === 1;
checks.privilegeMineTotal = (await page.locator(".privilege-list-body .pager").innerText()).includes(
  "共 14 条",
);

if ((await page.locator(".privilege-filters select").count()) === 3) {
  await page.locator(".privilege-filters select").nth(0).selectOption("demo-mysql-rw");
  await page.waitForTimeout(300);
  checks.privilegeMineFilter =
    (await page.locator(".privilege-mine-table tbody tr").count()) === 4;
  await page.locator(".privilege-filter-clear").click();
  await page.waitForTimeout(300);
  checks.privilegeMineFilterCleared =
    (await page.locator(".privilege-mine-table tbody tr").count()) === 14;

  const dbHeader = page.locator('.privilege-mine-table th[data-sort="database"]');
  if ((await dbHeader.count()) === 1) {
    await dbHeader.click();
    await page.waitForTimeout(200);
    const ascDb = await page
      .locator(".privilege-mine-table tbody tr td:nth-child(3)")
      .first()
      .innerText();
    await dbHeader.click();
    await page.waitForTimeout(200);
    const descDb = await page
      .locator(".privilege-mine-table tbody tr td:nth-child(3)")
      .first()
      .innerText();
    checks.privilegeMineSort = ascDb === "demo_alerts" && descDb === "demo_user";
  }
}

await page.screenshot({ path: "preview/smoke-privileges.png", fullPage: true });

// 再次点击左侧菜单：重新进入后页签回到申请工单，继续验证工单详情与日志
await page.locator(".side-nav .nav-child", { hasText: "权限管理" }).click();
await page.waitForTimeout(700);
checks.privilegeMenuResetTab = (
  await page.locator(".privilege-toolbar .btn-default.is-on").innerText()
).includes("申请工单");
await page.screenshot({ path: "preview/smoke-privilege-reset.png", fullPage: true });

// 工单详情：点工单名称进入，展示审批流、其他信息 9 列与数据库清单
await page.locator(".privilege-title-link").first().click();
await page.waitForSelector(".privilege-detail-body", { timeout: 6000 });
await page.waitForTimeout(400);
const detailText = await page.locator(".privilege-detail-body").innerText();
checks.privilegeDetailTitle = detailText.includes("工单名称：查询数据");
checks.privilegeDetailFlow = (await page.locator(".privilege-flow-node").count()) > 0;
checks.privilegeDetailColumns =
  (await page.locator(".privilege-detail-table thead th").count()) === 9;
checks.privilegeDetailItems =
  detailText.includes("demo_materials") &&
  detailText.includes("demo_config") &&
  detailText.includes("审核通过");
await page.screenshot({ path: "preview/smoke-privilege-detail.png", fullPage: true });
await page.locator(".privilege-back").click();
await page.waitForSelector(".privilege-table", { timeout: 6000 });
await page.waitForTimeout(300);

await page.locator(".privilege-table .btn-quiet").first().click();
await page.waitForSelector(".privilege-log-table", { timeout: 6000 });
const logRows = await page.locator(".privilege-log-table tbody tr").count();
checks.privilegeLog = logRows > 0;
// 抬头 / 页脚与申请、审核弹窗同源，页脚条数与表格行数一致
checks.privilegeLogHeader =
  (await page.locator(".privilege-log-modal .pa-title").innerText()).trim() === "工单日志";
checks.privilegeLogFoot = (
  await page.locator(".privilege-log-modal .pa-foot-note").innerText()
).includes(`${logRows} 条`);
await page.screenshot({ path: "preview/smoke-privilege-log.png", fullPage: true });
await page.locator(".privilege-modal .modal-foot .btn-default").click();
await page.waitForTimeout(300);

await page.locator(".privilege-apply-btn").click();
await page.waitForSelector(".privilege-modal", { timeout: 6000 });
const applyText = await page.locator(".privilege-modal").innerText();
checks.privilegeDialog =
  applyText.includes("申请数据库查询权限") && applyText.includes("请选择组");
// 版式：编号三段分区，默认 7 个带标签的字段（选到表级时多出「数据表」一格）
checks.privilegeApplySections =
  (await page.locator(".privilege-apply-modal .pa-section").count()) === 3 &&
  applyText.includes("申请说明") &&
  applyText.includes("授权范围") &&
  applyText.includes("授权时长");
checks.privilegeApplyLabels =
  (await page.locator(".privilege-apply-modal .pa-label").count()) === 7;
// 授权时间四档分段控件：默认不选，点「一月」直接选中
checks.privilegeApplyValiditySegments =
  (await page.locator(".privilege-apply-modal .pa-segment").count()) === 4;
await page.locator(".privilege-apply-modal .pa-segment", { hasText: "一月" }).click();
checks.privilegeApplyValidityPick =
  (await page.locator(".privilege-apply-modal .pa-segment.is-on").innerText()).trim() === "一月";
// 常用行数胶囊：点 500 直接写进输入框
await page.locator(".privilege-apply-modal .pa-chip", { hasText: "500" }).click();
checks.privilegeApplyLimitChip =
  (await page.locator(".privilege-apply-modal #pa-limit").inputValue()) === "500";
// 选中资源组后回填「审批流程：」，未选组时不显示提示条
checks.privilegeAuditorsHidden = (await page.locator(".privilege-auditors").count()) === 0;
await page.locator(".privilege-modal select").first().selectOption("Demo-Group-A");
await page.waitForSelector(".privilege-auditors", { timeout: 6000 });
checks.privilegeAuditors = (await page.locator(".privilege-auditors").innerText()).includes(
  "TEST-Leader-Express",
);
await page.screenshot({ path: "preview/smoke-privilege-apply.png", fullPage: true });
await page.locator(".privilege-modal .modal-foot .btn-default").click();
await page.waitForTimeout(300);

await page.locator(".privilege-table .btn-warning").first().click();
await page.waitForFunction(() => document.body.innerText.includes("审核权限申请"), null, {
  timeout: 6000,
});
checks.privilegeAudit =
  (await page.locator(".privilege-summary").count()) === 1 &&
  (await page.locator(".privilege-textarea").count()) === 1;
// 版式：工单卡 + 审核意见两段，终止流程用红色描边而非 Bootstrap 橙实心
checks.privilegeAuditSections =
  (await page.locator(".privilege-audit-modal .pa-section").count()) === 2 &&
  (await page.locator(".privilege-audit-modal").innerText()).includes("申请内容") &&
  (await page.locator(".privilege-audit-modal").innerText()).includes("审核意见");
checks.privilegeAuditDangerButton =
  (await page.locator(".privilege-audit-modal .pa-foot .pa-btn-danger").count()) === 1;
await page.screenshot({ path: "preview/smoke-privilege-audit.png", fullPage: true });
// 终止流程必须先填备注：空备注点终止 → 红色提示条出现，弹窗不关闭
await page.locator(".privilege-audit-modal .pa-btn-danger").click();
checks.privilegeAuditRemarkRequired =
  (await page.locator(".privilege-audit-modal .pa-error").count()) === 1 &&
  (await page.locator(".privilege-audit-modal").count()) === 1;
await page.screenshot({ path: "preview/smoke-privilege-audit-required.png", fullPage: true });
await page.locator(".privilege-modal .modal-foot .btn-default").click();
await page.waitForTimeout(300);

// SQL 上线：10 列列表 + 状态标签 + 公告条
await page.locator(".side-nav .nav-item", { hasText: "SQL审核" }).click();
await page.waitForTimeout(400);
await page.locator(".side-nav .nav-child", { hasText: "SQL上线" }).click();
await page.waitForSelector(".workflow-table", { timeout: 8000 });
await page.waitForTimeout(400);
checks.workflowColumns = (await page.locator(".workflow-table thead th").count()) === 10;
checks.workflowRows = (await page.locator(".workflow-table tbody tr").count()) > 5;
checks.workflowStatus =
  (await page.locator(".workflow-table .privilege-status").count()) > 0 &&
  (await page.locator(".workflow-table").innerText()).includes("已正常结束");
checks.workflowNotice = (await page.locator(".app-announcement").innerText()).includes(
  "演示用公告",
);
await page.screenshot({ path: "preview/smoke-workflow.png", fullPage: true });

// 重复点击「SQL上线」：重新挂载后仍回到 10 列列表
await page.locator(".side-nav .nav-child", { hasText: "SQL上线" }).click();
await page.waitForSelector(".workflow-table", { timeout: 8000 });
await page.waitForTimeout(400);
checks.workflowReenter = (await page.locator(".workflow-table thead th").count()) === 10;

// 待我审核：只保留 /workflow/list/ 里当前轮到我的 SQL 工单，状态筛选锁死为待审核
await page.locator(".workflow-scope .btn-default", { hasText: "待我审核" }).click();
await page.waitForTimeout(700);
checks.workflowAuditOnlyMine =
  (await page.locator(".workflow-table tbody tr").count()) === 1 &&
  (await page.locator(".workflow-table tbody").innerText()).includes("DDL - 车辆表结构修改");
checks.workflowAuditStatusLocked = await page
  .locator(".workflow-toolbar select")
  .first()
  .isDisabled();
checks.workflowAuditNoDeniedNotice = (await page.locator(".workflow-audit-denied").count()) === 0;
await page.screenshot({ path: "preview/smoke-workflow-audit-scope.png", fullPage: true });
await page.locator(".workflow-scope .btn-default", { hasText: "申请工单" }).click();
await page.waitForTimeout(700);
checks.workflowAuditRecovered =
  (await page.locator(".workflow-table tbody tr").count()) > 1 &&
  !(await page.locator(".workflow-toolbar select").first().isDisabled());

// 日期区间：默认最近 30 日，下拉预设与 Archery 的 daterangepicker 对齐
checks.workflowDateRange = /^\d{4}-\d{2}-\d{2} - \d{4}-\d{2}-\d{2}$/.test(
  (await page.locator(".date-range-trigger .date-range-label").innerText()).trim(),
);
await page.locator(".date-range-trigger").click();
await page.waitForSelector(".date-range-menu", { timeout: 4000 });
const presetLabels = (await page.locator(".date-range-item").allInnerTexts()).map((text) =>
  text.trim(),
);
checks.workflowDatePresets =
  presetLabels.length === 11 &&
  ["最近7日", "最近30日", "最近365日", "本月", "上个月", "本年", "上一年", "自定义"].every(
    (label) => presetLabels.includes(label),
  );
checks.workflowDateActive =
  (await page.locator(".date-range-item.active").innerText()).trim() === "最近30日";
await page.locator(".date-range-item", { hasText: "自定义" }).click();
checks.workflowDateCustom =
  (await page.locator(".date-range-custom input[type='date']").count()) === 2;
await page.screenshot({ path: "preview/smoke-workflow-date.png", fullPage: true });
await page.locator(".date-range-item", { hasText: "全部" }).click();
await page.waitForTimeout(700);
checks.workflowDateAll = (await page.locator(".date-range-label").innerText()).trim() === "全部";

// 操作日志：workflow_type=2 的工单日志
await page.locator(".workflow-table .btn-quiet").first().click();
await page.waitForSelector(".privilege-log-table", { timeout: 6000 });
checks.workflowLog = (await page.locator(".privilege-log-table tbody tr").count()) > 0;
await page.locator(".privilege-modal .modal-foot .btn-default").click();
await page.waitForTimeout(300);

// 详情：状态胶囊 + 元信息行 + 审批流 chip + 其他信息 10 项卡片 + 页签 + SQL 列表
await page.locator(".workflow-table .privilege-title-link").first().click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
await page.waitForTimeout(400);
checks.workflowDetailTitle = (await page.locator(".workflow-title").innerText()).includes(
  "DDL - 车辆表结构修改",
);
// 头部：状态胶囊与列表页同一套语义色，紧贴标题右侧
checks.workflowDetailStatusPill =
  (await page.locator(".workflow-title-row .privilege-status").count()) === 1 &&
  (await page.locator(".workflow-title-row .privilege-status").innerText()).includes("已正常结束");
// 头部元信息行：实例 / 库 / 组 / 发起人 / 发起时间，不必滚到「其他信息」才看得到
checks.workflowDetailMeta =
  (await page.locator(".workflow-meta-item").count()) === 5 &&
  (await page.locator(".workflow-meta").innerText().catch(() => ""))
    .replace(/\s+/g, "")
    .includes("test-express-rw");
// 审批流：每个节点是一枚带状态标记的 chip
checks.workflowDetailFlow = (await page.locator(".workflow-flow-node").count()) === 2;
checks.workflowDetailFlowMarks =
  (await page.locator(".workflow-flow-node .workflow-flow-mark").count()) === 2;
// 已结束工单的节点状态在详情页 HTML 里解析不到（mock 送空 state），前端兜底按「已通过」渲染
checks.workflowDetailFlowFallback =
  (await page.locator(".workflow-flow-node.is-passed").count()) === 2;
checks.workflowDetailInfoItems = (await page.locator(".workflow-info-item").count()) === 10;
checks.workflowDetailInfoValues = (await page
  .locator(".workflow-detail-scroll")
  .innerText()).includes("无限制");
// 卡片里的长值换行而不是省略号截断
const infoValueCount = await page.locator(".workflow-info-value").count();
checks.workflowDetailInfoNoTruncate =
  infoValueCount === 10 &&
  (await page
    .locator(".workflow-info-value")
    .evaluateAll((nodes) => nodes.every((node) => node.scrollWidth <= node.clientWidth + 1)));
checks.workflowDetailTabs = (await page.locator(".workflow-tab").count()) === 2;
const tabCountBadge = page.locator(".workflow-tab.active .workflow-tab-count");
checks.workflowDetailTabCount =
  (await tabCountBadge.count()) === 1 && (await tabCountBadge.innerText()).trim() === "2";
// 搜索框常驻页签行：切页签时行高不跳
checks.workflowDetailSearchInTabs =
  (await page.locator(".workflow-tabs .workflow-sql-search").count()) === 1;
checks.workflowGridColumns = (await page.locator(".workflow-grid thead th").count()) === 10;
checks.workflowGridRows = (await page.locator(".workflow-grid tbody tr").count()) >= 2;
checks.workflowGridWarningRow = (await page.locator(".workflow-grid tbody tr.is-warn").count()) === 1;
// 「操作」列不再是恒为「-」的死列，换成行内复制 SQL
checks.workflowGridCopyButton =
  (await page.locator(".workflow-grid tbody .workflow-copy-sql").count()) === 2;
// 展开按钮不再被 34px 单元格挤成「+ …」：按钮完整落在单元格内
checks.workflowGridCaretNotClipped = await page.evaluate(() => {
  const cells = Array.from(document.querySelectorAll(".workflow-grid td.workflow-grid-caret"));
  return (
    cells.length > 0 && cells.every((cell) => cell.scrollWidth <= cell.clientWidth + 1)
  );
});
checks.workflowRecordCount = (
  await page.locator(".workflow-record-count").innerText()
).includes("总共 2 条记录") &&
  (await page.locator(".workflow-record-count .workflow-count-chip").count()) === 1;
// 详情区高度随内容走：2 行时贴合表格，不撑到固定高度、也不出现内层纵向滚动
const fewRowGrid = await page.locator(".workflow-grid-wrap").evaluate((el) => ({
  height: el.getBoundingClientRect().height,
  innerScroll: el.scrollHeight > el.clientHeight + 1,
}));
checks.workflowDetailGridHug = fewRowGrid.height < 240;
checks.workflowDetailGridNoInnerScroll = !fewRowGrid.innerScroll;
await page.screenshot({ path: "preview/smoke-workflow-detail.png", fullPage: true });

// 查看提交信息 → 整页切到「提交SQL」并按原工单预填（标题 / SQL / 组 / 实例 / 库 / 备份）
await page.locator(".workflow-title-row .btn", { hasText: "查看提交信息" }).click();
await page.waitForSelector(".workflow-submit-page", { timeout: 8000 });
await page.waitForTimeout(900);
checks.workflowCommitPrefill = (await page.locator(".workflow-prefill").innerText()).includes(
  "5338",
);
checks.workflowCommitTitle =
  (await page.locator(".workflow-submit-side input[type='text']").first().inputValue()) ===
  "DDL - 车辆表结构修改";
const commitSqlText = await page.locator(".workflow-submit-editor .cm-content").innerText();
checks.workflowCommitSql = commitSqlText.includes("axle_number");
// 原工单首行是 `use schedule-dev`，属切换库上下文，不带到新工单
checks.workflowCommitSqlNoUse = !/\buse\s+`?schedule-dev`?/i.test(commitSqlText);
checks.workflowCommitGroup =
  (await page.locator(".workflow-submit-side select").nth(0).inputValue()) === "3";
checks.workflowCommitInstance =
  (await page.locator(".workflow-submit-side select").nth(1).inputValue()) === "12";
// 原工单的库不在资源清单里时也要显示出来（select 兜底选项）
checks.workflowCommitDatabase =
  (await page.locator(".workflow-submit-side select").nth(2).inputValue()) === "schedule-dev";
checks.workflowCommitBackup =
  (await page.locator(".workflow-submit-side select").nth(3).inputValue()) === "true";
// 带入的 SQL 还没检测过，提交按钮保持禁用
checks.workflowCommitNeedCheck = await page
  .locator(".workflow-submit-actions .btn-success")
  .isDisabled();
await page.screenshot({ path: "preview/smoke-workflow-commit.png", fullPage: true });

// 返回：回到来源工单详情，继续原有详情断言
await page.locator(".workflow-back-btn").click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
await page.waitForTimeout(400);
checks.workflowCommitBack = (await page.locator(".workflow-title").innerText()).includes(
  "DDL - 车辆表结构修改",
);

// 上线其他实例 → 同一提交页，除目标实例与库外全部带入（名称 / SQL / 组 / 备份 / 执行时间窗）
checks.workflowOtherBtnOrder = await page
  .locator(".workflow-title-row .btn")
  .evaluateAll((nodes) => nodes.map((node) => node.textContent.trim()))
  .then((texts) => texts.indexOf("上线其他实例") === texts.indexOf("查看提交信息") - 1);
await page.locator(".workflow-title-row .btn", { hasText: "上线其他实例" }).click();
await page.waitForSelector(".workflow-submit-page", { timeout: 8000 });
await page.waitForTimeout(900);
checks.workflowOtherNotice = (await page.locator(".workflow-prefill").innerText()).includes(
  "选择要上线的实例与数据库",
);
checks.workflowOtherTitle =
  (await page.locator(".workflow-submit-side input[type='text']").first().inputValue()) ===
  "DDL - 车辆表结构修改";
checks.workflowOtherGroup =
  (await page.locator(".workflow-submit-side select").nth(0).inputValue()) === "3";
checks.workflowOtherInstanceEmpty =
  (await page.locator(".workflow-submit-side select").nth(1).inputValue()) === "0";
checks.workflowOtherDatabaseEmpty =
  (await page.locator(".workflow-submit-side select").nth(2).inputValue()) === "";
checks.workflowOtherSql = (
  await page.locator(".workflow-submit-editor .cm-content").innerText()
).includes("axle_number");
checks.workflowOtherBackup =
  (await page.locator(".workflow-submit-side select").nth(3).inputValue()) === "true";
checks.workflowOtherNeedCheck = await page
  .locator(".workflow-submit-actions .btn-success")
  .isDisabled();
await page.screenshot({ path: "preview/smoke-workflow-other-instance.png", fullPage: true });

await page.locator(".workflow-back-btn").click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
await page.waitForTimeout(400);

// 展开全部 → 每行下方显示完整 SQL
await page.locator(".workflow-grid-toolbar .btn").first().click();
await page.waitForTimeout(300);
checks.workflowExpandAll = (await page.locator(".workflow-grid-sql-full").count()) === 2;
await page.screenshot({ path: "preview/smoke-workflow-expand.png", fullPage: true });
await page.locator(".workflow-grid-toolbar .btn").first().click();
await page.waitForTimeout(200);

// 工单日志页签：内嵌表格而不是弹窗，按时间倒序、页签行内有搜索框、表格下方有记录数
await page.locator(".workflow-tab", { hasText: "工单日志" }).click();
await page.waitForTimeout(600);
checks.workflowDetailLog = (await page.locator(".workflow-log-wrap tbody tr").count()) === 6;
checks.workflowDetailLogOrder = (
  await page.locator(".workflow-log-table tbody tr").first().innerText()
).includes("执行结束");
checks.workflowDetailLogCount = (
  await page.locator(".workflow-record-count").innerText()
).includes("总共 6 条记录");
checks.workflowDetailLogSearch = (await page.locator(".workflow-log-search").count()) === 1;
await page.screenshot({ path: "preview/smoke-workflow-log.png", fullPage: true });
await page.locator(".workflow-log-search").fill("审核通过");
await page.waitForTimeout(300);
checks.workflowDetailLogFilter = (await page.locator(".workflow-log-table tbody tr").count()) === 2;
await page.locator(".workflow-log-search").fill("没有这样的日志");
await page.waitForTimeout(300);
checks.workflowDetailLogNoMatch =
  (await page.locator(".workflow-log-wrap").count()) === 0 &&
  (await page.locator(".workflow-detail-scroll .empty").last().innerText()).includes(
    "没有匹配",
  );
await page.locator(".workflow-log-search").fill("");
await page.waitForTimeout(300);
await page.locator(".workflow-tab", { hasText: "工单详情" }).click();
await page.waitForTimeout(300);

// 详情页直接下载回滚 SQL（不进子页）：sidecar 落盘后 toast 提示保存路径
await page.locator(".workflow-actions .btn", { hasText: "下载回滚SQL" }).click();
await page.waitForTimeout(700);
checks.workflowRollbackDownload = (
  await page.locator(".toast").first().innerText().catch(() => "")
).includes("rollback_5338.sql");

// 查看回滚SQL → 独立子页：序号 + 执行语句 / 回滚语句 两列对照（首行两列各含执行与回滚片段）
await page.locator(".workflow-actions .btn", { hasText: "查看回滚SQL" }).click();
await page.waitForSelector(".rollback-grid", { timeout: 6000 });
const firstRollbackRow = await page.locator(".rollback-grid tbody tr").first().innerText();
checks.workflowRollback =
  (await page.locator(".rollback-grid thead th").count()) === 4 &&
  (await page.locator(".rollback-grid tbody tr").count()) === 2 &&
  firstRollbackRow.includes("int(11)") &&
  firstRollbackRow.includes("varchar(10)");
checks.workflowRollbackIndex =
  (await page.locator(".rollback-grid tbody .rollback-grid-index").allInnerTexts())
    .map((text) => text.trim())
    .join(",") === "#1,#2";
// 头部补回滚目标：组 / 目标实例 / 库，进页面就知道这批语句落在哪
checks.workflowRollbackMeta =
  (await page.locator(".rollback-head .workflow-meta-item").count()) === 3 &&
  (await page.locator(".rollback-head").innerText()).includes("test-express-rw") &&
  (await page.locator(".rollback-head").innerText()).includes("schedule-dev");
// goInception 补的库名前缀（`schedule-dev`.`表` / schedule-dev.表）在展示时就已剥离
checks.workflowRollbackQualifierStripped = !(
  await page.locator(".rollback-grid tbody").innerText()
).includes("schedule-dev");
// 两行截断但不横向裁切：正文在单元格内换行，不出现溢出
checks.workflowRollbackClamp = await page.evaluate(() => {
  const cells = Array.from(document.querySelectorAll(".rollback-grid td.workflow-grid-sql"));
  return cells.length === 4 && cells.every((cell) => cell.scrollWidth <= cell.clientWidth + 1);
});
checks.workflowRollbackCopy =
  (await page.locator(".rollback-grid tbody .rollback-copy").count()) === 2;
checks.workflowRollbackCount = (
  await page.locator(".workflow-record-count").innerText()
).includes("总共 2 条记录");
// 底栏固定在面板底部：记录数与下载/提交按钮不用滚到底才看得到
checks.workflowRollbackFooterPinned = await page.evaluate(() => {
  const panel = document.querySelector(".workflow-detail-panel");
  const actions = document.querySelector(".rollback-actions");
  if (!panel || !actions) return false;
  return (
    Math.abs(actions.getBoundingClientRect().bottom - panel.getBoundingClientRect().bottom) <= 2 &&
    !actions.closest(".workflow-detail-scroll")
  );
});
await page.screenshot({ path: "preview/smoke-workflow-rollback.png", fullPage: true });

// 回滚子页展开行：执行语句与回滚语句全文
await page.locator(".rollback-grid .workflow-grid-caret .op-btn").first().click();
await page.waitForTimeout(300);
checks.workflowRollbackExpand =
  (await page.locator(".workflow-grid-expanded .workflow-grid-sql-full").count()) === 2;
await page.locator(".rollback-grid .workflow-grid-caret .op-btn").first().click();
await page.waitForTimeout(200);

// 提交回滚请求：组/实例/库锁定为原工单值（弹窗内无可编辑控件），确认后提示新工单号并回到详情
await page.locator(".workflow-actions .btn", { hasText: "提交回滚请求" }).click();
await page.waitForSelector(".privilege-modal", { timeout: 6000 });
await page.waitForTimeout(600);
const rollbackSubmitText = await page.locator(".privilege-modal").innerText();
checks.workflowRollbackSubmitFixed =
  (await page.locator(".privilege-modal select, .privilege-modal input").count()) === 0 &&
  rollbackSubmitText.includes("Test-Express") &&
  rollbackSubmitText.includes("test-express-rw") &&
  rollbackSubmitText.includes("schedule-dev") &&
  rollbackSubmitText.includes("不可修改");
// 弹窗内直接展示将提交的执行语句与回滚语句全文（不只是条数）
checks.workflowRollbackSubmitStatements =
  (await page.locator(".privilege-modal .rollback-submit-item").count()) === 2 &&
  rollbackSubmitText.includes("axle_number varchar(10)") &&
  rollbackSubmitText.includes("insert into bs_vehicle_info (id, plate_no)");
// goInception 的多行 DDL 在弹窗里压成单行（引号内空白不动），弹窗也放宽到 720px
checks.workflowRollbackSubmitFlattened = rollbackSubmitText.includes(
  "alter table bs_vehicle_info modify column `axle_number` int(11)",
);
checks.workflowRollbackSubmitWide =
  ((await page.locator(".privilege-modal").boundingBox())?.width ?? 0) >= 680;
await page.screenshot({ path: "preview/smoke-workflow-rollback-submit.png", fullPage: true });
await page.locator(".privilege-modal .modal-foot .btn-warning").click();
await page.waitForTimeout(600);
checks.workflowRollbackSubmit = (
  await page.locator(".toast").last().innerText().catch(() => "")
).includes("2088");

// 提交成功后已回到工单详情，返回列表
await page.locator(".privilege-back").click();
await page.waitForSelector(".workflow-table", { timeout: 6000 });
await page.waitForTimeout(300);

// 40 条 SQL 的工单：详情区随内容撑开（不设上限、不内层滚动），整页滚动承载
await page
  .locator(".workflow-table tbody tr", { hasText: "批量索引变更" })
  .locator(".privilege-title-link")
  .click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
await page.waitForTimeout(400);
checks.workflowDetailManyRecords = (
  await page.locator(".workflow-record-count").innerText()
).includes("总共 40 条记录");
const manyRowGrid = await page.locator(".workflow-grid-wrap").evaluate((el) => ({
  height: el.getBoundingClientRect().height,
  innerScroll: el.scrollHeight > el.clientHeight + 1,
}));
checks.workflowDetailGridGrows = manyRowGrid.height > 800;
checks.workflowDetailGridUnlimited = !manyRowGrid.innerScroll;
checks.workflowDetailBodyScrolls = await page
  .locator(".workflow-detail-scroll")
  .evaluate((el) => el.scrollHeight > el.clientHeight + 40);
// 底部操作条固定在面板底栏（不在滚动区里）：长工单不用一路滚到底才看到按钮
checks.workflowDetailActionsOutsideScroll = await page.evaluate(() => {
  const actions = document.querySelector(".workflow-detail-actions");
  return Boolean(actions) && !actions.closest(".workflow-detail-scroll");
});
checks.workflowDetailActionsPinned = await page.evaluate(() => {
  const panel = document.querySelector(".workflow-detail-panel");
  const actions = document.querySelector(".workflow-detail-actions");
  if (!panel || !actions) return false;
  return (
    Math.abs(actions.getBoundingClientRect().bottom - panel.getBoundingClientRect().bottom) <= 2
  );
});
await page.screenshot({ path: "preview/smoke-workflow-detail-many.png", fullPage: true });
// 滚到底：记录数与最后操作信息仍完整可达
await page
  .locator(".workflow-detail-scroll")
  .evaluate((el) => el.scrollTo({ top: el.scrollHeight }));
await page.waitForTimeout(200);
checks.workflowDetailFooterReachable = await page.evaluate(() => {
  const body = document.querySelector(".workflow-detail-scroll");
  const count = document.querySelector(".workflow-record-count");
  if (!body || !count) return false;
  const bodyRect = body.getBoundingClientRect();
  const rect = count.getBoundingClientRect();
  return rect.top >= bodyRect.top && rect.bottom <= bodyRect.bottom + 1;
});
checks.workflowDetailActionsReachable = await page.evaluate(() => {
  const panel = document.querySelector(".workflow-detail-panel");
  const buttons = Array.from(document.querySelectorAll(".workflow-detail-actions .btn"));
  if (!panel || buttons.length === 0) return false;
  const panelRect = panel.getBoundingClientRect();
  return buttons.every((button) => {
    const rect = button.getBoundingClientRect();
    return rect.top >= panelRect.top && rect.bottom <= panelRect.bottom + 1;
  });
});
await page.screenshot({ path: "preview/smoke-workflow-detail-many-bottom.png", fullPage: true });
await page.locator(".privilege-back").click();
await page.waitForSelector(".workflow-table", { timeout: 6000 });
await page.waitForTimeout(300);

// 待审核工单：审核入口只对 workflow_manreviewing 出现
await page
  .locator(".workflow-table tbody tr", { hasText: "发票中心oit加字段" })
  .locator(".privilege-title-link")
  .click();
await page.waitForSelector(".workflow-grid", { timeout: 8000 });
checks.workflowAuditEntry = (await page.locator(".workflow-actions .btn-success").count()) === 1;
checks.workflowExecuteEntry = (await page.locator(".workflow-actions .btn-primary").count()) === 0;
// 审批流当前节点单独高亮并列出审批人，而不是把审批人塞进红色下划线文本
checks.workflowDetailFlowCurrent =
  (await page.locator(".workflow-flow-node.is-current").count()) === 1 &&
  (await page.locator(".workflow-flow-node.is-current .workflow-flow-reviewers").count()) === 1;
// 字段数不整除时自动降列（待审核工单 9 个字段 → 3 列），最后一行不留空槽
checks.workflowDetailInfoColumnsFit = await page.evaluate(() => {
  const grid = document.querySelector(".workflow-info-grid");
  if (!grid) return false;
  const items = grid.querySelectorAll(".workflow-info-item").length;
  const tracks = getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length;
  return items > 0 && tracks > 0 && items % tracks === 0;
});
await page.locator(".workflow-actions .btn-success").click();
await page.waitForSelector(".privilege-modal", { timeout: 6000 });
checks.workflowAuditDialog = (await page.locator(".privilege-modal").innerText()).includes(
  "审核上线工单",
);
await page.screenshot({ path: "preview/smoke-workflow-audit.png", fullPage: true });
await page.locator(".privilege-modal .modal-foot .btn-default").click();
await page.waitForTimeout(300);
await page.locator(".privilege-back").click();
await page.waitForSelector(".workflow-table", { timeout: 6000 });

// 提交 SQL（整页）：左编辑器 + 右字段列 + 底部检测结果
await page.locator(".workflow-toolbar .btn-success").click();
await page.waitForSelector(".workflow-submit-page", { timeout: 6000 });
await page.waitForTimeout(400);
checks.workflowSubmitPage = (await page.locator(".workflow-submit-panel").innerText()).includes(
  "SQL上线",
);
checks.workflowSubmitEditor = (await page.locator(".workflow-submit-editor .cm-content").count()) === 1;
checks.workflowSubmitFields =
  (await page.locator(".workflow-submit-side .form-control").count()) >= 5 &&
  (await page.locator(".workflow-submit-side").innerText()).includes("仅支持10M内的SQL文件");
checks.workflowSubmitActions =
  (await page.locator(".workflow-submit-actions .btn").count()) === 3;
checks.workflowCheckPanel = (await page.locator(".workflow-check-panel").innerText()).includes(
  "检测结果",
);
// 空态下检测结果面板应紧凑（只留标题与占位行）
const checkPanelEmptyHeight = (await page.locator(".workflow-check-panel").boundingBox()).height;
checks.workflowCheckEmptyCompact = checkPanelEmptyHeight < 130;
// 编辑区固定高度：检测结果出现后不应被压缩
const editorHeightBefore = (await page.locator(".workflow-submit-editor").boundingBox()).height;

await page.locator(".workflow-submit-side input[type='text']").nth(0).fill("提交SQL测试");
checks.workflowSubmitDisabled = await page
  .locator(".workflow-submit-actions .btn-success")
  .isDisabled();
await page.locator(".workflow-submit-side select").nth(0).selectOption("3");
await page.waitForTimeout(500);
checks.workflowAuditors = (await page.locator(".workflow-auditors").innerText()).includes(
  "TEST-Leader-Express->DBA",
);
await page.locator(".workflow-submit-side select").nth(1).selectOption("12");
await page.waitForTimeout(400);
await page.locator(".workflow-submit-side select").nth(2).selectOption("demo_schedule");
await page.locator(".workflow-submit-editor .cm-content").click();
await page.keyboard.type("alter table vehicles add column axle_number int;");
await page.locator(".workflow-submit-actions .btn-danger").click();
await page.waitForTimeout(500);
checks.workflowCheck = (await page.locator(".workflow-check-summary").innerText()).includes(
  "1 个警告",
);
checks.workflowCheckColumns =
  (await page.locator(".workflow-check-table thead th").count()) === 6;
checks.workflowCheckRows =
  (await page.locator(".workflow-check-table tbody tr").count()) === 2;
checks.workflowCheckStatus = (await page.locator(".workflow-check-table tbody tr").first().innerText()).includes(
  "pass",
);
checks.workflowCheckRecords = (
  await page.locator(".workflow-record-count").innerText()
).includes("总共 2 条记录");
// 有结果后面板长高，且底部记录数始终可见（不被表格滚动区吞掉）
const checkPanelFilledHeight = (await page.locator(".workflow-check-panel").boundingBox()).height;
checks.workflowCheckHeightGrows = checkPanelFilledHeight > checkPanelEmptyHeight + 40;
checks.workflowCheckFooterVisible =
  (await page.locator(".workflow-check-body > .workflow-record-count").boundingBox()).height > 0;
checks.workflowEditorFixed =
  Math.abs((await page.locator(".workflow-submit-editor").boundingBox()).height - editorHeightBefore) <= 1;
checks.workflowSubmitEnabled = !(await page
  .locator(".workflow-submit-actions .btn-success")
  .isDisabled());
await page.screenshot({ path: "preview/smoke-workflow-submit.png", fullPage: true });

// 结果很多时：面板封顶（不挤压编辑器）且表格内部滚动
await page.locator(".workflow-submit-editor .cm-content").click();
await page.keyboard.press("Control+A");
await page.keyboard.type("many rows for height check");
await page.locator(".workflow-submit-actions .btn-danger").click();
await page.waitForTimeout(600);
const manyPanelHeight = (await page.locator(".workflow-check-panel").boundingBox()).height;
checks.workflowCheckUnlimited = manyPanelHeight > 600;
checks.workflowCheckNoInnerScroll = await page
  .locator(".workflow-check-body > .grid-wrap")
  .evaluate((el) => el.scrollHeight <= el.clientHeight + 1);
checks.workflowSubmitBodyScrolls = await page
  .locator(".workflow-submit-body")
  .evaluate((el) => el.scrollHeight > el.clientHeight + 40);
checks.workflowEditorFixedOnMany =
  Math.abs((await page.locator(".workflow-submit-editor").boundingBox()).height - editorHeightBefore) <= 1;
// 面板长高时右侧字段列不能被裁切
checks.workflowSubmitSideIntact = await page
  .locator(".workflow-submit-side")
  .evaluate((el) => el.scrollHeight <= el.clientHeight + 1);
await page.screenshot({ path: "preview/smoke-workflow-many-results.png", fullPage: true });

// 恢复原 SQL 并重新检测，验证「内容一变就禁用提交」
await page.locator(".workflow-submit-editor .cm-content").click();
await page.keyboard.press("Control+A");
await page.keyboard.type("alter table vehicles add column axle_number int;");
checks.workflowSubmitDisabledAfterEdit = await page
  .locator(".workflow-submit-actions .btn-success")
  .isDisabled();
await page.locator(".workflow-submit-actions .btn-danger").click();
await page.waitForTimeout(600);
await page.locator(".workflow-submit-actions .btn-success").click();
await page.waitForTimeout(300);
checks.workflowConfirm = (await page.locator(".workflow-confirm").count()) === 1;
await page.locator(".workflow-confirm .btn").click();
await page.waitForTimeout(900);
checks.workflowSubmitted = (await page.locator(".workflow-table").innerText()).includes(
  "提交SQL测试",
);

await page.locator(".side-nav .nav-child", { hasText: "在线查询" }).click();
await page.waitForTimeout(400);
checks.backToQuery = (await page.locator(".query-panel").count()) === 1;

// 2FA：验证码框只展示连接配置里选定的方式，不允许临时切换
const otpPage = await browser.newPage({ viewport: { width: 1440, height: 900 } });
otpPage.on("pageerror", (error) => errors.push(`2fa pageerror: ${error.message}`));
otpPage.on("console", (message) => {
  if (message.type() === "error") errors.push(`2fa console: ${message.text()}`);
});
await otpPage.goto(`${pathToFileURL(preview).href}?2fa=1`, { waitUntil: "load" });
await otpPage.waitForSelector(".modal", { timeout: 8000 });
const modalText = await otpPage.locator(".modal").innerText();
checks.otpDialog = modalText.includes("验证方式") && modalText.includes("身份验证器");
checks.otpNoSelector = !modalText.includes("短信验证码");
await otpPage.screenshot({ path: "preview/smoke-2fa.png", fullPage: true });

await otpPage.locator(".modal input").fill("123456");
await otpPage.getByRole("button", { name: "验证并连接" }).click();
await otpPage.waitForSelector(".modal", { state: "detached", timeout: 8000 });
checks.otpCompleted = (await otpPage.locator(".stat-grid").count()) > 0;

console.log(JSON.stringify({ checks, errors }, null, 2));
await browser.close();

if (errors.length > 0 || Object.values(checks).some((ok) => !ok)) {
  process.exit(1);
}
