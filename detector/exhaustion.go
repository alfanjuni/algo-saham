package detector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

type Candle struct {
	Time      string
	Price     float64
	BuyLot    float64
	SellLot   float64
	BuyValue  float64
	SellValue float64
	BuyFreq   float64
	SellFreq  float64
	NetLot    float64 // BuyLot - SellLot
	NetValue  float64 // BuyValue - SellValue
}

type SignalType string

const (
	SignalBuy              SignalType = "BUY"
	SignalSell             SignalType = "SELL"
	SignalSellerExhaustion SignalType = "SELLER_EXHAUSTION"
	SignalBuyerExhaustion  SignalType = "BUYER_EXHAUSTION"
)

type Signal struct {
	Time     string
	Type     SignalType
	Score    float64
	Strength string
	Triggers []string
}

type SymbolResult struct {
	Symbol  string
	Price   float64
	Signals []Signal
	OldAnlz OrderBookAnalysis // Tambahkan analisa lama
	Err     error
}

func parseRaw(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case int:
		return float64(val)
	default:
		return 0
	}
}

func rollingZScore(values []float64, i int, window int) float64 {
	start := i - window
	if start < 0 {
		start = 0
	}
	subset := values[start:i]
	if len(subset) < 3 {
		return 0
	}

	sum := 0.0
	for _, v := range subset {
		sum += v
	}
	mean := sum / float64(len(subset))

	variance := 0.0
	for _, v := range subset {
		variance += (v - mean) * (v - mean)
	}
	stddev := math.Sqrt(variance / float64(len(subset)))

	if stddev == 0 {
		return 0
	}
	return (values[i] - mean) / stddev
}

func extractField(candles []Candle, f func(Candle) float64) []float64 {
	res := make([]float64, len(candles))
	for i, c := range candles {
		res[i] = f(c)
	}
	return res
}

