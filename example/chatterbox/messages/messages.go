// Package messages holds the client/erver messages that are sent on the write in JSON format.
package messages

import (
	"encoding/json"
	"fmt"
)

// ClientMsgType is the type of message being sent from a client.
type ClientMsgType int

const (
	// CMUnknown indicates that the message type is unknown.  This means the code did not set
	// the MessgeType.
	CMUnknown ClientMsgType = 0
	// CMSubscribe indicates that the user is trying to subscribe to a channel. This should be the
	// first message we see in a connection.
	CMSubscribe ClientMsgType = 1
	// CMDrop indicates they wish to stop communicating on a channel and we should remove them.
	CMDrop ClientMsgType = 2
	// CMSend indicates they are sending a text to everyone.
	CMSendText ClientMsgType = 3
)

// Clientrepresents a message from the client.
type Client struct {
	// Type is the type of message.
	Type ClientMsgType

	// User is the user's name.
	User string

	// Channel is the channel they wish to connect to. If it doesn't exist it will be created.
	Channel string

	// Text is textual data that will be used if the Type == CMSendText.
	Text Text
}

// Validate validates that the messsage is valid.
func (m Client) Validate() error {
	if m.Type == CMUnknown {
		return fmt.Errorf("client did not set the message type")
	}

	if m.User == "" {
		return fmt.Errorf("client did not set user")
	}

	if m.Channel == "" {
		return fmt.Errorf("client did not send channel")
	}

	switch m.Type {
	case CMSendText:
		if err := m.Text.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Marshal turns our message into JSON.
func (m Client) Marshal() []byte {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err) // This should never happen.
	}
	return b
}

// Unmarshal takes a binary version of a message and turns it into a struct.
func (m *Client) Unmarshal(b []byte) error {
	return json.Unmarshal(b, m)
}

// Text contains textual information that is being sent.
type Text struct {
	// Text is the text the user is sending.
	Text string
}

// Validate validates the TextMessage.
func (t Text) Validate() error {
	if len(t.Text) > 500 {
		return fmt.Errorf("cannot send more than 500 characters in a single TextMessage")
	}
	return nil
}

// ServerMsgType indicates the type of message being sent from the server.
type ServerMsgType int

const (
	// SMUnknown indicates that the message type is unknown.
	SMUnknown ServerMsgType = 0
	// SMError indicates that the server had some type of error.
	SMError ServerMsgType = 1
	// SMSendText is text intended for the client.
	SMSendText = 2
)

type Server struct {
	// Type is the type of message we are sending.
	Type ServerMsgType

	// Text is the text of the message if Type == SMSendText.
	Text Text
}

// Validate validates that the messsage is valid.
func (m Server) Validate() error {
	switch m.Type {
	case SMUnknown:
		return fmt.Errorf("client did not set the message type")
	case SMSendText, SMError:
		if err := m.Text.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Marshal turns our message into JSON.
func (m Server) Marshal() []byte {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err) // This should never happen.
	}
	return b
}

// Unmarshal takes a binary version of a message and turns it into a struct.
func (m *Server) Unmarshal(b []byte) error {
	return json.Unmarshal(b, m)
}
