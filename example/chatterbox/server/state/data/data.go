// Package data holds the Store object that is used by our boutique instances.
package data

import (
	"os"
	"time"
)

// OpenFile contains access to a file and the last time we accessed it.
type OpenFile struct {
	*os.File

	// LastAccess is the last time the file was accessed.
	LastAccess time.Time
}

// IsZero indicates that OpenFile has not been initialized.
func (o OpenFile) IsZero() bool {
	if o.File == nil {
		return true
	}
	return false
}

// Message represents a message sent.
type Message struct {
	// ID is the ID of the message in the order it was sent.
	ID int
	// Timestamp is the time in which the message was written.
	Timestamp time.Time
	// User is the user who sent the message.
	User string
	// Text is the text of the message.
	Text string
}

// State holds our state data for each communication channel that is open.
type State struct {
	// ServerID is a UUID that uniquely represents this server instance.
	ServerID string

	// Channel is the channel this represents.
	Channel string
	// Users are the users in the Channel.
	Users []string
	// Messages in the current messages.
	Messages []Message

	// LogDebug indicates to start logging debug information.
	// LogChan indicates to log chat messages.
	LogDebug, LogChan bool
	// DebugFile holds access to the a debug file we opened for this channel.
	DebugFile OpenFile
	// ChanFile holds access to the chat log for the channel.
	ChanFile OpenFile
}
