package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/vigneshprabhu/tickforge/pkg/models"
)

func main() {
	symbol := flag.String("symbol", "INFY", "symbol to generate")
	count := flag.Int("count", 5, "number of ticks to generate")
	startPrice := flag.Float64("price", 100, "starting price")
	flag.Parse()

	if *count <= 0 {
		log.Fatal("count must be positive")
	}
	if *startPrice <= 0 {
		log.Fatal("price must be positive")
	}

	encoder := json.NewEncoder(os.Stdout)
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < *count; i++ {
		tick := models.Tick{
			Symbol:    models.NormalizeSymbol(*symbol),
			Price:     *startPrice + float64(i)*0.05,
			Volume:    int64(100 + i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
		if err := encoder.Encode(tick); err != nil {
			log.Fatalf("encode tick: %v", err)
		}
	}
}
