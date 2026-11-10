# 测试与验证证据

日期：2026-10-30。环境：Windows + PowerShell，Go 1.22.12，Node 24.15，pnpm 10.11，DBX 插件 CLI 0.1.9。

## 1. 后端红图（实现前）

`cd backend && go test ./internal/archery/...`

```text
--- FAIL: TestLoginAndInstances (0.01s)
    client_test.go:47: EnsureSession: not implemented
--- FAIL: TestLoginWrongPassword (0.00s)
--- FAIL: TestTwoFAFlow (0.00s)
--- FAIL: TestTwoFAWrongCode (0.00s)
--- FAIL: TestReloginAfterSessionExpiry (0.00s)
--- FAIL: TestConcurrentReloginSingleFlight (0.00s)
--- FAIL: TestQueryNormalizesObjectRows (0.00s)
--- FAIL: TestQueryServerError (0.00s)
--- FAIL: TestQueryForbidden (0.00s)
--- FAIL: TestResourcesAndDescribe (0.00s)
--- FAIL: TestQueryLogsAndFavorite (0.00s)
--- FAIL: TestVersionInfo (0.00s)
--- FAIL: TestJWTMode (0.00s)
FAIL	github.com/nixing178/dbx-plugin-archery/internal/archery	0.249s
```

`go test ./internal/rpc/...`（桩实现阶段）

```text
--- FAIL: TestConnectRequiresConnectionID
--- FAIL: TestNotConnected
--- FAIL: TestConnectOverviewInstances
--- FAIL: TestQueryLimitRules
--- FAIL: TestReadOnlyGuard
--- FAIL: TestTwoFAConnectPending
--- FAIL: TestUIStateRoundTrip
--- FAIL: TestDisconnect
--- FAIL: TestErrorMessageCarriesKind
```

## 2. 后端绿图

```text
$ go vet ./...
（无输出）

$ go test ./...
?   	github.com/nixing178/dbx-plugin-archery	[no test files]
ok  	github.com/nixing178/dbx-plugin-archery/internal/archery	0.619s
ok  	github.com/nixing178/dbx-plugin-archery/internal/rpc	0.608s
```

期间修正两个真实缺陷：

1. 登录请求未带 `X-CSRFToken`（只在已登录请求里加），导致 `/authenticate/` 403。
2. 并发重登：`resetAuth` 清空会话的瞬间被其他请求看到，触发多次登录；改为 `EnsureSession` 在 `loginMu` 内二次检查。

## 3. 前端红图

`pnpm test`（i18n 模块尚未创建）

```text
✓ src/lib/result.test.ts (10 tests)
✓ src/lib/tree.test.ts (4 tests)
❯ src/lib/i18n.test.ts (0 test)
Error: Failed to load url ../i18n ... Does the file exist?
Test Files  1 failed | 2 passed (3)
```

## 4. 前端绿图

```text
✓ src/lib/result.test.ts (10 tests)
✓ src/lib/tree.test.ts (4 tests)
✓ src/lib/i18n.test.ts (3 tests)
Test Files  3 passed (3)
     Tests  17 passed (17)
```

覆盖：CSV/TSV/Markdown 转义、NULL、分页边界、数字/耗时格式、树过滤与去重、中英文 key 集合一致。

## 5. 构建与免安装预览

```text
$ pnpm typecheck
（无输出，退出码 0）

$ pnpm build
version check ok: 0.1.0
ui/index.html  548.82 kB │ gzip: 184.47 kB
✓ built in 12.55s
```

`node scripts/smoke-preview.mjs`（Edge headless，真实交互）

```json
{
  "checks": {
    "instance": true,
    "editor": true,
    "toolbar": true,
    "tables": true,
    "rows": true,
    "resultText": true,
    "describe": true
  },
  "errors": []
}
```

以上断言由 `scripts/smoke-preview.mjs` 自动校验（运行时才写出截图，截图不入库）。

## 6. 打包与 sidecar 身份

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Packaging: dist/io.github.nixing178.archery-0.1.0-windows-x64.dbxp
Success: Built dist/io.github.nixing178.archery-0.1.0-windows-x64.dbxp
Signature: unsigned review candidate
```

包内容：

```text
assets/connection.svg
assets/plugin.svg
bin/windows-x64/dbx-plugin-archery.exe
manifest.json
ui/index.html
checksums.json
```

`plugin/initialize` 直连 sidecar：

```json
{"id":1,"jsonrpc":"2.0","result":{"capabilities":["connections"],"plugin":{"id":"io.github.nixing178.archery","version":"0.1.0"},"protocolVersion":1}}
```

## 7. v0.1.1：2FA 方式收进连接配置

红图：

```text
--- FAIL: TestTwoFAUsesConfiguredType
    client_test.go:132: Login with configured sms: 短信验证码校验失败！
--- FAIL: TestSMSTypeWithoutPhone
    client_test.go:147: expected invalid_config for missing phone, got 短信验证码校验失败！
--- FAIL: TestTwoFAConnectPending
    service_test.go:158: expected twofaType totp, got map[...]
--- FAIL: TestTwoFAConnectPending
    service_test.go:161: expected username in connect result, got map[...]
```

绿图：

```text
$ gofmt -l internal   # 仅格式化 live_test.go
$ go vet ./...        # 无输出
$ go test ./...
ok  	.../internal/archery	0.687s
ok  	.../internal/rpc	0.882s
```

前端冒烟（新增 2FA 断言）：

```json
{
  "checks": {
    "otpDialog": true,
    "otpNoSelector": true,
    "otpCompleted": true
  },
  "errors": []
}
```

冒烟断言 `otpDialog` / `otpNoSelector` 覆盖这一点（只有「验证方式：身份验证器 + 验证码」，无短信选项/手机号输入）。

打包：

```text
Success: Built dist/io.github.nixing178.archery-0.1.1-windows-x64.dbxp
manifest: id=io.github.nixing178.archery version=0.1.1
fields: ... auth_mode, twofa_type, twofa_phone, ...
plugin/initialize -> {"plugin":{"id":"io.github.nixing178.archery","version":"0.1.1"},"protocolVersion":1}
```

## 8. 真实实例探测（https://archery.example.com）

```text
GET /api/info        -> 200 {"archery": {"version": "1.14.0"}, ...}
GET /login/          -> 200，Set-Cookie: csrftoken=...，页面含 csrfmiddlewaretoken
POST /authenticate/  -> 200 {"status": 1, "msg": "用户名或密码错误，请重新输入！", "data": ""}
                        （用错误密码探测，说明 X-CSRFToken + 表单编码被 Archery 接受，未返回 403）
```

`go test -tags live ./internal/archery -run TestLiveArchery -v`（无密码）：

```text
live_test.go:42: Archery version = 1.14.0
live_test.go:49: CSRF/login handshake reached Archery: 用户名或密码错误，请重新输入！
--- PASS: TestLiveArchery (0.49s)
```

未完成：真实密码登录 → 实例列表 → 库列表 → `select 1`（需要密码）。

## 9. v0.1.2：修复「invalid params: missing connection id」

红图（复现宿主报错原文）：

```text
--- FAIL: TestConnectWithoutTopLevelConnectionID
    service_test.go:94: connect without connectionId: invalid params: missing connection id
```

根因：DBX 生命周期回调 `connection/connect` 只下发 `connection.id`，不带顶层 `connectionId`；原实现只读顶层字段。

绿图：

```text
$ go test ./...
ok  	.../internal/archery	(cached)
ok  	.../internal/rpc	0.341s
```

打包后直接向 sidecar 模拟宿主回调（只带 `connection.id`，指向 127.0.0.1:9）：

```text
id=2 connection/connect -> {"code":-32000,"message":"network: 无法连接 Archery：Get \"http://127.0.0.1:9/login/\": ..."}
```

说明已越过 id 校验、进入真实网络阶段；`archery/instances` 仍返回 `not_connected`（connect 未成功，符合预期）。

## 10. v0.1.3：修复工作台「invalid params: missing connection id」

现象：0.1.2 打开工作台报 `Plugin internal error: invalid params: missing connection id`。

根因：`window.dbxPlugin.context` 在 `ready` 之后才填充，旧实现 `onMounted` 里先读 context 再 `await ready`，`connectionId` 恒为空。

前端红图（预览 mock 改成「ready 后填 context + invoke 校验 connectionId」后，冒烟直接卡在实例树）：

```text
locator.click: Timeout 30000ms exceeded.
- waiting for locator('.tree-row').filter({ hasText: 'shop' })...
```

后端红图（无 id 的单会话兜底）：

```text
--- FAIL: TestInvokeWithoutConnectionIDUsesSingleSession
    ui connect without id: invalid params: missing connection id
--- FAIL: TestInvokeWithoutConnectionIDAmbiguous
    expected not_connected for ambiguous sessions, got invalid params: missing connectionId
```

绿图：

```text
$ go test ./...
ok  	.../internal/archery	(cached)
ok  	.../internal/rpc	0.793s

$ pnpm test
Test Files  4 passed (4)
     Tests  23 passed (23)

$ node scripts/smoke-preview.mjs
{ "checks": { "instance": true, ..., "otpCompleted": true }, "errors": [] }
```

打包后驱动 sidecar（假 Archery，完整复现宿主生命周期）：

```text
host connect (connection.id only): {"connectionId":"conn-1","success":true,...}
ui connect (no id):                {"connectionId":"conn-1","success":true,...}
instances (no id):                 {"instances":[{"name":"fake-mysql",...}]}
PROBE PASS
```

## 11. v0.1.4：SQL 查询页对齐 Archery 原生体验

范围：右栏配置区（实例/库/查看表结构/行数 + 美化/执行计划/SQL 查询）、常用查询下拉、光标语句执行、查询历史 8 列表格与实例/库筛选。

R1 红图（`src/lib/sql.ts` 只留签名）：

```text
$ pnpm typecheck
exit 0
$ pnpm test
Test Files  1 failed | 6 passed (7)
     Tests  15 failed | 50 passed (65)
```

R1 绿图：

```text
$ pnpm test
Test Files  7 passed (7)
     Tests  65 passed (65)
```

R2 红图（`src/lib/history.ts` 占位）：

```text
$ pnpm test -- src/lib/history.test.ts
Test Files  1 failed (1)
     Tests  5 failed | 2 passed (7)
```

R2 绿图：

```text
$ pnpm test
Test Files  8 passed (8)
     Tests  72 passed (72)
```

后端红图（查询历史未透传实例/库筛选）：

```text
--- FAIL: TestQueryLogsAndFavorite
    client_test.go:374: instance/db filters not forwarded: map[limit:[20] search:[select] star:[true]]
```

后端绿图：

```text
$ go test ./...
ok  	.../internal/archery	0.342s
ok  	.../internal/rpc	0.447s
$ go vet ./...
(no output)
```

预览冒烟（1440×900，Edge 无头）：

```text
{ "checks": { "configPane": true, "configActions": true, "favorites": true,
  "beautify": true, "explain": true, "structurePrompt": true, "structurePanel": true,
  "historyColumns": true, "historyRows": true, "historyFilters": true, "historyExpand": true },
  "errors": [] }
```

以上断言由 `scripts/smoke-preview.mjs` 自动校验（运行时才写出截图，截图不入库）。

### R3 补遗：系统公告 + 结果列宽拖拽 + 打包

公告解析红图：

```text
--- FAIL: TestParseAnnouncement
    dashboard_test.go:96: expected announcement
--- FAIL: TestParseAnnouncementWithoutHeading
    dashboard_test.go:116: expected announcement
```

绿图：

```text
--- PASS: TestParseAnnouncement (0.00s)
--- PASS: TestParseAnnouncementWithoutHeading (0.00s)
--- PASS: TestParseAnnouncementMissing (0.00s)
ok  	.../internal/archery	0.836s
```

预览冒烟新增检查：

```text
"announcement": true, "columnResize": true, "errors": []
```

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Success: Built dist/io.github.nixing178.archery-0.1.4-windows-x64.dbxp
Signature: unsigned review candidate
产物 4.73 MB
```

## 12. v0.1.5：页面 UI 还原 Archery 网页

基准：Archery 1.14 网页（AdminLTE 3 / Bootstrap 4）。

改动范围（只动样式与品牌区，不改业务结构）：

- 色板：primary `#007bff`、page-bg `#f4f6f9`、border `#dee2e6`、success/warning/danger 标准色；深色主题按同结构映射
- 度量：顶栏 57px、侧栏 220px、菜单项 40px/14px、page-bar 44px、按钮 31px、表单 30px、表头/单元格 8-12px 内距、正文字号 14px
- 层次：查询页改为灰底 + 实例树卡片 + 工作区卡片（3px 圆角 + AdminLTE 阴影）；多标签与面板 tab 改为下划线激活态
- 品牌区：新增蓝色 `A` logo
- 公告条：info/warning/danger 使用 Bootstrap alert 配色，深色主题单独覆盖

验证：

```text
$ pnpm typecheck
exit 0
$ pnpm test
Test Files  8 passed (8)
     Tests  72 passed (72)
$ pnpm build
ui/index.html 890.2 kB (gzip 274.6 kB)
$ node scripts/smoke-preview.mjs
32 项检查全 true（新增 darkTheme），"errors": []
```

以上断言由 `scripts/smoke-preview.mjs` 自动校验（运行时才写出截图，截图不入库）。

打包：`dist/io.github.nixing178.archery-0.1.5-windows-x64.dbxp`（4.73 MB，unsigned review candidate）。

修复点：
1. `onMounted` 先 `await plugin.ready`，再读 context；`resolveConnectionId` 兼容 `connectionId / connection.id / id`。
2. sidecar 在 `connectionId` 缺失时，若当前恰好一个会话则复用它；多会话返回 `not_connected`。
3. 所有 RPC 错误消息统一 `<kind>: <detail>` 前缀，UI 可正确本地化（不再落到 `internal`）。

## 11. 未覆盖 / 待实测

- 真实实例只验证到「登录握手 + 错误密码」：`https://archery.example.com` 的 `demo_user` 未提供密码，实例/库/查询/历史尚未真实跑通。
- JWT 模式仅按接口实现，未在 v1.14.0 实测（默认权限类只允许超管）。
- 短信 2FA 未实测（需要绑定手机号的账号）。

## 13. v0.1.6：会话落盘复用（不再每次要验证码）

根因：sidecar 进程随连接开关重启（`plugin-data/io.dbx.ssh/sidecar-exit.log` 里多个不同 pid 的 `serve-start`），CookieJar 与会话表都只在内存（`client.go` / `service.go`），所以每次打开都是全新登录 → Archery 返回 2FA 临时会话 → 必然弹验证码。网页版不用重验，是因为浏览器一直握着 Django 的 `sessionid`。

红图：

```text
$ go test ./...
# github.com/nixing178/dbx-plugin-archery/internal/archery [build failed]
vet: internal/archery/client_test.go:437:11: undefined: NewFileSessionStore
FAIL    .../internal/archery [build failed]
--- FAIL: TestConnectRestoresPersistedSession (0.01s)
    service_test.go:347: expected restored session without 2FA,
    got map[... pendingTwofa:true twofaType:totp username:tester ...]
FAIL    .../internal/rpc
```

绿图：

```text
$ go vet ./...
exit 0
$ go test -count=1 ./...
?   .../dbx-plugin-archery   [no test files]
ok  .../internal/archery     0.655s
ok  .../internal/rpc         0.659s

$ go test -count=1 ./internal/archery -run TestRestoreSession -v
--- PASS: TestRestoreSessionSkipsLoginAnd2FA      (0.19s)
--- PASS: TestRestoreSessionDropsExpiredSnapshot (0.15s)
$ go test -count=1 ./internal/rpc -run TestConnectRestoresPersistedSession -v
--- PASS: TestConnectRestoresPersistedSession    (0.13s)
$ node scripts/check-version.mjs
version check ok: 0.1.6
```

实现：

1. `internal/archery/session_store.go`：`session.json` 原子写（tmp+rename，0600），key = `sha256(baseURL + username)`，只存 `sessionid` / `csrftoken`。
2. `client.go`：`NewClient` 载入快照；`EnsureSession` 首次调用做一次 `GET /group/user_all_instances/` 探测，200 即复用会话（探测本身也算活动，服务端按活动续期）；探测失败或网络错误分别走「删快照回退登录」「保留快照」两条路径；`finishLogin` 落盘；`authLost` 时删快照。
3. `Login` 在 2FA 待验证状态只校验验证码，不重跑密码阶段（避免重复发短信）。
4. `rpc/service.go`：懒建 store 注入 client；`connection/test` 传 `nil`，保证保存连接仍是真实密码校验。

前端回归（无 UI 改动，仅版本号变化）：

```text
$ pnpm test
Test Files  8 passed (8) / Tests 72 passed (72)
$ pnpm build
version check ok: 0.1.6
ui/index.html 890.22 kB (gzip 274.63 kB)
```

打包：`dist/io.github.nixing178.archery-0.1.6-windows-x64.dbxp`（4.75 MB，unsigned review candidate，sha256 `bbfc0fa834c96de8bf4c0d12783bb479b03efa174effaac4e51245183f083811`）。

待实测：真实实例上「验码一次 → 关闭再打开连接」，确认不再弹验证码；以及 `session.json` 删除后回到验证码流程。

## 13. 查询权限管理（v0.1.8，2026-10-31）

按 Archery v1.14.0 源码核验端点（`sql/query_privileges.py`、`sql/urls.py`、`sql/templates/queryapplylist.html`、`sql/templates/queryapplydetail.html`、`common/workflow.py`）：

| 用途 | 端点 | 方法 / 参数 |
| --- | --- | --- |
| 申请列表 | `/query/applylist/` | POST `limit, offset, search` → `{total, rows[]}` |
| 提交申请 | `/query/applyforprivileges/` | POST `title, group_name, instance_name, priv_type, db_name, db_list[], table_list[], valid_date, limit_num` |
| 组下拉 | `/queryapplylist/` | GET 页面 `#group_name` 的 option（`user_groups`，仅用户所属组） |
| 组内实例 | `/group/instances/` | POST `group_name, tag_code=can_read` |
| 审核 | `/query/privaudit/` | POST `apply_id, audit_status, audit_remark`；成功 302 回详情页，失败渲染 error.html |
| 操作日志 | `/workflow/log/` | POST `workflow_id, workflow_type=1` |

红图：

```text
$ go vet ./...
exit 0
$ go test ./...
--- FAIL: TestPrivilegeAppliesParsesRows
--- FAIL: TestPrivilegeGroupsParsesSelect
--- FAIL: TestApplyPrivilegeEncodesTableMode
--- FAIL: TestAuditPrivilegeServerMessage
...（archery 包 16 项）
FAIL    .../internal/archery   0.369s
--- FAIL: TestPrivilegeListLimitRules
--- FAIL: TestPrivilegeApplyValidation（6 个子用例）
--- FAIL: TestPrivilegeApplyAllowedInReadOnly
...（rpc 包 11 项）
FAIL    .../internal/rpc       0.355s

$ pnpm test
❯ src/lib/privilege.test.ts (14 tests | 12 failed)
```

绿图：

```text
$ go vet ./...
exit 0
$ go test ./...
ok  .../internal/archery   1.091s
ok  .../internal/rpc       0.506s

$ pnpm test
Test Files  10 passed (10) / Tests 93 passed (93)
$ pnpm typecheck
exit 0
$ pnpm build
version check ok: 0.1.8
ui/index.html 936.47 kB (gzip 286.38 kB)

$ node scripts/smoke-preview.mjs
checks.privilegeColumns = true
checks.privilegeRows = true
checks.privilegeStatus = true
checks.privilegeLog = true
checks.privilegeDialog = true
checks.privilegeAudit = true
errors: []
```

实现要点：

1. `internal/archery/client.go`：申请列表/组实例/申请提交走 `envelope`；组下拉从 `/queryapplylist/` 的 `#group_name` 解析并 `html.UnescapeString` 去重；审核单独走 `fetchPage` 风格流程（`CheckRedirect` 返回 `ErrUseLastResponse`，3xx 视为成功，200 的 `error.html` 用 `<h4>` 取错误文案），避免把 302 误判为会话失效。
2. `internal/rpc/service.go`：新增 7 个方法，列表 `limit` 默认 20 / 上限 200、`offset` 负值归零；申请按 `privType` 校验 `dbList` 或 `dbName + tableList`；申请与审核不加入 `writeMethods`，只读模式放行（只写平台工单）。
3. 前端 `lib/privilege.ts`：状态/级别映射、`validDatePreset` 按 Archery `addDate()` 输出不补零的 `Y-M-D`、表单校验、Django `values()` 与驼峰双键归一化；`views/PrivilegesView.vue` + 申请/审核/日志三个弹窗 + `MultiSelect`，11 列固定布局对齐截图。
4. 预览 `preview/mock.js` 增加 14 条与截图同构的工单（含待审核、审核不通过）与四个弹窗数据；`scripts/smoke-preview.mjs` 新增 6 项断言。

以上断言由 `scripts/smoke-preview.mjs` 自动校验（运行时才写出截图，截图不入库）。

待实测：真实 Archery 账号（具备 `sql.menu_queryapplylist`、`sql.query_applypriv`、`sql.query_review`）上提交一条库级与一条表级申请，确认列表刷新与审批流；用无审核权限账号验证「你无该工单的审核权限」提示。

打包：`dist/io.github.nixing178.archery-0.1.8-windows-x64.dbxp`（5,046,906 字节 / 4.81 MB，unsigned review candidate，sha256 `a446f5aef3b7b106fe8e277d438de52286280906141e1cfa0a2062338009fac9`）；包内 `manifest.json` 版本 0.1.8，`ui/index.html` 与本地构建 sha256 一致，`checksums.json` 覆盖 5 个文件。

## 14. 工单详情页（v0.1.9，2026-11-02）

按 Archery v1.14.0 的 `sql/templates/queryapplydetail.html` + `sql/templates/workflow_display.html` 核验详情页结构：

| 区块 | 渲染形态 | 解析方式 |
| --- | --- | --- |
| 工单名称 | `<h4>工单名称：<span>标题</span></h4>` | 正则取 span 文本 |
| 审批流 | `h5` 内 `span/u` 节点链，节点间 `glyphicon-arrow-right`（模板本身缺 `</h5>`） | 按箭头切块，`text-success` / `text-danger` 判状态，`text-primary` 取当前审批人 |
| 其他信息 | 9 列表格（申请人…组），行 `class="success"` | 前端直接用列表行数据，不二次解析 |
| 数据库/表清单 | `panel-heading` + `panel-body`，`format_str` 输出分行文本 | 按 `<br>` / `</p>` / `</li>` 拆块，单块逗号串再二次拆分 |
| 操作信息 | `last_operation_info` 表格 | `<th>操作信息</th>` 后第一个 `<td>` |
| 审核入口 | `#btnPass` | 沿用既有探测 |

红图：

```text
$ go test ./internal/archery/...
internal\archery\privilege_test.go:360:12: detail.ApplyID undefined (type *PrivilegeDetail has no field or method ApplyID)
internal\archery\privilege_test.go:366:12: undefined: PrivilegeReviewNode
internal\archery\privilege_test.go:383:12: detail.ItemKind undefined
FAIL	github.com/nixing178/dbx-plugin-archery/internal/archery [build failed]

$ pnpm test -- src/lib/privilege.test.ts
Test Files  1 failed (1)
     Tests  10 failed | 14 passed (24)
TypeError: normalizePrivilegeDetail is not a function
```

绿图：

```text
$ gofmt -l internal/archery/models.go   # 无输出
$ go vet ./...
（无输出）
$ go test -count=1 ./...
ok  	.../internal/archery	0.714s
ok  	.../internal/rpc	0.475s

$ pnpm test
Test Files  10 passed (10)
     Tests  103 passed (103)
$ pnpm typecheck
exit 0
$ pnpm build
version check ok: 0.1.9
ui/index.html 944.61 kB (gzip 288.13 kB)

$ node scripts/smoke-preview.mjs
checks.privilegeDetailTitle = true
checks.privilegeDetailFlow = true
checks.privilegeDetailColumns = true
checks.privilegeDetailItems = true
errors: []
```

实现要点：

1. `internal/archery/client.go`：`PrivilegeDetail` 从「只探测 `#btnPass`」扩展为 `parsePrivilegeDetail`，按区块切分标题、审批流、清单与最后操作信息；任一区块缺失返回空值，不整页报错。`models.go` 增加 `PrivilegeReviewNode` 与详情字段。
2. `src/lib/privilege.ts`：`normalizePrivilegeDetail` 兼容驼峰/下划线键并丢弃空名节点（`auto` 除外），`reviewNodeText` 处理当前节点审批人，`detailItems` 在清单类型与工单不一致或为空时回退列表行的 `db_list` / `table_list`。
3. 新增 `views/PrivilegeDetailView.vue`（工单名称 / 审批流 / 其他信息 9 列 / 清单 / 操作信息，底部审核与操作日志入口），列表页工单名称改为可点链接，返回列表时保留原分页数据。
4. 预览 mock 按工单号生成详情（40 号为单节点审核通过，其余给出通过/当前/未到达三节点），新增 4 项冒烟断言。

待实测：真实 Archery 账号打开一条待审核工单，确认审批流当前节点与审批人、`last_operation_info` 文案与网页一致。

打包：`dist/io.github.nixing178.archery-0.1.9-windows-x64.dbxp`（5,058,729 字节 / 4.82 MB，unsigned review candidate，sha256 `7cfb5a7e934ebd2d3db23e23b5ae99c22daa9d513b915cb8a8199779f3836b91`）；包内 `manifest.json` 版本 0.1.9、`ui/index.html` 与本地构建 sha256 一致（`ba7624341035b41cfd465e2631a7974a78f3e8728124829118d94da3dd11fa17`），`checksums.json` 覆盖 5 个文件。

从包里解出 sidecar 驱动 `plugin/initialize`（`python scripts/probe-sidecar.py`）：

```text
initialize: {"id":1,"jsonrpc":"2.0","result":{"capabilities":["connections"],"plugin":{"id":"io.github.nixing178.archery","version":"0.1.9"},"protocolVersion":1}}
host connect (connection.id only): {"connectionId":"conn-1","success":true,"version":"1.14.0"}
ui connect (no id): {"connectionId":"conn-1","success":true,"version":"1.14.0"}
instances (no id): {"instances":[{"id":1,"name":"fake-mysql","type":"master","dbType":"mysql"}]}
PROBE PASS
```

