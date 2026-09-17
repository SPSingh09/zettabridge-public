package store

// TimeWindow is a local-time interval within a day (HH:MM, 24h).
type TimeWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// TradingSchedule maps weekday keys (mon..sun) to allowed windows. nil = 24/7.
type TradingSchedule map[string][]TimeWindow
