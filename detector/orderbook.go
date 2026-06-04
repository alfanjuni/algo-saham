package detector

import (
	"strconv"
)

const (
	TopLevel  = 3
	Threshold = 2.0
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

type OrderBookResponse struct {
	Data struct {
		Item []Item `json:"item"`
	} `json:"data"`
}

type OrderBookAnalysis struct {
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

func ToFloat(v string) float64 {
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func AnalyzeOrderBook(item Item) OrderBookAnalysis {
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
		bidVol += ToFloat(item.Bid[i].Volume)
		bidQueue += ToFloat(item.Bid[i].QueNum)
	}

	for i := 0; i < topOffer; i++ {
		offerVol += ToFloat(item.Offer[i].Volume)
		offerQueue += ToFloat(item.Offer[i].QueNum)
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

	return OrderBookAnalysis{
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
