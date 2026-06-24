package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jeehoon/graylog-cli/pkg/graylog"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/jeehoon/graylog-cli/pkg/timeutil"
)

const (
	tuiMaxCachedPages = 8

	tuiViewLogs      = "logs"
	tuiViewHistogram = "histogram"
	tuiViewDetail    = "detail"

	tuiFooterStatus = "status"
	tuiFooterHelp   = "help"

	tuiPromptNone         = ""
	tuiPromptQuery        = "query"
	tuiPromptRange        = "range"
	tuiPromptApplication  = "application"
	tuiPromptPart         = "part"
	tuiPromptLocalFilter  = "local filter"
	tuiPromptSort         = "sort"
	tuiPromptLimit        = "limit"
	tuiPromptPage         = "page"
	tuiPromptColumns      = "columns"
	tuiPromptDetailSearch = "detail search"

	tuiDetailJSONTree = "JSON Tree"
	tuiDetailPretty   = "Pretty"
	tuiDetailRaw      = "Raw"

	tuiModeRows   = "rows"
	tuiModeGroups = "groups"
)

type searchTUIFetcher func(*graylog.QueryRequest) searchTUIResult

type searchTUIModel struct {
	clientCfg  *client.Config
	qreq       *graylog.QueryRequest
	decoderCfg *client.DecoderConfig
	fetcher    searchTUIFetcher

	view       string
	footerMode string
	prompt     tuiPrompt

	messages      []*client.Message
	cachedPages   map[int][]*client.Message
	cacheOrder    []int
	total         uint64
	msgCnt        uint64
	effectiveFrom string
	effectiveTo   string

	selected    int
	listScroll  int
	columns     []string
	localFilter string
	bodyMode    string

	groups              []tuiGroup
	fieldCandidatesData []tuiFieldCandidate
	calcRunning         bool
	calcProgress        string
	calcGeneration      int

	histogram         []tuiHistogramBucket
	histogramSelected int

	detailTab        string
	detailScroll     int
	detailSearch     string
	detailMatchIndex int

	err         error
	width       int
	height      int
	loading     bool
	lastRefresh time.Time

	logsTable      table.Model
	groupsTable    table.Model
	histogramTable table.Model
	detailViewport viewport.Model
	promptInput    textinput.Model
	help           help.Model
}

type tuiPrompt struct {
	kind     string
	value    string
	selected int
}

type searchTUIResult struct {
	messages      []*client.Message
	lines         []string
	msgCnt        uint64
	total         uint64
	effectiveFrom string
	effectiveTo   string
	histogram     []tuiHistogramBucket
	err           error
}

type tuiHistogramBucket struct {
	From  string
	To    string
	Count uint64
}

type tuiGroup struct {
	Count   int
	Latest  string
	Level   string
	Source  string
	Pattern string
	Message *client.Message
}

type tuiFieldCandidate struct {
	Label  string
	Filter string
	Count  int
}

type tuiCalcResult struct {
	generation int
	groups     []tuiGroup
	candidates []tuiFieldCandidate
}

type searchTUITick time.Time

type tuiHelpKeyMap []key.Binding

func (km tuiHelpKeyMap) ShortHelp() []key.Binding {
	return []key.Binding(km)
}

func (km tuiHelpKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{[]key.Binding(km)}
}

func newTUITable() table.Model {
	styles := table.DefaultStyles()
	styles.Header = lipgloss.NewStyle().Bold(true)
	styles.Cell = lipgloss.NewStyle()
	styles.Selected = lipgloss.NewStyle().Reverse(true)
	return table.New(
		table.WithFocused(true),
		table.WithStyles(styles),
		table.WithKeyMap(tuiTableKeyMap()),
	)
}

func tuiTableKeyMap() table.KeyMap {
	return table.KeyMap{
		LineUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("up", "up"),
		),
		LineDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("down", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "half down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		GotoBottom: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "bottom"),
		),
	}
}

func (m *searchTUIModel) initComponents() {
	m.logsTable = newTUITable()
	m.groupsTable = newTUITable()
	m.histogramTable = newTUITable()
	m.detailViewport = viewport.New(1, 1)
	m.promptInput = textinput.New()
	m.promptInput.Prompt = ""
	m.help = help.New()
	m.refreshComponents()
}

func (m *searchTUIModel) refreshComponents() {
	width := m.componentWidth()
	bodyHeight := m.bodyHeight()
	m.help.Width = width

	m.logsTable.SetWidth(width)
	m.logsTable.SetHeight(bodyHeight)
	m.logsTable.SetColumns(m.tableColumns())
	m.logsTable.SetRows(m.tableRows())
	m.logsTable.SetCursor(m.selected)
	m.logsTable.Focus()

	m.groupsTable.SetWidth(width)
	m.groupsTable.SetHeight(bodyHeight)
	m.groupsTable.SetColumns(groupTableColumns(width))
	m.groupsTable.SetRows(m.groupTableRows())
	m.groupsTable.SetCursor(m.selected)
	m.groupsTable.Focus()

	m.histogramTable.SetWidth(width)
	m.histogramTable.SetHeight(bodyHeight)
	m.histogramTable.SetColumns(histogramTableColumns(width))
	m.histogramTable.SetRows(m.histogramTableRows())
	m.histogramTable.SetCursor(m.histogramSelected)
	m.histogramTable.Focus()

	m.detailViewport.Width = width
	m.detailViewport.Height = bodyHeight
	m.detailViewport.SetContent(strings.Join(m.detailLines(), "\n"))
	m.detailViewport.SetYOffset(m.detailScroll)

	m.promptInput.Width = width
	if m.prompt.kind != tuiPromptNone {
		m.promptInput.Prompt = m.prompt.kind + ": "
		m.promptInput.SetValue(m.prompt.value)
		m.promptInput.Focus()
	} else {
		m.promptInput.Prompt = ""
		m.promptInput.SetValue("")
		m.promptInput.Blur()
	}
}

func (m searchTUIModel) componentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return max(20, m.width)
}

func isTableNavigationKey(key string) bool {
	switch key {
	case "up", "down", "pgup", "pgdown", "home", "end", "ctrl+u", "ctrl+d":
		return true
	default:
		return false
	}
}

