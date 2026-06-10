package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

// ─── API Response Structs ────────────────────────────────────────────────────

type ValueField struct {
	Raw       string `json:"raw"`
	Formatted string `json:"formatted"`
}

type PricePoint struct {
	Time  string      `json:"time"`
	Value *ValueField `json:"value"`
}

type APIResponse struct {
	Message string `json:"message"`
	Data    struct {
		Prices []PricePoint `json:"prices"`
	} `json:"data"`
}

// ─── Domain Types ────────────────────────────────────────────────────────────

type Candle struct {
	Price float64
	Time  string
}

type SwingType string

const (
	SwingHigh SwingType = "H"
	SwingLow  SwingType = "L"
)

type SwingPoint struct {
	Type  SwingType
	Index int
	Price float64
	Time  string
}

// ─── Fetch Data ──────────────────────────────────────────────────────────────

func fetchPrices(symbol, date, token string) ([]Candle, error) {
	url := fmt.Sprintf(
		"https://exodus.stockbit.com/order-trade/trade-book/chart?symbol=%s&time_interval=1m&date=%s",
		symbol, date,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("origin", "https://stockbit.com")
	req.Header.Set("referer", "https://stockbit.com/")
	req.Header.Set("x-platform", "web")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w\nBody: %s", err, string(body[:min(200, len(body))]))
	}

	return parsePrices(apiResp.Data.Prices), nil
}

// ─── Load from local JSON file (for testing) ────────────────────────────────

func loadFromFile(path string) ([]Candle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}

	return parsePrices(apiResp.Data.Prices), nil
}

func parsePrices(points []PricePoint) []Candle {
	var candles []Candle
	for _, p := range points {
		if p.Value == nil {
			continue
		}
		price, err := strconv.ParseFloat(p.Value.Raw, 64)
		if err != nil || price == 0 {
			continue
		}
		candles = append(candles, Candle{Price: price, Time: p.Time})
	}
	return candles
}

// ─── Swing High / Low Detection ─────────────────────────────────────────────
//
// A swing high at index i = local peak with `swingPointsLength` candles on each side:
//   price[i] > price[i-1..i-swingPointsLength] AND price[i] > price[i+1..i+swingPointsLength]
//
// A swing low at index i = local trough (same logic, inverted).

func detectSwings(candles []Candle, swingPointsLength int) []SwingPoint {
	n := len(candles)
	var swings []SwingPoint

	for i := swingPointsLength; i < n-swingPointsLength; i++ {
		isHigh := true
		isLow := true

		for j := 1; j <= swingPointsLength; j++ {
			if candles[i].Price <= candles[i-j].Price || candles[i].Price <= candles[i+j].Price {
				isHigh = false
			}
			if candles[i].Price >= candles[i-j].Price || candles[i].Price >= candles[i+j].Price {
				isLow = false
			}
		}

		if isHigh {
			swings = append(swings, SwingPoint{
				Type:  SwingHigh,
				Index: i,
				Price: candles[i].Price,
				Time:  candles[i].Time,
			})
		} else if isLow {
			swings = append(swings, SwingPoint{
				Type:  SwingLow,
				Index: i,
				Price: candles[i].Price,
				Time:  candles[i].Time,
			})
		}
	}

	return swings
}

// ─── Filter and Structure Swings for SMC ───────────────────────────────────
//
// Filters swings to ensure strict alternation between H and L,
// and keeps only the most significant swings.

func filterAndStructureSwings(swings []SwingPoint) []SwingPoint {
	if len(swings) < 2 {
		return swings
	}

	var filtered []SwingPoint
	var lastType SwingType

	for _, s := range swings {
		// If first swing, just add it
		if len(filtered) == 0 {
			filtered = append(filtered, s)
			lastType = s.Type
			continue
		}

		// If same type as last, keep the more significant one
		if s.Type == lastType {
			lastIndex := len(filtered) - 1
			lastSwing := filtered[lastIndex]

			// For highs: keep the higher one, for lows: keep the lower one
			if (s.Type == SwingHigh && s.Price > lastSwing.Price) ||
				(s.Type == SwingLow && s.Price < lastSwing.Price) {
				// Replace the last one with this one
				filtered[lastIndex] = s
			}
		} else {
			// Different type, add it
			filtered = append(filtered, s)
			lastType = s.Type
		}
	}

	return filtered
}

// ─── Market Structure ────────────────────────────────────────────────────────
//
// Dari swing points, tentukan urutan H1, L1, H2, L2, ... secara kronologis.
// Label dihitung per type: H counter dan L counter masing-masing independen.

type StructurePoint struct {
	Label string
	Price float64
	Time  string
}

func buildMarketStructure(swings []SwingPoint) []StructurePoint {
	var structure []StructurePoint
	hCount := 0
	lCount := 0

	for _, s := range swings {
		switch s.Type {
		case SwingHigh:
			hCount++
			structure = append(structure, StructurePoint{
				Label: fmt.Sprintf("H%d", hCount),
				Price: s.Price,
				Time:  s.Time,
			})
		case SwingLow:
			lCount++
			structure = append(structure, StructurePoint{
				Label: fmt.Sprintf("L%d", lCount),
				Price: s.Price,
				Time:  s.Time,
			})
		}
	}

	return structure
}

