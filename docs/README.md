# Boutique

## One line summary

Boutique is an immutable data store with subscriptions to field changes.

## The long summary

Boutique provides a data store for storing immutable data.  This allows data
retrieved from the store to be used without providing synchronization even as
changes are made to data in the store by other go-routines.

In addition, Boutique allows subscriptions to be registered for changes to a
data field or any field changes.  Data is versioned, so you can compare the
version number between the data retrieved and the last data pulled.

Finally, Boutique supports middleware for any change that is being committed
to the store.  This allows for sets of features, such as a storage record of
changes to the store.

## Before we get started

### Go doesn't have immutable objects, does it?

Correct, Go doesn't have immutable objects.  It does contain immutable types,
such as strings and constants.  However, immutability in this case is simply a
contract to only change the data through the Boutique store.  All changes in
the store must copy the data before committing the changes.

We will cover how this works later.

## What is the cost of using a generic immutable store?

There are three main drawbacks for using Boutique:

* Boutique writes are slower than a non generic implementation due to type
assertion,  reflection and data copies
* In very certain circumstances, Boutique can have runtime errors due to using
interface{}
* Storage updates are done via Actions, which adds some complexity

The first, running slower is because we must not only type assert at different
points, but reflection is used to detect changes in the data fields that are in
the data being stored.  This cost can be made up for by reads of data without
synchronization and reduced complexity in the subscription model.

The second, runtime errors, happen when one of two events occur.  The type of
data to be stored in Boutique is changed on a write.  The first data passed to
the store is the only type that can be stored.  Any attempt to store a different
type of data will be result in an error.  The second way is if the data being
stored in Boutique is not a struct type.  The top level data must be a struct.  
In a non-generic store, these would be caught by the compiler.  But these are
easy to avoid.

The third is more difficult.  Changes are routed through Actions.  Actions
trigger Modifers, which also must be written.  The concepts take a bit to
understand and you have to be careful to copy the data and not mutate the data
when writing Modifiers.   This adds a certain amount of complexity, but changes
to the store are easily readable from a readability perspective.

## Best use cases?

Boutique is useful in the following scenarios:

* You want a web based application to store state on the server and not in
Javascript clients.  Keep the client as a simple renderer of data changes
sent from the server.
* Your application has lots of clients, each which need to store state and
receive updates.
* You have an application that needs to store a single state and send changes to
clients or go-routines.  However, be aware that Boutique has runtime costs and
may not be appropriate if every nanosecond counts.

## Let's get started!

### First, define what data you want to store

To start with, the data to be stored must be of type struct.  Now to be clear,
this cannot be \*struct, it must be a plain struct.  It is also important to
note that only public fields can received notification of subscriber changes.

<<<<<<< HEAD
For this example, we are going to use part of the example application included
with Boutique, a chat server called ChatterBox.  Users access the chat server,
subscribing to a comm channel.  They can then send and receive messages.
We are going to include middleware to help debug and log all conversations.

This example is not going to include all of the application's functions, just
enough to cover the basics.

So let's start by defining the data we need, which is going to be stored
in state/data/data.go

```go
// Package data holds the Store object that is used by our boutique instances.
package data

import (
	"os"
	"time"
)

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
=======
For this example, we are going to use part of the example application included with Boutique, a chat server.  Users access the chat server, subscribing to a comm channel.  They can then send and receive messages.  We are going to include middleware to help debug and log all conversations. 

This example is not going to include all of the application's functions, just enough to cover the basics.

So let's define the data we need, which is going to be stored in state/data/data.go:

```go
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
>>>>>>> origin/master
}
```

<<<<<<< HEAD
So what is going on here.  First there is the State object.  This is the
center of our Boutique universe per say.  All changes happen to this object.
Each comm channel that is opened for users to communicate on has its own
State object.  

Inside here, we have different attributes related to the state of the channel.
ServerID lets us identify the particular instance's log files, Channel holds
the name of our channel.  Users is the list of current users in the Channel,
while Messages is the current buffer of user messages waiting to be sent out
to the users.

```go
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
=======
// IsZero indicates that OpenFile has not been initialized.
func (o OpenFile) IsZero() bool {
	if o.File == nil {
		return true
	}
	return false
}

