package cmd

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jeehoon/graylog-cli/pkg/graylog"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
)

func testTUIModel() searchTUIModel {
	qreq := graylog.NewQueryRequest(nil, "", "level:3 AND service:api")
	qreq.BaseQuery = "level:3"
	qreq.Application = "api"
	qreq.Part = "checkout"
	qreq.PageLimit = 4
	qreq.Offset = 0
	qreq.Refresh = "5s"
	qreq.Output = client.OutputCompact
	qreq.Fields = []string{"request_id", "application", "part"}

	model := newSearchTUIModel(nil, qreq, client.DefaultDecoderConfig(), func(qreq *graylog.QueryRequest) searchTUIResult {
		if qreq.QueryType == graylog.QueryTypeHistogram {
			return testTUIHistogramResult()
		}
		page := qreq.Offset / qreq.PageLimit
		return testTUIResult(page*qreq.PageLimit, qreq.PageLimit, 12)
	})
	model.width = 120
	model.height = 24
	model.applyResult(testTUIResult(0, 4, 12))
	applyTestCalc(&model)
	return model
}

func applyTestCalc(model *searchTUIModel) {
	model.applyCalcResult(tuiCalcResult{
		generation: model.calcGeneration,
		groups:     calculateGroups(model.visibleMessages(), model.decoderCfg),
		candidates: calculateFieldCandidates(model.visibleMessages(), model.columns, model.decoderCfg),
	})
}

func testTUIResult(start int, count int, total uint64) searchTUIResult {
	messages := make([]*client.Message, 0, count)
	for idx := 0; idx < count; idx++ {
		seq := start + idx
		level := float64(6)
		levelText := "INFO"
		if seq%3 == 0 {
			level = float64(3)
			levelText = "ERROR"
		} else if seq%3 == 1 {
			level = float64(4)
			levelText = "WARN"
		}
		messages = append(messages, &client.Message{Message: map[string]any{
			"timestamp":   fmt.Sprintf("2026-06-24T14:%02d:%02d.000Z", seq/60, seq%60),
			"level":       level,
			"level_text":  levelText,
			"source":      fmt.Sprintf("api-%d", seq%2+1),
			"message":     fmt.Sprintf("failed request %d for tenant %d\nstack line", seq, seq%2),
			"request_id":  fmt.Sprintf("req-%03d", seq),
			"application": "api",
			"part":        "checkout",
			"nested": map[string]any{
				"duration_ms": seq * 10,
				"user":        fmt.Sprintf("user-%d", seq),
			},
		}})
	}
	return searchTUIResult{
		messages:      messages,
		msgCnt:        uint64(count),
		total:         total,
		effectiveFrom: "2026-06-24T14:00:00.000Z",
		effectiveTo:   "2026-06-24T14:15:00.000Z",
	}
}

func testTUIHistogramResult() searchTUIResult {
	return searchTUIResult{
		histogram: []tuiHistogramBucket{
			{From: "2026-06-24T14:00:00.000Z", To: "2026-06-24T14:05:00.000Z", Count: 2},
			{From: "2026-06-24T14:05:00.000Z", To: "2026-06-24T14:10:00.000Z", Count: 7},
			{From: "2026-06-24T14:10:00.000Z", To: "2026-06-24T14:15:00.000Z", Count: 3},
		},
		effectiveFrom: "2026-06-24T14:00:00.000Z",
		effectiveTo:   "2026-06-24T14:15:00.000Z",
	}
}

func TestSearchTUIRendersV2ComponentLayoutAndTable(t *testing.T) {
	model := testTUIModel()

	view := model.View()
	for _, want := range []string{
		"graylog-cli search --tui | view: logs",
		"range: 2026-06-24T14:00:00.000Z ~ 2026-06-24T14:15:00.000Z",
		"timestamp",
		"level",
		"source",
		"message",
		"refresh:",
		"STATUS page 1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "stack line") {
		t.Fatalf("logs view rendered multiline detail content:\n%s", view)
	}
}

func TestSearchTUIUsesBubblesTableForLogRows(t *testing.T) {
	model := testTUIModel()
	model.selected = 1
	model.refreshComponents()

	if got := len(model.logsTable.Rows()); got != len(model.visibleMessages()) {
		t.Fatalf("table rows = %d, want %d", got, len(model.visibleMessages()))
	}
	if model.logsTable.Cursor() != 1 {
		t.Fatalf("table cursor = %d, want 1", model.logsTable.Cursor())
	}
	cols := model.logsTable.Columns()
	if len(cols) == 0 || cols[0].Title != "timestamp" {
		t.Fatalf("table columns = %#v, want timestamp first", cols)
	}
	view := model.View()
	for _, forbidden := range []string{"┌", "┐", "└", "┘", "├", "┤"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("view still uses manual frame character %q:\n%s", forbidden, view)
		}
	}
}