func isViewportNavigationKey(key string) bool {
	switch key {
	case "up", "down", "pgup", "pgdown", "home", "end", "ctrl+u", "ctrl+d":
		return true
	default:
		return false
	}
}

func runSearchTUI(clientCfg *client.Config, qreq *graylog.QueryRequest, decoderCfg *client.DecoderConfig) error {
	model := newSearchTUIModel(clientCfg, qreq, decoderCfg, nil)
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func newSearchTUIModel(clientCfg *client.Config, qreq *graylog.QueryRequest, decoderCfg *client.DecoderConfig, fetcher searchTUIFetcher) searchTUIModel {
	if fetcher == nil {
		fetcher = newGraylogTUIFetcher(clientCfg, decoderCfg)
	}
	columns := []string{"timestamp", "level", "source", "message"}
	for _, field := range qreq.Fields {
		if !hasString(columns, field) {
			columns = append(columns, field)
		}
	}
	model := searchTUIModel{
		clientCfg:         clientCfg,
		qreq:              qreq,
		decoderCfg:        decoderCfg,
		fetcher:           fetcher,
		view:              tuiViewLogs,
		footerMode:        tuiFooterStatus,
		columns:           columns,
		bodyMode:          tuiModeRows,
		detailTab:         tuiDetailJSONTree,
		detailMatchIndex:  -1,
		cachedPages:       map[int][]*client.Message{},
		cacheOrder:        []int{},
		loading:           true,
		histogramSelected: 0,
	}
	model.initComponents()
	return model
}

func newGraylogTUIFetcher(clientCfg *client.Config, decoderCfg *client.DecoderConfig) searchTUIFetcher {
	return func(qreq *graylog.QueryRequest) searchTUIResult {
		res, err := graylog.Search(clientCfg, qreq)
		if err != nil {
			return searchTUIResult{err: err}
		}

		searchRes := res.SearchTypes[qreq.MessageId]
		result := searchTUIResult{}
		if searchRes != nil && searchRes.EffectiveTimerange != nil {
			result.effectiveFrom = searchRes.EffectiveTimerange.From
			result.effectiveTo = searchRes.EffectiveTimerange.To
		}

		if qreq.QueryType == graylog.QueryTypeHistogram {
			if searchRes != nil {
				result.histogram = histogramBuckets(searchRes.Rows)
			}
			return result
		}

		lines, msgCnt, total, err := graylog.RenderMessageLines(qreq, res, decoderCfg)
		if err != nil {
			return searchTUIResult{err: err}
		}
		result.lines = lines
		result.msgCnt = msgCnt
		result.total = total
		if searchRes != nil {
			result.messages = reverseMessages(searchRes.Messages)
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

func histogramBuckets(rows []*client.Row) []tuiHistogramBucket {
	buckets := make([]tuiHistogramBucket, 0, len(rows))
	for _, row := range rows {
		if row == nil || len(row.Key) == 0 {
			continue
		}
		count := uint64(0)
		if len(row.Values) > 0 && row.Values[0] != nil && row.Values[0].Value > 0 {
			count = uint64(row.Values[0].Value)
		}
		buckets = append(buckets, tuiHistogramBucket{From: row.Key[0], Count: count})
	}
	for idx := range buckets {
		if idx+1 < len(buckets) {
			buckets[idx].To = buckets[idx+1].From
		}
	}
	return buckets
}

func histogramAllZero(buckets []tuiHistogramBucket) bool {
	if len(buckets) == 0 {
		return true
	}
	for _, bucket := range buckets {
		if bucket.Count != 0 {
			return false
		}
	}
	return true
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
		m.refreshComponents()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case searchTUITick:
		if m.qreq.Follow && m.qreq.Offset == 0 && m.view == tuiViewLogs {
			m.loading = true
			return m, tea.Batch(m.fetch(), m.tick())
		}
		if m.qreq.Follow {
			return m, m.tick()
		}
		return m, nil
	case searchTUIResult:
		m.applyResult(msg)
		if m.view == tuiViewHistogram && msg.histogram != nil {
			m.refreshComponents()
			return m, nil
		}
		m.refreshComponents()
		return m, m.startBackgroundCalc()
	case tuiCalcResult:
		m.applyCalcResult(msg)
		m.refreshComponents()
		return m, nil
	}
	return m, nil
}

func (m searchTUIModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.prompt.kind != tuiPromptNone {
		return m.handlePromptKey(msg)
	}
	switch m.view {
	case tuiViewDetail:
		return m.handleDetailKeyMsg(msg)
	case tuiViewHistogram:
		return m.handleHistogramKeyMsg(msg)
	default:
		return m.handleLogsKeyMsg(msg)
	}
}

func (m searchTUIModel) handleLogsKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isTableNavigationKey(msg.String()) {
		var cmd tea.Cmd
		if m.bodyMode == tuiModeGroups {
			m.groupsTable, cmd = m.groupsTable.Update(msg)
			m.selected = m.groupsTable.Cursor()
		} else {
			m.logsTable, cmd = m.logsTable.Update(msg)
			m.selected = m.logsTable.Cursor()
		}
		m.listScroll = m.selected
		m.detailScroll = 0
		m.refreshComponents()
		return m, cmd
	}
	return m.handleLogsKey(msg.String())
}

func (m searchTUIModel) handleLogsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.calcRunning {
			m.cancelBackgroundCalc()
		}
	case "/":
		m.openPrompt(tuiPromptQuery, m.baseQuery())
	case "t":
		m.openPrompt(tuiPromptRange, m.rangePromptValue())
	case "a":
		m.openPrompt(tuiPromptApplication, m.qreq.Application)
	case "P":
		m.openPrompt(tuiPromptPart, m.qreq.Part)
	case "f":
		m.openPrompt(tuiPromptLocalFilter, m.localFilter)
	case "s":
		m.openPrompt(tuiPromptSort, m.qreq.Sort)
	case "l":
		m.openPrompt(tuiPromptLimit, strconv.Itoa(m.qreq.PageLimit))
	case "g":
		m.openPrompt(tuiPromptPage, strconv.Itoa(m.currentPage()))
	case "c":
		m.openPrompt(tuiPromptColumns, strings.Join(m.columns, ","))
	case "tab":
		m.toggleFooter()
	case "enter":
		if len(m.visibleMessages()) > 0 {
			m.view = tuiViewDetail
			m.detailScroll = 0
			m.detailMatchIndex = -1
		}
	case "down":
		m.moveSelection(1)
	case "up":
		m.moveSelection(-1)
	case "n", "right":
		return m.nextPage()
	case "p", "b", "left":
		return m.prevPage(false)
	case "r":
		m.loading = true
		return m, m.fetch()
	case "G":
		if m.bodyMode == tuiModeRows {
			m.bodyMode = tuiModeGroups
		} else {
			m.bodyMode = tuiModeRows
		}
		m.selected = 0
		m.listScroll = 0
		if len(m.groups) == 0 && len(m.visibleMessages()) > 0 && !m.calcRunning {
			return m, m.startBackgroundCalc()
		}
	case "h":
		m.view = tuiViewHistogram
		m.loading = true
		return m, m.fetchHistogram()
	}
	return m, nil
}