## 15. SQL 上线工单（v0.2.0，2026-11-02）

按 Archery v1.14.0 源码核验端点（`sql/urls.py`、`sql/sql_workflow.py`、`sql/models.py`、`sql/templates/sqlsubmit.html`、`sql_api/urls.py`、`sql_api/api_workflow.py`）：

| 用途 | 端点 | 方法 / 参数 |
| --- | --- | --- |
| 工单列表 | `/sqlworkflow_list/` | POST `navStatus, instance_id, group_id, start_date, end_date, limit, offset, search, syntax_type[]` → `{total, rows[]}`（行 key 保留 `instance__instance_name`） |
| 待我审核 | `/sqlworkflow_list_audit/` | 同上，需 `sql.audit_user` |
| 审核通过 | `/passed/` | POST `workflow_id, audit_remark`；成功 302 回详情，失败渲染 error.html |
| 终止 / 驳回 | `/cancel/` | POST `workflow_id, cancel_remark`（提交人=撤回，审核人=驳回） |
| 立即 / 手工执行 | `/execute/` | POST `workflow_id, mode=auto\|manual` |
| 定时执行 | `/timingtask/` | POST `workflow_id, run_date=YYYY-MM-DD HH:mm` |
| 工单内容 | `/sqlworkflow/detail_content/` | GET `workflow_id` → `{rows[]}`（`stage/stagestatus/errlevel/errormessage/sql/affected_rows/backup_dbname/execute_time`） |
| 工单详情页 | `/detail/<id>/` | HTML：审批流（`workflow_display.html`）+ 操作信息 |
| 筛选下拉 | `/sqlworkflow/` | HTML：状态 / 实例 / 组三个 `<select>`（option `value` = id） |
| 提交工单 | `/api/v1/workflow/` | POST JSON `{workflow:{workflow_name, demand_url, group_id, instance, db_name, is_backup, run_date_start, run_date_end, is_offline_export}, sql_content}` → 201 |
| SQL 检测 | `/api/v1/workflow/sqlcheck/` | POST JSON `{instance, db_name, full_sql}` |
| 工单日志 | `/workflow/log/` | POST `workflow_id, workflow_type=2` |

状态取值（`SQL_WORKFLOW_CHOICES`）：`workflow_manreviewing / workflow_review_pass / workflow_timingtask / workflow_queuing / workflow_executing / workflow_finish / workflow_abort / workflow_autoreviewwrong / workflow_exception`；`syntax_type` 为整数（0 其他 / 1 DDL / 2 DML / 3 离线导出），列表里由 `get_status_display()` 渲染成 DDL/DML。

红图：

```text
$ pnpm vitest run src/lib/workflow.test.ts
FAIL src/lib/workflow.test.ts
Error: Failed to load url ./workflow (resolved id: ./workflow) in src/lib/workflow.test.ts. Does the file exist?
Test Files  1 failed (1)
     Tests  no tests

$ pnpm typecheck
src/lib/workflow.test.ts(12,8): error TS2307: Cannot find module './workflow' or its corresponding type declarations.
src/lib/workflow.test.ts(13,15): error TS2305: Module '"../types"' has no exported member 'SqlSubmitForm'.

$ go vet ./...
（无输出，红测试可编译）

$ go test ./internal/rpc -run "TestSqlWorkflow|TestWorkflowLogAcceptsWorkflowType" -v
--- FAIL: TestSqlWorkflowListParams (0.01s)
    workflow_test.go:474: archery/sqlWorkflows: method_not_found: archery/sqlWorkflows
--- FAIL: TestSqlWorkflowListPrefersAuditEndpoint (0.00s)
--- FAIL: TestSqlWorkflowListIgnoresPartialDateRange (0.00s)
--- FAIL: TestSqlWorkflowOptionsParsesFilterSelects (0.00s)
--- FAIL: TestSqlWorkflowDetail (0.00s)
    workflow_test.go:508: archery/sqlWorkflowDetail: method_not_found: archery/sqlWorkflowDetail
--- FAIL: TestSqlWorkflowAuditPass (0.00s)
--- FAIL: TestSqlWorkflowAuditReject (0.00s)
--- FAIL: TestSqlWorkflowAuditValidation (0.00s)
--- FAIL: TestSqlWorkflowAuditSurfacesErrorPage (0.00s)
--- FAIL: TestSqlWorkflowExecute (0.00s)
--- FAIL: TestSqlWorkflowSchedule (0.00s)
--- FAIL: TestSqlWorkflowSubmitBuildsWorkflowJSON (0.00s)
--- FAIL: TestSqlWorkflowSubmitValidation (0.00s)（6 个子用例）
--- FAIL: TestSqlWorkflowCheck (0.00s)
--- FAIL: TestWorkflowLogAcceptsWorkflowType (0.00s)
    workflow_test.go:812: workflow_type should follow the request, got map[workflow_id:[9] workflow_type:[1]]
--- FAIL: TestSqlWorkflowWritesBlockedInReadOnly (0.00s)（4 个子用例 + 3 项只读放行）
FAIL	github.com/nixing178/dbx-plugin-archery/internal/rpc	0.260s
```

16 个顶层用例全部失败，失败原因均为「方法未实现 / 日志类型写死 1」，无编译错误；前端为模块缺失，符合红图预期。

第二轮红图（提交表单的数据源，新增 `internal/rpc/workflow_submit_test.go`）：

```text
--- FAIL: TestSqlSubmitOptionsParsesGroups (0.00s)
    workflow_submit_test.go:128: sqlSubmitOptions: method_not_found: archery/sqlSubmitOptions
--- FAIL: TestSqlWorkflowTargetsUsesCanWriteTag (0.00s)
    workflow_submit_test.go:155: sqlWorkflowTargets: method_not_found: archery/sqlWorkflowTargets
```

绿图：

```text
$ gofmt -l internal   # 无输出
$ go vet ./...        # 无输出
$ go test -count=1 ./...
ok  	.../internal/archery	0.309s
ok  	.../internal/rpc	0.293s

$ pnpm test
Test Files  13 passed (13)
     Tests  147 passed (147)
$ pnpm typecheck
exit 0
$ pnpm build
version check ok: 0.2.0
ui/index.html 1,008.60 kB (gzip 303.26 kB)
```

实现要点：

1. `internal/archery/client.go`：`SqlWorkflows` 按 Archery 的 `navStatus / instance_id / group_id / start_date / end_date / syntax_type[]` 拼表单，单边日期直接丢弃（Archery 只在两端都有时过滤）；`SqlWorkflowOptions` 从 `/sqlworkflow/` 解析实例/组下拉（option value 即 id）；`SqlSubmitOptions` 从 `/submitsql/` 解析组下拉的 `group-id` 属性与备份开关；`SqlWorkflowDetail` 用详情页 HTML 取审批流/操作信息，用 `/sqlworkflow/detail_content/` 取逐条审核结果，后者 500 时只丢该区块；审核/执行/定时走 `postWorkflowAction`（3xx 成功、200 的 `error.html` 用 `<h4>` 取文案）；`SubmitSqlWorkflow` 按网页同款 JSON `{workflow:{...}, sql_content}` 提交；`CheckSqlWorkflow` 同时发 `instance` 与 `instance_id`。
2. `internal/rpc/service.go`：只读守卫前移到方法分发之前（否则已实现的写方法会绕开名单），名单新增 `sqlWorkflowSubmit/Audit/Execute/Schedule/Cancel`；`workflowLog` 透传 `workflowType`（默认 1）；提交校验标题非空且 ≤50 字、组/实例/库/布尔外必填、时间窗两端格式与先后关系。
3. `src/lib/workflow.ts`：9 种状态 → 文案与色调、`syntax_type` 整数与 DDL/DML 字符串互转、行/详情/检测结果归一化（兼容驼峰与 Django 下划线键）、提交校验（必填顺序、标题 50 字、执行窗 ≥60 分钟且不早于当前）、审核/执行/终止按钮的状态门。
4. UI：`views/AuditOnlineView.vue`（10 列列表、状态/实例/组/日期筛选、我的工单/待我审核切换、搜索、分页、工单日志）、`views/SqlWorkflowDetailView.vue`（审批流 + 工单信息 8 列 + SQL 审核结果 6 列 + 四个动作入口）、`components/SqlSubmitDialog.vue`（组→实例→库级联、SQL 检测、告警二次确认）、`SqlWorkflowAuditDialog.vue`、`SqlWorkflowActionDialog.vue`；`WorkflowLogDialog` 参数化 `workflowType` 并兼容两种工单行。
5. `isPlaceholder` 改为按 tab 判断，`nav.test.ts` 同步把「SQL 上线」从占位改判为已实现（唯一修改的既有测试）。

预览冒烟（`node scripts/smoke-preview.mjs`，1440×900，Edge 无头）新增 16 项断言，全部为 true：

```text
workflowColumns / workflowRows / workflowStatus / workflowNotice / workflowLog
workflowDetailTitle / workflowDetailFlow / workflowDetailInfoColumns
workflowReviewColumns / workflowReviewRows / workflowAuditEntry / workflowExecuteEntry
workflowAuditDialog / workflowSubmitDialog / workflowCheck / workflowConfirm / workflowSubmitted
```

同一次运行里另有 2 项与本次无关的历史断言失败（HEAD 已如此，未改动）：

```text
checks.dictNotice = false       # 断言期望「演示用公告」，但 HEAD 的 .dict-notice 渲染的是 dict.notice 文案
checks.dictSearchIndex = false  # 搜索「客户」命中 customers/receivers/senders/customer_service_links（3 个首字母），断言仍写死 1
```

待实测：真实 Archery 账号（具备 `sql.menu_sqlworkflow`、`sql.sql_review`、`sql.sql_execute`）上跑一遍「提交 → 审核通过 → 立即执行」，并与网页的工单状态、审核结果逐条比对；`/submitsql/` 的组下拉若不带 `group-id` 属性，需要按实际页面结构调整解析。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist/io.github.nixing178.archery-0.2.0-windows-x64.dbxp
Signature: unsigned review candidate
```

包内容（解包核对）：

```text
checksums.json                                526 B
manifest.json                              10,342 B
assets/connection.svg                         415 B
assets/plugin.svg                             571 B
bin/windows-x64/dbx-plugin-archery.exe  8,476,672 B
ui/index.html                           1,014,100 B
```

- 包内 `manifest.json` 版本 **0.2.0**，中英文描述均已包含 SQL 上线；`checksums.json` 的 `manifest.json` / `ui/index.html` 哈希与解包文件一致
- 包内 `ui/index.html` 与本地 `pnpm build` 产物 sha256 一致（详情页改造后重新打包），说明打包未混入旧前端
- 最新产物见文末 v0.2.1（含详情页、登录页误判二次修复与 2FA 自动重试），`*.artifact.json` 记录一致

包内 sidecar 直连（`python scripts/probe-sidecar.py`）：

```text
initialize -> {"plugin":{"id":"io.github.nixing178.archery","version":"0.2.0"},"protocolVersion":1}
host connect (connection.id only) -> success, version 1.14.0
ui connect (no id)              -> success
instances (no id)               -> [{"id":1,"name":"fake-mysql",...}]
PROBE PASS

archery/sqlWorkflows      -> not_connected（方法已编入，未建会话）
archery/sqlWorkflowSubmit -> not_connected（同上，只读守卫已前置）
```

安装：DBX 插件中心 → 设置 → 允许未签名开发包 → 从文件安装 `dist/io.github.nixing178.archery-0.2.0-windows-x64.dbxp`。

### 详情页对齐 Archery 网页（同日追加）

按 `sql/templates/detail.html` 核验结构：标题 + 类型标签 + 查看提交信息；审批流；其他信息单行 10 列（发起人/目标实例/数据库/发起时间/可执行时间范围/结束时间/备份/当前状态/组/类型）；「工单详情 / 工单日志」两页签（日志仍为 `POST /workflow/log/` + `workflow_type=2`）；SQL 列表 9 列（ID/SQL内容/审核执行状态/审核执行信息/扫描影响行数/执行耗时/备份耗时/当前阶段/操作）+ 行展开；回滚按钮走 `GET /rollback/`，页面侧数据源为 `/sqlworkflow/backup_sql/`。

红图（第三轮，新增 `internal/archery/workflow_detail_test.go` 与 `src/lib/workflow-detail.test.ts`）：

```text
$ go test ./internal/archery -run "TestParseInfoFields|TestParseRollbackRows|TestRollbackSQL"
workflow_detail_test.go:32:12: undefined: parseInfoFields
workflow_detail_test.go:61:10: undefined: parseRollbackRows
workflow_detail_test.go:123:30: client.RollbackSQL undefined
FAIL	.../internal/archery [build failed]

$ pnpm vitest run src/lib/workflow-detail.test.ts
Error: Failed to load url ./workflow-detail ... Does the file exist?
Test Files  1 failed (1) / Tests  no tests
```

绿图：

```text
$ go vet ./...          # 无输出
$ go test -count=1 ./...
ok  	.../internal/archery	0.394s
ok  	.../internal/rpc	0.589s

$ pnpm test
Test Files  14 passed (14)
     Tests  158 passed (158)
$ pnpm typecheck
exit 0
$ pnpm build
ui/index.html 1,022.82 kB (gzip 306.69 kB)
```

实现要点：

1. `client.go`：`parseInfoFields` 取「其他信息」标题到下一个 `<h4>` 之间的表头 `th` 与唯一一行 `success` 的 `td` 按序配对，列数不一致整块丢弃；`RollbackSQL` 走 `/sqlworkflow/backup_sql/?workflow_id=`，兼容字符串行与 `{sql}` 行；`SqlWorkflowDetail` 增加 `infoFields`。
2. `service.go`：新增 `archery/sqlWorkflowRollback`，返回 `{rows, sql}`（`sql` 为以 `;\n\n` 拼接的整段回滚语句）。
3. `lib/workflow-detail.ts`：其他信息列名→i18n key 映射（未知列名原样展示）、SQL 行归一化（`id` 缺失用 1 起序号兜底、补 `backup_time`）、回滚入口判定（`workflow_finish`/`workflow_exception` 且有备份）。
4. `SqlWorkflowDetailView.vue` 重写：公告条 + 标题行（类型标签、查看提交信息弹窗）+ 审批流 + 其他信息 10 列 + 双页签 + SQL 列表（逐行/展开全部、搜索、紧凑）+ 记录数 + 操作信息 + 底部动作（审核通过/终止/执行/定时/查看回滚SQL/下载回滚SQL）。「工单日志」改为内嵌页签，不再弹窗。

预览冒烟新增 12 项断言并全部通过：

```text
workflowDetailInfoColumns (=10) / workflowDetailInfoValues ("无限制") / workflowDetailTabs (=2)
workflowGridColumns (=10) / workflowGridRows / workflowGridWarningRow (=1) / workflowRecordCount
workflowExpandAll (=2) / workflowDetailLog / workflowRollback
workflowAuditEntry / workflowExecuteEntry（待审核工单无执行入口）
```

截图：`preview/smoke-workflow-detail.png`（列表进入详情）、`smoke-workflow-expand.png`（展开全部）、`smoke-workflow-rollback.png`（回滚 SQL 弹窗）。

待实测：真实工单上核对「其他信息」是否仍为 10 列且行使用 `class="success"`（若改版为条纹行会退化为列表行兜底）、回滚 SQL 在无备份/无权限时的提示文案。

### 详情页 2FA 误报修复（真实实例暴露）

现象：真实实例打开工单详情报「无法读取工单详情：需要 2FA 验证：该账号启用了 2FA，请输入验证码」，同时审批流为空、其他信息退化为 8 列、状态显示「未知状态」。

### 回滚 SQL 子页与提交回滚请求（v0.2.15）

按 Archery `rollback.html` 页面截图核验：回滚页为「执行语句 / 回滚语句」两列表格 + 搜索/刷新/列筛选/导出工具栏 + 「提交回滚请求」按钮 + 记录数。数据源 `GET /sqlworkflow/backup_sql/` 的 rows 是 `get_rollback`（`sql/engines/goinception.py`）返回的 `[执行语句, 回滚语句]` 二元数组，按执行顺序倒序——旧实现 `parseRollbackRows` 只认 `string` / `{sql}` 行，真实二元数组被整体丢弃，「查看回滚SQL」因此拿不到内容。

红图（D1=A 独立子页、D2=B 实现提交，用户拍板）：

```text
$ go vet ./internal/archery
vet.exe: internal\archery\workflow_detail_test.go:80:13: rows[0].SQL undefined (type string has no field or method SQL)

$ pnpm exec vitest run src/lib/workflow-detail.test.ts
Test Files  1 failed (1)
     Tests  5 failed | 11 passed (16)
TypeError: normalizeRollbackRows is not a function
TypeError: filterRollbackRows is not a function
```

绿图与静态检查：

```text
$ go test ./... -count=1
ok ... internal/archery / internal/rpc

$ pnpm exec vitest run
Test Files 15 passed (15) / Tests 184 passed (184)

$ pnpm typecheck / pnpm build → exit 0，ui/index.html 1,061.22 kB (gzip 315.60 kB)
```

实现要点：

1. `client.go`：`parseRollbackRows` 返回 `[]SqlRollbackRow{id, sql, rollbackSql}`，兼容二元数组 / `{sql, rollback_sql|rollbackSql}` / 纯回滚字符串，空行丢弃、id 从 1 递增；`RollbackSQL` 返回结构化行。
2. `service.go`：`archery/sqlWorkflowRollback` 返回 `{rows, sql}`（`sql` 仅由回滚语句按 `;\n\n` 拼接）；新增 `archery/sqlWorkflowRollbackSubmit`（拉回滚语句 → `POST /api/v1/workflow/`，`is_backup=false`、不带执行时间窗、无回滚语句时报 `server` 错误），并列入只读守卫 `writeMethods`。
3. `lib/workflow-detail.ts`：`normalizeRollbackRows`（对象保留 Archery 的 id，缺失按序号兜底）、`filterRollbackRows`（执行语句或回滚语句命中即可）。
4. `SqlWorkflowRollbackView.vue` 新增：面包屑「SQL上线 / 回滚SQL」+ 标题行（条数标签、下载回滚SQL）+ 工具栏（展开全部/搜索/刷新/紧凑）+ 两列表格（行展开显示执行语句与回滚语句全文）+ 记录数 + 「提交回滚请求」；提交弹窗按 Archery 规则生成标题 `【回滚工单】原工单Id:{id} ,{原名}`，组/实例/库按原工单名称预填（组来自 `sqlSubmitOptions`，实例来自 `sqlWorkflowTargets`）。`SqlWorkflowDetailView.vue` 的「查看回滚SQL」由弹窗改为进入该子页，提交成功后回详情刷新。
5. `i18n.ts` 中英各增 9 键（`workflow.rollbackColSql/rollbackColRollback/rollbackStatements/rollbackSubmit/rollbackSubmitHint/rollbackSubmitDone/rollbackGroupRequired/rollbackInstanceRequired/rollbackDatabaseRequired`）。

预览冒烟回滚相关 4 项断言全部通过（总 149 项，dictNotice/dictSearchIndex 为历史遗留失败，与本次无关）：

```text
workflowRollback（两列=3 th、2 行、首行含执行与回滚片段）
workflowRollbackCount（总共 2 条记录）
workflowRollbackExpand（行展开 2 段全文）
workflowRollbackSubmit（toast 回滚工单已提交，工单号 #2088）
```

截图：`preview/smoke-workflow-rollback.png`（回滚子页两列表格）、`preview/smoke-workflow-rollback-submit.png`（提交弹窗预填组/实例/库）。

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.15`。

### 详情页下载回滚 SQL 入口（v0.2.15 追加）

按 Archery 详情页截图（「查看回滚SQL」+「下载回滚SQL」两个并排按钮）补回详情页直接下载入口（此前入口收进了回滚子页）。红图：`buildRollbackFile is not a function`（2 用例失败）→ 绿图：vitest 186 用例全绿、typecheck/build 通过、冒烟 150 项新增 `workflowRollbackDownload`（详情页点击后捕获 download 事件，文件名 `rollback_{id}.sql`）全绿。

实现要点：

1. `lib/workflow-detail.ts`：`buildRollbackFile` 对齐 Archery `views.rollback` 的下载格式（每段 `/*执行语句*/\n回滚语句\n`），替代旧的纯回滚语句拼接。
2. `lib/download.ts`：新增 `saveTextFile` 公共下载函数，详情页与回滚子页共用。
3. `SqlWorkflowDetailView.vue`：底部动作区恢复「下载回滚SQL」（`btn-info`，`rollbackVisible` 时显示，点击直接下载不进子页，无回滚语句时 toast 提示）；`SqlWorkflowRollbackView.vue` 的下载改用同格式。

### 下载回滚 SQL 在 DBX 沙箱内无反应修复（v0.2.16，真实环境暴露）

现象：真实 DBX 里点「下载回滚SQL」没有任何反应。原因：插件 UI 跑在 opaque-origin iframe 沙箱中，`<a download>` + Blob URL 被宿主拦截；预览冒烟跑在普通 Edge（file://）里触发了 download 事件，因此没暴露。核对 Go SDK（`third_party/dbx-plugin-sdk/sdk.go`）与宿主 bridge（`dbx.d.ts`）均无下载/对话框能力，落盘只能由 sidecar 完成。

红图（Go）：`undefined: downloadDir` → 绿图：`go test ./...` 全绿；前端 vitest 187 全绿、typecheck/build 通过、冒烟 150 项中回滚 5 项全绿（`workflowRollbackDownload` 改为断言 toast 含保存路径，另修正下载 toast 与提交 toast 重叠时取 `.toast` last）。

实现要点：

1. `service.go`：新增 `archery/saveTextFile`（`filename`/`content`）——文件名必须匹配 `^[A-Za-z0-9][A-Za-z0-9._-]*$` 防路径穿越；写入 `%USERPROFILE%\Downloads`（无 Downloads 退回主目录），重名追加 `-1/-2` 序号不覆盖；只写本机文件、不触数据库，不受只读守卫限制，返回 `{path}`。
2. `lib/download.ts`：`saveTextFile` 优先走 sidecar 落盘并返回路径；invoke 不可用（独立浏览器预览）时回退 `<a download>` 返回 null。
3. 详情页/回滚子页：成功后 toast「回滚 SQL 已保存：{path}」（`workflow.rollbackSavedTo`，en 对应），失败回退提示「已开始下载」。

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.16`。

### 回滚 SQL 自定义保存路径（v0.2.17）

「下载回滚SQL」升级为弹系统「另存为」对话框自选路径：默认文件名 `rollback_{id}.sql`、初始目录 Downloads、SQL 过滤器、覆盖确认与路径存在性校验由对话框（`OFN_OVERWRITEPROMPT | OFN_PATHMUSTEXIST`）负责；取消时静默返回 `{canceled}`。

红图（Go）：`undefined: promptSavePath` → 绿图：`go test ./...` 全绿、vitest 187 全绿、typecheck/build 通过、冒烟回滚 5 项全绿。

实现要点：

1. `savefile_windows.go`：syscall 调 comdlg32 `GetSaveFileNameW`（零新依赖），`OFN_NOCHANGEDIR` 防止改变 sidecar 工作目录；返回 0 且 `CommDlgExtendedError()==0` 视为用户取消。`savefile_other.go` 提供非 Windows 取消桩。
2. `service.go`：`archery/saveTextFile` 增加 `askPath`——确认后写入选定绝对路径（覆盖确认交对话框，不再追加序号），取消返回 `{canceled: true}`，不弹对话框时维持「直写 Downloads + 重名加序号」。
3. `lib/download.ts`：返回三态 `saved/canceled/browser`；详情页与回滚子页 `askPath: true`，saved→toast 路径、canceled→静默、browser→toast 已开始下载；invoke 超时放宽到 5 分钟（等待用户操作对话框）。

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.17`。

待实测：真实 DBX 里对话框弹出与前置表现（无 owner 窗口，可能不在最前）、超时边界（对话框停留超过 5 分钟后 UI 提示超时但文件仍会写入）。

待实测：真实工单上核对二元数组解析与倒序展示、无 `sql.sql_submit` 权限账号提交回滚请求的报错文案、只读连接下「提交回滚请求」的拦截提示。

根因：`looksLikeLoginPage()` 只要页面出现 `csrfmiddlewaretoken` 就判定为登录页。工单详情页的回滚/审核表单都带 `{% csrf_token %}`，于是 `/detail/<id>/` 被当成会话失效 → `dropPersisted()` 删掉落盘会话 → 触发重登 → 命中 2FA，整页失败。`/submitsql/`、`/queryapplydetail/` 等带表单的页面同理。

红图（新增 `internal/archery/auth_page_test.go`）：

```text
--- FAIL: TestLooksLikeLoginPage (0.00s)
    auth_page_test.go:21: page with csrf form must not be treated as the login page
--- FAIL: TestSqlWorkflowDetailDoesNotReloginOnCsrfForm (0.00s)
    auth_page_test.go:92: detail on a page with csrf form must not fail: Archery 会话已失效，请重新登录
```

绿图：

```text
$ go vet ./...        # 无输出
$ go test -count=1 ./...
ok  	.../internal/archery	0.655s
ok  	.../internal/rpc	0.318s
```

修复：判定收紧为「`login-form` 容器」或「`csrfmiddlewaretoken` + 密码框（`type="password"` / `name="password"`）」，业务页面即使带 csrf 也不再误判；新增用例同时断言详情页首屏只登录一次（不再触发重登）。

顺带修正兜底信息：详情解析失败时「当前状态」改用列表行的状态映射（不再显示「未知状态」），并补「类型」列。

重新打包：`dist/io.github.nixing178.archery-0.2.0-windows-x64.dbxp`（5,136,919 字节，sha256 `539d3d42d0a847c98e4156a45dde17947aeca22fdabad8651730aab0bc38155f`）。

影响面提示：旧版本在误判时会删除 `session.json`，升级后需重新连接一次（重新输密码/验证码），之后会话可正常复用。

