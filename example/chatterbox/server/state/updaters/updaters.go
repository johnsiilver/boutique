// Package updaters holds all the boutique.Updaters and the boutique.Modifer for the state store.
package updaters

import (
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/actions"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
)

// Modifier is a boutique.Modifier made up of all Updaters in this file.
var Modifier = boutique.NewModifier(SendMessage, DeleteMessages, AddUser, RemoveUser)

// SendMessage handles an Action of type ActSendMessage.
func SendMessage(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActSendMessage:
		up := action.Update.(actions.Message)
		s.Messages = boutique.CopyAppendSlice(s.Messages, data.Message{ID: up.ID, Timestamp: time.Now(), User: up.User, Text: up.Text}).([]data.Message)
	}
	return s
}

// DeleteMessages handles an Action of type ActDeleteMessages.
func DeleteMessages(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActDeleteMessages:
		id := action.Update.(int)
		glog.Infof("called deleted: %v", id)

		n := []data.Message{}
		found := false
		for _, v := range s.Messages {
			if v.ID == id {
				found = true
				continue
			}
			if found {
				n = append(n, v)
			}
		}
		if found {
			s.Messages = n
		}
	}
	return s
}

// AddUser handles an Action of type ActAddUser.
func AddUser(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActAddUser:
		s.Users = boutique.CopyAppendSlice(s.Users, action.Update).([]string)
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
