package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jeehoon/graylog-cli/pkg/graylog"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/jeehoon/graylog-cli/pkg/timeutil"
	"github.com/spf13/cobra"
)

var (
	Tick              = "■"
	Output            = client.OutputCompact
	Follow            = false
	Refresh           = "5s"
	TUI               = false
	SearchApplication = ""
	SearchPart        = ""
)

// searchCmd represents the search command
var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search Graylog logs",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		searchMessages(cmd, args)
	},
}

var searchHistogramCmd = &cobra.Command{
	Use:   "histogram [query]",
	Short: "Show a histogram for a query",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		searchAggregate(cmd, args, graylog.QueryTypeHistogram, "")
	},
}

var searchTopCmd = &cobra.Command{
	Use:   "top <field> [query]",
	Short: "Show top values for a field",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		searchAggregate(cmd, args[1:], graylog.QueryTypeFieldTop, args[0])
	},
}

func searchMessages(cmd *cobra.Command, args []string) {
	qreq, clientCfg, err := newSearchRequest(args)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	qreq.QueryType = graylog.QueryTypeMessage

	if TUI {
		if err := runSearchTUI(clientCfg, qreq, DecoderConfig); err != nil {
			log.Printf("failed to run TUI: err=%v", err)
		}
		return
	}

	if Follow {
		followSearch(clientCfg, qreq)
		return
	}

	runSearchLoop(clientCfg, qreq)
}

func searchAggregate(cmd *cobra.Command, args []string, queryType string, topField string) {
	qreq, clientCfg, err := newSearchRequest(args)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	qreq.QueryType = queryType
	qreq.TopFieldName = topField

	res, err := graylog.Search(clientCfg, qreq)
	if err != nil {
		log.Printf("failed to search Graylog: err=%v", err)
		return
	}

	if queryType == graylog.QueryTypeHistogram {
		graylog.PrintHistogram(res, qreq.MessageId, Tick)
	} else {
		graylog.PrintTop(res, qreq.MessageId, qreq.TopFieldName, Tick)
	}
	graylog.PrintSummary(res)
}

func newSearchRequest(args []string) (*graylog.QueryRequest, *client.Config, error) {
	cfg := getGraylogConfig()

	var ep, username, password string
	if ServerEndpoint != "" {
		ep = ServerEndpoint
	} else if cfg != nil {
		ep = cfg.Url
	} else {
		ep = "https://127.0.0.1"
	}

	if Username != "" {
		username = Username
	} else if cfg != nil {
		username = cfg.UserToken
	}

	if Password != "" {
		password = Password
	} else {
		password = "token"
	}

	clientCfg := &client.Config{
		Verbose:  Verbose,
		Endpoint: ep,
		Username: username,
		Password: password,
	}

	query := "*"
	if len(args) != 0 {
		query = strings.Join(args, " ")
	}
	query = buildSearchQuery(query, SearchApplication, SearchPart)

	if err := validateOutput(Output); err != nil {
		return nil, nil, err
	}
	if Limit <= 0 {
		return nil, nil, fmt.Errorf("invalid --limit %d: expected a positive integer", Limit)
	}

	qreq := graylog.NewQueryRequest(clientCfg, "", query)

	qreq.PageLimit = Limit
	qreq.Offset = Offset
	qreq.Sort = Sort
	qreq.Fields = DecoderConfig.FieldKeys
	qreq.Output = Output
	qreq.Follow = Follow
	qreq.Refresh = Refresh
	qreq.TUI = TUI

	if SearchFrom != "" && SearchTo != "" {
		qreq.SearchTimeRange.TimeType = graylog.TimeTypeAbsolute
		qreq.SearchTimeRange.AbsoluteStart = SearchFrom
		qreq.SearchTimeRange.AbsoluteEnd = SearchTo
	} else {
		qreq.SearchTimeRange.TimeType = graylog.TimeTypeRelative
		qreq.SearchTimeRange.RelativeRange = SearchRange
	}

	if _, err := timeutil.ParseDuration(qreq.SearchTimeRange.RelativeRange); qreq.SearchTimeRange.TimeType == graylog.TimeTypeRelative && err != nil {
		return nil, nil, fmt.Errorf("invalid --since value %q: %w", qreq.SearchTimeRange.RelativeRange, err)
	}
	if _, err := time.ParseDuration(Refresh); err != nil {
		return nil, nil, fmt.Errorf("invalid --refresh value %q: %w", Refresh, err)
	}

	return qreq, clientCfg, nil
}

func validateOutput(output string) error {
	switch output {
	case client.OutputCompact, client.OutputPretty, client.OutputJSON, client.OutputNDJSON:
		return nil
	default:
		return fmt.Errorf("invalid --output %q: expected compact, pretty, json, or ndjson", output)
	}
}

func buildSearchQuery(query string, application string, part string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		query = "*"
	}

	filters := []string{}
	if application != "" {
		filters = append(filters, "application:"+quoteQueryValue(application))
	}
	if part != "" {
		filters = append(filters, "part:"+quoteQueryValue(part))
	}
	if len(filters) == 0 {
		return query
	}

	filterQuery := strings.Join(filters, " AND ")
	if query == "*" {
		return filterQuery
	}
	return fmt.Sprintf("(%s) AND %s", query, filterQuery)
}

func quoteQueryValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\n\r\"") {
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		return `"` + value + `"`
	}
	return value
}

func runSearchLoop(clientCfg *client.Config, qreq *graylog.QueryRequest) {
	var msgCnt, total uint64

	for {
		if qreq.Offset < 0 {
			qreq.Offset = 0
		}

		msgCnt = 0
		total = 0

		res, err := graylog.Search(clientCfg, qreq)
		if err != nil {
			log.Printf("failed to search Graylog: err=%v", err)
			return
		}

		msgCnt, total = graylog.PrintMessage(qreq, res, DecoderConfig)
		if qreq.Output != client.OutputJSON && qreq.Output != client.OutputNDJSON {
			graylog.PrintSummary(res)
		}

		if !Pagination {
			break
		} else if uint64(qreq.Offset)+msgCnt >= total {
			// end of result
			break
		} else if !navigatePage(qreq, total, msgCnt) {
			break
		}
	}
}

func followSearch(clientCfg *client.Config, qreq *graylog.QueryRequest) {
	interval, err := time.ParseDuration(qreq.Refresh)
	if err != nil {
		log.Printf("invalid refresh duration: err=%v", err)
		return
	}

	for {
		qreq.Offset = 0
		res, err := graylog.Search(clientCfg, qreq)
		if err != nil {
			log.Printf("failed to search Graylog: err=%v", err)
			return
		}

		graylog.PrintMessage(qreq, res, DecoderConfig)
		if qreq.Output != client.OutputJSON && qreq.Output != client.OutputNDJSON {
			graylog.PrintSummary(res)
		}
		time.Sleep(interval)
	}
}

func navigatePage(qreq *graylog.QueryRequest, total uint64, msgCnt uint64) bool {
	// navigate the pages
START:
	key, err := waitUserKeyInput()
	if err != nil {
		log.Printf("failed to wait key: err=%v", err)
		return false
	}

	if key == "n" {
		qreq.Offset += Limit
		if uint64(qreq.Offset) >= total {
			return false
		} else {
			return true
		}
	} else if key == "b" {
		if qreq.Offset < 1 {
			fmt.Printf("The first page\n")
			goto START
		} else {
			qreq.Offset -= int(msgCnt)
			return true
		}
	} else if b, _ := strconv.Atoi(key); b > 0 {
		qreq.Offset = (b - 1) * qreq.PageLimit
		return true
	} else if key == "r" {
		return true
	}

	return false
}

func waitUserKeyInput() (string, error) {
	var text string

	fmt.Print("Input key [n next, b back, r refresh, q quit, page num]: ")
	scanner := bufio.NewScanner(os.Stdin)

	if scanner.Scan() {
		text = scanner.Text()
	}

	return text, nil
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.AddCommand(searchHistogramCmd)
	searchCmd.AddCommand(searchTopCmd)

	searchCmd.Flags().SortFlags = false

	searchCmd.PersistentFlags().IntVar(&Offset, "offset", Offset, "result offset")
	searchCmd.PersistentFlags().IntVar(&Limit, "limit", Limit, "maximum results per request")
	searchCmd.PersistentFlags().StringVar(&Sort, "sort", Sort, "sort as field:ASC or field:DESC")
	searchCmd.PersistentFlags().BoolVarP(&Pagination, "page", "p", Pagination, "enable page navigation")
	searchCmd.PersistentFlags().StringVar(&Output, "output", Output, "output format: compact, pretty, json, ndjson")
	searchCmd.PersistentFlags().BoolVar(&Follow, "follow", Follow, "refresh search results until interrupted")
	searchCmd.PersistentFlags().StringVar(&Refresh, "refresh", Refresh, "refresh interval for --follow and --tui")
	searchCmd.PersistentFlags().BoolVar(&TUI, "tui", TUI, "browse results in a terminal UI")
	searchCmd.PersistentFlags().StringVarP(&SearchApplication, "application", "a", SearchApplication, "add application:<value> to the query")
	searchCmd.PersistentFlags().StringVar(&SearchPart, "part", SearchPart, "add part:<value> to the query")

	// Search
	searchCmd.PersistentFlags().StringSliceVar(&DecoderConfig.HostnameKeys, "hostname", DecoderConfig.HostnameKeys, "fields to use as hostname/source")
	searchCmd.PersistentFlags().StringSliceVar(&DecoderConfig.TimestampKeys, "timestamp", DecoderConfig.TimestampKeys, "fields to use as timestamp")
	searchCmd.PersistentFlags().StringSliceVar(&DecoderConfig.LevelKeys, "level", DecoderConfig.LevelKeys, "fields to use as level")
	searchCmd.PersistentFlags().StringSliceVar(&DecoderConfig.TextKeys, "text", DecoderConfig.TextKeys, "fields to use as message text")
	searchCmd.PersistentFlags().StringSliceVarP(&DecoderConfig.FieldKeys, "fields", "F", DecoderConfig.FieldKeys, "fields to display")
	searchCmd.PersistentFlags().StringSliceVar(&DecoderConfig.SkipFieldKeys, "skip-fields", DecoderConfig.SkipFieldKeys, "fields to hide when --fields is not set")

	// Histogram
	searchCmd.PersistentFlags().StringVar(&Tick, "tick", Tick, "chart tick character")
}
