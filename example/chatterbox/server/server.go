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
	id := 0
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
			err := c.write(
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

		a, err := actions.SendMessage(id, usr, m.Text.Text)
		if err != nil {
			glog.Errorf("error sending message to client: %s", err)
			err := c.write(
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

		subDone := make(chan boutique.State, 1)
		store.Perform(a, boutique.WaitForSubscribers(subDone))
		id++
		go func() {
			_ = <-subDone
			store.Perform(actions.DeleteMessages(id-1), boutique.NoUpdate())
		}()
	}
}

// clientSender receives changes to the store's Messaages field and pushes them out to
// our websocket clients.
func (c *ChatterBox) clientSender(wg *sync.WaitGroup, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
	defer wg.Done()

	startState := store.State().Data.(data.State)
	var lastMsgID = -1
	if len(startState.Messages) > 0 {
		lastMsgID = startState.Messages[len(startState.Messages)-1].ID
	}

	sigCh, cancel, err := store.Subscribe("Messages")
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
		msgs := sig.State.Data.(data.State).Messages
		if len(msgs) == 0 { // This happens we delete the message queue at the end of this loop.
			continue
		}

		var toSend []data.Message
		toSend, lastMsgID = c.sendThis(msgs, lastMsgID)

		for _, ts := range toSend {
			msg := messages.Server{
				Type: messages.SMSendText,
				User: ts.User,
				Text: messages.Text{
					Text: ts.Text,
				},
			}
			if err := websocket.WriteJSON(conn, msg); err != nil {
				glog.Error(err)
				return
			}
		}
	}
}

// sendThis takes the Messages in the store, locates all Messages after
// lastMsgID and then returns a slice containing those Messages and the
// new lastMsgID.
// TODO(johnsiilver): Because these messages have ascending IDs, should probably
// look at the first ID and determine where the lastMsgID is instead of looping.
func (*ChatterBox) sendThis(msgs []data.Message, lastMsgID int) ([]data.Message, int) {
	toSend := []data.Message{}

	if len(msgs) > 1 {
		var (
			i     int
			msg   data.Message
			found bool
		)
		for i, msg = range msgs {
			if msg.ID == lastMsgID {
				found = true
				break
			}
		}
		if found {
			toSend = msgs[i+1:]
			lastMsgID = toSend[len(toSend)-1].ID
		} else {
			toSend = msgs
			lastMsgID = toSend[0].ID
		}
	} else { // FYI, we always have at least one Message.
		toSend = msgs
		lastMsgID = toSend[0].ID
	}
	return toSend, lastMsgID
}

// read reads a Message off the websocket.
func (*ChatterBox) read(conn *websocket.Conn) (messages.Client, error) {
	messageType, p, err := conn.ReadMessage()
	if err != nil {
		return messages.Client{}, err
	}

	m := messages.Client{}

	if messageType != websocket.TextMessage {
		return m, fmt.Errorf("someone is sending non-binary messages")
	}

	if err := m.Unmarshal(p); err != nil {
		return m, fmt.Errorf("init message could not be unmarshalled: %s", err)
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
