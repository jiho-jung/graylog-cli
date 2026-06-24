package render

import (
	"fmt"
	"strings"

	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/jeehoon/graylog-cli/pkg/timeutil"
)

const (
	Reset = "\033[0m"

	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"

	LightRed     = "\033[91m"
	LightGreen   = "\033[92m"
	LightYellow  = "\033[93m"
	LightBlue    = "\033[94m"
	LightMagenta = "\033[95m"
	LightCyan    = "\033[96m"

	White    = "\033[97m"
	DarkGray = "\033[90m"
)

func Render(dec *client.Decoder, useColor bool, msg *client.Message) string {
	fieldsMap := msg.Message
	keys, values := dec.Fields(fieldsMap)
	var fields []string
	for idx, key := range keys {
		value := values[idx]

		if useColor {
			key = Cyan + key + Reset
			value = Blue + value + Reset
		}

		fields = append(fields, fmt.Sprintf("%v:%v", key, value))
	}

	hostname := dec.Hostname(fieldsMap)

	if useColor {
		hostname = LightMagenta + hostname + Reset
	}

	lv := dec.Level(fieldsMap)
	level := lv.String()

	if useColor {
		switch lv {
		case client.LevelEmergency, client.LevelAlert, client.LevelCritical, client.LevelError:
			level = Red + level + Reset
		case client.LevelWarning:
			level = Yellow + level + Reset
		case client.LevelNotice, client.LevelInformational:
			level = White + level + Reset
		case client.LevelDebug:
			level = DarkGray + level + Reset
		}
	}

	timestamp := timeutil.Format(dec.Timestamp(fieldsMap))

	text := dec.Text(fieldsMap)

	output := fmt.Sprintln(hostname, timestamp, level, text, strings.Join(fields, " "))

	return strings.TrimSpace(output)
}
