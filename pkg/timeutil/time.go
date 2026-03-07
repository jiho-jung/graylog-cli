package timeutil

import (
	"regexp"
	"strings"
	"time"
)

func Format(ts time.Time) string {
	s := ts.Round(time.Millisecond).Format(time.RFC3339Nano)
	dot := strings.Index(s, ".")
	if dot == -1 {
		s = strings.Replace(s, "Z", ".000Z", 1)
		dot = strings.Index(s, ".")
	}
	zone := strings.Index(s, "Z")

	switch zone - dot {
	case 3:
		s = strings.Replace(s, "Z", "0Z", 1)
	case 2:
		s = strings.Replace(s, "Z", "00Z", 1)
	}

	return s
}

func Parse(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// ParseDuration parses a duration string.
// examples: "10d", "-1.5w" or "3Y4M5d".
// Add time units are "d"="D", "w"="W", "M", "y"="Y".
func ParseDuration(s string) (time.Duration, error) {
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}

	re := regexp.MustCompile(`(\d*\.\d+|\d+)[^\d]*`)
	unitMap := map[string]time.Duration{
		"d": 24,
		"D": 24,
		"w": 7 * 24,
		"W": 7 * 24,
		"M": 30 * 24,
		"y": 365 * 24,
		"Y": 365 * 24,
	}

	strs := re.FindAllString(s, -1)
	var sumDur time.Duration
	for _, str := range strs {
		var _hours time.Duration = 1
		for unit, hours := range unitMap {
			if strings.Contains(str, unit) {
				str = strings.ReplaceAll(str, unit, "h")
				_hours = hours
				break
			}
		}

		dur, err := time.ParseDuration(str)
		if err != nil {
			return 0, err
		}

		sumDur += dur * _hours
	}

	if neg {
		sumDur = -sumDur
	}
	return sumDur, nil
}
