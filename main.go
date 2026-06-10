package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	TopLevel               = 3
	Threshold              = 2.0
	STOCKBIT_TOKEN         = "Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6ImExNWQ5OGE2LTdkYzgtNDM3NS05NDk0LTEyOWJlM2RlODVkNCIsInR5cCI6IkpXVCJ9.eyJkYXRhIjp7InVzZSI6IkVrYXl1c25pdGEiLCJlbWEiOiJla2F5dXNuaXRhLm5zMTJAZ21haWwuY29tIiwiZnVsIjoiRWtheXVzbml0YSIsInNlcyI6Imt1OEJCTk0xaUV0RGRuWXUiLCJkdmMiOiI5ZGM1NzI4MGQ4MGIzMGFmNTgxMmJlNjBiOWJlZjdjOSIsInVpZCI6MzU1NDkxOCwiY291IjoiSUQifSwiZXhwIjoxNzgxMTQ1NjcxLCJpYXQiOjE3ODEwNTkyNzEsImlzcyI6IlNUT0NLQklUIiwianRpIjoiMjdjZmFjNDItNWVhMS00M2EwLWI4NzEtYTQ4MDlhNGM4Nzc4IiwibmJmIjoxNzgxMDU5MjcxLCJ2ZXIiOiJ2MSJ9.Mw_HXz8JAG6DwDmFzEp4hv6bU5DbrlseA9ZWYEv2gAFnVO5eCc2yuoROdFshC1D7hYx_nrE5TKmHqo-C_NS-DaSisfE4O8DJFWLycfHCGNvveKa31_3eK2y22EAPny_jBb7_Eci8lX0TyKMrJNnH0ZbXE0XS-OkLY2na_fPtWVRmP2R9hZd17xnxdh52F1wzMBjOWhlfFDqPKoJsQRYWic7IkigHloBRoM2-va5k8yw5BMOjKx3DWM572Oq4C_Yk60vCg6aXyD2LpkMOg9MKP_4yo-K69-TQ3RcESikwMUFWqLmHChNrkvZsM2RmFUOfAxu0B4op4UYL-rNI0QThow"
	DISCORD_WEBHOOK_URL    = "https://discord.com/api/webhooks/1511451021178704044/3TV6cdYYkcE3R9A-9BLhVVNjQRLCowcFSHJhSNemuszPEkSqJKSVUtS1Y-3SPCTVkQxj"
	FREQ_OFFER_WEBHOOK_URL = "https://discord.com/api/webhooks/1514083684775629085/YSlkyPYDvdxnuU9JXRbwztoi_tWcqmUuTJFuL22fgQkkuXdidD6dlvfv5bpQy42unbSj"
	FREQ_OFFER_THRESHOLD   = 3.0

	// Pengaturan Jam Operasional (WIB)
	UseTradingHours = true
	StartHour       = 8
	StartMinute     = 30
	EndHour         = 16
	EndMinute       = 30

	// Toggle Sinyal (true = ON, false = OFF)
	EnableSignalBuy     = true
	EnableSignalSell    = false
	EnableSignalNeutral = false
)

type OrderBookLevel struct {
	Price  string `json:"price"`
	QueNum string `json:"que_num"`
	Volume string `json:"volume"`
}

type Item struct {
	Symbol           string           `json:"symbol"`
	LastPrice        int              `json:"lastprice"`
	PercentageChange float64          `json:"percentage_change"`
	Bid              []OrderBookLevel `json:"bid"`
	Offer            []OrderBookLevel `json:"offer"`
}

type Response struct {
	Data struct {
		Item []Item `json:"item"`
	} `json:"data"`
}

type Analysis struct {
	Symbol           string
	LastPrice        int
	PercentageChange float64
	BidVolume        float64
	OfferVolume      float64
	BidQueue         float64
	OfferQueue       float64
	VolumeRatio      float64
	QueueRatio       float64
	Signal           string
}

type SignalResult struct {
	Analysis
	BidLot   float64
	OfferLot float64
}

