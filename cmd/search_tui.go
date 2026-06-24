package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jeehoon/graylog-cli/pkg/graylog"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
)

const (
	tuiModeList = "list"

	ansiReset   = "\033[0m"
	ansiReverse = "\033[7m"
)

type searchTUIFetcher func(*graylog.QueryRequest) searchTUIResult

type searchTUIModel struct {
	clientCfg     *client.Config
	qreq          *graylog.QueryRequest
	decoderCfg    *client.DecoderConfig
	fetcher       searchTUIFetcher
	messages      []*client.Message
	lines         []string
	total         uint64
	msgCnt        uint64
	effectiveFrom string
	effectiveTo   string
	selected      int
	listScroll    int
	detailAuto    bool
	detailScroll  int
	err           error
	width         int
	height        int
	loading       bool
	lastRefresh   time.Time
}

type searchTUIResult struct {
	messages      []*client.Message
	lines         []string
	msgCnt        uint64
	total         uint64
	effectiveFrom string
	effectiveTo   string
	err           error
}

type searchTUITick time.Time

func runSearchTUI(clientCfg *client.Config, qreq *graylog.QueryRequest, decoderCfg *client.DecoderConfig) error {
	model := newSearchTUIModel(clientCfg, qreq, decoderCfg, nil)
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func newSearchTUIModel(clientCfg *client.Config, qreq *graylog.QueryRequest, decoderCfg *client.DecoderConfig, fetcher searchTUIFetcher) searchTUIModel {
	if fetcher == nil {
		fetcher = newGraylogTUIFetcher(clientCfg, decoderCfg)
	}
	return searchTUIModel{
		clientCfg:  clientCfg,
		qreq:       qreq,
		decoderCfg: decoderCfg,
		fetcher:    fetcher,
		loading:    true,
	}
}

func newGraylogTUIFetcher(clientCfg *client.Config, decoderCfg *client.DecoderConfig) searchTUIFetcher {
	return func(qreq *graylog.QueryRequest) searchTUIResult {
		res, err := graylog.Search(clientCfg, qreq)
		if err != nil {
			return searchTUIResult{err: err}
		}

		lines, msgCnt, total, err := graylog.RenderMessageLines(qreq, res, decoderCfg)
		if err != nil {
			return searchTUIResult{err: err}
		}

		searchRes := res.SearchTypes[qreq.MessageId]
		result := searchTUIResult{
			lines:  lines,
			msgCnt: msgCnt,
			total:  total,
		}
		if searchRes != nil {
			result.messages = reverseMessages(searchRes.Messages)
			if searchRes.EffectiveTimerange != nil {
				result.effectiveFrom = searchRes.EffectiveTimerange.From
				result.effectiveTo = searchRes.EffectiveTimerange.To
			}
		}
		return result
	}
}

func reverseMessages(messages []*client.Message) []*client.Message {
	out := make([]*client.Message, 0, len(messages))
	for idx := len(messages) - 1; idx >= 0; idx-- {
		out = append(out, messages[idx])
	}
	return out
}

func (m searchTUIModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetch()}
	if m.qreq.Follow {
		cmds = append(cmds, m.tick())
	}
	return tea.Batch(cmds...)
}

func (m searchTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleListKey(msg.String())
	case searchTUITick:
		if m.qreq.Follow && m.qreq.Offset == 0 {
			m.loading = true
			return m, tea.Batch(m.fetch(), m.tick())
		}
		if m.qreq.Follow {
			return m, m.tick()
		}
		return m, nil
	case searchTUIResult:
		m.applyResult(msg)
		return m, nil
	}

	return m, nil
}

func (m searchTUIModel) handleListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		m.detailAuto = !m.detailAuto
		m.clampDetailScroll()
		return m, nil
	case "down":
		if len(m.lines) == 0 {
			return m, nil
		}
		if m.selected < len(m.lines)-1 {
			m.selected++
			m.detailScroll = 0
			m.ensureSelectedVisible()
			return m, nil
		}
		return m.nextPage()
	case "up":
		if len(m.lines) == 0 {
			return m, nil
		}
		if m.selected > 0 {
			m.selected--
			m.detailScroll = 0
			m.ensureSelectedVisible()
			return m, nil
		}
		return m.prevPage(true)
	case "n", "right", "pgdown":
		return m.nextPage()
	case "b", "left", "pgup":
		return m.prevPage(false)
	case "r":
		m.loading = true
		return m, m.fetch()
	case "shift+up", "alt+up", "esc [ 1 ; 2 a", "esc [ 1 ; 3 a":
		m.scrollDetail(-1)
		return m, nil
	case "shift+down", "alt+down", "esc [ 1 ; 2 b", "esc [ 1 ; 3 b":
		m.scrollDetail(1)
		return m, nil
	}
	return m, nil
}

func (m searchTUIModel) nextPage() (tea.Model, tea.Cmd) {
	if uint64(m.qreq.Offset)+m.msgCnt >= m.total {
		return m, nil
	}
	m.qreq.Offset += m.qreq.PageLimit
	m.selected = 0
	m.listScroll = 0
	m.detailScroll = 0
	m.loading = true
	return m, m.fetch()
}

func (m searchTUIModel) prevPage(selectLast bool) (tea.Model, tea.Cmd) {
	if m.qreq.Offset <= 0 {
		return m, nil
	}
	m.qreq.Offset -= m.qreq.PageLimit
	if m.qreq.Offset < 0 {
		m.qreq.Offset = 0
	}
	m.selected = 0
	m.listScroll = 0
	m.detailScroll = 0
	if selectLast {
		m.selected = m.qreq.PageLimit - 1
	}
	m.loading = true
	return m, m.fetch()
}

