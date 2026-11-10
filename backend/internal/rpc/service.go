package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nixing178/dbx-plugin-archery/internal/archery"
)

const (
	pluginID    = "io.github.nixing178.archery"
	maxStateRaw = 512 << 10
)

// Error 是 RPC 层的统一错误，main 会把它转换成 SDK 的 PluginError。
// Message 以 "<kind>: <detail>" 形式返回，UI 可据此做本地化。
type Error struct {
	Code    int
	Kind    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Request 是 DBX 生命周期请求与插件方法参数的合集。
type Request struct {
	ConnectionID string `json:"connectionId"`
	Connection   struct {
		ID                string         `json:"id"`
		Name              string         `json:"name"`
		Host              string         `json:"host"`
		Port              int            `json:"port"`
		Username          string         `json:"username"`
		Password          string         `json:"password"`
		APIToken          string         `json:"api_token"`
		ReadOnly          bool           `json:"read_only"`
		ExternalConfig    map[string]any `json:"external_config"`
		Secrets           map[string]any `json:"secrets"`
		ConnectionSecrets map[string]any `json:"connection_secrets"`
	} `json:"connection"`
	Runtime struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"runtime"`
	Secrets map[string]any `json:"secrets"`

	OTP      string `json:"otp"`
	OTPType  string `json:"otpType"`
	OTPPhone string `json:"otpPhone"`

	InstanceName string `json:"instanceName"`
	InstanceID   int    `json:"instanceId"`
	ResourceType string `json:"resourceType"`
	DBName       string `json:"dbName"`
	DBType       string `json:"dbType"`
	SchemaName   string `json:"schemaName"`
	TBName       string `json:"tbName"`
	SQLContent   string `json:"sqlContent"`
	LimitNum     int    `json:"limitNum"`

	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	Search     string `json:"search"`
	Star       bool   `json:"star"`
	QueryLogID int64  `json:"queryLogId"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	Alias      string `json:"alias"`

	// AuditID 是待办列表的审批记录号，与业务工单号 WorkflowID 不是一回事。
	ApplyID      int64    `json:"applyId"`
	AuditStatus  int      `json:"auditStatus"`
	AuditRemark  string   `json:"auditRemark"`
	AuditID      int64    `json:"auditId"`
	WorkflowID   int64    `json:"workflowId"`
	WorkflowType int      `json:"workflowType"`
	Title        string   `json:"title"`
	GroupName    string   `json:"groupName"`
	PrivType     int      `json:"privType"`
	DBList       []string `json:"dbList"`
	TableList    []string `json:"tableList"`
	ValidDate    string   `json:"validDate"`

	// SQL 上线工单：列表筛选、审核动作、执行方式与提交表单。
	Status       string   `json:"status"`
	GroupID      int      `json:"groupId"`
	SyntaxTypes  []string `json:"syntaxTypes"`
	Audit        bool     `json:"audit"`
	Action       string   `json:"action"`
	Mode         string   `json:"mode"`
	RunDate      string   `json:"runDate"`
	RunDateStart string   `json:"runDateStart"`
	RunDateEnd   string   `json:"runDateEnd"`
	Backup       bool     `json:"backup"`
	DemandURL    string   `json:"demandUrl"`
	// SQL 上线工单沿用更直观的字段名；与 SQLContent / AuditRemark 互为别名。
	SQLText string `json:"sql"`
	Remark  string `json:"remark"`

	// 账号设置：两步验证与修改密码。
	AuthType    string `json:"authType"`
	SecretKey   string `json:"secretKey"`
	Phone       string `json:"phone"`
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`

	// 文件落盘（下载回滚SQL / 导出 Excel）：UI 沙箱拦截 <a download>，由 sidecar 写下载目录。
	// ContentBase64 用于二进制内容，非空时优先于 Content；
	// InitialDir 是「另存为」的初始目录（UI 记住的上次保存位置）。
	Filename      string `json:"filename"`
	Content       string `json:"content"`
	ContentBase64 string `json:"contentBase64"`
	InitialDir    string `json:"initialDir"`
	AskPath       bool   `json:"askPath"`

	State json.RawMessage `json:"state"`
}

// Service 管理每个连接对应的 Archery 客户端。
type Service struct {
	mu       sync.Mutex
	sessions map[string]*archery.Client
	// scopes 记录 connectionId → Archery baseURL：UI 状态按环境分组，
	// 会话断开后仍能用它定位上一个环境的状态。
	scopes    map[string]string
	stateMu   sync.Mutex
	storeOnce sync.Once
	store     *archery.SessionStore
}

// NewService 创建 RPC 服务。
func NewService() *Service {
	return &Service{sessions: map[string]*archery.Client{}, scopes: map[string]string{}}
}

// writeMethods 是只读模式下必须拦截的写操作。
// SQL 上线工单会真正改库，提交、审核、执行、定时全部拦截；
// 查询权限工单只写平台工单、不动数据库，因此不在名单内。
var writeMethods = map[string]struct{}{
	"archery/workflowSubmit":            {},
	"archery/workflowAudit":             {},
	"archery/workflowExecute":           {},
	"archery/workflowCancel":            {},
	"archery/workflowSchedule":          {},
	"archery/sqlWorkflowSubmit":         {},
	"archery/sqlWorkflowRollbackSubmit": {},
	"archery/sqlWorkflowAudit":          {},
	"archery/sqlWorkflowExecute":        {},
	"archery/sqlWorkflowSchedule":       {},
	"archery/sqlWorkflowAlterRunDate":   {},
	"archery/sqlWorkflowCancel":         {},

	// 账号安全设置会改 Archery 侧配置（含发送短信），只读连接一律拦截。
	"archery/user/twoFactorStart":   {},
	"archery/user/twoFactorConfirm": {},
	"archery/user/twoFactorDisable": {},
	"archery/user/changePassword":   {},
}

// Handle 分发一个 RPC 方法。
func (s *Service) Handle(ctx context.Context, method string, raw json.RawMessage) (any, *Error) {
	var request Request
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, invalidParams("invalid request parameters")
		}
	}
	// DBX 生命周期回调（connection/connect）只下发 connection.id，
	// 工作台的 invoke 才带顶层 connectionId；这里统一成同一个键。
	if strings.TrimSpace(request.ConnectionID) == "" {
		request.ConnectionID = strings.TrimSpace(request.Connection.ID)
	}

	// 只读守卫必须先于分发：已实现的写方法会直接命中下面 case，不能再依赖兜底分支。
	if _, isWrite := writeMethods[method]; isWrite {
		client, err := s.session(request.ConnectionID)
		if err != nil {
			return nil, err
		}
		if client.ReadOnly() {
			return nil, &Error{Code: -32000, Kind: "read_only", Message: "read_only: 当前连接处于只读模式，已阻止写操作"}
		}
	}

	switch method {
	case "connection/test":
		return s.testConnection(ctx, request)
	case "connection/connect":
		return s.connect(ctx, request)
	case "connection/disconnect":
		return s.disconnect(request)
	case "archery/ping":
		return map[string]any{
			"ok":           true,
			"plugin":       pluginID,
			"language":     "go",
			"connectionId": request.ConnectionID,
		}, nil
	case "archery/sessionKeepAlive":
		return s.sessionKeepAlive(ctx, request)
	case "archery/overview":
		return s.overview(ctx, request)
	case "archery/instances":
		return s.instances(ctx, request)
	case "archery/resources":
		return s.resources(ctx, request)
	case "archery/dictTables":
		return s.dictTables(ctx, request)
	case "archery/tableMeta":
		return s.tableMeta(ctx, request)
	case "archery/describe":
		return s.describe(ctx, request)
	case "archery/query":
		return s.query(ctx, request)
	case "archery/queryLogs":
		return s.queryLogs(ctx, request)
	case "archery/favorite":
		return s.favorite(ctx, request)
	case "archery/dashboard":
		return s.dashboard(ctx, request)
	case "archery/announcement":
		return s.announcement(ctx, request)
	case "archery/dashboardTrends":
		return s.dashboardTrends(ctx, request)
	case "archery/privileges":
		return s.privileges(ctx, request)
	case "archery/myPrivileges":
		return s.myPrivileges(ctx, request)
	case "archery/privilegeOptions":
		return s.privilegeOptions(ctx, request)
	case "archery/privilegeInstances":
		return s.privilegeInstances(ctx, request)
	case "archery/privilegeApply":
		return s.privilegeApply(ctx, request)
	case "archery/privilegeDetail":
		return s.privilegeDetail(ctx, request)
	case "archery/privilegeAudit":
		return s.privilegeAudit(ctx, request)
	case "archery/privilegeAuditors":
		return s.privilegeAuditors(ctx, request)
	case "archery/workflowLog":
		return s.workflowLog(ctx, request)
	case "archery/sqlWorkflows":
		return s.sqlWorkflows(ctx, request)
	case "archery/sqlWorkflowOptions":
		return s.sqlWorkflowOptions(ctx, request)
	case "archery/sqlWorkflowDetail":
		return s.sqlWorkflowDetail(ctx, request)
	case "archery/sqlWorkflowAudit":
		return s.sqlWorkflowAudit(ctx, request)
	case "archery/sqlWorkflowExecute":
		return s.sqlWorkflowExecute(ctx, request)
	case "archery/sqlWorkflowSchedule":
		return s.sqlWorkflowSchedule(ctx, request)
	case "archery/sqlWorkflowAlterRunDate":
		return s.sqlWorkflowAlterRunDate(ctx, request)
	case "archery/sqlWorkflowSubmit":
		return s.sqlWorkflowSubmit(ctx, request)
	case "archery/sqlWorkflowCheck":
		return s.sqlWorkflowCheck(ctx, request)
	case "archery/sqlWorkflowRollback":
		return s.sqlWorkflowRollback(ctx, request)
	case "archery/sqlWorkflowRollbackSubmit":
		return s.sqlWorkflowRollbackSubmit(ctx, request)
	case "archery/sqlSubmitOptions":
		return s.sqlSubmitOptions(ctx, request)
	case "archery/sqlWorkflowTargets":
		return s.sqlWorkflowTargets(ctx, request)
	case "archery/sqlWorkflowAuditors":
		return s.sqlWorkflowAuditors(ctx, request)
	case "archery/workflowTodos":
		return s.workflowTodos(ctx, request)
	case "archery/workflowTodoTarget":
		return s.workflowTodoTarget(ctx, request)
	case "archery/user/twoFactorState":
		return s.twoFactorState(ctx, request)
	case "archery/user/twoFactorStart":
		return s.twoFactorStart(ctx, request)
	case "archery/user/twoFactorConfirm":
		return s.twoFactorConfirm(ctx, request)
	case "archery/user/twoFactorDisable":
		return s.twoFactorDisable(ctx, request)
	case "archery/user/changePassword":
		return s.changePassword(ctx, request)
	case "archery/saveTextFile":
		return s.saveTextFile(ctx, request)
	case "archery/uiState/get":
		return s.getUIState(request)
	case "archery/uiState/set":
		return s.setUIState(request)
	}

	return nil, &Error{Code: -32601, Kind: "method_not_found", Message: "method_not_found: " + method}
}

func (s *Service) testConnection(ctx context.Context, request Request) (any, *Error) {
	// 测试连接必须走完整密码校验，不能拿历史会话掩盖改错的密码。
	client, buildErr := buildClient(request, nil)
	if buildErr != nil {
		return nil, buildErr
	}
	defer client.CloseIdleConnections()

	version, _ := client.Version(ctx)
	if loginErr := client.EnsureSession(ctx); loginErr != nil {
		if archery.IsKind(loginErr, archery.KindTwoFARequired) {
			return map[string]any{
				"success":      true,
				"pendingTwofa": true,
				"twofaType":    client.Config().TwoFAType,
				"username":     client.Username(),
				"message":      "密码校验通过；该账号启用了 2FA，请在插件内完成验证",
				"version":      version,
				"readOnly":     client.ReadOnly(),
			}, nil
		}
		return nil, mapError(loginErr)
	}
	message := "已连接 Archery"
	if version != "" {
		message = "已连接 Archery " + version
	}
	return map[string]any{
		"success":      true,
		"pendingTwofa": false,
		"message":      message,
		"version":      version,
		"readOnly":     client.ReadOnly(),
	}, nil
}

func (s *Service) connect(ctx context.Context, request Request) (any, *Error) {
	connectionID := strings.TrimSpace(request.ConnectionID)
	if connectionID == "" {
		// 旧宿主可能只给 connection.id；再退一步，若当前只有一个会话就用它。
		if onlyID, _ := s.onlySession(); onlyID != "" {
			connectionID = onlyID
		} else {
			return nil, invalidParams("missing connection id; reopen the connection from DBX")
		}
	}

	client, err := s.resolveClient(connectionID, request)
	if err != nil {
		return nil, err
	}

	otp := archery.OTP{Code: request.OTP, Type: request.OTPType, Phone: request.OTPPhone}
	loginErr := func() error {
		if otp.Code != "" {
			return client.Login(ctx, otp)
		}
		return client.EnsureSession(ctx)
	}()
	if loginErr != nil && !archery.IsKind(loginErr, archery.KindTwoFARequired) {
		return nil, mapError(loginErr)
	}

	version, _ := client.Version(ctx)
	pending := archery.IsKind(loginErr, archery.KindTwoFARequired)
	message := "已连接 Archery"
	switch {
	case pending:
		message = "该账号启用了 2FA，请输入验证码"
	case version != "":
		message = "已连接 Archery " + version
	}
	return map[string]any{
		"success":      true,
		"connectionId": connectionID,
		"pendingTwofa": pending,
		"twofaType":    client.Config().TwoFAType,
		"username":     client.Username(),
		"message":      message,
		"version":      version,
		"readOnly":     client.ReadOnly(),
	}, nil
}

// resolveClient 优先复用已建立的会话（UI 补 2FA 时只有 connectionId，没有密码）。
func (s *Service) resolveClient(connectionID string, request Request) (*archery.Client, *Error) {
	host, _ := endpoint(request)
	if host == "" {
		s.mu.Lock()
		existing := s.sessions[connectionID]
		s.mu.Unlock()
		if existing == nil {
			return nil, invalidParams("connection config is missing; reconnect from DBX")
		}
		return existing, nil
	}

	baseURL, err := buildBaseURL(request)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	existing := s.sessions[connectionID]
	s.mu.Unlock()
	if existing != nil && existing.BaseURL() == baseURL {
		return existing, nil
	}

	client, buildErr := buildClient(request, s.sessionStore())
	if buildErr != nil {
		return nil, buildErr
	}
	s.mu.Lock()
	previous := s.sessions[connectionID]
	s.sessions[connectionID] = client
	s.scopes[connectionID] = baseURL
	s.mu.Unlock()
	if previous != nil {
		previous.CloseIdleConnections()
	}
	return client, nil
}

func (s *Service) disconnect(request Request) (any, *Error) {
	connectionID := strings.TrimSpace(request.ConnectionID)
	if connectionID == "" {
		if onlyID, _ := s.onlySession(); onlyID != "" {
			connectionID = onlyID
		}
	}
	if connectionID == "" {
		return nil, invalidParams("missing connectionId")
	}
	s.mu.Lock()
	client := s.sessions[connectionID]
	delete(s.sessions, connectionID)
	s.mu.Unlock()
	if client != nil {
		client.CloseIdleConnections()
	}
	return map[string]any{"success": true}, nil
}

// sessionKeepAlive 触发一次会话续期探测。
// UI 定期调用：会话有效时只多一个登录态 GET；失效时 sidecar 会自动用保存的密码重登，
// 2FA 账号则返回 twofa_required，交给 UI 弹验证码框。
func (s *Service) sessionKeepAlive(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	result, keepErr := client.KeepAlive(ctx)
	if keepErr != nil {
		return nil, mapError(keepErr)
	}
	return map[string]any{"session": result}, nil
}

func (s *Service) overview(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	version, versionErr := client.Version(ctx)
	if versionErr != nil && archery.IsKind(versionErr, archery.KindNetwork) {
		return nil, mapError(versionErr)
	}
	instances, listErr := client.Instances(ctx)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	return map[string]any{
		"version":       version,
		"username":      client.Username(),
		"readOnly":      client.ReadOnly(),
		"authMode":      authMode(client),
		"instanceCount": len(instances),
		"baseURL":       client.BaseURL(),
	}, nil
}

func (s *Service) instances(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	list, listErr := client.Instances(ctx)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if list == nil {
		list = []archery.Instance{}
	}
	return map[string]any{"instances": list}, nil
}

func (s *Service) resources(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	resourceType := strings.TrimSpace(request.ResourceType)
	switch resourceType {
	case "database", "schema", "table", "column":
	default:
		return nil, invalidParams("unsupported resourceType")
	}
	ref := archery.InstanceRef{ID: request.InstanceID, Name: strings.TrimSpace(request.InstanceName)}
	if ref.ID == 0 && ref.Name == "" {
		return nil, invalidParams("instanceName or instanceId is required")
	}
	list, listErr := client.Resources(ctx, ref, resourceType, request.DBName, request.SchemaName, request.TBName)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if list == nil {
		list = []archery.Resource{}
	}
	return map[string]any{"resources": list}, nil
}

// dictTables 返回数据字典（表名 + 注释）。
// 注释来自 Archery 数据字典接口；账号无该菜单权限时 commentAvailable=false，
// 前端据此提示「注释不可用」，表名仍然展示。
func (s *Service) dictTables(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	instanceName := strings.TrimSpace(request.InstanceName)
	dbName := strings.TrimSpace(request.DBName)
	if instanceName == "" || dbName == "" {
		return nil, invalidParams("instanceName and dbName are required")
	}
	tables, commentAvailable, dictErr := client.TableDictionary(
		ctx,
		archery.InstanceRef{ID: request.InstanceID, Name: instanceName},
		dbName,
		strings.TrimSpace(request.DBType),
	)
	if dictErr != nil {
		return nil, mapError(dictErr)
	}
	if tables == nil {
		tables = []archery.DictionaryTable{}
	}
	return map[string]any{"tables": tables, "commentAvailable": commentAvailable}, nil
}

// tableMeta 返回单表元数据（表信息 / 列信息 / 索引信息 / 建表语句）。
func (s *Service) tableMeta(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	instanceName := strings.TrimSpace(request.InstanceName)
	dbName := strings.TrimSpace(request.DBName)
	tbName := strings.TrimSpace(request.TBName)
	if instanceName == "" || dbName == "" || tbName == "" {
		return nil, invalidParams("instanceName, dbName and tbName are required")
	}
	meta, metaErr := client.TableMetadata(
		ctx,
		archery.InstanceRef{ID: request.InstanceID, Name: instanceName},
		dbName,
		tbName,
		strings.TrimSpace(request.DBType),
	)
	if metaErr != nil {
		return nil, mapError(metaErr)
	}
	return map[string]any{"meta": meta}, nil
}

func (s *Service) describe(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.InstanceName) == "" || strings.TrimSpace(request.DBName) == "" || strings.TrimSpace(request.TBName) == "" {
		return nil, invalidParams("instanceName, dbName and tbName are required")
	}
	describe, describeErr := client.Describe(ctx, archery.InstanceRef{ID: request.InstanceID, Name: request.InstanceName}, request.DBName, request.TBName, request.SchemaName)
	if describeErr != nil {
		return nil, mapError(describeErr)
	}
	return map[string]any{"describe": describe}, nil
}

func (s *Service) query(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.InstanceName) == "" {
		return nil, invalidParams("instanceName is required")
	}
	if strings.TrimSpace(request.DBName) == "" {
		return nil, invalidParams("dbName is required")
	}
	if strings.TrimSpace(request.SQLContent) == "" {
		return nil, invalidParams("sqlContent is required")
	}

	limit := request.LimitNum
	switch {
	case limit < 0:
		limit = 0
	case limit == 0:
		limit = client.Config().DefaultLimit
	}
	if limit > 10000 {
		limit = 10000
	}

	result, queryErr := client.Query(ctx, archery.QueryRequest{
		InstanceName: request.InstanceName,
		DBName:       request.DBName,
		SchemaName:   request.SchemaName,
		TBName:       request.TBName,
		SQL:          request.SQLContent,
		LimitNum:     limit,
	})
	if queryErr != nil {
		return nil, mapError(queryErr)
	}
	return map[string]any{"result": result}, nil
}

func (s *Service) queryLogs(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	page, logErr := client.QueryLogs(ctx, archery.LogQuery{
		Limit:        limit,
		Offset:       offset,
		Search:       strings.TrimSpace(request.Search),
		Star:         request.Star,
		QueryLogID:   int(request.QueryLogID),
		StartDate:    request.StartDate,
		EndDate:      request.EndDate,
		InstanceName: strings.TrimSpace(request.InstanceName),
		DBName:       strings.TrimSpace(request.DBName),
	})
	if logErr != nil {
		return nil, mapError(logErr)
	}
	return map[string]any{"page": page}, nil
}

func (s *Service) favorite(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.QueryLogID <= 0 {
		return nil, invalidParams("queryLogId is required")
	}
	if favoriteErr := client.Favorite(ctx, request.QueryLogID, request.Star, request.Alias); favoriteErr != nil {
		return nil, mapError(favoriteErr)
	}
	return map[string]any{"success": true}, nil
}

// announcement 只返回 Archery 首页的公告块，供所有页面顶部统一展示。
// 公告是锦上添花：首页取不到时返回 degraded，UI 不显示公告条也不报错。
func (s *Service) announcement(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	html, fetchErr := client.DashboardHTML(ctx, "", "")
	if fetchErr != nil {
		return map[string]any{
			"degraded":     true,
			"reason":       dashboardReason(fetchErr),
			"announcement": nil,
		}, nil
	}
	return map[string]any{
		"degraded":     false,
		"announcement": archery.ParseAnnouncement(html),
	}, nil
}

// dashboard 返回 Archery 首页四个指标卡的数字。
// 数字只存在于 /dashboard/ 的服务端渲染 HTML 中，取不到时降级而不是报错：
// UI 会用本地实例列表兜底实例数，其余指标显示破折号。
func (s *Service) dashboard(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	html, fetchErr := client.DashboardHTML(ctx, strings.TrimSpace(request.StartDate), strings.TrimSpace(request.EndDate))
	if fetchErr != nil {
		return map[string]any{
			"degraded": true,
			"reason":   dashboardReason(fetchErr),
		}, nil
	}
	counts, parseErr := archery.ParseDashboardCounts(html)
	announcement := archery.ParseAnnouncement(html)
	brand := archery.ParseBrand(html)
	// 首页「数据库实例类型统计」按全库实例统计，和当前账号可查询的实例清单不是一回事，
	// 图表优先用这里的数字，才能和网页一致。
	instanceStats := archery.ParseInstanceTypeStats(html)
	// 首页下方两张按日统计图（检索行数/次数、上线数量）同样只存在于这张页面里。
	trends := archery.ParseDashboardTrends(html)
	if parseErr != nil {
		return map[string]any{
			"degraded":      true,
			"reason":        "parse_failed",
			"announcement":  announcement,
			"brand":         brand.Name,
			"brandSuffix":   brand.Suffix,
			"instanceStats": instanceStats,
			"trends":        trends,
		}, nil
	}
	return map[string]any{
		"degraded":      false,
		"counts":        counts,
		"announcement":  announcement,
		"brand":         brand.Name,
		"brandSuffix":   brand.Suffix,
		"instanceStats": instanceStats,
		"trends":        trends,
	}, nil
}

// dashboardTrends 按时间范围重新取首页两张按日统计图。
// 页面视图 /dashboard/ 不认日期参数，真正按区间出数的是 /dashboard/api/。
func (s *Service) dashboardTrends(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	start := strings.TrimSpace(request.StartDate)
	end := strings.TrimSpace(request.EndDate)
	if start == "" || end == "" {
		return nil, invalidParams("startDate and endDate are required")
	}
	trends, trendErr := client.DashboardTrends(ctx, start, end)
	if trendErr != nil {
		return nil, mapError(trendErr)
	}
	return map[string]any{"trends": trends}, nil
}

// ---- 查询权限管理 ----

// privileges 返回权限申请列表；limit 默认 20、上限 200，与查询历史保持一致。
func (s *Service) privileges(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	page, listErr := client.PrivilegeApplies(ctx, limit, offset, strings.TrimSpace(request.Search))
	if listErr != nil {
		return nil, mapError(listErr)
	}
	return map[string]any{"page": page}, nil
}

// myPrivileges 返回当前账号的查询权限清单（Archery「我的权限」页）。
// 分页规则与申请列表一致：limit 默认 20、上限 200，offset 不为负。
func (s *Service) myPrivileges(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	page, listErr := client.UserQueryPrivileges(ctx, limit, offset, strings.TrimSpace(request.Search))
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if page.Rows == nil {
		page.Rows = []archery.UserPrivilege{}
	}
	return map[string]any{"page": page}, nil
}

// privilegeOptions 返回申请弹窗可选的资源组（当前用户所属组）。
func (s *Service) privilegeOptions(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	groups, listErr := client.PrivilegeGroups(ctx)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	return map[string]any{"groups": groups}, nil
}

func (s *Service) privilegeInstances(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	groupName := strings.TrimSpace(request.GroupName)
	if groupName == "" {
		return nil, invalidParams("groupName is required")
	}
	instances, listErr := client.GroupInstances(ctx, groupName)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if instances == nil {
		instances = []archery.GroupInstance{}
	}
	return map[string]any{"instances": instances}, nil
}

// privilegeApply 提交查询权限申请。
// 该动作只写 Archery 平台工单，不改数据库，因此不受 read_only 拦截。
func (s *Service) privilegeApply(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return nil, invalidParams("title is required")
	}
	groupName := strings.TrimSpace(request.GroupName)
	if groupName == "" {
		return nil, invalidParams("groupName is required")
	}
	instanceName := strings.TrimSpace(request.InstanceName)
	if instanceName == "" {
		return nil, invalidParams("instanceName is required")
	}
	if request.PrivType != archery.PrivilegeTypeDatabase && request.PrivType != archery.PrivilegeTypeTable {
		return nil, invalidParams("privType must be 1 (database) or 2 (table)")
	}
	dbName := strings.TrimSpace(request.DBName)
	dbList := cleanPrivilegeList(request.DBList)
	tableList := cleanPrivilegeList(request.TableList)
	switch request.PrivType {
	case archery.PrivilegeTypeDatabase:
		if len(dbList) == 0 {
			return nil, invalidParams("dbList is required for database-level privileges")
		}
	case archery.PrivilegeTypeTable:
		if dbName == "" {
			return nil, invalidParams("dbName is required for table-level privileges")
		}
		if len(tableList) == 0 {
			return nil, invalidParams("tableList is required for table-level privileges")
		}
	}
	validDate := strings.TrimSpace(request.ValidDate)
	if validDate == "" {
		return nil, invalidParams("validDate is required")
	}
	if request.LimitNum <= 0 {
		return nil, invalidParams("limitNum must be positive")
	}

	applyErr := client.ApplyPrivilege(ctx, archery.PrivilegeApplyInput{
		Title:        title,
		GroupName:    groupName,
		InstanceName: instanceName,
		PrivType:     request.PrivType,
		DBName:       dbName,
		DBList:       dbList,
		TableList:    tableList,
		ValidDate:    validDate,
		LimitNum:     request.LimitNum,
	})
	if applyErr != nil {
		return nil, mapError(applyErr)
	}
	return map[string]any{"success": true}, nil
}

func (s *Service) privilegeDetail(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.ApplyID <= 0 {
		return nil, invalidParams("applyId is required")
	}
	detail, detailErr := client.PrivilegeDetail(ctx, request.ApplyID)
	if detailErr != nil {
		return nil, mapError(detailErr)
	}
	return map[string]any{"detail": detail}, nil
}

func (s *Service) privilegeAudit(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.ApplyID <= 0 {
		return nil, invalidParams("applyId is required")
	}
	if request.AuditStatus != archery.AuditPass && request.AuditStatus != archery.AuditReject {
		return nil, invalidParams("auditStatus must be 1 (pass) or 2 (reject)")
	}
	auditErr := client.AuditPrivilege(ctx, request.ApplyID, request.AuditStatus, strings.TrimSpace(request.AuditRemark))
	if auditErr != nil {
		return nil, mapError(auditErr)
	}
	return map[string]any{"success": true}, nil
}

func (s *Service) workflowLog(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	page, logErr := client.WorkflowLogs(ctx, request.WorkflowID, request.WorkflowType)
	if logErr != nil {
		return nil, mapError(logErr)
	}
	return map[string]any{"page": page}, nil
}

// ---- 待办列表 ----

// workflowTodos 返回「待我审核」的待办列表（Archery /workflow/list/）。
// 只读：只查审批记录，不改任何数据，因此不在只读守卫名单内。
func (s *Service) workflowTodos(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	workflowType := request.WorkflowType
	if workflowType < 0 {
		workflowType = 0
	}
	page, listErr := client.WorkflowTodos(ctx, archery.WorkflowTodoQuery{
		Limit:        limit,
		Offset:       offset,
		Search:       strings.TrimSpace(request.Search),
		WorkflowType: workflowType,
	})
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if page.Rows == nil {
		page.Rows = []archery.WorkflowTodo{}
	}
	return map[string]any{"page": page}, nil
}

// workflowTodoTarget 把 audit_id 解析成业务工单号与工单类型，供审核入口跳转。
func (s *Service) workflowTodoTarget(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.AuditID <= 0 {
		return nil, invalidParams("auditId is required")
	}
	target, targetErr := client.WorkflowTodoTarget(ctx, request.AuditID)
	if targetErr != nil {
		return nil, mapError(targetErr)
	}
	return map[string]any{"target": target}, nil
}

// ---- SQL 上线工单 ----

// sqlWorkflows 返回工单列表；audit=true 时只返回当前需要我审核的工单。
func (s *Service) sqlWorkflows(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	page, listErr := client.SqlWorkflows(ctx, archery.SqlWorkflowQuery{
		Limit:       limit,
		Offset:      offset,
		Search:      strings.TrimSpace(request.Search),
		Status:      strings.TrimSpace(request.Status),
		InstanceID:  request.InstanceID,
		GroupID:     request.GroupID,
		StartDate:   strings.TrimSpace(request.StartDate),
		EndDate:     strings.TrimSpace(request.EndDate),
		SyntaxTypes: cleanPrivilegeList(request.SyntaxTypes),
		Audit:       request.Audit,
	})
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if page.Rows == nil {
		page.Rows = []archery.SqlWorkflow{}
	}
	return map[string]any{"page": page}, nil
}

// sqlWorkflowOptions 返回列表筛选用的实例与资源组下拉。
func (s *Service) sqlWorkflowOptions(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	options, listErr := client.SqlWorkflowOptions(ctx)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	return map[string]any{
		"instances": options.Instances,
		"groups":    options.Groups,
	}, nil
}

func (s *Service) sqlWorkflowDetail(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	detail, detailErr := client.SqlWorkflowDetail(ctx, request.WorkflowID)
	if detailErr != nil {
		return nil, mapError(detailErr)
	}
	return map[string]any{"detail": detail}, nil
}

// sqlWorkflowAudit 审核工单：pass=审核通过，reject=驳回（Archery 的 /cancel/）。
func (s *Service) sqlWorkflowAudit(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != archery.WorkflowActionPass && action != archery.WorkflowActionReject {
		return nil, invalidParams("action must be pass or reject")
	}
	remark := strings.TrimSpace(request.Remark)
	if remark == "" {
		remark = strings.TrimSpace(request.AuditRemark)
	}
	if action == archery.WorkflowActionReject && remark == "" {
		return nil, invalidParams("remark is required when rejecting")
	}
	if auditErr := client.AuditSqlWorkflow(ctx, request.WorkflowID, action, remark); auditErr != nil {
		return nil, mapError(auditErr)
	}
	return map[string]any{"success": true}, nil
}

// sqlWorkflowExecute 执行工单：auto=平台排队执行，manual=确认线下执行完成。
func (s *Service) sqlWorkflowExecute(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode != archery.WorkflowModeAuto && mode != archery.WorkflowModeManual {
		return nil, invalidParams("mode must be auto or manual")
	}
	if executeErr := client.ExecuteSqlWorkflow(ctx, request.WorkflowID, mode); executeErr != nil {
		return nil, mapError(executeErr)
	}
	return map[string]any{"success": true}, nil
}

// sqlWorkflowSchedule 设置定时执行，runDate 形如 2026-11-05 03:00。
func (s *Service) sqlWorkflowSchedule(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	runDate := strings.TrimSpace(request.RunDate)
	if _, parseErr := time.Parse("2006-01-02 15:04", runDate); parseErr != nil {
		return nil, invalidParams("runDate must be YYYY-MM-DD HH:mm")
	}
	if scheduleErr := client.ScheduleSqlWorkflow(ctx, request.WorkflowID, runDate); scheduleErr != nil {
		return nil, mapError(scheduleErr)
	}
	return map[string]any{"success": true}, nil
}

// sqlWorkflowAlterRunDate 审核人修改可执行时间；两端留空表示清空为「无限制」。
func (s *Service) sqlWorkflowAlterRunDate(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	start := strings.TrimSpace(request.RunDateStart)
	end := strings.TrimSpace(request.RunDateEnd)
	// Archery 允许单端为空，但详情页只有两端齐全才展示窗口，单端会静默变成「无限制」。
	if (start == "") != (end == "") {
		return nil, invalidParams("runDateStart and runDateEnd must be provided together")
	}
	if start != "" {
		startAt, parseErr := time.Parse("2006-01-02 15:04", start)
		if parseErr != nil {
			return nil, invalidParams("runDateStart must be YYYY-MM-DD HH:mm")
		}
		endAt, parseErr := time.Parse("2006-01-02 15:04", end)
		if parseErr != nil {
			return nil, invalidParams("runDateEnd must be YYYY-MM-DD HH:mm")
		}
		if endAt.Before(startAt) {
			return nil, invalidParams("runDateEnd must not be earlier than runDateStart")
		}
	}
	if alterErr := client.AlterSqlWorkflowRunDate(ctx, request.WorkflowID, start, end); alterErr != nil {
		return nil, mapError(alterErr)
	}
	return map[string]any{"success": true}, nil
}

// sqlWorkflowSubmit 提交 SQL 上线工单，返回工单号。
func (s *Service) sqlWorkflowSubmit(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return nil, invalidParams("title is required")
	}
	if len([]rune(title)) > 50 {
		return nil, invalidParams("title must not exceed 50 characters")
	}
	if request.GroupID <= 0 {
		return nil, invalidParams("groupId is required")
	}
	if request.InstanceID <= 0 {
		return nil, invalidParams("instanceId is required")
	}
	dbName := strings.TrimSpace(request.DBName)
	if dbName == "" {
		return nil, invalidParams("dbName is required")
	}
	sqlText := workflowSQL(request)
	if sqlText == "" {
		return nil, invalidParams("sql is required")
	}
	runDateStart := strings.TrimSpace(request.RunDateStart)
	runDateEnd := strings.TrimSpace(request.RunDateEnd)
	if runDateStart != "" && runDateEnd != "" {
		start, startErr := time.Parse("2006-01-02 15:04", runDateStart)
		end, endErr := time.Parse("2006-01-02 15:04", runDateEnd)
		if startErr != nil || endErr != nil {
			return nil, invalidParams("runDateStart and runDateEnd must be YYYY-MM-DD HH:mm")
		}
		if !end.After(start) {
			return nil, invalidParams("runDateEnd must be later than runDateStart")
		}
	}

	workflowID, submitErr := client.SubmitSqlWorkflow(ctx, archery.SqlWorkflowSubmitInput{
		Title:        title,
		DemandURL:    strings.TrimSpace(request.DemandURL),
		GroupID:      request.GroupID,
		InstanceID:   request.InstanceID,
		DBName:       dbName,
		SQL:          sqlText,
		IsBackup:     request.Backup,
		RunDateStart: runDateStart,
		RunDateEnd:   runDateEnd,
	})
	if submitErr != nil {
		return nil, mapError(submitErr)
	}
	return map[string]any{"workflowId": workflowID}, nil
}

// sqlWorkflowCheck 提交前做 SQL 检测。
func (s *Service) sqlWorkflowCheck(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.InstanceID <= 0 {
		return nil, invalidParams("instanceId is required")
	}
	dbName := strings.TrimSpace(request.DBName)
	if dbName == "" {
		return nil, invalidParams("dbName is required")
	}
	sqlText := workflowSQL(request)
	if sqlText == "" {
		return nil, invalidParams("sql is required")
	}
	summary, checkErr := client.CheckSqlWorkflow(ctx, request.InstanceID, dbName, sqlText)
	if checkErr != nil {
		return nil, mapError(checkErr)
	}
	return summary, nil
}

// sqlWorkflowRollback 返回工单回滚 SQL（详情页「查看回滚SQL」子页）。
func (s *Service) sqlWorkflowRollback(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	rows, rollbackErr := client.RollbackSQL(ctx, request.WorkflowID)
	if rollbackErr != nil {
		return nil, mapError(rollbackErr)
	}
	text := rollbackStatements(rows, "")
	if text != "" {
		text += ";"
	}
	return archery.SqlRollbackResult{Rows: rows, SQL: text}, nil
}

// sqlWorkflowRollbackSubmit 用回滚语句新建一个 SQL 上线工单（Archery 的「提交回滚请求」）。
// 回滚工单不开启备份、不带执行时间窗，组/实例/库沿用原工单。
func (s *Service) sqlWorkflowRollbackSubmit(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if request.WorkflowID <= 0 {
		return nil, invalidParams("workflowId is required")
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return nil, invalidParams("title is required")
	}
	if len([]rune(title)) > 100 {
		return nil, invalidParams("title must not exceed 100 characters")
	}
	if request.GroupID <= 0 {
		return nil, invalidParams("groupId is required")
	}
	if request.InstanceID <= 0 {
		return nil, invalidParams("instanceId is required")
	}
	dbName := strings.TrimSpace(request.DBName)
	if dbName == "" {
		return nil, invalidParams("dbName is required")
	}
	rows, rollbackErr := client.RollbackSQL(ctx, request.WorkflowID)
	if rollbackErr != nil {
		return nil, mapError(rollbackErr)
	}
	sqlText := rollbackStatements(rows, dbName)
	if sqlText == "" {
		return nil, &Error{Code: -32000, Kind: "server", Message: "server: 该工单没有可用的回滚语句"}
	}
	sqlText += ";"
	workflowID, submitErr := client.SubmitSqlWorkflow(ctx, archery.SqlWorkflowSubmitInput{
		Title:      title,
		GroupID:    request.GroupID,
		InstanceID: request.InstanceID,
		DBName:     dbName,
		SQL:        sqlText,
	})
	if submitErr != nil {
		return nil, mapError(submitErr)
	}
	return map[string]any{"workflowId": workflowID}, nil
}

// rollbackStatements 取全部回滚语句，按 Archery 返回顺序（执行顺序倒序）拼接。
// 库名前缀与语句自带的分号在这里一并归一：goInception 会把回滚语句补成
// 「`原库`.`表`」并自带结尾分号，而回滚工单本来就落在原工单的库上，
// 前缀既冗余又会在换库/换实例时打错库；分号只在整段末尾由调用方补一次。
func rollbackStatements(rows []archery.SqlRollbackRow, dbName string) string {
	statements := make([]string, 0, len(rows))
	for _, row := range rows {
		sqlText := strings.TrimSpace(stripDatabaseQualifier(row.RollbackSQL, dbName))
		sqlText = strings.TrimSpace(strings.TrimRight(sqlText, ";"))
		if sqlText != "" {
			statements = append(statements, sqlText)
		}
	}
	return strings.Join(statements, ";\n\n")
}

// stripDatabaseQualifier 去掉回滚语句里与原工单库同名的库名前缀，例如
// "ALTER TABLE `schedule-dev`.`bs_vehicle_info` ..." -> "ALTER TABLE `bs_vehicle_info` ..."。
// 只剥完整的库名 token（前后不接标识符字符，点前后允许空白）；字符串字面量
// ('…' / "…"，含反斜杠转义) 与别的库名（schedule-dev2、other 等）原样保留。
func stripDatabaseQualifier(sqlText, dbName string) string {
	db := strings.TrimSpace(dbName)
	if db == "" || sqlText == "" {
		return sqlText
	}
	var out strings.Builder
	out.Grow(len(sqlText))
	for index := 0; index < len(sqlText); {
		char := sqlText[index]
		if char == '\'' || char == '"' {
			end := index + 1
			for end < len(sqlText) {
				if sqlText[end] == '\\' && end+1 < len(sqlText) {
					end += 2
					continue
				}
				end++
				if sqlText[end-1] == char {
					break
				}
			}
			out.WriteString(sqlText[index:end])
			index = end
			continue
		}
		if char == '`' {
			close := strings.IndexByte(sqlText[index+1:], '`')
			if close >= 0 {
				close += index + 1
				after := skipSQLSpaces(sqlText, close+1)
				if strings.EqualFold(sqlText[index+1:close], db) &&
					after < len(sqlText) && sqlText[after] == '.' {
					index = skipSQLSpaces(sqlText, after+1)
					continue
				}
				out.WriteString(sqlText[index : close+1])
				index = close + 1
				continue
			}
		}
		if (index == 0 || !isSQLIdentChar(sqlText[index-1])) &&
			index+len(db) <= len(sqlText) && strings.EqualFold(sqlText[index:index+len(db)], db) {
			dot := skipSQLSpaces(sqlText, index+len(db))
			if dot < len(sqlText) && sqlText[dot] == '.' {
				table := skipSQLSpaces(sqlText, dot+1)
				if table < len(sqlText) {
					index = table
					continue
				}
			}
		}
		out.WriteByte(char)
		index++
	}
	return out.String()
}

