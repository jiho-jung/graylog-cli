package graylog

import (
	"fmt"

	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/jeehoon/graylog-cli/pkg/util"
)

const (
	fmtTitle = "= %-8s:"
)

func out(title string, format string, a ...interface{}) {
	b := []interface{}{title}
	b = append(b, a...)

	fmt.Printf(fmtTitle+format, b...)
}

func PrintSummary(res *client.Result) {
	out("State", " %v\n", res.State)
	if len(res.Errors) != 0 {
		out("Errors", " %v\n", res.Errors)
	}
	out("Query", " %v\n", res.Query.Query.QueryString)
	fmt.Println()
}

func PrintMessage(qreq *QueryRequest, res *client.Result, decoderCfg *client.DecoderConfig) (uint64, uint64) {
	msgId := qreq.MessageId
	searchRes, has := res.SearchTypes[msgId]
	if !has {
		return 0, 0
	}

	if decoderCfg == nil {
		decoderCfg = DefDecoderConfig
	}

	decoder := client.NewDecoder(decoderCfg)
	useColor := util.UseColor()
	msgCnt := uint64(len(searchRes.Messages))
	total := searchRes.TotalResults
	page := qreq.Offset/qreq.PageLimit + 1
	pastCnt := uint64(qreq.Offset) + msgCnt

	for idx := len(searchRes.Messages) - 1; idx >= 0; idx-- {
		msg := searchRes.Messages[idx]
		fmt.Println(client.Render(decoder, useColor, msg.Message))
	}

	fmt.Printf("========== Messages ==========\n")
	out("Range", " %v ~ %v\n", searchRes.EffectiveTimerange.From, searchRes.EffectiveTimerange.To)
	out("Messages", " %v/%v\n", pastCnt, searchRes.TotalResults)
	out("Page", " %d(%d)/%d\n", page, qreq.PageLimit, searchRes.TotalResults/uint64(qreq.PageLimit))

	return msgCnt, total
}

func PrintTop(res *client.Result, msgId string, topFldName string, tick string) {
	searchRes, has := res.SearchTypes[msgId]
	if !has {
		return
	}

	labels := []string{}
	data := []float64{}

	for _, row := range searchRes.Rows {
		if len(row.Key) == 0 {
			continue
		}

		key := row.Key[0]
		value := row.Values[0].Value
		labels = append(labels, key)
		data = append(data, value)
	}

	util.Chart(labels, data, tick)
	fmt.Printf("========== Top Values of [%v] field ==========\n", topFldName)
	out("Range", " %v ~ %v\n", searchRes.EffectiveTimerange.From, searchRes.EffectiveTimerange.To)
	out("Total", " %v\n", searchRes.Total)
}

func PrintHistogram(res *client.Result, msgId string, tick string) {
	searchRes, has := res.SearchTypes[msgId]
	if !has {
		return
	}

	labels := []string{}
	data := []float64{}

	for _, row := range searchRes.Rows {
		if len(row.Key) == 0 {
			continue
		}

		key := row.Key[0]
		value := row.Values[0].Value
		labels = append(labels, key)
		data = append(data, value)
	}

	util.Chart(labels, data, tick)
	fmt.Printf("========== Histogram ==========\n")
	out("Range", " %v ~ %v\n", searchRes.EffectiveTimerange.From, searchRes.EffectiveTimerange.To)
	out("Total", " %v\n", searchRes.Total)
}