func (m *searchTUIModel) applyResult(result searchTUIResult) {
	m.lines = result.lines
	m.messages = result.messages
	m.msgCnt = result.msgCnt
	m.total = result.total
	m.effectiveFrom = result.effectiveFrom
	m.effectiveTo = result.effectiveTo
	m.err = result.err
	m.loading = false
	m.lastRefresh = time.Now()
	m.detailScroll = 0
	if len(m.lines) == 0 {
		m.selected = 0
		m.listScroll = 0
		return
	}
	if m.selected >= len(m.lines) {
		m.selected = len(m.lines) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.ensureSelectedVisible()
	m.clampDetailScroll()
}

func (m searchTUIModel) View() string {
	return m.renderList()
}

func (m searchTUIModel) renderList() string {
	var b strings.Builder
	headerLines := m.headerLines("list")
	for _, line := range headerLines {
		b.WriteString(m.clip(line))
		b.WriteByte('\n')
	}

	if m.loading {
		b.WriteString("loading...\n")
		return b.String()
	}
	if m.err != nil {
		fmt.Fprintf(&b, "error: %v\n\npress r to retry or q to quit\n", m.err)
		return b.String()
	}

	listHeight, detailHeight := m.listAndDetailHeights(len(headerLines))
	for idx := 0; idx < listHeight; idx++ {
		lineIdx := m.listScroll + idx
		if lineIdx >= len(m.lines) {
			b.WriteByte('\n')
			continue
		}
		line := m.clip(m.lines[lineIdx])
		if lineIdx == m.selected {
			line = ansiReverse + line + ansiReset
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString(strings.Repeat("-", max(1, m.width)))
	b.WriteByte('\n')
	for _, line := range m.bottomDetailLines(detailHeight) {
		b.WriteString(m.clip(line))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m searchTUIModel) headerLines(mode string) []string {
	page := 1
	if m.qreq.PageLimit > 0 {
		page = m.qreq.Offset/m.qreq.PageLimit + 1
	}

	rangeText := "-"
	if m.effectiveFrom != "" || m.effectiveTo != "" {
		rangeText = fmt.Sprintf("%s ~ %s", m.effectiveFrom, m.effectiveTo)
	}

	help := "up/down move enter auto-detail shift/alt+up/down detail-scroll n/b page r refresh q quit"

	return []string{
		fmt.Sprintf("graylog-cli search | %s | page %d offset %d total %d | range %s", mode, page, m.qreq.Offset, m.total, rangeText),
		fmt.Sprintf("query: %s | %s", m.qreq.UserQuery, help),
	}
}

func (m searchTUIModel) listAndDetailHeights(headerHeight int) (int, int) {
	bodyHeight := m.height - headerHeight - 1
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	detailHeight := bodyHeight * 20 / 100
	if m.detailAuto {
		detailHeight = len(m.selectedPrettyLines())
		maxDetailHeight := bodyHeight / 2
		if detailHeight > maxDetailHeight {
			detailHeight = maxDetailHeight
		}
	}

	if detailHeight < 3 {
		detailHeight = 3
	}
	if detailHeight > bodyHeight/2 {
		detailHeight = bodyHeight / 2
	}
	listHeight := bodyHeight - detailHeight
	if listHeight < 1 {
		listHeight = 1
		detailHeight = bodyHeight - listHeight
	}
	return listHeight, detailHeight
}

func (m *searchTUIModel) ensureSelectedVisible() {
	listHeight := m.visibleListHeight()
	if listHeight <= 0 {
		return
	}
	if m.selected < m.listScroll {
		m.listScroll = m.selected
	}
	if m.selected >= m.listScroll+listHeight {
		m.listScroll = m.selected - listHeight + 1
	}
	if m.listScroll < 0 {
		m.listScroll = 0
	}
}

func (m searchTUIModel) visibleListHeight() int {
	listHeight, _ := m.listAndDetailHeights(len(m.headerLines(tuiModeList)))
	return listHeight
}

func (m searchTUIModel) bottomDetailLines(limit int) []string {
	lines := m.selectedPrettyLines()
	if limit <= 0 || len(lines) <= limit {
		return lines
	}
	if m.detailScroll > len(lines)-limit {
		return lines[len(lines)-limit:]
	}
	if m.detailScroll < 0 {
		return lines[:limit]
	}
	return lines[m.detailScroll : m.detailScroll+limit]
}

func (m searchTUIModel) selectedPrettyLines() []string {
	if len(m.messages) == 0 || m.selected >= len(m.messages) {
		return []string{"no selected log"}
	}
	decoder := client.NewDecoder(m.decoderCfg)
	text, err := client.RenderMessage(decoder, false, m.messages[m.selected], client.OutputPretty)
	if err != nil {
		return []string{fmt.Sprintf("error: %v", err)}
	}
	return strings.Split(text, "\n")
}

func (m *searchTUIModel) scrollDetail(delta int) {
	m.detailScroll += delta
	m.clampDetailScroll()
}

func (m *searchTUIModel) clampDetailScroll() {
	_, detailHeight := m.listAndDetailHeights(len(m.headerLines(tuiModeList)))
	maxScroll := len(m.selectedPrettyLines()) - detailHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
	if m.detailScroll > maxScroll {
		m.detailScroll = maxScroll
	}
}

func (m searchTUIModel) clip(line string) string {
	if m.width <= 0 {
		return line
	}
	if len(line) <= m.width {
		return line
	}
	return line[:m.width]
}

func (m searchTUIModel) fetch() tea.Cmd {
	qreq := *m.qreq
	return func() tea.Msg {
		return m.fetcher(&qreq)
	}
}

func (m searchTUIModel) tick() tea.Cmd {
	interval, err := time.ParseDuration(m.qreq.Refresh)
	if err != nil {
		interval = 5 * time.Second
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return searchTUITick(t)
	})
}