func skipSQLSpaces(text string, from int) int {
	index := from
	for index < len(text) && (text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n') {
		index++
	}
	return index
}

// isSQLIdentChar 判断是否仍是同一个标识符：含点号，避免把 a.b.c 里的 b 当库名剥掉。
func isSQLIdentChar(char byte) bool {
	return char == '_' || char == '$' || char == '`' || char == '.' ||
		(char >= '0' && char <= '9') || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

// sqlSubmitOptions 返回提交弹窗的组下拉与备份开关。
func (s *Service) sqlSubmitOptions(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	options, listErr := client.SqlSubmitOptions(ctx)
	if listErr != nil {
		return nil, mapError(listErr)
	}
	return map[string]any{
		"groups":        options.Groups,
		"backupEnabled": options.BackupEnabled,
	}, nil
}

// sqlWorkflowTargets 返回资源组内标记为「支持上线」（can_write）的实例。
func (s *Service) sqlWorkflowTargets(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	groupName := strings.TrimSpace(request.GroupName)
	if groupName == "" {
		return nil, invalidParams("groupName is required")
	}
	instances, listErr := client.GroupInstancesByTag(ctx, groupName, "can_write")
	if listErr != nil {
		return nil, mapError(listErr)
	}
	if instances == nil {
		instances = []archery.GroupInstance{}
	}
	return map[string]any{"instances": instances}, nil
}

// sqlWorkflowAuditors 返回资源组的 SQL 上线审批流（提交页蓝色提示条）。
func (s *Service) sqlWorkflowAuditors(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	groupName := strings.TrimSpace(request.GroupName)
	if groupName == "" {
		return nil, invalidParams("groupName is required")
	}
	display, auditors, auditorsErr := client.GroupAuditors(ctx, groupName, archery.WorkflowTypeSqlReview)
	if auditorsErr != nil {
		return nil, mapError(auditorsErr)
	}
	return map[string]any{
		"auditors":        auditors,
		"auditorsDisplay": display,
	}, nil
}

// privilegeAuditors 返回资源组的查询权限审批流（申请弹窗选中组后的蓝色提示条）。
func (s *Service) privilegeAuditors(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	groupName := strings.TrimSpace(request.GroupName)
	if groupName == "" {
		return nil, invalidParams("groupName is required")
	}
	display, auditors, auditorsErr := client.GroupAuditors(ctx, groupName, archery.WorkflowTypeQueryPriv)
	if auditorsErr != nil {
		return nil, mapError(auditorsErr)
	}
	return map[string]any{
		"auditors":        auditors,
		"auditorsDisplay": display,
	}, nil
}

// workflowSQL 取提交/检测用的 SQL 文本，兼容 sql 与 sqlContent 两种参数名。
func workflowSQL(request Request) string {
	if text := strings.TrimSpace(request.SQLText); text != "" {
		return text
	}
	return strings.TrimSpace(request.SQLContent)
}

// cleanPrivilegeList 去掉空白项，避免把空字符串提交给 Archery。
func cleanPrivilegeList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			cleaned = append(cleaned, text)
		}
	}
	return cleaned
}

