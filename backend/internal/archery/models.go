package archery

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Instance 是当前用户通过资源组可访问的 Archery 实例。
type Instance struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	DBType string `json:"dbType"`
}

// Resource 是实例下的一个库/表/列，Values 保留原始行便于 UI 展示备注等列。
type Resource struct {
	Name   string `json:"name"`
	Values []any  `json:"values"`
}

// DictionaryTable 是数据字典的一行：表名与注释各自独立成字段，
// 不借用资源行的位置数组，前端可以直接按字段渲染。
type DictionaryTable struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

// TableGrid 是 Archery 表元数据里的一个表格块（列名 + 位置行）。
type TableGrid struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// TableMetadata 对应 Archery /data_dictionary/table_info/ 的四块内容。
type TableMetadata struct {
	Meta      TableGrid `json:"meta"`
	Desc      TableGrid `json:"desc"`
	Index     TableGrid `json:"index"`
	CreateSQL string    `json:"createSql"`
}

// DescribeResult 是表结构。
// MySQL 系引擎的 describe 走 SHOW CREATE TABLE，rows[0][-1] 就是建表语句；
// 这类响应填 Table/DDL，UI 直接展示 DDL，否则回退到列清单表格。
type DescribeResult struct {
	Columns []string `json:"columns"`
	Types   []string `json:"types"`
	Rows    [][]any  `json:"rows"`
	Table   string   `json:"table,omitempty"`
	DDL     string   `json:"ddl,omitempty"`
}

// QueryRequest 是一次查询的全部参数。
type QueryRequest struct {
	InstanceName string
	DBName       string
	SchemaName   string
	TBName       string
	SQL          string
	LimitNum     int
}

// QueryResult 是归一化后的查询结果，行列均为位置数组。
type QueryResult struct {
	Columns             []string `json:"columns"`
	ColumnTypes         []string `json:"columnTypes"`
	Rows                [][]any  `json:"rows"`
	RowCount            int      `json:"rowCount"`
	AffectedRows        int64    `json:"affectedRows"`
	ElapsedMS           int64    `json:"elapsedMs"`
	Masked              bool     `json:"masked"`
	MaskRuleHit         bool     `json:"maskRuleHit"`
	Warning             string   `json:"warning,omitempty"`
	SecondsBehindMaster string   `json:"secondsBehindMaster,omitempty"`
}

// QueryLog 是一条查询历史。
type QueryLog struct {
	ID           int64  `json:"id"`
	InstanceName string `json:"instanceName"`
	DBName       string `json:"dbName"`
	SQL          string `json:"sql"`
	EffectRow    int64  `json:"effectRow"`
	CostTime     string `json:"costTime"`
	UserDisplay  string `json:"userDisplay"`
	Favorite     bool   `json:"favorite"`
	Alias        string `json:"alias"`
	CreateTime   string `json:"createTime"`
}

// QueryLogPage 是分页后的查询历史。
type QueryLogPage struct {
	Total int        `json:"total"`
	Rows  []QueryLog `json:"rows"`
}

// LogQuery 是查询历史的筛选条件。
type LogQuery struct {
	Limit        int
	Offset       int
	Search       string
	Star         bool
	QueryLogID   int
	StartDate    string
	EndDate      string
	InstanceName string
	DBName       string
}

// normalizeRows 把 Archery 的两种行格式（位置数组 / 对象）统一成位置数组。
// 对象行里的额外键会按字典序追加到列尾，避免静默丢字段。
func normalizeRows(raw []any, columns []string) ([]string, [][]any) {
	cols := append([]string(nil), columns...)
	extra := map[string]struct{}{}
	rows := make([][]any, 0, len(raw))
	for _, item := range raw {
		switch value := item.(type) {
		case []any:
			rows = append(rows, value)
		case map[string]any:
			row := make([]any, len(cols))
			for i, key := range cols {
				row[i] = value[key]
			}
			for key := range value {
				known := false
				for _, col := range cols {
					if col == key {
						known = true
						break
					}
				}
				if !known {
					if _, seen := extra[key]; !seen {
						extra[key] = struct{}{}
					}
				}
			}
			rows = append(rows, row)
		default:
			rows = append(rows, []any{item})
		}
	}
	if len(extra) > 0 {
		keys := make([]string, 0, len(extra))
		for key := range extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		cols = append(cols, keys...)
		for i, item := range raw {
			value, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range keys {
				rows[i] = append(rows[i], value[key])
			}
		}
	}
	return cols, rows
}

