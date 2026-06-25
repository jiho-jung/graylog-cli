package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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
		candidates: calculateFieldCandidates(model.visibleMessages(), model.columns, model.decoderCfg, model.tuiCfg),
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
		"[view: logs",
		"[range: 2026-06-24T14:00:00.0",
		"#",
		"timestamp",
		"level",
		"app",
		"source",
		"message",
		"row 1/4",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "refresh:") {
		t.Fatalf("header should not render refresh box:\n%s", view)
	}
	if strings.Contains(view, "\nstack line") {
		t.Fatalf("logs view rendered multiline detail content:\n%s", view)
	}
}

func TestSearchTUIUsesConfiguredColumnsForLogRows(t *testing.T) {
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
	if len(cols) == 0 || cols[0].Title != "#" {
		t.Fatalf("table columns = %#v, want row number first", cols)
	}
	view := model.View()
	if strings.Contains(view, "▶2") {
		t.Fatalf("selected row should not show row number cursor:\n%s", view)
	}
	if !strings.Contains(view, "2    2026") {
		t.Fatalf("selected row should keep row number without cursor:\n%s", view)
	}
}

func TestSearchTUIRendersSeparateBorderedViewports(t *testing.T) {
	model := testTUIModel()
	view := model.View()

	if got := strings.Count(view, "┌"); got < 3 {
		t.Fatalf("view should render header/body/footer boxes, got %d top borders:\n%s", got, view)
	}
	if model.headerViewport.Height == 0 {
		t.Fatal("header viewport height was not initialized")
	}
	if model.bodyViewport.Height == 0 {
		t.Fatal("body viewport height was not initialized")
	}
	if model.footerViewport.Height == 0 {
		t.Fatal("footer viewport height was not initialized")
	}
	if strings.Contains(view, "┘\n\n┌") {
		t.Fatalf("view should not render blank lines between boxes:\n%s", view)
	}
	if !strings.Contains(view, "┘\n┌") {
		t.Fatalf("view should render adjacent bordered boxes:\n%s", view)
	}
}

func TestSearchTUIHeaderViewportResetsOffset(t *testing.T) {
	model := testTUIModel()
	model.headerViewport.SetYOffset(10)

	view := model.View()
	if !strings.Contains(view, "[view: logs") {
		t.Fatalf("header should render from first line after offset reset:\n%s", view)
	}
}

func TestSearchTUIHeaderRefreshOnlyConfigFallsBackToDefault(t *testing.T) {
	oldConfig := graylogCliConfig
	t.Cleanup(func() {
		graylogCliConfig = oldConfig
	})
	graylogCliConfig.SearchTUI = defaultSearchTUIConfig()
	graylogCliConfig.SearchTUI.HeaderBoxes = []SearchTUIOutputBox{{Name: "refresh", Width: 18}}

	model := testTUIModel()
	view := model.View()
	if !strings.Contains(view, "[view: logs") {
		t.Fatalf("header should fall back to default boxes when refresh is removed:\n%s", view)
	}
	if strings.Contains(view, "refresh:") {
		t.Fatalf("header should ignore refresh box from config:\n%s", view)
	}
}

func TestSearchTUILoadingWithExistingMessagesDoesNotShowPromptLine(t *testing.T) {
	model := testTUIModel()
	model.loading = true

	view := model.View()
	if strings.Contains(view, "\nloading...\n") {
		t.Fatalf("loading prompt line should not render while existing rows remain:\n%s", view)
	}
}

func TestSearchTUIDefaultColumnsAndClipping(t *testing.T) {
	model := testTUIModel()
	widths := model.columnWidths()
	if len(model.columns) < 5 {
		t.Fatalf("columns = %#v, want timestamp/level/application/source/message", model.columns)
	}
	for idx, want := range []string{"timestamp", "level", "application", "source"} {
		if model.columns[idx] != want {
			t.Fatalf("column[%d] = %q, want %q", idx, model.columns[idx], want)
		}
	}
	if model.columns[len(model.columns)-1] != "message" {
		t.Fatalf("last column = %q, want message", model.columns[len(model.columns)-1])
	}
	if widths[0] < len("2026-06-25T00:09:35.855Z") {
		t.Fatalf("timestamp width = %d, want full ISO timestamp width", widths[0])
	}
	if got := displayColumnName("application"); got != "app" {
		t.Fatalf("application display = %q, want app", got)
	}
	if got := leftClip("2026-06-25T00:09:35.855Z", 10); got != "09:35.855Z" {
		t.Fatalf("leftClip timestamp = %q", got)
	}
	if got := leftClip("very-long-source-name", 6); got != "e-name" {
		t.Fatalf("leftClip source = %q", got)
	}
	if widths[len(widths)-1] <= 0 {
		t.Fatalf("message width = %d, want flex fill", widths[len(widths)-1])
	}
}