func dashboardReason(err error) string {
	switch {
	case archery.IsKind(err, archery.KindForbidden):
		return "forbidden"
	case archery.IsKind(err, archery.KindNotFound):
		return "not_found"
	case archery.IsKind(err, archery.KindAuthRequired):
		return "auth_required"
	default:
		return "unavailable"
	}
}

// ---- 账号设置：两步验证与修改密码 ----

func accountAuthType(value string) (string, *Error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "totp":
		return "totp", nil
	case "sms":
		return "sms", nil
	default:
		return "", invalidParams("authType must be totp or sms")
	}
}

// verifyAccountPassword 在开关两步验证前复核连接密码，与 Archery 网页一致。
// 旧版 Archery 没有 /api/v1/user/auth/ 时放行，避免功能整体不可用。
func verifyAccountPassword(ctx context.Context, client *archery.Client) *Error {
	password := client.Config().Password
	if strings.TrimSpace(password) == "" {
		return nil
	}
	if err := client.VerifyPassword(ctx, password); err != nil {
		if archery.IsKind(err, archery.KindNotFound) {
			return nil
		}
		return mapError(err)
	}
	return nil
}

func (s *Service) twoFactorState(ctx context.Context, request Request) (any, *Error) {
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	state, apiErr := client.TwoFactorState(ctx)
	if apiErr != nil {
		return nil, mapError(apiErr)
	}
	return map[string]any{"totp": state.TOTP, "sms": state.SMS}, nil
}

