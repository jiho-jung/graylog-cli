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
	tuiPromptColumnSave   = "save columns"

	tuiDetailJSONTree = "JSON Tree"
	tuiDetailPretty   = "Pretty"
	tuiDetailRaw      = "Raw"

	tuiModeRows   = "rows"
	tuiModeGroups = "groups"

	tuiRowNumberAbsolute = "absolute"
	tuiRowNumberPage     = "page"

	tuiLocalFilterFieldStage = "field"
	tuiLocalFilterValueStage = "value"
)

type searchTUIFetcher func(*graylog.QueryRequest) searchTUIResult

type searchTUIModel struct {
	clientCfg  *client.Config
	qreq       *graylog.QueryRequest
	decoderCfg *client.DecoderConfig
	fetcher    searchTUIFetcher
	tuiCfg     SearchTUIConfig

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

	selected         int
	listScroll       int
	columns          []string
	colWidths        []int
	localFilter      string
	localFilterField string
	localFilterValue string
	bodyMode         string
	expanded         map[string]bool

	detailPopupOpen   bool
	detailPopupScroll int

	columnResizeMode bool
	resizeColumn     int

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
	headerViewport viewport.Model
	bodyViewport   viewport.Model
	footerViewport viewport.Model
	detailViewport viewport.Model
	promptInput    textinput.Model
	help           help.Model
}

type tuiPrompt struct {
	kind     string
	value    string
	selected int
	stage    string
	field    string
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
	Field  string
	Value  string
	Count  int
}

type tuiCalcResult struct {
	generation int
	groups     []tuiGroup
	candidates []tuiFieldCandidate
}

type searchTUITick time.Time

type tuiHelpKeyMap []key.Binding

func defaultSearchTUIConfig() SearchTUIConfig {
	return SearchTUIConfig{
		ExpandedMaxLines:                  0,
		DetailPopupMaxLines:               20,
		RowNumberWidth:                    4,
		RowNumberMode:                     tuiRowNumberAbsolute,
		MaxCachedPages:                    tuiMaxCachedPages,
		PersistExpandedRows:               true,
		FollowRefreshPausesOnNonfirstPage: true,
		MessageWrap:                       true,
		CellOverflow:                      "clip",
		MinColumnWidth:                    4,
		ColumnResizeStep:                  2,
		PageScrollStep:                    10,
		DetailPopupWidthRatio:             0.85,
		DetailPopupPosition:               "center",
		DetailShowEmptyFields:             false,
		DetailFields:                      []string{"timestamp", "level", "source", "message", "application", "part"},
		FilterCandidateFields:             []string{"level", "source", "application", "part", "message"},
		FilterCandidateLimit:              20,
		MessagePatternCandidateLimit:      20,
		FilterMatchMode:                   "contains",
		FilterCaseSensitive:               false,
		HeaderVisible:                     true,
		FooterVisible:                     true,
		HeaderBoxGap:                      1,
		FooterBoxGap:                      1,
		BoxOverflow:                       "clip",
		Columns: []SearchTUIColumn{
			{Field: "timestamp", Width: 24},
			{Field: "level", Width: 8},
			{Field: "application", Width: 8},
			{Field: "source", Width: 18},
			{Field: "message", Width: 0},
		},
		HeaderBoxes: []SearchTUIOutputBox{
			{Name: "view", Width: 16},
			{Name: "range", Width: 28},
			{Name: "query", Width: 40},
			{Name: "page", Width: 14},
			{Name: "sort", Width: 24},
			{Name: "filter", Width: 18},
		},
		FooterBoxes: []SearchTUIOutputBox{
			{Name: "status", Width: 18},
			{Name: "row", Width: 14},
			{Name: "cached-pages", Width: 16},
			{Name: "refreshed", Width: 18},
		},
	}
}

func normalizedSearchTUIConfig(cfg SearchTUIConfig) SearchTUIConfig {
	def := defaultSearchTUIConfig()
	if cfg.ExpandedMaxLines < 0 {
		cfg.ExpandedMaxLines = def.ExpandedMaxLines
	}
	if cfg.DetailPopupMaxLines <= 0 {
		cfg.DetailPopupMaxLines = def.DetailPopupMaxLines
	}
	if cfg.RowNumberWidth <= 0 {
		cfg.RowNumberWidth = def.RowNumberWidth
	}
	if cfg.RowNumberMode != tuiRowNumberPage {
		cfg.RowNumberMode = tuiRowNumberAbsolute
	}
	if cfg.MaxCachedPages <= 0 {
		cfg.MaxCachedPages = def.MaxCachedPages
	}
	if cfg.CellOverflow == "" {
		cfg.CellOverflow = def.CellOverflow
	}
	if cfg.MinColumnWidth <= 0 {
		cfg.MinColumnWidth = def.MinColumnWidth
	}
	if cfg.ColumnResizeStep <= 0 {
		cfg.ColumnResizeStep = def.ColumnResizeStep
	}
	if cfg.PageScrollStep <= 0 {
		cfg.PageScrollStep = def.PageScrollStep
	}
	if cfg.DetailPopupWidthRatio <= 0 || cfg.DetailPopupWidthRatio > 1 {
		cfg.DetailPopupWidthRatio = def.DetailPopupWidthRatio
	}
	switch cfg.DetailPopupPosition {
	case "center", "right", "bottom":
	default:
		cfg.DetailPopupPosition = def.DetailPopupPosition
	}
	if len(cfg.DetailFields) == 0 {
		cfg.DetailFields = def.DetailFields
	}
	if len(cfg.FilterCandidateFields) == 0 {
		cfg.FilterCandidateFields = def.FilterCandidateFields
	}
	if cfg.FilterCandidateLimit <= 0 {
		cfg.FilterCandidateLimit = def.FilterCandidateLimit
	}
	if cfg.MessagePatternCandidateLimit <= 0 {
		cfg.MessagePatternCandidateLimit = def.MessagePatternCandidateLimit
	}
	if cfg.FilterMatchMode == "" {
		cfg.FilterMatchMode = def.FilterMatchMode
	}
	if cfg.HeaderBoxGap < 0 {
		cfg.HeaderBoxGap = def.HeaderBoxGap
	}
	if cfg.FooterBoxGap < 0 {
		cfg.FooterBoxGap = def.FooterBoxGap
	}
	if cfg.BoxOverflow == "" {
		cfg.BoxOverflow = def.BoxOverflow
	}
	if len(cfg.Columns) == 0 {
		cfg.Columns = def.Columns
	}
	if len(cfg.HeaderBoxes) == 0 {
		cfg.HeaderBoxes = def.HeaderBoxes
	}
	cfg.HeaderBoxes = normalizedHeaderBoxes(cfg.HeaderBoxes, def.HeaderBoxes)
	if len(cfg.FooterBoxes) == 0 {
		cfg.FooterBoxes = def.FooterBoxes
	}
	return cfg
}

