package models

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestTickValidate(t *testing.T) {
	validTick := Tick{
		Symbol:    "INFY",
		Price:     1485.50,
		Volume:    100,
		Timestamp: time.Date(2026, 5, 1, 10, 30, 2, 0, time.UTC),
	}

	tests := []struct {
		name string
		tick Tick
		err  error
	}{
		{name: "valid", tick: validTick},
		{name: "blank symbol", tick: Tick{Price: 1, Timestamp: validTick.Timestamp}, err: ErrSymbolRequired},
		{name: "zero price", tick: Tick{Symbol: "INFY", Timestamp: validTick.Timestamp}, err: ErrPriceInvalid},
		{name: "nan price", tick: Tick{Symbol: "INFY", Price: math.NaN(), Timestamp: validTick.Timestamp}, err: ErrPriceInvalid},
		{name: "negative volume", tick: Tick{Symbol: "INFY", Price: 1, Volume: -1, Timestamp: validTick.Timestamp}, err: ErrVolumeInvalid},
		{name: "missing timestamp", tick: Tick{Symbol: "INFY", Price: 1}, err: ErrTimestampRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tick.Validate()
			if !errors.Is(err, tt.err) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestNormalizeSymbol(t *testing.T) {
	got := NormalizeSymbol(" infy ")
	if got != "INFY" {
		t.Fatalf("NormalizeSymbol() = %q, want %q", got, "INFY")
	}
}