#### 二次修复（v0.2.1）：上一轮的判据仍然命中业务页

上一轮用「csrf token + 密码框」判定登录页，但 Archery 的 `common/templates/base.html`（所有业务页的父模板）本身就带一个「确认密码」模态框：

```html
<input type="password" class="form-control" id="passwd" placeholder="确认密码" required>
```

而 `detail.html` 的回滚/审核表单又带 `{% csrf_token %}`，两者叠加仍然命中，于是 0.2.0 依旧把详情页当成登录页。核对 v1.14.0 源码后确认登录页独有标记为：

| 标记 | 位置 |
| --- | --- |
| `class="login-form fade-in-effect"` / `login-form-wrapper` | `common/templates/login.html` |
| `#inputUsername` / `#inputPassword` / `#btnLogin` | `legacy_login_form.html` |
| `<title>Login To Archery…` | `common/templates/login.html` |

红图（把 base.html 的密码框写进 fixture）：

```text
--- FAIL: TestLooksLikeLoginPage
    business page with csrf form and password modal must not be treated as the login page
--- FAIL: TestSqlWorkflowDetailDoesNotReloginOnBusinessPage
    detail on a business page must not fail: Archery 会话已失效，请重新登录
```

绿图：

```text
$ go vet ./...        # 无输出
$ go test -count=1 ./...
ok  	.../internal/archery	0.797s
ok  	.../internal/rpc	0.610s
```

最终判据只认登录页独有标记（`login-form` / `#inputUsername` / `#inputPassword` / `#btnLogin` / `Login To Archery`），csrf token 与密码框不再参与判定。

同时补上「会话中途过期」的前端兜底：任一接口返回 `twofa_required` 时，`api.ts` 回调 App 弹出验证码框，验证通过后**自动重试原请求**，页面不再卡死；用户取消则保留原始报错。

版本 0.2.1（同号包在插件中心可能不会替换，故升位）：

```text
dist/io.github.nixing178.archery-0.2.1-windows-x64.dbxp
5,137,158 字节
sha256 d374e54f50df0e1092fc320a11ef9bb9db0fa129f8588684d93888efe3f0fbbc
包内 manifest 0.2.1；checksums 全对；包内 ui/index.html 与本地构建一致
```

### 工单列表日期区间（v0.2.2）

按 Archery 网页的 daterangepicker 重做筛选：单一触发器显示 `起 - 止`，下拉含 全部 / 今日 / 昨日 / 最近7日 / 最近30日 / 本月 / 上月 / 自定义；默认「最近30日（含今天）」，与网页一致（今天 2026-11-02 → 2026-10-04 ~ 2026-11-02）。

红图（`src/lib/date-range.test.ts` 11 用例）：

```text
Error: Failed to load url ./date-range ... Does the file exist?
Test Files  1 failed (1) / Tests  no tests
```

绿图：

```text
$ pnpm vitest run src/lib/date-range.test.ts
✓ 11 tests
$ pnpm test
Test Files  15 passed (15) / Tests 169 passed (169)
$ pnpm typecheck  exit 0
$ pnpm build      ui/index.html 1,028.26 kB (gzip 308.18 kB)
```

实现：

1. `lib/date-range.ts`：`presetRange` 生成预设区间（最近 7/30 日含今天；上月按 `new Date(y, m, 0)` 取月末，跨年正确）、`detectPreset` 反查高亮项、`formatRange` 触发器文案；全部按本地时区输出 `YYYY-MM-DD`，与 `start_date` / `end_date` 一致。
2. `components/DateRangeFilter.vue`：触发器 + 下拉，自定义区间内联两个日期输入并自动纠正起止顺序；点击外部关闭。
3. `AuditOnlineView.vue`：默认区间取 `presetRange("last30")`；移除原来的两个原生日期框与「重置筛选」（「全部」预设即可清空），右侧搜索/刷新/密度收成一组（`margin-left:auto`），工具栏回到单行。
4. `preview/mock.js` 增加按日期过滤（含当天）；新增 5 项冒烟断言：

```text
workflowDateRange（触发器为 "YYYY-MM-DD - YYYY-MM-DD"）
workflowDatePresets（8 个预设）
workflowDateActive（默认高亮「最近30日」）
workflowDateCustom（自定义显示 2 个日期输入）
workflowDateAll（选「全部」后文案为「全部」）
```

截图：`preview/smoke-workflow.png`、`smoke-workflow-date.png`。

打包：

```text
dist/io.github.nixing178.archery-0.2.2-windows-x64.dbxp
5,138,494 字节
sha256 b0c0421a59f06c05324267a2ba6040ecc244ceed1682f3e8026c8d9e4c97c957
```

### 提交 SQL 整页化（v0.2.3）

按 `sql/templates/sqlsubmit.html` 的网页布局，把原来的弹窗改成整页：

| 区域 | 实现 |
| --- | --- |
| 面板标题 | 「SQL上线」+ 返回列表 |
| 左侧 | 复用 `SqlEditor`（CodeMirror 6，行号/高亮/自动换行），撑满剩余高度 |
| 右侧栏 | 仅支持10M内的SQL文件 + 选择…（`.sql` 导入，超限拦截）、上线单名称、需求链接、组 → 实例（can_write）→ 数据库、可执行时间范围、备份开关、`美化` / `SQL检测` / `SQL提交`（蓝/红/绿，与网页同色） |
| 底部 | 「检测结果」面板：阶段 / 级别 / 说明 / SQL 内容，右上角显示「检测完成：N 个错误，M 个警告」 |
| 交互 | 美化走 `formatSql`（按实例 dbType 选方言，失败提示）；SQL 一改检测结论作废；有错误阻断提交，仅告警时二次确认；提交成功回列表并刷新 |

新增 `views/SqlSubmitView.vue` 与 `components/DateTimeRangeField.vue`（单入口 + 双 datetime-local 弹层，自动纠正起止顺序、可清空），删除 `components/SqlSubmitDialog.vue` 及其废弃样式。

冒烟新增/改写 9 项，全部 true：

```text
workflowSubmitPage（页面标题）、workflowSubmitEditor（编辑器）、workflowSubmitFields（右侧字段+10M 提示）
workflowSubmitActions（3 个动作按钮）、workflowCheckPanel（检测结果面板）
workflowCheck（1 个警告）、workflowCheckRows（1 行）、workflowConfirm（二次确认）、workflowSubmitted（回到列表且含新工单）
```

截图：`preview/smoke-workflow-submit.png`。

打包：

```text
dist/io.github.nixing178.archery-0.2.3-windows-x64.dbxp
5,140,041 字节
sha256 9fcbdb6f5b18b0a0fe801a305d3a48031cea13b8c2d9a0796f8151ce23bcc786
```

### 提交页细节对齐网页（v0.2.4）

对照 `sqlsubmit.html` 补齐三处差异：

| 项 | 网页实现 | 插件实现 |
| --- | --- | --- |
| 审批流程 | `POST /group/auditors/`（`group_name` + `workflow_type=2`）→ `data.auditors_display`，渲染成 `bg-primary` 蓝条 | 新增 `Client.GroupAuditors` + RPC `archery/sqlWorkflowAuditors`，选组后并行拉取，蓝条显示「审批流程：……」 |
| 检测结果表格 | 列 `ID / SQL语句 / 扫描影响行数 / 审核状态 / 审核信息`，状态原样 `pass\\|warning\\|error`，`detailView` 展开完整 SQL，`rowStyle` 告警黄/错误红 | 同列同序（含展开列），状态原样文本，warning/danger 行底色，上方搜索框，下方「显示第 X 到 Y 条记录，总共 N 条记录」 |
| 提交门控 | `#btn-submitsql` 初始 disabled，检测无 error 后才启用；编辑器内容变化再次禁用 | `canSubmit = 已检测当前 SQL && errorCount === 0`，SQL 一改即禁用 |
| 文件导入 | 读取后**追加**到编辑器末尾，>10M 提示后重置 | 同（追加 + 10M 拦截） |
| 选择按钮 | bootstrap-fileinput 默认 `btn-primary` | 补全局 `.btn-primary` 样式（此前主题只有 `.btn.primary`，所以按钮是白底） |

红图（新增 `internal/archery/workflow_auditors_test.go`）：

```text
internal/archery/workflow_auditors_test.go:62:43: client.GroupAuditors undefined
FAIL	.../internal/archery [build failed]
```

绿图：

```text
$ go test -count=1 ./...
ok  	.../internal/archery	0.304s
ok  	.../internal/rpc	0.354s
$ pnpm test       15 文件 / 169 用例
$ pnpm typecheck  exit 0
$ pnpm build      version check ok: 0.2.4
```

冒烟新增 6 项并全部通过：

```text
workflowAuditors（蓝条含 TEST-Leader-Express->DBA）
workflowSubmitDisabled（检测前禁用）/ workflowSubmitEnabled（检测后启用）
workflowCheckColumns（6 列）/ workflowCheckStatus（首行 pass）/ workflowCheckRecords（总共 2 条记录）
```

截图：`preview/smoke-workflow-submit.png`（与期望图逐项比对：蓝条位置、表格列、行底色、记录数文案、按钮着色）。

打包：

```text
dist/io.github.nixing178.archery-0.2.4-windows-x64.dbxp
5,143,735 字节
sha256 3b197cb5be1d68931e95a45e3ab22b3284ed925df5bfbfcba128da13c12b8bd6
```

### 检测结果高度动态化（v0.2.5）

原实现把面板写死 `max-height: 240px`：空态留白过大，结果多时又被截断。改成「跟随内容 + 封顶 + 内部滚动」：

| 规则 | 值 |
| --- | --- |
| 面板 | `flex: 0 0 auto; max-height: min(55%, 340px)`，高度由内容决定 |
| 面板体 | `overflow: hidden` + 纵向 flex；只有 `.grid-wrap` 滚动 |
| 空态 | `.empty` 在面板内压到 `min-height: 56px; padding: 12px` |
| 记录数 | `flex: none`，始终钉在面板底部（不随表格滚走） |
| 主区 | `min-height: 430px`，保证面板长高时右侧字段列不被裁切 |

### SQL 上线工单详情页 UI 优化（0.2.28 候选）

只动 `views/SqlWorkflowDetailView.vue`、`style.css`、`i18n.ts`、`components/NavIcon.vue` 与冒烟脚本；权限申请工单详情页维持原样。

| 区域 | 改动 |
| --- | --- |
| 标题区 | 标题 + 状态胶囊（复用 `.privilege-status`，与列表页同色）+ 等宽类型 chip；「上线其他实例 / 查看提交信息」右对齐 |
| 元信息行 | 标题下新增 实例 / 库 / 组 / 发起人 / 发起时间，不必滚到「其他信息」才看得到 |
| 审批流 | 纯文本 + 下划线 → 胶囊节点：已通过 ✓、当前节点蓝底并列审批人、未到达虚线；加载中显示 spinner（原来先闪「暂无审批流信息」） |
| 其他信息 | 单行 10 列表格 → 键值卡片网格（5×2，<860px 折 2 列），长值换行不截断，去掉整行绿底，「当前状态」用胶囊 |
| 页签 | SQL 详情 / 工单日志加条数徽章；搜索框常驻页签行右侧，切页签行高不再跳动 |
| SQL 列表 | 数字列右对齐 + `tabular-nums`；表头允许折行（英文 `Scanned / affected` 不再省略）；「操作」列由恒为「-」改为行内复制 SQL（`writeClipboard`，失败提示手动复制） |
| 底栏 | 操作按钮移出滚动区，固定在面板底部；无可用操作时整条不渲染 |
| 缺陷修复 | 第 5 列原为 `width: 24%`，与固定列宽相加超出表宽后该列被压成 1 字符宽：1100px 下该行高 1513px；改为「剩余空间自适应」+ 详情页 `min-width: 1180px`，行高回到 88px |

红图（先改冒烟断言，旧 UI 上 12 项 false，`errors: []`）：

```text
workflowDetailStatusPill / workflowDetailMeta / workflowDetailFlowMarks
workflowDetailInfoItems / workflowDetailInfoNoTruncate / workflowDetailTabCount
workflowDetailSearchInTabs / workflowGridCopyButton / workflowDetailFlowCurrent
workflowDetailActionsOutsideScroll / workflowDetailActionsPinned / workflowDetailActionsReachable
```

绿图：

```text
$ pnpm typecheck                  exit 0
$ pnpm build                      version check ok: 0.2.27
$ node scripts/smoke-preview.mjs  exit 0（新增 12 项全部 true，errors: []）
```

截图：`preview/smoke-workflow-detail.png`（浅色 1440）、`smoke-workflow-log.png`（日志页签）、`smoke-workflow-detail-many.png`（40 行）、`smoke-workflow-audit.png`（待审核：当前节点 + 审批人）。另本地复核深色、1100px、820px 与英文界面（临时脚本跑完即删）。

#### 视觉打磨（同批后续）

同一批结构之上做纯视觉升级，仍然只动 `SqlWorkflowDetailView.vue` 与 `style.css`：

| 元素 | 处理 |
| --- | --- |
| 工单头 | 8px 圆角状态色带：左侧 3px 状态色 + 同色 12% 径向渐变 + 同色描边（ok/warn/danger/muted 四套变量），标题 19px 负字距；`flex: none` 防被 `.panel-body` 的纵向 flex 压缩 |
| 元信息行 | 项间 1px 竖线分隔，标签 11.5px 弱化、值回到主文本色，读起来是一条账目而不是一串标签 |
| 审批流 | 胶囊换成编号时间线：20px 圆形序号 + 节点间真实连线的 1px 轨道（已通过的轨道转绿）、当前节点实心 + 3px 光环 + 2.6s 呼吸动画并单独列出审批人、未到达虚线 |
| 章节标题 | 11px / 700 / 0.09em 字距 + 向右渐隐细线，替掉普通 h4 |
| 其他信息 | 卡片加 hover 底色；标签 10.5px 字距、值 13px/500，强化「标签—值」对比 |
| 页签 | 方块页签改下划线式：激活项 2px 强调色压住分隔线，徽章转强调色 |
| 表格 | 表头 11.5px + 0.02em；未通过行的整行底色压到 8–9%，改由首列 2px 状态条表达；行 hover 底色；复制按钮平时 45% 透明、行内 hover 点亮 |
| 底栏 | 加上向上投影，与滚动内容分层 |
| 动效 | `workflow-rise` 入场（头部 + 时间线节点 0/40/80/120ms 错峰），`prefers-reduced-motion` 下全部关闭 |

过程中发现并修掉一个真实布局缺陷：`.workflow-detail-scroll` 同时带 `panel-body` 类，是**纵向 flex 容器**；头部加了 `overflow: hidden` 后自动最小高度失效，被压成 34px（标题被裁一半、元信息溢出画面）。加 `flex: none` 后头部恢复 83.75px（playwright 实测）。

验证：`pnpm typecheck` exit 0、`pnpm build` 通过、`node scripts/smoke-preview.mjs` exit 0（结构断言未改一行仍全绿、`errors: []`）；3x 放大复核头部与时间线（已通过 / 当前 / 未到达三态）与深色主题，临时截图脚本已删除。

#### 真实环境对齐预览（同批后续）

装包实测发现真实实例与预览有两处差异，成因不同：

| 现象 | 成因 | 处理 |
| --- | --- | --- |
| 已结束工单的审批流是灰底编号圆圈，不是绿勾 | 详情页节点状态靠 HTML class（`text-success` / `text-danger` / `<u>`）判定，这套 class 在部分 Archery 版本/定制模板上不渲染，解析后全部落到 `pending` | 新增 `resolveReviewNodes`：状态为 `workflow_finish` / `workflow_exception` 且**一个已通过/当前节点都没解析到**时，整条按「已通过」渲染；解析到任何进度就原样返回，仍在流转的工单不推断 |
| 「其他信息」最后一行右侧留一块带网格线的空白 | 卡片固定 5 列，真实工单常只有 8、9 个字段（值为空的字段会被丢弃），第二行填不满 | 列数按字段数取能整除的值（5/4/3/2，兜底 5）：8→4 列、9→3 列、10→5 列；`--info-cols` 由视图注入，<860px 仍折 2 列 |
| 缺「可执行时间范围」「结束时间」两列，其余 8 个字段正常 | `parseInfoFields` 把「其他信息」后面整段文本都算进区块，紧随其后的表格（SQL 列表等）表头也被当成列名，表头数与 `tr.success` 的单元格数不等 → 整块丢弃 → UI 回退到列表行的 8 个字段（回退清单里正好没有这两列） | 只取「其他信息」后的第一张表作为解析范围，表头与 `tr.success` 都在这张表内匹配；新增 `TestParseInfoFieldsIgnoresTrailingTables`（红：`expected 10 fields, got 0`） |

红图（单测先行）：`resolveReviewNodes is not a function`（5 例失败）；冒烟在旧构建 + 新 mock 上 `workflowDetailFlowFallback`、`workflowDetailInfoColumnsFit` 两项 false（mock 里已结束工单的节点 state 送空，复现真实页面）。

绿图：`pnpm test` 16 文件 / 226 用例通过（其中 `workflow-detail.test.ts` 52 例，含 `resolveReviewNodes` 5 例）、`pnpm typecheck` exit 0、`pnpm build` 通过、冒烟 exit 0 且 `errors: []`。

#### SQL 列表打磨（同批后续）

| 问题 | 处理 |
| --- | --- |
| 展开按钮被 34px 单元格挤成「+ …」 | 单元格加宽到 40px、左右内边距收到 2px 并 `text-overflow: clip`；`op-btn` 默认改弱色、hover 才转强调色，40 行时不再一片蓝色加号 |
| 序号与 SQL 正文贴在一起 | 序号列改左对齐、弱化色 + `tabular-nums`，去掉原来的右对齐 |
| 空值与真实数值混在一起 | 信息/执行时间/备份耗时/阶段为空时统一加 `is-empty`（弱色 + 0.7 透明度） |
| 多行内容行的状态与数字上下漂移 | 单元格 `vertical-align: top`，上下内边距 7px |
| 长标识被逐字符硬断（`axle_numbe` / `r`） | 审核信息列改 `overflow-wrap: anywhere` + `word-break: normal`，并把该列从 210px 加到 230px，刚好放下 `库.表.列` |
| 阶段列挤成三行 | 与 SQL 列、操作列重新分配：260 / 160 / 64，单行内容回到 37px 行高 |
| 记录数旁没有整体结论 | 新增 `N 条警告` / `N 条错误` 胶囊（`workflow.warnCount` / `workflow.errorCount`，中英各一份） |

红图：`workflowGridCaretNotClipped`（展开按钮溢出单元格）、`workflowRecordCount`（无告警胶囊）两项 false；绿图：`pnpm test` 226 例、`pnpm typecheck` exit 0、`pnpm build` 通过、冒烟 exit 0 且 `errors: []`。1280 / 1440 与深色三态均已目视复核。

#### 全站视觉统一（0.2.28 候选）

同一套语言铺到所有页面，先做全局基座（一处改动全站受益），再点调三处最显眼的。

| 层 | 改动 |
| --- | --- |
| 设计令牌 | 新增 `--radius-card: 8px`、`--radius-control: 6px`、`--card-shadow`、`--ring`，替掉散落的 3–4px 直角色 |
| 面板/卡片 | `.panel`、`.card`、`.stat-card` 统一 8px 圆角 + 轻阴影；头部 40/36px、13.5px 标题、圆角跟随 |
| 表格 | `.data-table` 表头 11.5px + 0.02em 字距 + 弱化色，单元格内边距 12 → 10px（列表页、字典、历史、权限、审核结果全部生效） |
| 表单 | `.form-control` 32px / 30px（sm）、6px 圆角；聚焦改为强调色边框 + `--ring`，不再用写死的 `#80bdff` |
| 按钮 | `.btn`/`.tool-btn` 6px 圆角 + `:focus-visible` 焦点环；新增 `.btn-quiet`（纯文字行内动作） |
| 导航 | 一级选中项补左侧 2px 强调条；二级缩进 40 → 42px、字号 12.5px |
| 仪表盘 | 指标卡从「整块高饱和色」改为白底细边 + 40px 色块图标，数值 24px/600；图表调色板降一档饱和度（`#4f8ed6 / #4aab6e / #d8a43c / #cd7268 / #8b7ec8 / #47a6b5`） |
| 查询页 | 「美化 / 执行计划」由青色/橙色实心改为描边次级按钮，只留「SQL 查询」为绿色主操作；`.config-actions .btn` 不再强制白色文字 |
| 数据字典 | A–Z 索引由两行方块改为横向胶囊（字母 + 表数一行），行加 hover 底色 |
| 列表动作 | 「操作日志」由青色实心按钮改为 `.btn-quiet` 文字动作，13 行也不再是青色墙 |

验证：`pnpm test` 16 文件 / 226 例通过、`pnpm typecheck` exit 0、`pnpm build` 通过、冒烟 192 项断言全 true 且 `errors: []`。浅色与深色下逐页复核：仪表盘、查询/结果、历史、数据字典、表元数据弹窗、SQL 上线列表/详情/提交页、权限列表。

#### 仪表盘专项（同批后续）

| 区域 | 改动 |
| --- | --- |
| 取数条件 | 时间范围与刷新从「图表与统计卡之间」移到指标卡之上，作为整页筛选行 |
| 环图区 | 环图下方一排图例改成右侧分布列表：类型 + 数量 + 占比 + 4px 进度条；环图缩到 172px，<900px 回落竖排 |
| 空态卡 | 「SQL 查询统计 / SQL 上线统计」补图标与居中留白（96px），不再是一行灰字 |
| 深色 | 冒烟新增 `smoke-dark-dashboard.png` 与 `darkDashboard` 断言（4 张指标卡 + ≥2 个 SVG + 分布条存在） |

验证：冒烟 exit 0（`darkDashboard` 通过）、`errors: []`；浅色 1440 与深色 1440 截图逐项目视。

#### 两张按日统计卡（同批后续）

原先这两张卡是「图表数据待 Archery Dashboard 接口开放」的占位，实际数据就在同一份 `/dashboard/` HTML 里。

| 区域 | 改动 |
| --- | --- |
| 后端 | 新增 `ParseDashboardTrends`：按面板标题定位 `SQL查询统计` / `SQL上线数量`，解码其后的 ECharts `option`；折线取 `[日期, 值]` 对，柱状取 `data` + `xAxis.data`，并解析顶部「统计时间范围」 |
| 接口 | dashboard 响应新增 `trends`（dates / query / release / rangeLabel），计数解析失败时也返回 |
| 前端 | 新增 `TrendChart.vue`：自绘 SVG 折线（主序列带数值标签、次序列弱化不标值）与柱状（0 值不标数字、柱顶标值），4 档整齐刻度 + 千分位；卡片头显示 Archery 的统计区间与图例；`<1100px` 两张卡叠成单列 |
| 细节 | 数值标签描一圈卡片底色（`paint-order: stroke`），压住网格线/刻度仍可读；首尾点标签按位置改锚点，避免越界；超过 14 天不再逐点标数值，日期刻度按跨度与卡片宽度自适应（见第 20 节） |
| 时间范围 | 两张卡上方新增「统计时间范围」选择器。**注意取数路径**：`/dashboard/` 页面视图不解析任何日期参数（`common/dashboard.py::pyecharts` 里硬编码 `[D-6, D+1]`），真正按区间出数的是 `/dashboard/api/?start_date=&end_date=`（`DashboardApi`，返回 `{"chart": {line1, bar1, ...}}` 的 pyecharts 片段）。因此切换区间只重取这两张图（新增 `archery/dashboardTrends`），不重新拉整页 |
| 预设 | `lib/date-range` 补齐 Archery daterangepicker 的预设：最近 365 日 / 本年 / 上一年，并把「上月」统一成「上个月」，共享选择器（SQL 上线筛选、权限列表）同步生效 |
| 首页专用预设 | `DateRangeFilter` 新增 `presets` 入参：首页只给 最近7日 / 最近30日 / 最近365日 / 本月 / 本年 / 上个月 / 上一年 / 自定义（**去掉「全部」**）；列表页仍用含「全部 / 今日 / 昨日」的完整预设 |
| 区间口径 | 首页另加 `:end-offset-days="1"`：Archery 首页的「最近 7 日」实为 `[D-6, D+1]`（含明天，共 8 天），滚动区间右端跟着推一天；预设反查也按这套口径比对，所以默认停在「最近7日」而不是「自定义」 |
| 默认值不依赖页面 | 原先靠页面回显的「统计时间范围」文案反推初始区间，文案解析不到时（部分实例）区间为空 → 选择器退化成「全部」。改为由 `presetRangeWithOffset("last7", 1)` 自己算出默认值；页面的区间只用于「一致就跳过那次 `/dashboard/api/` 请求」 |

红图：`dashboardTrendPlaceholder`、`dashboardTrendQuery`、`dashboardTrendRelease` 三项 false（旧构建仍是两张空占位卡）；Go 侧 `undefined: ParseDashboardTrends`。

绿图：`TestParseDashboardTrends`（真实片段 → 8 个日期、检索行数 `1870,2435,167,0,1943,3246,1066,0`、检索次数第 6 天 230、上线数量 `5,11,2,0,16,16,4,0`、区间 `2026-10-29 - 2026-11-05`）与 `TestParseDashboardTrendsMissing` 通过；`go test ./...` 两包通过、`go vet` 无输出；`pnpm test` 224 例、`pnpm typecheck` exit 0、`pnpm build` 通过、冒烟 exit 0 且 `errors: []`。

时间范围控制单独一组：

