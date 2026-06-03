package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	TopLevel            = 3
	Threshold           = 2.0
	STOCKBIT_TOKEN      = "Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6ImExNWQ5OGE2LTdkYzgtNDM3NS05NDk0LTEyOWJlM2RlODVkNCIsInR5cCI6IkpXVCJ9.eyJkYXRhIjp7InVzZSI6IkVrYXl1c25pdGEiLCJlbWEiOiJla2F5dXNuaXRhLm5zMTJAZ21haWwuY29tIiwiZnVsIjoiRWtheXVzbml0YSIsInNlcyI6Imt1OEJCTk0xaUV0RGRuWXUiLCJkdmMiOiI5ZGM1NzI4MGQ4MGIzMGFmNTgxMmJlNjBiOWJlZjdjOSIsInVpZCI6MzU1NDkxOCwiY291IjoiSUQifSwiZXhwIjoxNzgwNTEyODAxLCJpYXQiOjE3ODA0MjY0MDEsImlzcyI6IlNUT0NLQklUIiwianRpIjoiNDRjOTcyOGMtZDI2ZC00YzQ0LTllMjAtNjlhMmMwMTQwYWQ3IiwibmJmIjoxNzgwNDI2NDAxLCJ2ZXIiOiJ2MSJ9.fJXPzUpKepugO2vHFIDf7PkQWjHEMpnx8VIyP81N5WyLqShq-TQDGCkmfkQanE9Oeflka24SJMEVIkdRESBkPxYpB5pdeOEzGYoR-ViG0gLEu8MJUIn8MsF98Pv9hICo-8bKw3GhRRhDx6wKHUNwr_zi-bVOwKmhPoyVPyZALb4STrHh6dknILFjeRLyFDjD_09CJ9yE8VRBKha7pmF8pSy36u1ajyvfq6kWuF0w8VDt_Ar8IGUFxzK5vT4Or38ISGpl1Z6H-64hJJLK8DUlXrgKnJ7KxFeJfcPx5jrr7uP3o8NIVg2ANDQC6VavFDxXC2fRaFuxaM6s8lP96YY1xg"
	DISCORD_WEBHOOK_URL = "https://discord.com/api/webhooks/1511451021178704044/3TV6cdYYkcE3R9A-9BLhVVNjQRLCowcFSHJhSNemuszPEkSqJKSVUtS1Y-3SPCTVkQxj"
)

type OrderBookLevel struct {
	Price  string `json:"price"`
	QueNum string `json:"que_num"`
	Volume string `json:"volume"`
}

type Item struct {
	Symbol    string           `json:"symbol"`
	LastPrice int              `json:"lastprice"`
	Bid       []OrderBookLevel `json:"bid"`
	Offer     []OrderBookLevel `json:"offer"`
}

type Response struct {
	Data struct {
		Item []Item `json:"item"`
	} `json:"data"`
}

type Analysis struct {
	Symbol      string
	LastPrice   int
	BidVolume   float64
	OfferVolume float64
	BidQueue    float64
	OfferQueue  float64
	VolumeRatio float64
	QueueRatio  float64
	Signal      string
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
		Symbol:      item.Symbol,
		LastPrice:   item.LastPrice,
		BidVolume:   bidVol,
		OfferVolume: offerVol,
		BidQueue:    bidQueue,
		OfferQueue:  offerQueue,
		VolumeRatio: volumeRatio,
		QueueRatio:  queueRatio,
		Signal:      signal,
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

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	token := STOCKBIT_TOKEN
	discordWebhook := DISCORD_WEBHOOK_URL

	if token == "" {
		log.Fatal("STOCKBIT_TOKEN kosong")
	}

	if discordWebhook == "" {
		log.Println("Warning: DISCORD_WEBHOOK_URL tidak ditemukan di .env")
	}

	// Template ID yang ingin diambil (default 0)
	// Berdasarkan info user, template ID 0 berisi daftar saham yang ingin dipantau
	templateIDs := []string{"177667"}

	for {
		var buyList []string
		var sellList []string

		fmt.Printf("\n[%s] Memulai analisis...\n", time.Now().Format("15:04:05"))

		var discordMsg strings.Builder
		discordMsg.WriteString(fmt.Sprintf("🚀 **Algo Bid Offer 3 Papan Teratas [%s]**\n", time.Now().Format("15:04:05")))
		discordMsg.WriteString("```\n")
		discordMsg.WriteString(fmt.Sprintf("%-6s | %5s | %10s | %10s | %5s | %5s | %s\n", "Symbol", "Price", "Bid (Lot)", "Off (Lot)", "Vol", "Freq", "Signal"))
		discordMsg.WriteString(strings.Repeat("-", 68) + "\n")

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

				discordMsg.WriteString(fmt.Sprintf("$%-5s | %5d | %10.0f | %10.0f | %5.2f | %5.2f | %s\n",
					result.Symbol, result.LastPrice, bidLot, offerLot, result.VolumeRatio, result.QueueRatio, result.Signal))

				switch result.Signal {
				case "BUY":
					buyList = append(buyList, fmt.Sprintf("$%s @ %d", result.Symbol, result.LastPrice))

				case "SELL":
					sellList = append(sellList, fmt.Sprintf("$%s @ %d", result.Symbol, result.LastPrice))
				}
			}
		}
		discordMsg.WriteString("```")

		fmt.Println()
		fmt.Println("======== BUY OFFER RITEL ==========")
		for _, s := range buyList {
			fmt.Println(s)
		}

		fmt.Println()
		fmt.Println("======== SELL OFFER BANDAR =======")
		for _, s := range sellList {
			fmt.Println(s)
		}

		// Kirim notifikasi Discord (Semua saham + Highlight BUY/SELL)
		if discordWebhook != "" {
			if len(buyList) > 0 {
				discordMsg.WriteString("\n✅ **BUY:**\n")
				for _, s := range buyList {
					discordMsg.WriteString(fmt.Sprintf("- %s\n", s))
				}
			}
			if len(sellList) > 0 {
				discordMsg.WriteString("\n❌ **SELL:**\n")
				for _, s := range sellList {
					discordMsg.WriteString(fmt.Sprintf("- %s\n", s))
				}
			}

			sendDiscordNotification(discordWebhook, discordMsg.String())
		}

		fmt.Printf("\nSelesai. Menunggu 20 detik untuk refresh berikutnya...\n")
		time.Sleep(20 * time.Second)
	}
}
