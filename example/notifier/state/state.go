// Package state contains our Hub, which is used to store data for a particular
// channel users are communicating on.
package state

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/notifier/state/data"
	"github.com/johnsiilver/boutique/example/notifier/state/modifiers"
)

// New is the constructor for a boutique.Store
func New() (*boutique.Store, error) {
	d := data.State{
		Tracking: map[string]data.Stock{},
	}

	return boutique.New(d, modifiers.All, nil)
}
