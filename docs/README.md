# Boutique

## One line summary

Boutique is an immutable state store with subscriptions to field changes.

## The long summary

Boutique provides a state store for storing immutable data.  This allows data
retrieved from the store to be used without providing synchronization even as
changes are made to data in the store by other go-routines.

In addition, Boutique allows subscriptions to be registered for changes to a
data field or any field changes.  Data is versioned, so you can compare the
version number between the data retrieved and the last data pulled.

Finally, Boutique supports middleware for any change that is being committed
to the store.  This allows for sets of features, such as a storage record of
changes to the store.

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
* In very certain circumstances, Boutique can have runtime errors due to using
interface{}
* Storage updates are done via **Actions**, which adds some complexity

The first, running slower is because we must not only type assert at different
points, but reflection is used to detect changes in the data fields that are in
the data being stored.  We also need to copy data out of maps, slices, etc...
into new maps, slices, etc... This cost is lessened by reads of data
without synchronization and reduced complexity in the subscription model.

The second, runtime errors, happen when one of two events occur.  The type of
data to be stored in Boutique is changed on a write.  The first data passed to
the store is the only type that can be stored.  Any attempt to store a different
type of data will result in an error.  The second way is if the data being
stored in Boutique is not a struct type.  The top level data must be a struct.
In a non-generic store, these would be caught by the compiler.  But these are
are generally non-issues.

The third is more difficult.  Changes are routed through **Actions**.  
**Actions** trigger **Modifers**, which also must be written.  The concepts
take a bit to understand and you have to be careful not to mutate the data when
writing **Modifier(s)**.   This adds a certain amount of
complexity. But once you get used to the method, the code is
easy to follow.

## Where are some example applications?

You can find several example applications of varying sophistication here:

IRC like chat server/client using websockets with a sample terminal UI.
Welcome back to the 70's:
http://github.com/johnsiilver/boutique/example/chatterbox

Stock buy/sell point notifier using desktop notifications:
http://github.com/johnsiilver/boutique/example/notifier

## What does using Boutique look like?

Forgetting all the setup, usage looks like this:
```go
// Create a boutique.Store which holds the State object (not defined here)
// with a Modifier function called AddUser (for changing field State.User).
store, err := boutique.New(State{}, boutique.NewModifiers(AddUser), nil)
if err != nil {
	// Do something
}

// Create a subscription to changes in the "Users" field.
userNotify, cancel, err := store.Subscribe("Users")
if err != nil {
	// Do something
}
defer cancel()  // Cancel our subscription when the function closes.

// Print out the user list whenever State.Users changes.
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
	// Do something.
}

// Change field .Users to contain "Joe".
if err := store.Perform(AddUser("Joe")); err != nil {
	// Do something.
}

// We can also just grab the state at any time.
s := store.State()
fmt.Println(s.Version)  // The current version of the Store.
fmt.Println(s.FieldVersions["Users"]) // The version of the .Users field.
fmt.Println(s.Data.(State).Users) // The .Users field.

```
Key things to note here:

* The State object retrieved from the signal requires no read locks.
* Perform() calls to do not require manual locks.
* Everything is versioned.
* Subscribers only receive the **latest**, not every update.  This cuts down on
unnecessary processing (it is possible, with Middleware to get every update).
* This is just scratching the surface with what you can do, especially with
Middleware.

## Start simply: the basics

http://github.com/johnsiilver/boutique/example/basic

This application simply spins up a bunch of goroutines and we use a
boutique.Store to track the number of goroutines running.

In itself, not practical, but it will help define our concepts.

### First, define what datat you want to store

To start with, the data to be stored must be of type struct.  Now to be clear,
this cannot be \*struct, it must be a plain struct.  It is also important to
note that only public fields can received notification of subscriber changes.

Here's the state we want to store:
```go
// State is our state data for Boutique.  This is what data we want to store.
type State struct {
	// Goroutines is how many goroutines we are running.
	Goroutines int
}
```

### Now, we need to define Actions for making changes to the State

```go
// These are our ActionTypes.  This inform us of what kind of change we want
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

So let's talk about what is going on.  First, we transform the **copy** of our
State object into its concrete state (instead of interface{}).  

Now we check to see if this is an action.Type we handle.  If not, we simply
skip doing anything (which will return the State as it was before the
Modifier was called).

If it was an ActIncr Action, we increment .Goroutines by action.Update, which
will be of type int.

If it was an ActDecr Action, we decrement .Goroutines by 1.

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
Here we subscribe to the .Goroutines field.  Whenever an update happens to this
field, we will get notified on channel **ch**.  

However, we will only get the **latest** update, not every update.
This is important to remember.

**cancel()*** cancels the subscription when the printer ends.

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

* We get a signal that .Goroutines has changed and print the value.
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
new boutique.Store instance.  We pass it the initial state, **State** and
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

  └── state
    ├── state.go
    ├── actions
    │   └── actions.go
		├── data
		|   └── data.go
		├── middleware
		|   └── middleware.go
    └── modifiers
        └── modifiers.go

The files are best organized by using them as follows:

* state.go - Holds the constructor for a boutique.Store for your application
* actions.go - Holds the actions that will be used by the updaters to update
the store
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

 * Send and receive messages on a channel to other users on the channel
 * View who is on a channel
 * Change channels

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

We have defined two types here, one that Indicates the Action is trying to
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

	s, err := boutique.New(d, updaters.All, nil)
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
  const field = "Messages"
  // lastMsgID tracks the last ID for a message we have seen. This allows us
  // to only send messages in the queue we haven't seen before.
  var lastMsgID = -1
  if len(startData.Messages) > 0 {
    lastMsgID = startData.Messages[len(startData.Messages)-1].ID
  }

  // lastVersion keeps track of the last version number of the Messages field.
  // We need this because it is possible for the field to change between
  // signals.
  var lastVersion uint64

  // Subscribe to changes to the "Messages" field in our Store.
  sigCh, cancel, err := store.Subscribe(field)
  if err != nil {
    // Send the error back on the websocket and close
    ...
  }
  defer cancel() // Stop our subscription.

  for sig := range sigCh {
    // If the Signal's field version is less than what we've already seen, then
    // just continue the loop.
    if sig.State.FieldVersions[field] <= lastVersion {
      continue
    }

		msgs := sig.State.Data.(data.State).Messages

		if len(msgs) == 0 {
			continue
		}

    // Send any messages that have appeared in the store since we last sent.
    // Note: This would get ugly if we didn't delete messages after they were
    // sent, which the application does, but we are not showing that code
    // here and is done in another method.
		var toSend []data.Message
		toSend, lastMsgID = c.sendThis(msgs, lastMsgID)

    // Send our message to the client via the websocket.
    ...
	}
```

