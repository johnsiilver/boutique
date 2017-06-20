// Package state contains our Hub, which is used to store data for a particular
// channel users are communicating on.
package state

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/middleware"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/updaters"
	"github.com/pborman/uuid"
)

// Hub contains the central store and our middleware.
type Hub struct {
	// Store is our boutique.Store.
	Store *boutique.Store

	// Logging holds middleware that is used to log changes.
	Logging *middleware.Logging
}

// New is the contstructor for Hub.
func New(channelName string) (*Hub, error) {
	l := &middleware.Logging{}
	d := data.State{
		ServerID: uuid.New(),
		Channel:  channelName,
		Users:    make([]string, 0, 1),
		Messages: make([]data.Message, 0, 10),
	}

	mw := []boutique.Middleware{l.DebugLog, l.ChannelLog}

	s, err := boutique.New(d, updaters.Modifier, mw)
	if err != nil {
		return nil, err
	}

	return &Hub{Store: s, Logging: l}, nil
}
