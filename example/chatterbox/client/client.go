/*
Package client provides a ChatterBox client for communicating with a ChatterBox server.

Usage is simple:
  user := "some user"
  channel := "some channel"
  c, err := New("<host:port>", user)
  if err != nil {
    // Do something
  }

  // Must subscribe before doing anything else.
  if err := c.Subscribe("<channel name>"); err != nil {
    // Do something
  }

  stop := make(chan struct{})
  // Receive messages.
  go func() {
    for {
      select {
      case <-stop:
        fmt.Println("Exiting channel")
        return
      case m := <-c.Messages:
        // Ignore messages from yourself, we don't have separate panes to display in.
        if m.User == user {
          continue
        }
        fmt.Println("%s: %s", m.User, m.Text.Text)
      }
    }
  }()

  for {
    reader := bufio.NewReader(os.Stdin)
    fmt.Print(">")
    text, _ := reader.ReadString('\n')
    if text == "exit" {
      fmt.Printf("Exiting comm channel %s\n", channel)
      break
    }
    if err := c.SendText(text); err != nil {
      fmt.Printf("Error: %s, exiting....\n", err)
      break
    }
  }
*/
package client

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/johnsiilver/boutique/example/chatterbox/messages"
)

// ChatterBox is a client for the ChatterBox service.
// You must service all publically available channels or the client might
// freeze.
// TODO(johnsiilver): Fix that, its ridiculous.
type ChatterBox struct {
	// The websocket connection.
	conn *websocket.Conn

	mu      sync.Mutex
	channel atomic.Value
	kill    chan struct{}

	user atomic.Value // holds a string
	dead atomic.Value // holds a bool

	// Messages are text messages arriving from the server to this client.
	Messages chan messages.Server
	// ServerErrors are errors sent from the server to the client.
	ServerErrors chan error
	// UserUpdates are updates to the users who are in the comm channel.
	UserUpdates chan []string

	subscribed chan messages.Server
}

// New is the constructor for ChatterBox.
func New(addr string, username string) (*ChatterBox, error) {
	d := websocket.Dialer{
		HandshakeTimeout:  10 * time.Second,
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		EnableCompression: true,
	}
	conn, resp, err := d.Dial(fmt.Sprintf("ws://%s/", addr), nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("problem connecting to server: %s status: %s", resp.Status, err)
		}
		return nil, fmt.Errorf("problem connecting to server: %s", err)
	}

	c := &ChatterBox{
		conn:         conn,
		kill:         make(chan struct{}),
		ServerErrors: make(chan error, 10),
		Messages:     make(chan messages.Server, 10),
		UserUpdates:  make(chan []string, 10),
		subscribed:   make(chan messages.Server, 1),
	}
	c.user.Store(username)
	c.dead.Store(false)
	c.channel.Store("")

	go c.serverReceiver()
	return c, nil
}

// serverReceiver receives mdsssages from the ChatterBox server.
func (c *ChatterBox) serverReceiver() {
	for {
		var sm messages.Server

		select {
		case v := <-c.readConn():
			switch t := v.(type) {
			case error:
				glog.Errorf("client error receiving from server, client is dead: %s", t)
				return
			case messages.Server:
				sm = t
			default:
				glog.Errorf("readConn is broken")
				return
			}
		}

		switch sm.Type {
		case messages.SMError:
			glog.Infof("server sent an error back: %s", sm.Text.Text)
			c.ServerErrors <- errors.New(sm.Text.Text)
		case messages.SMSendText:
			c.Messages <- sm
		case messages.SMSubAck:
			glog.Infof("server acknowledged subscription to channel")
			c.subscribed <- sm
			c.UserUpdates <- sm.Users
		default:
			glog.Infof("dropping message of type %v, I don't understand the type", sm.Type)
		}
	}
}

// readConn reads a single entry off the websocket and returns the result on a chan.
// The result is either an error or messages.Server.
func (c *ChatterBox) readConn() chan interface{} {
	ch := make(chan interface{}, 1)

	go func() {
		sm := messages.Server{}
		if err := c.conn.ReadJSON(&sm); err != nil {
			glog.Errorf("problem reading message from server, killing the client connection: %s", err)
			c.dead.Store(true)
			ch <- err
		}
		ch <- sm
	}()
	return ch
}

// Subscribe to a new comm channel.  This must be the first method called or it will get rejected.
func (c *ChatterBox) Subscribe(name string) (users []string, err error) {
	if name == "" {
		return nil, fmt.Errorf("Subcribe(name) cannot be empty string")
	}

	if c.dead.Load().(bool) {
		return nil, fmt.Errorf("this client's connection is dead, can't subscribe")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel.Load().(string) != "" {
		return nil, fmt.Errorf("cannot subscribe to channel %s, you must unsubscribed to channel %s", name, c.channel.Load().(string))
	}

	msg := messages.Client{Type: messages.CMSubscribe, Channel: name, User: c.user.Load().(string)}
	if err := c.conn.WriteJSON(msg); err != nil {
		c.dead.Store(true)
		return nil, fmt.Errorf("connection to server is broken, this client is dead: %s", err)
	}

	select {
	case m := <-c.subscribed:
		c.channel.Store(name)
		return m.Users, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("never received subscribe acknowledge")
	}
}

// SendText sends a text message to others on our comm channel.
func (c *ChatterBox) SendText(t string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel.Load().(string) == "" {
		return fmt.Errorf("you must be subscribed to a channel before sending a message")
	}

	msg := messages.Client{
		Type:    messages.CMSendText,
		Channel: c.channel.Load().(string),
		User:    c.user.Load().(string),
		Text: messages.Text{
			Text: t,
		},
	}
	if err := msg.Validate(); err != nil {
		glog.Errorf("client message had validation error: %s", err)
		return err
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("connection to server is broken, this client is dead: %s", err)
	}

	return nil
}