- 红（第一轮）：`dashboardRangeControl`、`dashboardRangeApplied` 两项 false（旧构建没有选择器）；`parseRangeLabel is not a function`（3 例）
- 红（第二轮，真实缺陷）：mock 改成与 Archery 一致——`/dashboard/` 整页不吃日期参数后，`dashboardRangeApplied` 重新变 false，复现「切换时间范围数据不变」
- 绿：`TestParseTrendFragments`（chart 片段 → 8 列日期/双折线/柱值/区间文案）、`TestDashboardTrendsUsesChartAPI`（请求 `/dashboard/api/` 且带 `start_date`/`end_date`）；`parseRangeLabel` 3 例；冒烟断言选择器显示 Archery 回显区间，切成「最近30日」后刻度数增加、触发文案变化且**卡片副标题等于所选区间**（截图 `preview/smoke-dashboard-range.png`）
- 连带更新：`workflowDatePresets` 从「恰好 8 项」改成「11 项且包含最近365日/上个月/本年/上一年/自定义」，比原来更严
- 红（第三轮，默认值依赖页面文案）：mock 的 `/dashboard` 把 `rangeLabel` 置空（复现真实实例解析不到区间文案）后，`dashboardRangeControl` / `dashboardRangePresets` / `dashboardRangeApplied` 三项 false，选择器显示「全部」
- 绿：`presetRangeWithOffset` 4 例（+1 天、跨月、日历区间不受影响、无偏移等价于原预设）通过；冒烟断言选择器文案必须含 `2026-10-29` 且**不含「全部」**

#### 仪表盘数据对不上（同批后续）

| 问题 | 根因 | 处理 |
| --- | --- | --- |
| 插件四张卡取不到数（网页是 2244 / 401 / 88 / 19） | 真实首页用的是 **AdminLTE panel 版指标卡**：`<div class="huge">2244</div>` + 紧随的无 class `<div>SQL上线工单</div>`，既不是 `small-box` 也不是 `info-box`；解析器只认 `info-box-number`，一个数字也取不到，直接降级 | 新增 panel 解析（`huge` + 下一个 `div` 成对），连同 small-box、info-box 三种结构都支持 |
| 即使结构匹配，四张卡也会整体错位 | 只取前四个 `info-box-number` 按位置映射；顺序变化或页面多出别的卡片时全部串位 | 两种模板都按「文案 + 数字」成对归类（上线工单 / 查询工单 / 用户 / 实例），四类认全才采用，认不全才退回顺序取数 |
| 「统计时间范围」与数字无关 | 该控件既没参与 `/dashboard/` 取数，也没过滤实例图，`rangeHint` 却写着「仅作用于图表」 | 换成口径说明「指标取自 Archery 首页全量统计；实例图按当前账号可见实例实时汇总」，删掉无效的 `defaultRange`/`Range` 及两条单测 |
| 实例类型图只有 2 个（网页是 19 个） | 图表原先按 `/group/user_all_instances/` 的**当前账号可查询实例**统计，而首页「数据库实例类型统计」是**全库实例**；权限收敛时两者差很多 | 新增 `ParseInstanceTypeStats`：从首页 HTML 的 ECharts `option_*` 里解析柱状图（环境 × 类型）与环图（类型总量），随 dashboard 返回 `instanceStats`；图表优先用它，取不到才退回本地实例清单 |

红图：
- `TestParseDashboardCountsFromPanelCards`（按用户提供的真实 `/dashboard/` 片段构造）→ `dashboard counters not found: want 4, got 0`，复现「网页有 2244/401/88/19、插件是横杠」
- `TestParseDashboardCountsFromSmallBoxes` → 同样 `got 0`
- `TestParseDashboardCountsMapsByLabelNotOrder` → `{SQLWorkflow:19 SQLQuery:87 ActiveUsers:396 Instances:2200}`
- `TestParseDashboardCountsSkipsUnknownLabels` → `{SQLWorkflow:12 …}`

绿图：`go test ./internal/archery -run TestParseDashboard` 10 例全过（panel / small-box / info-box 三种结构 + 乱序 + 多余卡片）、`go test ./...` 两包通过、`go vet ./...` 无输出；`pnpm typecheck` exit 0、`pnpm test` 16 文件 / 224 例、冒烟 exit 0 且 `errors: []`。

图表数据源单独一组：

- 红：`ParseInstanceTypeStats` 未实现（build failed）；冒烟 `dashboardUsesHomepageStats` 在旧构建 + 新 mock 上 false（图表仍是本地清单的 14/3/1/1）
- 绿：`TestParseInstanceTypeStats`（真实柱状图 + 环图片段 → dbTypes `mysql,doris,redis`、envs `master,slave`、master `14,1,1`、slave `2,0,1`、类型总量 `16/2/1`）、`TestParseInstanceTypeStatsMissingPanel`；冒烟断言分布列表恰为 `mysql=16,redis=2,doris=1` 且中心总数为 19

Go 侧绿图（「其他信息」解析）：

```text
$ go test ./internal/archery -run TestParseInfoFields -v
--- PASS: TestParseInfoFields
--- PASS: TestParseInfoFieldsIgnoresTrailingTables
--- PASS: TestParseInfoFieldsMissing
$ go test ./...   ok archery 0.313s / ok rpc 0.374s
$ go vet ./...    （无输出）
```

冒烟新增 5 项（用 SQL 含 `many` 时返回 40 行的 mock 分支验证封顶）：

```text
workflowCheckEmptyCompact    空态面板 < 130px
workflowCheckHeightGrows     有结果后高度增加 > 40px
workflowCheckFooterVisible   记录数行可见
workflowCheckHeightCapped    40 行时面板 < 提交区 62%，且表格内部滚动
workflowSubmitSideIntact     面板长高后右侧字段列无裁切（scrollHeight ≤ clientHeight）
```

截图：`preview/smoke-workflow-submit.png`（2 行）、`smoke-workflow-many-results.png`（40 行封顶）。

打包：

```text
dist/io.github.nixing178.archery-0.2.5-windows-x64.dbxp
5,143,795 字节
sha256 0bf82751d140772a70d5ba7132b04ea30daa319513e4bc6d3c360c02ce0bfd3f
```

### 提交页高度规则调整（v0.2.6）

v0.2.5 用「面板封顶 55%/340px + 表格内部滚动」控制高度，但会压缩编辑区。改为：

| 元素 | 规则 |
| --- | --- |
| 编辑区（`.workflow-submit-main`） | `flex: 0 0 auto; height: 430px` —— 固定高度，检测结果再长也不压缩 |
| 检测结果面板 | 不设 `max-height`，随行数撑开（40 行时 > 600px） |
| 表格区 | `overflow: visible`，不再内部滚动 |
| 提交区（`.workflow-submit-body`） | 保持 `overflow: auto`，结果过多时整体滚动承载 |

冒烟断言相应改写（去掉「封顶」断言，新增定高与不限高断言），全部 true：

```text
workflowEditorFixed        2 行结果后编辑区高度不变（±1px）
workflowEditorFixedOnMany  40 行结果后编辑区高度仍不变
workflowCheckUnlimited     40 行时面板高度 > 600px
workflowCheckNoInnerScroll 表格区不再内部滚动（scrollHeight ≤ clientHeight）
workflowSubmitBodyScrolls  结果过多时提交区整体可滚动
workflowSubmitSideIntact   右侧字段列无裁切
```

截图：`preview/smoke-workflow-submit.png`（2 行，整屏无需滚动）、`smoke-workflow-many-results.png`（40 行，面板向下延伸，编辑区保持 430px）。

打包：

```text
dist/io.github.nixing178.archery-0.2.6-windows-x64.dbxp
5,143,767 字节
sha256 8fd3623a0e129bca6320f9701d95e4a6f6a3c611b9b53dab1ee9803016ae055f
```

### 「查看提交信息」改为进入提交页并预填（v0.2.18）

原实现是详情页标题行的弹窗：只读展示上线单名称、发起人/实例/库/组与拼接后的 SQL，看完即止。改为点按钮整页进入「提交SQL」，把原工单的提交内容带进表单，改完可直接重新提交：

| 预填项 | 来源 |
| --- | --- |
| 上线单名称 | 详情标题，缺失时用列表行 `workflowName` |
| SQL 内容 | 详情 SQL 列表逐条补分号后按行拼接；空行丢弃，`use 库名` 切库语句整条排除 |
| 组 / 目标实例 / 数据库 | 详情「其他信息」同名列，缺失时回退列表行；组与实例按名称匹配下拉 id，库不在资源清单时补一个兜底选项 |
| 备份 | 「其他信息」的「备份」列（是/否），缺失时用列表行 `is_backup` |
| 执行时间窗 | 「其他信息」的「可执行时间范围」；`无限制` 视为不限，时间取到分钟 |

新增纯函数（`lib/workflow-detail.ts`）：`parseRunDateRange`、`buildCommitSql`、`matchOptionIdByName`、`ensureOptionValue`、`buildSubmitPrefill`；`SqlSubmitPrefill` 类型放在 `types.ts`。

红图：

```text
$ pnpm exec vitest run src/lib/workflow-detail.test.ts
TypeError: buildSubmitPrefill is not a function
TypeError: parseRunDateRange is not a function
Test Files  1 failed (1)
     Tests  13 failed | 19 passed (32)
```

绿图：

```text
$ pnpm test
Test Files  15 passed (15)
     Tests  200 passed (200)
$ pnpm typecheck   exit 0
$ pnpm build       version check ok: 0.2.18 / ui/index.html 1,063.64 kB
```

交互改动：

1. `SqlWorkflowDetailView.vue`：删除提交信息弹窗与 `.workflow-commit-sql` 样式，按钮改为 `emit("commit", buildSubmitPrefill(...))`。
2. `AuditOnlineView.vue`：新增 `submitPrefill`；详情 → 提交页时保留 `detailRow`，提交页「返回」回到原详情；提交成功回列表并清空预填。
3. `SqlSubmitView.vue`：新增 `prefill` 入参；组/实例下拉由 `watch` 改为显式 `@change` 串联，预填链路按「组 → 实例 → 库」逐级 await 后再落库名；顶部加来源提示条。

预览冒烟新增 10 项并全部通过：

```text
workflowCommitPrefill     提示条含 #5338
workflowCommitTitle       名称回填「DDL - 车辆表结构修改」
workflowCommitSql         编辑器含 axle_number（语句补分号）
workflowCommitSqlNoUse    编辑器不含 `use schedule-dev` 切库语句
workflowCommitGroup       组回填 Test-Express（id 3）
workflowCommitInstance    实例回填 test-express-rw（id 12）
workflowCommitDatabase    库回填 schedule-dev（不在资源清单时显示兜底选项）
workflowCommitBackup      备份回填「备份 SQL」
workflowCommitNeedCheck   带入的 SQL 未检测，提交按钮保持禁用
workflowCommitBack        返回后回到来源工单详情
```

脚本整体仍因 2 项既有断言（`dictNotice`、`dictSearchIndex`，见前文「已知差异」）返回 1，与本轮改动无关；workflow 相关断言全为 true、`errors: []`。

截图：`preview/smoke-workflow-commit.png`。

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.18`。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.18-windows-x64.dbxp
Signature: unsigned review candidate

dist/io.github.nixing178.archery-0.2.18-windows-x64.dbxp
6,528,964 字节
sha256 308afc8c1917fb7a1986ae1ab00a27f3ef201171c78f8007f81b8dbd24c327aa
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  11,031,040 B
manifest.json                             10,404 B   version=0.2.18
ui/index.html                          1,070,269 B   sha256 ec833cb1…（与本地 pnpm build 产物一致）
checksums.json                               526 B
```

包体比 0.2.17（5,166,324 B / exe 8,528,384 B）大约 1.36 MB：本机只有 Go 1.27，`plugin-cli` 不传 `-ldflags "-s -w"`，实测同一源码裁剪后 exe 为 7,660,544 B、未裁剪 11,031,040 B，属工具链差异，与本次功能无关。

待实测：真实工单上核对「可执行时间范围」的实际分隔符（当前按时间戳提取，`~`、`-`、`至` 都能取到两段）、详情解析失败时的兜底预填，以及原工单的组/实例在当前账号下不可提交时的下拉空值表现。

#### 预填 SQL 排除 USE 语句（v0.2.19）

`detail_content` 的第一行常是引擎插入的 `use \`schedule-dev\``（切库上下文）。原实现把它一起带进提交页，新工单会被这条语句改掉默认库，因此 `buildCommitSql` 增加过滤：整条匹配 `^use\s+[库名];?$`（忽略大小写、允许反引号/引号/换行）的语句丢弃，其余语句照旧补分号拼接。

红图：

```text
$ pnpm exec vitest run src/lib/workflow-detail.test.ts
+ use `schedule-dev`
+  USE schedule-dev;
+ use
+   `express_dev`;;
- alter table bs_vehicle_info add column axle_number int;
Test Files  1 failed (1)
     Tests  1 failed | 33 passed (34)
```

绿图：

```text
$ pnpm test        15 files / 202 tests passed
$ pnpm typecheck   exit 0
$ pnpm build       ui/index.html 1,063.70 kB
smoke              workflowCommitSqlNoUse = true，errors: []
```

截图：`preview/smoke-workflow-commit.png`（编辑器只剩 `alter table … axle_number …;`，1 行 89 字符）。

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.19`（0.2.18 的包不含本次 USE 过滤，已由 0.2.19 取代）。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.19-windows-x64.dbxp
Signature: unsigned review candidate

dist/io.github.nixing178.archery-0.2.19-windows-x64.dbxp
6,529,024 字节
sha256 de5a30aab1322c783cf726d448c56f7d68e0ccd8e24a7a68860c9cd3f3fdfe8f
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  11,031,040 B
manifest.json                             10,404 B   version=0.2.19
ui/index.html                          1,070,329 B   sha256 2dd22221…（与本地 pnpm build 产物一致）
checksums.json                               526 B
```

#### 「上线其他实例」预填换目标（v0.2.20）

详情页标题行在「查看提交信息」左侧新增「上线其他实例」：同样整页进入「提交SQL」，但目标实例与数据库留空，其余内容照原工单带入，用于把同一批 SQL 发布到另一套实例。

| 预填项 | 上线其他实例 | 查看提交信息 |
| --- | --- | --- |
| 上线单名称 / SQL / 组 / 备份 / 执行时间窗 | 带入 | 带入 |
| 目标实例 / 数据库 | 留空，重新选择 | 带入 |

实现：

1. `types.ts`：`SqlSubmitPrefill` 增加 `mode: "commit" | "other"`，提交页据此切换页头提示。
2. `lib/workflow-detail.ts`：抽出 `SubmitPrefillSource`；`buildSubmitPrefill` 标记 `mode: "commit"`；新增 `buildOtherInstancePrefill`，复用同一份取值后清空 `instanceName` / `dbName` 并标记 `mode: "other"`。
3. `SqlWorkflowDetailView.vue`：两个按钮共用一个 `prefillSource`，都通过 `commit` 事件切到提交页。
4. `SqlSubmitView.vue`：页头提示改为 `prefillNotice` computed，`mode=other` 时提示「请选择要上线的实例与数据库后重新检测提交」。

红图：

```text
$ pnpm exec vitest run src/lib/workflow-detail.test.ts
FAIL  buildSubmitPrefill > rebuilds the submit form from the ticket detail
       mode: "commit" 缺失
FAIL  buildOtherInstancePrefill > keeps everything except the target instance and database
TypeError: buildOtherInstancePrefill is not a function
FAIL  buildOtherInstancePrefill > still falls back to the list row for the other fields
TypeError: buildOtherInstancePrefill is not a function
Test Files  1 failed (1)
     Tests  3 failed | 33 passed (36)
```

绿图：

```text
$ pnpm test        15 files / 204 tests passed
$ pnpm typecheck   exit 0
$ pnpm build       version check ok: 0.2.20 / ui/index.html 1,064.27 kB
smoke              workflowOtherBtnOrder / Notice / Title / Group / InstanceEmpty / DatabaseEmpty / Sql / Backup / NeedCheck = true，errors: []
```

冒烟脚本整体仍因 2 项既有断言（`dictNotice`、`dictSearchIndex`）返回 1，与本轮改动无关。

截图：`preview/smoke-workflow-detail.png`（标题行「上线其他实例」紧邻「查看提交信息」左侧）、`preview/smoke-workflow-other-instance.png`（组 Test-Express、名称与 SQL/备份已带入，实例与库停在占位项，SQL提交禁用，页头提示需重新选实例与库）。

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.20`。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.20-windows-x64.dbxp
Signature: unsigned review candidate

dist/io.github.nixing178.archery-0.2.20-windows-x64.dbxp
5,167,171 字节
sha256 dd62573eefda6a0c6f8e76cfccfe6b7f8df990d749fd3cbc7ca9c8d7b718f75b
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  8,528,384 B
manifest.json                            10,404 B   version=0.2.20
ui/index.html                          1,070,986 B   sha256 78f7dba4…（与本地 pnpm build 产物一致）
checksums.json                              526 B
```

### 提交回滚请求锁定原工单目标

现象：回滚子页「提交回滚请求」弹窗把组 / 目标实例 / 数据库做成了可编辑下拉与输入框。虽然按原工单预填，但用户可以把回滚语句提交到别的组/实例/库上——回滚语句只对原库有意义，落错库等于误操作。

期望：三项固定为原工单值，弹窗内不可修改；按名称匹配不到时禁止提交，绝不退到列表里的其它组/实例。

红图（新增 `resolveRollbackTarget` 单测）：

```text
$ pnpm test -- workflow-detail
FAIL src/lib/workflow-detail.test.ts > resolveRollbackTarget > resolves the original group and instance ...
TypeError: resolveRollbackTarget is not a function
Test Files 1 failed (1) | Tests 6 failed | 36 passed (42)
```

绿图：

```text
$ pnpm test         16 files / 216 tests passed
$ pnpm typecheck    exit 0
$ pnpm build        version check ok: 0.2.24 / ui/index.html 1,067.21 kB
smoke               workflowRollbackDownload / Rollback / RollbackCount / RollbackExpand / RollbackSubmitFixed / RollbackSubmit = true，errors: []
```

实现要点：

1. `src/lib/workflow-detail.ts`：新增 `resolveRollbackTarget({row, groups, instances})`——组与实例只按名称同名匹配（忽略首尾空格与大小写），匹配不到返回 `missing=group|instance`，原工单无库名返回 `missing=database`，绝不回退到列表里的其它选项。
2. `src/views/SqlWorkflowRollbackView.vue`：弹窗删除组/实例下拉与库输入框，只展示原工单值；打开即解析目标，解析中禁用「确定」，解析失败或缺库时显示原因并禁用提交；提交参数取解析结果（非表单状态）。
3. `src/i18n.ts`：`rollbackSubmitHint` 改为「原工单的组/实例/库」；新增 `rollbackTargetFixed` / `rollbackGroupUnmatched` / `rollbackInstanceUnmatched` / `rollbackDatabaseMissing`，移除不再使用的 `rollbackGroupRequired` / `rollbackInstanceRequired` / `rollbackDatabaseRequired`（中英同步）。
4. `src/style.css`：`.rollback-fixed-note` 只读说明行。
5. `scripts/smoke-preview.mjs`：新增 `workflowRollbackSubmitFixed`——弹窗内 select/input 数为 0，且展示 Test-Express / test-express-rw / schedule-dev 与「不可修改」。

第二轮（用户追加）：弹窗内直接展示将提交的执行语句与回滚语句全文，不必先回子页表格核对。

红图：

```text
$ node scripts/smoke-preview.mjs
"workflowRollbackSubmitStatements": false
```

绿图：

```text
$ pnpm typecheck   exit 0
$ pnpm build       ui/index.html 1,068.68 kB
smoke              workflowRollbackSubmitStatements = true，errors: []
```

6. `SqlWorkflowRollbackView.vue`：汇总卡下方新增 `.rollback-submit-list`——按 `rows` 全量（不受子页搜索框过滤影响）逐条展示 `#序号` + 执行语句 + 回滚语句，`pre` 保留换行、长 SQL 折行，区域 `max-height: 260px` 内滚动，避免弹窗随语句数量无限变高。
7. `style.css`：`.rollback-submit-list` / `-item` / `-index` / `-line` / `-label` / `-sql` 一组样式；`scripts/smoke-preview.mjs` 增加 `workflowRollbackSubmitStatements`（弹窗内 2 条明细且含 `axle_number varchar(10)` 与 `insert into bs_vehicle_info (id, plate_no)`）。

截图：`preview/smoke-workflow-rollback-submit.png`（汇总卡：组 Test-Express、目标实例 test-express-rw、数据库 schedule-dev、2 条回滚语句 + 「沿用原工单，不可修改」；其下 `#1/#2` 两条执行语句与回滚语句全文；底部蓝色说明与取消/确定）。

待实测：真实 Archery 上原工单的组/实例已改名或权限收敛时的拦截文案（弹窗只提示、不允许提交）。

第三轮（用户截图反馈）：0.2.26 装进 DBX 后，弹窗只有 560px 宽，goInception 返回的 DDL 带硬换行与两级缩进，按原文渲染后 `alter table` / `bs_vehicle_info` / `modify` / `column …` 各自成行，看起来像排版坏掉；执行语句与回滚语句之间也缺少分隔。

红图（新增 `formatSqlPreview` 单测）：

```text
$ pnpm test -- workflow-detail
× formatSqlPreview > flattens the multi-line DDL that goInception rewrites
× formatSqlPreview > keeps whitespace inside string literals
× formatSqlPreview > keeps escaped quotes inside literals
× formatSqlPreview > trims the outer whitespace and blank lines
× formatSqlPreview > returns an empty string for blank input
Test Files 1 failed (1) | Tests 5 failed | 42 passed (47)
```

绿图：

```text
$ pnpm test         16 files / 221 tests passed
$ pnpm typecheck    exit 0
$ pnpm build        version check ok: 0.2.27 / ui/index.html 1,069.23 kB
smoke               RollbackSubmitFixed / Statements / Flattened / Wide / Submit = true，errors: []
```

8. `src/lib/workflow-detail.ts`：新增 `formatSqlPreview`——引号外（`'…'` / `"…"` / `` `…` ``，含反斜杠转义）的连续空白与换行压成单个空格，引号内原样保留；只用于弹窗预览，提交内容与子页表格仍是原文。
9. `SqlWorkflowRollbackView.vue`：弹窗加 `rollback-submit-modal`；语句改为「`#序号` 执行语句」+「回滚语句」两段，SQL 走 `formatSqlPreview` 单行化。
10. `style.css`：弹窗宽 `min(720px, 100%)`、正文 `max-height: min(72vh, 620px)` 整体滚动（去掉列表内层滚动），SQL 块浅底 + 边框、回滚语句标签用强调色。
11. `preview/mock.js` 第 1 条执行语句改成 goInception 真实多行格式；`scripts/smoke-preview.mjs` 增加 `workflowRollbackSubmitFlattened`（弹窗里是 `alter table bs_vehicle_info modify column …` 一行）与 `workflowRollbackSubmitWide`（弹窗宽 ≥ 680）。

截图：`preview/smoke-workflow-rollback-submit.png`（720px 弹窗：汇总卡 → `#1`、`#2` 各两段单行 SQL，条目间虚线分隔 → 蓝色说明 → 取消/确定）。

版本：`package.json` / `manifest.json` / `backend/main.go` 同步升至 `0.2.27`。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.27-windows-x64.dbxp
Signature: unsigned review candidate

dist/io.github.nixing178.archery-0.2.27-windows-x64.dbxp
5,168,581 字节
sha256 c87520c41e438136edc52dbfd8daa1920c0c9f02dbc7f95447471e098d6148a3
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  8,528,384 B
manifest.json                            10,404 B   version=0.2.27
ui/index.html                          1,076,145 B   sha256 7e8293ca…（与本地 pnpm build 产物一致）
checksums.json                              526 B
```

安装：DBX 插件中心 → 设置 → 允许未签名开发包 → 从文件安装 `dist/io.github.nixing178.archery-0.2.27-windows-x64.dbxp`。

## 16. 顶栏铃铛 → 待办列表（v0.2.28，2026-11-05）

按 Archery v1.14.0 源码核验链路：

| 用途 | 端点 / 来源 | 说明 |
| --- | --- | --- |
| 铃铛入口 | `common/templates/base.html` | `<a href="/workflow/"><span class="fa fa-bell">{{ todo }}</span></a>` |
| 待办页 | `/workflow/` → `sql/views.py::workflows` | 只渲染 `workflow.html`，数据靠前端异步拉 |
| 待办数据 | `POST /workflow/list/` → `common/workflow.py::lists` | 参数 `limit / offset / workflow_type / search`，返回 `{total, rows[]}`，行字段 `audit_id / workflow_type / workflow_title / create_user_display / create_time / current_status / audit_auth_groups / current_audit / group_name`；服务端按「用户所在资源组 + 当前审核组是我的组 + WAITING」过滤 |
| 待办详情 | `/workflow/<audit_id>/` → `sql/views.py::workflowsdetail` | 按 `workflow_type` 302 到 `/queryapplydetail/<id>/`、`/detail/<id>/`、`/archive/<id>/` |

`workflow_type`：1 查询权限 / 2 SQL 上线 / 3 数据归档（`workflow.html` 的下拉取值）；`current_status`：0 待审核 / 1 审核通过 / 2 审核不通过。

红图：

```text
$ pnpm exec vitest run src/lib/todo.test.ts
Error: Failed to load url ./todo ... Does the file exist?
Test Files  1 failed (1) / Tests  no tests

$ go test ./internal/rpc -run "TestWorkflowTodo" -count=1
--- FAIL: TestWorkflowTodosParams            archery/workflowTodos: method_not_found
--- FAIL: TestWorkflowTodosLimitRules        archery/workflowTodos: method_not_found
--- FAIL: TestWorkflowTodoTargetResolvesSqlWorkflow      archery/workflowTodoTarget: method_not_found
--- FAIL: TestWorkflowTodoTargetResolvesPrivilegeAndArchive  method_not_found
--- FAIL: TestWorkflowTodoTargetUnresolved   expected auditId validation error, got method_not_found
--- FAIL: TestWorkflowTodosAllowedInReadOnly method_not_found
```

绿图：

```text
$ pnpm exec vitest run src/lib/todo.test.ts
✓ src/lib/todo.test.ts (13 tests)

$ go vet ./...        # 无输出
$ go test ./... -count=1
ok  .../internal/archery  0.397s
ok  .../internal/rpc      0.394s

