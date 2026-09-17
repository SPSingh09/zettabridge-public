package livebrokers

import (
	"encoding/csv"
	"io"
	"strings"
)

func findSecurityInInstrumentCSV(r io.Reader, symbol string) (securityID, tradingSymbol string, ok bool) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return "", "", false
	}

	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return "", "", false
	}

	idCol := csvColumnIndex(header, "SEM_SMST_SECURITY_ID", "SECURITY_ID")
	symCol := csvColumnIndex(header, "SM_SYMBOL_NAME", "SYMBOL_NAME", "SEM_TRADING_SYMBOL")
	displayCol := csvColumnIndex(header, "SEM_CUSTOM_SYMBOL", "DISPLAY_NAME")

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) == 0 {
			continue
		}

		rowSymbol := csvField(row, symCol)
		display := csvField(row, displayCol)
		id := csvField(row, idCol)
		if id == "" {
			continue
		}

		if symbolMatches(symbol, rowSymbol, display) {
			ts := rowSymbol
			if ts == "" {
				ts = display
			}
			return id, ts, true
		}
	}
	return "", "", false
}

func csvColumnIndex(header []string, names ...string) int {
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[strings.ToUpper(strings.TrimSpace(n))] = struct{}{}
	}
	for i, h := range header {
		if _, ok := want[strings.ToUpper(strings.TrimSpace(h))]; ok {
			return i
		}
	}
	return -1
}

func csvField(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func symbolMatches(want, rowSymbol, display string) bool {
	want = strings.ToUpper(strings.TrimSpace(want))
	for _, candidate := range []string{rowSymbol, display} {
		c := strings.ToUpper(strings.TrimSpace(candidate))
		if c == "" {
			continue
		}
		if c == want {
			return true
		}
		if strings.HasPrefix(c, want+"-") {
			return true
		}
	}
	return false
}
