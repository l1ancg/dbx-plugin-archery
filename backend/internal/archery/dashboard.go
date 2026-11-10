package archery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DashboardCounts 是 Archery 首页四个指标卡的全量计数。
// Archery 没有为这四个数字提供 JSON 接口，只能从 /dashboard/ 的 HTML 里取。
type DashboardCounts struct {
	SQLWorkflow int `json:"sqlWorkflow"`
	SQLQuery    int `json:"sqlQuery"`
	ActiveUsers int `json:"activeUsers"`
	Instances   int `json:"instances"`
}

// InstanceTypeStats 是首页「数据库实例类型统计」面板里的数据：
// 柱状图给「环境 × 数据库类型」，环图给「数据库类型」总量。
// 首页统计的是全库实例，和当前账号可查询的实例清单不是一回事，因此单独解析。
type InstanceTypeStats struct {
	DBTypes []string    `json:"dbTypes"`
	Envs    []string    `json:"envs"`
	Series  []EnvSeries `json:"series"`
	Types   []TypeCount `json:"types"`
}

// EnvSeries 是某个环境在各数据库类型上的实例数（顺序与 DBTypes 一致）。
type EnvSeries struct {
	Env    string `json:"env"`
	Values []int  `json:"values"`
}

// TypeCount 是某个数据库类型的实例总数。
type TypeCount struct {
	DBType string `json:"dbType"`
	Count  int    `json:"count"`
}

// DashboardTrends 是首页两张按日趋势图的数据：
// 「SQL查询统计」的双折线（检索行数 / 检索次数）与「SQL上线数量」的柱状数据。
type DashboardTrends struct {
	Dates      []string      `json:"dates"`
	Query      []TrendSeries `json:"query"`
	Release    []int         `json:"release"`
	RangeLabel string        `json:"rangeLabel"`
}

// TrendSeries 是一条按日折线：Name 为图例名，Values 与 Dates 一一对应。
type TrendSeries struct {
	Name   string `json:"name"`
	Values []int  `json:"values"`
}

