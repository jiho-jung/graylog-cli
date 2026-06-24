package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jeehoon/graylog-cli/pkg/graylog"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
)

func testTUIModel() searchTUIModel {
	qreq := graylog.NewQueryRequest(nil, "", "*")
	qreq.PageLimit = 3
	qreq.Offset = 0
	qreq.Refresh = "5s"
	qreq.Output = client.OutputCompact

	model := newSearchTUIModel(nil, qreq, client.DefaultDecoderConfig(), func(qreq *graylog.QueryRequest) searchTUIResult {
		return testTUIResult(3, 6)
	})
	model.width = 120
	model.height = 20
	model.applyResult(testTUIResult(3, 6))
	return model
}

func testTUIResult(count int, total uint64) searchTUIResult {
	messages := make([]*client.Message, 0, count)
	lines := make([]string, 0, count)
	for idx := 0; idx < count; idx++ {
		messages = append(messages, &client.Message{Message: map[string]any{
			"timestamp":  "2026-06-22T01:02:03.004Z",
			"level":      float64(3),
			"source":     "api-1",
			"message":    "message",
			"request_id": idx,
		}})
		lines = append(lines, "line")
	}
	return searchTUIResult{
		messages:      messages,
		lines:         lines,
		msgCnt:        uint64(count),
		total:         total,
		effectiveFrom: "2026-06-22T01:00:00.000Z",
		effectiveTo:   "2026-06-22T01:10:00.000Z",
	}
}

func testTUIResultWithMessage(message string, fieldCount int) searchTUIResult {
	msg := map[string]any{
		"timestamp": "2026-06-22T01:02:03.004Z",
		"level":     float64(3),
		"source":    "api-1",
		"message":   message,
	}
	for idx := 0; idx < fieldCount; idx++ {
		msg[fmt.Sprintf("field_%02d", idx)] = idx
	}
	return searchTUIResult{
		messages: []*client.Message{{Message: msg}},
		lines:    []string{"line"},
		msgCnt:   1,
		total:    1,
	}
}

func TestSearchTUIArrowMovementInsidePage(t *testing.T) {
	model := testTUIModel()

	updated, cmd := model.handleListKey("down")
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("down inside page returned command, want nil")
	}
	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1", model.selected)
	}

	updated, cmd = model.handleListKey("up")
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("up inside page returned command, want nil")
	}
	if model.selected != 0 {
		t.Fatalf("selected = %d, want 0", model.selected)
	}
}

func TestSearchTUIDownAtLastRowLoadsNextPage(t *testing.T) {
	model := testTUIModel()
	model.selected = 2

	updated, cmd := model.handleListKey("down")
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("down at last row returned nil command, want fetch command")
	}
	if model.qreq.Offset != 3 {
		t.Fatalf("offset = %d, want 3", model.qreq.Offset)
	}
	if model.selected != 0 {
		t.Fatalf("selected = %d, want 0", model.selected)
	}
}

func TestSearchTUIUpAtFirstRowLoadsPreviousPage(t *testing.T) {
	model := testTUIModel()
	model.qreq.Offset = 3
	model.selected = 0

	updated, cmd := model.handleListKey("up")
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("up at first row returned nil command, want fetch command")
	}
	if model.qreq.Offset != 0 {
		t.Fatalf("offset = %d, want 0", model.qreq.Offset)
	}
}

func TestSearchTUIEnterTogglesAutoDetail(t *testing.T) {
	model := testTUIModel()

	updated, cmd := model.handleListKey("enter")
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("enter returned command, want nil")
	}
	if !model.detailAuto {
		t.Fatal("detailAuto = false, want true")
	}

	updated, cmd = model.handleListKey("enter")
	model = updated.(searchTUIModel)
	if cmd != nil {
		t.Fatal("second enter returned command, want nil")
	}
	if model.detailAuto {
		t.Fatal("detailAuto = true, want false")
	}
}

func TestSearchTUIBottomDetailShowsPrettyFields(t *testing.T) {
	model := testTUIModel()

	lines := model.bottomDetailLines(10)
	got := strings.Join(lines, "\n")
	for _, want := range []string{
		"2026-06-22T01:02:03.004Z ERROR api-1",
		"message: message",
		"request_id: 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("bottom detail = %q, missing %q", got, want)
		}
	}
}

