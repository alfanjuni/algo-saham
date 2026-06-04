package detector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Response for Trade Book Chart API
type TradeBookChartResponse struct {
	Data struct {
		Buy []struct {
			Time string `json:"time"`
			Lot  struct {
				Raw interface{} `json:"raw"`
			} `json:"lot"`
			Value struct {
				Raw interface{} `json:"raw"`
			} `json:"value"`
			Frequency struct {
				Raw interface{} `json:"raw"`
			} `json:"frequency"`
		} `json:"buy"`
		Sell []struct {
			Time string `json:"time"`
			Lot  struct {
				Raw interface{} `json:"raw"`
			} `json:"lot"`
			Value struct {
				Raw interface{} `json:"raw"`
			} `json:"value"`
			Frequency struct {
				Raw interface{} `json:"raw"`
			} `json:"frequency"`
		} `json:"sell"`
		Prices []struct {
			Time  string `json:"time"`
			Value struct {
				Raw interface{} `json:"raw"`
			} `json:"value"`
		} `json:"prices"`
		NetValuesVolume []struct {
			Time string `json:"time"`
			Lot  struct {
				Raw interface{} `json:"raw"`
			} `json:"lot"`
		} `json:"net_values_volume"`
		BookTotal struct {
			BuyLot         interface{} `json:"buy_lot"`
			SellLot        interface{} `json:"sell_lot"`
			BuyPercentage  string      `json:"buy_percentage"`
			SellPercentage string      `json:"sell_percentage"`
		} `json:"book_total"`
	} `json:"data"`
}

func FetchOrderBook(templateID string, token string) (*OrderBookResponse, error) {
	url := fmt.Sprintf("https://exodus.stockbit.com/company-price-feed/v2/orderbook/template/%s", templateID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", token) // Expecting "Bearer ..."
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://stockbit.com")
	req.Header.Set("Referer", "https://stockbit.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result OrderBookResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func FetchTradeBook(symbol string, date string, token string) (*TradeBookChartResponse, error) {
	url := fmt.Sprintf("https://exodus.stockbit.com/order-trade/trade-book/chart?symbol=%s&time_interval=1m&date=%s", symbol, date)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result TradeBookChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