// Announcement 是 Archery 首页的系统公告，模板渲染为 alert 块。
type Announcement struct {
	Level   string `json:"level"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Brand 是 Archery 导航栏品牌：模板固定输出 Archery + custom_title_suffix。
// 例如 Name=Archery、Suffix=(广州跨境&其他测试环境)，取不到时两者都为空。
type Brand struct {
	Name   string `json:"name"`
	Suffix string `json:"suffix"`
}

var (
	// 公告块：class 含 alert 的第一个 div，内容取到紧随的 </div>。
	alertPattern      = regexp.MustCompile(`(?is)<div[^>]*class\s*=\s*"[^"]*\balert\b[^"]*"[^>]*>(.*?)</div>`)
	alertLevelPattern = regexp.MustCompile(`(?is)\balert-([a-z]+)\b`)
	headingPattern    = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	buttonPattern     = regexp.MustCompile(`(?is)<button[^>]*>.*?</button>`)
	tagPattern        = regexp.MustCompile(`(?s)<[^>]+>`)
	spacePattern      = regexp.MustCompile(`\s+`)
	entityReplacer    = strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	)
)

// ParseAnnouncement 从 /dashboard/ 的 HTML 中提取第一条 alert 公告。
// 取不到时返回 nil，UI 直接不显示公告条。
func ParseAnnouncement(html []byte) *Announcement {
	if len(html) == 0 {
		return nil
	}
	match := alertPattern.FindSubmatch(html)
	if len(match) < 2 {
		return nil
	}
	level := "info"
	if value := alertLevelPattern.FindSubmatch(match[0]); len(value) > 1 {
		level = strings.ToLower(string(value[1]))
	}
	body := buttonPattern.ReplaceAll(match[1], []byte(" "))
	title := ""
	if heading := headingPattern.FindSubmatch(body); len(heading) > 1 {
		title = stripHTML(string(heading[1]))
		body = headingPattern.ReplaceAll(body, []byte(" "))
	}
	content := truncateRunes(stripHTML(string(body)), 400)
	if title == "" && content == "" {
		return nil
	}
	return &Announcement{Level: level, Title: title, Content: content}
}

// 首页图表脚本：var option_<hash> = { ... };，option 本身是合法 JSON。
var (
	instanceTypePanelPattern = regexp.MustCompile(`(?is)数据库实例类型统计`)
	optionScriptPattern      = regexp.MustCompile(`(?is)var\s+option_[0-9a-zA-Z]+\s*=\s*`)
	queryTrendHeading        = regexp.MustCompile(`(?is)SQL查询统计`)
	releaseTrendHeading      = regexp.MustCompile(`(?is)SQL上线数量`)
	// 首页顶部「统计时间范围：2026-10-29 - 2026-11-05」
	trendRangePattern = regexp.MustCompile(`(?is)id\s*=\s*"reservation"[^>]*>.*?<span>\s*([^<]+?)\s*</span>`)
)

type chartSeries struct {
	Type string          `json:"type"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type chartOption struct {
	Series []chartSeries `json:"series"`
	XAxis  []struct {
		Data []string `json:"data"`
	} `json:"xAxis"`
}

// ParseInstanceTypeStats 从首页「数据库实例类型统计」面板解析出柱状图与环图数据。
// 两块都解析成功才返回，避免半张图与指标卡对不上；取不到时 UI 退回本地实例清单。
func ParseInstanceTypeStats(html []byte) *InstanceTypeStats {
	heading := instanceTypePanelPattern.FindIndex(html)
	if heading == nil {
		return nil
	}
	tail := html[heading[1]:]

	var bar, pie *chartOption
	for _, loc := range optionScriptPattern.FindAllIndex(tail, 8) {
		decoder := json.NewDecoder(bytes.NewReader(tail[loc[1]:]))
		var option chartOption
		if err := decoder.Decode(&option); err != nil {
			continue
		}
		switch {
		case bar == nil && option.hasBarData():
			copied := option
			bar = &copied
		case pie == nil && option.hasPieData():
			copied := option
			pie = &copied
		}
		if bar != nil && pie != nil {
			break
		}
	}
	if bar == nil || pie == nil {
		return nil
	}

	stats := &InstanceTypeStats{DBTypes: bar.XAxis[0].Data}
	for _, item := range bar.Series {
		if item.Type != "bar" {
			continue
		}
		var values []int
		if err := json.Unmarshal(item.Data, &values); err != nil {
			continue
		}
		stats.Envs = append(stats.Envs, item.Name)
		stats.Series = append(stats.Series, EnvSeries{Env: item.Name, Values: values})
	}
	for _, item := range pie.Series {
		if item.Type != "pie" {
			continue
		}
		var slices []struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}
		if err := json.Unmarshal(item.Data, &slices); err != nil {
			continue
		}
		for _, slice := range slices {
			stats.Types = append(stats.Types, TypeCount{DBType: slice.Name, Count: slice.Value})
		}
		break
	}
	if len(stats.Series) == 0 || len(stats.Types) == 0 {
		return nil
	}
	return stats
}

// ParseDashboardTrends 解析首页两张按日统计图：柱状图给上线数量，折线图给检索行数/次数。
// 两张都取不到时返回 nil，UI 保留占位卡。
func ParseDashboardTrends(html []byte) *DashboardTrends {
	trends := &DashboardTrends{}
	if label := trendRangePattern.FindSubmatch(html); len(label) == 2 {
		trends.RangeLabel = stripHTML(string(label[1]))
	}

	if line := chartOptionAfterHeading(html, queryTrendHeading); line != nil {
		trends.Dates, trends.Query = lineTrend(line)
	}

	if bar := chartOptionAfterHeading(html, releaseTrendHeading); bar != nil {
		values, dates := barTrend(bar)
		trends.Release = values
		if len(trends.Dates) == 0 {
			trends.Dates = dates
		}
	}

	if len(trends.Query) == 0 && len(trends.Release) == 0 {
		return nil
	}
	return trends
}

// ParseTrendFragments 从 /dashboard/api/ 返回的 chart 片段里取两张按日图。
// 片段是 pyecharts 的 render_embed() HTML，结构与首页一致（var option_* = {...}）。
func ParseTrendFragments(charts map[string]string, startDate, endDate string) *DashboardTrends {
	trends := &DashboardTrends{}
	if startDate != "" && endDate != "" {
		trends.RangeLabel = startDate + " - " + endDate
	}
	if option := chartOptionFromFragment(charts["line1"]); option != nil {
		trends.Dates, trends.Query = lineTrend(option)
	}
	if option := chartOptionFromFragment(charts["bar1"]); option != nil {
		values, dates := barTrend(option)
		trends.Release = values
		if len(trends.Dates) == 0 {
			trends.Dates = dates
		}
	}
	if len(trends.Query) == 0 && len(trends.Release) == 0 {
		return nil
	}
	return trends
}

