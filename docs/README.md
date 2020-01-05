# Boutique

## One line summary

Boutique is an immutable state store with subscriptions to field changes.

## The long summary

Boutique is an experiment in versioned, generic state storage for Go.

It provides a state store for storing immutable data.  This allows data
retrieved from the store to be used without synchronization.

In addition, Boutique allows subscriptions to be registered for changes to a
data field or any field changes.  Data is versioned, so you can compare the
version number between the data retrieved and the last data pulled.

Finally, Boutique supports middleware for any change that is being committed
to the store.  This allows for features like debugging, long term storage,
authorization checks, ... to be created.

## Best use cases?

Boutique is useful for:

* A web based application that stores state on the server and not in
Javascript clients. I like to use it instead of Redux.
* An application that has lots of clients, each which need to store state and
receive updates.
* An application that has listeners sharing a single state with updates pushed
to all listeners.

## Before we get started

### Go doesn't have immutable objects, does it?

Correct, Go doesn't have immutable objects.  It does contain immutable types,
such as strings and constants.  However, immutability in this case is simply a
contract to only change the data through the Boutique store.  All changes in
the store must copy the data before committing the changes.

On Unix based systems, it is possible to test your code to ensure no mutations.
See: http://godoc.org/github.com/lukechampine/freeze

I have seen no way to do this for Windows.

## What is the cost of using a generic immutable store?

There are three main drawbacks for using Boutique:

* Boutique writes are slower than a non generic implementation due to type
assertion, reflection and data copies
* In very specific circumstances, Boutique can have runtime errors due to using
interface{}
* Storage updates are done via **Actions**, which adds some complexity

The first, running slower is because we must not only type assert at different
points, but reflection is used to detect changes in the data fields of the
stored data.  We also need to copy data out of maps, slices, etc...  into new
maps, slices, etc... This cost is lessened by reads of data without
synchronization and reduced complexity in the subscription model.

The second, runtime errors, happen when one of two events occur.  The type of
data to be stored in Boutique is changed on a write.  The first data type
passed to the store is the only type that can be stored.  Any attempt to store
a different type of data will result in an error.  The second way is if the
data being stored in Boutique is not a struct type.  The top level data must be
a struct.  In a non-generic store, these would be caught by the compiler.  But
these are generally non-issues.

The third is more difficult.  Changes are routed through **Actions**.
**Actions** trigger **Modifers**, which also must be written.  The concepts
take a bit to understand and you have to be careful not to mutate the data when
writing **Modifier(s)**.   This adds a certain amount of complexity. But once
you get used to the method, the code is easy to follow.

## Where are some example applications?

You can find several example applications of varying sophistication here:

IRC like chat server/client using websockets with a sample terminal UI.
Welcome back to the 70's:

