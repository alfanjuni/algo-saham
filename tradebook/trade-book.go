package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
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

type NetValuePoint struct {
	Time  string      `json:"time"`
	Value *ValueField `json:"value"`
}

type APIResponse struct {
	Message string `json:"message"`
	Data    struct {
		Prices    []PricePoint    `json:"prices"`
		NetValues []NetValuePoint `json:"net_values"`
	} `json:"data"`
}

type MarketMoverResponse struct {
	Message string `json:"message"`
	Data    struct {
		MoverList []struct {
			StockDetail struct {
				Code   string `json:"code"`
				Price  int    `json:"price"`
				Change struct {
					Value      int     `json:"value"`
					Percentage float64 `json:"percentage"`
				} `json:"change"`
			} `json:"stock_detail"`
		} `json:"mover_list"`
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

func fetchPrices(symbol, date, token string) ([]Candle, []NetValue, error) {
	url := fmt.Sprintf(
		"https://exodus.stockbit.com/order-trade/trade-book/chart?symbol=%s&time_interval=1m&date=%s",
		symbol, date,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse JSON: %w\nBody: %s", err, string(body[:min(200, len(body))]))
	}

	return parsePrices(apiResp.Data.Prices), parseNetValues(apiResp.Data.NetValues), nil
}

func fetchMarketMoverSymbols(token string) ([]string, error) {
	url := "https://exodus.stockbit.com/order-trade/market-mover?mover_type=MOVER_TYPE_TOP_VALUE&filter_stocks=FILTER_STOCKS_TYPE_MAIN_BOARD&filter_stocks=FILTER_STOCKS_TYPE_DEVELOPMENT_BOARD&filter_stocks=FILTER_STOCKS_TYPE_ACCELERATION_BOARD&filter_stocks=FILTER_STOCKS_TYPE_NEW_ECONOMY_BOARD"

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

	var marketMoverResp MarketMoverResponse
	if err := json.Unmarshal(body, &marketMoverResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w\nBody: %s", err, string(body[:min(200, len(body))]))
	}

	symbols := make([]string, 0, len(marketMoverResp.Data.MoverList))
	for _, mover := range marketMoverResp.Data.MoverList {
		if mover.StockDetail.Code != "" {
			symbols = append(symbols, mover.StockDetail.Code)
		}
	}

	return symbols, nil
}

// ─── Load from local JSON file (for testing) ────────────────────────────────

func loadFromFile(path string) ([]Candle, []NetValue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, nil, err
	}

	return parsePrices(apiResp.Data.Prices), parseNetValues(apiResp.Data.NetValues), nil
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

// ─── Net Value Parsing and Z-score Detection ─────────────────────────────────

type NetValue struct {
	Value     float64
	Time      string
	Formatted string
}

func parseNetValues(points []NetValuePoint) []NetValue {
	var netValues []NetValue
	for _, p := range points {
		if p.Value == nil {
			continue
		}
		value, err := strconv.ParseFloat(p.Value.Raw, 64)
		if err != nil {
			continue
		}
		netValues = append(netValues, NetValue{
			Value:     value,
			Time:      p.Time,
			Formatted: p.Value.Formatted,
		})
	}
	return netValues
}

func calculateZScore(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate standard deviation
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	stdDev := 0.0
	if len(values) > 1 {
		variance = variance / float64(len(values)-1)
		stdDev = 0.0
		if variance > 0 {
			stdDev = math.Sqrt(variance)
		}
	}

	// Calculate Z-scores
	zScores := make([]float64, len(values))
	for i, v := range values {
		if stdDev > 0 {
			zScores[i] = (v - mean) / stdDev
		} else {
			zScores[i] = 0
		}
	}

	return zScores
}

type BigSignal struct {
	FormattedValue string
	Time           string
	IsBuy          bool
	RawValue       float64
}

type ExhaustionSignal struct {
	Type              string
	CurrentPrice      float64
	LastRelevantPrice float64
	CurrentValue      string
	LastValue         string
	CurrentTime       string
	LastSignalTime    string
}

func detectBigSignals(netValues []NetValue, threshold float64) []BigSignal {
	if len(netValues) < 2 {
		return nil
	}

	// Extract raw values
	values := make([]float64, len(netValues))
	for i, nv := range netValues {
		values[i] = nv.Value
	}

	// Calculate Z-scores
	zScores := calculateZScore(values)

	// Detect big signals
	var signals []BigSignal
	for i, nv := range netValues {
		if zScores[i] >= threshold {
			signals = append(signals, BigSignal{
				FormattedValue: nv.Formatted,
				Time:           nv.Time,
				IsBuy:          true,
				RawValue:       nv.Value,
			})
		} else if zScores[i] <= -threshold {
			signals = append(signals, BigSignal{
				FormattedValue: nv.Formatted,
				Time:           nv.Time,
				IsBuy:          false,
				RawValue:       nv.Value,
			})
		}
	}

	return signals
}

func printBigSignals(signals []BigSignal) {
	if len(signals) == 0 {
		fmt.Println("=== No Big Signals Detected ===")
		fmt.Println()
		return
	}

	var bigBuys []BigSignal
	var bigSells []BigSignal

	for _, s := range signals {
		if s.IsBuy {
			bigBuys = append(bigBuys, s)
		} else {
			bigSells = append(bigSells, s)
		}
	}

	if len(bigSells) > 0 {
		fmt.Println("Big Sell:")
		for _, s := range bigSells {
			fmt.Printf("{\"%s\", \"%s\"}\n", s.FormattedValue, s.Time)
		}
		fmt.Println()
	}

	if len(bigBuys) > 0 {
		fmt.Println("Big Buy:")
		for _, s := range bigBuys {
			fmt.Printf("{\"%s\", \"%s\"}\n", s.FormattedValue, s.Time)
		}
		fmt.Println()
	}
}

func findNetValueAtOrBeforeTime(time string, bigSignals []BigSignal) *BigSignal {
	var lastSignal *BigSignal
	for _, signal := range bigSignals {
		if signal.Time <= time {
			lastSignal = &signal
		}
	}
	return lastSignal
}

func findNearestBigSellBeforeTime(time string, bigSignals []BigSignal) *BigSignal {
	var lastSell *BigSignal
	for _, signal := range bigSignals {
		if !signal.IsBuy && signal.Time <= time {
			lastSell = &signal
		}
	}
	return lastSell
}

func findNearestBigBuyBeforeTime(time string, bigSignals []BigSignal) *BigSignal {
	var lastBuy *BigSignal
	for _, signal := range bigSignals {
		if signal.IsBuy && signal.Time <= time {
			lastBuy = &signal
		}
	}
	return lastBuy
}

func getNetValueAtOrNearTime(targetTime string, netValues []NetValue) *NetValue {
	for _, nv := range netValues {
		if nv.Time == targetTime {
			return &nv
		}
	}
	// Find nearest before target time
	var nearestBefore *NetValue
	for _, nv := range netValues {
		if nv.Time < targetTime {
			if nearestBefore == nil || nv.Time > nearestBefore.Time {
				nearestBefore = &nv
			}
		}
	}
	return nearestBefore
}

func detectExhaustion(swings []SwingPoint, bigSignals []BigSignal, netValues []NetValue) []ExhaustionSignal {
	var exhaustionSignals []ExhaustionSignal

	if len(swings) < 2 || len(bigSignals) == 0 {
		return exhaustionSignals
	}

	var lastSwingHigh *SwingPoint
	var lastSwingLow *SwingPoint

	var maxBigSell *BigSignal // most negative (biggest sell)
	var maxBigBuy *BigSignal  // most positive (biggest buy)

	var lastUsedBigSell *BigSignal
	var lastUsedBigBuy *BigSignal

	for idx, swing := range swings {
		currentBigSell := findNearestBigSellAtOrBeforeTime(swing.Time, bigSignals)
		currentBigBuy := findNearestBigBuyAtOrBeforeTime(swing.Time, bigSignals)

		// Update maxBigSell/maxBigBuy with any new bigger signals up to swing time
		for _, signal := range bigSignals {
			if signal.Time > swing.Time {
				continue
			}
			if !signal.IsBuy {
				if maxBigSell == nil || signal.RawValue < maxBigSell.RawValue {
					maxBigSell = &signal
				}
			} else {
				if maxBigBuy == nil || signal.RawValue > maxBigBuy.RawValue {
					maxBigBuy = &signal
				}
			}
		}

		if idx == 0 {
			if swing.Type == SwingHigh {
				lastSwingHigh = &swing
			} else {
				lastSwingLow = &swing
			}
			lastUsedBigSell = maxBigSell
			lastUsedBigBuy = maxBigBuy
			continue
		}

		// Now update last used for this iteration with current max values
		var currentCheckBigSell *BigSignal = lastUsedBigSell
		var currentCheckBigBuy *BigSignal = lastUsedBigBuy
		if maxBigSell != nil {
			currentCheckBigSell = maxBigSell
		}
		if maxBigBuy != nil {
			currentCheckBigBuy = maxBigBuy
		}

		if swing.Type == SwingLow {
			if lastSwingLow != nil {
				if swing.Price < lastSwingLow.Price {
					if currentBigSell != nil && currentCheckBigSell != nil {
						if currentBigSell.RawValue > currentCheckBigSell.RawValue {
							exhaustionSignals = append(exhaustionSignals, ExhaustionSignal{
								Type:              "Seller Exhaustion",
								CurrentPrice:      swing.Price,
								LastRelevantPrice: lastSwingLow.Price,
								CurrentValue:      currentBigSell.FormattedValue,
								LastValue:         currentCheckBigSell.FormattedValue,
								CurrentTime:       swing.Time,
								LastSignalTime:    currentCheckBigSell.Time,
							})
						}
					}
				}
			}
			lastSwingLow = &swing
		} else if swing.Type == SwingHigh {
			if lastSwingHigh != nil {
				if swing.Price > lastSwingHigh.Price {
					if currentBigBuy != nil && currentCheckBigBuy != nil {
						if currentBigBuy.RawValue < currentCheckBigBuy.RawValue {
							exhaustionSignals = append(exhaustionSignals, ExhaustionSignal{
								Type:              "Buyer Exhaustion",
								CurrentPrice:      swing.Price,
								LastRelevantPrice: lastSwingHigh.Price,
								CurrentValue:      currentBigBuy.FormattedValue,
								LastValue:         currentCheckBigBuy.FormattedValue,
								CurrentTime:       swing.Time,
								LastSignalTime:    currentCheckBigBuy.Time,
							})
						}
					}
				}
			}
			lastSwingHigh = &swing
		}

		// Now update last used for next iteration with max values
		if maxBigSell != nil {
			lastUsedBigSell = maxBigSell
		}
		if maxBigBuy != nil {
			lastUsedBigBuy = maxBigBuy
		}
	}

	return exhaustionSignals
}

func findNearestBigSellAtOrBeforeTime(targetTime string, bigSignals []BigSignal) *BigSignal {
	var lastSell *BigSignal
	for _, signal := range bigSignals {
		if !signal.IsBuy && signal.Time <= targetTime {
			if lastSell == nil || signal.Time > lastSell.Time {
				lastSell = &signal
			}
		}
	}
	return lastSell
}

func findNearestBigBuyAtOrBeforeTime(targetTime string, bigSignals []BigSignal) *BigSignal {
	var lastBuy *BigSignal
	for _, signal := range bigSignals {
		if signal.IsBuy && signal.Time <= targetTime {
			if lastBuy == nil || signal.Time > lastBuy.Time {
				lastBuy = &signal
			}
		}
	}
	return lastBuy
}

func findNearestBigSignalForType(targetTime string, swingType SwingType, bigSignals []BigSignal) *BigSignal {
	if swingType == SwingLow {
		return findNearestBigSellBeforeOrAtTime(targetTime, bigSignals)
	}
	return findNearestBigBuyBeforeOrAtTime(targetTime, bigSignals)
}

func findNearestBigSellBeforeOrAtTime(targetTime string, bigSignals []BigSignal) *BigSignal {
	var lastSell *BigSignal
	for _, signal := range bigSignals {
		if !signal.IsBuy && signal.Time <= targetTime {
			if lastSell == nil || signal.Time > lastSell.Time {
				lastSell = &signal
			}
		}
	}
	return lastSell
}

func findNearestBigBuyBeforeOrAtTime(targetTime string, bigSignals []BigSignal) *BigSignal {
	var lastBuy *BigSignal
	for _, signal := range bigSignals {
		if signal.IsBuy && signal.Time <= targetTime {
			if lastBuy == nil || signal.Time > lastBuy.Time {
				lastBuy = &signal
			}
		}
	}
	return lastBuy
}

func printExhaustionSignals(exhaustions []ExhaustionSignal, symbol, date string) {
	if len(exhaustions) == 0 {
		fmt.Println("=== No Exhaustion Signals Detected ===")
		fmt.Println()
		return
	}

	fmt.Println("=== Exhaustion Signals ===")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Date: %s\n", date)
	fmt.Println()
	for _, e := range exhaustions {
		fmt.Printf("%s:\n", e.Type)
		fmt.Printf("  current_price: %.0f\n", e.CurrentPrice)
		fmt.Printf("  last_price: %.0f\n", e.LastRelevantPrice)
		fmt.Printf("  current_value: \"%s\"\n", e.CurrentValue)
		fmt.Printf("  last_value: \"%s\"\n", e.LastValue)
		fmt.Printf("  current_time: \"%s\"\n", e.CurrentTime)
		fmt.Printf("  last_signal_time: \"%s\"\n", e.LastSignalTime)
		fmt.Println()
	}

	// Send to Discord webhook
	sendToDiscord(exhaustions, symbol, date)
}

func sendToDiscord(exhaustions []ExhaustionSignal, symbol, date string) {
	type DiscordEmbedField struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Inline bool   `json:"inline"`
	}
	type DiscordEmbed struct {
		Title  string              `json:"title"`
		Color  int                 `json:"color"` // Green for buy, Red for sell
		Fields []DiscordEmbedField `json:"fields"`
	}
	type DiscordWebhook struct {
		Embeds []DiscordEmbed `json:"embeds"`
	}

	webhookURL := "https://discord.com/api/webhooks/1512179043481423993/cJYeoeE-X_1-4tg4yCJCzpstkurpPe5B2MdCUuZO8BiexFhL9KpWJ6x8pDF7MHLRfjY-"

	embeds := make([]DiscordEmbed, 0, len(exhaustions))
	for _, e := range exhaustions {
		color := 0xFF0000 // Red for sell (Seller Exhaustion)
		if e.Type == "Buyer Exhaustion" {
			color = 0x00FF00 // Green for buy (Buyer Exhaustion)
		}
		embeds = append(embeds, DiscordEmbed{
			Title: fmt.Sprintf("%s - %s", symbol, e.Type),
			Color: color,
			Fields: []DiscordEmbedField{
				{Name: "Date", Value: date, Inline: true},
				{Name: "Current Price", Value: fmt.Sprintf("%.0f", e.CurrentPrice), Inline: true},
				{Name: "Last Price", Value: fmt.Sprintf("%.0f", e.LastRelevantPrice), Inline: true},
				{Name: "Current Value", Value: e.CurrentValue, Inline: true},
				{Name: "Last Value", Value: e.LastValue, Inline: true},
				{Name: "Current Time", Value: fmt.Sprintf(`"%s"`, e.CurrentTime), Inline: true},
				{Name: "Last Signal Time", Value: fmt.Sprintf(`"%s"`, e.LastSignalTime), Inline: true},
			},
		})
	}

	webhookData := DiscordWebhook{
		Embeds: embeds,
	}

	jsonData, err := json.Marshal(webhookData)
	if err != nil {
		fmt.Printf("Error marshaling Discord webhook data: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating Discord webhook request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending Discord webhook request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Successfully sent Exhaustion Signals to Discord!")
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
		date              = "2026-06-10"
		token             = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImExNWQ5OGE2LTdkYzgtNDM3NS05NDk0LTEyOWJlM2RlODVkNCIsInR5cCI6IkpXVCJ9.eyJkYXRhIjp7InVzZSI6IkVrYXl1c25pdGEiLCJlbWEiOiJla2F5dXNuaXRhLm5zMTJAZ21haWwuY29tIiwiZnVsIjoiRWtheXVzbml0YSIsInNlcyI6Imt1OEJCTk0xaUV0RGRuWXUiLCJkdmMiOiI5ZGM1NzI4MGQ4MGIzMGFmNTgxMmJlNjBiOWJlZjdjOSIsInVpZCI6MzU1NDkxOCwiY291IjoiSUQifSwiZXhwIjoxNzgxMTQ1NjcxLCJpYXQiOjE3ODEwNTkyNzEsImlzcyI6IlNUT0NLQklUIiwianRpIjoiMjdjZmFjNDItNWVhMS00M2EwLWI4NzEtYTQ4MDlhNGM4Nzc4IiwibmJmIjoxNzgxMDU5MjcxLCJ2ZXIiOiJ2MSJ9.Mw_HXz8JAG6DwDmFzEp4hv6bU5DbrlseA9ZWYEv2gAFnVO5eCc2yuoROdFshC1D7hYx_nrE5TKmHqo-C_NS-DaSisfE4O8DJFWLycfHCGNvveKa31_3eK2y22EAPny_jBb7_Eci8lX0TyKMrJNnH0ZbXE0XS-OkLY2na_fPtWVRmP2R9hZd17xnxdh52F1wzMBjOWhlfFDqPKoJsQRYWic7IkigHloBRoM2-va5k8yw5BMOjKx3DWM572Oq4C_Yk60vCg6aXyD2LpkMOg9MKP_4yo-K69-TQ3RcESikwMUFWqLmHChNrkvZsM2RmFUOfAxu0B4op4UYL-rNI0QThow"
		swingPointsLength = 5   // candles on each side to confirm swing high/low (higher = fewer, more significant swings)
		zScoreThreshold   = 2.0 // Z-score threshold for Big Buy/Sell detection
		localFile         = ""  // set to "" to fetch from API
	)

	// Fetch list of symbols from market-mover endpoint
	symbols, err := fetchMarketMoverSymbols(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch market mover symbols: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Fetched %d symbols from market mover\n", len(symbols))
	fmt.Println()

	// Loop through each symbol
	for _, symbol := range symbols {
		fmt.Printf("Processing %s...\n", symbol)
		fmt.Println(strings.Repeat("-", 50))

		var candles []Candle
		var netValues []NetValue
		var errFetch error

		if localFile != "" {
			fmt.Printf("Loading from local file: %s\n", localFile)
			candles, netValues, errFetch = loadFromFile(localFile)
		} else {
			candles, netValues, errFetch = fetchPrices(symbol, date, token)
		}

		if errFetch != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", symbol, errFetch)
			fmt.Println()
			continue
		}

		if len(candles) == 0 {
			fmt.Printf("No price data found for %s, skipping...\n", symbol)
			fmt.Println()
			continue
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

		// Detect and print Big Buy/Sell signals from net_values
		fmt.Println()
		bigSignals := detectBigSignals(netValues, zScoreThreshold)
		printBigSignals(bigSignals)

		// Detect and print Exhaustion signals
		exhaustions := detectExhaustion(swings, bigSignals, netValues)
		printExhaustionSignals(exhaustions, symbol, date)

		fmt.Println()
		fmt.Println()
	}
}