func DetectSignals(candles []Candle) []Signal {
	if len(candles) < 5 {
		return nil
	}

	prices := extractField(candles, func(c Candle) float64 { return c.Price })
	buyLots := extractField(candles, func(c Candle) float64 { return c.BuyLot })
	sellLots := extractField(candles, func(c Candle) float64 { return c.SellLot })
	buyVals := extractField(candles, func(c Candle) float64 { return c.BuyValue })
	sellVals := extractField(candles, func(c Candle) float64 { return c.SellValue })
	buyFreqs := extractField(candles, func(c Candle) float64 { return c.BuyFreq })
	sellFreqs := extractField(candles, func(c Candle) float64 { return c.SellFreq })
	netLots := extractField(candles, func(c Candle) float64 { return c.NetLot })
	netVals := extractField(candles, func(c Candle) float64 { return c.NetValue })

	avgSells := make([]float64, len(candles))
	avgBuys := make([]float64, len(candles))
	for i := range candles {
		if candles[i].SellLot > 0 {
			avgSells[i] = candles[i].SellValue / candles[i].SellLot
		}
		if candles[i].BuyLot > 0 {
			avgBuys[i] = candles[i].BuyValue / candles[i].BuyLot
		}
	}

	i := len(candles) - 1
	var signals []Signal

	// 1a. detectSellerExhaustion
	se := Signal{Time: candles[i].Time, Type: SignalSellerExhaustion, Triggers: []string{}}
	if prices[i] < prices[i-1] && prices[i-1] < prices[i-2] {
		se.Score += 1.0
		se.Triggers = append(se.Triggers, "lower_low")
	}
	zSell := rollingZScore(sellLots, i, 20)
	if zSell <= -2.0 {
		se.Score += 2.0
		se.Triggers = append(se.Triggers, fmt.Sprintf("sell_vol↓(z=%.1f)", zSell))
	} else if zSell <= -1.5 {
		se.Score += 1.5
		se.Triggers = append(se.Triggers, fmt.Sprintf("sell_vol↓(z=%.1f)", zSell))
	}
	zSellFreq := rollingZScore(sellFreqs, i, 20)
	if zSellFreq <= -2.0 {
		se.Score += 2.0
		se.Triggers = append(se.Triggers, fmt.Sprintf("sell_freq↓(z=%.1f)", zSellFreq))
	} else if zSellFreq <= -1.5 {
		se.Score += 1.5
		se.Triggers = append(se.Triggers, fmt.Sprintf("sell_freq↓(z=%.1f)", zSellFreq))
	}
	zNetLot := rollingZScore(netLots, i, 20)
	if zNetLot >= 1.5 && i >= 2 && netLots[i-1] < 0 && netLots[i-2] < 0 {
		se.Score += 2.0
		se.Triggers = append(se.Triggers, fmt.Sprintf("netLot_flip(z=%.1f)", zNetLot))
	}
	zNetVal := rollingZScore(netVals, i, 20)
	if prices[i] < prices[i-1] && zNetVal >= 1.5 {
		se.Score += 2.0
		se.Triggers = append(se.Triggers, fmt.Sprintf("netval_div(z=%.1f)", zNetVal))
	}
	zAvgSell := rollingZScore(avgSells, i, 20)
	if zAvgSell <= -1.5 {
		se.Score += 1.5
		se.Triggers = append(se.Triggers, fmt.Sprintf("avg_sell↓(z=%.1f)", zAvgSell))
	}
	if se.Score >= 3.0 {
		se.Strength = getStrength(se.Score)
		signals = append(signals, se)
	}

	// 1b. detectBuyerExhaustion
	be := Signal{Time: candles[i].Time, Type: SignalBuyerExhaustion, Triggers: []string{}}
	if prices[i] > prices[i-1] && prices[i-1] > prices[i-2] {
		be.Score += 1.0
		be.Triggers = append(be.Triggers, "higher_high")
	}
	zBuy := rollingZScore(buyLots, i, 20)
	if zBuy <= -2.0 {
		be.Score += 2.0
		be.Triggers = append(be.Triggers, fmt.Sprintf("buy_vol↓(z=%.1f)", zBuy))
	} else if zBuy <= -1.5 {
		be.Score += 1.5
		be.Triggers = append(be.Triggers, fmt.Sprintf("buy_vol↓(z=%.1f)", zBuy))
	}
	zBuyFreq := rollingZScore(buyFreqs, i, 20)
	if zBuyFreq <= -2.0 {
		be.Score += 2.0
		be.Triggers = append(be.Triggers, fmt.Sprintf("buy_freq↓(z=%.1f)", zBuyFreq))
	} else if zBuyFreq <= -1.5 {
		be.Score += 1.5
		be.Triggers = append(be.Triggers, fmt.Sprintf("buy_freq↓(z=%.1f)", zBuyFreq))
	}
	if zNetLot <= -1.5 && i >= 2 && netLots[i-1] > 0 && netLots[i-2] > 0 {
		be.Score += 2.0
		be.Triggers = append(be.Triggers, fmt.Sprintf("netLot_flip_dn(z=%.1f)", zNetLot))
	}
	if prices[i] > prices[i-1] && zNetVal <= -1.5 {
		be.Score += 2.0
		be.Triggers = append(be.Triggers, fmt.Sprintf("netval_div_dn(z=%.1f)", zNetVal))
	}
	zAvgBuy := rollingZScore(avgBuys, i, 20)
	if zAvgBuy <= -1.5 {
		be.Score += 1.5
		be.Triggers = append(be.Triggers, fmt.Sprintf("avg_buy↓(z=%.1f)", zAvgBuy))
	}
	if be.Score >= 3.0 {
		be.Strength = getStrength(be.Score)
		signals = append(signals, be)
	}

	// 1c. detectBuy
	bs := Signal{Time: candles[i].Time, Type: SignalBuy, Triggers: []string{}}
	if zNetLot >= 2.0 {
		bs.Score += 2.0
		bs.Triggers = append(bs.Triggers, fmt.Sprintf("netLot_surge(z=%.1f)", zNetLot))
	} else if zNetLot >= 1.5 {
		bs.Score += 1.5
		bs.Triggers = append(bs.Triggers, fmt.Sprintf("netLot_surge(z=%.1f)", zNetLot))
	}
	zBuyVal := rollingZScore(buyVals, i, 20)
	if zBuyVal >= 2.0 {
		bs.Score += 2.0
		bs.Triggers = append(bs.Triggers, fmt.Sprintf("buy_val_surge(z=%.1f)", zBuyVal))
	} else if zBuyVal >= 1.5 {
		bs.Score += 1.5
		bs.Triggers = append(bs.Triggers, fmt.Sprintf("buy_val_surge(z=%.1f)", zBuyVal))
	}
	if zBuyFreq >= 1.5 {
		bs.Score += 1.5
		bs.Triggers = append(bs.Triggers, fmt.Sprintf("buy_freq_surge(z=%.1f)", zBuyFreq))
	}
	if prices[i] > prices[i-1] && zBuy >= 1.5 {
		bs.Score += 2.0
		bs.Triggers = append(bs.Triggers, fmt.Sprintf("price_vol_up(z=%.1f)", zBuy))
	}
	if bs.Score >= 3.0 {
		bs.Strength = getStrength(bs.Score)
		signals = append(signals, bs)
	}

	// 1d. detectSell
	ss := Signal{Time: candles[i].Time, Type: SignalSell, Triggers: []string{}}
	if zNetLot <= -2.0 {
		ss.Score += 2.0
		ss.Triggers = append(ss.Triggers, fmt.Sprintf("netLot_dump(z=%.1f)", zNetLot))
	} else if zNetLot <= -1.5 {
		ss.Score += 1.5
		ss.Triggers = append(ss.Triggers, fmt.Sprintf("netLot_dump(z=%.1f)", zNetLot))
	}
	zSellVal := rollingZScore(sellVals, i, 20)
	if zSellVal >= 2.0 {
		ss.Score += 2.0
		ss.Triggers = append(ss.Triggers, fmt.Sprintf("sell_val_surge(z=%.1f)", zSellVal))
	} else if zSellVal >= 1.5 {
		ss.Score += 1.5
		ss.Triggers = append(ss.Triggers, fmt.Sprintf("sell_val_surge(z=%.1f)", zSellVal))
	}
	if zSellFreq >= 1.5 {
		ss.Score += 1.5
		ss.Triggers = append(ss.Triggers, fmt.Sprintf("sell_freq_surge(z=%.1f)", zSellFreq))
	}
	if prices[i] < prices[i-1] && zSell >= 1.5 {
		ss.Score += 2.0
		ss.Triggers = append(ss.Triggers, fmt.Sprintf("price_vol_dn(z=%.1f)", zSell))
	}
	if ss.Score >= 3.0 {
		ss.Strength = getStrength(ss.Score)
		signals = append(signals, ss)
	}

	return signals
}

