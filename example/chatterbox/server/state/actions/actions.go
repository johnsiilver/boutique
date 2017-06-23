// Package actions details boutique.Actions that are used by modifiers to modify the store.
package actions

import (
	"fmt"
	"time"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
)

const (
	// ActSendMessage indicates we want to send a message via the store.
	ActSendMessage = iota
	// ActDeleteMessages indicates we want to delete messages from the store.
	ActDeleteMessages
	// ActAddUser indicates the Action wants to add a user to the store.
	ActAddUser
	// ActRemoveUser indicates the Action wants to remove a user from the store.
	ActRemoveUser
)

// SendMessage sends a message via the store.
func SendMessage(id int, user string, s string) (boutique.Action, error) {
	if len(s) > 500 {
		return boutique.Action{}, fmt.Errorf("cannot send a message of more than 500 characters")
	}
	m := data.Message{ID: id, Timestamp: time.Now(), User: user, Text: s}
	return boutique.Action{Type: ActSendMessage, Update: m}, nil
}

// DeleteMessages deletes messages in our .Messages slice from the front until
// we reach lastMsgID (inclusive).
func DeleteMessages(lastMsgID int) boutique.Action {
	return boutique.Action{Type: ActDeleteMessages, Update: lastMsgID}
}

// AddUser adds a user to the store, indicating a new user is in the room.
func AddUser(u string) boutique.Action {
	return boutique.Action{Type: ActAddUser, Update: u}
}

// RemoveUser removes a user from the store, indicating a user has left the room.
func RemoveUser(u string) boutique.Action {
	return boutique.Action{Type: ActRemoveUser, Update: u}
}