// stringList 把 JSON 数组转成字符串切片，容忍数字与 null。
func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, toText(item))
	}
	return out
}

func toText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func toInt64(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return n
		}
		if f, err := typed.Float64(); err == nil {
			return int64(math.Round(f))
		}
	case float64:
		return int64(math.Round(typed))
	case int:
		return int64(typed)
	case int64:
		return typed
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func toBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	case json.Number:
		return typed.String() != "0"
	case float64:
		return typed != 0
	}
	return false
}

// secondsToMillis 把 "0.03" 这类耗时字符串转成毫秒。
func secondsToMillis(value any) int64 {
	text := toText(value)
	if text == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(text, 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return int64(math.Round(seconds * 1000))
}

// 查询权限级别，对应 Archery 的 priv_type。
const (
	PrivilegeTypeDatabase = 1
	PrivilegeTypeTable    = 2
)

// 工单审核动作，对应 Archery 的 WorkflowAction。
const (
	AuditPass   = 1
	AuditReject = 2
)

// WorkflowTypeQueryPriv 是查询权限工单的审批流类型。
const WorkflowTypeQueryPriv = 1

// PrivilegeApply 是一条查询权限申请工单。
type PrivilegeApply struct {
	ApplyID      int64  `json:"applyId"`
	Title        string `json:"title"`
	InstanceName string `json:"instanceName"`
	DBList       string `json:"dbList"`
	PrivType     int    `json:"privType"`
	TableList    string `json:"tableList"`
	LimitNum     int    `json:"limitNum"`
	ValidDate    string `json:"validDate"`
	UserDisplay  string `json:"userDisplay"`
	Status       int    `json:"status"`
	CreateTime   string `json:"createTime"`
	GroupName    string `json:"groupName"`
}

// PrivilegeApplyPage 是分页后的权限申请。
type PrivilegeApplyPage struct {
	Total int              `json:"total"`
	Rows  []PrivilegeApply `json:"rows"`
}

// UserPrivilege 是「我的权限」里的一条查询权限记录。
// 字段来自 Archery /query/userprivileges/ 的 values()，不含工单与审批信息。
type UserPrivilege struct {
	PrivilegeID  int64  `json:"privilegeId"`
	UserDisplay  string `json:"userDisplay"`
	InstanceName string `json:"instanceName"`
	DBName       string `json:"dbName"`
	PrivType     int    `json:"privType"`
	TableName    string `json:"tableName"`
	LimitNum     int    `json:"limitNum"`
	ValidDate    string `json:"validDate"`
}

// UserPrivilegePage 是分页后的「我的权限」。
type UserPrivilegePage struct {
	Total int             `json:"total"`
	Rows  []UserPrivilege `json:"rows"`
}

// GroupInstance 是资源组内可申请的实例。
type GroupInstance struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	DBType string `json:"dbType"`
}

// PrivilegeApplyInput 是提交权限申请的全部参数。
type PrivilegeApplyInput struct {
	Title        string
	GroupName    string
	InstanceName string
	PrivType     int
	DBName       string
	DBList       []string
	TableList    []string
	ValidDate    string
	LimitNum     int
}

// PrivilegeReviewNode 是工单详情页审批流上的一个节点。
// State 取值：auto（无需审批）/ passed（已通过）/ current（当前节点）/ pending（未到达）。
type PrivilegeReviewNode struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Reviewers []string `json:"reviewers"`
}