$ pnpm test
Test Files  18 passed (18) / Tests  266 passed (266)
$ pnpm typecheck      exit 0
$ pnpm build          version check ok: 0.2.28 / ui/index.html 1,108.04 kB (gzip 327.89 kB)
```

预览冒烟新增 10 项，全部 true：

```text
todoTitle（标题「待办列表」）        todoColumns（7 列）           todoRows（2 行）
todoStatus（待审核蓝标 ×2）          todoRecords（显示第 1 到第 2 条记录，总共 2 条记录）
todoTypeOptions（4 个类型选项）      todoTypeFilter（只看 SQL上线申请 → 1 行）
todoAuditDetail（audit_id 118 → 工单 5339 详情，审核入口可用）
todoBack（从详情返回待办列表）      todoPrivilegeDetail（audit_id 117 → 权限工单 40 详情）
```

同一次运行里另有 5 项与本次无关的既有失败（日期硬编码导致，非本轮引入）：
`dashboardTrendQuery / dashboardTrendRelease / dashboardRangeControl / dashboardRangePresets / dashboardRangeApplied`。
`preview/mock.js` 的 `/dashboard/` 固定回放 `2026-10-29 - 2026-11-05` 窗口，而页面默认区间按当天计算
（2026-11-05 → `2026-10-30 - 2026-11-06`），UI 因此改走 `/dashboard/api/` 取数，断言里的
`3,246 / 16 / 11` 与 `2026-10-29` 全部对不上；断言的期望值需要改成随当天计算。

实现要点：

1. `internal/archery/client.go`：`WorkflowTodos` 拼 `limit/offset/workflow_type/search` 打 `/workflow/list/`，按 `values()` 的下划线键名归一化；`WorkflowTodoTarget` 直接请求 `/workflow/<audit_id>/`，用 `Location` 末段正则（`/detail/`、`/queryapplydetail/`、`/archive/`）反推工单类型与工单号，兼容部署在子路径下的实例。
2. `internal/rpc/service.go`：新增 `archery/workflowTodos`（limit 默认 20 / 上限 200、offset 负值归零、`workflowType` 缺省 0）与 `archery/workflowTodoTarget`（`auditId` 必填）；两者只读，不加入 `writeMethods`。
3. `src/lib/todo.ts`：行归一化（兼容 Django 下划线键与 sidecar 驼峰键）、类型/状态文案与色调（待审核用蓝色 `is-info`）、`todoRowRange` 页脚区间，以及 `todoToSqlWorkflowRow` / `todoToPrivilegeRow` 两个详情页入参构造。
4. `src/views/TodoView.vue`：7 列表格 + 类型筛选 + 标题搜索 + 刷新/紧凑 + 服务端分页；「审核」先解析工单号再按类型进 `SqlWorkflowDetailView` / `PrivilegeDetailView`，归档工单提示到网页处理；标题区展示 Archery 首页公告。
5. `src/lib/nav.ts` + `App.vue` + `SideNav.vue`：新增 `{ page: "todo" }` 路由（`routeKey=todo`，随窗口恢复），铃铛点击进入；`activeNav` 返回空串让侧栏保持无选中，`SideNav` 的 `active` 放宽为 `NavKey | ""`。
6. `src/i18n.ts` 中英各增 24 键（`todo.*`），并把 `workflow.records` 的中文文案补成 Archery 原文「显示第 X 到第 Y 条记录」。

待实测：真实 Archery 账号（有资源组审核权限）上核对 `/workflow/list/` 的返回顺序与「审核」跳转；无 `sql.audit_user` 但属于当前审核组的账号是否仍能在待办里看到工单；归档工单的提示文案。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist/io.github.nixing178.archery-0.2.28-windows-x64.dbxp
Signature: unsigned review candidate

dist/io.github.nixing178.archery-0.2.28-windows-x64.dbxp
5,215,167 字节
sha256 c65372608f4324ffb6aec9f4f13e028da1ff33f85dc7e8aa42b2ca9672e6f785
```

截图：`preview/smoke-todo.png`（铃铛进入的待办列表：公告条 + 待办列表标题 + 全部/搜索/刷新/列 + 7 列表格 + 页脚记录数）。

## 17. 待办列表：铃铛支持重复点击（v0.2.30，2026-11-05）

问题：`TodoView` 没带 `:key`，点铃铛只是 `go({ page: "todo" })`。已经在待办页（或停在该页里打开的工单详情）时再点铃铛，路由没变化、组件不重建，页面毫无反应；而左侧菜单的约定是「重复点击即重新进入」，铃铛不一致。

红图（加 `:key` 之前）：

```text
todoBellReenter=false      # 停在 workflow_type=2 筛选态再点铃铛，仍是 1 行
todoBellFromDetail=false   # 停在 SQL 上线工单详情再点铃铛，仍留在工单详情
```

修复一行：`src/App.vue` 给 `TodoView` 加 `:key="navEntry"`。`go()` 每次调用都 `navEntry += 1`（该计数原本就为左侧菜单准备），组件随之重新挂载 → 筛选、页码、详情态全部归零并重新拉取 `/workflow/list/`。

绿图：

```text
todoBellReenter=true       # 重新进入后筛选归位，回到全部 2 条（含「查询权限申请」行）
todoBellFromDetail=true    # 详情页点铃铛直接回到待办列表
其余 10 项 todo 断言保持 true；errors: []
$ pnpm test        18 files / 273 tests passed（含本轮其他在途改动的用例）
$ pnpm typecheck   exit 0
$ pnpm build       version check ok: 0.2.30
同一轮冒烟仍有 5 项既有 dashboard 日期漂移断言失败（见第 16 节说明），与本轮无关
```

打包：`dist/io.github.nixing178.archery-0.2.30-windows-x64.dbxp` 构建通过（unsigned review candidate）；该产物随后因第 18 节的计数改动重建，最终大小与哈希见第 18 节。

## 18. 待办列表：铃铛右侧显示待办总数（v0.2.30，2026-11-05）

对齐 Archery `common/templates/base.html` 的 `<span class="fa fa-bell"></span> {{ todo }}`：数字直接跟在铃铛右侧，不做红点徽标，`0` 也照实显示。

红图（未实现时）：

```text
headerBellCount=false        # 页面上没有 .header-bell-count
headerBellCountRight=false   # 取不到元素，无法判断相对图标的位置
```

实现：

1. `src/App.vue`：`todoCount = ref<number | null>(null)`；`loadTodoCount()` 复用 `archery/workflowTodos`（`limit=1&offset=0`）读 `page.total`，不新增端点；在 `loadWorkspace()` 里与 `loadDashboard()` 并行触发；`watch(route.page)` 在离开待办页时补一次；`TodoView` 的 `@total` 回传总数，审核后列表重载即同步。
2. `src/views/TodoView.vue`：每次列表加载成功后 `emit("total", total.value)`（失败不回传，保留旧值）。
3. `src/components/AppHeader.vue`：新增 `todoCount?: number | null`，`bellCount` 为 `null` 时不渲染数字（只读连接、无审核权限、旧版 sidecar 都会走到 `null`）。
4. `src/style.css`：`.header-icon.has-count { width: auto; padding: 0 6px; gap: 3px }`，数字 12px、`tabular-nums`、`color: inherit`（跟随 hover 与深色主题）。

绿图：

```text
headerBellCount=true         # 铃铛按钮内出现「2」（mock 待办 2 条）
headerBellCountRight=true    # 数字左边界 ≥ 图标右边界，垂直居中对齐
其余 12 项 todo / 冒烟断言保持 true；errors: []
$ pnpm test        18 files / 273 tests passed
$ pnpm typecheck   exit 0
$ pnpm build       version check ok: 0.2.30 / ui/index.html 1,111.38 kB (gzip 328.88 kB)
```

打包（最终产物）：

```text
dist/io.github.nixing178.archery-0.2.30-windows-x64.dbxp
5,219,298 字节
sha256 4972c60ac052028138dd91c63e680ba9eb3c75d42d6108a35010c67615b1d8a6
```

截图：`preview/smoke-dashboard.png` 顶栏（你好，demo_user! ② 铃铛 — 2），`preview/smoke-todo.png`（待办页顶栏同样带数字）。

## 19. 系统公告：所有页面顶部统一动态获取（v0.2.31，2026-11-05）

问题：Archery 系统公告只在查询页 / 待办 / SQL 审核 / 提交 / 工单详情 5 处内联渲染，模板复制了 5 份；数据挂在 `dashboard` 响应上，只在登录与进入首页时刷新；首页解析降级会连带公告消失；数据字典顶部还写死了一条与公告同文的「库表设计规范」文案。查询历史 / 表结构 / 权限管理 / 工单回滚 / 占位页完全看不到公告。

红图（实现前）：

```text
$ go test ./internal/rpc -run TestAnnouncement -count=1 -v
--- FAIL: TestAnnouncementParsesDashboardAlert
    announcement_test.go:31: announcement: method_not_found: archery/announcement
--- FAIL: TestAnnouncementDegradesWhenDashboardUnavailable
    announcement_test.go:72: announcement: method_not_found: archery/announcement

$ pnpm vitest run src/lib/announcement.test.ts
Error: Failed to load url ./announcement in src/lib/announcement.test.ts. Does the file exist?
Test Files  1 failed (1)
```

实现：

1. `backend/internal/rpc/service.go`：新增只读 `archery/announcement`，复用 `client.DashboardHTML(ctx, "", "")` + `archery.ParseAnnouncement`；首页取不到时返回 `degraded` / `reason` 与 `announcement: null`，UI 不显示公告也不报错。
2. `src/lib/announcement.ts`：`ANNOUNCEMENT_TTL_MS = 60_000` 与 `shouldRefreshAnnouncement`（首次、到期、时钟回拨均重新拉取）。
3. `src/components/AnnouncementBar.vue`：统一公告条（alert + level + 标题/正文 + info 图标），所有页面共用。
4. `src/App.vue`：公告条挂在 `app-main` 顶部，位于 `v-if/v-else-if` 页面链之外；`loadWorkspace()` 启动拉取，`watch(routeKey)` 切页按 TTL 刷新；删除查询页内联公告与传给待办 / 审核页的 prop。
5. `src/views/*`：`TodoView` / `AuditOnlineView` / `SqlSubmitView` / `SqlWorkflowDetailView` 删除公告模板与 prop 链；`DictView` 删除硬编码规范文案（`dict.notice` 中英键、`.dict-notice` 样式同步移除）。
6. `preview/mock.js` 补 `archery/announcement` 预览桩；`scripts/smoke-preview.mjs` 断言统一到 `.app-announcement`，新增首页 `dashboardAnnouncement` 与字典页 `dictAnnouncement`。

绿图：

```text
$ go test ./internal/rpc -run TestAnnouncement -count=1 -v
--- PASS: TestAnnouncementParsesDashboardAlert (0.01s)
--- PASS: TestAnnouncementDegradesWhenDashboardUnavailable (0.00s)
$ go vet ./...            # 无输出
$ go test ./... -count=1
ok  .../internal/archery  0.351s
ok  .../internal/rpc      0.421s
$ pnpm test               19 files / 278 tests passed
$ pnpm typecheck          exit 0
$ pnpm build              version check ok: 0.2.31 / ui/index.html 1,109.94 kB (gzip 328.61 kB)
冒烟：dashboardAnnouncement / announcement / dictAnnouncement / workflowNotice 均 true；errors: []
```

同轮仍有 5 项既有 dashboard 日期漂移断言失败（`dashboardTrendQuery / dashboardTrendRelease / dashboardRangeControl / dashboardRangePresets / dashboardRangeApplied`，见第 16 节说明），与本轮无关。

截图：`preview/smoke-dashboard.png`（首页顶部公告条）、`preview/smoke-query.png`、`preview/smoke-dict-empty.png`、`preview/smoke-workflow.png`（四页同一公告条）。

待实测：真实 Archery 上确认无 Dashboard 菜单权限的账号（公告接口降级）不影响其他页面；后台修改公告后，60 秒内切页可见新内容。

打包：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.31-windows-x64.dbxp
Signature: unsigned review candidate

dist/io.github.nixing178.archery-0.2.31-windows-x64.dbxp
6,575,873 字节
sha256 a408756611bb643d4ede819cc516102fb7672505c9be584d2de05e00add1943e
```

包体 6.27 MB 属历史 6.2 MB 档（exe 未裁剪，见第 15 节的工具链说明），与本次 UI 改动无关。

## 20. 按日统计图：日期刻度按时间跨度自适应（2026-11-05）

问题：x 轴标签此前按 `ceil(天数/10)` 固定切 10 份，与跨度、卡片宽度都无关。一年区间会排 11 个 `MM-DD`，首尾都是 `11-06`，无法区分年份；卡片变窄时也不会减少标签。

红图（实现前）：

```text
$ pnpm vitest run src/lib/trend.test.ts
FAIL src/lib/trend.test.ts > buildDateTicks > labels every day when a week fits the axis
TypeError: buildDateTicks is not a function
...
 Test Files  1 failed (1)
      Tests  8 failed | 281 passed (289)
```

实现：

1. `src/lib/trend.ts` 新增 `buildDateTicks(dates, plot, mode)`：按首尾日期算跨度，短区间按天等距抽稀、跨年长区间改按自然月；标签容量由「字宽 + 最小空隙」和绘图区宽度算出并封顶 8 个；首尾标签必留且改用 `start` / `end` 锚点朝内对齐，尾部不足一个步长时用末点替换，避免最后两个日期贴在一起。
2. `src/components/TrendChart.vue` 删除 `labelStep`，x 轴改为直接渲染 `buildDateTicks` 结果（默认元素数从「一天一个」变成实际显示的 6~8 个）。
3. `scripts/smoke-preview.mjs`：`dashboardRangeApplied` 不再比较 `.axis-label` 元素数，改为两档都 ≤ 8 且标签内容整体变化。

绿图：

```text
$ pnpm test          19 files / 289 tests passed
$ pnpm typecheck     exit 0
$ pnpm build         ui/index.html 1,111.17 kB (gzip 329.11 kB)
```

实测布局（1280×900，卡片宽 521px，viewBox 640×220）：

| 区间 | 天数 | 标签 | 结果 |
| --- | --- | --- | --- |
| 2026-10-30 ~ 2026-11-06 | 8 | `MM-DD` | 8 个全标，首尾贴边 |
| 2026-10-06 ~ 2026-11-05 | 31 | `MM-DD` | 6 个，间距均匀 |
| 2025-11-06 ~ 2026-11-06 | 366 | `YYYY-MM` | 6 个（2025-12 / 2026-02 / 04 / 06 / 08 / 10），无重复、无重叠 |

冒烟：`errors: []`；第 16 节记录的 5 项既有日期漂移断言仍为 false，本轮无新增失败。

打包（同版本号重新出包，覆盖第 19 节的旧 sha256；打包会话需先把 `C:\Program Files\Go\bin` 补进 PATH，否则报 `Failed to start Go module setup: program not found`）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Success: Built dist\io.github.nixing178.archery-0.2.31-windows-x64.dbxp
Signature: unsigned review candidate
6,576,421 字节
sha256 de5ecd722dc2103080771e4063e82ac2637ea959803c8002974F42A4B21DA81E
```

包内 `ui/index.html` 与本地构建产物 sha256 一致（`667bd5bb…c8e3f`），确认打进去的是新刻度逻辑。

### 区间超过 30 天改上下排列（同日追加）

- 规则：首尾日期相差 > 30 天时 `.stat-panels` 加 `is-stacked` 改单列；「最近30日」实际 31 个点、跨度 30 天，仍左右排列。
- 宽度自适应：`TrendChart` 用 `ResizeObserver` 量 `.trend-body` 的内容宽，viewBox 按 1:1 重建（两列 495×220、单列 1027×320；单列高度按 `宽 × 0.34` 封顶 320），字号与刻度不再随卡片等比缩放。
- 红：`dateSpanDays` / `shouldStackTrendPanels` is not a function → 5 failed | 29 passed。
- 绿：`pnpm test` 294 passed、`pnpm typecheck` exit 0。
- 实测（1280×900）：8 天与 31 天为两列（520.5px × 2）；32 天与 366 天为单列（卡片 1053px，两卡同宽同 left，上下相距 385px），刻度重叠数 0；单列 366 天仍是 6 个 `YYYY-MM`。
- 冒烟新增 `dashboardTrendStacked: true`（切「最近365日」后两卡同宽、上下排列）；既有 5 项日期漂移断言仍 false。
- 截图：`preview/smoke-dashboard-stacked.png`。
- 重新出包（覆盖上一轮同名产物）：6,576,667 字节，sha256 `7377567b…48753c`；包内 `ui/index.html` 与本地构建一致（`7d8ad11e…0301e5`）。

### 柱状图悬停读数（同日追加）

- `src/lib/trend.ts` 新增 `nearestColumn(x, count, plot, mode)`：折线取最近点、柱状取所在格心；落在绘图区外返回 `null`，避免在纵轴刻度或卡片留白上误触发首/末柱。
- `TrendChart.vue`：柱状图复用 `hoverIndex` 与折线浮层，显示「日期 / 系列名 / 数值」；被指到的柱改深蓝（`.trend-bar.is-active`），浮层贴柱顶，靠近顶部时自动翻到下方。
- 红：`nearestColumn is not a function` → 3 failed | 34 passed。
- 绿：`pnpm test` 297 passed、`pnpm typecheck` exit 0。
- 实测（1280×900）：31 天区间悬停第 16 根柱 → 浮层 `2026-10-21 / SQL 上线统计 / 10`，仅 1 根柱高亮；8 天区间同样命中，且不再需要靠柱顶数值标签。
- 冒烟：无新增失败（既有 5 项日期漂移仍 false）。
- 出包：6,576,808 字节，sha256 `e7b78c3d…6f9750`；包内 `ui/index.html` 与本地构建一致（`e5c04e2d…a3da0e`）。

## 21. 「我的权限」页优化（v0.2.33，2026-11-05）

范围拍板：完整版（P1 视觉 + P2 交互）、操作列只做复制、保持平表。

现状问题：操作列恒为 `-`；用户/实例/有效期逐行重复；无汇总、筛选、排序；`dense` 声明后没有开关（死状态）；权限级别直接显示英文枚举；有效期只是纯文本。

红图：

```text
$ pnpm typecheck
exit 0

$ pnpm exec vitest run src/lib/privilege.test.ts
Test Files  1 failed (1)
     Tests  19 failed | 30 passed (49)

$ node scripts/smoke-preview.mjs   # 旧构建 + 新 mock
privilegeMineSummary / Filters / Expiry / Level / Copy / Export / Total = false
errors: []
```

红图里有一条测试期望写错（把 `2026-1-5` 当成 2027-01-05），绿图阶段改为 `2027-1-5`，属用例笔误，不改产品行为。

绿图：

```text
$ pnpm test
Test Files  19 passed (19)
     Tests  325 passed (325)
$ pnpm typecheck   exit 0
$ pnpm build       ui/index.html 1,129.15 kB (gzip 333.57 kB)
冒烟：我的权限 10 项新增断言全 true，errors: []
```

实现要点：

1. `src/lib/privilege.ts`：新增 `privilegeExpiry`（本地年月日解析、UTC 日序号做差；0~7 天为 soon，负数 expired，空值/非法日期 unknown）、`privilegeCopyText`（表级 `库.表`、库级库名）、`privilegeLevelKey`（库级/表级，非 1 一律表级）、`filterUserPrivileges`（关键字 + 实例 + 级别 + 到期状态，AND 组合）、`sortUserPrivileges`（实例/数据库/到期日；到期日排序 unknown 恒最后；稳定 tie-break）、`summarizeUserPrivileges`（各状态条数与覆盖实例数）。
2. `src/views/PrivilegesView.vue`：页签一次取 `limit=200`，客户端筛选/排序/分页；汇总条、筛选行、可排序表头、到期徽标与过期行弱化、行内复制、CSV 导出、筛选空态/清空、加载失败重试、页脚记录数；接通 `dense` 紧凑开关；搜索仍走服务端（`search`），清空关键字会重新拉取。
3. `src/i18n.ts`：中英各增 26 键（级别、到期状态、汇总、筛选、复制、导出、记录数与截断提示）。
4. `src/style.css`：汇总条/筛选行/徽标/排序标记/复制按钮样式；`.privilege-mine-table` 设 `min-width: 820px`，窄窗口横向滚动，避免有效期徽标被固定列宽裁掉。
5. `preview/mock.js`：我的权限扩到 14 条（临期/过期/表级各 1 条，临期与过期日期按运行日计算），搜索覆盖实例名。
6. `scripts/smoke-preview.mjs`：新增 10 项断言（汇总条、筛选控件、到期徽标、库级文案、复制、导出、记录数、实例筛选与清空、表头排序）。

截图：`preview/smoke-privileges.png`（默认实例升序 + 临期/过期徽标 + 复制列 + 汇总条），另本地复核 1100 / 860 与深色（临时脚本与截图已删除）。

同轮仍有的 5 项既有失败（dashboard 日期漂移，见第 16 节）与本轮无关。

打包（0.2.32 已有同号包，升至 0.2.33）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.33-windows-x64.dbxp
Signature: unsigned review candidate
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  11,107,328 B
manifest.json                             10,419 B   version=0.2.33
ui/index.html                          1,136,905 B   sha256 45c2a3c2…（与本地 pnpm build 产物一致）
checksums.json                               526 B
```

产物：6,587,642 字节，sha256 `573706e7eb7fb8e083bf59a9d7c9706cd854b11126ebebee0edf7dee56d061ea`；
`*.artifact.json` 的 size / sha256 与产物一致。

包内 sidecar 直连（`python scripts/probe-sidecar.py`，exe 从包内解出，跑完已清理）：

```text
initialize -> {"plugin":{"id":"io.github.nixing178.archery","version":"0.2.33"},"protocolVersion":1}
host connect (connection.id only) -> success, version 1.14.0
ui connect (no id)              -> success
instances (no id)               -> [{"id":1,"name":"fake-mysql",...}]
PROBE PASS
```

版本：`package.json` / `manifest.json` / `backend/main.go` / README 同步升至 `0.2.33`。

## 22. 表结构 tab：内容高度动态变化（v0.2.37，2026-11-06）

问题：结果面板里的「表结构」tab 和结果 / 消息 / 历史一样由编辑器分隔条定高、面板内滚动。DDL 只有几行时下面留一大片空白；DDL 很长时（宽表）又在 300 多像素高的面板里滚，编辑区却始终占着 380px。

口径（本轮拍板）：高度随内容 + 溢出交给外层整体滚动（与第 15 节 v0.2.6 提交页同一口径，不在面板内滚）；只改「表结构」tab，其余三个 tab 的定高与内滚保持不变。

红图（实现前，`scripts/smoke-preview.mjs` 新增断言）：

```text
describeShortNotStretched   false   4 列 routes：面板仍拉到底，与查询列底部齐平（0 留白）
describeLongPanelGrows      false   30 列 bs_vehicle_info：面板仍 334px（内容 837px）
describeLongColumnScrolls   false   查询列 scrollHeight == clientHeight（整列不滚）
describeLongNoInnerScroll   false   .ddl-wrap 面板内滚（长 DDL 被关在面板里）
```

实现要点：

1. `src/App.vue`：`.query-column` 加 `:class="{ 'is-describe': active.panel === 'describe' }"`，只在表结构 tab 生效。
2. `src/style.css`：`.query-column.is-describe` 一组 6 条规则 —— 列 `overflow-y: auto`；`.query-panel` `flex: none`（编辑区不被表结构内容压缩）；`.result-panel` `flex: none` + `min-height: 220px`（空态 / 加载态不至于缩成一条）；`.result-body` `flex: 1 0 auto`（basis 取内容高度、不收缩：内容多高就多高，未到 220 下限时把空态撑满居中）；`.ddl-wrap` / `.grid-wrap` `flex: none`；`.result-body` / `.ddl-wrap` `overflow: visible`（滚动条只在列上出现）。
3. `preview/mock.js`：新增 `bs_vehicle_info`（30 列，长 DDL 样本）与 `legacy_import_log`（describe 只回列清单、无 DDL，走表格回退分支）。
4. `scripts/smoke-preview.mjs`：新增 10 项断言（短 DDL 不撑满 / 无内滚 / DDL 全可见，长 DDL 面板长高 / 列可滚 / 无内滚 / 编辑区高度不变，回退表格贴合内容，切回结果 tab 后列恢复不滚动且 `scrollTop` 归零）。

绿图：

```text
$ pnpm test
Test Files  20 passed (20)
     Tests  332 passed (332)
$ pnpm build        ui/index.html 1,190.40 kB (gzip 344.17 kB)
$ node scripts/smoke-preview.mjs
describeShortNotStretched / describeShortNoInnerScroll / describeShortDdlFullyVisible   true
describeLongPanelGrows / describeLongColumnScrolls / describeLongNoInnerScroll / describeLongEditorIntact   true
describeFallbackGridFits / describeBackToResult   true
errors: []
```

实测几何（1440×900，`.query-column` clientHeight 769，面板顶 552）：

| 状态 | 结果面板高度 | 查询列 | 面板内滚动 |
| --- | --- | --- | --- |
| 空态（未选表） | 220（下限），空态提示在 174px 面板体内居中 | 不滚 | 无 |
| routes（4 列） | 276 | 不滚，距列底 57px | 无（`.ddl-wrap` client 190 == scroll 190） |
| bs_vehicle_info（30 列） | 813 | 滚 479px（scroll 1248） | 无（`.ddl-wrap` client 727 == scroll 727） |
| legacy_import_log（无 DDL） | 299（列清单表格 4 行） | 不滚 | 无（`.grid-wrap` client 187 == scroll 187） |

窄窗口与深色（同一套规则，2x 截图复核，编辑区高度全程 426 / client 424，未被表结构内容压缩）：

| 视口 | routes（4 列） | bs_vehicle_info（30 列） |
| --- | --- | --- |
| 1440×900 | 面板 276，列不滚，`.ddl-wrap` 无内滚 | 面板 813，列滚 479px |
| 1280×800 | 面板 276，列滚 42px（编辑区 426 + 面板 276 > 列 669） | 面板 813，列滚 579px |
| 1100×720 | 面板 276，列滚 122px | 面板 813，列滚 659px |
| 860×600 | 面板 276，列滚 242px（结果面板需滚一下才可见） | 面板 839，列滚 805px |
| 1440×900 深色 | 面板 276，列不滚 | 面板 813，列滚 479px |

各视口滚到底后 `.ddl-body` 底边（871 / 771 / 691 / 572）都在面板下边距之内，末行不被裁切；深色沿用 `.ddl-body` 原有变量，无新增颜色。

### 去掉 DDL 上方的小标题（同一轮）

DDL 上方的「Create Table」小标题纯重复（建表语句首行就是 `CREATE TABLE`），删除：`DescribePanel.vue` 去掉 `h3.ddl-title`、`style.css` 去掉 `.ddl-title` 规则、`i18n.ts` 中英各删 `describe.ddlTitle`；冒烟新增 `describeNoDdlTitle`（`.ddl-title` 计数为 0）。上面两表因此比带标题时矮 24px（标题行 + 6px 下边距），数字已按现状重测；无 DDL 的回退表格不受影响（299 不变）。

截图：`preview/smoke-describe.png`（6 列 addresses）、`preview/smoke-describe-tall.png`（30 列，滚动前后），另本地 2x 复核空态 / 短 / 长 / 回退四种状态与 1280 / 1100 / 860 / 深色四种视口（临时脚本与截图已删除）。

同轮仍有的 7 项既有失败（首页按日图日期漂移 5 项，见第 16 节；另有 `sidebarChildren`、`workflowCheckEmptyCompact`）与本轮无关。

打包（0.2.36 已打过，但缺「去掉 Create Table 标题」这一改，`dist/` 里也没有同号包，直接升至 0.2.37）：

```text
$ pnpm build
version check ok: 0.2.37

