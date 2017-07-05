// Package modifiers holds all the boutique.Updaters and the boutique.Modifer for the state store.
package modifiers

import (
	"sort"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/actions"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
)

// All is a boutique.Modifiers made up of all Modifier(s) in this file.
var All = boutique.NewModifiers(SendMessage, AddUser, RemoveUser)

// SendMessage handles an Action of type ActSendMessage.
func SendMessage(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActSendMessage:
		msg := action.Update.(data.Message)
		msg.ID = s.NextMsgID
		s.Messages = boutique.CopyAppendSlice(s.Messages, msg).([]data.Message)
		s.NextMsgID = s.NextMsgID + 1
	}
	return s
}

// AddUser handles an Action of type ActAddUser.
func AddUser(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActAddUser:
		s.Users = boutique.CopyAppendSlice(s.Users, action.Update).([]string)
		sort.Strings(s.Users)
	}
	return s
}

// RemoveUser handles an Action of type ActRemoveUser.
func RemoveUser(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActRemoveUser:
		n := make([]string, 0, len(s.Users)-1)
		for _, u := range s.Users {
			if u != action.Update.(string) {
				n = append(n, u)
			}
		}
		s.Users = n
	}
	return s
}
