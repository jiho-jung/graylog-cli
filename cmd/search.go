package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jeehoon/graylog-cli/pkg/graylog"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/spf13/cobra"
)

var (
	Histogram = false
	TermsTop  = ""
	Tick      = "■"
)

// searchCmd represents the search command
var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "searching logs with query (default subcmd)",
	Run:   search,
}

func search(cmd *cobra.Command, args []string) {
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

	/*
		q := "*"
		if len(args) != 0 {
			q = args[0]
		}
	*/

	qreq := graylog.NewQueryRequest(clientCfg, "", Query)

	qreq.PageLimit = Limit
	qreq.Offset = Offset

	if SearchFrom != "" && SearchTo != "" {
		qreq.SearchTimeRange.TimeType = graylog.TimeTypeAbsolute
		qreq.SearchTimeRange.AbsoluteStart = SearchFrom
		qreq.SearchTimeRange.AbsoluteEnd = SearchTo
	} else {
		qreq.SearchTimeRange.TimeType = graylog.TimeTypeRelative
		qreq.SearchTimeRange.RelativeRange = SearchRange
	}

	if Histogram {
		qreq.QueryType = graylog.QueryTypeHistogram
	} else if TermsTop != "" {
		qreq.QueryType = graylog.QueryTypeFieldTop
		qreq.TopFieldName = TermsTop
	} else {
		qreq.QueryType = graylog.QueryTypeMessage
	}

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

		if Histogram {
			graylog.PrintHistogram(res, qreq.MessageId, Tick)
			Pagination = false
		} else if TermsTop != "" {
			qreq.QueryType = graylog.QueryTypeFieldTop
			qreq.TopFieldName = TermsTop
			graylog.PrintTop(res, qreq.MessageId, qreq.TopFieldName, Tick)
			Pagination = false
		} else {
			msgCnt, total = graylog.PrintMessage(qreq, res, DecoderConfig)
		}

		graylog.PrintSummary(res)

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
	}

	return false
}

func waitUserKeyInput() (string, error) {
	var text string

	fmt.Print("Inputkey[n(ext),b(ack),q(uit),(page)num]: ")
	scanner := bufio.NewScanner(os.Stdin)

	if scanner.Scan() {
		text = scanner.Text()
	}

	return text, nil
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().SortFlags = false

	searchCmd.Flags().IntVar(&Offset, "offset", Offset, "")
	searchCmd.Flags().IntVar(&Limit, "limit", Limit, "")
	searchCmd.Flags().StringVar(&Sort, "sort", Sort, "")
	searchCmd.Flags().StringVarP(&Query, "query", "q", Query, "query string")
	searchCmd.Flags().BoolVarP(&Pagination, "page", "p", Pagination, "Pagination")

	// Search
	searchCmd.Flags().StringSliceVar(&DecoderConfig.HostnameKeys, "hostname", DecoderConfig.HostnameKeys, "")
	searchCmd.Flags().StringSliceVar(&DecoderConfig.TimestampKeys, "timestamp", DecoderConfig.TimestampKeys, "")
	searchCmd.Flags().StringSliceVar(&DecoderConfig.LevelKeys, "level", DecoderConfig.LevelKeys, "")
	searchCmd.Flags().StringSliceVar(&DecoderConfig.TextKeys, "text", DecoderConfig.TextKeys, "")
	searchCmd.Flags().StringSliceVarP(&DecoderConfig.FieldKeys, "fields", "F", DecoderConfig.FieldKeys, "")
	searchCmd.Flags().StringSliceVar(&DecoderConfig.SkipFieldKeys, "skip-fields", DecoderConfig.SkipFieldKeys, "")

	// Histogram
	searchCmd.Flags().BoolVarP(&Histogram, "histogram", "H", Histogram, "")
	searchCmd.Flags().StringVarP(&TermsTop, "top", "T", TermsTop, "")
	searchCmd.Flags().StringVar(&Tick, "tick", Tick, "")
}