func toFloat(v string) float64 {
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func fetchOrderBook(templateID string, token string) (*Response, error) {

	url := fmt.Sprintf(
		"https://exodus.stockbit.com/company-price-feed/v2/orderbook/template/%s",
		templateID,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://stockbit.com")
	req.Header.Set("Referer", "https://stockbit.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Response

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func analyze(item Item) Analysis {

	topBid := TopLevel
	if len(item.Bid) < topBid {
		topBid = len(item.Bid)
	}

	topOffer := TopLevel
	if len(item.Offer) < topOffer {
		topOffer = len(item.Offer)
	}

	var bidVol float64
	var offerVol float64
	var bidQueue float64
	var offerQueue float64

	for i := 0; i < topBid; i++ {
		bidVol += toFloat(item.Bid[i].Volume)
		bidQueue += toFloat(item.Bid[i].QueNum)
	}

	for i := 0; i < topOffer; i++ {
		offerVol += toFloat(item.Offer[i].Volume)
		offerQueue += toFloat(item.Offer[i].QueNum)
	}

	volumeRatio := 0.0
	queueRatio := 0.0

	if bidVol > 0 {
		volumeRatio = offerVol / bidVol
	}

	if bidQueue > 0 {
		queueRatio = offerQueue / bidQueue
	}

	signal := "NEUTRAL"

	// Banyak ritel di offer => BUY
	if volumeRatio >= Threshold &&
		queueRatio >= Threshold {
		signal = "BUY"
	}

	// Banyak ritel di bid => SELL
	if bidVol > 0 &&
		offerVol > 0 &&
		bidQueue > 0 &&
		offerQueue > 0 {

		if (bidVol/offerVol) >= Threshold &&
			(bidQueue/offerQueue) >= Threshold {
			signal = "SELL"
		}
	}

	return Analysis{
		Symbol:           item.Symbol,
		LastPrice:        item.LastPrice,
		PercentageChange: item.PercentageChange,
		BidVolume:        bidVol,
		OfferVolume:      offerVol,
		BidQueue:         bidQueue,
		OfferQueue:       offerQueue,
		VolumeRatio:      volumeRatio,
		QueueRatio:       queueRatio,
		Signal:           signal,
	}
}

func sendDiscordNotification(webhookURL string, message string) {
	const maxChars = 1900 // Limit Discord 2000, gunakan 1900 untuk aman

	if len(message) <= maxChars {
		sendRawDiscordRequest(webhookURL, message)
		return
	}

	// Pecah berdasarkan baris agar tidak memotong tengah tabel
	lines := strings.Split(message, "\n")
	var currentMsg strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		// Cek apakah kita sedang di dalam code block
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
		}

		// Jika menambah baris ini melebihi limit
		if currentMsg.Len()+len(line)+10 > maxChars {
			msgToSend := currentMsg.String()
			if inCodeBlock {
				msgToSend += "```" // Tutup code block di pesan ini
			}
			sendRawDiscordRequest(webhookURL, msgToSend)

			currentMsg.Reset()
			if inCodeBlock {
				currentMsg.WriteString("```\n") // Buka kembali code block di pesan baru
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

	payload := map[string]string{
		"content": message,
	}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Error sending Discord notification: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		log.Printf("Discord returned non-OK status: %d\n", resp.StatusCode)
	}
}

func isTradingTime() bool {
	if !UseTradingHours {
		return true
	}

	// Gunakan zona waktu WIB (UTC+7)
	wib := time.FixedZone("WIB", 7*3600)
	now := time.Now().In(wib)

	// Cek hari (Senin-Jumat)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := StartHour*60 + StartMinute
	endMinutes := EndHour*60 + EndMinute

	return currentMinutes >= startMinutes && currentMinutes <= endMinutes
}

func formatBidOfferTable(item Item) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 **Bid Offer: %s** (Price: %d)\n", item.Symbol, item.LastPrice))
	sb.WriteString("```\n")
	sb.WriteString(fmt.Sprintf("%-6s | %10s | %6s | %6s | %10s | %6s\n", "Freq", "Lot", "Bid", "Offer", "Lot", "Freq"))
	sb.WriteString(strings.Repeat("-", 59) + "\n")

	maxLevels := 10
	bidLen := len(item.Bid)
	offLen := len(item.Offer)
	rows := bidLen
	if offLen > rows {
		rows = offLen
	}
	if rows > maxLevels {
		rows = maxLevels
	}

	var totalBidLot, totalOffLot float64
	var totalBidFreq, totalOffFreq float64

	for i := 0; i < rows; i++ {
		bFreq, bLot, bPrice := "-", "-", "-"
		if i < bidLen {
			bFreq = item.Bid[i].QueNum
			lot := toFloat(item.Bid[i].Volume) / 100
			bLot = fmt.Sprintf("%.0f", lot)
			bPrice = item.Bid[i].Price
			totalBidLot += lot
			totalBidFreq += toFloat(item.Bid[i].QueNum)
		}

		oFreq, oLot, oPrice := "-", "-", "-"
		if i < offLen {
			oFreq = item.Offer[i].QueNum
			lot := toFloat(item.Offer[i].Volume) / 100
			oLot = fmt.Sprintf("%.0f", lot)
			oPrice = item.Offer[i].Price
			totalOffLot += lot
			totalOffFreq += toFloat(item.Offer[i].QueNum)
		}

		sb.WriteString(fmt.Sprintf("%-6s | %10s | %6s | %6s | %10s | %6s\n", bFreq, bLot, bPrice, oPrice, oLot, oFreq))
	}

	sb.WriteString(strings.Repeat("-", 59) + "\n")
	sb.WriteString(fmt.Sprintf("%-6.0f | %10.0f | %15s | %10.0f | %6.0f\n", totalBidFreq, totalBidLot, "TOTAL", totalOffLot, totalOffFreq))
	sb.WriteString("```")
	return sb.String()
}

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	token := STOCKBIT_TOKEN
	discordWebhook := DISCORD_WEBHOOK_URL
	freqOfferWebhook := FREQ_OFFER_WEBHOOK_URL

	if token == "" {
		log.Fatal("STOCKBIT_TOKEN kosong")
	}

	if discordWebhook == "" {
		log.Println("Warning: DISCORD_WEBHOOK_URL tidak ditemukan di .env")
	}

	if freqOfferWebhook == "" {
		log.Println("Warning: FREQ_OFFER_WEBHOOK_URL tidak ditemukan di .env")
	}

	// Template ID yang ingin diambil (default 0)
	// Berdasarkan info user, template ID 0 berisi daftar saham yang ingin dipantau
	templateIDs := []string{"177667"}
	wib := time.FixedZone("WIB", 7*3600)

	for {
		nowWIB := time.Now().In(wib)
		if !isTradingTime() {
			fmt.Printf("[%s] Di luar jam operasional (08:30-16:30 WIB). Menunggu...\n", nowWIB.Format("15:04:05"))
			time.Sleep(1 * time.Minute)
			continue
		}

		fmt.Printf("\n[%s] Memulai analisis...\n", nowWIB.Format("15:04:05"))

		var signals []SignalResult

		for _, tid := range templateIDs {
			resp, err := fetchOrderBook(tid, token)
			if err != nil {
				log.Printf("Error fetching template %s: %v\n", tid, err)
				continue
			}

			for _, item := range resp.Data.Item {
				result := analyze(item)

				// Konversi ke Lot (/100)
				bidLot := result.BidVolume / 100
				offerLot := result.OfferVolume / 100

				fmt.Printf(
					"%-6s | Price=%d BidLot=%10.0f OfferLot=%10.0f BidQ=%6.0f OfferQ=%6.0f VR=%.2f QR=%.2f => %s\n",
					result.Symbol,
					result.LastPrice,
					bidLot,
					offerLot,
					result.BidQueue,
					result.OfferQueue,
					result.VolumeRatio,
					result.QueueRatio,
					result.Signal,
				)

				// Filter berdasarkan toggle sinyal
				showSignal := false
				if result.Signal == "BUY" && EnableSignalBuy {
					showSignal = true
				} else if result.Signal == "SELL" && EnableSignalSell {
					showSignal = true
				} else if result.Signal == "NEUTRAL" && EnableSignalNeutral {
					showSignal = true
				}

				if showSignal {
					signals = append(signals, SignalResult{
						Analysis: result,
						BidLot:   bidLot,
						OfferLot: offerLot,
					})
				}

				// Notifikasi khusus jika Freq Offer (QueueRatio) > threshold
				if result.QueueRatio > FREQ_OFFER_THRESHOLD && freqOfferWebhook != "" {
					tableMsg := formatBidOfferTable(item)
					sendDiscordNotification(freqOfferWebhook, tableMsg)
				}
			}
		}

		// Kirim ke Discord jika ada sinyal yang lolos filter
		if len(signals) > 0 && discordWebhook != "" {
			// Urutkan berdasarkan Freq (QueueRatio) terbesar
			sort.Slice(signals, func(i, j int) bool {
				return signals[i].QueueRatio > signals[j].QueueRatio
			})

			var discordMsg strings.Builder
			discordMsg.WriteString(fmt.Sprintf("🚀 **Algo Bid Offer 3 Papan Teratas [%s]**\n", nowWIB.Format("15:04:05")))
			discordMsg.WriteString("```\n")
			discordMsg.WriteString(fmt.Sprintf("%-6s | %5s | %5s | %5s | %-6s | %s\n", "Symbol", "Price", "Gain", "Vol", "Freq", "Signal"))
			discordMsg.WriteString(strings.Repeat("-", 47) + "\n")

			for _, res := range signals {
				icon := "⚪️" // Default Neutral
				label := res.Signal
				if res.Signal == "BUY" {
					icon = "🟢"
				} else if res.Signal == "SELL" {
					icon = "🔴"
				} else if res.Signal == "NEUTRAL" {
					label = "N"
				}

				freqBase := fmt.Sprintf("%.2f", res.QueueRatio)
				// Gunakan format string langsung tanpa padding fmt untuk kolom yang ada emoji
				var freqDisplay string
				if res.QueueRatio > FREQ_OFFER_THRESHOLD {
					freqDisplay = fmt.Sprintf("%-4s🔥", freqBase)
				} else {
					freqDisplay = fmt.Sprintf("%-4s  ", freqBase)
				}

				discordMsg.WriteString(fmt.Sprintf("$%-5s | %5d | %4.0f%% | %5.2f | %-6s | %s %s\n",
					res.Symbol, res.LastPrice, res.PercentageChange, res.VolumeRatio, freqDisplay, icon, label))
			}
			discordMsg.WriteString("```")

			sendDiscordNotification(discordWebhook, discordMsg.String())
		}

		fmt.Printf("\n[%s] Selesai. Menunggu 20 detik untuk refresh berikutnya...\n", nowWIB.Format("15:04:05"))
		time.Sleep(20 * time.Second)
	}
}