func TestSearchTUIDefaultLayoutUsesTwentyPercentDetail(t *testing.T) {
	model := testTUIModel()

	listHeight, detailHeight := model.listAndDetailHeights(len(model.headerLines(tuiModeList)))
	if detailHeight != 3 {
		t.Fatalf("detailHeight = %d, want minimum 3", detailHeight)
	}
	if listHeight != 14 {
		t.Fatalf("listHeight = %d, want 14", listHeight)
	}
}

func TestSearchTUIAutoDetailShowsShortDetailFully(t *testing.T) {
	model := testTUIModel()
	model.height = 30
	model.detailAuto = true

	_, detailHeight := model.listAndDetailHeights(len(model.headerLines(tuiModeList)))
	if detailHeight != len(model.selectedPrettyLines()) {
		t.Fatalf("detailHeight = %d, want pretty line count %d", detailHeight, len(model.selectedPrettyLines()))
	}
}

func TestSearchTUIAutoDetailCapsAtHalfBody(t *testing.T) {
	model := testTUIModel()
	model.height = 30
	model.applyResult(testTUIResultWithMessage("long message", 30))
	model.detailAuto = true

	_, detailHeight := model.listAndDetailHeights(len(model.headerLines(tuiModeList)))
	bodyHeight := model.height - len(model.headerLines(tuiModeList)) - 1
	if detailHeight != bodyHeight/2 {
		t.Fatalf("detailHeight = %d, want half body %d", detailHeight, bodyHeight/2)
	}
}

func TestSearchTUIDetailScrollChangesVisibleLinesAndClamps(t *testing.T) {
	model := testTUIModel()
	model.height = 20
	model.applyResult(testTUIResultWithMessage("long message", 20))
	model.detailAuto = true

	before := strings.Join(model.bottomDetailLines(5), "\n")
	model.scrollDetail(2)
	after := strings.Join(model.bottomDetailLines(5), "\n")
	if before == after {
		t.Fatal("detail scroll did not change visible lines")
	}

	model.scrollDetail(-100)
	if model.detailScroll != 0 {
		t.Fatalf("detailScroll = %d, want 0", model.detailScroll)
	}
	model.scrollDetail(100)
	_, detailHeight := model.listAndDetailHeights(len(model.headerLines(tuiModeList)))
	maxScroll := len(model.selectedPrettyLines()) - detailHeight
	if model.detailScroll != maxScroll {
		t.Fatalf("detailScroll = %d, want max %d", model.detailScroll, maxScroll)
	}
}

func TestSearchTUISelectionAndPageResetDetailScroll(t *testing.T) {
	model := testTUIModel()
	model.detailScroll = 2

	updated, _ := model.handleListKey("down")
	model = updated.(searchTUIModel)
	if model.detailScroll != 0 {
		t.Fatalf("detailScroll after selection move = %d, want 0", model.detailScroll)
	}

	model.detailScroll = 2
	model.selected = 2
	updated, cmd := model.handleListKey("down")
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("down at page end returned nil command, want fetch command")
	}
	if model.detailScroll != 0 {
		t.Fatalf("detailScroll after page change = %d, want 0", model.detailScroll)
	}
}

func TestSearchTUIApplyResultResetsDetailScroll(t *testing.T) {
	model := testTUIModel()
	model.detailScroll = 2

	model.applyResult(testTUIResult(3, 6))
	if model.detailScroll != 0 {
		t.Fatalf("detailScroll after result = %d, want 0", model.detailScroll)
	}
}

func TestSearchTUIRefreshKeepsQueryAndOffset(t *testing.T) {
	model := testTUIModel()
	model.qreq.Offset = 3
	query := model.qreq.UserQuery

	updated, cmd := model.handleListKey("r")
	model = updated.(searchTUIModel)
	if cmd == nil {
		t.Fatal("refresh returned nil command, want fetch command")
	}
	if model.qreq.Offset != 3 {
		t.Fatalf("offset = %d, want 3", model.qreq.Offset)
	}
	if model.qreq.UserQuery != query {
		t.Fatalf("query = %q, want %q", model.qreq.UserQuery, query)
	}
}