// ─── Print Output ────────────────────────────────────────────────────────────

func printStructure(structure []StructurePoint) {
	fmt.Println("=== MARKET STRUCTURE (Swing High / Low) ===")
	fmt.Println()
	fmt.Printf("%-6s  %-10s  %s\n", "Label", "Price", "Time")
	fmt.Println("-------------------------------")

	for _, s := range structure {
		fmt.Printf("%-6s  %-10.0f  %s\n", s.Label, s.Price, s.Time)
	}

	fmt.Println()

	// Also print in compact notation as requested
	fmt.Println("=== COMPACT NOTATION ===")
	fmt.Println()
	for _, s := range structure {
		fmt.Printf("%s: {%.0f, \"%s\"}\n", s.Label, s.Price, s.Time)
	}
}

func printSummary(candles []Candle, swings []SwingPoint, structure []StructurePoint) {
	fmt.Printf("\n=== SUMMARY ===\n")
	fmt.Printf("Total candles   : %d\n", len(candles))
	fmt.Printf("Swing points    : %d\n", len(swings))
	fmt.Printf("Structure points: %d\n", len(structure))

	if len(candles) > 0 {
		first := candles[0]
		last := candles[len(candles)-1]
		fmt.Printf("Period          : %s — %s\n", first.Time, last.Time)
		fmt.Printf("Open            : %.0f\n", first.Price)
		fmt.Printf("Close           : %.0f\n", last.Price)
	}

	// Find overall high and low
	overallHigh := 0.0
	overallLow := 1e18
	for _, c := range candles {
		if c.Price > overallHigh {
			overallHigh = c.Price
		}
		if c.Price < overallLow {
			overallLow = c.Price
		}
	}
	fmt.Printf("Day High        : %.0f\n", overallHigh)
	fmt.Printf("Day Low         : %.0f\n", overallLow)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	// Config
	const (
		symbol            = "BBCA"
		date              = "2026-06-10"
		token             = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImExNWQ5OGE2LTdkYzgtNDM3NS05NDk0LTEyOWJlM2RlODVkNCIsInR5cCI6IkpXVCJ9.eyJkYXRhIjp7InVzZSI6IkVrYXl1c25pdGEiLCJlbWEiOiJla2F5dXNuaXRhLm5zMTJAZ21haWwuY29tIiwiZnVsIjoiRWtheXVzbml0YSIsInNlcyI6Imt1OEJCTk0xaUV0RGRuWXUiLCJkdmMiOiI5ZGM1NzI4MGQ4MGIzMGFmNTgxMmJlNjBiOWJlZjdjOSIsInVpZCI6MzU1NDkxOCwiY291IjoiSUQifSwiZXhwIjoxNzgxMTQ1NjcxLCJpYXQiOjE3ODEwNTkyNzEsImlzcyI6IlNUT0NLQklUIiwianRpIjoiMjdjZmFjNDItNWVhMS00M2EwLWI4NzEtYTQ4MDlhNGM4Nzc4IiwibmJmIjoxNzgxMDU5MjcxLCJ2ZXIiOiJ2MSJ9.Mw_HXz8JAG6DwDmFzEp4hv6bU5DbrlseA9ZWYEv2gAFnVO5eCc2yuoROdFshC1D7hYx_nrE5TKmHqo-C_NS-DaSisfE4O8DJFWLycfHCGNvveKa31_3eK2y22EAPny_jBb7_Eci8lX0TyKMrJNnH0ZbXE0XS-OkLY2na_fPtWVRmP2R9hZd17xnxdh52F1wzMBjOWhlfFDqPKoJsQRYWic7IkigHloBRoM2-va5k8yw5BMOjKx3DWM572Oq4C_Yk60vCg6aXyD2LpkMOg9MKP_4yo-K69-TQ3RcESikwMUFWqLmHChNrkvZsM2RmFUOfAxu0B4op4UYL-rNI0QThow"
		swingPointsLength = 5                 // candles on each side to confirm swing high/low (higher = fewer, more significant swings)
		localFile         = "trade-book.json" // set to "" to fetch from API
	)

	fmt.Printf("Market Structure Analyzer — %s (%s)\n\n", symbol, date)

	// Load candles: prefer local file if present, fallback to API
	var candles []Candle
	var err error

	if localFile != "" {
		fmt.Printf("Loading from local file: %s\n", localFile)
		candles, err = loadFromFile(localFile)
	} else {
		fmt.Printf("Fetching from API: %s %s\n", symbol, date)
		candles, err = fetchPrices(symbol, date, token)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if len(candles) == 0 {
		fmt.Println("No price data found.")
		os.Exit(1)
	}

	// Detect initial swings
	rawSwings := detectSwings(candles, swingPointsLength)

	// Filter and structure swings for SMC (strict alternation H → L → H → L)
	swings := filterAndStructureSwings(rawSwings)

	// Build labeled market structure
	structure := buildMarketStructure(swings)

	// Output
	printSummary(candles, swings, structure)
	fmt.Println()
	printStructure(structure)
}