func (s *Service) twoFactorStart(ctx context.Context, request Request) (any, *Error) {
	authType, authErr := accountAuthType(request.AuthType)
	if authErr != nil {
		return nil, authErr
	}
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if verifyErr := verifyAccountPassword(ctx, client); verifyErr != nil {
		return nil, verifyErr
	}
	if authType == "sms" {
		phone := strings.TrimSpace(request.Phone)
		if phone == "" {
			return nil, invalidParams("phone is required")
		}
		if sendErr := client.SendSMSCode(ctx, phone); sendErr != nil {
			return nil, mapError(sendErr)
		}
		return map[string]any{"authType": "sms", "phone": phone}, nil
	}
	enrollment, enrollErr := client.StartTOTPEnrollment(ctx)
	if enrollErr != nil {
		return nil, mapError(enrollErr)
	}
	return map[string]any{
		"authType":  "totp",
		"secretKey": enrollment.SecretKey,
		"qrCode":    enrollment.QRCode,
	}, nil
}

func (s *Service) twoFactorConfirm(ctx context.Context, request Request) (any, *Error) {
	authType, authErr := accountAuthType(request.AuthType)
	if authErr != nil {
		return nil, authErr
	}
	if strings.TrimSpace(request.OTP) == "" {
		return nil, invalidParams("otp is required")
	}
	if authType == "totp" && strings.TrimSpace(request.SecretKey) == "" {
		return nil, invalidParams("secretKey is required")
	}
	if authType == "sms" && strings.TrimSpace(request.Phone) == "" {
		return nil, invalidParams("phone is required")
	}
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if confirmErr := client.ConfirmTwoFactor(ctx, authType, request.OTP, request.SecretKey, request.Phone); confirmErr != nil {
		return nil, mapError(confirmErr)
	}
	return map[string]any{"success": true}, nil
}

