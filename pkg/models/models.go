package models

import (
	"errors"
	"math"
	"strings"
	"time"
)

const (
	Timeframe1m = "1m"

	maxSymbolLength = 32
)

var (
	ErrSymbolRequired    = errors.New("symbol is required")
	ErrSymbolTooLong     = errors.New("symbol exceeds max length")
	ErrPriceInvalid      = errors.New("price must be a finite number greater than zero")
	ErrVolumeInvalid     = errors.New("volume must be greater than or equal to zero")
	ErrTimestampRequired = errors.New("timestamp is required")
)

type Tick struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Volume    int64     `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

type Candle struct {
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

func NormalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
func (t Tick) Validate() error {
	symbol := NormalizeSymbol(t.Symbol)
	switch {
	case symbol == "":
		return ErrSymbolRequired
	case len(symbol) > maxSymbolLength:
		return ErrSymbolTooLong
	case t.Price <= 0 || math.IsNaN(t.Price) || math.IsInf(t.Price, 0):
		return ErrPriceInvalid
	case t.Volume < 0:
		return ErrVolumeInvalid
	case t.Timestamp.IsZero():
		return ErrTimestampRequired
	default:
		return nil
	}
}
