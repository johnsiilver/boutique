// Package data holds the Store object that is used by our boutique instances.
package data

// Stock represents a stock we are tracking.
type Stock struct {
	// Symbol is the stock symbol: googl or msft.
	Symbol string
	// Current is the current price.
	Current float64
	// Buy is a point in which we want to buy this stock.
	Buy float64
	// Sell is a point when we want to sell this stock.
	Sell float64
}

// State holds the data stored in boutique.Store.
type State struct {
	// Tracking is the current stocks we are tracking.
	Tracking map[string]Stock
}