func (m searchTUIModel) handleHistogramKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isTableNavigationKey(msg.String()) {
		var cmd tea.Cmd
		m.histogramTable, cmd = m.histogramTable.Update(msg)
		m.histogramSelected = m.histogramTable.Cursor()
		m.refreshComponents()
		return m, cmd
	}
	return m.handleHistogramKey(msg.String())
}

func (m searchTUIModel) handleHistogramKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = tuiViewLogs
	case "r":
		m.loading = true
		return m, m.fetchHistogram()
	case "down":
		if m.histogramSelected < len(m.histogram)-1 {
			m.histogramSelected++
		}
	case "up":
		if m.histogramSelected > 0 {
			m.histogramSelected--
		}
	case "enter":
		if len(m.histogram) == 0 {
			return m, nil
		}
		bucket := m.histogram[m.histogramSelected]
		to := bucket.To
		if to == "" {
			to = m.effectiveTo
		}
		if bucket.From != "" && to != "" {
			m.qreq.SearchTimeRange = graylog.TimeRange{
				TimeType:      graylog.TimeTypeAbsolute,
				AbsoluteStart: bucket.From,
				AbsoluteEnd:   to,
			}
			m.qreq.Offset = 0
			m.resetSearchState()
			m.view = tuiViewLogs
			m.loading = true
			return m, m.fetch()
		}
	}
	return m, nil
}

func (m searchTUIModel) handleDetailKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isViewportNavigationKey(msg.String()) {
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		m.detailScroll = m.detailViewport.YOffset
		return m, cmd
	}
	return m.handleDetailKey(msg.String())
}

func (m searchTUIModel) handleDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "enter":
		m.view = tuiViewLogs
	case "tab":
		m.nextDetailTab()
	case "down":
		m.scrollDetail(1)
	case "up":
		m.scrollDetail(-1)
	case "pgdown":
		m.scrollDetail(max(1, m.detailHeight()-1))
	case "pgup":
		m.scrollDetail(-max(1, m.detailHeight()-1))
	case "/":
		m.openPrompt(tuiPromptDetailSearch, m.detailSearch)
	case "n":
		m.nextDetailMatch(1)
	case "N":
		m.nextDetailMatch(-1)
	case "g":
		m.detailScroll = 0
	case "G":
		m.detailScroll = max(0, len(m.detailLines())-m.detailHeight())
	}
	return m, nil
}

func (m searchTUIModel) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.prompt.kind == tuiPromptLocalFilter {
		switch msg.String() {
		case "up":
			m.selectLocalFilterCandidate(-1)
			m.refreshComponents()
			return m, nil
		case "down", "tab":
			m.selectLocalFilterCandidate(1)
			m.refreshComponents()
			return m, nil
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.prompt = tuiPrompt{}
		m.refreshComponents()
		return m, nil
	case tea.KeyEnter:
		m.prompt.value = m.promptInput.Value()
		return m.applyPrompt()
	}
	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(msg)
	m.prompt.value = m.promptInput.Value()
	return m, cmd
}

func (m *searchTUIModel) openPrompt(kind, value string) {
	m.prompt = tuiPrompt{kind: kind, value: value}
	if kind == tuiPromptLocalFilter {
		m.prompt.selected = m.localFilterCandidateIndex(value)
	}
	m.refreshComponents()
}

func (m *searchTUIModel) selectLocalFilterCandidate(delta int) {
	if len(m.fieldCandidatesData) == 0 {
		return
	}
	m.prompt.selected += delta
	if m.prompt.selected < 0 {
		m.prompt.selected = 0
	}
	if m.prompt.selected >= len(m.fieldCandidatesData) {
		m.prompt.selected = len(m.fieldCandidatesData) - 1
	}
	m.prompt.value = m.fieldCandidatesData[m.prompt.selected].Filter
}

func (m searchTUIModel) localFilterCandidateIndex(filter string) int {
	for idx, candidate := range m.fieldCandidatesData {
		if candidate.Filter == filter {
			return idx
		}
	}
	return 0
}

