// Package actions details boutique.Actions that are used by modifiers to modify the store.
package actions

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/notifier/state/data"
)

const (
	// ActTrack indicates we want to add a new stock to track.
	ActTrack boutique.ActionType = iota
	// ActUntrack indicates we want to remove a stock from tracking.
	ActUntrack
	// ActChangePoint indicates we want to change the buy/sell for a stock.
	ActChangePoint
)

// Track tells the application to track a new stock.
func Track(symbol string, buy, sell float64) boutique.Action {
	m := data.Stock{Symbol: symbol, Buy: buy, Sell: sell}
	return boutique.Action{Type: ActTrack, Update: m}
}

// Untrack tells the application to stop tracking a stock.
func Untrack(symbol string) boutique.Action {
	return boutique.Action{Type: ActUntrack, Update: symbol}
}

// ChangeBuy indicates we wish to change the buy point for a stock.
func ChangeBuy(symbol string, buy float64) boutique.Action {
	return boutique.Action{Type: ActChangePoint, Update: data.Stock{Symbol: symbol, Current: -1.0, Buy: buy, Sell: -1.0}}
}

// ChangeSell indicates we wish to change the sell point for a stock.
func ChangeSell(symbol string, sell float64) boutique.Action {
	return boutique.Action{Type: ActChangePoint, Update: data.Stock{Symbol: symbol, Current: -1.0, Buy: -1.0, Sell: sell}}
}

// ChangeCurrent indicates we wish to change the current price of the stock.
func ChangeCurrent(symbol string, current float64) boutique.Action {
	return boutique.Action{Type: ActChangePoint, Update: data.Stock{Symbol: symbol, Current: current, Buy: -1.0, Sell: -1}}
}