func (s *Service) twoFactorDisable(ctx context.Context, request Request) (any, *Error) {
	authType, authErr := accountAuthType(request.AuthType)
	if authErr != nil {
		return nil, authErr
	}
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if verifyErr := verifyAccountPassword(ctx, client); verifyErr != nil {
		return nil, verifyErr
	}
	if disableErr := client.DisableTwoFactor(ctx, authType); disableErr != nil {
		return nil, mapError(disableErr)
	}
	return map[string]any{"success": true}, nil
}

func (s *Service) changePassword(ctx context.Context, request Request) (any, *Error) {
	oldPassword := strings.TrimSpace(request.OldPassword)
	newPassword := strings.TrimSpace(request.NewPassword)
	if oldPassword == "" || newPassword == "" {
		return nil, invalidParams("oldPassword and newPassword are required")
	}
	if oldPassword == newPassword {
		return nil, invalidParams("newPassword must differ from oldPassword")
	}
	client, err := s.session(request.ConnectionID)
	if err != nil {
		return nil, err
	}
	if changeErr := client.ChangePassword(ctx, oldPassword, newPassword); changeErr != nil {
		return nil, mapError(changeErr)
	}
	return map[string]any{"success": true}, nil
}

func (s *Service) session(connectionID string) (*archery.Client, *Error) {
	if strings.TrimSpace(connectionID) == "" {
		if _, client := s.onlySession(); client != nil {
			return client, nil
		}
		return nil, &Error{
			Code:    -32000,
			Kind:    "not_connected",
			Message: "not_connected: 无法确定连接（未建立或多连接并存），请从 DBX 重新打开该连接",
		}
	}
	s.mu.Lock()
	client := s.sessions[connectionID]
	s.mu.Unlock()
	if client == nil {
		return nil, &Error{
			Code:    -32000,
			Kind:    "not_connected",
			Message: "not_connected: 连接尚未建立，请重新打开该 Archery 连接",
		}
	}
	return client, nil
}