func (m searchTUIModel) applyPrompt() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.prompt.value)
	kind := m.prompt.kind
	promptSelected := m.prompt.selected
	m.prompt = tuiPrompt{}

	switch kind {
	case tuiPromptQuery:
		if value == "" {
			m.err = fmt.Errorf("query cannot be empty")
			return m, nil
		}
		m.qreq.BaseQuery = value
		m.applyServerQuery()
		return m.refetchFromFirstPage()
	case tuiPromptRange:
		timerange, err := parseTUITimeRange(value)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.qreq.SearchTimeRange = timerange
		return m.refetchFromFirstPage()
	case tuiPromptApplication:
		m.qreq.Application = value
		m.applyServerQuery()
		return m.refetchFromFirstPage()
	case tuiPromptPart:
		m.qreq.Part = value
		m.applyServerQuery()
		return m.refetchFromFirstPage()
	case tuiPromptSort:
		if !validSort(value) {
			m.err = fmt.Errorf("invalid sort %q: expected field:ASC or field:DESC", value)
			return m, nil
		}
		m.qreq.Sort = normalizeSort(value)
		return m.refetchFromFirstPage()
	case tuiPromptLimit:
		limit, err := strconv.Atoi(value)
		if err != nil || limit <= 0 {
			m.err = fmt.Errorf("invalid limit %q", value)
			return m, nil
		}
		m.qreq.PageLimit = limit
		return m.refetchFromFirstPage()
	case tuiPromptPage:
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 {
			m.err = fmt.Errorf("invalid page %q", value)
			return m, nil
		}
		m.qreq.Offset = (page - 1) * m.qreq.PageLimit
		m.selected = 0
		m.listScroll = 0
		m.loading = true
		return m, m.fetch()
	case tuiPromptColumns:
		cols := parseColumns(value)
		if len(cols) == 0 {
			m.err = fmt.Errorf("columns cannot be empty")
			return m, nil
		}
		m.columns = cols
	case tuiPromptLocalFilter:
		if value == "" && len(m.fieldCandidatesData) > 0 && promptSelected >= 0 && promptSelected < len(m.fieldCandidatesData) {
			value = m.fieldCandidatesData[promptSelected].Filter
		}
		m.localFilter = value
		m.selected = 0
		m.listScroll = 0
		return m, m.startBackgroundCalc()
	case tuiPromptDetailSearch:
		m.detailSearch = value
		m.detailMatchIndex = -1
		if value != "" {
			m.nextDetailMatch(1)
		}
	}
	return m, nil
}

func validSort(sortText string) bool {
	parts := strings.SplitN(sortText, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return false
	}
	order := strings.ToUpper(strings.TrimSpace(parts[1]))
	return order == "ASC" || order == "DESC"
}

func normalizeSort(sortText string) string {
	parts := strings.SplitN(sortText, ":", 2)
	return strings.TrimSpace(parts[0]) + ":" + strings.ToUpper(strings.TrimSpace(parts[1]))
}

func parseTUITimeRange(value string) (graylog.TimeRange, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return graylog.TimeRange{}, fmt.Errorf("range cannot be empty")
	}
	if strings.Contains(value, ",") {
		parts := strings.SplitN(value, ",", 2)
		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])
		if from == "" || to == "" {
			return graylog.TimeRange{}, fmt.Errorf("absolute range must be from,to")
		}
		return graylog.TimeRange{
			TimeType:      graylog.TimeTypeAbsolute,
			AbsoluteStart: from,
			AbsoluteEnd:   to,
		}, nil
	}
	if _, err := timeutil.ParseDuration(value); err != nil {
		return graylog.TimeRange{}, fmt.Errorf("invalid relative range %q: %w", value, err)
	}
	return graylog.TimeRange{TimeType: graylog.TimeTypeRelative, RelativeRange: value}, nil
}

func parseColumns(value string) []string {
	fields := strings.Split(value, ",")
	cols := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" && !hasString(cols, field) {
			cols = append(cols, field)
		}
	}
	return cols
}

func (m *searchTUIModel) applyServerQuery() {
	m.qreq.UserQuery = buildSearchQuery(m.baseQuery(), m.qreq.Application, m.qreq.Part)
}

func (m searchTUIModel) baseQuery() string {
	if strings.TrimSpace(m.qreq.BaseQuery) != "" {
		return m.qreq.BaseQuery
	}
	return m.qreq.UserQuery
}

func (m searchTUIModel) rangePromptValue() string {
	if m.qreq.SearchTimeRange.TimeType == graylog.TimeTypeAbsolute {
		return m.qreq.SearchTimeRange.AbsoluteStart + "," + m.qreq.SearchTimeRange.AbsoluteEnd
	}
	return m.qreq.SearchTimeRange.RelativeRange
}

func (m searchTUIModel) refreshText() string {
	if m.qreq.Follow {
		return "follow " + m.qreq.Refresh
	}
	if m.lastRefresh.IsZero() {
		return "-"
	}
	return m.lastRefresh.Format("15:04:05")
}

func (m searchTUIModel) refetchFromFirstPage() (tea.Model, tea.Cmd) {
	m.qreq.Offset = 0
	m.resetSearchState()
	m.loading = true
	return m, m.fetch()
}

func (m *searchTUIModel) applyResult(result searchTUIResult) {
	m.err = result.err
	m.loading = false
	m.lastRefresh = time.Now()
	if result.err != nil {
		return
	}
	if m.view == tuiViewHistogram && result.histogram != nil {
		m.histogram = result.histogram
		if m.histogramSelected >= len(m.histogram) {
			m.histogramSelected = max(0, len(m.histogram)-1)
		}
		m.effectiveFrom = result.effectiveFrom
		m.effectiveTo = result.effectiveTo
		return
	}
	m.messages = result.messages
	m.storeCachedPage(m.qreq.Offset, result.messages)
	m.msgCnt = uint64(len(result.messages))
	if result.msgCnt != 0 {
		m.msgCnt = result.msgCnt
	}
	m.total = result.total
	m.effectiveFrom = result.effectiveFrom
	m.effectiveTo = result.effectiveTo
	m.detailScroll = 0
	m.clampSelection()
	m.ensureSelectedVisible()
}

func (m *searchTUIModel) applyCalcResult(result tuiCalcResult) {
	if result.generation != m.calcGeneration {
		return
	}
	m.groups = result.groups
	m.fieldCandidatesData = result.candidates
	m.calcRunning = false
	m.calcProgress = fmt.Sprintf("groups %d | candidates %d ready", len(result.groups), len(result.candidates))
	m.clampSelection()
	m.ensureSelectedVisible()
}

func (m *searchTUIModel) cancelBackgroundCalc() {
	m.calcGeneration++
	m.calcRunning = false
	m.calcProgress = "calculation cancelled"
}

func (m *searchTUIModel) startBackgroundCalc() tea.Cmd {
	m.calcGeneration++
	generation := m.calcGeneration
	m.calcRunning = true
	m.calcProgress = "calculating groups and field candidates"
	m.groups = nil
	m.fieldCandidatesData = nil
	messages := append([]*client.Message{}, m.visibleMessages()...)
	columns := append([]string{}, m.columns...)
	decoderCfg := m.decoderCfg
	return func() tea.Msg {
		return tuiCalcResult{
			generation: generation,
			groups:     calculateGroups(messages, decoderCfg),
			candidates: calculateFieldCandidates(messages, columns, decoderCfg),
		}
	}
}