// chartOptionFromFragment 取片段里第一份能解码且带 series 的 ECharts option。
func chartOptionFromFragment(fragment string) *chartOption {
	if fragment == "" {
		return nil
	}
	raw := []byte(fragment)
	for _, span := range optionScriptPattern.FindAllIndex(raw, 4) {
		decoder := json.NewDecoder(bytes.NewReader(raw[span[1]:]))
		var option chartOption
		if err := decoder.Decode(&option); err != nil {
			continue
		}
		if len(option.Series) > 0 {
			return &option
		}
	}
	return nil
}

// lineTrend 同时支持 [[日期, 值], ...] 与「纯数值 + xAxis.data 提供日期」两种序列结构。
func lineTrend(option *chartOption) ([]string, []TrendSeries) {
	var dates []string
	series := make([]TrendSeries, 0, len(option.Series))
	for _, item := range option.Series {
		if item.Type != "line" {
			continue
		}
		var pairs [][]any
		if err := json.Unmarshal(item.Data, &pairs); err == nil && len(pairs) > 0 && len(pairs[0]) == 2 {
			values := make([]int, 0, len(pairs))
			points := make([]string, 0, len(pairs))
			for _, pair := range pairs {
				points = append(points, chartText(pair[0]))
				values = append(values, chartInt(pair[1]))
			}
			if len(dates) == 0 {
				dates = points
			}
			series = append(series, TrendSeries{Name: item.Name, Values: values})
			continue
		}
		var values []int
		if err := json.Unmarshal(item.Data, &values); err != nil {
			continue
		}
		if len(dates) == 0 && len(option.XAxis) > 0 {
			dates = option.XAxis[0].Data
		}
		series = append(series, TrendSeries{Name: item.Name, Values: values})
	}
	return dates, series
}

// barTrend 取出柱状序列与日期轴，返回顺序为 (值, 日期)。
func barTrend(option *chartOption) ([]int, []string) {
	dates := []string(nil)
	if len(option.XAxis) > 0 {
		dates = option.XAxis[0].Data
	}
	for _, item := range option.Series {
		if item.Type != "bar" {
			continue
		}
		var values []int
		if err := json.Unmarshal(item.Data, &values); err == nil {
			return values, dates
		}
		var pairs [][]any
		if err := json.Unmarshal(item.Data, &pairs); err == nil && len(pairs) > 0 {
			points := make([]string, 0, len(pairs))
			out := make([]int, 0, len(pairs))
			for _, pair := range pairs {
				if len(pair) < 2 {
					continue
				}
				points = append(points, chartText(pair[0]))
				out = append(out, chartInt(pair[1]))
			}
			return out, points
		}
	}
	return nil, dates
}

// chartOptionAfterHeading 取标题后面第一份能解码且带 series 的 ECharts option。
func chartOptionAfterHeading(html []byte, heading *regexp.Regexp) *chartOption {
	loc := heading.FindIndex(html)
	if loc == nil {
		return nil
	}
	tail := html[loc[1]:]
	for _, span := range optionScriptPattern.FindAllIndex(tail, 4) {
		decoder := json.NewDecoder(bytes.NewReader(tail[span[1]:]))
		var option chartOption
		if err := decoder.Decode(&option); err != nil {
			continue
		}
		if len(option.Series) > 0 {
			return &option
		}
	}
	return nil
}

func chartText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func chartInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(typed); err == nil {
			return parsed
		}
	}
	return 0
}

func (o chartOption) hasBarData() bool {
	if len(o.XAxis) == 0 || len(o.XAxis[0].Data) == 0 {
		return false
	}
	for _, item := range o.Series {
		if item.Type == "bar" {
			return true
		}
	}
	return false
}

func (o chartOption) hasPieData() bool {
	for _, item := range o.Series {
		if item.Type == "pie" {
			return true
		}
	}
	return false
}

