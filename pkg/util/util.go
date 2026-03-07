package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func UseColor() bool {
	finfo, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	return finfo.Mode()&os.ModeCharDevice == os.ModeCharDevice
}

func RandomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func Chart(labels []string, data []float64, tick string) {
	length := len(labels)
	if len(labels) > len(data) {
		length = len(data)
	}

	var file = os.Stdout
	var maxLabelLength int
	var maxValue float64

	for i := 0; i < length; i++ {
		label := labels[i]
		value := data[i]
		if maxLabelLength < len(label) {
			maxLabelLength = len(label)
		}

		if maxValue < value {
			maxValue = value
		}
	}

	maxBarLength := float64(50)
	labelFmt := fmt.Sprintf("%%%ds", maxLabelLength)

	for i := 0; i < length; i++ {
		label := labels[i]
		value := data[i]

		barLength := (value / maxValue) * maxBarLength
		bar := strings.Repeat(tick, int(barLength))

		s := fmt.Sprintf(labelFmt+":%s %.3f", label, bar, value)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		fmt.Fprintln(file, s)
	}
}