```go

for sig := range sigCh {
  msgs := sig.State.Data.(data.State).Messages
  lastVersion = sig.State.FieldVersions[field]
  if len(msgs) == 0 {
    continue
  }

  for {
    var toSend []data.Message
    toSend, lastMsgID = c.latestMsgs(msgs, lastMsgID)
    if len(toSend) > 0 {
      if err := c.sendMessages(conn, toSend); err != nil {
        glog.Errorf("error sending message to client on channel %s: %s", chName, err)
        return
      }
    }

    if store.FieldVersion(field) > lastVersion {
      state = store.State()
      msgs = state.Data.(data.State).Messages
      lastVersion = state.FieldVersions[field]
      continue
    }
    break
  }
}
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

    if err := store.Perform(actions.SendMessage(usr, m.Text.Text)); err != nil {
      // Send the error back to the websocket client.
    }
  }
```

Now we simply read messages off the websocket.Conn object, update our Store
with an actions.SendMessage(), and all of our clients are magically updated!

### Middleware

#### Introduction
**Middleware** allows you to extend the Store by inserting data handlers into
the Perform() calls either before the commit to the Store or after the data
has been committed.

**Middleware** can:

* Change store data as an update passed through.
* Deny an update.
* Signal or spin off other async calls
* See the end result of the change

A few example middleware applications:

* Log all State changes for debug purposes
* Write certain data changes to storage
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
	// NewDate is the proposed new State.Data field in the Store. This can be modified by the
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

So first we have Action.  By observing the Action.Type, you can see what the
Action was that Perform() was called with.  Altering this has no effect.

NewData is the NewData will result State.Data from the Action.  It has not
been committed. Alerting this by itself will have no effect, but I will show
how to alter it in a moment and affect a change.

GetState is actually a function that you can call to get the current State
object.

Let's skip Committed for the moment, we'll get back to it later.

WG is very important.  Your **Middleware** must call WG.Done() before exiting or
your Perform() call will block forever.  There is a handy log message that
catches these if your forget during development.

Now that we have args out of the way, let's talk about the return values:

```go
(changedData interface{}, stop bool, err error)
```

changedData represents the State.Data you want to change. If you are not going
to edit the data, then you can simply return nil here.  Otherwise you may
modify args.NewData and then return it here to affect your change.

stop is an indicator that you want to prevent other **Middleware** from
executing and immediately commit the change.

err indicates you wish to prevent the change and send an error to the Perform()
caller.

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

First thing: defer our args.WG.Done() call!  Not doing this will cause problems.

Next we need to go through our []data.Message until we locate the first index
that is within our time limit.  Anything from there till the end of our slice
does not need to be deleted.

```go

for i, m = range d.Messages {
	if now.Sub(m.Timestamp) < CleanTimer {
		deleteAll = false
		break
	}
}
```
deleteAll simply lets us know if we find any Message not expired.  If we don't,
we delete all messages.

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

 * Trigger third-party code and you don't need to modify data
 * You want to trigger something to happen after the commit to the store occurs

 The key here is that no matter what, Asynchronous **Middleware** cannot alter
 the data.

Let's create some **Middleware** that can be turned on or off at anytime and
lets us record a diff of our Store on each commit.

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
		defer args.WG.Done() // Signal when we are done. Not doing this will caused the program to stall.

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

So let's break this down, starting with pConfig.

I need something to diff the Store, and in this case I've decided to use the
pretty library by Kyle Lemons. I love this library for diffs and it gives
a lot of control on how things are diffed.  You can find it here:

"github.com/kylelemons/godebug/pretty"

Next we need to setup our Logger.  If the user starts the server with debug
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
proper logging, so we don't want multiple Process() calls to occur.

Next, we finally use that args.Committed channel.  
```go
state := <-args.Committed
```
This channel will return the committed state right after it is committed to the
store.  Now we have the data we need to write a diff to a file.

Finally we simply write out the diff of the Store to disk and update our
.lastData attribute.
```go
_, err := l.file.WriteString(fmt.Sprintf("%s\n\n", pConfig.Compare(l.lastData, state)))
if err != nil {
	glog.Errorf("problem writing to debug file: %s", err)
	return
}
l.lastData = state
```


## Previous works

Boutique has its origins from the Redux library: [http://redux.js.org](http://redux.js.org)

Redux is very useful for Javascript clients that need to store state. However,
while Redux is great, its still Javascript.  Boutique extends the idea for the
server side with a much strong subscription model for updating listeners.