// onlySession 仅在恰好一个连接时返回它，供缺少 connectionId 的宿主兜底。
func (s *Service) onlySession() (string, *archery.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) != 1 {
		return "", nil
	}
	for id, client := range s.sessions {
		return id, client
	}
	return "", nil
}

func buildBaseURL(request Request) (string, *Error) {
	host, port := endpoint(request)
	scheme := configString(request.Connection.ExternalConfig, "scheme", "https")
	basePath := configString(request.Connection.ExternalConfig, "base_path", "")
	baseURL, err := archery.BuildBaseURL(scheme, host, port, basePath)
	if err != nil {
		return "", mapError(err)
	}
	return baseURL, nil
}

func buildClient(request Request, store *archery.SessionStore) (*archery.Client, *Error) {
	baseURL, err := buildBaseURL(request)
	if err != nil {
		return nil, err
	}
	client, clientErr := archery.NewClient(archery.Config{
		BaseURL:            baseURL,
		Username:           strings.TrimSpace(request.Connection.Username),
		Password:           connectionPassword(request),
		AuthMode:           configString(request.Connection.ExternalConfig, "auth_mode", "session"),
		ReadOnly:           configBool(request.Connection.ExternalConfig, "read_only", request.Connection.ReadOnly),
		InsecureSkipVerify: configBool(request.Connection.ExternalConfig, "insecure_skip_verify", false),
		Timeout:            timeSeconds(configInt(request.Connection.ExternalConfig, "connect_timeout_secs", 20)),
		QueryTimeout:       timeSeconds(configInt(request.Connection.ExternalConfig, "query_timeout_secs", 120)),
		TwoFAPhone:         configString(request.Connection.ExternalConfig, "twofa_phone", ""),
		TwoFAType:          configString(request.Connection.ExternalConfig, "twofa_type", "totp"),
		DefaultLimit:       configInt(request.Connection.ExternalConfig, "default_limit_num", 100),
		KeepAlive:          keepAliveDuration(request.Connection.ExternalConfig),
		SessionStore:       store,
	})
	if clientErr != nil {
		return nil, mapError(clientErr)
	}
	return client, nil
}

