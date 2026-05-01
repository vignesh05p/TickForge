package models

import "time"

// Tick is a market-like tick at the domain boundary (serialization details TBD).
type Tick struct {
	Symbol    string
	Price     float64
	Volume    int64
	Timestamp time.Time
}

// Candle is a completed OHLCV bucket for a symbol and timeframe.
type Candle struct {
	Symbol    string
	Timeframe string
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int64
	StartTime time.Time
	EndTime   time.Time
}