[http://github.com/johnsiilver/boutique/example/chatterbox](http://github.com/johnsiilver/boutique/blob/master/example/chatterbox/chatterbox.go)

Stock buy/sell point notifier using desktop notifications:

[http://github.com/johnsiilver/boutique/example/notifier](http://github.com/johnsiilver/boutique/blob/master/example/notifier/notifier.go)

## What does using Boutique look like?

Forgetting all the setup, usage looks like this:
```go
// Create a boutique.Store which holds the State object (not defined here)
// with a Modifier function called AddUser (for changing field State.User).
store, err := boutique.New(State{}, boutique.NewModifiers(AddUser), nil)
if err != nil {
	// Handle the error.
}

// Create a subscription to changes in the "Users" field.
userNotify, cancel, err := store.Subscribe("Users")
if err != nil {
	// Handle the error.
}
defer cancel()  // Cancel our subscription when the function closes.

// Print out the latest user list whenever State.Users changes.
go func(){
	for signal := range userNotify {
		fmt.Println("current users:")
		for _, user := range signal.State.Data(State).Users {
			fmt.Printf("\t%s\n", user)
		}
	}
}()

// Change field .Users to contain "Mary".
if err := store.Perform(AddUser("Mary")); err != nil {
	// Handle the error.
}

// Change field .Users to contain "Joe".
if err := store.Perform(AddUser("Joe")); err != nil {
	// Handle the error.
}

// We can also just grab the state at any time.
s := store.State()
fmt.Println(s.Version)                // The current version of the Store.
fmt.Println(s.FieldVersions["Users"]) // The version of the .Users field.
fmt.Println(s.Data.(State).Users)     // The .Users field.

```

Key things to note here:

* The State object retrieved from the signal requires no locks.
* Perform() calls do not require locks.
* Everything is versioned.
* Subscribers only receive the **latest** update, not every update.
This cuts down on unnecessary processing (it is possible, with Middleware
to get every update).
* This is just scratching the surface with what you can do, especially with
Middleware.

## Start simply: the basics

[http://github.com/johnsiilver/boutique/example/basic](http://github.com/johnsiilver/boutique/blob/master/example/basic/basic.go)

This application simply spins up a bunch of goroutines and we use a
boutique.Store to track the number of goroutines running.

In itself, not practical, but it will help define our concepts.

### First, define what data you want to store

To start with, the data to be stored must be of type struct.  Now to be clear,
this cannot be a pointer to struct (\*struct), it must be a plain struct.  It
is also important to note that only public fields can received notification of
subscriber changes.

Here's the state we want to store:
```go
// State is our state data for Boutique.
type State struct {
	// Goroutines is how many goroutines we are running.
	Goroutines int
}
```

### Now, we need to define Actions for making changes to the State

```go
// These are our ActionTypes.  This informs us of what kind of change we want
// to do with an Action.
const (
	// ActIncr indicates we are incrementing the Goroutines field.
	ActIncr boutique.ActionType = iota

	// ActDecr indicates we are decrementing the Gorroutines field.
	ActDecr
)

// IncrGoroutines creates an ActIncr boutique.Action.
func IncrGoroutines(n int) boutique.Action {
	return boutique.Action{Type: ActIncr, Update: n}
}

// DecrGoroutines creates and ActDecr boutique.Action.
func DecrGoroutines() boutique.Action {
	return boutique.Action{Type: ActDecr}
}
```
Here we have two Action creator functions:
* IncrGoroutines which is used to increment the Goroutines count by n
* DecrGoroutines which is used to decrement the Goroutines count by 1

**boutique.Action** contains two fields:
* Type - Indicates the type of change that is to be made
* Update - a blank interface{} where you can store whatever information
	is needed for the change.  In the case of an ActIncr change, it is the
	number of Goroutines we are adding.  It can also be nil, as sometimes you only
	need the Type to make the change.

### Define our Modifiers, which are what implement a change to the State

```go
// HandleIncrDecr is a boutique.Modifier for handling ActIncr and ActDecr boutique.Actions.
func HandleIncrDecr(state interface{}, action boutique.Action) interface{} {
	s := state.(State)

	switch action.Type {
	case ActIncr:
		s.Goroutines = s.Goroutines + action.Update.(int)
	case ActDecr:
		s.Goroutines = s.Goroutines - 1
	}

	return s
}
```
We only have a single Modifier which handles Actions of type **ActIncr** and
**ActDecr**.  We could have made two Modifier(s), but opted for a single one.

A modifier has to implement the following signature:
```go
type Modifier func(state interface{}, action Action) interface{}
```

So let's talk about what is going on.  A **Modifier** is called when a change is
being made to the boutique.Store.  It is passed a copy of the data that is
stored.  We need to modify that data if the action that is passed is one that
our Modifier is designed for.  

First, we transform the **copy** of our State object into its concrete
state (instead of interface{}).  

Now we check to see if this is an **action.Type** we handle.  If not, we simply
skip doing anything (which will return the state data as it was before the
**Modifier** was called).

If it was an **ActIncr Action**, we increment **.Goroutines** by
**action.Update**, which will be of type **int**.

If it was an **ActDecr Action**, we decrement **.Goroutines** by 1.

### Let's create a subscriber to print out the current .Gouroutines number
```go
func printer(killMe, done chan struct{}, store *boutique.Store) {
	defer close(done)
	defer store.Perform(DecrGoroutines())

	// Subscribe to the .Goroutines field changes.
	ch, cancel, err := store.Subscribe("Goroutines")
	if err != nil {
		panic(err)
	}
	defer cancel() // Cancel our subscription when this goroutine ends.

	for {
		select {
		case sig := <-ch: // This is the latest change to the .Goroutines field.
			fmt.Println(sig.State.Data.(State).Goroutines)
			// Put a 1 second pause in.  Remember, we won't receive 1000 increment
			// signals and 1000 decrement signals.  We will always receive the
			// latest data, which may be far less than 2000.
			time.Sleep(1 * time.Second)
		case <-killMe: // We were told to die.
			return
		}
	}
}
```
The **close()** lets others know when this printer dies.

The **store.Perform(DecrGoroutines())** reduces our goroutine count by 1 when
the printer ends.

```go
// Subscribe to the .Goroutines field changes.
ch, cancel, err := store.Subscribe("Goroutines")
if err != nil {
	panic(err)
}
defer cancel() // Cancel our subscription when this goroutine ends.
```
Here we subscribe to the **.Goroutines** field.  Whenever an update happens
to this field, we will get notified on channel **ch**.  

However, we will only get the **latest** update, not every update.
This is important to remember.

**cancel()** cancels the subscription when the printer ends.

```go
for {
	select {
	case sig := <-ch: // This is the latest change to the .Goroutines field.
		fmt.Println(sig.State.Data.(State).Goroutines)
		// Put a 1 second pause in.  Remember, we won't receive 1000 increment
		// signals and 1000 decrement signals.  We will always receive the
		// latest data, which may be far less than 2000.
		time.Sleep(1 * time.Second)
	case <-killPrinter: // We were told to die.
		return
	}
}
```
Finally we loop and listen for one of two things to happen:

* We get a **boutique.Signal** that **.Goroutines** has changed and
print the value.
* We have been signaled to die, so we kill the printer goroutine by returning.

### Let's create our main()

```go
func main() {
	// Create our new Store with our default State{} object and our only
	// Modifier.  We are not going to define Middleware, so we pass nil.
	store, err := boutique.New(State{}, boutique.NewModifiers(HandleIncrDecr), nil)
	if err != nil {
		panic(err)
	}
```

Now we start using what we've created.  Here you can see we create the
new boutique.Store instance.  We pass it the initial state, **State{}** and
we give it a collection of **Modifier(s)**.  In our case we only have one
**Modifier**, so we pass **HandleIncrDecr**.  The final **nil** simply indicates
that we are not passing any Middleware (we talk about it later).

```go
// killPrinter lets us signal our printer goroutine that we no longer need
// its services.
killPrinter := make(chan struct{})
// printerKilled informs us that the printer goroutine has exited.
printerKilled := make(chan struct{})

go printer()
```
Now we create a couple of channels to signal that we want **printer()** to die
and another let us know **printer()** has died.  We then kick off our
**printer()**.

```go
wg := sync.WaitGroup{}

// Spin up a 1000 goroutines that sleep between 0 and 5 seconds.
// Pause after generating every 100 for 0 - 8 seconds.
for i := 0; i < 1000; i++ {
	if i%100 == 0 {
		time.Sleep(time.Duration(rand.Intn(8)) * time.Second)
	}
	store.Perform(IncrGoroutines(1))
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer store.Perform(DecrGoroutines())
		time.Sleep(time.Duration(rand.Intn(5)) * time.Second)
	}()
}

wg.Wait()          // Wait for the goroutines to finish.
close(killPrinter) // kill the printer.
<-printerKilled    // wait for the printer to die.

fmt.Printf("Final goroutine count: %d\n", store.State().Data.(State).Goroutines)
fmt.Printf("Final Boutique.Store version: %d\n", store.Version())
```

Now into the final stretch.  We kick off 1000 goroutines, pausing for 8 seconds
after spinning off every 100.  Every time we spin off a goroutine, we
increment our goroutine count with a **Perform()** call and increment a
**sync.WaitGroup** so we know when all goroutines are done.  

We then wait for all goroutines to finish, kill the **printer()** and print out
some values from the Store.


## Let's try a more complex example!

###  A little about file layout

Boutique provides storage that is best designed in a modular method:
```
  └── state
    ├── state.go
    ├── actions
    │   └── actions.go
    ├── data
    |   └── data.go
    ├── middleware
    |   └── middleware.go
    └── modifiers
        └── modifiers.go
```

The files are best organized by using them as follows:

* state.go - Holds the constructor for a boutique.Store for your application
* actions.go - Holds the actions that will be used by the **Modifiers** to
  update the store
* data.go - Holds the definition of your state object
* middleware.go = Holds middleware for acting on proposed changes to your data.
  This is not required
* modifiers.go - Holds all the Modifier(s) that are used by the boutique.Store
  to modify the store's data

**Note**: These are all simply suggestions, you can combine this in a
single file or name the files whatever you wish.

### First, define what data you want to store

For this example we are going to use the example application ChatterBox included
with Boutique.  This provides an IRC like service using websockets.  Users
can access the chat server and subscribe to a channel where they can:

 * Send and receive messages on a comm channel to other users on the comm
 channel
 * View who is on a comm channel
 * Change comm channels

We are going to include middleware that:

 * Prevents messages being sent over 500 characters.
 * Allows debug logging of the boutique.Store as it is updated.
 * Deletes older messages in the boutique.Store that are no longer needed.

This example is not going to include all of the application's functions, just
enough to cover the subjects we are discussing. For example, we don't want to
get into how websockets work as that isn't important for understanding Boutique.

Let's start by defining the data we need, which is going to be stored
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
	// Messages waiting to be sent out to our users.
	Messages []Message
```

First there is the State object. Each comm channel that is opened for users to
communicate on has its own State object.  

Inside here, we have different attributes related to the state of the channel.
**ServerID** lets us identify the particular instance's log files,
**Channel** holds the name of our channel.  Users is the list of current users
in the **Channel**, while **Messages** is the current buffer of user messages
waiting to be sent out to the users.


### Create our actions

Here I'm going to include a smaller version of the actions.go from our example
application, to keep it simple.

```go
// Package actions details boutique.Actions that are used by Modifiers to modify the store.
package actions

import (
	"fmt"

	"github.com/johnsiilver/boutique"
)

const (
	// ActSendMessage indicates we want to send a message via the store.
	ActSendMessage boutique.ActionType = iota
	// ActAddUser indicates the Action wants to add a user to the store.
	ActAddUser
)

// SendMessage sends a message via the store.
func SendMessage(user string, s string) boutique.Action {
	m := data.Message{Timestamp: time.Now(), User: user, Text: s}
	return boutique.Action{Type: ActSendMessage, Update: m}, nil
}

// AddUser adds a user to the store, indicating a new user is in the comm channel.
func AddUser(u string) boutique.Action {
	return boutique.Action{Type: ActAddUser, Update: u}
}
```

Let's talk about the constants we defined:

```go
const (
	// ActSendMessage indicates we want to send a message via the store.
	ActSendMessage boutique.ActionType = iota
	// ActAddUser indicates the Action wants to add a user to the store.
	ActAddUser
)
```
These are our Action Types that will be used for signaling.  By convention,
these should be prefixed with "Act" to indicate its an Action type.

We have defined two types here, one that indicates the Action is trying to
send a message via the Store and one that is trying to add a user to the Store.

Now we have our Action creators:

```go
// SendMessage sends a message via the store.
func SendMessage(id int, user string, s string) boutique.Action {
	m := data.Message{Timestamp: time.Now(), User: user, Text: s}
	return boutique.Action{Type: ActSendMessage, Update: m}, nil
}

// AddUser adds a user to the store, indicating a new user is in the room.
func AddUser(u string) boutique.Action {
	return boutique.Action{Type: ActAddUser, Update: u}
}
```

SendMessage takes in the user sending the message, and the text
message itself. It creates a boutique.Action setting the Type to ActSendMessage
and the Update to a data.Message, which we will use to update our store.

### Writing Modifiers

So we need to touch on something we did not talk about in our first example.
There is a fundamental rule that MUST be obeyed by all **Modifier(s)**:

**THOU SHALL NOT MUTATE DATA!**

Non-references can be changed directly.  But reference types
or pointer values must be copied and replaced, never modified.  
This allows downstream readers to ignore locking.

So if you want to add a value to a slice, you must copy the slice, add the
new value, then change the reference in the Store.  You must never directly
append.  This is relatively fast on modern processors when data fits in the
cache.

The only exception to this is synchronization Types that can be copied, such
as a channel or \*sync.WaitGroup.  Do this sparingly!

We provide a few utility functions such as **CopyAppendSlice()**,
**ShallowCopy()**, and **DeepCopy()** to help ease this.

Here are some **Modifier(s)** to handle our **Actions**.  We could write one
Modifier to handle all **Actions** or multiple **Modifier(s)** handling each
individual **Actions**. I've chosen the latter, as I find it more readable.

```go
// All is a boutique.Modifiers made up of all Modifier(s) in this file.
var All = boutique.NewModifiers(SendMessage, AddUser)

// SendMessage handles an Action of type ActSendMessage.
func SendMessage(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActSendMessage:
		msg := action.Update.(data.Message)
		msg.ID = s.NextMsgID
		s.Messages = boutique.CopyAppendSlice(s.Messages, msg).([]data.Message)
		s.NextMsgID = s.NextMsgID + 1
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

All **Modifier(s)** follow the boutique.Modifier signature.  They receive a State
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

Now lets talk about **Modifiers**.

```go
var Modifiers = boutique.NewModifiers(SendMessage, AddUser)
```

Every update to boutique.Store is done through .Perform().  When .Perform()
is called, it runs all of your **Modifier(s)** in the order you choose.  These
**Modifier(s)** are registered to boutique.Store via the New() call.  
A **Modifiers** is a collection of **Modifier(s)** in the order they will
be applied.

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
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/modifiers"
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

	s, err := boutique.New(d, modifiers.All, nil)
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

Then the store is initiated containing our starting data, our **Modifiers**, and
no **Middleware** (we will come back to this).

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
// subscribe subscribes a user to the channel.
func (c *ChatterBox) subscribe(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, m messages.Client) (*state.Hub, error) {

c.chMu.Lock()
defer c.chMu.Unlock()

var (
	hub *state.Hub
	err error
)

// See if the channel exists, if so, get it.
mchan, ok := c.channels[m.Channel]
if ok {
	hub = mchan.hub
	if mchan.users[m.User] { // Don't allow two users with the same name to the same channel.
		// Send error on websocket and exit.
		...
		return nil, err
	}
} else {  // Channel doesn't exist, create it.
	hub, err = state.New(m.Channel)
	if err != nil {
		return nil, err
	}
	mchan = &channel{ctx: ctx, cancel: cancel, hub: hub, users: map[string]bool{m.User: true}}
	c.channels[m.Channel] = mchan
}

// Add the user to the channel.
mchan.users[m.User] = true
if err = hub.Store.Perform(actions.AddUser(m.User)); err != nil {
	return nil, err
}

// Send  a websocket acknowledgement.
...

glog.Infof("client %v: subscribed to %s as %s", conn.RemoteAddr(), m.User, m.Channel)
return hub, nil
```
At this point, nothing special has happened.  You've spent a lot more time
updating a struct, which is not all that useful.

But here is some payoff.  Every time someone subscribes to the channel, we can
now update all clients to the new user lists.  We also now have the ability to
subscribe all listeners to message updates.  Every time a user on this channel
submits a message, So for the person who just created the comm channel, lets
send him updates whenever anyone sends on the channel or joins/leaves the
channel.

#### Updating a client when new messages arrive

```go
// clientSender receives changes to the store's Messages/Users fields and pushes
// them out to our websocket clients.
func (c *ChatterBox) clientSender(ctx context.Context, usr string, chName string, conn *websocket.Conn, store *boutique.Store) {
	const (
		msgField   = "Messages"
		usersField = "Users"
	)

	state := store.State()
	startData := state.Data.(data.State)

	// We only want to send messages we haven't seen before.
	// Cleanup is done in Middleware.
	var lastMsgID = -1
	if len(startData.Messages) > 0 {
		lastMsgID = startData.Messages[len(startData.Messages)-1].ID
	}

	// Subscribe to our store's "Messages" field.
	msgCh, msgCancel, err := store.Subscribe(msgField)
	if err != nil {
		c.sendError(conn, err)
		return
	}
	defer msgCancel()  // cancel the subscription when this function ends.

	// Subscribe to our store's "Users" field.
	usersCh, usersCancel, err := store.Subscribe(usersField)
	if err != nil {
		c.sendError(conn, err)
		return
	}
	defer usersCancel()

	// Loop looking for messages to come in, users to subscribe/leave or
	// the context object to be cancelled.
	for {
		select {
		// Our .Messages changed.
		case msgSig := <-msgCh:
			msgs := msgSig.State.Data.(data.State).Messages
			if len(msgs) == 0 { // Don't send blank messages.
				continue
			}

			// Send all messages we haven't seen.
			var toSend []data.Message
			toSend, lastMsgID = c.latestMsgs(msgs, lastMsgID) // helper method.
			if len(toSend) > 0 {
				// This simply sends on the websocket.
				if err := c.sendMessages(conn, toSend); err != nil {
					glog.Errorf("error sending message to client on channel %s: %s", chName, err)
					return
				}
			}
		// Our .Users changed.
		case userSig := <-usersCh:
			// Send the message out to this client on a websocket.
			if err := c.write(conn, messages.Server{Type: messages.SMUserUpdate, Users: userSig.State.Data.(data.State).Users}); err != nil {
				c.sendError(conn, err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
```
The first thing we need to do is get the existing Store's data and calculate
the last Message. We only want to send messages after this:

```go
const (
	msgField   = "Messages"
	usersField = "Users"
)

state := store.State()
startData := state.Data.(data.State)

// We only want to send messages we haven't seen before.
// Cleanup is done in Middleware.
var lastMsgID = -1
if len(startData.Messages) > 0 {
	lastMsgID = startData.Messages[len(startData.Messages)-1].ID
}
```

Now that we've done that, let's subscribe to the fields we care about:
```go
// Subscribe to our store's "Messages" field.
msgCh, msgCancel, err := store.Subscribe(msgField)
if err != nil {
	c.sendError(conn, err)
	return
}
defer msgCancel()  // cancel the subscription when this function ends.

// Subscribe to our store's "Users" field.
usersCh, usersCancel, err := store.Subscribe(usersField)
if err != nil {
	c.sendError(conn, err)
	return
}
defer usersCancel()
```

Finally, the **for** loop, which reads off our various channels until
our context is killed.  Each time we receive from the subscription channels,
we update our client.
```go
for {
	select {
	// Our .Messages changed.
	case msgSig := <-msgCh:
		msgs := msgSig.State.Data.(data.State).Messages
		if len(msgs) == 0 { // Don't send blank messages.
			continue
		}

		// Send all messages we haven't seen.
		var toSend []data.Message
		toSend, lastMsgID = c.latestMsgs(msgs, lastMsgID) // helper method.
		if len(toSend) > 0 {
			// This simply sends on the websocket.
			if err := c.sendMessages(conn, toSend); err != nil {
				glog.Errorf("error sending message to client on channel %s: %s", chName, err)
				return
			}
		}
	// Our .Users changed.
	case userSig := <-usersCh:
		// Send the message out to this client on a websocket.
		if err := c.write(conn, messages.Server{Type: messages.SMUserUpdate, Users: userSig.State.Data.(data.State).Users}); err != nil {
			c.sendError(conn, err)
			return
		}
	case <-ctx.Done():
		return
	}
}
```

#### Update the Store.Messages when a client sends a new Message

```go
// clientReceiver is used to process messages that are received over the websocket from the client.
// This is meant to be run in a goroutine as it blocks for the life of the conn and decrements
// wg when it finally ends.
func (c *ChatterBox) clientReceiver(ctx context.Context, wg *sync.WaitGroup, conn *websocket.Conn) {
	defer wg.Done() // Let the caller know we are done.
	defer c.unsubscribe(user, comm) // When we no longer can talk to the client, unsubscribe.

	var (
		cancel context.CancelFunc
		hub    *state.Hub
		user   string
		comm   string
	)

	for {
		m, err := c.read(conn) // Reads from the websocket.
		if err != nil {
			glog.Errorf("client %s terminated its connection", conn.RemoteAddr())
			if cancel != nil {
				cancel()
			}
			return
		}

		err = m.Validate() // Validate the message.
		if err != nil {
			glog.Errorf("error: client %v message did not validate: %v: %#+v: ignoring...", conn.RemoteAddr(), err, m)
			if err = c.sendError(conn, err); err != nil {
				return
			}
			continue
		}

		switch t := m.Type; t {
		case messages.CMSendText:  // User wants to send a message.
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
		case messages.CMSubscribe: // User wants to subscribe to a channel.
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

			go c.clientSender(ctx, user, comm, conn, hub.Store) // Start sending messages to the client.
		case messages.CMDrop: // User wants to drop from a channel.
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
		default: // We don't understand the message.
			glog.Errorf("error: client %v had unknown message %v, ignoring", conn.RemoteAddr(), t)
			if err := c.sendError(conn, fmt.Errorf("received message type from client %v that the server doesn't understand", t)); err != nil {
				glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
				return
			}
		}
	}
}
```
So we will take this one section at a time:
```go
defer wg.Done() // Let the caller know we are done.
defer c.unsubscribe(user, comm) // When we no longer can talk to the client, unsubscribe.
```
Here, we let our caller know that we are done and unsubscribe from the channel
when this function ends.  This function ends only when we cannot read/write on
the websocket.

```go
var (
	cancel context.CancelFunc
	hub    *state.Hub
	user   string
	comm   string
)
```
Here we are keeping track of variables that only exist if we are subscribed to
a channel.  The **cancel()** is for when we want to kill the **clientSender()**.
**Hub** contains the boutique.Store for this channel's data.  **User** is the
current user the client is connected as.  Finally, **comm** is the name of the
channel we are in.

```go
for {
	m, err := c.read(conn) // Reads from the websocket.
	if err != nil {
		glog.Errorf("client %s terminated its connection", conn.RemoteAddr())
		if cancel != nil {
			cancel()
		}
		return
	}

	err = m.Validate() // Validate the message.
	if err != nil {
		glog.Errorf("error: client %v message did not validate: %v: %#+v: ignoring...", conn.RemoteAddr(), err, m)
		if err = c.sendError(conn, err); err != nil {
			return
		}
		continue
	}
```
Now we enter our loop.  We read off the websocket for a messsage. We then
**Validate()** that message.

```go
for {
	...
	switch t := m.Type; t {
	case messages.CMSendText:  // User wants to send a message.
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
}
```
Next we switch on the message type.  If the message is for sending text on
the channel, we check if we are in a channel with **if hub == nil**.  If we are,
we simply call **.Perform(actions.SendMessage(...))**, which will cause all
other clients to be updated via their subscriptions in **clientSender()**.

```go
...
switch t := m.Type; t {
	...
	case messages.CMSubscribe: // User wants to subscribe to a channel.
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

		go c.clientSender(ctx, user, comm, conn, hub.Store) // Start sending messages to the client.
```
Now our next type of message is when the client wants to subscribe to a channel.
We use a **context** with a **cancelFunc** to let us kill the **clientSender()**
we create.

If we are already subscribed to a channel, **unsubscribe()**.  

Then we **subscribe()** to the channel, and finally call **clientSender()**.

```go
...
switch t := m.Type; t {
	...
	case messages.CMDrop: // User wants to drop from a channel.
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
```
This message type signals the user wants to drop from a channel.
We simply reset our hub/user/comm variables and let the remote side know it
was successful.

```go
...
switch t := m.Type; t {
	...
	default: // We don't understand the message.
		glog.Errorf("error: client %v had unknown message %v, ignoring", conn.RemoteAddr(), t)
		if err := c.sendError(conn, fmt.Errorf("received message type from client %v that the server doesn't understand", t)); err != nil {
			glog.Errorf("error: client %v: %s", conn.RemoteAddr(), err)
			return
		}
	}
} // End function
```
Finally, we have our default statement, which is where we inform the client we
don't know what it is asking us to do.

#### Wrapping up the non-Middleware

There are of course a bunch of helper functions we didn't discuss, but those
aren't relevant to the discussion.

At this point, we have made a program that:
* Creates a boutique.Store for each comm channel that exists.
* Clients in a comm channel are updated of changes via various subscriptions
that trigger updates to be sent on their connected websockets.

There are other ways to do this of course.  You could create arrays of channels
that you lock/unlock as you add/remove users.  You could send messages down
those channels that are then processed and sent via the websocket.

And in some cases, that might be simpler.  But as your program expands, this
paradigm gets more complicated.  

Maybe you want to:
* Prevent messages longer than 500 characters.
* Log all messages to persistent storage.
* Allow debugging of all changes with users/messages.
* Add authentication checks on message sends.

Don't get me wrong, you can absolutely do this without Boutique.  You can do
this more efficiently.  But it takes a lot more thought and planning to get
right (and probably a lot of refactoring).  Boutique adds structure that can
make this simplistic.  

Those "Maybe you want to's" above are easily accomplished by adding Middleware.

### Middleware

#### Introduction
**Middleware** allows you to extend the Store by inserting data handlers into
the Perform() calls either before the commit to the Store or after the data
has been committed.

**Middleware** can:

* Change store data as an update passed through
* Deny an update
* Signal or spin off other async calls
* See the end result of the change

A few example middleware applications:

* Log all State changes for debug purposes
* Write certain data changes to long term storage
* Authorize/Deny changes
* Update certain fields in conjunction with the update type
* Provide cleanup mechanisms for certain fields
* ...

#### Defining Middleware
**Middleware** is simply a function/method that implements the
following signature:

```go
type Middleware func(args *MWArgs) (changedData interface{}, stop bool, err error)
```

Let's talk first about the args that are provided:

```go
type MWArgs struct {
	// Action is the Action that is being performed.
	Action Action
	// NewData is the proposed new State.Data field in the Store. This can be modified by the
	// Middleware and returned as the changedData return value.
	NewData interface{}
	// GetState if a function that will return the current State of the Store.
	GetState GetState
	// Committed is only used if the Middleware will spin off a goroutine.  In that case,
	// the committed state will be sent via this channel. This allows Middleware that wants
	// to do something based on the final state (like logging) to work.  If the data was not
	// committed due to another Middleware cancelling the commit, State.IsZero() will be true.
	Committed chan State

	// WG must have .Done() called by all Middleware once it has finished. If using Committed, you must
	// not call WG.Done() until your goroutine is completed.
	WG *sync.WaitGroup
}
```

So first we have **Action**.  By observing the **Action.Type**, you can see
what the **Action** was that **Perform()** was called with.
Altering this has no effect.

**NewData** is the **State.Data** that will result from the Action.  It has not
been committed. Altering this by itself will have no effect, but I will show
how to alter it in a moment and affect a change.

**GetState** is a function that you can call to get the current **State**
object.

Let's skip **Committed** for the moment, we'll get back to it later.

**WG** is very important.  Your **Middleware** must call **WG.Done()** before
exiting or your **Perform()** call will block forever.  There is a handy log
message that catches these if your forget during development.

Now that we have args out of the way, let's talk about the return values:

```go
(changedData interface{}, stop bool, err error)
```

**changedData** represents the **State.Data** with your changes, if any.
If you are not going to edit the data, then you can simply return **nil** here.
Otherwise you may modify **args.NewData** and then return it here to affect
your change.

**stop** is an indicator that you want to prevent other **Middleware** from
executing and immediately commit the change.

**err** indicates you wish to prevent the change and send an error to the
**Perform()** caller.

#### A synchronous Middleware

So let's design a synchronous **Middleware** that cleans up older Messages in
our example application. This will be synchronous because we do this before our
Perform is completed.

```go
// CleanMessages deletes data.State.Messages older than 1 Minute.
func CleanMessages(args *boutique.MWArgs) (changedData interface{}, stop bool, err error) {
	// Remember to do this, otherwise the Middleware will block a Perform() call.
	defer args.WG.Done()

	d := args.NewData.(data.State)  // Assert the data to the correct type.

	var (
		i         int
		m         data.Message
		deleteAll = true
		now     = time.Now()
	)

	// Find the first message that is within our expiring time.
	// Every message after that is still good.
	for i, m = range d.Messages {
		if now.Sub(m.Timestamp) < CleanTimer {
			deleteAll = false
			break
		}
	}

	switch {
	// Nothing should be changed, so return nil for the changedData.
	case i == 0:
		return nil, false, nil
	// Looks like all the Messages are expired, so kill them.
	case deleteAll:
		d.Messages = []data.Message{}
	// Copy the non-expired Messages into a new slice and then assign it to our
	// new data.State object.
	case len(d.Messages[i:]) > 0:
		newMsg := make([]data.Message, len(d.Messages[i:]))
		copy(newMsg, d.Messages[i:])
		d.Messages = newMsg
	}
	// Return our altered data.State object.
	return d, false, nil
}
```

First thing: defer our **args.WG.Done()** call!  Not doing this will cause
problems.

Next we need to go through our **[]data.Message** until we locate the first
index that is within our time limit.  Anything from there till the end of our
slice does not need to be deleted.

```go

for i, m = range d.Messages {
	if now.Sub(m.Timestamp) < CleanTimer {
		deleteAll = false
		break
	}
}
```
**deleteAll** simply lets us know if we find any **Message** not expired.
If we don't, we delete all messages.

Finally our switch statement handles all our cases.  Most are self explanatory,
but there is one that we should look at:

```go
case len(d.Messages[i:]) > 0:
	newMsg := make([]data.Message, len(d.Messages[i:]))
	copy(newMsg, d.Messages[i:])
	d.Messages = newMsg
```

Here we copy the data from the slice into a new slice, though that isn't
strictly necessary. **Middleware** is run after **Modifiers**, so all this data
is a copy already.  However, not doing so will make the slice smaller, but the
underlying array will continue to grow.  Your len() may be 0, but your
capacity might be 50,000.  Not what you want in a cleanup **Middleware**!

#### Asynchronous Middleware

Asynchronous **Middleware** is useful when you want to trigger something to
happen or view the final committed data.  However, it comes with the limitation
that you cannot alter the data.

Use asynchronous Middleware when:
* Triggering other code and your not required to modify data here
* You want to trigger something to happen after the commit to the store occurs

The key here is that no matter what, Asynchronous **Middleware** cannot alter
the data.

Let's create some **Middleware** that can be turned on or off at anytime and
lets us record a diff of our Store on each commit.  We can use this to debug
our application by observing changes to the Store's data.

```go
var pConfig = &pretty.Config{
	Diffable: true,

	// Field and value options
	IncludeUnexported:   false,
	PrintStringers:      true,
	PrintTextMarshalers: true,
}

type Logging struct {
	lastData boutique.State
	file *os.File
}

func NewLogging(fName string) (*Logging, error) {
	f, err := os.OpenFile(fName, os.O_WRONLY+ os.O_CREATEflag, 0664)
	if err != nil {
		return nil, err
	}
	return &Logging{file: f}, nil
}

func (l *Logging) DebugLog(args *boutique.MWArgs) (changedData interface{}, stop bool, err error) {
	go func() { // Set off our Asynchronous method.
		defer args.WG.Done() // Signal when we are done. Not doing this will cause the program to stall.

		state := <-args.Committed // Wait for our data to get committed.

		if state.IsZero() { // This indicates that another middleware killed the commit.  No need to log.
			return
		}

		d := state.Data.(data.State) // Typical type assertion.

		_, err := l.file.WriteString(fmt.Sprintf("%s\n\n", pConfig.Compare(l.lastData, state)))
		if err != nil {
			glog.Errorf("problem writing to debug file: %s", err)
			return
		}
		l.lastData = state
	}()

	return nil, false, nil // Don't change any data and let other Middleware execute.
}
```

So let's break this down, starting with **pConfig**.

I need something to diff the Store, and in this case I've decided to use the
pretty library by Kyle Lemons. I love this library for diffs and it gives
a lot of control on how things are diffed.  You can find it here:

http://github.com/kylelemons/godebug/pretty

Next we need to setup our **Logger**.  If the user starts the server with debug
logging turned on, we include this in our **Middleware**. If not we don't.

```go
type Logging struct {
	lastData boutique.State
	file *os.File
}
```

Here we are storing the last state we saw the boutique Store in and the file
that we are going to write our logs to.

Let's skip on down to the nitty gritty, shall we?

```go
defer args.WG.Done()
go func() { // Set off our Asynchronous method.
	state := <-args.Committed // Wait for our data to get committed.

	if state.IsZero() { // This indicates that another middleware killed the commit.  No need to log.
		return
	}

	d := state.Data.(data.State) // Typical type assertion.

	_, err := l.file.WriteString(fmt.Sprintf("%s\n\n", pConfig.Compare(l.lastData, state)))
	if err != nil {
		glog.Errorf("problem writing to debug file: %s", err)
		return
	}
	l.lastData = state
}()
```

First thing we do is kick off our **Middleware** into async mode with a
goroutine. If we didn't need to wait for the data to be committed, we would
simply do the:
```go
args.WG.Done()
```
immediately before the goroutine.  But we need to keep ordering intact for
proper logging, so we don't want multiple **Process()** calls to occur.

Next, we finally use that **args.Committed** channel.  
```go
state := <-args.Committed
```
This channel will return the committed state right after it is committed to the
store.  Now we have the data we need to write a diff to a file.

Finally we simply write out the diff of the Store to disk and update our
**.lastData** attribute.
```go
_, err := l.file.WriteString(fmt.Sprintf("%s\n\n", pConfig.Compare(l.lastData, state)))
if err != nil {
	glog.Errorf("problem writing to debug file: %s", err)
	return
}
l.lastData = state
```

## Final thoughts

I hope this has given a decent explanation on what you can do with Boutique.
In the future, I hope to restructure this into sections with some video
introductions and guide to testing.

Hopefully you will find this useful.  Happy coding!

## Previous works

Boutique has its origins from the Redux library: [http://redux.js.org](http://redux.js.org)

Redux is very useful for Javascript clients that need to store state. Its ideas
were the basis of this model.