func normalizedHeaderBoxes(boxes []SearchTUIOutputBox, fallback []SearchTUIOutputBox) []SearchTUIOutputBox {
	out := make([]SearchTUIOutputBox, 0, len(boxes))
	for _, box := range boxes {
		if box.Name == "refresh" {
			continue
		}
		out = append(out, box)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

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
	m.headerViewport = viewport.New(1, 1)
	m.bodyViewport = viewport.New(1, 1)
	m.footerViewport = viewport.New(1, 1)
	m.detailViewport = viewport.New(1, 1)
	m.promptInput = textinput.New()
	m.promptInput.Prompt = ""
	m.help = help.New()
	m.refreshComponents()
}

func (m *searchTUIModel) refreshComponents() {
	width := m.viewportContentWidth()
	m.help.Width = width

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

	bodyHeight := m.bodyHeight()

	m.headerViewport.Width = width
	m.headerViewport.Height = m.headerContentHeight(m.view)
	m.headerViewport.SetContent(strings.Join(m.headerLines(m.view), "\n"))
	m.headerViewport.SetYOffset(0)

	m.footerViewport.Width = width
	m.footerViewport.Height = m.footerContentHeight()
	m.footerViewport.SetContent(strings.Join(m.footerLines(), "\n"))
	m.footerViewport.SetYOffset(0)

	m.bodyViewport.Width = width
	m.bodyViewport.Height = bodyHeight

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
}

func (m searchTUIModel) componentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return max(20, m.width)
}

func (m searchTUIModel) viewportContentWidth() int {
	return max(1, m.componentWidth()-2)
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
	tuiCfg := normalizedSearchTUIConfig(graylogCliConfig.SearchTUI)
	if len(graylogCliConfig.SearchTUI.Columns) == 0 && len(graylogCliConfig.SearchTUI.HeaderBoxes) == 0 && len(graylogCliConfig.SearchTUI.FooterBoxes) == 0 && graylogCliConfig.SearchTUI.ExpandedMaxLines == 0 {
		tuiCfg = defaultSearchTUIConfig()
	}
	columns, colWidths := searchTUIColumns(tuiCfg, qreq.Fields)
	model := searchTUIModel{
		clientCfg:         clientCfg,
		qreq:              qreq,
		decoderCfg:        decoderCfg,
		fetcher:           fetcher,
		tuiCfg:            tuiCfg,
		view:              tuiViewLogs,
		footerMode:        tuiFooterStatus,
		columns:           columns,
		colWidths:         colWidths,
		bodyMode:          tuiModeRows,
		detailTab:         tuiDetailJSONTree,
		detailMatchIndex:  -1,
		cachedPages:       map[int][]*client.Message{},
		cacheOrder:        []int{},
		expanded:          map[string]bool{},
		loading:           true,
		histogramSelected: 0,
		resizeColumn:      -1,
		detailPopupScroll: 0,
	}
	model.initComponents()
	return model
}

