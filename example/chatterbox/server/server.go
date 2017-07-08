// Package server implements a websocket server that sets up an irc like server.
package server

import (
	"context"
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
	ctx    context.Context
	cancel context.CancelFunc
	hub    *state.Hub
	users  map[string]bool
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

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go c.clientReceiver(r.Context(), wg, conn)

	wg.Wait()
}

// subscribe subscribes a user to the channel.
func (c *ChatterBox) subscribe(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, m messages.Client) (*state.Hub, error) {
	c.chMu.Lock()
	defer c.chMu.Unlock()

	var (
		hub *state.Hub
		err error
	)

	mchan, ok := c.channels[m.Channel]
	if ok {
		hub = mchan.hub
		if mchan.users[m.User] {
			c.write( // Ignore error, because its reporting an error, an error here has nothing to do.
				conn,
				messages.Server{
					Type: messages.SMError,
					Text: messages.Text{
						Text: fmt.Sprintf("a user named %s is already in this channel: %s", m.User, m.Channel),
					},
				},
			)
			return nil, fmt.Errorf("subscribe erorr")
		}
	} else {
		hub, err = state.New(m.Channel)
		if err != nil {
			return nil, err
		}
		mchan = &channel{ctx: ctx, cancel: cancel, hub: hub, users: map[string]bool{m.User: true}}
		c.channels[m.Channel] = mchan
	}

	mchan.users[m.User] = true
	if err = hub.Store.Perform(actions.AddUser(m.User)); err != nil {
		return nil, err
	}

	err = c.write(
		conn,
		messages.Server{
			Type:  messages.SMSubAck,
			Users: hub.Store.State().Data.(data.State).Users,
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
	glog.Infof("client %v: subscribed to %s as %s", conn.RemoteAddr(), m.User, m.Channel)
	return hub, nil
}

// unsubscribe unsubscribes user "u" from channel "c".
func (c *ChatterBox) unsubscribe(u string, channel string) {
	c.chMu.Lock()
	defer c.chMu.Unlock()

	mchan, ok := c.channels[channel]
	if !ok {
		return
	}

	mchan.cancel()
	delete(mchan.users, u)
	if err := mchan.hub.Store.Perform(actions.RemoveUser(u)); err != nil {
		glog.Errorf("problem removing user from Store: %s", err)
	}
}

// clientReceiver is used to process messages that are received over the websocket from the client.
// This is meant to be run in a goroutine as it blocks for the life of the conn and decrements
// wg when it finally ends.
func (c *ChatterBox) clientReceiver(ctx context.Context, wg *sync.WaitGroup, conn *websocket.Conn) {
	defer wg.Done()

	var (
		cancel context.CancelFunc
		hub    *state.Hub
		user   string
		comm   string
	)
	defer c.unsubscribe(user, comm)

	for {
		m, err := c.read(conn)
		if err != nil {
			glog.Errorf("client %s terminated its connection", conn.RemoteAddr())
			if cancel != nil {
				cancel()
			}
			return
		}

		err = m.Validate()
		if err != nil {
			glog.Errorf("error: client %v message did not validate: %v: %#+v: ignoring...", conn.RemoteAddr(), err, m)
			if err = c.sendError(conn, err); err != nil {
				return
			}
			continue
		}

		switch t := m.Type; t {
		case messages.CMSendText:
			if hub == nil {
				if err := c.sendError(conn, fmt.Errorf("cannot send a message, not subscribed to channel")); err != nil {
					glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
					return
				}
			}

			if err := hub.Store.Perform(actions.SendMessage(user, m.Text.Text)); err != nil {
				c.sendError(conn, fmt.Errorf("problem calling store.Perform(): %s", err))
				continue
			}
		case messages.CMSubscribe:
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()

			// If we already are subscribed, unsubscribe.
			if hub != nil {
				c.unsubscribe(user, comm)
				if err := c.write(conn, messages.Server{Type: messages.SMChannelDrop}); err != nil {
					glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
					return
				}
				if cancel != nil {
					cancel()
				}
			}

			// Now subscribe to the new channel.
			var err error
			hub, err = c.subscribe(ctx, cancel, conn, m)
			if err != nil {
				if err = c.sendError(conn, err); err != nil {
					glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
				}
				return
			}
			user = m.User
			comm = m.Channel

			go c.clientSender(ctx, user, comm, conn, hub.Store)
		case messages.CMDrop:
			if hub == nil {
				if err := c.sendError(conn, fmt.Errorf("error: cannot drop a channel, your not subscribed to any")); err != nil {
					glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
					return
				}
			}
			if cancel != nil {
				cancel()
			}
			c.unsubscribe(user, comm)

			if err := c.write(conn, messages.Server{Type: messages.SMChannelDrop}); err != nil {
				glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
				return
			}
			hub = nil
			user = ""
			comm = ""
		default:
			glog.Errorf("error: client %v had unknown message %v, ignoring", conn.RemoteAddr(), t)
			if err := c.sendError(conn, fmt.Errorf("received message type from client %v that the server doesn't understand", t)); err != nil {
				glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
				return
			}
		}
	}
}

// clientSender receives changes to the store's Messaages field and pushes them out to
// our websocket clients.
func (c *ChatterBox) clientSender(ctx context.Context, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
	const (
		msgField   = "Messages"
		usersField = "Users"
	)

	state := store.State()
	startData := state.Data.(data.State)

	var lastMsgID = -1
	if len(startData.Messages) > 0 {
		lastMsgID = startData.Messages[len(startData.Messages)-1].ID
	}

	msgCh, msgCancel, err := store.Subscribe(msgField)
	if err != nil {
		c.sendError(conn, err)
		return
	}
	defer msgCancel()

	usersCh, usersCancel, err := store.Subscribe(usersField)
	if err != nil {
		c.sendError(conn, err)
		return
	}
	defer usersCancel()

	for {
		select {
		case msgSig := <-msgCh:
			msgs := msgSig.State.Data.(data.State).Messages
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
		case userSig := <-usersCh:
			if err := c.write(conn, messages.Server{Type: messages.SMUserUpdate, Users: userSig.State.Data.(data.State).Users}); err != nil {
				c.sendError(conn, err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *ChatterBox) sendError(conn *websocket.Conn, err error) error {
	glog.ErrorDepth(1, err)
	wErr := c.write(
		conn,
		messages.Server{
			Type: messages.SMError,
			Text: messages.Text{
				Text: err.Error(),
			},
		},
	)
	return wErr
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
	defer conn.SetWriteDeadline(time.Time{})
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("problem writing msg: %s", err)
	}
	return nil
}
