package graylog

import (
	"encoding/json"
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
	lines, msgCnt, total, err := RenderMessageLines(qreq, res, decoderCfg)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return 0, 0
	}

	for _, line := range lines {
		fmt.Println(line)
	}
	if qreq.Output != client.OutputJSON && qreq.Output != client.OutputNDJSON {
		PrintMessageSummary(qreq, res, msgCnt)
	}

	return msgCnt, total
}

func RenderMessageLines(qreq *QueryRequest, res *client.Result, decoderCfg *client.DecoderConfig) ([]string, uint64, uint64, error) {
	msgId := qreq.MessageId
	searchRes, has := res.SearchTypes[msgId]
	if !has {
		return nil, 0, 0, nil
	}

	if decoderCfg == nil {
		decoderCfg = DefDecoderConfig
	}

	decoder := client.NewDecoder(decoderCfg)
	useColor := util.UseColor() && qreq.Output != client.OutputJSON && qreq.Output != client.OutputNDJSON
	msgCnt := uint64(len(searchRes.Messages))
	total := searchRes.TotalResults

	if qreq.Output == client.OutputJSON {
		messages := make([]map[string]any, 0, len(searchRes.Messages))
		for idx := len(searchRes.Messages) - 1; idx >= 0; idx-- {
			messages = append(messages, searchRes.Messages[idx].Message)
		}
		b, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return nil, 0, 0, err
		}
		return []string{string(b)}, msgCnt, total, nil
	}

	lines := []string{}
	for idx := len(searchRes.Messages) - 1; idx >= 0; idx-- {
		msg := searchRes.Messages[idx]
		line, err := client.RenderMessage(decoder, useColor, msg, qreq.Output)
		if err != nil {
			return nil, 0, 0, err
		}
		lines = append(lines, line)
	}

	return lines, msgCnt, total, nil
}

func PrintMessageSummary(qreq *QueryRequest, res *client.Result, msgCnt uint64) {
	msgId := qreq.MessageId
	searchRes, has := res.SearchTypes[msgId]
	if !has {
		return
	}

	page := qreq.Offset/qreq.PageLimit + 1
	pastCnt := uint64(qreq.Offset) + msgCnt

	fmt.Printf("========== Messages ==========\n")
	out("Range", " %v ~ %v\n", searchRes.EffectiveTimerange.From, searchRes.EffectiveTimerange.To)
	out("Messages", " %v/%v\n", pastCnt, searchRes.TotalResults)
	out("Page", " %d(%d)/%d\n", page, qreq.PageLimit, searchRes.TotalResults/uint64(qreq.PageLimit))
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