func (m searchTUIModel) View() string {
	m.refreshComponents()
	switch m.view {
	case tuiViewDetail:
		return m.renderDetail()
	case tuiViewHistogram:
		return m.renderHistogram()
	default:
		return m.renderLogs()
	}
}

func (m searchTUIModel) renderLogs() string {
	body := m.logsTable.View()
	if m.bodyMode == tuiModeGroups {
		body = m.groupsTable.View()
	}
	if m.loading && len(m.messages) == 0 {
		body = m.statusViewport("loading...").View()
	}
	if m.err != nil && len(m.messages) == 0 {
		body = m.statusViewport(fmt.Sprintf("error: %v\npress r to retry or q to quit", m.err)).View()
	}
	return m.renderScreen(tuiViewLogs, body)
}

func (m searchTUIModel) renderHistogram() string {
	if m.loading && len(m.histogram) == 0 {
		return m.renderScreen(tuiViewHistogram, m.statusViewport("loading histogram...").View())
	}
	if m.err != nil && len(m.histogram) == 0 {
		return m.renderScreen(tuiViewHistogram, m.statusViewport(fmt.Sprintf("error: %v\npress r to retry or Esc to return", m.err)).View())
	}
	if len(m.histogram) == 0 || histogramAllZero(m.histogram) {
		return m.renderScreen(tuiViewHistogram, m.statusViewport("no histogram data").View())
	}
	return m.renderScreen(tuiViewHistogram, m.histogramTable.View())
}

func (m searchTUIModel) renderDetail() string {
	return m.renderScreen(tuiViewDetail, m.detailViewport.View())
}

func (m searchTUIModel) renderScreen(view string, body string) string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.headerView(view),
		body,
		m.promptView(),
		m.footerView(),
	)
}

func (m searchTUIModel) headerView(view string) string {
	lines := m.headerLines(view)
	style := lipgloss.NewStyle().Width(m.componentWidth())
	return style.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m searchTUIModel) promptView() string {
	if m.prompt.kind != tuiPromptNone {
		prompt := m.promptInput.View()
		if m.prompt.kind == tuiPromptLocalFilter {
			candidates := strings.Join(m.fieldCandidateLabels(m.prompt.selected), " ")
			if candidates != "" {
				prompt = lipgloss.JoinVertical(lipgloss.Left, prompt, "candidates "+candidates)
			}
		}
		return prompt
	}
	return m.promptLine()
}

func (m searchTUIModel) footerView() string {
	if m.footerMode == tuiFooterHelp || m.view == tuiViewDetail || m.view == tuiViewHistogram {
		return m.help.View(m.activeHelpKeyMap())
	}
	return m.footerLine()
}

func (m searchTUIModel) statusViewport(content string) viewport.Model {
	vp := viewport.New(m.componentWidth(), m.bodyHeight())
	vp.SetContent(content)
	return vp
}

func (m searchTUIModel) headerLines(view string) []string {
	page := m.currentPage()
	pageTotal := "?"
	if m.qreq.PageLimit > 0 && m.total > 0 {
		pageTotal = strconv.Itoa(int((m.total + uint64(m.qreq.PageLimit) - 1) / uint64(m.qreq.PageLimit)))
	}
	rangeText := m.rangeText()
	filterState := "off"
	if m.localFilter != "" {
		filterState = "on"
	}
	line1 := fmt.Sprintf("graylog-cli search --tui | view: %s", view)
	if view == tuiViewDetail {
		line1 += " | tab: " + m.detailTab
	}
	if view == tuiViewHistogram && len(m.histogram) > 0 {
		line1 += fmt.Sprintf(" | bucket: %d/%d", m.histogramSelected+1, len(m.histogram))
	}
	line2 := fmt.Sprintf("range: %s | query: %s | page: %d/%s | total: %d", rangeText, m.qreq.UserQuery, page, pageTotal, m.total)
	line3 := fmt.Sprintf("offset: %d | limit: %d | sort: %s | app: %s | part: %s | local filter: %s | refresh: %s", m.qreq.Offset, m.qreq.PageLimit, m.qreq.Sort, emptyDash(m.qreq.Application), emptyDash(m.qreq.Part), filterState, m.refreshText())
	return []string{line1, line2, line3}
}

func (m searchTUIModel) promptLine() string {
	if m.prompt.kind != tuiPromptNone {
		if m.prompt.kind == tuiPromptLocalFilter {
			candidates := strings.Join(m.fieldCandidateLabels(m.prompt.selected), " ")
			if candidates != "" {
				return fmt.Sprintf("%s: %s | candidates %s", m.prompt.kind, m.prompt.value, candidates)
			}
		}
		return fmt.Sprintf("%s: %s", m.prompt.kind, m.prompt.value)
	}
	if m.err != nil {
		return fmt.Sprintf("error: %v", m.err)
	}
	if m.loading {
		return "loading..."
	}
	return ""
}

func (m searchTUIModel) footerLine() string {
	if m.footerMode == tuiFooterHelp || m.view == tuiViewDetail || m.view == tuiViewHistogram {
		switch m.view {
		case tuiViewDetail:
			return "Enter/Esc back | Tab next | / search | n/N match | g/G top/bottom | q quit"
		case tuiViewHistogram:
			return "up/down bucket | Enter narrow range | Esc logs | r retry | q quit"
		default:
			return "/ query | t range | a app | P part | f filter | s sort | l limit | g page | c columns | Enter detail | Tab help | h histogram | G groups | r retry | q quit"
		}
	}
	refreshed := "-"
	if !m.lastRefresh.IsZero() {
		refreshed = m.lastRefresh.Format("15:04:05")
	}
	return fmt.Sprintf("STATUS page %d | row %d/%d | cached pages %d | refreshed %s | %s",
		m.currentPage(), min(m.selected+1, len(m.visibleItems())), len(m.visibleItems()), len(m.cachedPages), refreshed, m.statusSuffix())
}

