// Package middleware provides middleware to our boutique.Container.
package middleware

import (
	"fmt"
	"reflect"

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
		defer args.WG.Done() // Signal when we are done. Not doing this will caused the program to stall.
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