func stripHTML(raw string) string {
	text := tagPattern.ReplaceAllString(raw, " ")
	text = entityReplacer.Replace(text)
	return strings.TrimSpace(spacePattern.ReplaceAllString(text, " "))
}

func truncateRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}

// 首选完整 span 结构；模板改版时用宽松模式兜底。
var (
	infoBoxSpanPattern = regexp.MustCompile(`(?is)<span[^>]*class\s*=\s*"[^"]*info-box-number[^"]*"[^>]*>(.*?)</span>`)
	infoBoxLoosePattern = regexp.MustCompile(`(?is)info-box-number[^>]*>\s*([0-9][0-9,]*)`)
	// 文案 + 数字成对出现，用来按文案归类而不是按顺序取数。
	infoBoxPairPattern = regexp.MustCompile(`(?is)<span[^>]*class\s*=\s*"[^"]*info-box-text[^"]*"[^>]*>(.*?)</span>\s*<span[^>]*class\s*=\s*"[^"]*info-box-number[^"]*"[^>]*>(.*?)</span>`)
	// Archery 1.14 首页的真实结构是 AdminLTE 的 panel 版指标卡：
	// <div class="huge">2244</div> 后面紧跟一个无 class 的 <div>SQL上线工单</div>。
	hugePairPattern = regexp.MustCompile(`(?is)<div[^>]*class\s*=\s*"[^"]*\bhuge\b[^"]*"[^>]*>(.*?)</div>\s*<div[^>]*>(.*?)</div>`)
	// 另有 small-box 版本：数字在 h3、文案在 p。两种都要认，只认 info-box 会一个数字也取不到。
	smallBoxPattern       = regexp.MustCompile(`(?is)<div[^>]*class\s*=\s*"[^"]*\bsmall-box\b[^"]*"[^>]*>(.*?)</div>`)
	smallBoxNumberPattern = regexp.MustCompile(`(?is)<h3[^>]*>(.*?)</h3>`)
	smallBoxLabelPattern  = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	numberPattern         = regexp.MustCompile(`[0-9][0-9,]*`)
	brandPattern          = regexp.MustCompile(`(?is)<a[^>]*class\s*=\s*"[^"]*\bnavbar-brand\b[^"]*"[^>]*>(.*?)</a>`)
)

// dashboardInfoCard 是首页的一张统计卡：文案 + 数字。
type dashboardInfoCard struct {
	Label string
	Value int
}

// parseDashboardCards 提取「文案 + 数字」成对的统计卡，三种模板都支持：
// AdminLTE panel（huge + 无 class 的 div）、small-box（h3 + p）与 info-box（text + number）。
func parseDashboardCards(html []byte) []dashboardInfoCard {
	if cards := parseHugeCards(html); len(cards) > 0 {
		return cards
	}
	if cards := parseSmallBoxCards(html); len(cards) > 0 {
		return cards
	}
	cards := make([]dashboardInfoCard, 0, 6)
	for _, match := range infoBoxPairPattern.FindAllSubmatch(html, -1) {
		value, ok := parseDashboardNumber(match[2])
		if !ok {
			continue
		}
		cards = append(cards, dashboardInfoCard{Label: stripHTML(string(match[1])), Value: value})
	}
	return cards
}

// parseHugeCards 解析 Archery 1.14 首页实际使用的 panel 版指标卡。
func parseHugeCards(html []byte) []dashboardInfoCard {
	cards := make([]dashboardInfoCard, 0, 6)
	for _, match := range hugePairPattern.FindAllSubmatch(html, -1) {
		value, ok := parseDashboardNumber(match[1])
		if !ok {
			continue
		}
		cards = append(cards, dashboardInfoCard{Label: stripHTML(string(match[2])), Value: value})
	}
	return cards
}

func parseSmallBoxCards(html []byte) []dashboardInfoCard {
	cards := make([]dashboardInfoCard, 0, 6)
	for _, block := range smallBoxPattern.FindAllSubmatch(html, -1) {
		number := smallBoxNumberPattern.FindSubmatch(block[1])
		label := smallBoxLabelPattern.FindSubmatch(block[1])
		if number == nil || label == nil {
			continue
		}
		value, ok := parseDashboardNumber(number[1])
		if !ok {
			continue
		}
		cards = append(cards, dashboardInfoCard{Label: stripHTML(string(label[1])), Value: value})
	}
	return cards
}

