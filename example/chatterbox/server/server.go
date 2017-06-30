// Package server implements a websocket server that sets up an irc like server.
package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/messages"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/actions"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type channel struct {
	hub   *state.Hub
	users map[string]bool
}

// ChatterBox implements a websocket server for sending messages in channels.
// Similar to IRC.
type ChatterBox struct {
	chMu     sync.RWMutex
	channels map[string]*channel
}

// New is the constructor for ChatterBox.
func New() *ChatterBox {
	return &ChatterBox{channels: map[string]*channel{}}
}

// Handler implements http.HandleFunc.
func (c *ChatterBox) Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		glog.Errorf("error connecting to server: %s", err)
		return
	}

	m, err := c.read(conn)
	if err != nil {
		glog.Error(err)
		return
	}

	if m.Type != messages.CMSubscribe {
		glog.Errorf("first message on a websocket must be of type Subscribe")
		return
	}

	if err = m.Validate(); err != nil {
		glog.Error(err)
		return
	}

	state, err := c.subscribe(conn, m)
	if err != nil {
		return
	}
	defer c.unsubscribe(m.User, m.Channel)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go c.clientReceiver(wg, m.User, m.Channel, conn, state.Store)
	go c.clientSender(wg, m.User, m.Channel, conn, state.Store)

	wg.Wait()
}

// subscribe subscribes a user to the channel.
func (c *ChatterBox) subscribe(conn *websocket.Conn, m messages.Client) (*state.Hub, error) {
	c.chMu.Lock()
	defer c.chMu.Unlock()

	hub, err := state.New(m.Channel)
	if err != nil {
		return nil, err
	}

	mchan, ok := c.channels[m.Channel]
	if !ok {
		mchan = &channel{hub: hub, users: map[string]bool{m.User: true}}
		c.channels[m.Channel] = mchan
	} else {
		if mchan.users[m.User] {
			c.write( // Ignore error, because its reporting an error, an error here has nothing to do.
				conn,
				messages.Server{
					Type: messages.SMError,
					Text: messages.Text{
						Text: fmt.Sprintf("a user names %s is already in this channel: %s", m.User, m.Channel),
					},
				},
			)
			return nil, fmt.Errorf("subscribe erorr")
		}
	}

	mchan.users[m.User] = true
	err = c.write(
		conn,
		messages.Server{
			Type: messages.SMSubAck,
		},
	)
	if err != nil {
		glog.Errorf("problem writing subAck for user %s to chan %s: %s", m.User, m.Channel, err)
		delete(mchan.users, m.User)
		if len(mchan.users) == 0 {
			delete(c.channels, m.Channel)
		}
		return nil, fmt.Errorf("could not subscribe user %s: %s", m.User, err)
	}
	return mchan.hub, nil
}

// unsubscribe unsubscribes user "u" from channel "c".
func (c *ChatterBox) unsubscribe(u string, channel string) {
	c.chMu.Lock()
	defer c.chMu.Unlock()

	mchan, ok := c.channels[channel]
	if !ok {
		return
	}

	delete(mchan.users, u)
}

// clientReceiver is used to process messages that are received over the websocket from the client.
// This is meant to be run in a goroutine as it blocks for the life of the conn and decrements
// wg when it finally ends.
func (c *ChatterBox) clientReceiver(wg *sync.WaitGroup, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
	defer wg.Done()

	for {
		m, err := c.read(conn)
		if err != nil {
			glog.Error(err)
			return
		}
		if m.Type != messages.CMSendText {
			glog.Errorf("error: connected client for user %s on channel %s sent message of type %v after init stage", usr, chName, m.Type)
			continue
		}

		err = m.Validate()
		if err != nil {
			glog.Error(err)
			err = c.write(
				conn,
				messages.Server{
					Type: messages.SMError,
					Text: messages.Text{
						Text: err.Error(),
					},
				},
			)
			if err != nil {
				glog.Errorf("problem writing to %s: %s", chName, err)
				return
			}
			continue
		}

		//subDone := make(chan boutique.State, 1)
		if err := store.Perform(actions.SendMessage(usr, m.Text.Text)); err != nil {
			// TODO(jdoak): abstract sending error messages to the client into a method
			// and then do that here.  Then continue unless there is an error.
			glog.Infof("problem calling store.Perform(): %s", err)
			return
		}
	}
}