func (m searchTUIModel) activeHelpKeyMap() tuiHelpKeyMap {
	switch m.view {
	case tuiViewDetail:
		return tuiHelpKeyMap{
			key.NewBinding(key.WithKeys("enter", "esc"), key.WithHelp("enter/esc", "back")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
			key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n/N", "match")),
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		}
	case tuiViewHistogram:
		return tuiHelpKeyMap{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("up/down", "bucket")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "narrow")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "logs")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry")),
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		}
	default:
		return tuiHelpKeyMap{
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "query")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "range")),
			key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "help")),
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		}
	}
}

func (m searchTUIModel) statusSuffix() string {
	if m.err != nil {
		return "r retry"
	}
	if m.loading {
		return "loading"
	}
	if m.calcRunning || m.calcProgress != "" {
		return m.calcProgress
	}
	if m.localFilter != "" {
		return fmt.Sprintf("filter %q | candidates %s", m.localFilter, strings.Join(m.fieldCandidateLabels(-1), " "))
	}
	return "r retry"
}

func (m searchTUIModel) tableColumns() []table.Column {
	widths := m.columnWidths()
	cols := make([]table.Column, 0, len(m.columns))
	for idx, column := range m.columns {
		width := 10
		if idx < len(widths) {
			width = widths[idx]
		}
		cols = append(cols, table.Column{Title: column, Width: width})
	}
	return cols
}

func (m searchTUIModel) tableRows() []table.Row {
	messages := m.visibleMessages()
	rows := make([]table.Row, 0, len(messages))
	for _, msg := range messages {
		row := make(table.Row, 0, len(m.columns))
		for _, column := range m.columns {
			row = append(row, singleLine(m.fieldValue(msg, column)))
		}
		rows = append(rows, row)
	}
	return rows
}

func groupTableColumns(width int) []table.Column {
	return []table.Column{
		{Title: "count", Width: 7},
		{Title: "latest", Width: 20},
		{Title: "level", Width: 8},
		{Title: "source", Width: 16},
		{Title: "message pattern", Width: max(10, width-57)},
	}
}

func (m searchTUIModel) groupTableRows() []table.Row {
	groups := m.visibleGroups()
	rows := make([]table.Row, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, table.Row{
			strconv.Itoa(group.Count),
			group.Latest,
			group.Level,
			group.Source,
			singleLine(group.Pattern),
		})
	}
	if m.calcRunning && len(rows) == 0 {
		return []table.Row{{"0", "", "", "", "calculating similar log groups..."}}
	}
	return rows
}

func histogramTableColumns(width int) []table.Column {
	return []table.Column{
		{Title: "time", Width: 22},
		{Title: "count", Width: 8},
		{Title: "bar", Width: max(5, width-33)},
	}
}

func (m searchTUIModel) histogramTableRows() []table.Row {
	maxCount := uint64(1)
	for _, bucket := range m.histogram {
		if bucket.Count > maxCount {
			maxCount = bucket.Count
		}
	}
	barWidth := max(5, m.componentWidth()-33)
	rows := make([]table.Row, 0, len(m.histogram))
	for _, bucket := range m.histogram {
		fill := int(uint64(barWidth) * bucket.Count / maxCount)
		rows = append(rows, table.Row{
			compactTime(bucket.From),
			strconv.FormatUint(bucket.Count, 10),
			strings.Repeat("■", fill),
		})
	}
	return rows
}

func (m searchTUIModel) columnWidths() []int {
	fixed := map[string]int{"timestamp": 20, "level": 8, "source": 18}
	widths := make([]int, len(m.columns))
	remaining := m.componentWidth() - max(0, len(m.columns)-1)
	flexCount := 0
	for idx, column := range m.columns {
		if width, ok := fixed[column]; ok {
			widths[idx] = width
			remaining -= width
		} else {
			flexCount++
		}
	}
	if flexCount == 0 {
		return widths
	}
	flexWidth := max(8, remaining/flexCount)
	for idx := range widths {
		if widths[idx] == 0 {
			widths[idx] = flexWidth
		}
	}
	return widths
}

func (m searchTUIModel) detailLines() []string {
	msg := m.selectedMessage()
	if msg == nil {
		return []string{"no selected log"}
	}
	switch m.detailTab {
	case tuiDetailPretty:
		decoder := client.NewDecoder(m.decoderCfg)
		text, err := client.RenderMessage(decoder, false, msg, client.OutputPretty)
		if err != nil {
			return []string{fmt.Sprintf("error: %v", err)}
		}
		return strings.Split(text, "\n")
	case tuiDetailRaw:
		return strings.Split(m.rawMessage(msg), "\n")
	default:
		if raw, ok := msg.Message["message"].(string); ok {
			var parsed any
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				return jsonTreeLines(parsed, 0)
			}
		}
		return jsonTreeLines(msg.Message, 0)
	}
}

func jsonTreeLines(value any, indent int) []string {
	prefix := strings.Repeat("  ", indent)
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			child := v[key]
			switch child.(type) {
			case map[string]any, []any:
				lines = append(lines, prefix+key+":")
				lines = append(lines, jsonTreeLines(child, indent+1)...)
			default:
				lines = append(lines, jsonScalarLines(prefix, key, child)...)
			}
		}
		return lines
	case []any:
		lines := make([]string, 0, len(v))
		for idx, child := range v {
			switch child.(type) {
			case map[string]any, []any:
				lines = append(lines, fmt.Sprintf("%s[%d]:", prefix, idx))
				lines = append(lines, jsonTreeLines(child, indent+1)...)
			default:
				lines = append(lines, jsonScalarLines(prefix, fmt.Sprintf("[%d]", idx), child)...)
			}
		}
		return lines
	default:
		return []string{prefix + stringify(v)}
	}
}

func jsonScalarLines(prefix string, key string, value any) []string {
	text := stringify(value)
	if !strings.Contains(text, "\n") {
		return []string{fmt.Sprintf("%s%s: %s", prefix, key, text)}
	}
	lines := []string{prefix + key + ":"}
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, prefix+"  "+line)
	}
	return lines
}

func (m searchTUIModel) rawMessage(msg *client.Message) string {
	if msg == nil {
		return ""
	}
	if value, ok := msg.Message["message"].(string); ok {
		return value
	}
	b, err := json.Marshal(msg.Message)
	if err != nil {
		return fmt.Sprint(msg.Message)
	}
	return string(b)
}