func TestSearchTUIEnterOpensSeparateDetailView(t *testing.T) {
	model := testTUIModel()

	updated, cmd := model.handleLogsKey("enter")
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("enter returned command, want nil")
	}
	if model.view != tuiViewDetail {
		t.Fatalf("view = %q, want detail", model.view)
	}
	view := model.View()
	if !strings.Contains(view, "view: detail | tab: JSON Tree") {
		t.Fatalf("detail header missing JSON Tree tab:\n%s", view)
	}
	if !strings.Contains(view, "nested:") || !strings.Contains(view, "duration_ms: 0") {
		t.Fatalf("detail JSON tree missing nested fields:\n%s", view)
	}

	updated, _ = model.handleDetailKey("tab")
	model = updated.(searchTUIModel)
	if model.detailTab != tuiDetailPretty {
		t.Fatalf("detailTab = %q, want Pretty", model.detailTab)
	}
	updated, _ = model.handleDetailKey("tab")
	model = updated.(searchTUIModel)
	if model.detailTab != tuiDetailRaw {
		t.Fatalf("detailTab = %q, want Raw", model.detailTab)
	}
	if !strings.Contains(model.View(), "stack line") {
		t.Fatalf("raw detail should preserve multiline message:\n%s", model.View())
	}
}

func TestSearchTUILocalFilterUsesCachedPages(t *testing.T) {
	model := testTUIModel()
	model.cachedPages[4] = testTUIResult(4, 4, 12).messages

	model.openPrompt(tuiPromptLocalFilter, "req-006")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("local filter returned nil command, want background calc")
	}
	if got := len(model.visibleMessages()); got != 1 {
		t.Fatalf("filtered messages = %d, want 1", got)
	}
	if !strings.Contains(model.View(), "req-006") {
		t.Fatalf("filtered cached row not rendered:\n%s", model.View())
	}
}

func TestSearchTUILocalFilterPromptShowsCandidates(t *testing.T) {
	model := testTUIModel()
	model.openPrompt(tuiPromptLocalFilter, "")

	line := model.promptLine()
	for _, want := range []string{"local filter:", "candidates", "application=api"} {
		if !strings.Contains(line, want) {
			t.Fatalf("prompt line = %q, missing %q", line, want)
		}
	}
}

func TestSearchTUISortPromptResetsCacheAndFetches(t *testing.T) {
	model := testTUIModel()
	model.cachedPages[4] = testTUIResult(4, 4, 12).messages

	model.openPrompt(tuiPromptSort, "source:asc")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("sort prompt returned nil command, want fetch")
	}
	if model.qreq.Sort != "source:ASC" {
		t.Fatalf("sort = %q, want source:ASC", model.qreq.Sort)
	}
	if model.qreq.Offset != 0 {
		t.Fatalf("offset = %d, want 0", model.qreq.Offset)
	}
	if len(model.cachedPages) != 0 {
		t.Fatalf("cached pages = %d, want 0", len(model.cachedPages))
	}
}

func TestSearchTUIPageNavigationUsesFetchAndCache(t *testing.T) {
	model := testTUIModel()

	updated, cmd := model.nextPage()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("next page returned nil command, want fetch")
	}
	if model.qreq.Offset != 4 {
		t.Fatalf("offset = %d, want 4", model.qreq.Offset)
	}

	model.applyResult(testTUIResult(4, 4, 12))
	applyTestCalc(&model)
	updated, cmd = model.prevPage(false)
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("previous cached page returned command, want nil")
	}
	if model.qreq.Offset != 0 {
		t.Fatalf("offset = %d, want 0", model.qreq.Offset)
	}
}

func TestSearchTUICacheEvictsLeastRecentlyUsedPage(t *testing.T) {
	model := testTUIModel()
	for page := 0; page < tuiMaxCachedPages+2; page++ {
		model.qreq.Offset = page * model.qreq.PageLimit
		model.applyResult(testTUIResult(page*model.qreq.PageLimit, 1, 100))
	}

	if len(model.cachedPages) != tuiMaxCachedPages {
		t.Fatalf("cached pages = %d, want %d", len(model.cachedPages), tuiMaxCachedPages)
	}
	if _, ok := model.cachedPages[0]; ok {
		t.Fatal("oldest page offset 0 was not evicted")
	}
	latestOffset := (tuiMaxCachedPages + 1) * model.qreq.PageLimit
	if _, ok := model.cachedPages[latestOffset]; !ok {
		t.Fatalf("latest page offset %d missing from cache", latestOffset)
	}
}

