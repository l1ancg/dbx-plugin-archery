# Archery 审核查询 · DBX 插件

[中文](#中文) | [English](#english)

---

## 中文

在 DBX 工作台内浏览 [Archery](https://github.com/hhyo/Archery) 实例资源、执行 SQL 查询、查看查询历史与表结构。

- 插件 ID：`io.github.nixing178.archery`
- 当前版本：`0.1.0`
- 形态：Go Sidecar + Vue3 单文件控制台（视觉对齐 Archery 1.14 网页：Bootstrap 3 / SB Admin 2 的顶栏、侧栏与面板度量，CodeMirror 6 编辑器；面板圆角、表格表头、表单与页签度量全站统一）
- 许可证：Apache-2.0

### 功能（v0.1.0）

| 能力 | 说明 |
| --- | --- |
| 控制台外壳 | 顶栏品牌取自 Archery 导航栏 `navbar-brand`（`Archery` + `custom_title_suffix`，如 `Archery(广州跨境&其他测试环境)`），取不到时退回 DBX 连接名，右侧为用户区；左侧 6 项导航（Dashboard、SQL审核、SQL查询、SQL优化、工具插件、相关文档）并可展开二级菜单（SQL查询默认展开：在线查询 / 数据字典 / 权限管理），上次所在页面随窗口恢复，左侧菜单（含当前页）重复点击都会重新进入该页并刷新数据；所有页面顶部统一展示 Archery 系统公告（取自首页 alert 块，60 秒内复用、切页按 TTL 刷新，取不到不显示） |
| 仪表盘 | 四张指标卡（SQL 上线工单、SQL 查询工单、有效平台用户、实例数量）+ 实例类型柱状/环形图；四个数字按卡片文案归类取值，不依赖 Archery 模板顺序，首页多出别的 info-box 也不会串位；实例类型柱状/环形图与「SQL 查询统计（检索行数/次数折线）」「SQL 上线统计（上线数量柱状）」都取 Archery 首页同源图表数据（实例图按全库实例、趋势图按首页统计区间），两张按日图上方可切换统计时间范围（最近7/30/365日、本月、本年、上个月、上一年、自定义，无「全部」，默认最近7日且右端含明天 `[D-6, D+1]`，与 Archery 首页口径一致；Archery 首页页面不吃日期参数，切换时走 `/dashboard/api/?start_date=&end_date=` 只重取这两张图），取不到时才退回占位或当前账号可见实例；指标读取失败时降级为破折号，不阻塞页面 |
| 连接与登录 | 用户名/密码走 Archery 会话登录，自动处理 Django CSRF；支持 TOTP / 短信 2FA |
| 实例资源树 | 实例 → 库 → 表，懒加载 + 搜索，双击表名查看结构 |
| 数据字典 | 进入时默认不预选实例与库；选定后按首字母分组列出表名与中文注释（注释取自 Archery `/data_dictionary/table_list/`，需要 `sql.menu_data_dictionary` 菜单权限，缺权限时仅显示表名并给出提示），A–Z 索引可跳转并显示每组表数，支持表名/注释搜索；点击表名打开表元数据弹窗（表信息 / 列信息 / 索引信息 / 建表语句，取自 `/data_dictionary/table_info/`），弹窗内可复制表名 |
| SQL 查询 | 多标签编辑器、SQL 高亮、`Ctrl+Enter` 执行（选区优先，选中多条按顺序逐条执行、每条一个结果页签；无选区时执行光标所在语句）、实例 / 库为可搜索下拉（点一下就开、候选上千也只渲染前 200 条）、常用查询下拉、美化、执行计划、查看表结构、行数限制；每个页签独立保存编辑器状态（光标 / 选区 / 撤销历史不跨页签串台，v0.2.59） |
| 结果网格 | 列类型提示、NULL 样式、列宽拖拽、分页（每页条数候选 30 / 50 / 100 / 200 / 500 / 1000，默认 30；上限跟随查询目标行数，上限本身留一档；列表高度随每页条数变化，分页条与查询历史页签同版式）、搜索（工具栏内输入即按可见列过滤，大小写不敏感，命中内容在单元格内高亮并显示匹配行数；Ctrl+F 聚焦、Esc 清除；搜索时复制与导出只带匹配行）、复制下拉（CSV / TSV / Markdown 三项合并成一个「复制 ▾」按钮）、「列配置」浮层（勾选控制每列显示 / 隐藏：按列名或类型搜索，支持模糊 / 前缀 / 精确匹配，↑↓ 选择、Enter 切换、Esc 关闭，底部显示已隐藏列数并可一键全部显示，至少保留一列；CSV / TSV / Markdown 与 Excel 导出都只带可见列，有搜索时只带匹配行）、导出 Excel（.xlsx，导出当前结果页签的全部行，有搜索时只导出匹配行，表头加粗、首行冻结、自动筛选；每次弹系统「另存为」可自选保存位置，并记住上次目录；超 5MB 提示调小行数上限） |
| 结果面板页签（v0.2.38） | 内层页签挂在外层编辑器 tab 上，切换编辑器 tab 就切一整套：「消息 / 查询历史」固定在最前且不可关闭，「结果 N」执行时就地刷新当前页签（不再每次执行新增），表结构按表多开；页签行右侧的「+ / −」手动新开一条空结果、关掉当前结果（停在消息 / 查询历史 / 表结构上时「−」为禁用态）；结果与表结构带 × 可关，可拖拽互调顺序（系统页签钉在原位）；「行数」从底部执行区移到「查询目标」区，与实例 / 库 / 表同组 |
| 查询历史 | 8 列表格（操作/用户/实例/库/查询时间/语句/返回行数/耗时）、实例与库筛选、搜索、仅看收藏、行内展开/收藏/载入、分页 |
| 我的权限 | 8 列当前账号的有效查询权限（用户/实例/数据库/权限级别/表/结果集/有效时间/操作），顶部汇总有效 / 临期 / 已过期 / 覆盖实例；支持实例、权限级别、到期状态筛选与实例/数据库/有效期表头排序，搜索用户/库/表；有效期按 7 天阈值标记「N 天后到期 / 已过期」并弱化过期行，权限级别显示库级/表级；操作列复制库名或 `库.表`；可按当前筛选结果导出 CSV（UTF-8 BOM）。数据取自 Archery `/query/userprivileges/`，进入页签一次取前 200 条后在客户端筛选、排序、分页，超过 200 条时提示用搜索收窄；普通账号由服务端强制只返回本人记录，超管与资源组管理员可见组内记录 |
| 待办列表（v0.2.28） | 点击顶栏铃铛进入 Archery 的「待办列表」（`/workflow/`，数据接口 `/workflow/list/`）：7 列（申请标题/组/申请类型/申请人/申请时间/审核状态/操作）、申请类型筛选（全部 / 查询权限 / SQL 上线 / 数据归档）、标题搜索、刷新与紧凑显示、服务端分页与「显示第 X 到第 Y 条记录，总共 N 条记录」页脚，铃铛右侧显示待办总数（取不到时只显示铃铛），重复点击铃铛随时回到列表首页（筛选与页码归零）；「审核」按 Archery 的 302 语义把 `audit_id` 解析成业务工单号（`/workflow/<audit_id>/` → `/detail/`、`/queryapplydetail/`、`/archive/`），SQL 上线与查询权限工单直接进插件内详情页完成审核，数据归档工单提示到 Archery 网页处理 |
| 申请工单 / 权限申请（v0.1.8） | 点击左侧「权限管理」默认选中「申请工单」页签（已在页面内重复点击也会回到该页签），保留 11 列权限申请工单（工单名称/实例/数据库/权限级别/结果集/有效时间/工单状态/申请人/申请时间/组/操作）、服务端搜索与分页、行内展开库表清单、操作日志弹窗（与申请/审核同源抬头，表格改卡片、操作信息可换行读全、页脚显示条数）；「权限申请」按钮打开弹窗，按「组 → 实例 → 数据库/表」级联，支持 DATABASE（多库）与 TABLE（MySQL 单库多表）两种级别、一天/一周/一月/一年授权时间、查询限制行数；申请弹窗按「申请说明 / 授权范围 / 授权时长」编号三段排版，每个字段独立标签 + 必填星标，短字段两列并排，授权时间改为四档分段控件、行数上限带「行」后缀与 100/500/1000 常用值胶囊，下拉加载中显示转圈，窄面板自动收成单列；待审核工单可审核通过或终止流程（备注必填，审核弹窗同为编号两段「申请内容 / 审核意见」，工单卡两列对齐、库表清单按顿号换行，终止流程为红色描边按钮）；选中资源组后回填该组在查询权限工单下的审批流程（v0.2.7） |
| 工单详情（v0.1.9） | 点工单名称进入详情页：工单名称、审批流（已通过 / 当前节点与审批人 / 未到达 / 无需审批，箭头连接）、其他信息 9 列表格、数据库或表清单、最后操作信息；详情页可直接审核与查看操作日志；任一区块解析不到时回退列表行数据 |
| SQL 上线（v0.2.0） | 「SQL 上线」列表对齐 Archery 网页：状态/实例/组/日期区间筛选、搜索、服务端分页、10 列工单表格与工单日志；「提交SQL」为整页表单（左侧 SQL 编辑器 + 右侧上线单名称/需求链接/组→实例（can_write）→库/执行时间窗/审批流程提示，底部检测结果表格：ID/SQL语句/扫描影响行数/审核状态/审核信息，可展开完整 SQL），支持 `.sql` 文件导入（≤10M，追加到编辑器）与美化；SQL 检测无 error 才允许提交，仅告警时二次确认；工单详情对齐 Archery 网页：标题区带状态色带与状态胶囊、实例/库/组/发起人/时间元信息行、审批流编号时间线（已通过 ✓ / 当前节点实心并列出审批人 / 未到达虚线；已结束工单若详情页解析不出节点状态，整条按已通过渲染）、其他信息键值卡片（按字段数自动选列：8→4、9→3、10→5，最后一行不留空槽）、工单详情/工单日志页签（带条数徽章，搜索框常驻页签行；v0.2.21：日志按操作时间倒序，页签行右侧可按操作/操作人/操作信息搜索，表格下方显示第 N 到第 M 条与总条数）、SQL 列表（ID/状态/信息/影响行数/执行与备份耗时/当前阶段，数字列右对齐、空值弱化、记录数旁给出「N 条警告 / N 条错误」小结，行末可一键复制该条 SQL，支持逐行或全部展开完整 SQL）、操作信息，以及固定在面板底部（不随内容滚动）的「查看回滚SQL / 下载回滚SQL」操作条；「查看提交信息」不再弹窗，点击后整页进入提交 SQL 页并按原工单预填名称/SQL/组/实例/库/备份/执行时间窗（预填 SQL 自动去掉 `use 库名` 切库语句；来源工单号在页头提示，返回即回原详情）；「上线其他实例」（v0.2.20）位于「查看提交信息」左侧，点击同样整页进入提交页，但目标实例与库留空，名称/SQL/组/备份/执行时间窗照原工单带入，需重新选目标并检测后才可提交；「查看回滚SQL」为独立子页（v0.2.15）：执行语句/回滚语句两列表格（数据取 `backup_sql` 的 `[执行语句, 回滚语句]` 二元组，按执行顺序倒序），支持搜索、逐行/全部展开全文；「下载回滚SQL」详情页与子页均可直接下载（`rollback_{id}.sql`，每段 `/*执行语句*/\n回滚语句`，对齐 Archery 下载格式）；「提交回滚请求」按 Archery 规则（`【回滚工单】原工单Id:{id} ,{原名}`）新建回滚工单（不开启备份），组/目标实例/数据库锁定为原工单值、弹窗内不可修改，按名称匹配不到任一项时禁止提交；弹窗内直接列出将提交的每条执行语句与回滚语句全文（goInception 的多行 DDL 压成单行预览，引号内空白不动）；可审核通过/驳回、立即执行（平台排队）或标记线下完成、设置定时执行与终止流程；「待我审核」只列出当前轮到我的工单：sidecar 取 Archery `/workflow/list/`（`current_audit` 命中我的审核组）与待审核工单求交集，状态固定为待审核（`/sqlworkflow_list_audit/` 在 v1.14 与普通列表同源，不再使用）；列表/详情/提交页顶部公告取自 Archery 首页 alert 公告（与查询页同源，取不到不显示）；只读连接拦截全部写操作；v0.2.29：「可执行时间变更」在详情页底栏与「审核通过」同条件显示（仅待审核工单），弹窗按日期时间范围改写可执行窗口并预填当前值，两端留空即清空为「无限制」（Archery `/alter_run_date/`），保存后原地刷新详情 |
| 表结构 | 表名搜索下拉（候选来自当前库，支持键盘上下选择）；结果面板按表多开 tab（同一张表再点一次原地刷新，跨库重名时标题带库名，见 v0.2.38），展示 `SHOW CREATE TABLE` 的 DDL、查询耗时与获取时间，响应没有 DDL 时回退到列名/类型/可空/索引/备注表格（DDL 直接展示，不挂「Create Table」小标题）；tab 高度随内容变化（v0.2.37）：短 DDL 就地收缩不再撑满面板，长 DDL 继续长高、溢出交给整个查询列滚动，面板内不再出现滚动条，空态/加载态保留 220px 面板下限并居中 |
| 会话可靠性 | 会话过期自动重登，并发请求只登录一次；2FA 待验证状态在 UI 内完成 |
| 会话复用（v0.1.6） | 登录态（`sessionid` / `csrftoken`）落盘复用：重开连接时先探测会话，仍有效就直接进控制台，不再输密码和验证码；只有服务端判定空闲超时才重新验证 |
| 会话自动续期（v0.2.61） | 工作台打开期间后台静默心跳：按「会话续期间隔」（默认 120 秒，连接配置可改，0 表示关闭）发一个无副作用的登录态 GET（`/group/user_all_instances/`），把 Archery 的最后活动时间推到最新，长时间不操作也不会掉登录态；心跳发现会话已被服务端回收时用连接里保存的密码自动重登（无 2FA 完全静默），2FA 账号仍弹验证码框；网络类失败按 60s→120s→240s→最长 5 分钟退避重试，不打扰用户；确认需要人工验证时顶栏转为「会话已过期」并停止心跳，避免反复弹框，完成验证后自动恢复 |

### 安装

```powershell
# 1. 打包（需要 Go 1.22+，与 backend/go.mod 一致，并把 go 放进 PATH）
npx --yes @dbx-app/plugin-cli@0.1.9 package .

# 2. DBX 插件中心 → 设置 → 允许未签名开发包 → 从文件安装
#    dist/io.github.nixing178.archery-0.1.0-windows-x64.dbxp
```

### 连接配置

| 字段 | 说明 |
| --- | --- |
| 协议 / IP / 端口 / 上下文路径 | 例如 `https` + `archery.example.com` + `443` + `/archery` |
| 用户名 / 密码 | Archery 账号；密码只在 sidecar 进程内存中使用，不落盘 |
| 认证方式 | `会话登录`（推荐，普通用户可用）/ `JWT`（实验性，仅超管） |
| 2FA 验证方式 | `身份验证器（TOTP）` 或 `短信验证码`，默认 TOTP；验证码框不再让用户选方式 |
| 2FA 手机号 | 仅短信验证方式需要，缺失时连接会直接报配置错误 |
| 只读模式 | 默认开启；查询不受影响，写操作（工单）将在 v0.2 被拦截 |
| 默认返回行数 | 查询未指定时的 `limit_num`，默认 100 |
| 查询 / 连接超时 | 默认 120s / 20s |

开启 2FA 的账号：保存连接时只做密码校验，插件内首次打开工作台会弹出验证码输入框；之后只要 Archery 侧会话还有效（未空闲超时），重开连接不会再要验证码。

### 兼容性

| Archery 版本 | 登录 | 实例/库/表 | 查询/历史/收藏 | 说明 |
| --- | --- | --- | --- | --- |
| v1.10 – v1.14（会话） | 支持 | 支持 | 支持 | 主要路径，按 v1.14.0 源码核验 |
| master / 新 API | 支持 | 支持（能力探测） | 支持 | 优先 `/api/v1/sqlquery/*`，不存在时回退会话接口 |
| JWT 模式 | 支持 | 部分 | 仅新版 API | `/api/v1/instance/` 默认只允许超管，普通账号请用会话模式 |

会话模式使用 Archery 的 Web 端点（`/authenticate/`、`/group/user_all_instances/`、`/instance/instance_resource/`、`/query/`、`/query/querylog/`、`/query/userprivileges/`），仪表盘指标额外抓取 `/dashboard/` 并解析 `info-box-number`；版本升级若调整了这些路径，插件需要同步适配。

### 开发

```powershell
pnpm install --ignore-scripts
pnpm test              # Vitest：结果归一化/导出、树过滤、i18n key 对齐
pnpm typecheck
pnpm build             # 版本一致性 + vue-tsc + 单文件构建 → ui/index.html
node scripts/make-preview.mjs   # 免安装预览 → preview/index.html
node scripts/smoke-preview.mjs  # Edge 无头跑通「展开库 → 查询 → 表结构」并截图

cd backend
go test ./...          # 会话/CSRF/2FA/重登单飞/结果归一化/只读守卫
go vet ./...
```

`node scripts/smoke-preview.mjs` 会把各页面截图写到 `preview/`（仅本地生成，不入库）。预览默认浅色，追加 `?theme=dark` 可切换深色。

真实实例联调（不填密码只验证 CSRF/登录握手）：

```powershell
$env:ARCHERY_URL="https://archery.example.com"
$env:ARCHERY_USER="demo_user"
$env:ARCHERY_PASSWORD="<密码>"   # 留空则只跑到登录失败为止
cd backend
go test -tags live ./internal/archery -run TestLiveArchery -v
```

填了密码会继续验证：实例列表 → 第一个实例的库列表 → `select 1 as ok` 查询。

### 架构与安全

```
UI (opaque-origin iframe)  --invoke-->  Go sidecar  --HTTP(S)-->  Archery
```

- 插件 UI 不能直连外网，全部请求由 sidecar 代理，CookieJar、CSRF、TLS 校验都在 sidecar 内处理。
- 密码只在 sidecar 内存中保存；插件数据目录写入 `ui-state.json`（标签页与布局，按 Archery 环境分组，不同环境的连接互不可见）与 `session.json`（会话 Cookie，权限 0600，只存 `sessionid` / `csrftoken`）。删除 `session.json` 等同于插件内退出登录。
- sidecar 进程随连接开关重启，因此会话只能靠落盘复用。会话有效期最终由 Archery 的 Django 会话策略决定：工作台打开期间插件会按 `keep_alive_secs`（默认 120 秒）发一个无害登录态 GET 续期，这依赖 `SESSION_SAVE_EVERY_REQUEST` 之类的「按活动时间顺延」配置；若服务端使用固定到期时间，心跳无法延长 Cookie，但会话一旦被判失效，下一次心跳或用户操作会自动用保存的密码重登（2FA 账号仍需验证码）。
- 所有写方法经过只读守卫（`read_only`）。SQL 上线工单的提交、审核、执行与定时会真正改库，因此在只读连接下全部拦截；查询权限的申请与审核只写 Archery 平台工单、不动数据库，因此放行。
- 支持内网自签证书（连接表单显式开启「跳过 TLS 证书校验」）。

### 路线图

- v0.2.0（已发布）：SQL 上线工单（提交、我的工单、待我审核、通过/驳回、立即/定时执行、工单日志）
- v0.3.0：SQL 分析、只读 MCP 工具、商店候选包

---

## English

Browse [Archery](https://github.com/hhyo/Archery) instance resources, run SQL queries, and inspect query history and table structures inside the DBX workbench.

- Plugin ID: `io.github.nixing178.archery`
- Version: `0.1.0`
- Shape: Go sidecar + single-file Vue3 console (visuals aligned with the Archery 1.14 web UI: Bootstrap 3 / SB Admin 2 header, sidebar and panel metrics, CodeMirror 6 editor; panel radii, table headers, forms and tabs share one scale across pages)
- License: Apache-2.0

### Features (v0.1.0)

| Area | Details |
| --- | --- |
| Console | `Archery(connection)` header with user area and a six-entry sidebar (Dashboard, SQL audit, SQL query, SQL tuning, Plugins, Docs) with expandable sub menus (SQL query opens by default: online query / data dictionary / privileges); the last page is restored on reopen; Archery system announcements show at the top of every page when present (read from the dashboard alert block, reused for 60s, refreshed on page switches) |
| Dashboard | Four metric cards (release/query tickets, active users, instances) plus instance-type bar and donut charts; counters are matched by card label rather than template order, so extra info boxes no longer shift them; the instance-type charts plus the daily "query stats" (rows/scans line) and "release stats" (tickets per day bar) all read the chart data embedded in the Archery dashboard page, the two daily charts have a date-range picker (7/30/365 days, this month, this year, last month, last year, custom — no \"all\", default last 7 days) that refetches just those charts through `/dashboard/api/?start_date=&end_date=` (the dashboard page itself ignores date params), falling back to placeholders or the account-visible list only when unavailable; counters degrade to em dashes instead of blocking the page |
| Auth | Archery session login with Django CSRF handling; TOTP / SMS 2FA |
| Resource tree | Instance → database → table, lazy loading with search; double-click a table to describe it |
| Data dictionary | Nothing is preselected on entry; after picking an instance and database it lists tables with comments grouped A–Z (comments come from Archery `/data_dictionary/table_list/` and require the `sql.menu_data_dictionary` menu permission — without it only table names show, with a hint); the letter index jumps and shows per-letter counts, search matches table names or comments; clicking a table opens the metadata dialog (table info / columns / indexes / create statement from `/data_dictionary/table_info/`), where the table name can be copied |
| Query | Multi-tab editor, SQL highlighting, `Ctrl+Enter` (selection first — multiple selected statements run one by one, each in its own result tab; otherwise the statement at the cursor), searchable instance / database dropdowns (one click opens, only the first 200 candidates are rendered), saved-query picker, format, explain, table-structure lookup, row limit; each tab keeps its own editor state, so cursor, selection and undo history never leak across tabs (v0.2.59) |
| Result grid | Column type hints, NULL styling, column resizing, pagination (per-page options 30 / 50 / 100 / 200 / 500 / 1000, defaulting to 30, capped by the query row limit with the limit itself as a step; the grid height follows the page size, using the same pager layout as History), search (typing in the toolbar filters the visible columns case-insensitively, highlights matches inside cells and shows the match count; Ctrl+F focuses it and Esc clears it; copy and export include only matching rows while searching), a copy dropdown (CSV / TSV / Markdown merged into one "Copy ▾" button), column-settings popover (checkboxes toggle each column's visibility with search by name or type, fuzzy / prefix / exact matching, ↑↓ to move, Enter to toggle, Esc to close, a footer showing the hidden count with one-click "Show all", at least one column always kept, and CSV / TSV / Markdown plus Excel export limited to the visible columns and, while searching, to matching rows), export to Excel (.xlsx with every row of the current result tab — only matching rows while searching — bold header, frozen first row and auto filter; a native Save As dialog lets you pick the location and remembers the last folder; prompts to lower the row limit above 5 MB) |
| Result tabs (v0.2.38) | The inner tabs belong to the current editor tab, so switching editors switches the whole set: "Messages / History" are pinned first and cannot be closed, "Result N" refreshes in place on every run (no longer appended per execution), table structures open one tab per table; the +/− buttons at the right of the tab strip open an empty result tab or close the current one (− is disabled while Messages / History / a table structure is active); result and describe tabs carry a × and can be dragged to reorder (system tabs stay put); the row limit moved from the action area into the Target group, next to instance / database / table |
| History | Eight-column grid (actions/user/instance/database/time/statement/rows/cost), instance and database filters, search, favorites, inline expand/favorite/load, pagination |
| My privileges | Eight-column grid of the account's effective query privileges (user/instance/database/level/table/row cap/valid until/actions) with user/database/table search and server-side pagination, backed by Archery `/query/userprivileges/`; Archery itself scopes results to the current user, while superusers and resource-group managers see group records; the actions column is read-only for now |
| Todo list (v0.2.28) | The header bell opens Archery's todo list (`/workflow/`, data from `/workflow/list/`): seven columns (title / group / request type / applicant / applied at / status / actions), a request-type filter (all / query privilege / SQL release / data archive), title search, refresh and compact mode, server-side paging with a "showing X to Y of N records" footer, the pending total shown right of the bell (bell only when the count is unavailable), and repeated bell clicks always land back on the first page of the list (filter and paging reset); "Review" follows Archery's 302 semantics and resolves the `audit_id` into a business ticket id first (`/workflow/<audit_id>/` → `/detail/`, `/queryapplydetail/`, `/archive/`), so SQL release and query privilege tickets open the in-plugin detail page for approval, while archive tickets point at the Archery web UI |
| Requests / privileges (v0.1.8) | The "My requests" tab keeps the eleven-column privilege request grid (ticket/instance/database/level/row cap/valid until/status/applicant/applied at/group/actions), server-side search and pagination, inline database/table list and the workflow log dialog (same header and footer as the request dialogs, the table in a card with wrapping detail text and a record count in the footer); "Request access" opens the dialog that cascades group → instance → database/tables with DATABASE (multi-database) and TABLE (single database, multi-table, MySQL only) levels, day/week/month/year validity and a query row cap; the dialog is laid out as three numbered blocks (request / scope / duration) with a label and required mark per field, short fields paired in two columns, validity as a four-way segmented control, the row cap carrying a unit suffix plus 100/500/1000 quick picks, a spinner while options load, and a single column on narrow panels; pending tickets can be approved or terminated (remark required; the review dialog reuses the same two numbered blocks — requested access / review note — with a two-column ticket card that wraps database and table lists at the separator, and a red outlined terminate action); picking a resource group fills in that group's approval flow for query-privilege tickets (v0.2.7) |
| Ticket detail (v0.1.9) | Click a ticket title to open the detail page: title, approval flow (passed / current node with reviewers / pending / auto-pass, joined by arrows), the nine-column ticket table, the database or table list and the last operation info; the detail page can audit the ticket and open the workflow log; any block that cannot be parsed falls back to the list row |
| SQL release (v0.2.0) | The "SQL release" grid mirrors Archery: status/instance/group/date filters, search, server-side pagination, ten columns and the workflow log; "Submit SQL" is a full page (SQL editor on the left; ticket name, requirement link, group → instance (`can_write`) → database, execution window and approval flow on the right, plus a check-result grid with ID / SQL / scanned rows / status / message and expandable SQL) with `.sql` file import (up to 10 MB, appended to the editor) and formatting; submission stays disabled until the check reports no error, and warnings ask for a second confirmation; the ticket detail mirrors Archery: a status-tinted header band with a status pill and an instance / database / group / submitter / time meta line, a numbered approval timeline (passed / current node filled with its reviewers / not reached; a finished ticket whose page exposes no node states is rendered as fully passed), the ticket fields as a key-value card grid with the column count picked from the field count (8→4, 9→3, 10→5, no empty cells), SQL detail / workflow log tabs with count badges and a resident search box (v0.2.21: logs are newest-first, searchable by action / operator / detail, with an "N to M of total" footer), a SQL grid (id, status, message, affected rows, execute and backup duration, stage; numeric columns right-aligned, empty cells dimmed, warning/error counts summarised next to the record count, per-row copy-SQL action, per-row or expand-all full SQL), last operation info, and a "View / Download rollback SQL" action bar pinned to the panel bottom; "View rollback SQL" is a dedicated sub page (v0.2.15) with an executed-SQL / rollback-SQL two-column grid (rows come from `backup_sql`'s `[executed, rollback]` pairs in reverse execution order), search and per-row or expand-all full text; "View submission" reopens the submit page prefilled with the ticket name, SQL, group, instance, database, backup and execution window; "Deploy to another instance" (v0.2.20, to its left) does the same but leaves the instance and database empty so the same SQL can be released on another target; "Download rollback SQL" works from the detail page and the sub page (`rollback_{id}.sql`, one `/*executed*/\nrollback` block per pair, matching Archery's download format); "Submit rollback request" prefills group/instance/database and creates a rollback ticket following Archery's naming (`【回滚工单】原工单Id:{id} ,{name}`, no backup); approve/reject, platform or manual execution, scheduling and abort; read-only connections block every write; v0.2.29: "Change execution window" sits in the detail action bar next to "Review" under the same condition (pending tickets only); the dialog rewrites the window through a start/end picker prefilled from the ticket and clears it to unlimited when both ends are empty (Archery `/alter_run_date/`), refreshing the detail in place afterwards |
| Describe | Table search dropdown (candidates from the current database, keyboard navigation); the result panel opens one tab per table (re-opening the same table refreshes it in place, and the title gains the database name when two databases share a table name — v0.2.38) showing the `SHOW CREATE TABLE` DDL with query duration and fetch time, falling back to a column/type/nullable/key/comment grid when the response has no DDL (the DDL is shown as-is, without a "Create Table" caption); the tab height follows its content (v0.2.37): a short DDL shrinks the panel instead of stretching it, a long DDL keeps growing and the whole query column scrolls, with no inner scrollbar in the panel and a 220px floor for the empty/loading states |
| Reliability | Automatic re-login on session expiry with single-flight concurrency; 2FA handled in the UI |
| Session reuse (v0.1.6) | The Django session (`sessionid` / `csrftoken`) is persisted and probed on open: while Archery still accepts it, reopening the connection skips both password and OTP. Re-verification happens only after the server-side idle timeout |
| Session keep-alive (v0.2.61) | While the workbench is open the sidecar sends a harmless authenticated GET (`/group/user_all_instances/`) every `keep_alive_secs` (default 120 s, configurable per connection, 0 disables), keeping Archery's last-activity timestamp fresh so an idle workbench no longer loses its session. If the server has already dropped the session, the heartbeat re-logs in with the saved password (silent without 2FA; 2FA accounts still get the OTP dialog). Network failures back off 60 s → 120 s → 240 s → at most 5 min without bothering the user. Once human verification is genuinely required the status pill switches to "Session expired" and the heartbeat stops, so the OTP dialog is not reopened every minute; it restarts automatically after a successful verification |

### Install

```powershell
# Requires Go 1.22+ (matching backend/go.mod) on PATH
npx --yes @dbx-app/plugin-cli@0.1.9 package .
# DBX plugin center → allow unsigned dev packages → install from file
```

### Compatibility

| Archery | Login | Metadata | Query | Notes |
| --- | --- | --- | --- | --- |
| v1.10 – v1.14 (session) | Yes | Yes | Yes | Primary path, verified against v1.14.0 sources |
| master / new API | Yes | Yes (capability probe) | Yes | Prefers `/api/v1/sqlquery/*`, falls back to session endpoints |
| JWT mode | Yes | Partial | New API only | `/api/v1/instance/` is superuser-only by default; use session mode for normal accounts |

### Development

```powershell
pnpm install --ignore-scripts
pnpm test
pnpm typecheck
pnpm build
node scripts/make-preview.mjs
node scripts/smoke-preview.mjs

cd backend
go test ./...
```

### Security

The plugin UI is an opaque-origin iframe and never talks to Archery directly. All HTTP traffic, cookies, CSRF tokens and optional TLS-skip handling live in the Go sidecar. The password stays in sidecar memory. Two files are written to the plugin data directory: `ui-state.json` (tabs and layout, grouped per Archery environment so connections to different environments never see each other's tabs) and `session.json` (session cookies, mode 0600, only `sessionid` / `csrftoken`); deleting `session.json` is the equivalent of logging out of the plugin. Session lifetime is ultimately decided by Archery's Django settings: while the workbench is open the plugin sends a harmless authenticated GET every `keep_alive_secs` (default 120 s) to keep the last-activity timestamp fresh, which relies on Django sliding the expiry on activity (`SESSION_SAVE_EVERY_REQUEST` and friends); with a fixed expiry the heartbeat cannot extend the cookie, but as soon as the server rejects the session the next heartbeat or user action re-logs in with the saved password (2FA accounts still need a code). SQL release submit/review/execute/schedule really change the database, so the read-only guard blocks all of them; privilege requests and reviews only create Archery-side tickets, so they bypass the guard.