func (m searchTUIModel) visibleMessages() []*client.Message {
	source := m.messages
	if m.localFilter != "" {
		source = m.cachedMessages()
	}
	if m.localFilter == "" {
		return source
	}
	filter := strings.ToLower(m.localFilter)
	out := make([]*client.Message, 0, len(source))
	for _, msg := range source {
		if strings.Contains(strings.ToLower(messageSearchText(msg)), filter) {
			out = append(out, msg)
		}
	}
	return out
}

func (m searchTUIModel) cachedMessages() []*client.Message {
	offsets := make([]int, 0, len(m.cachedPages))
	for offset := range m.cachedPages {
		offsets = append(offsets, offset)
	}
	sort.Ints(offsets)
	out := []*client.Message{}
	for _, offset := range offsets {
		out = append(out, m.cachedPages[offset]...)
	}
	return out
}

func (m searchTUIModel) visibleGroups() []tuiGroup {
	return m.groups
}

func calculateGroups(messages []*client.Message, decoderCfg *client.DecoderConfig) []tuiGroup {
	byPattern := map[string]*tuiGroup{}
	order := []string{}
	for _, msg := range messages {
		pattern := messagePattern(fieldValue(decoderCfg, msg, "message"))
		group, ok := byPattern[pattern]
		if !ok {
			group = &tuiGroup{
				Pattern: pattern,
				Latest:  fieldValue(decoderCfg, msg, "timestamp"),
				Level:   fieldValue(decoderCfg, msg, "level"),
				Source:  fieldValue(decoderCfg, msg, "source"),
				Message: msg,
			}
			byPattern[pattern] = group
			order = append(order, pattern)
		}
		group.Count++
		if fieldValue(decoderCfg, msg, "timestamp") > group.Latest {
			group.Latest = fieldValue(decoderCfg, msg, "timestamp")
			group.Message = msg
		}
		if severityRank(fieldValue(decoderCfg, msg, "level")) > severityRank(group.Level) {
			group.Level = fieldValue(decoderCfg, msg, "level")
		}
	}
	groups := make([]tuiGroup, 0, len(order))
	for _, pattern := range order {
		groups = append(groups, *byPattern[pattern])
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			return groups[i].Latest > groups[j].Latest
		}
		return groups[i].Count > groups[j].Count
	})
	return groups
}