// keepAliveDuration 把连接配置里的秒数转成心跳间隔：
// 缺省 120 秒，<=0 表示关闭自动续期（传负值给 Client）。
func keepAliveDuration(config map[string]any) time.Duration {
	seconds := configInt(config, "keep_alive_secs", int(archery.DefaultKeepAlive/time.Second))
	if seconds <= 0 {
		return -1
	}
	return time.Duration(seconds) * time.Second
}

func endpoint(request Request) (string, int) {
	host := strings.TrimSpace(request.Runtime.Host)
	if host == "" {
		host = strings.TrimSpace(request.Connection.Host)
	}
	port := request.Runtime.Port
	if port == 0 {
		port = request.Connection.Port
	}
	return host, port
}

func connectionPassword(request Request) string {
	if password := stringValue(request.Connection.Password); password != "" {
		return password
	}
	for _, secrets := range []map[string]any{
		request.Connection.Secrets,
		request.Connection.ConnectionSecrets,
		request.Secrets,
	} {
		for _, key := range []string{"password", "passwd"} {
			if password := stringValue(secrets[key]); password != "" {
				return password
			}
		}
	}
	return ""
}

func authMode(client *archery.Client) string {
	if client.Config().SessionMode() {
		return "session"
	}
	return "jwt"
}

// uiStateVersion 是状态文件的版本：v2 起按 Archery 环境（baseURL）分组。
const uiStateVersion = 2

// uiStateEntry 是一个环境（一套 Archery）的界面状态。
type uiStateEntry struct {
	BaseURL string          `json:"baseURL"`
	SavedAt time.Time       `json:"savedAt"`
	State   json.RawMessage `json:"state"`
}

// uiStateFile 是 v2 状态文件；states 的键是 baseURL，空键保留给 v1 遗留的全局状态，等待被认领。
type uiStateFile struct {
	Version int                     `json:"version"`
	States  map[string]uiStateEntry `json:"states"`
}

// stateScope 返回 UI 状态的分组键：同一套 Archery 环境共用一个状态，
// 不同环境的连接互不可见。否则打开巴西连接会看到广州连接留下的页签与 SQL。
// 优先用请求里带的连接配置（生命周期回调、带 connection 对象的调用），
// 工作台的普通 invoke 只有 connectionId，回落到该连接的会话地址。
func (s *Service) stateScope(request Request) string {
	if host, _ := endpoint(request); host != "" {
		if baseURL, err := buildBaseURL(request); err == nil && baseURL != "" {
			return baseURL
		}
	}
	connectionID := strings.TrimSpace(request.ConnectionID)
	if connectionID == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if client := s.sessions[connectionID]; client != nil {
		return client.BaseURL()
	}
	return s.scopes[connectionID]
}