// Message represents a message sent.
type Message struct {
	// ID is the ID of the message in the order it was sent, starting at 0.
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
>>>>>>> origin/master
}

```
Message identifies a Message that was sent by a user.  The ID is a unique
identifier that starts at 0 and goes up.  Timestamp is when the message was
sent, User is who sent it, and Text is the actual message.

Now that we have our data to store in Boutique, let us talk about how to
signal a change to the store, via Actions.

### Create our actions

Changes to the data stored in Boutique is done via the Perform() method.
One of the arguments to Perform is an Action, which tells the program to
alter its state in some way.  

A boutique.Action contains two fields:

* Type, which indicates a type of action that is being committed to the store.
* Update, which can be nil or contain a type that is used in updating the store.
This may be a value that will be placed in a field, a key that will be deleted
from a map, or whatever is needed.

Its important that you understand that this simply signals a change, it does
not make a change.  You don't always need Update, because sometimes the signal
via Type is enough.  Say you had a field, Version, that needed to be
incremented.  It would simply be enough to pass an Action with type
ActVerIncr.  

But often times, you need to do more, such as change a value, merge two
structs, etc.  That is when Update is used, to pass the value.

Here I'm going to include a smaller version of the actions from our example
application, to keep it simple.

```go
// Package actions details boutique.Actions that are used by modifiers to modify the store.
package actions

import (
	"fmt"

	"github.com/johnsiilver/boutique"
)

const (
	// ActSendMessage indicates we want to send a message via the store.
	ActSendMessage = iota
	// ActAddUser indicates the Action wants to add a user to the store.
	ActAddUser
)

// SendMessage sends a message via the store.
func SendMessage(id int, user string, s string) (boutique.Action, error) {
	if len(s) > 500 {
		return boutique.Action{}, fmt.Errorf("cannot send a message of more than 500 characters")
	}
	return boutique.Action{Type: ActSendMessage, Update: data.Message{ID: id, Timestamp: time.Now(), User: user, Text: s}}, nil
}

// AddUser adds a user to the store, indicating a new user is in the room.
func AddUser(u string) boutique.Action {
	return boutique.Action{Type: ActAddUser, Update: u}
}
```

Let's talk about the constants we defined:

```go
const (
	// ActSendMessage indicates we want to send a message via the store.
	ActSendMessage = iota
	// ActAddUser indicates the Action wants to add a user to the store.
	ActAddUser
)
```
These are our Action Types that will be used for signaling.  By convention,
these should be prefixed with "Act" to indicate its an Action type.

We have defined two types here, one that Indicates the Action is trying to
send a message via the Store and one that is trying to add a user to the Store.

Now we have our Action creators:

```go
// SendMessage sends a message via the store.
func SendMessage(id int, user string, s string) (boutique.Action, error) {
	if len(s) > 500 {
		return boutique.Action{}, fmt.Errorf("cannot send a message of more than 500 characters")
	}
	return boutique.Action{Type: ActSendMessage, Update: data.Message{ID: id, Timestamp: time.Now(), User: user, Text: s}}, nil
}

// AddUser adds a user to the store, indicating a new user is in the room.
func AddUser(u string) boutique.Action {
	return boutique.Action{Type: ActAddUser, Update: u}
}
```

SendMessage takes in the ID of a message, the user sending it, and the text
message itself.  It validates the text message is the right size, or returns
an error (we could have done this validation via Middleware as well).  It then
creates a boutique.Action setting the type to ActSendMessage and the Update
to the Message type, which we will use to update our store.

Now, we have Actions that describe the changes we want to do, but how do we
make those changes?  

### Writing Modifiers

Modifiers interpret Actions and handle updating the data in the store.  All
Modifiers must conform to the following signature which is defined by
boutique.Modifier:

```go
type Modifier func(state interface{}, action Action) interface{}
```

The "state" is the data object that will get updated.  In our case,
this would be data.State that we defined in package data.  "action" is the
boutique.Action that is to be processed.  An Modifier does NOT have to handle an
Action, it only has to handle the Actions for which it recognizes.  If it
does not recognize the action.Type, it should simply return state as it was
passed.  Otherwise Modifier returns the updated state object.

There is a fundamental rule that MUST be obeyed by all Modifiers:

THOU SHALL NOT MUTATE DATA!

Non-reference or pointer values can be changed directly.  But reference types
or pointer values must be copied and replaced, but never modified.  
This allows downstream readers to ignore locking.

So if you want to add a value to a slice, you must copy the slice, add the
new value, then change the reference in the Store.  You must never directly
append.  This is relatively fast on modern processors when data fits in the
cache.

Here are some Modifiers to handle our Actions.  We could write one Modifier to
handle all Actions or multiple Modifiers handling each individual Actions.
I've chosen the latter, as I find it more readable.

```go
// Modifiers is a boutique.Modifiers made up of all Modifier(s) in this file.
var Modifier = boutique.NewModifiers(SendMessage, AddUser)