func calculateFieldCandidates(messages []*client.Message, columns []string, decoderCfg *client.DecoderConfig) []tuiFieldCandidate {
	fields := []string{"level", "source", "application", "part"}
	for _, column := range columns {
		if !hasString(fields, column) && column != "timestamp" && column != "message" {
			fields = append(fields, column)
		}
	}
	counts := map[string]int{}
	for _, msg := range messages {
		for _, field := range fields {
			value := fieldValue(decoderCfg, msg, field)
			if value != "" && value != "<nil>" {
				counts[field+"="+value]++
			}
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	if len(keys) > 8 {
		keys = keys[:8]
	}
	candidates := make([]tuiFieldCandidate, 0, len(keys))
	for _, key := range keys {
		candidates = append(candidates, tuiFieldCandidate{
			Label:  fmt.Sprintf("%s(%d)", key, counts[key]),
			Filter: key,
			Count:  counts[key],
		})
	}
	return candidates
}

func messagePattern(message string) string {
	fields := strings.Fields(message)
	for idx, field := range fields {
		if _, err := strconv.ParseFloat(strings.Trim(field, ".,:;()[]{}"), 64); err == nil {
			fields[idx] = "<num>"
		}
	}
	if len(fields) == 0 {
		return message
	}
	return strings.Join(fields, " ")
}

func severityRank(level string) int {
	switch strings.ToUpper(level) {
	case "EMERGENCY":
		return 8
	case "ALERT":
		return 7
	case "CRITICAL":
		return 6
	case "ERROR":
		return 5
	case "WARN", "WARNING":
		return 4
	case "NOTICE":
		return 3
	case "INFO", "INFORMATIONAL":
		return 2
	case "DEBUG":
		return 1
	default:
		return 0
	}
}

func (m searchTUIModel) selectedMessage() *client.Message {
	if m.bodyMode == tuiModeGroups && m.view == tuiViewLogs {
		groups := m.visibleGroups()
		if m.selected >= 0 && m.selected < len(groups) {
			return groups[m.selected].Message
		}
		return nil
	}
	messages := m.visibleMessages()
	if m.selected >= 0 && m.selected < len(messages) {
		return messages[m.selected]
	}
	return nil
}

func (m searchTUIModel) visibleItems() []struct{} {
	count := len(m.visibleMessages())
	if m.bodyMode == tuiModeGroups && m.view == tuiViewLogs {
		count = len(m.visibleGroups())
	}
	return make([]struct{}, count)
}

func (m *searchTUIModel) moveSelection(delta int) {
	count := len(m.visibleItems())
	if count == 0 {
		m.selected = 0
		m.listScroll = 0
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= count {
		m.selected = count - 1
	}
	m.ensureSelectedVisible()
}

func (m *searchTUIModel) clampSelection() {
	count := len(m.visibleItems())
	if count == 0 {
		m.selected = 0
		m.listScroll = 0
		return
	}
	if m.selected >= count {
		m.selected = count - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *searchTUIModel) ensureSelectedVisible() {
	height := max(1, m.bodyHeight()-1)
	if m.selected < m.listScroll {
		m.listScroll = m.selected
	}
	if m.selected >= m.listScroll+height {
		m.listScroll = m.selected - height + 1
	}
	if m.listScroll < 0 {
		m.listScroll = 0
	}
}

func (m searchTUIModel) nextPage() (tea.Model, tea.Cmd) {
	if m.total > 0 && uint64(m.qreq.Offset)+m.msgCnt >= m.total {
		return m, nil
	}
	m.qreq.Offset += m.qreq.PageLimit
	m.selected = 0
	m.listScroll = 0
	m.loading = true
	if cached, ok := m.cachedPages[m.qreq.Offset]; ok {
		m.touchCachedPage(m.qreq.Offset)
		m.messages = cached
		m.loading = false
		m.clampSelection()
		return m, nil
	}
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
	if selectLast {
		m.selected = m.qreq.PageLimit - 1
	}
	m.loading = true
	if cached, ok := m.cachedPages[m.qreq.Offset]; ok {
		m.touchCachedPage(m.qreq.Offset)
		m.messages = cached
		m.loading = false
		m.clampSelection()
		return m, nil
	}
	return m, m.fetch()
}

func (m *searchTUIModel) resetSearchState() {
	m.cachedPages = map[int][]*client.Message{}
	m.cacheOrder = []int{}
	m.messages = nil
	m.selected = 0
	m.listScroll = 0
	m.total = 0
	m.msgCnt = 0
	m.err = nil
}

func (m *searchTUIModel) storeCachedPage(offset int, messages []*client.Message) {
	m.cachedPages[offset] = messages
	m.touchCachedPage(offset)
	for len(m.cacheOrder) > tuiMaxCachedPages {
		evict := m.cacheOrder[0]
		m.cacheOrder = m.cacheOrder[1:]
		delete(m.cachedPages, evict)
	}
}

func (m *searchTUIModel) touchCachedPage(offset int) {
	next := m.cacheOrder[:0]
	for _, cachedOffset := range m.cacheOrder {
		if cachedOffset != offset {
			next = append(next, cachedOffset)
		}
	}
	m.cacheOrder = append(next, offset)
}

func (m *searchTUIModel) scrollDetail(delta int) {
	m.detailScroll += delta
	maxScroll := max(0, len(m.detailLines())-m.detailHeight())
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
	if m.detailScroll > maxScroll {
		m.detailScroll = maxScroll
	}
}

func (m searchTUIModel) detailHeight() int {
	return m.bodyHeight()
}

func (m searchTUIModel) bodyHeight() int {
	if m.height <= 0 {
		return 12
	}
	return max(1, m.height-6)
}

func (m *searchTUIModel) nextDetailTab() {
	switch m.detailTab {
	case tuiDetailJSONTree:
		m.detailTab = tuiDetailPretty
	case tuiDetailPretty:
		m.detailTab = tuiDetailRaw
	default:
		m.detailTab = tuiDetailJSONTree
	}
	m.detailScroll = 0
	m.detailMatchIndex = -1
}

func (m *searchTUIModel) nextDetailMatch(delta int) {
	if m.detailSearch == "" {
		return
	}
	lines := m.detailLines()
	matches := []int{}
	needle := strings.ToLower(m.detailSearch)
	for idx, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			matches = append(matches, idx)
		}
	}
	if len(matches) == 0 {
		m.err = fmt.Errorf("no detail match for %q", m.detailSearch)
		return
	}
	pos := 0
	for idx, lineIdx := range matches {
		if lineIdx == m.detailScroll {
			pos = idx
			break
		}
	}
	if m.detailMatchIndex >= 0 && m.detailMatchIndex < len(matches) {
		pos = m.detailMatchIndex
	}
	pos = (pos + delta + len(matches)) % len(matches)
	m.detailMatchIndex = pos
	m.detailScroll = matches[pos]
	m.err = nil
}

func (m *searchTUIModel) toggleFooter() {
	if m.footerMode == tuiFooterStatus {
		m.footerMode = tuiFooterHelp
	} else {
		m.footerMode = tuiFooterStatus
	}
}

func (m searchTUIModel) fetch() tea.Cmd {
	qreq := *m.qreq
	qreq.QueryType = graylog.QueryTypeMessage
	return func() tea.Msg {
		return m.fetcher(&qreq)
	}
}

func (m searchTUIModel) fetchHistogram() tea.Cmd {
	qreq := *m.qreq
	qreq.QueryType = graylog.QueryTypeHistogram
	qreq.Offset = 0
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

func (m searchTUIModel) currentPage() int {
	if m.qreq.PageLimit <= 0 {
		return 1
	}
	return m.qreq.Offset/m.qreq.PageLimit + 1
}

func (m searchTUIModel) rangeText() string {
	if m.qreq.SearchTimeRange.TimeType == graylog.TimeTypeAbsolute {
		return fmt.Sprintf("%s ~ %s", emptyDash(m.qreq.SearchTimeRange.AbsoluteStart), emptyDash(m.qreq.SearchTimeRange.AbsoluteEnd))
	}
	if m.effectiveFrom != "" || m.effectiveTo != "" {
		return fmt.Sprintf("%s ~ %s", emptyDash(m.effectiveFrom), emptyDash(m.effectiveTo))
	}
	return "last " + m.qreq.SearchTimeRange.RelativeRange
}

func (m searchTUIModel) fieldValue(msg *client.Message, field string) string {
	return fieldValue(m.decoderCfg, msg, field)
}

func fieldValue(decoderCfg *client.DecoderConfig, msg *client.Message, field string) string {
	if msg == nil {
		return ""
	}
	if field == "timestamp" {
		return timeutil.Format(client.NewDecoder(decoderCfg).Timestamp(msg.Message))
	}
	if field == "level" {
		return client.NewDecoder(decoderCfg).Level(msg.Message).String()
	}
	if field == "source" {
		return client.NewDecoder(decoderCfg).Hostname(msg.Message)
	}
	if field == "message" {
		return client.NewDecoder(decoderCfg).Text(msg.Message)
	}
	return stringify(msg.Message[field])
}

func (m searchTUIModel) fieldCandidateLabels(selected int) []string {
	limit := min(5, len(m.fieldCandidatesData))
	labels := make([]string, 0, limit)
	for idx := 0; idx < limit; idx++ {
		label := m.fieldCandidatesData[idx].Label
		if idx == selected {
			label = "▶" + label
		}
		labels = append(labels, label)
	}
	return labels
}

func messageSearchText(msg *client.Message) string {
	if msg == nil {
		return ""
	}
	parts := make([]string, 0, len(msg.Message))
	for key, value := range msg.Message {
		parts = append(parts, key+"="+stringify(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func compactTime(value string) string {
	value = strings.TrimSuffix(value, ".000Z")
	value = strings.TrimSuffix(value, "Z")
	value = strings.ReplaceAll(value, "T", " ")
	return value
}

func inferredField(messages []*client.Message, field string) string {
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		value := stringify(msg.Message[field])
		if value != "" {
			return value
		}
	}
	return "-"
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