// readUIState 读出状态文件；v1（0.2.59 及以前）的整份全局状态挂在空键上。
func readUIState(path string) uiStateFile {
	empty := uiStateFile{Version: uiStateVersion, States: map[string]uiStateEntry{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	var probe struct {
		Version int                     `json:"version"`
		States  map[string]uiStateEntry `json:"states"`
		State   json.RawMessage         `json:"state"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return empty
	}
	if probe.Version >= uiStateVersion && probe.States != nil {
		return uiStateFile{Version: uiStateVersion, States: probe.States}
	}
	if len(probe.State) == 0 {
		return empty
	}
	return uiStateFile{
		Version: uiStateVersion,
		States:  map[string]uiStateEntry{"": {SavedAt: time.Now().UTC(), State: probe.State}},
	}
}

func writeUIState(path string, file uiStateFile) *Error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &Error{Code: -32000, Kind: "internal", Message: "internal: 无法创建数据目录"}
	}
	file.Version = uiStateVersion
	payload, err := json.Marshal(file)
	if err != nil {
		return &Error{Code: -32000, Kind: "internal", Message: "internal: 无法序列化插件状态"}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return &Error{Code: -32000, Kind: "internal", Message: "internal: 无法写入插件状态"}
	}
	if err := os.Rename(tmp, path); err != nil {
		return &Error{Code: -32000, Kind: "internal", Message: "internal: 无法保存插件状态"}
	}
	return nil
}

// getUIState 读取当前环境的 UI 状态，缺失或没连上时返回 nil。
func (s *Service) getUIState(request Request) (any, *Error) {
	scope := s.stateScope(request)
	if scope == "" {
		// 不知道当前环境时不做恢复：宁可空页签，也不能端出别的环境的状态。
		return map[string]any{"state": nil}, nil
	}
	path, pathErr := stateFilePath()
	if pathErr != nil {
		return map[string]any{"state": nil}, nil
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	file := readUIState(path)
	entry, ok := file.States[scope]
	if !ok {
		legacy, hasLegacy := file.States[""]
		if !hasLegacy {
			return map[string]any{"state": nil}, nil
		}
		// 升级前的全局状态：由第一个连上来的环境认领，其它环境从空白开始。
		delete(file.States, "")
		legacy.BaseURL = scope
		legacy.SavedAt = time.Now().UTC()
		file.States[scope] = legacy
		_ = writeUIState(path, file)
		entry = legacy
	}
	if len(entry.State) == 0 {
		return map[string]any{"state": nil}, nil
	}
	var state any
	if err := json.Unmarshal(entry.State, &state); err != nil {
		return map[string]any{"state": nil}, nil
	}
	return map[string]any{"state": state}, nil
}

func (s *Service) setUIState(request Request) (any, *Error) {
	if len(request.State) == 0 {
		return nil, invalidParams("state is required")
	}
	if len(request.State) > maxStateRaw {
		return nil, invalidParams("state is too large")
	}
	scope := s.stateScope(request)
	if scope == "" {
		// 还没连上就落盘会写成「全局」状态，等于把 A 环境的页签端给 B 环境。
		return nil, &Error{Code: -32000, Kind: "not_connected", Message: "not_connected: 建立连接后才会保存界面状态"}
	}
	path, pathErr := stateFilePath()
	if pathErr != nil {
		return nil, pathErr
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	file := readUIState(path)
	file.States[scope] = uiStateEntry{BaseURL: scope, SavedAt: time.Now().UTC(), State: request.State}
	pruneUIState(file.States)
	if err := writeUIState(path, file); err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

// pruneUIState 丢掉长期没打开的环境，避免状态文件随环境数量无限增长。
func pruneUIState(states map[string]uiStateEntry) {
	const keep = 20
	if len(states) <= keep {
		return
	}
	type candidate struct {
		key     string
		savedAt time.Time
	}
	oldest := make([]candidate, 0, len(states))
	for key, entry := range states {
		if key == "" {
			continue
		}
		oldest = append(oldest, candidate{key: key, savedAt: entry.SavedAt})
	}
	sort.Slice(oldest, func(i, j int) bool { return oldest[i].savedAt.Before(oldest[j].savedAt) })
	for _, item := range oldest {
		if len(states) <= keep {
			return
		}
		delete(states, item.key)
	}
}

// sessionStore 懒加载会话落盘仓库；数据目录不可用时退化为纯内存会话。
func (s *Service) sessionStore() *archery.SessionStore {
	s.storeOnce.Do(func() {
		if dir := dataDir(); dir != "" {
			s.store = archery.NewFileSessionStore(filepath.Join(dir, "session.json"))
		}
	})
	return s.store
}

func dataDir() string {
	return strings.TrimSpace(os.Getenv("DBX_PLUGIN_DATA_DIR"))
}

func stateFilePath() (string, *Error) {
	dir := dataDir()
	if dir == "" {
		return "", &Error{Code: -32000, Kind: "not_connected", Message: "not_connected: 插件数据目录不可用"}
	}
	return filepath.Join(dir, "ui-state.json"), nil
}

func mapError(err error) *Error {
	var apiErr *archery.APIError
	if errors.As(err, &apiErr) {
		code := -32000
		if apiErr.Kind == archery.KindInvalidConfig {
			code = -32602
		}
		return &Error{
			Code:    code,
			Kind:    string(apiErr.Kind),
			Message: string(apiErr.Kind) + ": " + apiErr.Message,
		}
	}
	if err == nil {
		return nil
	}
	return &Error{Code: -32000, Kind: "internal", Message: "internal: " + err.Error()}
}

func invalidParams(message string) *Error {
	return &Error{Code: -32602, Kind: "invalid_params", Message: "invalid_params: " + message}
}

// downloadNamePattern 限定落盘文件名是不带路径的普通名字，防路径穿越。
var downloadNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// downloadDir 返回本机下载目录；没有 Downloads 时退回用户主目录。
// 变量以便测试替换。
var downloadDir = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		return home, nil
	}
	return dir, nil
}

// promptSavePath 弹出系统「另存为」对话框；变量以便测试替换。
// windows 实现在 savefile_windows.go（comdlg32 GetSaveFileNameW），其余平台为取消桩。
var promptSavePath = func(defaultName, initialDir string) (string, bool, error) {
	return promptSavePathOS(defaultName, initialDir)
}

// saveTextFile 把 UI 生成的文本文件写入下载目录，重名时追加 -1/-2 序号而不是覆盖。
func saveTextFileTo(dir, filename string, data []byte) (string, error) {
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); err == nil {
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		for index := 1; ; index++ {
			candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, index, ext))
			if _, statErr := os.Stat(candidate); os.IsNotExist(statErr) {
				path = candidate
				break
			}
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// saveTextFile 落盘 UI 生成的文本文件（下载回滚SQL 等）。
// 只写本机文件、不触数据库，因此不受只读守卫限制。
// askPath=true 时先弹系统「另存为」对话框（默认名/初始目录来自请求），取消返回 {canceled}。
func (s *Service) saveTextFile(ctx context.Context, request Request) (any, *Error) {
	filename := strings.TrimSpace(request.Filename)
	if filename == "" || !downloadNamePattern.MatchString(filename) {
		return nil, invalidParams("filename must be a plain file name")
	}
	dir, dirErr := downloadDir()
	if dirErr != nil {
		return nil, &Error{Code: -32000, Kind: "internal", Message: "internal: 无法定位下载目录"}
	}

	data := []byte(request.Content)
	// Excel 导出传二进制：JSON 字符串装不下原始字节，按 base64 解码。
	if request.ContentBase64 != "" {
		decoded, decodeErr := base64.StdEncoding.DecodeString(request.ContentBase64)
		if decodeErr != nil {
			return nil, invalidParams("contentBase64 must be valid base64")
		}
		data = decoded
	}
	var path string
	var writeErr error
	if request.AskPath {
		// 初始目录取 UI 记住的上次保存位置；失效时回退下载目录。
		chosen, canceled, promptErr := promptSavePath(filename, resolveSaveDir(request.InitialDir, dir))
		if promptErr != nil {
			return nil, &Error{Code: -32000, Kind: "internal", Message: "internal: 无法打开保存对话框"}
		}
		if canceled || chosen == "" {
			return map[string]any{"canceled": true}, nil
		}
		// 覆盖确认由对话框负责；写入选定的绝对路径。
		if writeErr = os.WriteFile(chosen, data, 0o644); writeErr != nil {
			return nil, &Error{Code: -32000, Kind: "internal", Message: "internal: 无法写入所选路径"}
		}
		return map[string]any{"path": chosen}, nil
	}

	path, writeErr = saveTextFileTo(dir, filename, data)
	if writeErr != nil {
		return nil, &Error{Code: -32000, Kind: "internal", Message: "internal: 无法写入下载目录"}
	}
	return map[string]any{"path": path}, nil
}

func configString(config map[string]any, key, def string) string {
	if config == nil {
		return def
	}
	value, ok := config[key]
	if !ok || value == nil {
		return def
	}
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return def
	}
	return text
}

func configBool(config map[string]any, key string, def bool) bool {
	if config == nil {
		return def
	}
	value, ok := config[key]
	if !ok || value == nil {
		return def
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	case float64:
		return typed != 0
	}
	return def
}

func configInt(config map[string]any, key string, def int) int {
	if config == nil {
		return def
	}
	value, ok := config[key]
	if !ok || value == nil {
		return def
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return def
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func timeSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	if seconds > 600 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}