func getStrength(score float64) string {
	if score >= 9.0 {
		return "STRONG"
	} else if score >= 6.0 {
		return "MODERATE"
	} else if score >= 3.0 {
		return "WEAK"
	}
	return ""
}

func ScanSymbols(templateID string, date string, bearerToken string) []SymbolResult {
	orderBookResp, err := FetchOrderBook(templateID, bearerToken)
	if err != nil {
		return []SymbolResult{{Err: err}}
	}

	items := orderBookResp.Data.Item
	results := make([]SymbolResult, len(items))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for idx, item := range items {
		wg.Add(1)
		go func(i int, itm Item) {
			defer wg.Done()

			s := itm.Symbol
			chart, err := FetchTradeBook(s, date, bearerToken)
			res := SymbolResult{
				Symbol:  s,
				Price:   float64(itm.LastPrice), // Gunakan harga dari template sebagai default
				OldAnlz: AnalyzeOrderBook(itm),
			}
			if err != nil {
				res.Err = err
			} else if len(chart.Data.Prices) > 0 {
				var candles []Candle
				for k := range chart.Data.Prices {
					c := Candle{
						Time:      chart.Data.Prices[k].Time,
						Price:     parseRaw(chart.Data.Prices[k].Value.Raw),
						BuyLot:    parseRaw(chart.Data.Buy[k].Lot.Raw),
						SellLot:   parseRaw(chart.Data.Sell[k].Lot.Raw),
						BuyValue:  parseRaw(chart.Data.Buy[k].Value.Raw),
						SellValue: parseRaw(chart.Data.Sell[k].Value.Raw),
						BuyFreq:   parseRaw(chart.Data.Buy[k].Frequency.Raw),
						SellFreq:  parseRaw(chart.Data.Sell[k].Frequency.Raw),
						NetLot:    parseRaw(chart.Data.NetValuesVolume[k].Lot.Raw),
					}
					c.NetValue = c.BuyValue - c.SellValue
					candles = append(candles, c)
				}
				res.Price = candles[len(candles)-1].Price
				res.Signals = DetectSignals(candles)
			}

			mu.Lock()
			results[i] = res
			mu.Unlock()
		}(idx, item)
	}
	wg.Wait()
	return results
}

