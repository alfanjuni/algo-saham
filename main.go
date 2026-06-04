package main

import (
	"algo-saham/detector"
	"algo-saham/domain"
	"algo-saham/helper"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	_ = godotenv.Load()

	token := os.Getenv("STOCKBIT_TOKEN")
	if token == "" {
		token = domain.STOCKBIT_TOKEN
	}

	discordWebhook := os.Getenv("DISCORD_WEBHOOK_URL")
	if discordWebhook == "" {
		discordWebhook = domain.DISCORD_WEBHOOK_URL
	}

	// Template ID yang ingin diambil
	templateID := "177667"

	for {
		if !helper.IsTradingTime() {
			detector.ClearTerminal()
			wib := time.FixedZone("WIB", 7*3600)
			fmt.Printf("[%s] Di luar jam operasional (08:30-16:30 WIB). Menunggu...\n", time.Now().In(wib).Format("15:04:05"))
			time.Sleep(1 * time.Minute)
			continue
		}

		// 1. Scan and analyze everything (Template + Tradebook)
		date := helper.GetEffectiveDate()
		results := detector.ScanSymbols(templateID, date, token)

		// 2. Print results table to terminal
		detector.PrintTable(results, time.Now())

		// Get Webhook URLs
		exhaustionWebhook := os.Getenv("EXHAUSTION_WEBHOOK_URL")
		if exhaustionWebhook == "" {
			exhaustionWebhook = domain.EXHAUSTION_WEBHOOK_URL
		}

		// 4. Send reports to Discord
		// Orderflow Scanner (Z-Score) - NONAKTIF
		// detector.SendZScoreOrderflowReport(exhaustionWebhook, results, time.Now())

		// Bid/Offer Analysis (3 Papan Teratas)
		detector.SendBidOfferAnalysisReport(discordWebhook, results, time.Now())

		// Market Exhaustion Analysis (Seller/Buyer Exhaustion)
		detector.SendExhaustionAnalysisReport(exhaustionWebhook, results, time.Now())

		// 5. Wait for next refresh
		time.Sleep(20 * time.Second)
	}
}