// SendMessage handles an Action of type ActSendMessage.
func SendMessage(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActSendMessage:
		s.Messages = boutique.CopyAppendSlice(s.Messages, actions.Update).([]data.Message)
	}
	return s
}

// AddUser handles an Action of type ActAddUser.
func AddUser(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActAddUser:
		s.Users = boutique.CopyAppendSlice(s.Users, action.Update).([]string)
	}
	return s
}
```

All Modifier(s) follow the boutique.Modifier signature.  They receive a State
and an Action.  

SendMessage receives immediately type asserts the state from an interface{} into
our concrete state.  This is always safe, because boutique.Store's are always
initialized with a starting state object.

```go
s := state.(data.State)
```

We then switch on the action.Type.  We are only interested if the
action.Type == actions.SendMessage.  Otherwise, we do nothing and just return
an unmodified state object.

```go
switch action.Type {
case actions.ActSendMessage:
  ...
return s
```

If we received an action.Type == actions.SendMessage, we now need to type
assert our action.Update so that we can retrieve the data we want to
modify data.Messages with.

```go
up := action.Update.(actions.Message)
s.Messages = boutique.CopyAppendSlice(s.Messages, data.Message{ID: up.ID, Timestamp: time.Now(), User: up.User, Text: up.Text}).([]data.Message)
```

Now we need to update our Messages slice to contain our new Messages.
Remember we cannot append to our existing Messages object, as that would be
updating a reference.  However, we can use a handy boutique.CopyAppendSlice()
method to simply make a copy of our slice and update it with our new message.

```go
boutique.CopyAppendSlice(s.Messages, data.Message{ID: up.ID, Timestamp: time.Now(), User: up.User, Text: up.Text})
```
This handles the copy and append, but it returns an interface{}, so you must
remember to type assert the result:

```go
.([]data.Message)
```

AddUser works in the same way:

```go
// AddUser handles an Action of type ActAddUser.
func AddUser(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActAddUser:
		s.Users = boutique.CopyAppendSlice(s.Users, action.Update).([]string)
	}
	return s
}
```

Now lets talk about Modifiers.

```go
var Modifiers = boutique.NewModifiers(SendMessage, AddUser)
```

Every update to boutique.Store is done through .Perform().  When .Perform()
is called, it runs all of your Modifier(s) in the order you choose.  These
Modifier(s) are registered to boutique.Store via the New() call.  A Modifiers is
a collection of Modifier(s) in the order they will be applied.

### Creating your boutique.Store

Generally speaking, it is a good idea to wrap your boutique.Store inside
another object, though this isn't required.  But either way, it is always a
good idea to have a constructor to handle the initial setup.

By convention, state.go is where you would want to do this.  

```go
// Package state contains our Hub, which is used to store data for a particular
// channel users are communicating on.
package state

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/data"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/middleware"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/updaters"
)

// Hub contains the central store and our middleware.
type Hub struct {
	// Store is our boutique.Store.
	Store *boutique.Store
}

