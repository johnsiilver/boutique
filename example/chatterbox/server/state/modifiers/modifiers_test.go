package modifiers

import (
	"runtime"
	"testing"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/actions"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
	"github.com/kylelemons/godebug/pretty"
	"github.com/lukechampine/freeze"
)

// supportedOS prevents tests from running on non-unix systems.
// Windows cannot freeze memory.
func supportedOS() bool {
	switch runtime.GOOS {
	case "linux", "darwin":
		return true
	}
	return false
}

func TestSendMessage(t *testing.T) {
	if !supportedOS() {
		return
	}

	// This validates that we didn't mutate our map.
	msgs := []data.Message{data.Message{ID: -1, User: "dave"}} // Need to have an initial entry.
	msgs = freeze.Slice(msgs).([]data.Message)
	d := data.State{Messages: msgs}

	newState := SendMessage(d, boutique.Action{Type: actions.ActSendMessage, Update: data.Message{User: "john"}})

	if diff := pretty.Compare([]data.Message{{ID: -1, User: "dave"}, {ID: 0, User: "john"}}, newState.(data.State).Messages); diff != "" {
		t.Errorf("TestSendMessage: -want/+got:\n%s", diff)
	}
}

func TestAddUser(t *testing.T) {
	if !supportedOS() {
		return
	}

	// This validates that we didn't mutate our map.
	users := []string{"dave"}
	users = freeze.Slice(users).([]string)
	d := data.State{Users: users}

	newState := AddUser(d, boutique.Action{Type: actions.ActAddUser, Update: "john"})

	if diff := pretty.Compare([]string{"dave", "john"}, newState.(data.State).Users); diff != "" {
		t.Errorf("TestAddUser: -want/+got:\n%s", diff)
	}
}

func TestRemoveUser(t *testing.T) {
	if !supportedOS() {
		return
	}

	// This validates that we didn't mutate our map.
	users := []string{"dave", "john"}
	users = freeze.Slice(users).([]string)
	d := data.State{Users: users}

	newState := RemoveUser(d, boutique.Action{Type: actions.ActRemoveUser, Update: "john"})

	if diff := pretty.Compare([]string{"dave"}, newState.(data.State).Users); diff != "" {
		t.Errorf("TestAddUser: -want/+got:\n%s", diff)
	}
}