func PrintTable(results []SymbolResult, scanTime time.Time) {
	wib := time.FixedZone("WIB", 7*3600)
	scanTime = scanTime.In(wib)

	// Clear screen
	fmt.Print("\033[H\033[2J")

	// --- TABLE 1: Z-SCORE HYBRID ---
	w1 := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
	fmt.Println("╔" + strings.Repeat("═", 83) + "╗")
	fmt.Printf("║  ORDERFLOW SCANNER (Z-SCORE)  │  %s  │  %d symbols               ║\n", scanTime.Format("02 Jan 2006 15:04"), len(results))
	fmt.Println("╠" + strings.Repeat("═", 8) + "╦" + strings.Repeat("═", 7) + "╦" + strings.Repeat("═", 8) + "╦" + strings.Repeat("═", 14) + "╦" + strings.Repeat("═", 42) + "╣")
	fmt.Fprintf(w1, "║ TICKER \t PRICE \t  SCORE \t SIGNAL       \t TRIGGERS                                  ║\n")
	fmt.Println("╠" + strings.Repeat("═", 8) + "╬" + strings.Repeat("═", 7) + "╬" + strings.Repeat("═", 8) + "╬" + strings.Repeat("═", 14) + "╬" + strings.Repeat("═", 42) + "╣")

	for _, res := range results {
		priceStr := fmt.Sprintf("%.0f", res.Price)
		scoreStr := "-"
		signalStr := "-"
		triggersStr := "-"
		color := "\033[2m" // Dim

		if res.Err != nil {
			signalStr = "ERROR"
			triggersStr = res.Err.Error()
		} else {
			if len(res.Signals) > 0 {
				sort.Slice(res.Signals, func(i, j int) bool {
					return res.Signals[i].Score > res.Signals[j].Score
				})

				topSignal := res.Signals[0]
				scoreStr = fmt.Sprintf("%.1f", topSignal.Score)

				var sigLabels []string
				for _, s := range res.Signals {
					label := string(s.Type)
					if s.Type == SignalSellerExhaustion {
						label = "SELL.EXH"
					}
					if s.Type == SignalBuyerExhaustion {
						label = "BUY.EXH"
					}
					sigLabels = append(sigLabels, label)
				}

				if res.OldAnlz.Signal != "NEUTRAL" {
					sigLabels = append(sigLabels, "OB:"+res.OldAnlz.Signal)
				}

				icon := "  "
				switch topSignal.Type {
				case SignalBuy:
					icon = "🟢"
					color = "\033[32m"
					if topSignal.Strength == "STRONG" {
						color = "\033[32;1m"
					}
				case SignalSell:
					icon = "🔴"
					color = "\033[31m"
					if topSignal.Strength == "STRONG" {
						color = "\033[31;1m"
					}
				case SignalSellerExhaustion:
					icon = "🔵"
					color = "\033[36m"
				case SignalBuyerExhaustion:
					icon = "🟡"
					color = "\033[35m"
				}

				signalStr = fmt.Sprintf("%s %s", icon, strings.Join(sigLabels, "/"))
				triggersStr = strings.Join(topSignal.Triggers, " ")
			} else {
				// No signals found
				signalStr = "⚪ NEUTRAL"
				if res.OldAnlz.Signal != "NEUTRAL" {
					signalStr = "⚪ NEUTRAL / OB:" + res.OldAnlz.Signal
				}
				triggersStr = "-"
			}
		}

		fmt.Fprintf(w1, "║ %s%s\033[0m \t %s \t   %s \t %s%s\033[0m \t %s \t ║\n",
			color, res.Symbol, priceStr, scoreStr, color, signalStr, triggersStr)
	}
	w1.Flush()
	fmt.Println("╚" + strings.Repeat("═", 8) + "╩" + strings.Repeat("═", 7) + "╩" + strings.Repeat("═", 8) + "╩" + strings.Repeat("═", 14) + "╩" + strings.Repeat("═", 42) + "╝")

	fmt.Println() // Spacer

	// --- TABLE 2: ALGO BID OFFER (OLD FEATURE) ---
	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
	fmt.Println("╔" + strings.Repeat("═", 83) + "╗")
	fmt.Printf("║  ALGO BID OFFER 3 PAPAN TERATAS  │  %s                            ║\n", scanTime.Format("15:04:05"))
	fmt.Println("╠" + strings.Repeat("═", 8) + "╦" + strings.Repeat("═", 7) + "╦" + strings.Repeat("═", 11) + "╦" + strings.Repeat("═", 11) + "╦" + strings.Repeat("═", 8) + "╦" + strings.Repeat("═", 8) + "╦" + strings.Repeat("═", 14) + "╣")
	fmt.Fprintf(w2, "║ TICKER \t PRICE \t BID (LOT)  \t OFF (LOT)  \t VOL R. \t QUE R. \t SIGNAL       ║\n")
	fmt.Println("╠" + strings.Repeat("═", 8) + "╬" + strings.Repeat("═", 7) + "╬" + strings.Repeat("═", 11) + "╬" + strings.Repeat("═", 11) + "╬" + strings.Repeat("═", 8) + "╬" + strings.Repeat("═", 8) + "╬" + strings.Repeat("═", 14) + "╣")

	// Copy for sorting
	oldResults := make([]SymbolResult, len(results))
	copy(oldResults, results)

	sort.Slice(oldResults, func(i, j int) bool {
		if oldResults[i].OldAnlz.Signal != oldResults[j].OldAnlz.Signal {
			return oldResults[i].OldAnlz.Signal == "BUY"
		}
		return oldResults[i].Symbol < oldResults[j].Symbol
	})

	for _, res := range oldResults {
		if res.OldAnlz.Signal == "NEUTRAL" {
			continue
		}

		icon := "🟢 BUY "
		color := "\033[32m"
		if res.OldAnlz.Signal == "SELL" {
			icon = "🔴 SELL"
			color = "\033[31m"
		}

		bidLot := res.OldAnlz.BidVolume / 100
		offLot := res.OldAnlz.OfferVolume / 100

		fmt.Fprintf(w2, "║ %s%s\033[0m \t %d \t %.0f \t %.0f \t %.2f \t %.2f \t %s%s\033[0m ║\n",
			color, res.Symbol, res.OldAnlz.LastPrice, bidLot, offLot, res.OldAnlz.VolumeRatio, res.OldAnlz.QueueRatio, color, icon)
	}
	w2.Flush()
	fmt.Println("╚" + strings.Repeat("═", 8) + "╩" + strings.Repeat("═", 7) + "╩" + strings.Repeat("═", 11) + "╩" + strings.Repeat("═", 11) + "╩" + strings.Repeat("═", 8) + "╩" + strings.Repeat("═", 8) + "╩" + strings.Repeat("═", 14) + "╝")

	fmt.Printf("\nLast update: %s  |  Press Ctrl+C to exit\n", scanTime.Format("15:04:05"))
}