func TestSearchTUILogRowsFitViewportWidth(t *testing.T) {
	model := testTUIModel()
	model.width = 60
	model.messages = []*client.Message{{Message: map[string]any{
		"timestamp":   "2026-06-25T00:09:35.855Z",
		"level":       "INFO",
		"application": "api",
		"source":      "very-long-source-name-that-should-left-clip",
		"message":     strings.Repeat("long-message ", 20),
	}}}
	model.cachedPages[0] = model.messages

	for _, line := range strings.Split(model.renderLogBody(), "\n") {
		if visibleLen(line) > model.viewportContentWidth() {
			t.Fatalf("line width = %d, want <= %d:\n%q", visibleLen(line), model.viewportContentWidth(), line)
		}
	}
}

func TestSearchTUICursorMovesWithinVisibleWindow(t *testing.T) {
	model := testTUIModel()
	model.messages = testTUIResult(0, 12, 12).messages
	model.cachedPages[0] = model.messages
	model.height = 10
	model.listScroll = 0
	model.selected = 0

	updated, _ := model.handleLogsKeyMsg(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(searchTUIModel)
	if model.selected != 1 {
		t.Fatalf("selected = %d, want one-row movement", model.selected)
	}
	if model.visibleStartIndex() != 0 {
		t.Fatalf("visible start = %d, want cursor to move before scrolling", model.visibleStartIndex())
	}
}

func TestSearchTUIPageKeysUseConfiguredScrollStep(t *testing.T) {
	model := testTUIModel()
	model.messages = testTUIResult(0, 30, 30).messages
	model.cachedPages[0] = model.messages
	model.tuiCfg.PageScrollStep = 10

	updated, _ := model.handleLogsKeyMsg(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(searchTUIModel)
	if model.selected != 10 {
		t.Fatalf("selected = %d, want 10", model.selected)
	}

	updated, _ = model.handleLogsKeyMsg(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(searchTUIModel)
	if model.selected != 0 {
		t.Fatalf("selected = %d, want 0", model.selected)
	}
}

func TestSearchTUIScrollsByConfiguredStepAtBodyBottom(t *testing.T) {
	model := testTUIModel()
	model.tuiCfg.HeaderVisible = false
	model.tuiCfg.FooterVisible = false
	model.tuiCfg.PageScrollStep = 10
	model.height = 12
	model.messages = testTUIResult(0, 30, 30).messages
	model.cachedPages[0] = model.messages
	model.selected = 0
	model.listScroll = 0

	for range 11 {
		updated, _ := model.handleLogsKeyMsg(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(searchTUIModel)
	}
	if model.selected != 11 {
		t.Fatalf("selected = %d, want 11", model.selected)
	}
	if model.listScroll != 10 {
		t.Fatalf("listScroll = %d, want configured 10-line scroll", model.listScroll)
	}
}

func TestSearchTUIExpandedRowsKeepSelectedLineVisible(t *testing.T) {
	model := testTUIModel()
	model.tuiCfg.HeaderVisible = false
	model.tuiCfg.FooterVisible = false
	model.tuiCfg.ExpandedMaxLines = 10
	model.tuiCfg.PageScrollStep = 10
	model.height = 12
	model.messages = testTUIResult(0, 30, 30).messages
	model.cachedPages[0] = model.messages
	model.selected = 0
	model.listScroll = 0

	updated, _ := model.handleLogsKey(" ")
	model = updated.(searchTUIModel)
	updated, _ = model.handleLogsKeyMsg(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(searchTUIModel)

	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1", model.selected)
	}
	selectedLine := model.selectedLineIndex()
	if selectedLine < model.listScroll || selectedLine >= model.listScroll+model.visibleLogBodyRows() {
		t.Fatalf("selected line %d outside visible range [%d,%d)", selectedLine, model.listScroll, model.listScroll+model.visibleLogBodyRows())
	}
	if !strings.Contains(model.renderLogBody(), "2    2026") {
		t.Fatalf("selected row is not visible after expanded row scroll:\n%s", model.renderLogBody())
	}
}

func TestSearchTUIEnterOpensDetailPopup(t *testing.T) {
	model := testTUIModel()

	updated, cmd := model.handleLogsKey("enter")
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("enter returned command, want nil")
	}
	if !model.detailPopupOpen {
		t.Fatal("detail popup did not open")
	}
	view := model.View()
	for _, want := range []string{"detail", "timestamp:", "request_id:", "Esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail popup missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "┏") || !strings.Contains(view, "┃") {
		t.Fatalf("detail popup should use thick border:\n%s", view)
	}
	if strings.Contains(view, "\x1b[7m") {
		t.Fatalf("detail popup should not overlap inverse selected row styling:\n%s", view)
	}
	updated, _ = model.handleDetailPopupKey("esc")
	model = updated.(searchTUIModel)
	if model.detailPopupOpen {
		t.Fatal("detail popup stayed open after Esc")
	}
}

func TestSearchTUISpaceTogglesExpandedRow(t *testing.T) {
	model := testTUIModel()

	updated, _ := model.handleLogsKey(" ")
	model = updated.(searchTUIModel)
	if len(model.expanded) != 1 {
		t.Fatalf("expanded rows = %d, want 1", len(model.expanded))
	}
	view := model.View()
	if !strings.Contains(view, "request_id: req-000") || !strings.Contains(view, "stack line") {
		t.Fatalf("expanded row missing details:\n%s", view)
	}

	updated, _ = model.nextPage()
	model = updated.(searchTUIModel)
	updated, _ = model.prevPage(false)
	model = updated.(searchTUIModel)
	if len(model.expanded) != 1 {
		t.Fatalf("expanded state not preserved across cached page navigation")
	}

	updated, _ = model.handleLogsKey(" ")
	model = updated.(searchTUIModel)
	if len(model.expanded) != 0 {
		t.Fatalf("expanded rows = %d, want closed", len(model.expanded))
	}
}

func TestSearchTUIExpandedDefaultShowsAllLines(t *testing.T) {
	model := testTUIModel()
	model.tuiCfg.ExpandedMaxLines = defaultSearchTUIConfig().ExpandedMaxLines
	model.messages = []*client.Message{{Message: map[string]any{
		"timestamp":   "2026-06-25T00:09:35.855Z",
		"level":       "INFO",
		"source":      "api-1",
		"message":     "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\nline13\nline14\nline15\nline16\nline17\nline18\nline19\nline20\nline21",
		"application": "api",
		"part":        "checkout",
	}}}
	model.cachedPages[0] = model.messages

	lines := model.expandedLines(model.messages[0])
	if len(lines) < 21 {
		t.Fatalf("expanded lines = %d, want all message lines visible by default", len(lines))
	}
}

func TestSearchTUILastLineExpansionScrollsExpandedContentIntoView(t *testing.T) {
	model := testTUIModel()
	model.tuiCfg.HeaderVisible = false
	model.tuiCfg.FooterVisible = false
	model.tuiCfg.ExpandedMaxLines = 10
	model.height = 8
	model.messages = testTUIResult(0, 12, 12).messages
	model.cachedPages[0] = model.messages
	model.selected = 11
	model.ensureSelectedVisible()

	updated, _ := model.handleLogsKey(" ")
	model = updated.(searchTUIModel)
	body := model.renderLogBody()
	if !strings.Contains(body, "12   2026") {
		t.Fatalf("last selected row is not visible after expand:\n%s", body)
	}
	if !strings.Contains(body, "timestamp: 2026-06-24T14:00:11.000Z") || !strings.Contains(body, "stack line") {
		t.Fatalf("expanded content is not visible after expanding last row:\n%s", body)
	}
}

func TestSearchTUIExpandedRowsCanBeClearedOnPageMove(t *testing.T) {
	model := testTUIModel()
	model.tuiCfg.PersistExpandedRows = false

	updated, _ := model.handleLogsKey(" ")
	model = updated.(searchTUIModel)
	if len(model.expanded) != 1 {
		t.Fatalf("expanded rows = %d, want 1", len(model.expanded))
	}

	updated, _ = model.nextPage()
	model = updated.(searchTUIModel)
	if len(model.expanded) != 0 {
		t.Fatalf("expanded rows = %d, want cleared", len(model.expanded))
	}
}

func TestSearchTUIDetailPopupScrolls(t *testing.T) {
	model := testTUIModel()
	model.tuiCfg.DetailPopupMaxLines = 3
	model.detailPopupOpen = true

	updated, _ := model.handleDetailPopupKey("down")
	model = updated.(searchTUIModel)
	if model.detailPopupScroll == 0 {
		t.Fatal("detail popup did not scroll")
	}

	updated, _ = model.handleDetailPopupKey("esc")
	model = updated.(searchTUIModel)
	if model.detailPopupOpen {
		t.Fatal("detail popup stayed open after Esc")
	}
}

func TestSearchTUIColumnResizeAndSave(t *testing.T) {
	oldConfigFile := ConfigFile
	oldConfig := graylogCliConfig
	t.Cleanup(func() {
		ConfigFile = oldConfigFile
		graylogCliConfig = oldConfig
	})
	ConfigFile = filepath.Join(t.TempDir(), "graylog.toml")
	graylogCliConfig = GraylogCliConfig{
		GraylogEndpoint: map[string]*GraylogLogin{"dev2": {Url: "https://example.com", UserToken: "token"}},
		SearchTUI:       defaultSearchTUIConfig(),
	}
	model := testTUIModel()
	model.resizeColumn = 0

	updated, _ := model.handleLogsKey("c")
	model = updated.(searchTUIModel)
	if !model.columnResizeMode {
		t.Fatal("column resize mode did not open")
	}
	before := model.columnWidths()[0]
	updated, _ = model.handleColumnResizeKey("+")
	model = updated.(searchTUIModel)
	if got := model.columnWidths()[0]; got <= before {
		t.Fatalf("column width = %d, want > %d", got, before)
	}
	updated, _ = model.handleColumnResizeKey("esc")
	model = updated.(searchTUIModel)
	if model.prompt.kind != tuiPromptColumnSave {
		t.Fatalf("prompt kind = %q, want save prompt", model.prompt.kind)
	}
	updated, _ = model.handlePromptKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(searchTUIModel)
	if _, err := os.Stat(ConfigFile); err != nil {
		t.Fatalf("saved config missing: %v", err)
	}
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[SearchTUI]") || !strings.Contains(string(data), "[GraylogEndpoint.dev2]") {
		t.Fatalf("saved config did not preserve expected sections:\n%s", string(data))
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

func TestSearchTUILocalFilterCandidatesUseUnfilteredData(t *testing.T) {
	model := testTUIModel()
	model.cachedPages[4] = testTUIResult(4, 4, 12).messages

	model.openPrompt(tuiPromptLocalFilter, "req-006")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("local filter returned nil command, want background calc")
	}
	model.applyCalcResult(cmd().(tuiCalcResult))

	values := model.localFilterValueCandidates("request_id")
	seen := map[string]bool{}
	for _, value := range values {
		seen[value.Value] = true
	}
	if !seen["req-000"] || !seen["req-006"] {
		t.Fatalf("request_id candidates should use unfiltered cached data, got %#v", values)
	}
}

func TestSearchTUILocalFilterCanBeClearedFromPrompt(t *testing.T) {
	model := testTUIModel()
	model.openPrompt(tuiPromptLocalFilter, "req-000")
	updated, cmd := model.applyPrompt()
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("local filter returned nil command, want background calc")
	}
	if model.localFilter == "" {
		t.Fatal("local filter was not applied")
	}

	model.openPrompt(tuiPromptLocalFilter, model.localFilter)
	updated, cmd = model.handlePromptKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("clear filter returned nil command, want background calc")
	}
	if model.localFilter != "" || model.localFilterField != "" || model.localFilterValue != "" {
		t.Fatalf("local filter not cleared: %q %q %q", model.localFilter, model.localFilterField, model.localFilterValue)
	}
}

func TestSearchTUILocalFilterPromptShowsCandidates(t *testing.T) {
	model := testTUIModel()
	model.openPrompt(tuiPromptLocalFilter, "")

	line := model.promptLine()
	for _, want := range []string{"local filter:", "candidates", "application"} {
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
	if !model.detailPopupOpen {
		t.Fatal("group representative detail popup did not open")
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
	selectedField := model.localFilterFields()[1]
	if model.prompt.value != selectedField {
		t.Fatalf("prompt value = %q, want selected field %q", model.prompt.value, selectedField)
	}
	if !strings.Contains(model.promptView(), selectedField) {
		t.Fatalf("prompt view did not show selected field %q: %q", selectedField, model.promptView())
	}

	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(searchTUIModel)
	if model.prompt.stage != tuiLocalFilterValueStage {
		t.Fatalf("prompt stage = %q, want value", model.prompt.stage)
	}
	selectedValue := model.prompt.value

	updated, cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("candidate enter returned nil command, want background calc")
	}
	if model.localFilterField != selectedField || model.localFilterValue != selectedValue {
		t.Fatalf("local filter = %s/%s, want %s/%s", model.localFilterField, model.localFilterValue, selectedField, selectedValue)
	}
	if len(model.visibleMessages()) == 0 {
		t.Fatal("candidate filter removed all generated messages")
	}
}

func TestSearchTUIDetailPopupShowsNestedFieldsAsPrettyValues(t *testing.T) {
	model := testTUIModel()
	model.detailPopupOpen = true

	view := model.View()
	for _, want := range []string{"nested:", "duration_ms", "user-0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail popup missing %q:\n%s", want, view)
		}
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

func TestSearchTUIDetailPopupPreservesJSONMessageString(t *testing.T) {
	model := testTUIModel()
	model.messages = []*client.Message{{Message: map[string]any{
		"timestamp": "2026-06-24T14:00:00.000Z",
		"level":     float64(3),
		"source":    "api-1",
		"message":   `{"event":"checkout","items":[{"sku":"A","qty":2}],"stacktrace":"line1\nline2"}`,
	}}}
	model.cachedPages[0] = model.messages
	model.detailPopupOpen = true

	view := model.View()
	for _, want := range []string{`message: {"event":"checkout"`, "stacktrace", "line1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail popup missing %q:\n%s", want, view)
		}
	}
}
