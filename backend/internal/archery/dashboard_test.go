package archery

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func dashboardCard(label, value string) string {
	return `<div class="col-md-3 col-sm-6 col-xs-12">
        <div class="info-box">
            <span class="info-box-icon bg-blue"><i class="fa fa-check"></i></span>
            <div class="info-box-content">
                <span class="info-box-text">` + label + `</span>
                <span class="info-box-number">` + value + `</span>
            </div>
        </div>
    </div>`
}

// smallBox 按 Archery 首页（AdminLTE dashboard.html）的真实结构构造：
// 数字在 h3、文案在 p、底部「更多」，整块没有 info-box-number。
func smallBox(label, value, color string) string {
	return `<div class="col-lg-3 col-xs-6">
        <div class="small-box bg-` + color + `">
            <div class="inner">
                <h3>` + value + `</h3>
                <p>` + label + `</p>
            </div>
            <a href="/sqlworkflow/" class="small-box-footer">更多 <i class="fa fa-arrow-circle-right"></i></a>
        </div>
    </div>`
}

// dashboardPanel 按 Archery 1.14 首页的真实结构构造：AdminLTE 的 panel 版指标卡，
// 数字在 div.huge、文案是紧随其后的一个无 class 的 div，既不是 small-box 也不是 info-box。
func dashboardPanel(label, value, color string) string {
	return `<div class="col-lg-3 col-md-6">
        <div class="panel panel-` + color + `">
            <div class="panel-heading">
                <div class="row">
                    <div class="col-xs-3">
                        <i class="fa fa-check fa-5x"></i>
                    </div>
                    <div class="col-xs-9 text-right">
                        <div class="huge">` + value + `</div>
                        <div>` + label + `</div>
                    </div>
                </div>
            </div>
            <a href="/sqlworkflow/">
                <div class="panel-footer">
                    <span class="pull-left">更多</span>
                    <span class="pull-right"><i class="fa fa-arrow-circle-right"></i></span>
                    <div class="clearfix"></div>
                </div>
            </a>
        </div>
    </div>`
}

func dashboardPage(values ...string) string {
	labels := []string{"SQL上线工单", "SQL查询工单", "有效平台用户", "实例数量"}
	var body string
	for index, value := range values {
		label := "unknown"
		if index < len(labels) {
			label = labels[index]
		}
		body += dashboardCard(label, value)
	}
	return `<html><body><div class="row">` + body + `</div></body></html>`
}