// PrivilegeDetail 是权限工单详情页的解析结果。
// 任一区块解析不到都留空，由 UI 回退到列表行数据，不让详情页整页失败。
type PrivilegeDetail struct {
	ApplyID       int64                 `json:"applyId"`
	Title         string                `json:"title"`
	CanReview     bool                  `json:"canReview"`
	ReviewNodes   []PrivilegeReviewNode `json:"reviewNodes"`
	Items         []string              `json:"items"`
	ItemKind      string                `json:"itemKind"`
	LastOperation string                `json:"lastOperation"`
}

// WorkflowLog 是一条工单操作日志。
type WorkflowLog struct {
	OperationType string `json:"operationType"`
	Operator      string `json:"operator"`
	Time          string `json:"time"`
	Info          string `json:"info"`
}

// WorkflowLogPage 是分页后的工单日志。
type WorkflowLogPage struct {
	Total int           `json:"total"`
	Rows  []WorkflowLog `json:"rows"`
}

// WorkflowTypeArchive 是数据归档工单的审批流类型（WorkflowType.ARCHIVE）。
const WorkflowTypeArchive = 3

// WorkflowTodo 是待办列表的一行（Archery /workflow/list/ 的 values 行）。
// 只含审批记录自身字段，业务工单号需要再解析详情去向。
type WorkflowTodo struct {
	AuditID      int64  `json:"auditId"`
	WorkflowType int    `json:"workflowType"`
	Title        string `json:"title"`
	Applicant    string `json:"applicant"`
	CreateTime   string `json:"createTime"`
	Status       int    `json:"status"`
	GroupName    string `json:"groupName"`
	CurrentAudit string `json:"currentAudit"`
	AuditGroups  string `json:"auditGroups"`
}

// WorkflowTodoPage 是分页后的待办列表。
type WorkflowTodoPage struct {
	Total int            `json:"total"`
	Rows  []WorkflowTodo `json:"rows"`
}

// WorkflowTodoQuery 是待办列表的筛选条件；WorkflowType 为 0 表示全部类型。
type WorkflowTodoQuery struct {
	Limit        int
	Offset       int
	Search       string
	WorkflowType int
}

// WorkflowTodoTarget 是待办对应的业务工单：
// Archery 的 /workflow/<audit_id>/ 会 302 到对应详情页，由跳转地址反推类型与工单号。
type WorkflowTodoTarget struct {
	AuditID      int64 `json:"auditId"`
	WorkflowID   int64 `json:"workflowId"`
	WorkflowType int   `json:"workflowType"`
}

// WorkflowTypeSqlReview 是 SQL 上线工单的审批流类型（WorkflowType.SQL_REVIEW）。
const WorkflowTypeSqlReview = 2

// SQL 上线工单的审核动作，对应 Archery 的 /passed/ 与 /cancel/。
const (
	WorkflowActionPass   = "pass"
	WorkflowActionReject = "reject"
)

// SQL 上线工单的执行方式：auto=平台排队执行，manual=线下手工执行。
const (
	WorkflowModeAuto   = "auto"
	WorkflowModeManual = "manual"
)

// SqlWorkflow 是一条 SQL 上线工单（/sqlworkflow_list/ 的 values 行）。
// 状态是 workflow_* 字符串，syntax_type 是整数（0 其他 / 1 DDL / 2 DML / 3 离线导出）。
type SqlWorkflow struct {
	ID              int64  `json:"id"`
	WorkflowName    string `json:"workflowName"`
	EngineerDisplay string `json:"engineerDisplay"`
	Status          string `json:"status"`
	IsBackup        bool   `json:"isBackup"`
	CreateTime      string `json:"createTime"`
	InstanceName    string `json:"instanceName"`
	DBName          string `json:"dbName"`
	GroupName       string `json:"groupName"`
	SyntaxType      int    `json:"syntaxType"`
	ExportFormat    string `json:"exportFormat"`
}