// New is the constructor for Hub.
func New(channelName string, serverID string) (*Hub, error) {
	d := data.State{
		ServerID: serverID,
		Channel:  channelName,
		Users:    []string{},
		Messages: []data.Message{},
	}

	s, err := boutique.New(d, updaters.Modifier, nil)
	if err != nil {
		return nil, err
	}

	return &Hub{Store: s}, nil
}
```

Here I've create a constructor that sets up our boutique.Store.  New()
creates our initial data object, data.State, giving it a unique serverID
(I like pborman's UUID library for generating unique IDs), the name of the
channel we are storing state for, our initial Users and Messages.

Then the store is initiated containing our starting data, our Modifiers, and
no Middleware (we will come back to this).

Alright, let's see how we can use this.

### Using the Store

The example application has some complex logic, mostly around dealing with
data coming in on a websocket and pushing the data out.  We will skip around
that and just talk about using the Stores.  So you might not see a 1:1
correlation with the code.

Our application will receive websocket connections.  The first thing we expect
to happen is to receive a request to subscribe to a channel.  If we do not,
that connection is rejected.

If their subscription request contains comm channel name that doesn't exist,
we then create one:

```go
func (c *ChatterBox) subscribe(conn *websocket.Conn, m messages.Client) (*state.Hub, error) {
  c.chMu.Lock()
  defer c.chMu.Unlock()

  // Create our new boutique.Store.
  hub, err := state.New(m.Channel, c.serverID)
  if err != nil {
    return nil, err
  }

  // Let's go ahead and register this user.
  if err := hub.Perform(actions.AddUser(m.User)) {
    // Send an error back on the websocket and return.
  }

  // Record in our application the new channel we created.
  // The lock above is protecting this map. Go 1.8 should have a sync.Map type.
  c.channels[channelName] = &channel{hub: hub}
  ...
  go clientSender(wg, user, m.Channel, conn, hub.store) // Discussed below.
  go clientReceiver() // Discussed below.
```

At this point, nothing special has happened.  You've spent a lot more time
updating a struct, which is not all that useful.

But now is some payoff.  Every time a user on this channel submits a message,
you want to update the store and have all users updated with that message.
So for the person who just created the comm channel, lets send him updates
whenever anyone sends on the channel.

#### Updating clients when new messages arrive

```go
func (c *ChatterBox) clientSender(wg *sync.WaitGroup, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
  ...

  // Subscribe to changes to the "Messages" field in our Store.
  sigCh, cancel, err := store.Subscribe("Messages")
  if err != nil {
    // Send the error back on the websocket and close
    ...
  }
  defer cancel() // Stop our subscription.

  for sig := range sigCh {
		msgs := sig.State.Data.(data.State).Messages
		if len(msgs) == 0 {
			continue
		}

    // Send any messages that have appeared in the store since we last sent.
    // Note: This would get ugly if we didn't delete messages after they were
    // sent, which the application does, but we are not showing here.
		toSend := []data.Message{}

		if len(msgs) > 1 {
      var (
			  i   int
			  msg data.Message
        found bool
      )
      // Find the last message we sent.
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
		} else {
			toSend = msgs
			lastMsgID = toSend[0].ID
		}

    // Send our message to the client via the websocket.
    ...
```

The first thing that happens if we subscribe to the Store's Messages field.

```go
sigCh, cancel, err := store.Subscribe("Messages")
```

Here we get back a channel which contains the data.Store that was committed.
Because we are an immutable data store, it is safe to use this object without
locking (unless you a mutating, which would be bad).  

cancel() is key, as it tells us to stop receiving updates after we are
finish listening.

We then begin looping over the channel.

```go
for sig := range sigCh {
  ...
}
```
Inside here we need to gather up all messages that have been added since the
last time we looped and then send them to our client via the websocket.Conn.

#### Update the Store.Messages when a client sends a new Message

```go
func (c *ChatterBox) clientReceiver(wg *sync.WaitGroup, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
  for {
    // Get a client message from the websocket.Conn
    ...

    a, err := actions.SendMessage(usr, m.Text.Text)
		if err != nil {
      // Send the error back to the websocket client.
      ...
      continue
    }

    if err := store.Perform(a); err != nil {
      // Send the error back to the websocket client.
    }
  }
```

Now we simply read messages off the websocket.Conn object, update our Store
with an actions.SendMessage(), and all of our clients are magically updated!

## Previous works

Boutique is based on the Redux library: [http://redux.js.org](http://redux.js.org)