func TestParseDashboardCounts(t *testing.T) {
	counts, err := ParseDashboardCounts([]byte(dashboardPage("2200", "396", "87", "19")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2200 {
		t.Errorf("SQLWorkflow = %d, want 2200", counts.SQLWorkflow)
	}
	if counts.SQLQuery != 396 {
		t.Errorf("SQLQuery = %d, want 396", counts.SQLQuery)
	}
	if counts.ActiveUsers != 87 {
		t.Errorf("ActiveUsers = %d, want 87", counts.ActiveUsers)
	}
	if counts.Instances != 19 {
		t.Errorf("Instances = %d, want 19", counts.Instances)
	}
}

func TestParseDashboardCountsToleratesWhitespaceAndSeparators(t *testing.T) {
	page := dashboardPage(" 2,200 ", "\n396\n", "87", "0")
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2200 {
		t.Errorf("SQLWorkflow = %d, want 2200", counts.SQLWorkflow)
	}
	if counts.SQLQuery != 396 {
		t.Errorf("SQLQuery = %d, want 396", counts.SQLQuery)
	}
	if counts.Instances != 0 {
		t.Errorf("Instances = %d, want 0", counts.Instances)
	}
}

// 首页四张卡按文案归类，不能依赖模板里的出现顺序：顺序一变数字就全错位。
func TestParseDashboardCountsMapsByLabelNotOrder(t *testing.T) {
	page := `<html><body>` +
		dashboardCard("实例数量", "19") +
		dashboardCard("有效平台用户", "87") +
		dashboardCard("SQL查询工单", "396") +
		dashboardCard("SQL上线工单", "2200") +
		`</body></html>`
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2200 || counts.SQLQuery != 396 || counts.ActiveUsers != 87 || counts.Instances != 19 {
		t.Fatalf("counts mapped by position instead of label: %+v", counts)
	}
}

// 首页除了四张卡还可能有别的 info-box（待办、审批等），不能把它们的数字算进来。
func TestParseDashboardCountsSkipsUnknownLabels(t *testing.T) {
	page := `<html><body>` +
		dashboardCard("待办审批", "12") +
		dashboardCard("SQL上线工单", "2200") +
		dashboardCard("SQL查询工单", "396") +
		dashboardCard("有效平台用户", "87") +
		dashboardCard("实例数量", "19") +
		`</body></html>`
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2200 || counts.SQLQuery != 396 || counts.ActiveUsers != 87 || counts.Instances != 19 {
		t.Fatalf("unknown info-box leaked into counts: %+v", counts)
	}
}

// 真实 /dashboard/ 用的就是这种 panel 卡：不解析这块，四个指标全是横杠。
func TestParseDashboardCountsFromPanelCards(t *testing.T) {
	page := `<html><body><div class="row">` +
		dashboardPanel("SQL上线工单", "2244", "primary") +
		dashboardPanel("SQL查询工单", "401", "green") +
		dashboardPanel("有效平台用户", "88", "yellow") +
		dashboardPanel("实例数量", "19", "red") +
		`</div>
<div id="toolbar">统计时间范围：<span>2026-10-29 - 2026-11-05</span></div>
</body></html>`
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2244 || counts.SQLQuery != 401 || counts.ActiveUsers != 88 || counts.Instances != 19 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

// panel 卡同样按文案归类：顺序变化不影响结果。
func TestParseDashboardCountsFromPanelCardsByLabel(t *testing.T) {
	page := `<html><body><div class="row">` +
		dashboardPanel("实例数量", "19", "red") +
		dashboardPanel("有效平台用户", "88", "yellow") +
		dashboardPanel("SQL查询工单", "401", "green") +
		dashboardPanel("SQL上线工单", "2244", "primary") +
		`</div></body></html>`
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2244 || counts.SQLQuery != 401 || counts.ActiveUsers != 88 || counts.Instances != 19 {
		t.Fatalf("counts mapped by position instead of label: %+v", counts)
	}
}

// Archery 1.14 首页是 small-box（h3 + p），不解析这块的话四个指标全是横杠。
func TestParseDashboardCountsFromSmallBoxes(t *testing.T) {
	page := `<html><body><div class="row">` +
		smallBox("SQL上线工单", "2244", "aqua") +
		smallBox("SQL查询工单", "401", "green") +
		smallBox("有效平台用户", "88", "yellow") +
		smallBox("实例数量", "19", "red") +
		`</div></body></html>`
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2244 || counts.SQLQuery != 401 || counts.ActiveUsers != 88 || counts.Instances != 19 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

// small-box 同样按文案归类：顺序变了或多出别的卡也不能串位。
func TestParseDashboardCountsFromSmallBoxesByLabel(t *testing.T) {
	page := `<html><body><div class="row">` +
		smallBox("待办审批", "12", "blue") +
		smallBox("实例数量", "19", "red") +
		smallBox("SQL查询工单", "401", "green") +
		smallBox("SQL上线工单", "2244", "aqua") +
		smallBox("有效平台用户", "88", "yellow") +
		`</div></body></html>`
	counts, err := ParseDashboardCounts([]byte(page))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.SQLWorkflow != 2244 || counts.SQLQuery != 401 || counts.ActiveUsers != 88 || counts.Instances != 19 {
		t.Fatalf("counts mapped by position instead of label: %+v", counts)
	}
}

func TestParseDashboardCountsRejectsIncompletePage(t *testing.T) {
	page := dashboardPage("2200", "396")
	if _, err := ParseDashboardCounts([]byte(page)); err == nil {
		t.Fatal("expected error for a page with only two counters")
	}
}

func TestParseDashboardCountsRejectsLoginPage(t *testing.T) {
	if _, err := ParseDashboardCounts([]byte("<html><body>login</body></html>")); err == nil {
		t.Fatal("expected error for a page without counters")
	}
}

// 首页「数据库实例类型统计」面板：柱状图给「环境 × 类型」，环图给「类型」总量。
// 片段取自真实页面（ECharts option 是合法 JSON）。
const dashboardInstanceTypeHTML = `<div class="panel panel-default">
  <div class="panel-heading"><i class="fa fa-bar-chart-o fa-fw"></i> 数据库实例类型统计</div>
  <div class="panel-body">
    <div class="row">
      <div class="col-lg-6"><div><script>
        var chart_bar = echarts.init(document.getElementById('bar'), 'white', {renderer: 'canvas'});
        var option_bar = {"color":["#5470c6"],"series":[{"type":"bar","name":"master","data":[14,1,1]},{"type":"bar","name":"slave","data":[2,0,1]}],"xAxis":[{"data":["mysql","doris","redis"]}]};
        chart_bar.setOption(option_bar);
      </script></div></div>
      <div class="col-lg-6"><div><script>
        var option_pie = {"series":[{"type":"pie","data":[{"name":"mysql","value":16},{"name":"redis","value":2},{"name":"doris","value":1}]}]};
      </script></div></div>
    </div>
  </div>
</div>`

func TestParseInstanceTypeStats(t *testing.T) {
	stats := ParseInstanceTypeStats([]byte(dashboardInstanceTypeHTML))
	if stats == nil {
		t.Fatal("expected instance type stats")
	}
	if strings.Join(stats.DBTypes, ",") != "mysql,doris,redis" {
		t.Fatalf("unexpected dbTypes: %v", stats.DBTypes)
	}
	if strings.Join(stats.Envs, ",") != "master,slave" {
		t.Fatalf("unexpected envs: %v", stats.Envs)
	}
	if len(stats.Series) != 2 {
		t.Fatalf("unexpected series: %+v", stats.Series)
	}
	if strings.Join(intsToText(stats.Series[0].Values), ",") != "14,1,1" {
		t.Fatalf("unexpected master values: %v", stats.Series[0].Values)
	}
	if strings.Join(intsToText(stats.Series[1].Values), ",") != "2,0,1" {
		t.Fatalf("unexpected slave values: %v", stats.Series[1].Values)
	}
	if len(stats.Types) != 3 || stats.Types[0].DBType != "mysql" || stats.Types[0].Count != 16 {
		t.Fatalf("unexpected type totals: %+v", stats.Types)
	}
}

// 首页两张按日统计图：「SQL查询统计」是双折线（检索行数/检索次数），
// 「SQL上线数量」是单柱状。片段取自真实页面。
const dashboardTrendsHTML = `<html><body>
<div class="panel panel-default">
  <div class="panel-heading"><i class="fa fa-bar-chart-o fa-fw"></i> SQL查询统计</div>
  <div id="chart-line1" class="panel-body"><script>
    var option_line = {"series":[{"type":"line","name":"\u68c0\u7d22\u884c\u6570","data":[["2026-10-29",1870],["2026-10-30",2435],["2026-10-31",167],["2026-11-01",0],["2026-11-02",1943],["2026-11-03",3246],["2026-11-04",1066],["2026-11-05",0]]},{"type":"line","name":"\u68c0\u7d22\u6b21\u6570","data":[["2026-10-29",37],["2026-10-30",95],["2026-10-31",10],["2026-11-01",0],["2026-11-02",78],["2026-11-03",230],["2026-11-04",52],["2026-11-05",0]]}]};
  </script></div>
</div>
<div class="panel panel-default">
  <div class="panel-heading"><i class="fa fa-bar-chart-o fa-fw"></i> SQL上线数量</div>
  <div id="chart-bar1" class="panel-body"><script>
    var option_bar = {"series":[{"type":"bar","data":[5,11,2,0,16,16,4,0]}],"xAxis":[{"data":["2026-10-29","2026-10-30","2026-10-31","2026-11-01","2026-11-02","2026-11-03","2026-11-04","2026-11-05"]}]};
  </script></div>
</div>
<div id="reservation" class="form-control">统计时间范围：<i class="fa fa-calendar"></i>&nbsp;<span>2026-10-29 - 2026-11-05</span> <i class="fa fa-caret-down"></i></div>
</body></html>`

func TestParseDashboardTrends(t *testing.T) {
	trends := ParseDashboardTrends([]byte(dashboardTrendsHTML))
	if trends == nil {
		t.Fatal("expected trends")
	}
	if strings.Join(trends.Dates, ",") != "2026-10-29,2026-10-30,2026-10-31,2026-11-01,2026-11-02,2026-11-03,2026-11-04,2026-11-05" {
		t.Fatalf("unexpected dates: %v", trends.Dates)
	}
	if len(trends.Query) != 2 {
		t.Fatalf("expected two query series, got %+v", trends.Query)
	}
	if trends.Query[0].Name != "检索行数" || strings.Join(intsToText(trends.Query[0].Values), ",") != "1870,2435,167,0,1943,3246,1066,0" {
		t.Fatalf("unexpected query rows series: %+v", trends.Query[0])
	}
	if trends.Query[1].Name != "检索次数" || trends.Query[1].Values[5] != 230 {
		t.Fatalf("unexpected query count series: %+v", trends.Query[1])
	}
	if strings.Join(intsToText(trends.Release), ",") != "5,11,2,0,16,16,4,0" {
		t.Fatalf("unexpected release values: %v", trends.Release)
	}
	if trends.RangeLabel != "2026-10-29 - 2026-11-05" {
		t.Fatalf("unexpected range label: %q", trends.RangeLabel)
	}
}

// /dashboard/api/ 的 chart 片段：结构与首页一致，但只有图表本身。
const (
	chartAPILineFragment = `<div class="chart-container"><script>
  var chart_1 = echarts.init(document.getElementById('1'), 'white', {renderer: 'canvas'});
  var option_1 = {"series":[{"type":"line","name":"检索行数","data":[["2026-10-06",1870],["2026-10-07",2435]]},{"type":"line","name":"检索次数","data":[["2026-10-06",37],["2026-10-07",95]]}]};
</script></div>`
	chartAPIBarFragment = `<script>var option_2 = {"series":[{"type":"bar","data":[5,11]}],"xAxis":[{"data":["2026-10-06","2026-10-07"]}]};</script>`
)

func TestParseTrendFragments(t *testing.T) {
	trends := ParseTrendFragments(
		map[string]string{"line1": chartAPILineFragment, "bar1": chartAPIBarFragment},
		"2026-10-06",
		"2026-10-07",
	)
	if trends == nil {
		t.Fatal("expected trends")
	}
	if strings.Join(trends.Dates, ",") != "2026-10-06,2026-10-07" {
		t.Fatalf("unexpected dates: %v", trends.Dates)
	}
	if len(trends.Query) != 2 || trends.Query[0].Values[1] != 2435 {
		t.Fatalf("unexpected query series: %+v", trends.Query)
	}
	if strings.Join(intsToText(trends.Release), ",") != "5,11" {
		t.Fatalf("unexpected release values: %v", trends.Release)
	}
	if trends.RangeLabel != "2026-10-06 - 2026-10-07" {
		t.Fatalf("unexpected range label: %q", trends.RangeLabel)
	}
}

// 页面 /dashboard/ 不认日期参数，真正按区间取数的是 /dashboard/api/。
func TestDashboardTrendsUsesChartAPI(t *testing.T) {
	fs := newFake(t, fakeOptions{dashboardChart: map[string]string{
		"line1": chartAPILineFragment,
		"bar1":  chartAPIBarFragment,
	}})
	client := newTestClient(t, fs)

	trends, err := client.DashboardTrends(context.Background(), "2026-10-06", "2026-10-07")
	if err != nil {
		t.Fatalf("DashboardTrends: %v", err)
	}
	if trends.RangeLabel != "2026-10-06 - 2026-10-07" {
		t.Fatalf("unexpected range: %+v", trends)
	}
	fs.mu.Lock()
	query := fs.dashboardQuery
	fs.mu.Unlock()
	if query.Get("start_date") != "2026-10-06" || query.Get("end_date") != "2026-10-07" {
		t.Fatalf("unexpected query: %v", query)
	}
}

func TestParseDashboardTrendsMissing(t *testing.T) {
	if trends := ParseDashboardTrends([]byte(`<html><body>no charts</body></html>`)); trends != nil {
		t.Fatalf("expected nil trends, got %+v", trends)
	}
}

func TestParseInstanceTypeStatsMissingPanel(t *testing.T) {
	if stats := ParseInstanceTypeStats([]byte(`<html><body>no charts</body></html>`)); stats != nil {
		t.Fatalf("expected nil stats, got %+v", stats)
	}
}

func intsToText(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}

func announcementPage(level, title, content string) string {
	return `<html><body>
<div class="alert alert-` + level + ` alert-dismissible">
  <button type="button" class="close" data-dismiss="alert" aria-hidden="true">×</button>
  <h4><i class="icon fa fa-warning"></i> ` + title + `</h4>
  ` + content + `
</div>
` + dashboardPage("2200", "396", "87", "19") + `</body></html>`
}

func TestParseAnnouncement(t *testing.T) {
	page := announcementPage("warning", "注意!", "<p>核心表保持精简，尽量不加字段。</p><br>非核心字段放到扩展表。")
	got := ParseAnnouncement([]byte(page))
	if got == nil {
		t.Fatal("expected announcement")
	}
	if got.Level != "warning" {
		t.Errorf("Level = %q, want warning", got.Level)
	}
	if got.Title != "注意!" {
		t.Errorf("Title = %q, want 注意!", got.Title)
	}
	if !strings.Contains(got.Content, "核心表保持精简") || !strings.Contains(got.Content, "非核心字段放到扩展表") {
		t.Errorf("Content = %q", got.Content)
	}
	if strings.Contains(got.Content, "<p>") || strings.Contains(got.Content, "&nbsp;") {
		t.Errorf("Content still contains markup: %q", got.Content)
	}
}

func TestParseAnnouncementWithoutHeading(t *testing.T) {
	page := `<div class="alert alert-info">系统维护时间：周六 02:00-04:00</div>`
	got := ParseAnnouncement([]byte(page))
	if got == nil {
		t.Fatal("expected announcement")
	}
	if got.Level != "info" {
		t.Errorf("Level = %q, want info", got.Level)
	}
	if got.Title != "" {
		t.Errorf("Title = %q, want empty", got.Title)
	}
	if got.Content != "系统维护时间：周六 02:00-04:00" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestParseAnnouncementMissing(t *testing.T) {
	if got := ParseAnnouncement([]byte(dashboardPage("1", "2", "3", "4"))); got != nil {
		t.Fatalf("expected nil announcement, got %+v", got)
	}
	if got := ParseAnnouncement(nil); got != nil {
		t.Fatalf("expected nil announcement for empty input, got %+v", got)
	}
}

func TestParseBrand(t *testing.T) {
	page := []byte(`<html><body>
<nav><a class="navbar-brand" href="/index/"><strong>Archery(广州跨境&amp;其他测试环境)</strong></a></nav>
</body></html>`)
	got := ParseBrand(page)
	if got.Name != "Archery" {
		t.Errorf("Name = %q, want Archery", got.Name)
	}
	if got.Suffix != "(广州跨境&其他测试环境)" {
		t.Errorf("Suffix = %q", got.Suffix)
	}
}

// 模板改版或自定义品牌时不要再强行拆前缀。
func TestParseBrandCustomName(t *testing.T) {
	page := []byte(`<a class="navbar-brand" href="/index/"><strong>公司数据平台</strong></a>`)
	got := ParseBrand(page)
	if got.Name != "公司数据平台" || got.Suffix != "" {
		t.Errorf("unexpected brand: %+v", got)
	}
}

func TestParseBrandMissing(t *testing.T) {
	if got := ParseBrand([]byte(dashboardPage("1", "2", "3", "4"))); got.Name != "" || got.Suffix != "" {
		t.Fatalf("expected empty brand, got %+v", got)
	}
	if got := ParseBrand(nil); got.Name != "" {
		t.Fatalf("expected empty brand for nil input, got %+v", got)
	}
}