// clientSender receives changes to the store's Messaages field and pushes them out to
// our websocket clients.
func (c *ChatterBox) clientSender(wg *sync.WaitGroup, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
	const field = "Messages"
	defer wg.Done()

	state := store.State()
	startData := state.Data.(data.State)

	var lastMsgID = -1
	var lastVersion uint64
	if len(startData.Messages) > 0 {
		lastMsgID = startData.Messages[len(startData.Messages)-1].ID
	}

	sigCh, cancel, err := store.Subscribe(field)
	if err != nil {
		glog.Error(err)
		c.write(
			conn,
			messages.Server{
				Type: messages.SMError,
				Text: messages.Text{
					Text: err.Error(),
				},
			},
		)
		return
	}
	defer cancel()

	for sig := range sigCh {
		if sig.State.FieldVersions[field] <= lastVersion {
			continue
		}

		msgs := sig.State.Data.(data.State).Messages
		if len(msgs) == 0 { // This happens we delete the message queue at the end of this loop.
			continue
		}

		var toSend []data.Message
		toSend, lastMsgID = c.latestMsgs(msgs, lastMsgID)
		if len(toSend) > 0 {
			if err := c.sendMessages(conn, toSend); err != nil {
				glog.Errorf("error sending message to client on channel %s: %s", chName, err)
				return
			}
		}
	}
}

// latestMsgs takes the Messages in the store, locates all Messages after
// lastMsgID and then returns a slice containing those Messages and the
// new lastMsgID.
// TODO(johnsiilver): Because these messages have ascending IDs, should probably
// look at the first ID and determine where the lastMsgID is instead of looping.
func (*ChatterBox) latestMsgs(msgs []data.Message, lastMsgID int) ([]data.Message, int) {
	if len(msgs) == 0 {
		return nil, -1
	}

	var (
		toSend []data.Message
		i      int
		msg    data.Message
		found  bool
	)

	for i, msg = range msgs {
		if msg.ID == lastMsgID {
			if i == len(msgs)-1 { // If its is the last message, then there is nothing new.
				return nil, lastMsgID
			}
			found = true
			break
		}
	}

	switch found {
	case true:
		if len(msgs) == 1 {
			toSend = msgs
			lastMsgID = msgs[0].ID
		} else {
			toSend = msgs[i+1:]
			lastMsgID = toSend[len(toSend)-1].ID
		}
	default: // All the messages are new, so send them all.
		toSend = msgs
		lastMsgID = toSend[len(toSend)-1].ID
	}
	return toSend, lastMsgID
}

// sendMessages sends a list of data.Message to the client via a Websocket.
func (*ChatterBox) sendMessages(conn *websocket.Conn, msgs []data.Message) error {
	for _, ts := range msgs {
		msg := messages.Server{
			Type: messages.SMSendText,
			User: ts.User,
			Text: messages.Text{
				Text: ts.Text,
			},
		}
		if err := websocket.WriteJSON(conn, msg); err != nil {
			return err
		}
	}
	return nil
}

// read reads a Message off the websocket.
func (*ChatterBox) read(conn *websocket.Conn) (messages.Client, error) {
	m := messages.Client{}
	if err := conn.ReadJSON(&m); err != nil {
		return messages.Client{}, err
	}

	return m, nil
}

// write writes a Message to the weboscket.
func (*ChatterBox) write(conn *websocket.Conn, msg messages.Server) error {
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("problem writing msg: %s", err)
	}
	return nil
}