func SendZScoreOrderflowReport(webhookURL string, results []SymbolResult, scanTime time.Time) {
	if webhookURL == "" {
		return
	}

	wib := time.FixedZone("WIB", 7*3600)
	scanTime = scanTime.In(wib)

	var discordMsg strings.Builder
	discordMsg.WriteString(fmt.Sprintf("🚀 **Orderflow Scanner [%s]**\n", scanTime.Format("15:04:05")))

	if len(results) == 0 {
		discordMsg.WriteString("No symbols detected.")
		sendDiscordNotification(webhookURL, discordMsg.String())
		return
	}

	discordMsg.WriteString("```\n")
	discordMsg.WriteString(fmt.Sprintf("%-6s | %5s | %5s | %s\n", "Symbol", "Price", "Score", "Signal"))
	discordMsg.WriteString(strings.Repeat("-", 35) + "\n")

	for _, res := range results {
		// Get highest score among BUY/SELL
		var top Signal
		foundSignal := false
		for _, sig := range res.Signals {
			if sig.Type == SignalBuy || sig.Type == SignalSell {
				if sig.Score > top.Score {
					top = sig
					foundSignal = true
				}
			}
		}

		icon := "⚪"
		signalLabel := "NEUTRAL"

		if foundSignal {
			icon = "🟢"
			if top.Type == SignalSell {
				icon = "🔴"
			}
			signalLabel = string(top.Type)
		}

		// Tambahkan info orderbook jika ada
		if res.OldAnlz.Signal != "NEUTRAL" {
			signalLabel += "/OB:" + res.OldAnlz.Signal
		}

		scoreStr := "-"
		if foundSignal {
			scoreStr = fmt.Sprintf("%.1f", top.Score)
		}

		discordMsg.WriteString(fmt.Sprintf("%-6s | %5.0f | %5s | %s %s\n",
			res.Symbol, res.Price, scoreStr, icon, signalLabel))
	}
	discordMsg.WriteString("```")

	sendDiscordNotification(webhookURL, discordMsg.String())
}