// SqlWorkflowPage 是分页后的 SQL 上线工单。
type SqlWorkflowPage struct {
	Total int           `json:"total"`
	Rows  []SqlWorkflow `json:"rows"`
}

// SqlWorkflowQuery 是列表筛选条件；Archery 只在起止日期同时存在时按时间过滤。
type SqlWorkflowQuery struct {
	Limit       int
	Offset      int
	Search      string
	Status      string
	InstanceID  int
	GroupID     int
	StartDate   string
	EndDate     string
	SyntaxTypes []string
	// Audit 为真时只返回当前需要我审核的工单：
	// 由 /workflow/list/ 的审批队列与待审核工单求交集，不再请求 /sqlworkflow_list_audit/。
	Audit bool
}

// SqlWorkflowOption 是筛选下拉的一项：id 对应 instance_id / group_id。
type SqlWorkflowOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SqlWorkflowOptions 是列表页的实例与资源组下拉。
type SqlWorkflowOptions struct {
	Instances []SqlWorkflowOption `json:"instances"`
	Groups    []SqlWorkflowOption `json:"groups"`
}

// SqlSubmitOptions 是提交弹窗的组下拉与备份开关（取自 /submitsql/ 页面）。
type SqlSubmitOptions struct {
	Groups        []SqlWorkflowOption `json:"groups"`
	BackupEnabled bool                `json:"backupEnabled"`
}

// SqlReviewRow 是工单内容里的一条 SQL 审核/执行结果。
type SqlReviewRow struct {
	Sequence     string `json:"sequence"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	ErrorLevel   int    `json:"errorLevel"`
	ErrorMessage string `json:"errorMessage"`
	SQL          string `json:"sql"`
	AffectedRows int64  `json:"affectedRows"`
	BackupDBName string `json:"backupDbName"`
	ExecuteTime  string `json:"executeTime"`
}

// InfoField 是详情页「其他信息」表格的一列。
// 列名与顺序由 Archery 决定，前端按关键词映射本地化标题。
type InfoField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// SqlRollbackRow 是一条回滚明细：执行语句与对应的回滚语句。
// goInception 引擎按执行顺序倒序返回，先执行的语句最后回滚。
type SqlRollbackRow struct {
	ID          int    `json:"id"`
	SQL         string `json:"sql"`
	RollbackSQL string `json:"rollbackSql"`
}

// SqlRollbackResult 是工单回滚 SQL：Rows 为逐条明细，SQL 为拼好的整段回滚文本（下载用）。
type SqlRollbackResult struct {
	Rows []SqlRollbackRow `json:"rows"`
	SQL  string           `json:"sql"`
}

// SqlWorkflowDetail 是工单详情：审批流与操作信息取详情页 HTML，
// SQL 审核/执行结果取 /sqlworkflow/detail_content/。
type SqlWorkflowDetail struct {
	WorkflowID    int64                 `json:"workflowId"`
	Title         string                `json:"title"`
	Status        string                `json:"status"`
	ReviewNodes   []PrivilegeReviewNode `json:"reviewNodes"`
	InfoFields    []InfoField           `json:"infoFields"`
	LastOperation string                `json:"lastOperation"`
	ReviewRows    []SqlReviewRow        `json:"reviewRows"`
}

// SqlWorkflowSubmitInput 是提交 SQL 上线工单的参数。
type SqlWorkflowSubmitInput struct {
	Title        string
	DemandURL    string
	GroupID      int
	InstanceID   int
	DBName       string
	SQL          string
	IsBackup     bool
	RunDateStart string
	RunDateEnd   string
}

// SqlCheckSummary 是提交前的 SQL 检测结果汇总。
type SqlCheckSummary struct {
	ErrorCount   int            `json:"errorCount"`
	WarningCount int            `json:"warningCount"`
	Rows         []SqlReviewRow `json:"rows"`
}
