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

	user     atomic.Value // holds a string
	dead     atomic.Value // holds a bool
	deadOnce sync.Once

	// Messages are text messages arriving from the server to this client.
	Messages chan messages.Server
	// ServerErrors are errors sent from the server to the client.
	ServerErrors chan error
	// UserUpdates are updates to the users who are in the comm channel.
	UserUpdates chan []string
	// ChannelDrop are updates saying we've been unsubscribed to a comm channel.
	ChannelDrop chan struct{}
	// Subscribed is an update saying we've been subscribed to a comm channel.
	Subscribed chan messages.Server
	// Done indicates that the server connection is dead, this client is Done.
	Done chan struct{}
}

// New is the constructor for ChatterBox.
func New(addr string) (*ChatterBox, error) {
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
		ChannelDrop:  make(chan struct{}, 1),
		Subscribed:   make(chan messages.Server, 1),
		Done:         make(chan struct{}),
	}
	c.dead.Store(false)
	c.channel.Store("")

	go c.serverReceiver()
	return c, nil
}

// serverReceiver receives messages from the ChatterBox server.
func (c *ChatterBox) serverReceiver() {
	defer c.makeDead()

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
			c.Subscribed <- sm
			c.UserUpdates <- sm.Users
		case messages.SMUserUpdate:
			c.UserUpdates <- sm.Users
		case messages.SMChannelDrop:
			c.ChannelDrop <- struct{}{}
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
			ch <- err
		}
		ch <- sm
	}()
	return ch
}

func (c *ChatterBox) makeDead() {
	c.deadOnce.Do(func() {
		c.dead.Store(true)
		close(c.Done)
	})
}

// Subscribe to a new comm channel.
func (c *ChatterBox) Subscribe(comm, user string) (users []string, err error) {
	if comm == "" {
		return nil, fmt.Errorf("cannot subscribe to an unnamed comm channel")
	}

	if user == "" {
		return nil, fmt.Errorf("cannot subscribe with an empty user name")
	}

	if c.dead.Load().(bool) {
		return nil, fmt.Errorf("this client's connection is dead, can't subscribe")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel.Load().(string) != "" {
		return nil, fmt.Errorf("cannot subscribe to channel %s, you must unsubscribe to channel %s", comm, c.channel.Load().(string))
	}

	msg := messages.Client{Type: messages.CMSubscribe, Channel: comm, User: user}
	if err := c.conn.WriteJSON(msg); err != nil {
		c.makeDead()
		return nil, fmt.Errorf("connection to server is broken, this client is dead: %s", err)
	}

	select {
	case m := <-c.Subscribed:
		c.user.Store(user)
		c.channel.Store(comm)
		return m.Users, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("never received subscribe acknowledge")
	}
}

// Drop disconnects from a comm channel channel.
func (c *ChatterBox) Drop() error {
	if c.dead.Load().(bool) {
		return fmt.Errorf("this client's connection is dead, can't drop from a channel")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	comm := c.channel.Load().(string)
	user := c.user.Load().(string)
	if c.channel.Load().(string) == "" {
		return nil
	}

	msg := messages.Client{Type: messages.CMDrop, Channel: comm, User: user}
	if err := c.conn.WriteJSON(msg); err != nil {
		c.makeDead()
		return fmt.Errorf("connection to server is broken, this client is dead: %s", err)
	}

	select {
	case _ = <-c.ChannelDrop:
		c.user.Store("")
		c.channel.Store("")
		return nil
	case <-time.After(5 * time.Second):
		c.makeDead()
		return fmt.Errorf("never received drop acknowledge")
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