func searchTUIColumns(tuiCfg SearchTUIConfig, fields []string) ([]string, []int) {
	columns := make([]string, 0, len(tuiCfg.Columns)+len(fields))
	widths := make([]int, 0, len(tuiCfg.Columns)+len(fields))
	messageWidth := 0
	hasMessage := false
	for _, column := range tuiCfg.Columns {
		field := strings.TrimSpace(column.Field)
		if field == "" || hasString(columns, field) {
			continue
		}
		if field == "message" {
			hasMessage = true
			messageWidth = column.Width
			continue
		}
		columns = append(columns, field)
		widths = append(widths, max(tuiCfg.MinColumnWidth, column.Width))
	}
	if len(columns) == 0 {
		for _, column := range defaultSearchTUIConfig().Columns {
			columns = append(columns, column.Field)
			widths = append(widths, column.Width)
		}
	}
	for _, field := range fields {
		if field == "message" {
			hasMessage = true
			continue
		}
		if !hasString(columns, field) {
			columns = append(columns, field)
			widths = append(widths, 0)
		}
	}
	if hasMessage {
		columns = append(columns, "message")
		widths = append(widths, messageWidth)
	}
	return columns, widths
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
		canRefresh := m.view == tuiViewLogs
		if m.tuiCfg.FollowRefreshPausesOnNonfirstPage {
			canRefresh = canRefresh && m.qreq.Offset == 0
		}
		if m.qreq.Follow && canRefresh {
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
	if m.detailPopupOpen {
		return m.handleDetailPopupKey(msg.String())
	}
	if m.columnResizeMode {
		return m.handleColumnResizeKey(msg.String())
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
		key := msg.String()
		switch key {
		case "down":
			m.moveSelection(1)
		case "up":
			m.moveSelection(-1)
		case "pgdown":
			m.moveSelection(m.tuiCfg.PageScrollStep)
		case "pgup":
			m.moveSelection(-m.tuiCfg.PageScrollStep)
		case "ctrl+d":
			m.moveSelection(max(1, m.tuiCfg.PageScrollStep/2))
		case "ctrl+u":
			m.moveSelection(-max(1, m.tuiCfg.PageScrollStep/2))
		case "home":
			m.selected = 0
			m.listScroll = 0
		case "end":
			m.selected = max(0, len(m.visibleItems())-1)
			m.ensureSelectedVisible()
		}
		m.detailScroll = 0
		m.refreshComponents()
		return m, nil
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
		m.columnResizeMode = true
		if m.resizeColumn < 0 && len(m.columns) > 0 {
			m.resizeColumn = 0
		}
	case "tab":
		m.toggleFooter()
	case "enter":
		if len(m.visibleMessages()) > 0 {
			m.detailPopupOpen = true
			m.detailPopupScroll = 0
		}
	case " ":
		m.toggleExpandedSelected()
	case "space":
		m.toggleExpandedSelected()
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

func (m searchTUIModel) handleDetailPopupKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.detailPopupOpen = false
		m.detailPopupScroll = 0
	case "down":
		m.scrollDetailPopup(1)
	case "up":
		m.scrollDetailPopup(-1)
	case "pgdown":
		m.scrollDetailPopup(max(1, m.detailPopupHeight()-1))
	case "pgup":
		m.scrollDetailPopup(-max(1, m.detailPopupHeight()-1))
	case "home":
		m.detailPopupScroll = 0
	case "end":
		m.detailPopupScroll = max(0, len(m.detailPopupLines())-m.detailPopupHeight())
	}
	return m, nil
}

func (m searchTUIModel) handleColumnResizeKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.columnResizeMode = false
		m.openPrompt(tuiPromptColumnSave, "save column widths? y/N")
	case "left":
		if m.resizeColumn > 0 {
			m.resizeColumn--
		}
	case "right":
		if m.resizeColumn < len(m.columns)-1 {
			m.resizeColumn++
		}
	case "+", "=":
		m.resizeSelectedColumn(m.tuiCfg.ColumnResizeStep)
	case "-":
		m.resizeSelectedColumn(-m.tuiCfg.ColumnResizeStep)
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			idx := int(key[0] - '1')
			if idx < len(m.columns) {
				m.resizeColumn = idx
			}
		}
	}
	m.refreshComponents()
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
	if m.prompt.kind == tuiPromptColumnSave {
		switch strings.ToLower(msg.String()) {
		case "y":
			m.prompt = tuiPrompt{}
			if err := saveSearchTUIColumns(m.currentTUIColumns()); err != nil {
				m.err = err
			} else {
				m.err = nil
				m.calcProgress = "column widths saved"
			}
			m.refreshComponents()
			return m, nil
		case "n", "esc":
			m.prompt = tuiPrompt{}
			m.refreshComponents()
			return m, nil
		}
		return m, nil
	}
	if m.prompt.kind == tuiPromptLocalFilter {
		switch msg.String() {
		case "ctrl+u":
			return m.clearLocalFilter()
		case "up":
			m.selectLocalFilterCandidate(-1)
			m.refreshComponents()
			return m, nil
		case "down", "tab":
			m.selectLocalFilterCandidate(1)
			m.refreshComponents()
			return m, nil
		case "enter":
			if m.prompt.stage == tuiLocalFilterFieldStage {
				m.acceptLocalFilterField()
				m.refreshComponents()
				return m, nil
			}
		case "esc":
			if m.prompt.stage == tuiLocalFilterValueStage {
				m.prompt.stage = tuiLocalFilterFieldStage
				m.prompt.field = ""
				m.prompt.selected = 0
				m.prompt.value = ""
				m.refreshComponents()
				return m, nil
			}
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

func (m searchTUIModel) clearLocalFilter() (tea.Model, tea.Cmd) {
	m.prompt = tuiPrompt{}
	m.localFilter = ""
	m.localFilterField = ""
	m.localFilterValue = ""
	m.selected = 0
	m.listScroll = 0
	m.refreshComponents()
	return m, m.startBackgroundCalc()
}

func (m *searchTUIModel) openPrompt(kind, value string) {
	m.prompt = tuiPrompt{kind: kind, value: value}
	if kind == tuiPromptLocalFilter {
		m.prompt.stage = tuiLocalFilterFieldStage
		m.prompt.selected = m.localFilterFieldIndex(m.localFilterField)
		m.prompt.value = value
	}
	m.refreshComponents()
}

func (m *searchTUIModel) selectLocalFilterCandidate(delta int) {
	if m.prompt.stage == tuiLocalFilterValueStage {
		values := m.localFilterValueCandidates(m.prompt.field)
		if len(values) == 0 {
			return
		}
		m.prompt.selected += delta
		if m.prompt.selected < 0 {
			m.prompt.selected = 0
		}
		if m.prompt.selected >= len(values) {
			m.prompt.selected = len(values) - 1
		}
		m.prompt.value = values[m.prompt.selected].Value
		return
	}
	fields := m.localFilterFields()
	if len(fields) == 0 {
		return
	}
	m.prompt.selected += delta
	if m.prompt.selected < 0 {
		m.prompt.selected = 0
	}
	if m.prompt.selected >= len(fields) {
		m.prompt.selected = len(fields) - 1
	}
	m.prompt.value = fields[m.prompt.selected]
}

func (m *searchTUIModel) acceptLocalFilterField() {
	fields := m.localFilterFields()
	if len(fields) == 0 {
		return
	}
	if m.prompt.selected < 0 || m.prompt.selected >= len(fields) {
		m.prompt.selected = 0
	}
	m.prompt.field = fields[m.prompt.selected]
	m.prompt.stage = tuiLocalFilterValueStage
	m.prompt.selected = 0
	values := m.localFilterValueCandidates(m.prompt.field)
	if len(values) > 0 {
		m.prompt.value = values[0].Value
	}
}

func (m searchTUIModel) localFilterFieldIndex(field string) int {
	fields := m.localFilterFields()
	for idx, candidate := range fields {
		if candidate == field {
			return idx
		}
	}
	return 0
}

func (m searchTUIModel) applyPrompt() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.prompt.value)
	kind := m.prompt.kind
	promptSelected := m.prompt.selected
	promptField := m.prompt.field
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
		field := promptField
		if value == "" {
			m.localFilterField = ""
			m.localFilterValue = ""
			m.localFilter = ""
			m.selected = 0
			m.listScroll = 0
			return m, m.startBackgroundCalc()
		}
		if field != "" {
			values := m.localFilterValueCandidates(field)
			if value == "" && len(values) > 0 && promptSelected >= 0 && promptSelected < len(values) {
				value = values[promptSelected].Value
			}
			m.localFilterField = field
			m.localFilterValue = value
			if value != "" {
				m.localFilter = fmt.Sprintf("%s contains %q", field, value)
			} else {
				m.localFilter = ""
			}
		} else {
			m.localFilterField = ""
			m.localFilterValue = ""
			m.localFilter = value
		}
		m.selected = 0
		m.listScroll = 0
		return m, m.startBackgroundCalc()
	case tuiPromptDetailSearch:
		m.detailSearch = value
		m.detailMatchIndex = -1
		if value != "" {
			m.nextDetailMatch(1)
		}
	case tuiPromptColumnSave:
		// handled by handlePromptKey
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
	groupMessages := append([]*client.Message{}, m.visibleMessages()...)
	candidateMessages := append([]*client.Message{}, m.unfilteredMessages()...)
	columns := append([]string{}, m.columns...)
	decoderCfg := m.decoderCfg
	tuiCfg := m.tuiCfg
	return func() tea.Msg {
		return tuiCalcResult{
			generation: generation,
			groups:     calculateGroups(groupMessages, decoderCfg),
			candidates: calculateFieldCandidates(candidateMessages, columns, decoderCfg, tuiCfg),
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
	body := m.renderLogBody()
	if m.bodyMode == tuiModeGroups {
		body = m.groupsTable.View()
	}
	if m.loading && len(m.messages) == 0 {
		body = m.statusViewport("loading...").View()
	}
	if m.err != nil && len(m.messages) == 0 {
		body = m.statusViewport(fmt.Sprintf("error: %v\npress r to retry or q to quit", m.err)).View()
	}
	if m.detailPopupOpen {
		body = m.renderBodyWithDetailPopup(body)
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

func (m *searchTUIModel) renderScreen(view string, body string) string {
	m.headerViewport.Width = m.viewportContentWidth()
	m.headerViewport.Height = m.headerContentHeight(view)
	m.headerViewport.SetContent(strings.Join(m.headerLines(view), "\n"))
	m.headerViewport.SetYOffset(0)

	m.bodyViewport.Width = m.viewportContentWidth()
	m.bodyViewport.Height = m.bodyHeight()
	m.bodyViewport.SetContent(body)

	m.footerViewport.Width = m.viewportContentWidth()
	m.footerViewport.Height = m.footerContentHeight()
	m.footerViewport.SetContent(strings.Join(m.footerLines(), "\n"))
	m.footerViewport.SetYOffset(0)

	parts := []string{}
	for _, part := range []string{m.headerView(view), m.bodyView(), m.footerView()} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n")
}

func (m searchTUIModel) headerView(view string) string {
	if !m.tuiCfg.HeaderVisible {
		return ""
	}
	return m.viewportBox(m.headerViewport.View(), m.headerViewport.Height)
}

func (m searchTUIModel) bodyView() string {
	return m.viewportBox(m.bodyViewport.View(), m.bodyViewport.Height)
}

func (m searchTUIModel) promptView() string {
	if m.prompt.kind != tuiPromptNone {
		prompt := m.promptInput.View()
		if m.prompt.kind == tuiPromptLocalFilter {
			candidates := strings.Join(m.localFilterCandidateLabels(), " ")
			if candidates != "" {
				prompt = lipgloss.JoinVertical(lipgloss.Left, prompt, "Ctrl+U clear | candidates "+candidates)
			}
		}
		return prompt
	}
	return m.promptLine()
}

func (m searchTUIModel) footerView() string {
	if !m.tuiCfg.FooterVisible {
		return ""
	}
	return m.viewportBox(m.footerViewport.View(), m.footerViewport.Height)
}

func (m searchTUIModel) viewportBox(content string, height int) string {
	return lipgloss.NewStyle().
		Width(m.viewportContentWidth()).
		Height(max(1, height)).
		Border(lipgloss.NormalBorder()).
		Render(content)
}

func (m searchTUIModel) statusViewport(content string) viewport.Model {
	vp := viewport.New(m.viewportContentWidth(), m.bodyHeight())
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
	values := map[string]string{
		"view":    "view: " + view,
		"range":   "range: " + rangeText,
		"query":   "query: " + m.qreq.UserQuery,
		"page":    fmt.Sprintf("page: %d/%s", page, pageTotal),
		"offset":  fmt.Sprintf("offset: %d", m.qreq.Offset),
		"limit":   fmt.Sprintf("limit: %d", m.qreq.PageLimit),
		"sort":    "sort: " + m.qreq.Sort,
		"filter":  "filter: " + filterState,
		"refresh": "refresh: " + m.refreshText(),
	}
	if view == tuiViewHistogram && len(m.histogram) > 0 {
		values["view"] = fmt.Sprintf("view: %s bucket: %d/%d", view, m.histogramSelected+1, len(m.histogram))
	}
	return m.outputBoxLines(m.tuiCfg.HeaderBoxes, values, m.tuiCfg.HeaderBoxGap)
}

func (m searchTUIModel) legacyHeaderLines(view string) []string {
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
			candidates := strings.Join(m.localFilterCandidateLabels(), " ")
			if candidates != "" {
				return fmt.Sprintf("%s: %s | Ctrl+U clear | candidates %s", m.prompt.kind, m.prompt.value, candidates)
			}
		}
		return fmt.Sprintf("%s: %s", m.prompt.kind, m.prompt.value)
	}
	if m.err != nil {
		return fmt.Sprintf("error: %v", m.err)
	}
	if m.loading && len(m.messages) == 0 {
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
	values := map[string]string{
		"status":       m.statusSuffix(),
		"page":         fmt.Sprintf("page %d", m.currentPage()),
		"row":          fmt.Sprintf("row %d/%d", min(m.selected+1, len(m.visibleItems())), len(m.visibleItems())),
		"cached-pages": fmt.Sprintf("cached %d", len(m.cachedPages)),
		"refreshed":    "refreshed " + refreshed,
	}
	lines := m.outputBoxLines(m.tuiCfg.FooterBoxes, values, m.tuiCfg.FooterBoxGap)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func (m searchTUIModel) footerLines() []string {
	lines := []string{}
	if prompt := m.promptView(); prompt != "" {
		lines = append(lines, strings.Split(prompt, "\n")...)
	}
	if m.footerMode == tuiFooterHelp || m.view == tuiViewDetail || m.view == tuiViewHistogram {
		help := m.help.View(m.activeHelpKeyMap())
		if help != "" {
			lines = append(lines, strings.Split(help, "\n")...)
		}
	} else if line := m.footerLine(); line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (m searchTUIModel) headerContentHeight(view string) int {
	if !m.tuiCfg.HeaderVisible {
		return 0
	}
	return max(1, len(m.headerLines(view)))
}

func (m searchTUIModel) footerContentHeight() int {
	if !m.tuiCfg.FooterVisible {
		return 0
	}
	return max(1, len(m.footerLines()))
}

func (m searchTUIModel) frameHeight(visible bool, contentHeight int) int {
	if !visible {
		return 0
	}
	return max(1, contentHeight) + 2
}

func (m searchTUIModel) outputBoxLines(boxes []SearchTUIOutputBox, values map[string]string, gap int) []string {
	if len(boxes) == 0 {
		return nil
	}
	width := m.viewportContentWidth()
	lines := []string{}
	current := ""
	separator := strings.Repeat(" ", max(0, gap))
	for _, box := range boxes {
		if box.Width <= 0 {
			continue
		}
		value := padClip(values[box.Name], box.Width)
		part := "[" + value + "]"
		next := part
		if current != "" {
			next = current + separator + part
		}
		if visibleLen(next) > width && current != "" {
			lines = append(lines, current)
			current = part
			continue
		}
		current = next
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func (m searchTUIModel) renderLogBody() string {
	height := m.bodyHeight()
	if height <= 0 {
		return ""
	}
	widths := m.columnWidths()
	lines := []string{m.fitLogLine(m.logHeaderLine(widths))}
	content := m.logBodyLines(widths)
	start := m.visibleStartIndex()
	for idx := start; idx < len(content) && len(lines) < height; idx++ {
		lines = append(lines, content[idx])
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m searchTUIModel) logBodyLines(widths []int) []string {
	messages := m.visibleMessages()
	lines := []string{}
	for idx, msg := range messages {
		line := m.logRowLine(idx, msg, widths)
		line = m.fitLogLine(line)
		if idx == m.selected && !m.detailPopupOpen {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, line)
		key := m.messageKey(idx, msg)
		if m.expanded[key] {
			for _, expanded := range m.expandedLines(msg) {
				lines = append(lines, m.fitLogLine(m.expandedLine(expanded, widths)))
			}
		}
	}
	return lines
}

func (m searchTUIModel) selectedLineIndex() int {
	if m.bodyMode == tuiModeGroups && m.view == tuiViewLogs {
		return m.selected
	}
	messages := m.visibleMessages()
	if len(messages) == 0 || m.selected <= 0 {
		return 0
	}
	line := 0
	for idx := 0; idx < len(messages) && idx < m.selected; idx++ {
		line++
		key := m.messageKey(idx, messages[idx])
		if m.expanded[key] {
			line += len(m.expandedLines(messages[idx]))
		}
	}
	return line
}

func (m searchTUIModel) selectedBlockEndIndex() int {
	line := m.selectedLineIndex()
	messages := m.visibleMessages()
	if m.selected < 0 || m.selected >= len(messages) {
		return line
	}
	key := m.messageKey(m.selected, messages[m.selected])
	if m.expanded[key] {
		line += len(m.expandedLines(messages[m.selected]))
	}
	return line
}

func (m searchTUIModel) visibleStartIndex() int {
	count := m.visibleContentLineCount()
	if count == 0 {
		return 0
	}
	visibleRows := m.visibleLogBodyRows()
	start := m.listScroll
	maxStart := max(0, count-visibleRows)
	if start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start
}

func (m searchTUIModel) visibleLogBodyRows() int {
	return max(1, m.bodyHeight()-1)
}

func (m searchTUIModel) visibleContentLineCount() int {
	if m.bodyMode == tuiModeGroups && m.view == tuiViewLogs {
		return len(m.visibleGroups())
	}
	return len(m.logBodyLines(m.columnWidths()))
}

func (m searchTUIModel) logHeaderLine(widths []int) string {
	cells := []string{padClip("#", m.tuiCfg.RowNumberWidth)}
	for idx, column := range m.columns {
		width := 8
		if idx < len(widths) {
			width = widths[idx]
		}
		label := displayColumnName(column)
		if m.columnResizeMode && idx == m.resizeColumn {
			label = "*" + label
		}
		cells = append(cells, padClip(label, width))
	}
	return strings.Join(cells, " ")
}

func (m searchTUIModel) logRowLine(idx int, msg *client.Message, widths []int) string {
	cells := []string{padClip(m.rowNumber(idx), m.tuiCfg.RowNumberWidth)}
	for colIdx, column := range m.columns {
		width := 8
		if colIdx < len(widths) {
			width = widths[colIdx]
		}
		value := singleLine(m.fieldValue(msg, column))
		if column == "timestamp" || column == "source" {
			cells = append(cells, leftClip(value, width))
		} else {
			cells = append(cells, padClip(value, width))
		}
	}
	return strings.Join(cells, " ")
}

func (m searchTUIModel) expandedLine(value string, widths []int) string {
	total := m.tuiCfg.RowNumberWidth
	for _, width := range widths {
		total += width + 1
	}
	return padClip("", m.tuiCfg.RowNumberWidth) + " " + padClip(value, max(1, total-m.tuiCfg.RowNumberWidth-1))
}

func (m searchTUIModel) fitLogLine(line string) string {
	return padClip(line, m.viewportContentWidth())
}

func (m searchTUIModel) rowNumber(idx int) string {
	if m.tuiCfg.RowNumberMode == tuiRowNumberPage {
		return strconv.Itoa(idx + 1)
	}
	return strconv.Itoa(m.qreq.Offset + idx + 1)
}

func (m searchTUIModel) expandedLines(msg *client.Message) []string {
	lines := m.prettyMessageLines(msg, m.tuiCfg.DetailFields, m.tuiCfg.DetailShowEmptyFields)
	out := []string{}
	for _, line := range lines {
		if m.tuiCfg.MessageWrap {
			out = append(out, wrapLine(line, max(20, m.viewportContentWidth()-m.tuiCfg.RowNumberWidth-2))...)
		} else {
			out = append(out, line)
		}
		if m.tuiCfg.ExpandedMaxLines > 0 && len(out) >= m.tuiCfg.ExpandedMaxLines {
			return out[:m.tuiCfg.ExpandedMaxLines]
		}
	}
	return out
}

func (m searchTUIModel) renderBodyWithDetailPopup(body string) string {
	bodyLines := strings.Split(body, "\n")
	popup := m.detailPopupBoxLines()
	if len(popup) == 0 {
		return body
	}
	top := 0
	if m.tuiCfg.DetailPopupPosition == "bottom" {
		top = max(0, len(bodyLines)-len(popup))
	} else if m.tuiCfg.DetailPopupPosition == "center" {
		top = max(0, (len(bodyLines)-len(popup))/2)
	}
	left := max(0, (m.viewportContentWidth()-visibleLen(popup[0]))/2)
	if m.tuiCfg.DetailPopupPosition == "right" {
		left = max(0, m.viewportContentWidth()-visibleLen(popup[0]))
	}
	for idx, line := range popup {
		target := top + idx
		if target >= len(bodyLines) {
			break
		}
		bodyLines[target] = overlayLine(bodyLines[target], line, left, m.viewportContentWidth())
	}
	return strings.Join(bodyLines, "\n")
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
	cols := make([]table.Column, 0, len(m.columns)+1)
	cols = append(cols, table.Column{Title: "#", Width: m.tuiCfg.RowNumberWidth})
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
	for idx, msg := range messages {
		row := make(table.Row, 0, len(m.columns)+1)
		row = append(row, m.rowNumber(idx))
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
	barWidth := max(5, m.viewportContentWidth()-33)
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
	widths := make([]int, len(m.columns))
	remaining := m.viewportContentWidth() - m.tuiCfg.RowNumberWidth - 1 - max(0, len(m.columns)-1)
	flexCount := 0
	for idx, column := range m.columns {
		if idx < len(m.colWidths) && m.colWidths[idx] > 0 {
			widths[idx] = max(m.tuiCfg.MinColumnWidth, m.colWidths[idx])
			remaining -= widths[idx]
			continue
		}
		switch column {
		case "timestamp":
			widths[idx] = 24
			remaining -= widths[idx]
		case "level":
			widths[idx] = 8
			remaining -= widths[idx]
		case "application":
			widths[idx] = 8
			remaining -= widths[idx]
		case "source":
			widths[idx] = 18
			remaining -= widths[idx]
		default:
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

func (m searchTUIModel) detailPopupBoxLines() []string {
	content := m.detailPopupLines()
	height := m.detailPopupHeight()
	if len(content) > height {
		end := min(len(content), m.detailPopupScroll+height)
		content = content[m.detailPopupScroll:end]
	}
	popupWidth := int(float64(m.viewportContentWidth()) * m.tuiCfg.DetailPopupWidthRatio)
	popupWidth = max(20, min(m.viewportContentWidth(), popupWidth))
	innerWidth := max(1, popupWidth-2)
	border := lipgloss.ThickBorder()
	lines := []string{border.TopLeft + padClip(" detail ", innerWidth, border.Top) + border.TopRight}
	for _, line := range content {
		lines = append(lines, border.Left+padClip(line, innerWidth)+border.Right)
	}
	for len(lines) < height+1 {
		lines = append(lines, border.Left+padClip("", innerWidth)+border.Right)
	}
	lines = append(lines, border.BottomLeft+padClip(" Esc close ", innerWidth, border.Bottom)+border.BottomRight)
	return lines
}

func (m searchTUIModel) detailPopupLines() []string {
	return m.prettyMessageLines(m.selectedMessage(), m.tuiCfg.DetailFields, m.tuiCfg.DetailShowEmptyFields)
}

func (m searchTUIModel) prettyMessageLines(msg *client.Message, preferred []string, showEmpty bool) []string {
	if msg == nil {
		return []string{"no selected log"}
	}
	lines := []string{}
	seen := map[string]struct{}{}
	for _, field := range preferred {
		value := m.fieldValue(msg, field)
		if value == "" && !showEmpty {
			continue
		}
		lines = append(lines, splitKeyValueLines(field, value)...)
		seen[field] = struct{}{}
	}
	keys := make([]string, 0, len(msg.Message))
	for key := range msg.Message {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := stringify(msg.Message[key])
		if value == "" && !showEmpty {
			continue
		}
		lines = append(lines, splitKeyValueLines(key, value)...)
	}
	if len(lines) == 0 {
		return []string{"no fields"}
	}
	return lines
}

func splitKeyValueLines(key string, value string) []string {
	parts := strings.Split(value, "\n")
	if len(parts) == 0 {
		return []string{key + ":"}
	}
	lines := []string{fmt.Sprintf("%s: %s", key, parts[0])}
	for _, part := range parts[1:] {
		lines = append(lines, "  "+part)
	}
	return lines
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
	source := m.unfilteredMessages()
	if m.localFilter == "" {
		return source
	}
	if m.localFilterField != "" {
		needle := m.localFilterValue
		if !m.tuiCfg.FilterCaseSensitive {
			needle = strings.ToLower(needle)
		}
		out := make([]*client.Message, 0, len(source))
		for _, msg := range source {
			haystack := fieldValue(m.decoderCfg, msg, m.localFilterField)
			if m.localFilterField == "message" {
				haystack = messagePattern(haystack)
			}
			if !m.tuiCfg.FilterCaseSensitive {
				haystack = strings.ToLower(haystack)
			}
			if strings.Contains(haystack, needle) {
				out = append(out, msg)
			}
		}
		return out
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

func (m searchTUIModel) unfilteredMessages() []*client.Message {
	if m.localFilter != "" {
		return m.cachedMessages()
	}
	return m.messages
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

func calculateFieldCandidates(messages []*client.Message, columns []string, decoderCfg *client.DecoderConfig, tuiCfg SearchTUIConfig) []tuiFieldCandidate {
	fields := append([]string{}, tuiCfg.FilterCandidateFields...)
	for _, column := range columns {
		if !hasString(fields, column) && column != "timestamp" {
			fields = append(fields, column)
		}
	}
	type candidateKey struct {
		field string
		value string
	}
	counts := map[candidateKey]int{}
	for _, msg := range messages {
		for _, field := range fields {
			value := fieldValue(decoderCfg, msg, field)
			if field == "message" {
				value = messagePattern(value)
			}
			if value != "" && value != "<nil>" {
				counts[candidateKey{field: field, value: value}]++
			}
		}
	}
	keys := make([]candidateKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			if keys[i].field == keys[j].field {
				return keys[i].value < keys[j].value
			}
			return keys[i].field < keys[j].field
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	candidates := make([]tuiFieldCandidate, 0, len(keys))
	fieldCounts := map[string]int{}
	for _, key := range keys {
		limit := tuiCfg.FilterCandidateLimit
		if key.field == "message" {
			limit = tuiCfg.MessagePatternCandidateLimit
		}
		if fieldCounts[key.field] >= limit {
			continue
		}
		fieldCounts[key.field]++
		filter := key.field + "=" + key.value
		candidates = append(candidates, tuiFieldCandidate{
			Label:  fmt.Sprintf("%s(%d)", filter, counts[key]),
			Filter: filter,
			Field:  key.field,
			Value:  key.value,
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
	height := m.visibleLogBodyRows()
	selectedLine := m.selectedLineIndex()
	if selectedLine < m.listScroll {
		m.listScroll = selectedLine
	}
	if selectedLine >= m.listScroll+height {
		step := max(1, m.tuiCfg.PageScrollStep)
		m.listScroll += step
		for selectedLine >= m.listScroll+height {
			m.listScroll += step
		}
		if selectedLine < m.listScroll {
			m.listScroll = selectedLine
		}
	}
	if m.listScroll < 0 {
		m.listScroll = 0
	}
	maxStart := max(0, m.visibleContentLineCount()-height)
	if m.listScroll > maxStart {
		m.listScroll = maxStart
	}
}

func (m *searchTUIModel) ensureSelectedBlockVisible() {
	height := m.visibleLogBodyRows()
	selectedLine := m.selectedLineIndex()
	blockEnd := m.selectedBlockEndIndex()
	if selectedLine < m.listScroll {
		m.listScroll = selectedLine
	}
	if blockEnd >= m.listScroll+height {
		m.listScroll = min(selectedLine, blockEnd-height+1)
	}
	if m.listScroll < 0 {
		m.listScroll = 0
	}
	maxStart := max(0, m.visibleContentLineCount()-height)
	if m.listScroll > maxStart {
		m.listScroll = maxStart
	}
}

func (m searchTUIModel) nextPage() (tea.Model, tea.Cmd) {
	if m.total > 0 && uint64(m.qreq.Offset)+m.msgCnt >= m.total {
		return m, nil
	}
	if !m.tuiCfg.PersistExpandedRows {
		m.expanded = map[string]bool{}
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
	if !m.tuiCfg.PersistExpandedRows {
		m.expanded = map[string]bool{}
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
	m.detailPopupOpen = false
	m.detailPopupScroll = 0
	m.expanded = map[string]bool{}
	m.total = 0
	m.msgCnt = 0
	m.err = nil
}

func (m *searchTUIModel) storeCachedPage(offset int, messages []*client.Message) {
	m.cachedPages[offset] = messages
	m.touchCachedPage(offset)
	for len(m.cacheOrder) > m.tuiCfg.MaxCachedPages {
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
	used := m.frameHeight(m.tuiCfg.HeaderVisible, m.headerContentHeight(m.view))
	used += m.frameHeight(m.tuiCfg.FooterVisible, m.footerContentHeight())
	return max(1, m.height-used)
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

func (m *searchTUIModel) toggleExpandedSelected() {
	msg := m.selectedMessage()
	if msg == nil {
		return
	}
	key := m.messageKey(m.selected, msg)
	if m.expanded == nil {
		m.expanded = map[string]bool{}
	}
	if m.expanded[key] {
		delete(m.expanded, key)
		m.ensureSelectedVisible()
		return
	}
	m.expanded[key] = true
	m.ensureSelectedBlockVisible()
}

func (m searchTUIModel) messageKey(index int, msg *client.Message) string {
	if msg != nil {
		for _, field := range []string{"gl2_message_id", "_id"} {
			if value := stringify(msg.Message[field]); value != "" {
				return field + ":" + value
			}
		}
	}
	return fmt.Sprintf("offset:%d:index:%d", m.qreq.Offset, index)
}

func (m *searchTUIModel) scrollDetailPopup(delta int) {
	m.detailPopupScroll += delta
	maxScroll := max(0, len(m.detailPopupLines())-m.detailPopupHeight())
	if m.detailPopupScroll < 0 {
		m.detailPopupScroll = 0
	}
	if m.detailPopupScroll > maxScroll {
		m.detailPopupScroll = maxScroll
	}
}

func (m searchTUIModel) detailPopupHeight() int {
	return min(m.tuiCfg.DetailPopupMaxLines, max(1, m.bodyHeight()-2))
}

func (m *searchTUIModel) resizeSelectedColumn(delta int) {
	if m.resizeColumn < 0 || m.resizeColumn >= len(m.columns) {
		return
	}
	widths := m.columnWidths()
	if len(m.colWidths) < len(m.columns) {
		next := make([]int, len(m.columns))
		copy(next, m.colWidths)
		m.colWidths = next
	}
	m.colWidths[m.resizeColumn] = max(m.tuiCfg.MinColumnWidth, widths[m.resizeColumn]+delta)
}

func (m searchTUIModel) currentTUIColumns() []SearchTUIColumn {
	widths := m.columnWidths()
	columns := make([]SearchTUIColumn, 0, len(m.columns))
	for idx, field := range m.columns {
		width := 0
		if idx < len(widths) {
			width = widths[idx]
		}
		columns = append(columns, SearchTUIColumn{Field: field, Width: width})
	}
	return columns
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

func (m searchTUIModel) localFilterCandidateLabels() []string {
	if m.prompt.stage == tuiLocalFilterValueStage {
		values := m.localFilterValueCandidates(m.prompt.field)
		limit := min(5, len(values))
		labels := make([]string, 0, limit)
		for idx := 0; idx < limit; idx++ {
			label := values[idx].Label
			if idx == m.prompt.selected {
				label = "▶" + label
			}
			labels = append(labels, label)
		}
		return labels
	}
	fields := m.localFilterFields()
	limit := min(5, len(fields))
	labels := make([]string, 0, limit)
	for idx := 0; idx < limit; idx++ {
		label := fields[idx]
		if idx == m.prompt.selected {
			label = "▶" + label
		}
		labels = append(labels, label)
	}
	return labels
}

func (m searchTUIModel) localFilterFields() []string {
	seen := map[string]struct{}{}
	fields := []string{}
	for _, candidate := range m.fieldCandidatesData {
		if candidate.Field == "" {
			continue
		}
		if _, ok := seen[candidate.Field]; ok {
			continue
		}
		seen[candidate.Field] = struct{}{}
		fields = append(fields, candidate.Field)
	}
	return fields
}

func (m searchTUIModel) localFilterValueCandidates(field string) []tuiFieldCandidate {
	values := []tuiFieldCandidate{}
	for _, candidate := range m.fieldCandidatesData {
		if candidate.Field == field {
			values = append(values, candidate)
		}
	}
	return values
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

func displayColumnName(column string) string {
	if column == "application" {
		return "app"
	}
	return column
}

func padClip(value string, width int, fill ...string) string {
	if width <= 0 {
		return ""
	}
	filler := " "
	if len(fill) > 0 && fill[0] != "" {
		filler = fill[0]
	}
	runes := []rune(value)
	if len(runes) > width {
		return string(runes[:width])
	}
	if len(runes) == width {
		return value
	}
	return value + strings.Repeat(filler, width-len(runes))
}

func leftClip(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > width {
		return string(runes[len(runes)-width:])
	}
	return padClip(value, width)
}

func visibleLen(value string) int {
	return len([]rune(value))
}

func wrapLine(value string, width int) []string {
	if width <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	if len(runes) <= width {
		return []string{value}
	}
	lines := []string{}
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		lines = append(lines, string(runes))
	}
	return lines
}

func overlayLine(base string, overlay string, left int, width int) string {
	if width <= 0 {
		return overlay
	}
	baseRunes := []rune(padClip(base, width))
	overlayRunes := []rune(overlay)
	if left < 0 {
		left = 0
	}
	for idx, r := range overlayRunes {
		pos := left + idx
		if pos >= len(baseRunes) {
			break
		}
		baseRunes[pos] = r
	}
	return string(baseRunes)
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