func TestSearchTUIHistogramNarrowsTimerange(t *testing.T) {
	model := testTUIModel()

	updated, cmd := model.handleLogsKey("h")
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("h returned nil command, want histogram fetch")
	}
	model.applyResult(testTUIHistogramResult())
	model.histogramSelected = 1

	updated, cmd = model.handleHistogramKey("enter")
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("histogram enter returned nil command, want logs fetch")
	}
	if model.view != tuiViewLogs {
		t.Fatalf("view = %q, want logs", model.view)
	}
	if model.qreq.SearchTimeRange.TimeType != graylog.TimeTypeAbsolute {
		t.Fatalf("timerange type = %q, want absolute", model.qreq.SearchTimeRange.TimeType)
	}
	if model.qreq.SearchTimeRange.AbsoluteStart != "2026-06-24T14:05:00.000Z" {
		t.Fatalf("absolute start = %q", model.qreq.SearchTimeRange.AbsoluteStart)
	}
}

func TestSearchTUIHistogramAllZeroShowsNoData(t *testing.T) {
	model := testTUIModel()
	model.view = tuiViewHistogram
	model.histogram = []tuiHistogramBucket{
		{From: "2026-06-24T14:00:00.000Z", To: "2026-06-24T14:05:00.000Z", Count: 0},
		{From: "2026-06-24T14:05:00.000Z", To: "2026-06-24T14:10:00.000Z", Count: 0},
	}

	view := model.View()
	if !strings.Contains(view, "no histogram data") {
		t.Fatalf("zero histogram did not render no data state:\n%s", view)
	}
}

func TestSearchTUIGroupModeOpensRepresentativeDetail(t *testing.T) {
	model := testTUIModel()

	updated, _ := model.handleLogsKey("G")
	model = updated.(searchTUIModel)
	if model.bodyMode != tuiModeGroups {
		t.Fatalf("bodyMode = %q, want groups", model.bodyMode)
	}
	view := model.View()
	if !strings.Contains(view, "message pattern") || !strings.Contains(view, "failed request <num> for tenant <num>") {
		t.Fatalf("group view missing pattern:\n%s", view)
	}

	updated, _ = model.handleLogsKey("enter")
	model = updated.(searchTUIModel)
	if model.view != tuiViewDetail {
		t.Fatalf("view = %q, want detail", model.view)
	}
}

func TestSearchTUIBackgroundCalcCanBeCancelled(t *testing.T) {
	model := testTUIModel()

	cmd := model.startBackgroundCalc()
	if cmd == nil {
		t.Fatal("background calc command is nil")
	}
	if !model.calcRunning {
		t.Fatal("calcRunning = false, want true")
	}

	updated, _ := model.handleLogsKey("esc")
	model = updated.(searchTUIModel)
	if model.calcRunning {
		t.Fatal("calcRunning = true after Esc cancel")
	}
	if model.calcProgress != "calculation cancelled" {
		t.Fatalf("calcProgress = %q", model.calcProgress)
	}

	stale := cmd().(tuiCalcResult)
	model.applyCalcResult(stale)
	if model.calcProgress != "calculation cancelled" {
		t.Fatalf("stale calc result was applied: %q", model.calcProgress)
	}
}

func TestSearchTUILocalFilterCandidateSelection(t *testing.T) {
	model := testTUIModel()
	model.openPrompt(tuiPromptLocalFilter, "")

	updated, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(searchTUIModel)
	if model.prompt.selected != 1 {
		t.Fatalf("prompt selected = %d, want 1", model.prompt.selected)
	}
	selectedFilter := model.fieldCandidatesData[1].Filter
	if model.prompt.value != selectedFilter {
		t.Fatalf("prompt value = %q, want selected candidate %q", model.prompt.value, selectedFilter)
	}
	if !strings.Contains(model.promptView(), selectedFilter) {
		t.Fatalf("prompt view did not show selected candidate %q: %q", selectedFilter, model.promptView())
	}

	updated, cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("candidate enter returned nil command, want background calc")
	}
	if model.localFilter != selectedFilter {
		t.Fatalf("localFilter = %q, want %q", model.localFilter, selectedFilter)
	}
	if len(model.visibleMessages()) == 0 {
		t.Fatal("candidate filter removed all generated messages")
	}
}

func TestSearchTUIDetailSearchScrollsToMatch(t *testing.T) {
	model := testTUIModel()
	model.view = tuiViewDetail
	model.detailTab = tuiDetailJSONTree

	model.openPrompt(tuiPromptDetailSearch, "duration_ms")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("detail search returned command, want nil")
	}
	if model.detailScroll == 0 {
		t.Fatal("detail search did not move scroll to match")
	}
	if model.err != nil {
		t.Fatalf("detail search err = %v", model.err)
	}
}

