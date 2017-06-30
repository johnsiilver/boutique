// Package middleware provides middleware to our boutique.Container.
package middleware

import (
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/actions"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
	"github.com/kylelemons/godebug/pretty"
)

var pConfig = &pretty.Config{
	Diffable: true,

	// Field and value options
	IncludeUnexported:   false,
	PrintStringers:      true,
	PrintTextMarshalers: true,

	Formatter: map[reflect.Type]interface{}{
		reflect.TypeOf(data.OpenFile{}):        nil,
		reflect.TypeOf((*Logging)(nil)).Elem(): nil,
	},
}

// Logging provides middleware for logging channels and state data debugging.
type Logging struct {
	lastData boutique.State
}

// DebugLog implements boutique.Middleware.
func (l *Logging) DebugLog(args *boutique.MWArgs) (changedData interface{}, stop bool, err error) {
	go func() {
		defer args.WG.Done()
		state := <-args.Committed
		if state.IsZero() { // This indicates that another middleware killed the commit.  No need to log.
			return
		}
		d := state.Data.(data.State)
		if !d.LogDebug {
			return
		}
		if d.DebugFile.IsZero() {
			glog.Errorf("asked to do a debug log, but data.State.DebufFile is zero vlaue")
			return
		}
		_, err := d.DebugFile.WriteString(fmt.Sprintf("%s\n\n", pConfig.Compare(l.lastData, state)))
		if err != nil {
			glog.Errorf("problem writing to debug file: %s", err)
		}
		l.lastData = state
	}()
	return nil, false, nil
}

// ChannelLog implements boutique.Middleware.
func (l *Logging) ChannelLog(args *boutique.MWArgs) (changedData interface{}, stop bool, err error) {
	if args.Action.Type != actions.ActSendMessage {
		args.WG.Done()
		return nil, false, nil
	}

	d := args.GetState().Data.(data.State)
	if !d.LogChan {
		args.WG.Done()
		return nil, false, nil
	}

	go func() {
		defer args.WG.Done() // Signal when we are done. Not doing this will caused the program to stall.
		state := <-args.Committed
		if state.IsZero() { // This indicates that another middleware killed the commit.  No need to log.
			return
		}

		d := state.Data.(data.State)
		if d.ChanFile.IsZero() {
			glog.Errorf("log channel messages, but data.State.ChanFile is zero vlaue")
			return
		}

		lmsg := d.Messages[len(d.Messages)-1]
		_, err := d.ChanFile.WriteString(fmt.Sprintf("%d:::%s:::%s:::%s", lmsg.ID, lmsg.Timestamp, lmsg.User, lmsg.Text))
		if err != nil {
			glog.Errorf("problem writing to channel file: %s", err)
		}

	}()
	return nil, false, nil
}

// CleanTimer is how old a message must be before it is deleted on the next Perform().
var CleanTimer = 1 * time.Minute

// CleanMessages deletes data.State.Messages older than 1 Minute.
func CleanMessages(args *boutique.MWArgs) (changedData interface{}, stop bool, err error) {
	defer args.WG.Done()

	d := args.NewData.(data.State)

	var (
		i         int
		m         data.Message
		deleteAll = true
		now       = time.Now()
	)
	for i, m = range d.Messages {
		if now.Sub(m.Timestamp) < CleanTimer {
			deleteAll = false
			break
		}
	}

	switch {
	case i == 0:
		return nil, false, nil
	case deleteAll:
		d.Messages = []data.Message{}
	case len(d.Messages[i:]) > 0:
		newMsg := make([]data.Message, len(d.Messages[i:]))
		copy(newMsg, d.Messages[i:])
		d.Messages = newMsg
	}
	return d, false, nil
}

// EnforceMsgLength tests that an actions.ActSendMessage does not contain a
// message longer than 500 characters.
func EnforceMsgLength(args *boutique.MWArgs) (changedData interface{}, stop bool, err error) {
	defer args.WG.Done()

	if args.Action.Type == actions.ActSendMessage {
		m := args.Action.Update.(data.Message)
		if len(m.Text) > 500 {
			return nil, false, fmt.Errorf("cannot send a Message > 500 characters")
		}
	}
	return nil, false, nil
}