$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Packaging: dist\io.github.nixing178.archery-0.2.37-windows-x64.dbxp
Success: Built dist\io.github.nixing178.archery-0.2.37-windows-x64.dbxp
Signature: unsigned review candidate
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  8,604,672 B   （本机 go1.22.12）
manifest.json                             10,419 B   version=0.2.37
ui/index.html                          1,198,161 B   sha256 4384c2be…（与本地 pnpm build 产物一致；is-describe 命中 9 处，ddl-title 命中 0 处）
checksums.json                               526 B
```

产物：5,238,751 字节，sha256 `ae540bba3558a626bf479dc4214a759fe407266fc103976763381e0f88f66981`；
`*.artifact.json` 的 size / sha256 与产物一致。

包内 sidecar 直连（`python scripts/probe-sidecar.py`，exe 从包内解出，跑完已清理）：

```text
initialize -> {"plugin":{"id":"io.github.nixing178.archery","version":"0.2.37"},"protocolVersion":1}
host connect (connection.id only) -> success, version 1.14.0
ui connect (no id)              -> success
instances (no id)               -> [{"id":1,"name":"fake-mysql",...}]
PROBE PASS
```

版本：`package.json` / `manifest.json` / `backend/main.go` / README（中英）同步升至 `0.2.37`。

## 23. 结果面板页签多开 + 行数上移（v0.2.38，2026-11-06）

需求四条：① 行数移到查询目标区；② 表结构页签多开；③ 查询结果页签多开；④ 页签可重排。

拍板口径：页签挂在外层编辑器 tab 上（切编辑器 tab 就换一整套）；系统页签「消息 / 查询历史」固定在最前、不可关闭；动态页签「结果 N / 表结构」按打开顺序排在后面，带 × 可关、可拖拽互调；结果序号只增不回收，不设上限。

红图（实现前）：

```text
$ pnpm exec vitest run src/lib/panels.test.ts
Test Files  1 failed (1)
     Tests  17 failed (17)        # panels.ts 只留签名，全部 throw "not implemented"

$ node scripts/smoke-preview.mjs
configLimitGrouped: false         # 行数仍在底部执行区
```

红图里 `movePanel` 有一条期望写错（把「插到目标位置」写成了「目标之后」），绿图阶段改成正确期望，属用例笔误。

绿图：

```text
$ pnpm test
Test Files  21 passed (21)
     Tests  350 passed (350)      # 新增 src/lib/panels.test.ts 18 条
$ pnpm build        version check ok: 0.2.37
$ node scripts/smoke-preview.mjs
configLimitGrouped / resultTabsMulti / resultTabClosable /
describeTabsMulti / describeTabReuse / describeTabClose / panelTabReorder = true
errors: []
```

实现要点：

1. `src/lib/panels.ts`（新增，纯逻辑）：`systemPanels` / `createResultPanel` / `createDescribePanel` / `initialPanels` / `nextResultIndex` / `emptyResultPanel` / `appendPanel` / `findDescribePanel` / `describeTitle` / `isClosable` / `closePanel` / `movePanel`。表结构页签 id 由「实例-库-表」拼成，天然去重；跨库重名标题升级成 `库.表`，库名也撞再升 `实例.库.表`。关闭激活页签时优先选右侧邻居，没有右侧退回左侧；拖拽只在动态页签之间生效。
2. `src/types.ts`：新增 `ResultPanelTab` / `DescribePanelTab` / `SystemPanelTab` 与联合类型 `PanelTab`；`QueryTab` 用 `panels` + `activePanel` 取代原来的 `result` + `panel`，错误 / 信息摘要仍留在 `QueryTab` 供「消息」页签用；`createTab` 默认 `[消息, 查询历史, 结果 1]` 并激活结果 1。
3. `src/App.vue`：`activePanel` / `panelLabel` / `activatePanel` / `closePanelTab` / `dragPanel` / `dropPanel` / `activateLatestResult`；`execute` 每次执行开一条结果页签（首次复用那条空的结果 1），失败时保留失败页签并切到「消息」；`selectTable` 命中同表就原地刷新；全局单例 `describe` 改为按页签存，右侧「表」下拉取最近打开的表名，独立「表结构」页取最近一条；行数控件搬进 `.config-section`（表之后）。
4. `src/style.css`：结果页签横向滚动、动态页签 `cursor: grab/grabbing`、`.nav-tab-close` 悬停转 `--danger`。
5. `scripts/smoke-preview.mjs`：新增 7 项断言 —— 行数归组、结果页签多开与可关、表结构多开与同表复用、关闭动态页签、拖拽重排（系统页签仍在原位）、切回结果页签后列恢复不滚动。

过程中修掉两个真问题：

- 新建页签后直接改 `createXxxPanel()` 返回的对象不触发重渲染（对象还没被 proxy 包过，加载态永远停在「正在读取表结构…」）：改成 `pushPanel()` 追加后从数组里取回响应式代理再改。
- 首次执行会多出一条永远空着的「结果 1」：`emptyResultPanel()` 命中那条空页签就复用。

截图：`preview/smoke-config.png`（结果 1 / 结果 2 多开 + 行数归组 + 执行计划页签）、`preview/smoke-describe.png`（orders / addresses 两条表结构页签各带 ×），另本地深色 2x 复核页签配色（临时脚本与截图已删除）。

打包（0.2.37 已打过但缺本轮页签改动，直接升至 0.2.38）：

```text
$ pnpm build
version check ok: 0.2.38

$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Packaging: dist\io.github.nixing178.archery-0.2.38-windows-x64.dbxp
Success: Built dist\io.github.nixing178.archery-0.2.38-windows-x64.dbxp
Signature: unsigned review candidate
```

包内容（解包核对）：

```text
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe  8,604,672 B   （本机 go1.22.12）
manifest.json                             10,419 B   version=0.2.38
ui/index.html                          1,201,791 B   sha256 fd4a2680…（与本地 pnpm build 一致；nav-tab-close 命中 3 处，ddl-title 命中 0 处）
checksums.json                               526 B
```

产物：5,240,008 字节，sha256 `c39d662a9aa2b4be41d9056a8fdb15b5a7ce9638964a2ac90faa88a9454513bb`；
`*.artifact.json` 的 size / sha256 与产物一致。

包内 sidecar 直连（`python scripts/probe-sidecar.py`，exe 从包内解出，跑完已清理）：

```text
initialize -> {"plugin":{"id":"io.github.nixing178.archery","version":"0.2.38"},"protocolVersion":1}
host connect (connection.id only) -> success, version 1.14.0
ui connect (no id)              -> success
instances (no id)               -> [{"id":1,"name":"fake-mysql",...}]
PROBE PASS
```

版本：`package.json` / `manifest.json` / `backend/main.go` / README（中英）同步升至 `0.2.38`。

## 24. 结果页签改为就地刷新 + 加减按钮（2026-11-07）

需求两条：① 移除「每次执行新增一条结果 N」；② 结果页签行右侧加「+ / −」按钮。

拍板口径：

- 执行落到「当前激活的结果页签」，没有激活结果页签时退回最后一条结果页签；全部关掉时按当前最大序号补一条（序号只增不回收的规则不变）。
- `+` 手动新开一条空结果页签并激活；`−` 关掉当前结果页签，停在「消息 / 查询历史 / 表结构」上时为禁用态（不猜要关哪条）。
- 执行失败仍切到「消息」，失败的那条结果页签保留原地（下次执行就地覆盖）。

红图（实现前，先写测试再实现）：

```text
$ pnpm test -- --reporter=json --outputFile=test-results/panels-red.json
failed 3 total 352
result tabs runs into the active result tab instead of opening another one | TypeError: targetResultPanel is not a function
result tabs falls back to the last result tab when a system or describe tab is active | TypeError: targetResultPanel is not a function
result tabs has nothing to run into once every result tab is closed | TypeError: targetResultPanel is not a function
```

绿图：

```text
$ pnpm test       failed 0 passed 352 total 352
$ pnpm typecheck  无输出（通过）
$ pnpm build      version check ok: 0.2.38 / 214 modules / ui/index.html 1,195.10 kB
$ node scripts/smoke-preview.mjs
resultTabsNoAutoAdd / resultTabClosable / resultTabActions /
resultTabAdd / resultTabRemove / resultTabRemoveDisabled = true
errors: []
```

冒烟脚本 255 项里另有 7 项既有失败（`dashboardTrendQuery` / `dashboardTrendRelease` / `dashboardRangeControl` / `dashboardRangePresets` / `dashboardRangeApplied` / `sidebarChildren` / `workflowCheckEmptyCompact`），原因分别是首页统计窗口按「当天」算而 mock 固定 `2026-10-29 ~ 2026-11-05`、导航子项数与提交页检测面板高度，均与本轮改动无关。

实现要点：

1. `src/lib/panels.ts`：删掉 `emptyResultPanel`（找空页签复用的旧逻辑），新增 `targetResultPanel(panels, activeId)` —— 激活项是结果页签就返回它，否则退回最后一条，一条都没有时返回 `undefined`。
2. `src/App.vue`：`execute` 改用 `ensureResultPanel(tab)`（`targetResultPanel` + 兜底新建），不再每次 `pushPanel`；新增 `addResultTab` / `closeResultTab`，`activateLatestResult`（载入历史后回结果页签）复用同一个兜底；模板在页签行右侧加 `.result-tab-actions` 两个 `.tab-action` 按钮，`−` 绑 `:disabled="activePanel.kind !== 'result'"`。
3. `src/i18n.ts`：新增 `result.addTab` / `result.closeTab`（中英各一条，保持 key 对齐）。
4. `src/style.css`：`.result-tab-actions` + `.tab-action`（22×22、1px `--border-strong`、4px 圆角、`--bg` 底），悬停转 `--accent`，禁用 0.4 透明度；深色主题沿用同一套变量。
5. `scripts/smoke-preview.mjs`：`resultTabsMulti`（执行两次 → 两条结果页签）改成 `resultTabsNoAutoAdd`（执行两次仍只有「结果 1」），新增 5 项断言 —— 加减按钮存在、`+` 新开并激活「结果 2」、`−` 关闭回到「结果 1」且旧结果还在、停在「消息」上 `−` 为禁用态、系统页签上点 `−` 不误关页签。

像素判定（临时 Playwright 脚本，4x 缩放，跑完已删）：14 项全通过 —— 标题栏高 44px、按钮 22×22、间距 4px、`+` 距面板右边界 14px（等于标题栏 `padding`）、按钮垂直居中、页签行右边界不越过按钮；深色主题下描边 `rgb(61,68,76)`、文字 `rgb(173,181,189)`。截图：`preview/check-tab-actions.png`（结果 1 + 加减按钮）、`preview/check-tab-actions-plus.png`（结果 2 激活）、`preview/check-tab-actions-dark.png`（深色）。

版本：升 `0.2.39`（`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）—— 0.2.38 已打包过一次且缺本轮改动，沿用「已打过就升号」的惯例。

打包：

```text
$ pnpm test          failed 0 passed 352 total 352
$ pnpm typecheck     通过
$ pnpm build         version check ok: 0.2.39 / 214 modules / ui/index.html 1,195.10 kB
$ node scripts/make-preview.mjs
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Packaging: dist\io.github.nixing178.archery-0.2.39-windows-x64.dbxp
Signature: unsigned review candidate
```

包内容（解包核对）：

```text
checksums.json                              526 B
manifest.json                            10,419 B   version=0.2.39
assets/connection.svg                       415 B
assets/plugin.svg                           571 B
bin/windows-x64/dbx-plugin-archery.exe 8,604,672 B   （本机 go1.22.12）
ui/index.html                         1,203,114 B   sha256 48ca252d…（与本地 pnpm build 一致；result-tab-actions 命中 2 处、新增结果页签 命中 1 处）
```

产物：5,240,223 字节，sha256 `cd0f58014fe089161d69b6a3651353ecbe49c32b635ad27f4f03836abfd31042`；`*.artifact.json` 的 size / sha256 与产物一致。

包内 sidecar 直连（`python scripts/probe-sidecar.py --exe <解包后的 exe>`，跑完已清理解包目录）：

```text
initialize -> {"plugin":{"id":"io.github.nixing178.archery","version":"0.2.39"},"protocolVersion":1}
host connect (connection.id only) -> success, version 1.14.0
ui connect (no id)              -> success
instances (no id)               -> [{"id":1,"name":"fake-mysql",...}]
PROBE PASS
```

## 25. 选中即执行修通 + 多语句逐条执行（v0.2.40，2026-11-07）

现象：用户只选中一条 SQL 点「SQL 查询」，却收到 MySQL 1064（`near '...' at line 2`）。

排查（预览页拦 `archery/query` 的 `sqlContent`）：

```text
选中第 2 行 + 点按钮      -> ["select * from sm_dept limit 1"]        # 一条、无换行
选中第 1-2 行 + 点按钮    -> ["select * from sm_post limit 1"]        # 发的是光标所在语句，不是选区
选中后按 Ctrl+Enter       -> []，文档多出两个空行                     # 键位根本没执行
```

三条入口各有问题：

1. `SqlEditor.vue` 的 `keymap.of([{key:"Mod-Enter"}])` 排在 `basicSetup` 之后，而 `basicSetup` 的 `defaultKeymap` 已把 `Mod-Enter` 绑成 `insertBlankLine`（`@codemirror/commands@6.11.1` `{key:"Mod-Enter", run:insertBlankLine}`），执行键位永远抢不到 → `Ctrl+Enter` 只插空行。
2. 「SQL 查询」按钮与全局 `Ctrl+Enter` 都是 `run()`（不带选区），走「光标所在语句」，选区被忽略。
3. `sqlForRun` 对选区原样 `return selection.trim()`，选区跨多条时把多语句整段发给 Archery；取不到光标时更会 `return tab.sql.trim()` 把整段发出 —— 这就是 1064 里出现 `at line 2` 的来源。

拍板口径（用户选定）：**选中多条语句时逐条顺序执行、每条一个结果页签**；无选区仍是「光标所在语句」，第 1 条复用当前结果页签、其余各开一条。

红图：

```text
$ pnpm test
failed 5 total 357
runSteps runs every statement of a multi-statement selection in order | TypeError: runSteps is not a function
runSteps strips the trailing semicolon of a single-statement selection | ...
runSteps keeps the script position of the statement at the cursor | ...
runSteps runs the whole script when neither selection nor cursor is available | ...
runSteps returns nothing when the selection holds no statement | ...
```

绿图：

```text
$ pnpm test        failed 0 passed 357 total 357
$ pnpm typecheck   通过
$ pnpm build       version check ok: 0.2.40 / 214 modules / ui/index.html 1,195.66 kB
$ node scripts/smoke-preview.mjs
selectionRunsStatements / ctrlEnterNoBlankLine / selectionTabPerStatement /
selectionHintPerTab / ctrlEnterCursorStatement / cursorStatementHint = true
errors: []
```

冒烟 261 项里 7 项既有失败同第 24 节（首页统计窗口日期漂移、导航子项数、提交页检测面板高度），与本轮无关。

实现要点：

1. `src/lib/sql.ts`：新增 `runSteps(sql, selection, cursor?)` 与 `RunStep`（text / index / total）。选区按 `splitStatements` 切开逐条返回（去掉结尾分号）；无选区取光标语句并带上它在整段里的序号；光标也拿不到时兜底整段。`statementAt` 抽出 `statementIn` 复用同一套光标定位。
2. `src/App.vue`：删除 `sqlForRun`；`execute` 加 `fresh` 参数并返回是否成功（第 2 条起各开一条结果页签，失败即停）；新增 `runAllSteps` 逐条执行，多条时给「已执行第 i / N 条语句」；`run()` 统一从 `editorSelection()` 读选区 + 光标（按钮 / 编辑器快捷键 / 全局快捷键共用一份）并加 `tab.running` 互斥；`explain()` 取第 1 条。
3. `src/components/SqlEditor.vue`：键位改 `Prec.highest(keymap.of(...))`，`Ctrl+Enter` 真正执行，事件仍携带选区与光标。
4. `src/i18n.ts`：`editor.runHint` 补充「多条逐条执行、各占一条结果页签」（中英）。
5. `scripts/smoke-preview.mjs`：拦截 `archery/query` 记录实际发出的 SQL，新增 6 项断言 —— 选中两条顺序发出两条、`Ctrl+Enter` 不插空行、每条一个页签（第 1 条复用当前页签）、每页签带「第 i / N 条」提示、无选区只发光标语句、光标提示序号正确。

端到端复核（临时 Playwright 脚本，4x 截图，跑完已删）：11 项全过 —— 只选一行发一条、选 1-2 行发两条且顺序正确、3 条各占一页签（新增 N-1 条）、`Ctrl+Enter` 只发一次（不与全局快捷键重复）、无选区走光标语句。截图：`preview/check-run-multi.png`（两条语句 → 结果 1 / 结果 2）、`preview/check-run-tabs.png`（三条语句 → 结果 1~4）。

打包（版本升 `0.2.40`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.40-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                              526 B
manifest.json                            10,419 B   version=0.2.40
assets/connection.svg                       415 B
assets/plugin.svg                           571 B
bin/windows-x64/dbx-plugin-archery.exe 8,604,672 B
ui/index.html                         1,203,693 B   sha256 6f5ccb71…（与本地 pnpm build 一致；包内 `Prec.highest` 命中 4 处、`已执行第` 命中 1 处）
```

产物：5,240,417 字节，sha256 `6498d71b8228e5fb2cfe25450c486940d965d18ef8f57cc89f7e23f9e0cfb50d`；`*.artifact.json` 与产物一致。

包内 sidecar 探针：`initialize → version 0.2.40`、host/ui connect 成功、`instances` 正常，`PROBE PASS`（解包目录已清理）。

## 26. 结果页签关闭按钮增强（2026-11-07）

现象：页签右侧的 `×` 是 16×16 的文本字形，颜色 `--fg-subtle`、13px，矢量感差、对齐偏低，命中区也小。

改动：

1. `src/components/NavIcon.vue`：新增 `close` 图标（16 网格，两条 45° 线，`stroke-linecap: round`）。
2. `src/App.vue`：关闭控件由文本 `×` 改为 `<NavIcon name="close" :size="11" />`，保留 `@click.stop` 与 `title`。
3. `src/style.css`：`.nav-tab-close` 命中区 16×16 → **20×20 / 4px 圆角**，图标线宽压到 1.75（11px 下不发虚）；默认 `--fg-subtle`，页签悬停 / 激活时升到 `--fg-muted`，自身悬停染 `--danger` + `rgba(220,53,69,.12)` 底、按下缩到 0.92；激活态页签的关闭按钮同样能变红（选择器写成 `.nav-tab .nav-tab-close:hover`，否则被 `.nav-tab.active .nav-tab-close` 的 (0,3,0) 权重压住）；页签右内边距 6px → 4px。

像素判定（临时 Playwright，4x 缩放，跑完已删）11 项全过：命中区 20×20、与页签垂直居中止差 ≤1px、图标 11×11、`stroke-width: 1.75px`、无文本字形、激活页签图标色 `rgb(73,80,87)`、页签高度仍 43px、悬停 `rgb(220,53,69) | rgba(220,53,69,.12)`、点击只关页签、深色下图标色 `rgb(173,181,189)`。截图：`preview/check-close.png`（常态）、`preview/check-close-hover.png`（悬停）、`preview/check-close-dark.png`（深色）。

回归：`pnpm typecheck` 通过、`pnpm build` 通过；`node scripts/smoke-preview.mjs` 261 项中仅第 24 节那 7 项既有失败，`resultTabRemove` / `describeTabClose` 等关闭相关断言全绿。

打包（版本升 `0.2.41`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ pnpm test        failed 0 passed 357 total 357
$ pnpm typecheck   通过
$ pnpm build       version check ok: 0.2.41 / ui/index.html 1,196.03 kB
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.41-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                              526 B
manifest.json                            10,419 B   version=0.2.41
assets/connection.svg                       415 B
assets/plugin.svg                           571 B
bin/windows-x64/dbx-plugin-archery.exe 8,604,672 B
ui/index.html                         1,204,065 B   sha256 0a40afb6…（与本地 pnpm build 一致；包内 `nav-tab-close:hover` 与 close 图标 path 各命中 1 处）
```

产物：5,240,513 字节，sha256 `63c9d0b1aa199126b5d6a447471b2ce85bf560411da0a66970b1cc4063c45143`；`*.artifact.json` 与产物一致。包内 sidecar 探针 `PROBE PASS`（解包目录已清理）。

## 27. 表名下拉「点一下就开」（2026-11-07）

现象：查询目标里的「表」要先把焦点挪走、再点回来才弹出候选，第一次点没反应。

根因：`TablePicker` 只在 `@focus` 里 `openOptions()`。以下三种情况输入框本来就保持焦点、`focus` 不会再触发，点击自然没反应：

1. 选完一张表：候选行的 `@mousedown.prevent` 阻止了失焦，输入框仍是焦点态；
2. 按 `Escape` 关掉候选：只清了列表，没清焦点；
3. 宿主 iframe 首次点击：第一下可能只把焦点交给输入框，不保证再派发一次 focus。

修复（`TablePicker.vue` + `style.css`）：

1. 输入框同时挂 `@mousedown` / `@click` / `@focus`，都走幂等的 `openOptions()`；点击即开，不依赖焦点变化。
2. `openOptions()` 在「已经打开但列表为空且没在加载」时补发一次 `emit("open")`，避免上次加载失败后一直空着。
3. 输入框右侧加下拉箭头（`chevron`，闭合朝下 / 展开朝上，`pointer-events: none`，线宽 1.7），表名区 `padding-right: 26px`，让纯文本框有「可展开」的可见提示，和上面的实例 / 库原生 select 视觉对齐。
4. 顺带修正：界面上可见的执行提示是 `query.tipRun`（`editor.runHint` 未被引用），第 25 轮改错了 key，这轮把 `query.tipRun` 中英都换成「选中即执行 / 多条逐条执行、各占一条结果页签」。

验证（临时 Playwright，4x 截图，跑完已删）10 项全过：未选实例库时禁用且点不开、首次点击即开（18 张表）、`Escape` 关闭后焦点仍在输入框且再点能开、选完表后焦点仍在输入框且再点能开、箭头方向随开合翻转且 `pointer-events: none`、关键字过滤行数变少、回车选中。

冒烟脚本新增 5 项（`tableCaret` / `tableReopenAfterPick` / `tableCaretRotates` / `tableEscapeCloses` / `tableReopenAfterEscape`），266 项里全为 true，其余仍是第 24 节那 7 项既有失败。截图：`preview/check-picker-open.png`（展开态 + 箭头朝上）、`preview/check-picker-picked.png`（收起态 + 新提示文案）。

打包（版本升 `0.2.42`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ pnpm test        failed 0 passed 357 total 357
$ pnpm typecheck   通过
$ pnpm build       version check ok: 0.2.42 / ui/index.html 1,196.62 kB
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.42-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                              526 B
manifest.json                            10,419 B   version=0.2.42
assets/connection.svg                       415 B
assets/plugin.svg                           571 B
bin/windows-x64/dbx-plugin-archery.exe 8,604,672 B
ui/index.html                         1,204,697 B   sha256 709a6d30…（与本地 pnpm build 一致；包内 `table-picker-caret` 命中 3 处、新提示「选中即执行选区」命中 1 处）
```

产物：5,240,605 字节，sha256 `06ae2842d1d0c514c0cc2ac2bf09d25a0526708f30c7dbdd26568c16fa4f1723`；`*.artifact.json` 与产物一致。包内 sidecar 探针 `PROBE PASS`（解包目录已清理）。

## 28. 实例 / 库下拉换成可搜索下拉（2026-11-07）

现象：查询目标里的「实例」「库」下拉打开卡顿、要等一下才出候选。

排查（原生 `<select>`，代码里没有任何 click / open 逻辑，卡顿只能来自控件本身）：

1. 宿主 iframe 首次点击常常只把焦点交给控件、不弹面板（和第 27 节的表下拉同源），表现为「点一下没反应，再点一下才弹」；
2. 候选是一次性铺开的：真实 Archery 的实例清单几十到几百条，单个 MySQL 实例的库几百到上千条，Chromium 打开原生 select 时会把所有 `<option>` 一起渲染，条数越多越慢；
3. 原生 select 没有搜索，几百个库里找一个只能滚。

修复：新增 `src/components/PickerSelect.vue`，实例 / 库不再用原生 select：

1. 触发器是普通按钮（`mousedown` 即开，JS 驱动，不吃 iframe 首次点击的亏），闭合显示当前值、展开显示搜索框 + 候选。
2. 候选只渲染前 200 条，其余用「还有 N 项，输入关键字继续筛选」提示（`lib/table-picker.ts` 新增 `optionWindow()`，复用 `filterTables` 的排序与前缀优先）。
3. 支持输入过滤、上下键移动、回车选中、Esc 关闭、点外部关闭；有值时触发器带 `×` 清空。
4. 打开下拉时补拉候选：实例为空时拉实例清单，库未缓存时拉库清单（`dbPending` 去重，重复点击不会重复请求），加载中显示 spinner。
5. `src/App.vue` 的 `onInstanceChange` 换成 `onInstancePick` / `onDbPick`；`dbLoading` / `instancesLoading` 驱动下拉加载态。

红图：`pnpm test` → `failed 3 total 360`，三条都是 `optionWindow is not a function`。绿图：`failed 0 passed 360`。

验证（临时 Playwright，注入 1000 个实例，4x 截图，跑完已删）14 项全过：

```text
打开耗时 31ms（< 400ms 门槛）  只渲染 200 行  底部提示「还有 800 项」
输入 inst_099 → 10 行、提示消失  ArrowDown + Enter 选中 inst_0991
库下拉首点即开、候选自动带出    选中后「表」恢复可用   × 清空生效
深色主题：弹层 rgb(26,30,35)、选中项 rgba(77,171,247,.16)、输入文字 rgb(233,236,239)
```

冒烟脚本新增 8 项（`targetPickers` / `instancePickerOpens` / `instancePickerOptions` / `instancePickerFilter` / `instancePicked` / `dbPickerOptions` / `dbPicked` / `tableReadyAfterPick`），274 项里全为 true，其余仍是第 24 节那 7 项既有失败。截图：`preview/smoke-target-picker.png`、`preview/check-instance-picker.png`、`preview/check-db-picker.png`、`preview/check-picker-dark.png`。

打包（版本升 `0.2.43`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ pnpm test        failed 0 passed 360 total 360
$ pnpm typecheck   通过
$ pnpm build       version check ok: 0.2.43 / ui/index.html 1,202.93 kB
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.43-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                              526 B
manifest.json                            10,419 B   version=0.2.43
assets/connection.svg                       415 B
assets/plugin.svg                           571 B
bin/windows-x64/dbx-plugin-archery.exe 8,604,672 B
ui/index.html                         1,211,058 B   sha256 0689594f…（与本地 pnpm build 一致；包内 `.picker-trigger` 命中 5 处、「输入关键字继续筛选」命中 1 处）
```

