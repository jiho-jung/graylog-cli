package graylog

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/jeehoon/graylog-cli/pkg/util"
)

var (
	DefSearchRange1H = "1h"
	DefPageLimit     = 100
	DefSortDesc      = "timestamp:DESC"

	DefDecoderConfig = &client.DecoderConfig{
		HostnameKeys: []string{
			"hostname",
			"source",
		},
		TimestampKeys: []string{
			"timestamp",
		},
		LevelKeys: []string{
			"level",
		},
		TextKeys: []string{
			"message",
		},
		SkipFieldKeys: []string{
			"@timestamp",
			"@version",
			"_id",
			"caller",
			"file",
			"function",
			"gl2_accounted_message_size",
			"gl2_message_id",
			"gl2_processing_duration_ms",
			"gl2_processing_timestamp",
			"gl2_receive_timestamp",
			"gl2_remote_ip",
			"gl2_remote_port",
			"gl2_source_input",
			"gl2_source_node",
			"hostname",
			"input",
			"level",
			"line",
			"message",
			"source",
			"streams",
			"timestamp",
		},
		FieldKeys: []string{},
	}
)

const (
	TimeTypeRelative = "relative"
	TimeTypeAbsolute = "absolute"
)

// RelativeRange: 1m, 2h, 3days, 4weeks
// "timerange": {type: "absolute", from: "2024-10-25T00:12:59.168Z", to: "2024-10-25T00:13:22.510Z"}
type TimeRange struct {
	TimeType      string `json:"time_type"`       // "relative" or "absolute"
	RelativeRange string `json:"range,omitempty"` // relative datetime
	AbsoluteStart string `json:"from,omitempty"`  // absolute start date
	AbsoluteEnd   string `json:"to,omitempty"`    // absolute end date
}

const (
	QueryTypeMessage   = "message"
	QueryTypeFieldTop  = "fieldtop"
	QueryTypeHistogram = "histogram"
)

type QueryRequest struct {
	QueryId         string
	MessageId       string
	QueryType       string // Message/FieldTop/Histogram
	TopFieldName    string
	SearchTimeRange TimeRange
	Offset          int
	PageLimit       int
	Sort            string
	Verbose         bool
	UserQuery       string
}

type QueryResult struct {
	client.Result

	QueryId   string
	RequestId string
	MessageId string

	EffectiveTimerange *client.Timerange `json:"effective_timerange"`
	TotalResults       uint64            `json:"total_results"`

	LogMessages []string
}

func NewQueryRequest(cfg *client.Config, queryId string, userQuery string) *QueryRequest {
	qreq := &QueryRequest{
		UserQuery:       "*",
		SearchTimeRange: TimeRange{TimeType: TimeTypeRelative, RelativeRange: DefSearchRange1H},
		PageLimit:       DefPageLimit,
		Sort:            DefSortDesc,
		Verbose:         false,
	}

	if queryId != "" {
		qreq.QueryId = queryId
	} else {
		qreq.QueryId = uuid.New().String()
	}

	qreq.MessageId = uuid.New().String()
	if len(userQuery) != 0 {
		qreq.UserQuery = userQuery
	}

	return qreq
}

func Search(cfg *client.Config, qreq *QueryRequest) (*client.Result, error) {
	var e error

	graylog := client.NewClient(cfg)

	requestId, _ := util.RandomHex(12)
	req := client.NewSearchRequest(requestId)
	query := client.NewSearchQuery(qreq.QueryId)
	query.SetQuery(qreq.UserQuery)

	dur := DefSearchRange1H
	if qreq.SearchTimeRange.TimeType == TimeTypeAbsolute &&
		qreq.SearchTimeRange.AbsoluteStart != "" &&
		qreq.SearchTimeRange.AbsoluteEnd != "" {
		query.SetTimerangeAbsolute(qreq.SearchTimeRange.AbsoluteStart, qreq.SearchTimeRange.AbsoluteEnd)
	} else {
		if qreq.SearchTimeRange.TimeType == TimeTypeRelative && qreq.SearchTimeRange.RelativeRange != "" {
			dur = qreq.SearchTimeRange.RelativeRange
		}

		//duration1, err1 := timeutil.ParseDuration(dur)
		duration, err := time.ParseDuration(dur)
		if err != nil {
			e = fmt.Errorf("ERROR: %w", err)
			return nil, e
		}
		query.SetTimerangeRelative(int(duration / time.Second))
	}

	switch qreq.QueryType {
	case QueryTypeHistogram:
		query.AppendSearchHistogram(qreq.MessageId)
	case QueryTypeFieldTop:
		query.AppendSearchTop(qreq.MessageId, qreq.TopFieldName, qreq.PageLimit)
	default:
		query.AppendSearchMessage(qreq.MessageId, qreq.PageLimit, qreq.Offset, qreq.Sort)
	}

	req.AddQuery(query)

	// Search
	if err := graylog.Post("/api/views/search", req, nil); err != nil {
		e = fmt.Errorf("ERROR: %w", err)
		return nil, e
	}

	// Execute
	var resp *client.SearchResponse

	if err := graylog.Post("/api/views/search/"+requestId+"/execute", nil, &resp); err != nil {
		e = fmt.Errorf("ERROR: %w", err)
		return nil, e
	}

	// Search Status
	for !resp.Execution.Done {
		path := fmt.Sprintf("/api/views/searchjobs/%v/%v/status", resp.ExecutingNode, resp.Id)
		if err := graylog.Get(path, nil, &resp); err != nil {
			e = fmt.Errorf("ERROR: %w", err)
			return nil, e
		}
		time.Sleep(500 * time.Millisecond)
	}

	result, has := resp.Results[qreq.QueryId]
	if !has {
		e = fmt.Errorf("ERROR: not found query result of %v", qreq.QueryId)
		return nil, e
	}

	return result, nil
}
