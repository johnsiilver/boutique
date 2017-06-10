package server

import (
	"net/http"
	"sync"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/johnsiilver/golib/boutique"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type channel struct {
  state *boutique.Container
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
	return &ChatterBox{}
}

func (c *ChatterBox) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		glog.Errorf("error connecting to server: %s", err)
		return
	}

  m, err := c.read(conn)
  if err != nil {
    glog.Errorf(err)
    return
  }

  if m.MessgeType != Subscribe {
    glog.Errorf("first message on a websocket must be of type Subscribe")
    return
  }

  if err := m.Validate(); err != nil {
    glog.Error(err)
    return
  }

  state, err := c.subscribe(m)
  if err != nil {
    return
  }
  defer c.unsubscribe(m.User, m.Channel)

  closer := make(chan struct{})
  wg := &sync.WaitGroup{}

  go c.clientReceiver(wg, m.User, m.Channel, conn, state)
  go c.clientSender(wg, m.User, m.Channel, conn, state)

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err = conn.WriteMessage(messageType, p); err != nil {
			return err
		}
	}
}

// subscribe subscribe a user to the channel.
func (c *ChatterBox) subscribe(m messages.Client) (*boutique.Container, error) {
  chMu.Lock()
  defer chMu.Unlock()

  mchan, ok := c.channels[m.Channel]
  if !ok {
    mchan = &channel{state: state.New(), users: map[string]bool{m.User: true}}
    c.channels[m.Channel] = mchan
  }else{
    if mchan.users[m.User] {
      c.write(conn, messages.Server{Type: ServerError, Text: message.Text{Text: fmt.Sprintf("a user names %s is already in this channel: %s", m.User)}})
      return nil, fmt.Errorf("subscribe erorr")
    }
    mchan.users[m.User] = true
  }
  return mchan.state, nil
}

// unsubscribe unsubscribes user "u" from channel "c".
func (c *ChatterBox) unsubscribe(u string, c string) {
  chMu.Lock()
  defer chMu.Unlock()

  mchan, ok := c.channels[c]
  if !ok {
    return
  }

  delete(mchan.users, u)
}

// clientReceiver is used to process messages that are received over the websocket from the client.
// This is meant to be run in a goroutine as it blocks for the life of the conn and decrements
// wg when it finally ends.
func (c *ChatterBox) clientReceiver(wg *sync.WaitGroup, usr string, chName string, conn *websocket.Conn, state *boutique.Container) {
  defer wg.Done()
  for {
      m, err := c.read(conn)
      if err != nil {
        glog.Error(err)
        return
      }
      if m.Type != m.SendText {
        glog.Error("connected client for user %s on channel %s sent message of type %v after init stage", usr, chName, m.Type)
        continue
      }

      if err := m.Validate(); err != nil {
        glog.Error(err)
        c.write(conn, messages.Server{Type: SMError, Error: err.Error()})
        continue
      }

      state.Perform(actions.SendMessage(m.Text.Text), nil)
  }
}

func (c *ChatterBox) clientSender(wg *sync.WaitGroup, usr string, chName string, conn *websocket, store *boutique.Container) {
  defer wg.Done()
  version := store.State().(state.Data).Version
  sigCh, cancel, err := state.Subscribe("messages")
  if err != nil {
    glog.Error(err)
    c.write(conn, messages.Server{Type: SMError, Error: err.Error()})
    return
  }

  for {
    sig := <-sigCh
    version = sig.Version
    data := store.State().(state.Data)


  }
}

// read reads a Message off the websocket.
func (ChatterBox) read(conn *websocket.Conn) (Message, error) {
  messageType, p, err := conn.ReadMessage()
  if err != nil {
    return
  }

  m := Message{}

  if messageType != websocket.BinaryMessage {
    return m, fmt.Errorf("someone is sending non-binary messages")
  }

  if err := m.Unmarshal(p); err != nil {
    return m, fmt.Errorf("init message could not be unmarshalled: %s", err)
  }
  return m, nil
}