产物：5,242,187 字节，sha256 `b686b6423bf875aacb8c3c834765dccec3e32024c01309d7b665da9ca06d248c`；`*.artifact.json` 与产物一致。包内 sidecar 探针 `PROBE PASS`（解包目录已清理）。

## 29. SQL 上线「待我审核」只查当前轮到我的工单（2026-11-09）

现象：「待我审核」页签里会出现别人正在审、或状态早已结束的工单，不是「现在轮到我审」的队列。

按 Archery v1.14.0 源码核验：

| 端点 | 实现 | 结论 |
| --- | --- | --- |
| `/sqlworkflow_list_audit/` | `sql/sql_workflow.py::sql_workflow_list_audit` | 只是 `_sql_workflow_list` 外面加 `sql.audit_user` 装饰器，查询条件与普通列表完全一致，**不按当前审核组过滤** |
| `/workflow/list/` | `common/workflow.py::lists` | `current_status=WAITING` + `group_id in 我的资源组` + `current_audit in 我的审核组`，是「当前待我审核」的唯一权威来源 |

修复：`internal/archery/client.go` 的 `SqlWorkflows(audit=true)` 改为：

1. 分页拉取 `/workflow/list/`（`workflow_type=2`，带搜索），得到当前待我审核的 SQL 工单队列；
2. 队列为空直接返回空页，不再请求工单列表；
3. 拉取 `/sqlworkflow_list/` 的待审核工单（`navStatus=workflow_manreviewing`，带实例/组/日期/类型筛选），与队列按「工单名 + 资源组」求交集（同一资源组 + 同类型工单共用一条审批流配置），发起人用于排除同名不同人的工单；
4. 交集后再按 `offset/limit` 分页，`total` 为交集条数；两边各最多扫描 1000 行，避免超大待审队列拖慢页面；
5. 不再请求 `/sqlworkflow_list_audit/`，也不再需要 `sql.audit_user`；`AuditOnlineView` 删掉 403 降级与说明条，状态筛选在「待我审核」下锁定为待审核（`workflowScopeStatus`）。

红图：

```text
$ go test ./internal/rpc -run "TestSqlWorkflowsAudit" -count=1
--- FAIL: TestSqlWorkflowsAuditOnlyCurrentReviewer   total = 0, want 1
--- FAIL: TestSqlWorkflowsAuditPagesAfterIntersection total = 0, want 3
--- FAIL: TestSqlWorkflowsAuditKeepsFiltersAndForcesPendingStatus  expected /workflow/list/ to be called

$ pnpm exec vitest run src/lib/workflow.test.ts
Tests  2 failed | 31 passed (33)   TypeError: workflowScopeStatus is not a function
```

绿图：

```text
$ go vet ./...                     无输出
$ go test ./... -count=1           ok internal/archery 1.071s / ok internal/rpc 1.099s
$ pnpm test                        Test Files 21 passed / Tests 382 passed
$ pnpm typecheck                   exit 0（pnpm build 内）
$ pnpm build                       version check ok: 0.2.48 / ui/index.html 1,216.66 kB（gzip 350.54 kB）
```

预览冒烟（`node scripts/smoke-preview.mjs`）新增 4 项，全部 true：

```text
workflowAuditOnlyMine      待我审核只剩待办队列里的 1 条「DDL - 车辆表结构修改」
workflowAuditStatusLocked  状态筛选在待我审核下禁用
workflowAuditNoDeniedNotice 不再出现 sql.audit_user 降级说明
workflowAuditRecovered     切回申请工单后恢复多行、状态筛选可用
```

其余 7 项失败与第 24/28 节相同（5 个 dashboard 日期硬编码 + `sidebarChildren` + `workflowCheckEmptyCompact`），非本轮引入。截图：`preview/smoke-workflow-audit-scope.png`（待我审核：1 行、状态「等待审核」锁定、页脚「显示第 1 到第 1 条记录，总共 1 条记录」）。

打包（版本升 `0.2.49`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ pnpm build        version check ok: 0.2.49 / ui/index.html 1,216.66 kB（gzip 350.54 kB）
$ pnpm preview      preview/index.html 已刷新
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.49-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                              526 B
manifest.json                            10,419 B   version=0.2.49
assets/connection.svg                       415 B
assets/plugin.svg                           571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,424 B   go1.22.12
ui/index.html                         1,224,832 B   sha256 2e322cd4…（与本地 pnpm build 一致）
```

产物：5,248,178 字节，sha256 `e23c3b869285e2af528b485719ede87f5baf40d562b68be10c643df36014e612`；`*.artifact.json` 与产物一致（size / sha256 相同）。包内 UI 抽查：`待我审核` 1 处、`sql.audit_user` 0 处、`auditDenied` 0 处；sidecar 构建信息为 go1.22.12。

打包注意：必须用 go1.22.x 构建（与 `backend/go.mod` 的 `go 1.22` 一致）；用 go1.27.0 构建的 sidecar 会涨到 11,118,592 B、整包 6,609,288 B。打包前先确保 PATH 中的 `go` 是 1.22.x。

## 30. 实例树搜索支持库 / 表（v0.2.50，2026-11-09）

现象：侧栏搜索框 placeholder 写「搜索实例 / 库 / 表」，实际只按实例名过滤。

预览复现（原实现）：

```text
KW demo_shop -> []      # 库名命中，整棵树为空
KW orders    -> []      # 表名命中，整棵树为空
KW zzzz      -> []      # 无任何提示
```

根因：`InstanceTree.vue` 的 `filteredInstances` 只调用 `filterByName(props.instances)`；库/表只在 `loadDbs` / `loadTables` 之后本地过滤，搜索既不触发展开也不触发加载，父级不命中就整枝消失。

红图（新增用例先冻结在 `src/lib/tree.test.ts`）：

```text
$ pnpm exec vitest run src/lib/tree.test.ts
TypeError: undefined is not a function  (treeKey)
Test Files  1 failed (1) / Tests  no tests

$ pnpm typecheck
src/lib/tree.test.ts(4,3): error TS2305: Module '"./tree"' has no exported member 'matchesNode'.
src/lib/tree.test.ts(7,3): error TS2305: Module '"./tree"' has no exported member 'treeKey'.
src/lib/tree.test.ts(8,3): error TS2305: Module '"./tree"' has no exported member 'visibleDbs'.
src/lib/tree.test.ts(9,3): error TS2305: Module '"./tree"' has no exported member 'visibleInstances'.
src/lib/tree.test.ts(10,3): error TS2305: Module '"./tree"' has no exported member 'visibleTables'.
```

实现：

1. `src/lib/tree.ts` 新增 `treeKey`、`matchesNode`（名称或备注列，大小写不敏感，空关键字视为命中）、`visibleInstances` / `visibleDbs` / `visibleTables`：父级自身命中显示整枝，子级命中保留父级；`NamedItem` 允许可选 `values`。
2. `src/components/InstanceTree.vue`：搜索词非空时 250ms 防抖预取「实例 → 库 → 表」（分批并发 4、结果缓存、run 令牌丢弃过期请求）；命中子级的实例/库自动展开，实例自身命中不改动用户展开状态；搜索态隐藏子级加载错误；无命中显示提示。
3. `src/i18n.ts` 中英各加 `sidebar.searching` / `sidebar.noMatch`。
4. `scripts/smoke-preview.mjs` 新增 3 项断言，截图 `preview/smoke-sidebar-search.png`。

绿图：

```text
$ pnpm exec vitest run  21 files / 398 tests passed（tree.test.ts 20 条）
$ pnpm typecheck        exit 0
$ pnpm build            version check ok: 0.2.50 / ui/index.html 1,218.49 kB（gzip 351.22 kB）
```

预览行为：

| 输入 | 结果 |
| --- | --- |
| `demo_shop` | `demo-mysql` 自动展开 → `demo_shop` 自动展开，显示其全部表 |
| `orders` | 仅保留含该表的库，库内只列出命中表 |
| `订单主库` | 命中备注列，保留 `demo_shop` 分支 |
| `demo-mysql` | 实例行 + 全部库（父级命中展示完整子树） |
| `zzzz` | 「没有匹配的实例 / 库 / 表」 |

冒烟：`sidebarSearchDb` / `sidebarSearchTable` / `sidebarSearchEmpty` / `instanceTree` / `tables` 全为 true，`errors: []`；其余 5 项既有失败（dashboard 日期硬编码等）与本次无关。

打包（版本升 `0.2.50`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步；打包前确保 PATH 中的 `go` 为 1.22.x）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.50-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.50
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,424 B   go1.22.12
ui/index.html                         1,226,688 B
```

产物：5,248,851 字节，sha256 `52911186b1822ef117a07ae56dc379b09bdcc9cc7bbc6d5397b3e2065972ad40`；`*.artifact.json` 的 size / sha256 与产物一致；包内 UI 抽查：`没有匹配的实例 / 库 / 表` 1 处、`sidebar.searching` 2 处。

## 31. 权限申请弹窗改版：编号分区表单（v0.2.51，2026-11-09）

现象（改版前）：

| 问题 | 表现 |
| --- | --- |
| 无字段标签 | 8 个控件全靠 placeholder 说明自己是谁，填完就丢上下文 |
| 层级扁平 | 说明 / 范围 / 时长三段混在一列等高控件里，看不出主次 |
| 授权时间是多余点击 | 一天 / 一周 / 一月 / 一年四个固定档，却要展开下拉再选一次 |
| 提示条与按钮不成体系 | 浅蓝提示块 + Bootstrap 默认绿按钮，与全站主色 `--accent` 不呼应 |
| 窄面板吃亏 | 定宽 560px + 单列堆叠，宿主面板拖窄后正文过长 |

实现：

1. `src/components/PrivilegeApplyDialog.vue`：抬头改「盾牌图标 + 主标题 + 一句副标题 + 圆形关闭」；正文拆 `01 申请说明 / 02 授权范围 / 03 授权时长` 三段（等宽字体编号 + 渐隐细线，是整窗唯一的重色）；每个字段补标签与红色必填星标，`label for` 绑定 `#pa-title` / `#pa-group` / `#pa-instance` / `#pa-level` / `#pa-database` / `#pa-limit`；资源组 / 实例、权限级别 / 数据库两列并排，表级时「数据表」独占整行；权限级别由裸 `DATABASE` / `TABLE` 改成「库级 · DATABASE」/「表级 · TABLE」；授权时间由下拉改四档分段控件（`role="radiogroup"` + `aria-checked`）；行数去掉与后缀打架的步进按钮，补「行」后缀与 100 / 500 / 1000 胶囊；下拉加载中 chevron 变转圈。
2. `src/i18n.ts`：中英各加 18 键（分区名、字段名、副标题、行数单位、页脚跟踪提示），`privilege.pick*` 占位去掉冒号。
3. `src/style.css`：新增 `.privilege-apply-mask` / `.privilege-apply-modal` 作用域样式（浅 / 深两套、`prefers-reduced-motion` 兜底、`max-width: 540px` 收紧留白、`460px` 段控件退回整行），共用 `.privilege-modal` 的审核 / 日志 / 回滚弹窗样式不动。
4. `scripts/smoke-preview.mjs`：新增 5 项断言（分区数、标签数、四档段控件、点选档位、胶囊回填）。

绿图：

```text
$ pnpm test         Test Files 21 passed (21) / Tests 398 passed (398)
$ pnpm typecheck    exit 0（pnpm build 内）
$ pnpm build        version check ok: 0.2.50 / ui/index.html 1,235.45 kB（gzip 354.48 kB）
```

预览行为：

| 检查 | 结果 |
| --- | --- |
| 分区标题 | 申请说明 / 授权范围 / 授权时长，`01/02/03` 编号 |
| 字段标签 | 默认 7 个（选到表级多出「数据表」一格） |
| 授权时间 | 4 档，点「一月」即选中（`.pa-segment.is-on`） |
| 行数胶囊 | 点 500 → `#pa-limit` 值为 `500` |
| 多选浮层 | 展开后压在正文之上，不被后续分区遮挡 |
| 窄面板 | 519px 视口下弹窗 479×664，页脚按钮留在视口内 |
| 深色 | 头部光晕、段控件选中态、提示条、错误条均走 `[data-theme="dark"]` |

冒烟新增 5 项全部 true：`privilegeApplySections` / `privilegeApplyLabels` / `privilegeApplyValiditySegments` / `privilegeApplyValidityPick` / `privilegeApplyLimitChip`。

其余 7 项失败与第 24 / 28 / 29 节相同（5 个 dashboard 日期硬编码 + `sidebarChildren` + `workflowCheckEmptyCompact`，本轮实测该面板 136px vs 阈值 130px），非本轮引入；`errors: []`。

两个坑（已写进 CSS 注释）：

1. 分区入场动画不能用 `animation-fill-mode: both`：动画结束后残留 identity `transform`，每个分区各自成为层叠上下文，把 MultiSelect 的绝对定位面板压到后面去；改 `backwards` 即可。
2. 窄面板媒体查询必须放在基础规则之后，否则 `display: flex` 之类的基础声明会反过来盖掉 `@media` 里的 `display: none`。

截图：`preview/smoke-privilege-apply.png`（三段表单 + 审批流程条 + 段控件选中「一月」+ 行数 500）。

打包（版本升 `0.2.51`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步；打包前确保 PATH 中的 `go` 为 1.22.x）：

```text
$ pnpm build        version check ok: 0.2.51 / ui/index.html 1,235.45 kB（gzip 354.48 kB）
$ pnpm preview      preview/index.html 已刷新
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.51-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.51
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,424 B   go1.22.12
ui/index.html                         1,243,901 B
```

产物：5,252,100 字节，sha256 `3aed2bbd95e5a6dcf0215bd1c6d8d12cf772da8b461662be5e113ed3b4f01a02`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.51，sidecar `go version -m` 为 go1.22.12（8,615,424 B，与 v0.2.49 / v0.2.50 产物同尺寸）。

## 32. 审核弹窗对齐：工单卡 + 审核意见（2026-11-09）

现象（改版前）：与第 31 节改完的申请弹窗并排时明显割裂。

| 问题 | 表现 |
| --- | --- |
| 抬头是裸文字 | 只有「审核权限申请」+ 一个 `×`，没有一句话说明「终止要写备注」 |
| 工单信息是一块灰卡 | 6px 圆角、10px 内边距，7 个字段两列挤在一起，标签与数值贴死 |
| 备注区没有显式规则 | 「终止必填」只藏在 placeholder 里，点了终止才报错 |
| 终止按钮比通过更抢眼 | Bootstrap 橙实心 `#f0ad4e` 的视觉重量大于绿色主操作 |

实现：

1. `src/style.css`：把第 31 节里 `.privilege-apply-modal` 前缀整体提成共用 `.pa-dialog`（89 处），申请 / 审核共用抬头、编号分区、字段、页脚；两边仍各自保留 `privilege-apply-modal` / `privilege-audit-modal` 类作为钩子，其它 `.privilege-modal` 弹窗不受影响。
2. `src/components/PrivilegeAuditDialog.vue`：抬头换「剪贴板图标 + 标题 + 副标题」；正文拆 `01 申请内容`（工单卡）与 `02 审核意见`（备注 + 「终止必填」标签 + 说明）；加载 / 详情失败 / 无权限 / 校验错误分别用转圈行、红条、暖色提示条；提交中保留 spinner。
3. 工单卡：标题独占一条压深并加分隔线，`dl` 仍两列但 `dt` 固定 56px 让数值对齐；库表清单覆盖全局 `word-break: break-all`，改为按「、」换行（`word-break: normal` + `overflow-wrap: anywhere`），不再把表名从中间劈开。
4. 终止流程由橙实心改红色描边弱底（`.pa-btn-danger`），审核通过沿用绿色主操作；窄面板 ≤540px 时工单卡退单列、页脚跟踪提示隐藏。
5. `src/i18n.ts` 中英各加 6 键（副标题、两段标题、「终止必填」、备注说明、页脚提示）。
6. `scripts/smoke-preview.mjs` 新增 3 项断言，并把「空备注点终止 → 红条且弹窗不关闭」纳入回归。

绿图：

```text
$ pnpm test         Test Files 21 passed (21) / Tests 398 passed (398)
$ pnpm typecheck    exit 0
$ pnpm build        version check ok: 0.2.51 / ui/index.html 1,239.48 kB（gzip 355.65 kB）
```

预览行为：

| 检查 | 结果 |
| --- | --- |
| 分区 | `01 申请内容` + `02 审核意见`（无审核权限时只有第 01 段 + 暖色提示） |
| 工单卡 | 7 行字段，`dt` 固定 56px 对齐；库表清单按「、」换行 |
| 终止流程 | 红色描边弱底，`backgroundColor` = `rgba(220, 53, 69, 0.06)`、文字 `rgb(220, 53, 69)` |
| 备注必填 | 空备注点终止 → 红条「终止流程必须填写审核备注」，弹窗不关闭 |
| 窄面板 | 480px 视口下弹窗 440×651，单列工单卡 + 页脚按钮在视口内 |
| 深色 | 工单卡、红条、终止按钮、textarea 聚焦环均走 `[data-theme="dark"]` |

冒烟新增 3 项全为 true：`privilegeAuditSections` / `privilegeAuditDangerButton` / `privilegeAuditRemarkRequired`；其余 7 项失败与第 24 / 28 / 29 / 31 节相同，`errors: []`。

截图：`preview/smoke-privilege-audit.png`（两段表单 + 工单卡 + 红描边终止）、`preview/smoke-privilege-audit-required.png`（空备注点终止后的红条）。

## 33. 工单日志弹窗对齐（v0.2.52，2026-11-09）

现象（改版前）：与第 31 / 32 节改完的申请、审核弹窗并排时割裂。

| 问题 | 表现 |
| --- | --- |
| 抬头是「工单日志 · 标题」一行 | 标题与工单名混排，长工单名把关闭按钮挤走，没有副标题层级 |
| 表格按数据网格渲染 | 整表等宽字体（中文尤其糊），操作信息被 `max-width: 460px` + 单行省略号截断，读不全 |
| 没有条数概念 | 只有表格行数，页脚是空的，右侧只有一个「关闭」 |

实现：

1. `src/components/WorkflowLogDialog.vue`：抬头换成「时钟图标 + 工单日志 + 副标题『工单：{title}』」（长名 `title` 截断并挂 `title` 提示）；正文三态分开渲染（加载转圈行 / 失败红条 / 空态居中提示）；表格包进卡片，页脚左「共 N 条」右「关闭」。
2. `src/style.css`：新增 `.privilege-log-modal` 块——宽 720px、表格卡片圆角与末行去边框、正文退回无衬线字体、操作列加重、时间列等宽、操作信息 `white-space: normal` 换行读全（覆盖 `.data-table` 的单行省略）。
3. `src/i18n.ts`：中英各加 `privilege.logSubtitle` / `privilege.logRecords` 两键。
4. `scripts/smoke-preview.mjs`：新增 2 项断言（抬头文案为「工单日志」、页脚条数与表格行数一致）。

绿图：

```text
$ pnpm test         Test Files 21 passed (21) / Tests 398 passed (398)
$ pnpm typecheck    exit 0
$ pnpm build        version check ok: 0.2.52 / ui/index.html 1,241.78 kB（gzip 356.16 kB）
```

预览行为：

| 检查 | 结果 |
| --- | --- |
| 抬头 | `工单日志` + `工单：查询数据`（长名一行截断） |
| 表格字体 | `inherit`（无衬线），时间列 `--mono` 11.5px |
| 操作信息 | 允许换行，不再出现省略号截断 |
| 页脚 | `共 2 条` 与表格 2 行一致；≤540px 时让位给关闭按钮 |
| 空 / 加载 / 失败 | 居中提示 / 转圈行 / 红条三种形态 |
| 深色 | 抬头、卡片边框、表头底色均走 `[data-theme="dark"]` |

冒烟新增 2 项全为 true：`privilegeLogHeader` / `privilegeLogFoot`；其余 7 项失败与第 24 / 28 / 29 节相同，`errors: []`。

截图：`preview/smoke-privilege-log.png`（工单日志弹窗：抬头副标题 + 卡片表格 + 页脚条数）。

打包（版本升 `0.2.52`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ pnpm build        version check ok: 0.2.52 / ui/index.html 1,241.78 kB（gzip 356.16 kB）
$ pnpm preview      preview/index.html 已刷新
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.52-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.52
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,424 B   go1.22.12
ui/index.html                         1,250,401 B
```

产物：5,253,856 字节，sha256 `06d0d5de757151c67f7290dedc48df5b0b5f8fe769843a816fac0f24ee5d34ef`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.52，sidecar 仍为 go1.22.12（8,615,424 B）。

## 34. 数据字典页与表元数据弹窗改版（v0.2.53，2026-11-09）

现象（改版前）：字典页是「两个下拉 + 一排字母胶囊 + 等宽两列表」，表一多就只剩一片同色行；弹窗是「标题 + 四个裸 h4 + data 表格」，与列表页并排时割裂。

| 问题 | 表现 |
| --- | --- |
| 分组头不吸顶 | 滚过几十行后不知道停在哪个字母段，只能靠顶部索引条猜 |
| 过滤没有反馈 | 命中哪一段、命中在表名还是注释，都看不出来 |
| 行没有层次 | 表名与注释同色同字号；注释为空时留一个空白格 |
| 加载只有一枚转圈 | 首屏切库时整块空白，没有目录的形状 |
| 弹窗层级弱 | 表名与标题挤一行；区块头是裸 h4；单元格带 `data-table` 的 `cursor: copy` 误导可复制；DDL 只能整段手选 |

实现：

1. `src/views/DictView.vue`：工具条按「实例 / 数据库」分组加竖线，右侧表数胶囊（过滤时切强调色）+ 内嵌图标搜索（`/` 聚焦、Esc 清空、清空按钮）+ 刷新；分组头改吸顶字母牌 + 计数 + 渐隐细线；表名 13px 等宽、注释 12px 次要色，命中片段就地 `<mark>` 高亮；加载改骨架屏（`role=status` + `aria-label`），空态分「未选实例 / 无可用库 / 未选库 / 无表或无匹配」四类。
2. `src/components/TableMetaDialog.vue`：抬头改「表图标 + 标题 + 库路径」；四个区块共用「竖条 + 渐隐线 + 计数」；DDL 加复制按钮；`role="dialog"` + 打开即聚焦 + Esc 关闭；索引为空、DDL 为空分别出虚线空态。
3. `src/style.css`：重写「数据字典」「表元数据弹窗」两段；工具条始终可换行（宿主面板拖到 1000px 不裁切），≤900px 表名列让位给注释；`prefers-reduced-motion` 覆盖骨架与过渡。
4. `src/lib/dict.ts` + `dict.test.ts`：新增 `splitMatch` 纯函数（关键词分段高亮）与 5 条用例。
5. `src/i18n.ts`：中英各加 16 键（选库提示、无库说明、加载、无匹配建议、清空搜索、`/` 提示、索引名、区块计数、复制 DDL、空索引 / 空 DDL）。
6. `scripts/smoke-preview.mjs`：`metaSections` 由 `.meta-block h4` 改 `.meta-block-title`，新增 9 项断言（区块计数、弹窗焦点、Esc、分组头吸顶、aria 三件套、`/` 聚焦与输入、1000px 工具条不溢出）。

绿图：

```text
$ pnpm test         Test Files 21 passed (21) / Tests 403 passed (403)
$ pnpm typecheck    exit 0
$ pnpm build        version check ok: 0.2.53 / ui/index.html 1,257.02 kB（gzip 359.57 kB）
```

预览行为（`preview/dict-01…12-*.png`、`preview/meta-empty.png`、`preview/dict-en-0*.png`）：

| 检查 | 结果 |
| --- | --- |
| 字母索引 | `A │1 C │2 D │5 …`，选中态实底，滚动时同步高亮 |
| 分组头 | 吸顶；跳转到 r 段时只有一枚贴在列表顶部 |
| 检索 | 「客户」4 / 31 张表，索引同步收窄为 3 组，注释命中片段带底色 |
| 点击区域 | 表名可点（打开弹窗），注释与空白不响应 |
| 窄宽度 | 1000px 工具条换行 85px 不溢出；800px 表名列 200px / 注释 288px |
| 英文 | `31 tables` / `Search table or comment` / `13 columns` 均不溢出，空态两行断行正常 |
| 深色 | 行、索引、弹窗、骨架与空态均走 `[data-theme="dark"]` |
| 可访问性 | 索引 `aria-label`、当前字母 `aria-current`、计数 `aria-live`、弹窗 `role=dialog` 且打开即聚焦、Esc 可关 |

冒烟新增 9 项全为 true；全站 319 项，失败仍为 7 项既有问题（`dashboardTrend*`×2、`dashboardRange*`×3、`sidebarChildren`、`workflowCheckEmptyCompact`）。基线核对：把本次改动 stash 后在 HEAD 上重跑为 310 项 / 同样 7 项失败，故与本次改动无关。

打包（版本升 `0.2.53`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ pnpm build        version check ok: 0.2.53 / ui/index.html 1,257.02 kB（gzip 359.57 kB）
$ pnpm preview      preview/index.html 已刷新
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.53-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.53
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,424 B   go1.22.12
ui/index.html                         1,265,993 B
```