func SendBidOfferAnalysisReport(webhookURL string, results []SymbolResult, scanTime time.Time) {
	if webhookURL == "" {
		return
	}

	wib := time.FixedZone("WIB", 7*3600)
	scanTime = scanTime.In(wib)

	var discordMsg strings.Builder
	discordMsg.WriteString(fmt.Sprintf("🚀 **Algo Bid Offer 3 Papan Teratas [%s]**\n", scanTime.Format("15:04:05")))

	if len(results) == 0 {
		discordMsg.WriteString("No symbols detected.")
		sendDiscordNotification(webhookURL, discordMsg.String())
		return
	}

	// Sort: BUY first, then SELL, then NEUTRAL
	sort.Slice(results, func(i, j int) bool {
		si := results[i].OldAnlz.Signal
		sj := results[j].OldAnlz.Signal
		if si != sj {
			// BUY (0) < SELL (1) < NEUTRAL (2)
			rank := map[string]int{"BUY": 0, "SELL": 1, "NEUTRAL": 2}
			return rank[si] < rank[sj]
		}
		return results[i].Symbol < results[j].Symbol
	})

	discordMsg.WriteString("```\n")
	discordMsg.WriteString(fmt.Sprintf("%-6s | %5s | %10s | %10s | %6s | %6s | %s\n", "Symbol", "Price", "Bid (Lot)", "Off (Lot)", "Vol", "Freq", "Signal"))
	discordMsg.WriteString(strings.Repeat("-", 68) + "\n")

	for _, res := range results {
		icon := "⚪"
		if res.OldAnlz.Signal == "BUY" {
			icon = "🟢"
		} else if res.OldAnlz.Signal == "SELL" {
			icon = "🔴"
		}

		// Lot = Volume / 100
		bidLot := res.OldAnlz.BidVolume / 100
		offLot := res.OldAnlz.OfferVolume / 100

		discordMsg.WriteString(fmt.Sprintf("$%-5s | %5d | %10.0f | %10.0f | %6.2f | %6.2f | %s %s\n",
			res.Symbol, res.OldAnlz.LastPrice, bidLot, offLot, res.OldAnlz.VolumeRatio, res.OldAnlz.QueueRatio, icon, res.OldAnlz.Signal))
	}
	discordMsg.WriteString("```")

	sendDiscordNotification(webhookURL, discordMsg.String())
}