func TestSearchTUIPromptInvalidSortStaysInTUI(t *testing.T) {
	model := testTUIModel()

	model.openPrompt(tuiPromptSort, "bad")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("invalid sort returned command, want nil")
	}
	if model.err == nil {
		t.Fatal("invalid sort did not set status error")
	}
	if model.view != tuiViewLogs {
		t.Fatalf("view = %q, want logs", model.view)
	}
}

func TestSearchTUIFooterHelpToggle(t *testing.T) {
	model := testTUIModel()

	updated, _ := model.handleLogsKey("tab")
	model = updated.(searchTUIModel)
	if model.footerMode != tuiFooterHelp {
		t.Fatalf("footerMode = %q, want help", model.footerMode)
	}
	if !strings.Contains(model.View(), "/ query") || !strings.Contains(model.View(), "tab help") {
		t.Fatalf("help footer missing keymap:\n%s", model.View())
	}
}

func TestSearchTUIHandlePromptTyping(t *testing.T) {
	model := testTUIModel()
	model.openPrompt(tuiPromptQuery, "")

	updated, _ := model.handlePromptKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("error")})
	model = updated.(searchTUIModel)
	if model.prompt.value != "error" {
		t.Fatalf("prompt value = %q, want error", model.prompt.value)
	}
}

func TestSearchTUIServerOptionPromptsRefetchFromFirstPage(t *testing.T) {
	model := testTUIModel()
	model.qreq.Offset = 8
	model.cachedPages[8] = testTUIResult(8, 4, 12).messages

	model.openPrompt(tuiPromptApplication, "worker")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("application prompt returned nil command, want fetch")
	}
	if model.qreq.Application != "worker" {
		t.Fatalf("application = %q, want worker", model.qreq.Application)
	}
	if model.qreq.UserQuery != "(level:3) AND application:worker AND part:checkout" {
		t.Fatalf("query = %q", model.qreq.UserQuery)
	}
	if model.qreq.Offset != 0 || len(model.cachedPages) != 0 {
		t.Fatalf("offset/cache = %d/%d, want 0/0", model.qreq.Offset, len(model.cachedPages))
	}

	model.openPrompt(tuiPromptLimit, "10")
	updated, cmd = model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("limit prompt returned nil command, want fetch")
	}
	if model.qreq.PageLimit != 10 {
		t.Fatalf("limit = %d, want 10", model.qreq.PageLimit)
	}
}

func TestSearchTUIRangePromptAcceptsRelativeAndAbsolute(t *testing.T) {
	model := testTUIModel()

	model.openPrompt(tuiPromptRange, "15m")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("relative range prompt returned nil command, want fetch")
	}
	if model.qreq.SearchTimeRange.TimeType != graylog.TimeTypeRelative || model.qreq.SearchTimeRange.RelativeRange != "15m" {
		t.Fatalf("relative range = %#v", model.qreq.SearchTimeRange)
	}

	model.openPrompt(tuiPromptRange, "2026-06-24T14:00:00Z,2026-06-24T14:05:00Z")
	updated, cmd = model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("absolute range prompt returned nil command, want fetch")
	}
	if model.qreq.SearchTimeRange.TimeType != graylog.TimeTypeAbsolute {
		t.Fatalf("timerange type = %q, want absolute", model.qreq.SearchTimeRange.TimeType)
	}
	if model.qreq.SearchTimeRange.AbsoluteEnd != "2026-06-24T14:05:00Z" {
		t.Fatalf("absolute end = %q", model.qreq.SearchTimeRange.AbsoluteEnd)
	}
}

func TestSearchTUIJSONTreeParsesJSONMessageString(t *testing.T) {
	model := testTUIModel()
	model.messages = []*client.Message{{Message: map[string]any{
		"timestamp": "2026-06-24T14:00:00.000Z",
		"level":     float64(3),
		"source":    "api-1",
		"message":   `{"event":"checkout","items":[{"sku":"A","qty":2}],"stacktrace":"line1\nline2"}`,
	}}}
	model.cachedPages[0] = model.messages
	model.view = tuiViewDetail

	view := model.View()
	for _, want := range []string{"event: checkout", "items:", "sku: A", "stacktrace:", "line1", "line2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("JSON tree view missing %q:\n%s", want, view)
		}
	}

	model.detailTab = tuiDetailRaw
	if !strings.Contains(model.View(), `{"event":"checkout"`) {
		t.Fatalf("raw view did not preserve JSON message string:\n%s", model.View())
	}
}