产物：5,257,269 字节，sha256 `45c7cb20efb8f92579308debeedfd62ce18200f55079b5daf3f78173f924d360`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.53，sidecar 仍为 go1.22.12（8,615,424 B，`go version -m` 核对）。

## 35. 在线查询导出 Excel（v0.2.54，2026-11-09）

需求：结果面板只有 CSV/TSV/Markdown 复制，导出 Excel 得先粘到别处再转存。本版在结果工具栏加「导出 Excel」，直接生成 .xlsx 并由 sidecar 落盘。

决策：真 .xlsx（零依赖手写 OOXML + store ZIP）/ 导出当前结果页签全部行（忽略前端分页）/ 不压缩，超 5MB 提示调小行数上限（sidecar 单条 JSON 消息 8MB，base64 膨胀 4/3）。

实现：

1. `src/lib/xlsx.ts`（新增）：六个 OOXML 部件 + store（无压缩）ZIP + CRC32；表头加粗（`s="1"`）、首行冻结、自动筛选；NULL 不写单元格、布尔 `t="b"`、`|整数| ≥ 1e15`、`NaN`、`Infinity` 转文本避免科学计数；文本转义 `& < > "`、剔除 XML 非法控制字符、单格按 Excel 上限 32767 截断；工作表名清洗非法字符并截断到 31 字符；超过 `maxBytes`（默认 5MB）抛 `XlsxTooLargeError`。
2. `src/lib/download.ts`：`toBase64`（手写、带 padding）与 `saveBinaryFile`（sidecar 桥 → 取消 → invoke 不可用时 Blob 回退），依赖可注入，单测不 mock 模块。
3. `src/api.ts`：`saveTextFile` 增加 `contentBase64?`；`backend/internal/rpc/service.go`：新增 `ContentBase64` 字段，非空时 base64 解码优先于 `Content`，非法 base64 返回 `-32602`。
4. `src/components/ResultGrid.vue` + `src/App.vue` + `src/i18n.ts`：工具栏「导出 Excel」，导出中禁用；成功 / 取消 / 超限 / 失败分别 toast；中英各 5 键。
5. 测试：`src/lib/xlsx.test.ts`（12 条）、`src/lib/download.test.ts`（4 条）、`backend/internal/rpc/download_test.go` 新增 `TestSaveTextFileBase64`。

红图（实现前，骨架抛占位错误 / 返回空串）：

```text
$ pnpm exec vitest run src/lib/xlsx.test.ts src/lib/download.test.ts
Test Files  2 failed (2)
     Tests  15 failed (15)

$ go test ./internal/rpc -run "TestSaveTextFile" -v
--- FAIL: TestSaveTextFileBase64 (0.01s)
    download_test.go:129: unexpected binary content: []
--- PASS: TestSaveTextFile / TestSaveTextFileAskPath（既有回归护栏，不参与判红）
```

绿图：

```text
$ pnpm test                     Test Files 23 passed (23) / Tests 419 passed (419)
$ pnpm exec vue-tsc --noEmit    exit 0
$ pnpm build                    version check ok: 0.2.54 / ui/index.html 1,265.00 kB（gzip 362.20 kB）
$ go vet ./...                  （无输出）
$ go test ./...                 archery ok / rpc ok
```

端到端（免安装预览，Edge 无头点「导出 Excel」）：

| 检查 | 结果 |
| --- | --- |
| 桥调用 | 捕获 1 次 `archery/saveTextFile`：`content=""`，`contentBase64` 解码 51,168 字节、以 `PK` 开头 |
| toast | `已导出：C:\Users\demo\Downloads\query-result-20261109-160550.xlsx` |
| 页面 | 无 `pageerror` / `console.error` |
| 真实解析器 | Python `zipfile` 打开通过，6 个 XML 部件全部可解析；`openpyxl` 读到 `sheet=result`、`A1:G138`，表头 7 列与首行数据正确 |

期间修掉一个真实缺陷：EOCD 的中央目录大小把 EOCD 自身 12 字节也算了进去（`writeU32/writeU16` 自己会推进 `at`），Python/Excel 报 `BadZipFile`。单测的 zip 解析器只走本地文件头、覆盖不到中央目录，随后补 1 条 EOCD 断言（回退实现时红：`expected 3753 to be 3741`）。

打包（版本升 `0.2.54`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.54-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.54
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,424 B   go1.22.12
ui/index.html                         1,274,052 B
```

产物：5,259,328 字节，sha256 `f0b3a993a32e02720f7e429ef265d0013035344d4fe106bd78b3b26ef746c0b8`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.54，sidecar 为 go1.22.12（8,615,424 B，`go version -m` 核对）。

## 36. 导出 Excel 可选择保存位置（v0.2.55，2026-11-09）

现象（改版前）：导出直接把文件写进下载目录，用户无法指定位置；`saveTextFile` 虽然支持 `askPath`，但 Windows 对话框的过滤器硬编码为 `SQL (*.sql)`，拿 Excel 文件名弹窗仍按 SQL 过滤。

决策：每次导出弹系统「另存为」；记住上次保存目录（随 `uiState` 持久化）；本轮只改 Excel 导出，回滚 SQL / 权限 CSV 维持现状。

实现：

1. `backend/internal/rpc/savefile.go`（新增）：`saveDialogFilter(name)` 按扩展名给过滤器与默认扩展名（`.xlsx` → `Excel 工作簿 (*.xlsx)`、`.sql` → `SQL (*.sql)`、其他 → `所有文件 (*.*)`）；`resolveSaveDir(requested, fallback)` 校验初始目录，不存在或不是目录时回退下载目录。
2. `backend/internal/rpc/savefile_windows.go`：对话框使用动态过滤器并设置 `LPSTRDefExt`；`backend/internal/rpc/service.go`：新增 `Request.InitialDir`，`askPath` 分支先 `resolveSaveDir` 再弹窗。
3. `src/lib/download.ts`：`saveBinaryFile` 增加 `askPath` / `initialDir`（仅在传入时写入参数对象，既有调用形状不变，老断言继续通过）；新增 `parentDir(path)` 从落盘路径取目录。
4. `src/components/ResultGrid.vue`：Excel 导出固定 `askPath: true` + `initialDir`（上次目录）；成功后 `emit("saveDir", 目录)`；按钮 title 提示「可选择保存位置」。
5. `src/types.ts` + `src/App.vue`：`UiState.exportDir` 恢复与落盘，接 `exportDir` prop / `saveDir` 事件；取消导出保持静默（与回滚 SQL 一致）。
6. `src/i18n.ts`：中英各 1 键（按钮提示）。测试：`download.test.ts` 新增 2 条、`download_test.go` 新增 `TestSaveDialogFilter` 与 `TestSaveTextFileAskPathInitialDir`。

红图（骨架返回空值）：

```text
$ pnpm exec vitest run src/lib/download.test.ts
Test Files  1 failed (1)
     Tests  2 failed | 4 passed (6)      # 新增两条红，既有 4 条护栏绿
  AssertionError: expected '' to be 'C:\Users\demo\Downloads'

$ go test ./internal/rpc -run "^(TestSaveDialogFilter|TestSaveTextFileAskPathInitialDir)$" -v
--- FAIL: TestSaveDialogFilter (0.00s)
    query-result-...xlsx: unexpected filter {Filter: DefExt:}
--- FAIL: TestSaveTextFileAskPathInitialDir (0.01s)
    expected initial dir "...\002", got "...\001"（实际回退到下载目录）
```

绿图：

```text
$ pnpm test                     Test Files 23 passed (23) / Tests 421 passed (421)
$ pnpm exec vue-tsc --noEmit    exit 0
$ pnpm build                    version check ok: 0.2.55 / ui/index.html 1,265.69 kB（gzip 362.52 kB）
$ go vet ./...                  （无输出）
$ go test ./...                 archery ok / rpc ok
```

端到端（免安装预览，连续点两次「导出 Excel」）：

| 检查 | 结果 |
| --- | --- |
| 按钮 title | `导出当前结果全部行，并选择保存位置` |
| 第 1 次调用 | `askPath=true`，无 `initialDir`（首次从下载目录开始） |
| 成功后持久化 | `archery/uiState/set` 的 `state.exportDir = C:\Users\demo\Documents\rollback` |
| 第 2 次调用 | `askPath=true`，`initialDir=C:\Users\demo\Documents\rollback` |
| 页面 | 无 `pageerror` / `console.error` |

打包（版本升 `0.2.55`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.55-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.55
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,615,936 B   go1.22.12
ui/index.html                         1,274,778 B
```

产物：5,260,271 字节，sha256 `a16ae2a45fc2e63354b90ee78e3c5e51d82113c0517116daee5c4ab80f02968c`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.55，sidecar 为 go1.22.12（8,615,936 B，`go version -m` 核对）。

## 0.2.56：结果复制合并成下拉 + 跳转列

版本：`package.json` / `manifest.json` / `backend/main.go` / README 中英同步升至 `0.2.56`。

改动：

- 结果工具栏的「复制 CSV / 复制 TSV / 复制 Markdown」三个按钮合并为一个「复制 ▾」下拉；选中后复制并 toast，点外部或 Esc 收起。
- 新增「跳转列」浮层：按列名 / 列类型搜索，支持模糊 / 前缀 / 精确三种匹配（纯函数在 `src/lib/column-jump.ts`），列表项显示列名、类型与 `#序号`；↑↓ 选择、Enter 跳转、Esc 关闭。
- 跳转定位读表头实际 `offsetLeft` 再设 `.grid-wrap` 的 `scrollLeft`（列宽默认自适应，`widths` 只在手动拖拽后有值，不能累加），目标表头高亮 1.6s 后自动清除。
- 面板 Teleport 到 body + fixed 定位：`.panel-body` 是 `overflow: hidden`，挂在工具栏里做 absolute 会被裁；下方空间不足时向上展开，`max-height` 按可用空间收敛。
- 顺手修掉 `result.emptyHint` 里残留的「结果单元格可点击复制」（中英各一处）。

绿图：

```text
$ pnpm test                     Test Files 25 passed (25) / Tests 447 passed (447)
$ pnpm typecheck                exit 0
$ pnpm build                    version check ok: 0.2.56 / ui/index.html 1,315.85 kB（gzip 377.64 kB）
```

端到端（免安装预览，1440×900 与 900×900 两档视口）：

| 检查 | 结果 |
| --- | --- |
| 工具栏 | `导出 Excel 复制 ▾ 跳转列` |
| 复制下拉 | `复制 CSV / 复制 TSV / 复制 Markdown`，点外部关闭 |
| 模糊 `id` | `id #1`、`customer_id #3` |
| 精确 `id` | 只 `id #1` |
| 前缀 `re` | 只 `remark #7` |
| 跳转定位 | 窄视口 `scrollLeft 167 = 最大可滚动量`，`remark` 表头高亮 |
| 高亮回收 / Esc | 1.6s 后清除；Esc 关闭面板 |
| 页面 | 无 `pageerror` / `console.error` |

打包（版本升 `0.2.56`）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.56-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.56
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,662,016 B   go1.22.12
ui/index.html                         1,325,932 B
```

产物：5,297,331 字节，sha256 `f3c75d00ae7891a74522e5b8deb7d2ddd720291fdeef71064d2ad50e07876d89`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.56，`ui/index.html` 与本地 `pnpm build` 产物 sha256 一致（`f02c05445bbcf3979136e1ac4fcbe66c720180e976f4d94925d736bd403e95ac`），sidecar 为 go1.22.12（8,662,016 B，`go version -m` 核对）。

## 0.2.57：跳转列改造为列显隐配置

版本：`package.json` / `manifest.json` / `backend/main.go` / README 中英同步升至 `0.2.57`。

改动：

- 「跳转列」浮层改为「列配置」：列表项带勾选框，点整行切换列的显示 / 隐藏；列名、类型与原始 `#N` 保留，隐藏项列名加删除线。
- 表头与数据行按同一套可见下标渲染（`visibleIndexes`），隐藏列不再进 DOM；列宽 `widths` 仍按原始列下标存，隐藏后再显示宽度不丢。
- 底部「已隐藏 N 列」+「全部显示」（无隐藏时禁用）；只剩一列可见时该项锁定（`aria-disabled` + 勾选框变淡 + title「至少保留一列」）。
- 纯函数抽到 `src/lib/column-picker.ts`：`allVisible` / `toggleVisible` / `visibleIndexes` / `hiddenCount` / `pickColumns`，`matchColumns` 从原 `column-jump.ts` 迁移复用。
- CSV / TSV / Markdown / Excel 导出改为按可见列（所见即所得），`exportTooLarge` 的行列数也按可见列统计。
- 换结果 / 重新执行时恢复全部显示；移除跳转定位、`scrollLeft` 与表头高亮逻辑。
- 删除 `ColumnJumpPanel.vue` / `column-jump.ts` / `column-jump.test.ts`。

绿图：

```text
$ pnpm test                     Test Files 25 passed (25) / Tests 455 passed (455)
$ pnpm typecheck                exit 0
$ pnpm build                    version check ok: 0.2.57 / ui/index.html 1,318.16 kB（gzip 378.16 kB）
```

端到端（免安装预览，1440×900）：

| 检查 | 结果 |
| --- | --- |
| 工具栏 | `导出 Excel 复制 ▾ 列配置` |
| 面板列表 | `id bigint #1` … `remark varchar(255) #7` |
| 取消 `order_no` | 表头 7→6 列、首行 `td` 同步减少，项带 `is-off` |
| 底部计数 | `已隐藏 1 列 全部显示` |
| 复制 CSV 表头 | `id,customer_id,amount,status,created_at,remark`（只含可见列） |
| 隐藏到最后一列 | 表头只剩 `# remark`、`已隐藏 6 列`、锁定项 1 个且 title=`至少保留一列` |
| 全部显示 | 恢复 7 列，按钮转禁用 |
| 搜索 `id` | `id #1`、`customer_id #3` |
| Esc | 面板关闭 |
| 页面 | 无 `pageerror` / `console.error` |

打包（版本升 `0.2.57`）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.57-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.57
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,662,016 B   go1.22.12
ui/index.html                         1,328,252 B
```

产物：5,297,871 字节，sha256 `35c588bd939ce3869b6dc0af5759c7bbbeffbe265a0e276aa9a3f9f63baec66d`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.57，`ui/index.html` 与本地 `pnpm build` 产物 sha256 一致（`cd67701cd574524c0d9099c260d4fbfe07197b76cb5c30493cd6a450796c6fe7`），sidecar 为 go1.22.12（8,662,016 B，`go version -m` 核对）。

## 0.2.58：结果网格支持搜索过滤与高亮

版本：`package.json` / `manifest.json` / `backend/main.go` / README 中英同步升至 `0.2.58`。

改动：

- 结果工具栏新增搜索框：输入即按可见列做大小写不敏感的子串过滤，命中片段在单元格内高亮，并显示「匹配 N / 总数 M 行」。
- 分页基于过滤后的结果集，`#` 保留原始行号；无命中时显示空态并禁用复制 / 导出。
- `Ctrl+F` 聚焦搜索框（焦点在编辑器或输入控件内时让给原控件），`Esc` 先清空再失焦。
- CSV / TSV / Markdown 复制与 Excel 导出在有搜索时只带匹配行。
- 纯函数 `matchRowIndexes` / `highlightSegments` 落在 `src/lib/result.ts`，配 5 个单测。
- `scripts/smoke-preview.mjs` 新增搜索过滤、计数、高亮、原始行号、分页文案、空态、导出禁用、Esc 与 Ctrl+F 共 10 项检查。

绿图：

```text
$ pnpm test                     Test Files 25 passed (25) / Tests 460 passed (460)
$ pnpm typecheck                exit 0
$ pnpm build                    version check ok: 0.2.58 / ui/index.html 1,323.74 kB（gzip 379.75 kB）
$ (backend) go test ./...       ok archery 0.391s / ok rpc 1.037s
```

端到端（免安装预览，1440×900）：

| 检查 | 结果 |
| --- | --- |
| 搜索 `paid` | 30/30 行均含 paid，计数 `匹配 34 / 137 行` |
| 高亮 | `.result-match` 数量 > 0 |
| 原始行号 | 首行 `#4`（原始第 4 行） |
| 分页文案 | `显示第 1 到第 30 条匹配记录，总共 34 条` |
| 无命中 | 空态行出现，导出按钮禁用 |
| Esc | 清空搜索并恢复 30 行 |
| Ctrl+F | 焦点落到 `.result-search` |
| 页面 | 无 `pageerror` / `console.error` |

打包（版本升 `0.2.58`）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.58-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.58
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,662,016 B   go1.22.12
ui/index.html                         1,333,971 B
```

产物：5,299,443 字节，sha256 `999c4f5d55dce2fcb9bf99cdacc2d746d053512b3b04d70dbcd6a4e3a42b1ec0`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.58，`ui/index.html` 与本地 `pnpm build` 产物 sha256 一致（`7311dd2ffe43619f0775b59b69c395b82328c024aea352bc02c44aa7bba8a52a`），sidecar 为 go1.22.12（8,662,016 B，`go version -m` 核对）。

## 0.2.59：在线查询页签撤销历史隔离（2026-11-10）

问题：所有编辑器页签共用同一个 CodeMirror 实例，切页签靠 `view.dispatch` 整篇替换文档，这次替换被记进了撤销历史——在新页签里按 `Ctrl+Z` 会把上一个页签的 SQL 撤进当前页签（Tab 内容缓存串台）；光标与选区也继承上一个页签的位置。

红图（`scripts/smoke-preview.mjs` 新增 4 项断言，修复前）：

```text
tabUndoIsolated     false   tab2 连按 3 次 Ctrl+Z：空 → select 1 -- TABONE（tab1 的内容）→ 空
tabUndoKeepsOwnSql  true
tabSwitchKeepsSql   true
tabClosedBack       true
```

实现：

1. `src/components/SqlEditor.vue`：新增可选 `tabId` prop；扩展组装抽成 `createState(doc)`；按页签用 `Map<string, EditorState>` 保存编辑器状态（`MAX_STATES = 20`，保存时先删后插、超限丢最早一份）；切页签改 `view.setState(...)` 整份换状态，不再整篇 dispatch；同一页签内的外部改动（常用查询 / 载入历史 / 美化 / 清空）仍走 dispatch，保留撤销历史。`tabId` 缺省时（SQL 上线提交页）行为不变。
2. `src/App.vue`：`<SqlEditor :tab-id="active.id">`。
3. `scripts/smoke-preview.mjs`：新增 `tabUndoIsolated` / `tabUndoKeepsOwnSql` / `tabSwitchKeepsSql` / `tabClosedBack` 四项断言。

绿图：

```text
$ pnpm typecheck               exit 0
$ pnpm test                    Test Files 25 passed (25) / Tests 460 passed (460)
$ pnpm build                   version check ok: 0.2.59 / ui/index.html 1,324.14 kB（gzip 379.93 kB）
$ (backend) go vet ./...       无输出
$ (backend) go test ./...      ok archery 0.355s / ok rpc 0.430s
$ node scripts/smoke-preview.mjs
    tabUndoIsolated / tabUndoKeepsOwnSql / tabSwitchKeepsSql / tabClosedBack = true
    errors: []
```

端到端（免安装预览，1440×900）：

| 检查 | 结果 |
| --- | --- |
| 切页签后正文 | tab1 / tab2 各自保持（`select 1 -- TABONE` / `select 2 -- TABTWO`） |
| tab2 连撤 3 次 | 只撤掉自己的输入，不再出现 tab1 的 SQL |
| tab1 连撤 2 次 | 只作用于 tab1 自身历史 |
| 常用查询带入 / 美化 / 清空 | 编辑器即时回显 |
| 内层页签切换（结果 ↔ 消息） | 编辑器内容不重置 |
| 提交页预填（无 `tabId`） | 原工单 SQL 正常带入 |
| 页面 | 无 `pageerror` / `console.error` |

同一次冒烟另有 7 项既有失败（dashboard 日期漂移 ×5、`sidebarChildren`、`workflowCheckEmptyCompact`），与本轮改动无关。

打包（版本升 `0.2.59`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.59-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.59
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,662,016 B   go1.22.12
ui/index.html                         1,334,366 B
```

产物：5,299,614 字节，sha256 `e0f1ac5b0117cc85a22024e16a69fd1cdac190cf9dfe6b6a5016162d8e4c2aed`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `manifest.json` version=0.2.59，`ui/index.html` 与本地 `pnpm build` 产物 sha256 一致（`59e411628cf1658918a3494c0cdc64ff23d1d3581e477189e9d66b94ba5d0e8c`）。

## 0.2.60：界面状态按 Archery 环境隔离（2026-11-10）

问题（实测两台环境）：同时配了「Archery(广州跨境&其他测试环境)」与「Archery(巴西)」两条连接，打开巴西连接的「在线查询」，页签仍是广州连接留下的 `test-express-rw/schedule-dev`，编辑器空着——顶部公告已经是巴西环境的，页签却来自另一个环境。原因：`ui-state.json` 只有一份全局状态，`archery/uiState/get|set` 完全不看 `connectionId`，谁后保存谁覆盖，两个环境互相污染。

红图（Go，实现前）：

```text
$ go test ./internal/rpc -run TestUIState -count=1
--- FAIL: TestUIStateScopedByEnvironment
    service_test.go:444: brazil connection restored guangzhou tabs: "select 1"
--- FAIL: TestUIStateRequiresEnvironment
    service_test.go:467: expected not_connected for unscoped set, got <nil>
--- FAIL: TestUIStateAdoptsLegacyGlobalFile
    service_test.go:504: legacy state leaked into another environment: "select 9"
```

前端红图（`src/lib/state.ts` 只留签名、抛 not implemented）：

```text
$ pnpm exec vitest run src/lib/state.test.ts
❯ src/lib/state.test.ts (4 tests | 4 failed)
  → not implemented
```

实现：

1. `backend/internal/rpc/service.go`：`Service` 新增 `scopes`（connectionId → baseURL），`resolveClient` 建立会话时同步记录；新增 `stateScope`（优先用请求里的连接配置，其次该连接的会话地址，最后 `scopes`）。`ui-state.json` 升到 v2 `{version:2, states:{<baseURL>:{baseURL,savedAt,state}}}`；`getUIState` 没有环境归属时返回 nil，`setUIState` 没有归属时返回 `not_connected`（宁可空页签，也不端出别的环境的数据）；v1 的整份全局状态由第一个连上来的环境认领一次，其余环境从空白开始；`pruneUIState` 只保留最近 20 个环境。
2. `src/lib/state.ts`（新增）+ `src/lib/state.test.ts`：`pruneTabTargets` 恢复页签时清掉当前环境不存在的实例目标（只清实例与库、保留 SQL）；实例清单为空时一律不动，避免误清。
3. `src/App.vue`：`restoreState` 恢复后按当前实例清单裁剪，发生裁剪时 toast 提示数量；`src/i18n.ts` 中英各 1 键 `toast.foreignTarget`。
4. `README.md`（中英）：数据目录说明补「按 Archery 环境分组」。

绿图：

```text
$ (backend) go vet ./...        无输出
$ (backend) go test ./...       ok archery 0.294s / ok rpc 0.364s
$ pnpm typecheck                exit 0
$ pnpm test                     Test Files 26 passed (26) / Tests 464 passed (464)
$ pnpm build                    version check ok: 0.2.60
$ node scripts/smoke-preview.mjs  errors: []（另 7 项既有失败与上轮相同，与本轮无关）
```

场景核对（Go 用例覆盖前三条，端到端待实测）：

| 场景 | 预期 |
| --- | --- |
| 广州连接存页签 → 打开巴西连接 | 巴西空白页签，不再出现 `test-express-rw/schedule-dev` |
| 两条连接各存一份页签 | 互不覆盖，各自恢复自己的实例 / 库 / SQL |
| 升级后首次打开 | 老 `ui-state.json` 被首个打开的环境认领，另一个环境空白 |
| 恢复出的实例已下线 / 不属于本环境 | 目标清空 + toast「已清除 N 个不属于当前环境的查询目标」，SQL 保留 |

打包（版本升 `0.2.60`，`package.json` / `manifest.json` / `backend/main.go` / README 中英同步）：

```text
$ npx --yes @dbx-app/plugin-cli@0.1.9 package .
Building: Go backend
Success: Built dist\io.github.nixing178.archery-0.2.60-windows-x64.dbxp
Signature: unsigned review candidate

checksums.json                             526 B
manifest.json                           10,419 B   version=0.2.60
assets/connection.svg                      415 B
assets/plugin.svg                          571 B
bin/windows-x64/dbx-plugin-archery.exe 8,673,280 B   go1.22.12
ui/index.html                         1,334,883 B
```

产物：5,305,564 字节，sha256 `2d9e3890a9b312e1f8ff30256e6082ef3f17133b57b80baa6370272bca0cb040`；`*.artifact.json` 的 size / sha256 与产物一致；包内 `ui/index.html` 与本地 `pnpm build` 产物 sha256 一致（`bfdc41eba0624559e4357e64628566791842fcbf59d2222ecf20a73fdcb72ba4`）。