func SendExhaustionAnalysisReport(webhookURL string, results []SymbolResult, scanTime time.Time) {
	if webhookURL == "" {
		return
	}

	wib := time.FixedZone("WIB", 7*3600)
	scanTime = scanTime.In(wib)

	var discordMsg strings.Builder
	discordMsg.WriteString(fmt.Sprintf("💤 **Exhaustion Scanner [%s]**\n", scanTime.Format("15:04:05")))

	if len(results) == 0 {
		discordMsg.WriteString("No symbols detected.")
		sendDiscordNotification(webhookURL, discordMsg.String())
		return
	}

	// Sort: Exhaustion signals first
	sort.Slice(results, func(i, j int) bool {
		hasExI := false
		for _, s := range results[i].Signals {
			if s.Type == SignalSellerExhaustion || s.Type == SignalBuyerExhaustion {
				hasExI = true
				break
			}
		}
		hasExJ := false
		for _, s := range results[j].Signals {
			if s.Type == SignalSellerExhaustion || s.Type == SignalBuyerExhaustion {
				hasExJ = true
				break
			}
		}
		if hasExI != hasExJ {
			return hasExI // true (1) comes before false (0)
		}
		return results[i].Symbol < results[j].Symbol
	})

	discordMsg.WriteString("```\n")
	discordMsg.WriteString(fmt.Sprintf("%-6s | %5s | %5s | %s\n", "Symbol", "Price", "Score", "Signal"))
	discordMsg.WriteString(strings.Repeat("-", 35) + "\n")

	for _, res := range results {
		// Get highest score among EXHAUSTION
		var top Signal
		found := false
		for _, sig := range res.Signals {
			if sig.Type == SignalSellerExhaustion || sig.Type == SignalBuyerExhaustion {
				if sig.Score > top.Score {
					top = sig
					found = true
				}
			}
		}

		icon := "⚪"
		label := "NEUTRAL"
		scoreStr := "-"

		if found {
			icon = "🔵"
			label = "SELL.EXH"
			if top.Type == SignalBuyerExhaustion {
				icon = "🟡"
				label = "BUY.EXH"
			}
			scoreStr = fmt.Sprintf("%.1f", top.Score)
		}

		discordMsg.WriteString(fmt.Sprintf("%-6s | %5.0f | %5s | %s %-8s\n",
			res.Symbol, res.Price, scoreStr, icon, label))
	}
	discordMsg.WriteString("```")

	sendDiscordNotification(webhookURL, discordMsg.String())
}

func sendDiscordNotification(webhookURL string, message string) {
	const maxChars = 1900

	if len(message) <= maxChars {
		sendRawDiscordRequest(webhookURL, message)
		return
	}

	lines := strings.Split(message, "\n")
	var currentMsg strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
		}

		if currentMsg.Len()+len(line)+10 > maxChars {
			msgToSend := currentMsg.String()
			if inCodeBlock {
				msgToSend += "```"
			}
			sendRawDiscordRequest(webhookURL, msgToSend)

			currentMsg.Reset()
			if inCodeBlock {
				currentMsg.WriteString("```\n")
			}
		}
		currentMsg.WriteString(line + "\n")
	}

	if currentMsg.Len() > 0 {
		sendRawDiscordRequest(webhookURL, currentMsg.String())
	}
}

func sendRawDiscordRequest(webhookURL string, message string) {
	if strings.TrimSpace(message) == "" || strings.TrimSpace(message) == "```" {
		return
	}

	payload := map[string]interface{}{
		"content": message,
	}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func ClearTerminal() {
	cmd := exec.Command("clear")
	if os.Getenv("OS") == "Windows_NT" {
		cmd = exec.Command("cls")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}