// classifyDashboardCard 按文案把卡片归到四个指标；认不出来的卡片（待办、审批等）返回空串。
func classifyDashboardCard(label string) string {
	text := strings.ToLower(label)
	switch {
	case strings.Contains(text, "查询"), strings.Contains(text, "query"):
		return "query"
	case strings.Contains(text, "上线"), strings.Contains(text, "release"):
		return "workflow"
	case strings.Contains(text, "用户"), strings.Contains(text, "user"):
		return "users"
	case strings.Contains(text, "实例"), strings.Contains(text, "instance"):
		return "instances"
	}
	return ""
}

// ParseBrand 从 /dashboard/ 的 HTML 里取导航栏品牌文本。
// 模板为 <a class="navbar-brand" ...><strong>Archery{{ custom_title_suffix }}</strong></a>，
// 取不到时返回零值，UI 退回连接名。
func ParseBrand(html []byte) Brand {
	match := brandPattern.FindSubmatch(html)
	if len(match) < 2 {
		return Brand{}
	}
	text := stripHTML(string(match[1]))
	if text == "" {
		return Brand{}
	}
	const prefix = "Archery"
	if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
		return Brand{Name: text[:len(prefix)], Suffix: strings.TrimSpace(text[len(prefix):])}
	}
	return Brand{Name: text}
}

// ParseDashboardCounts 从 /dashboard/ 的 HTML 中提取四个指标。
// 优先按卡片文案归类：模板顺序变化或首页多出别的 info-box（待办、审批等）时，
// 按位置取前四个数字会整体错位，四张卡就全对不上了。
func ParseDashboardCounts(html []byte) (DashboardCounts, error) {
	if counts, ok := countsByLabel(html); ok {
		return counts, nil
	}
	// 文案认不全（模板改版 / 本地化文案）时退回按顺序取数，保持旧行为。
	values := dashboardValues(html)
	if len(values) < 4 {
		return DashboardCounts{}, fmt.Errorf("dashboard counters not found: want 4, got %d", len(values))
	}
	return DashboardCounts{
		SQLWorkflow: values[0],
		SQLQuery:    values[1],
		ActiveUsers: values[2],
		Instances:   values[3],
	}, nil
}

// dashboardValues 按页面顺序取数字：优先用成对卡片（small-box / info-box），
// 再退回只认 info-box-number 的旧路径。
func dashboardValues(html []byte) []int {
	cards := parseDashboardCards(html)
	if len(cards) >= 4 {
		values := make([]int, 0, len(cards))
		for _, card := range cards {
			values = append(values, card.Value)
		}
		return values
	}
	return dashboardNumbers(html)
}

// countsByLabel 把「文案 + 数字」配对后按文案归类，四个指标齐全才算成功。
func countsByLabel(html []byte) (DashboardCounts, bool) {
	counts := DashboardCounts{}
	seen := make(map[string]bool, 4)
	for _, card := range parseDashboardCards(html) {
		key := classifyDashboardCard(card.Label)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch key {
		case "workflow":
			counts.SQLWorkflow = card.Value
		case "query":
			counts.SQLQuery = card.Value
		case "users":
			counts.ActiveUsers = card.Value
		case "instances":
			counts.Instances = card.Value
		}
	}
	return counts, len(seen) == 4
}

func dashboardNumbers(html []byte) []int {
	values := make([]int, 0, 4)
	for _, match := range infoBoxSpanPattern.FindAllSubmatch(html, -1) {
		if value, ok := parseDashboardNumber(match[1]); ok {
			values = append(values, value)
		}
	}
	if len(values) >= 4 {
		return values
	}
	// 模板结构未知时退回宽松匹配，避免整块能力直接失效。
	values = values[:0]
	for _, match := range infoBoxLoosePattern.FindAllSubmatch(html, -1) {
		if value, ok := parseDashboardNumber(match[1]); ok {
			values = append(values, value)
		}
	}
	return values
}

func parseDashboardNumber(raw []byte) (int, bool) {
	digits := numberPattern.FindString(string(raw))
	if digits == "" {
		return 0, false
	}
	value, err := strconv.Atoi(strings.ReplaceAll(digits, ",", ""))
	if err != nil {
		return 0, false
	}
	return value, true
}
