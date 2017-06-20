# Boutique

## One line summary

Boutique is an immutable data store with subscriptions to field changes.

## The long summary

Boutique provides a data store for storing immutable data.  This allows data retrieved from the store to be used without providing synchronization even as changes are made to data in the store by other go-routines.

In addition, Boutique allows subscriptions to be registered for changes to a data field or any field changes.  Data is versioned, so you can compare the version number between the data retrieved and the last data pulled.

Finally, Boutique supports middleware for any change that is being committed to the store.  This allows for sets of features, such as a storage record of changes to the store.

## Before we get started

### Go doesn't have immutable objects, does it?

Correct, Go doesn't have immutable objects.  It does contain immutable types, such as strings and constants.  However, immutability in this case is simply a contract to only change the data through the Boutique store.  All changes in the store must copy the data before committing the changes.

We will cover how this works later.

## What is the cost of using a generic immutable store?

There are three main drawbacks for using Boutique:

* Boutique writes are slower than a non generic implementation due to type assertion,  reflection and data copies
* In very certain circumstances, Boutique can have runtime errors due to using interface{}
* Storage updates are done via Actions, which adds some complexity

The first, running slower is because we must not only type assert at different points, but reflection is used to detect changes in the data fields that are in the data being stored.  This cost can be made up for by reads of data without synchronization and reduced complexity in the subscription model.

The second, runtime errors, happen when one of two events occur.  The type of data to be stored in Boutique is changed on a write.  The first data passed to the store is the only type that can be stored.  Any attempt to store a different type of data will be result in an error.  The second way is if the data being stored in Boutique is not a struct type.  The top level data must be a struct.  In a non-generic store, these would be caught by the compiler.  But these are easy to avoid.

The third is more difficult.  Changes are routed through Actions.  Actions trigger Updaters, which also must be written.  The concepts take a bit to understand and you have to be careful to copy the data and not mutate the data when writing Updaters.   tThis adds a certain amount of complexity, but changes to the store are easily readable from a readability perspective.

## Best use cases?

Boutique is useful in the following scenarios:

* You want a web based application to store state on the server and not in Javascript clients.  Keep the client as a simple renderer of data changes sent from the server.
* Your application has lots of clients, each which need to store state and receive updates.
* You have an application that needs to store a single state and send changes to clients.  However, be aware that boutique has runtime costs and may not be appropriate if every nanosecond counts.

## Let's get started!

### First, define what data you want to store

To start with, the data to be stored must be of type struct.  Now to be clear, this cannot be \*struct, it must be a plain struct.  It is also important to note that only public fields can received notification of subscriber changes.

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
}

```

### Create our actions

Boutique requires changes to be submitted via Actions.  A boutique.Action contains two fields:

* Type, which indicates a type of action that is being committed to the store.
* Update, which can be nil or contain a type that is used in updating the store.  This may be a value that will be placed in a field, a key that will be deleted from a map, or whatever is needed.

```go
package actions

import (
  "github.com/johnsiilver/boutique"
)

// These represent different type of actions we are committing to the store.
const (
    Unknown = iota
    ActAdd
    ActDelete
    ActSubmit
)

type AddUpdate struct {
  ID string
  Op Operation
}

// AddOp adds an operation to the store's Calculator's field at id "id".
func AddOp(id string, o Operation) boutique.Action {
  return boutique.Action{
    Type: ActAdd,
    Update: AddUpdate{id, o},
  }
}

// DeleteOp removes an operation from the store's Calculator field at id "id".
func DeleteOp(id string) boutique.Action {
  return boutique.Action{
    Type: ActDelete,
    Update: id,
  }
}

// Submit submits the calculation. This entails removing the Calculation from .Calculations
// and moving it to .Submitted.
func Submit(id string) boutique.Action {
  return boutique.Action{
    Type: ActSubmit,
    Update: id,
  }
}
```

### Writing Updaters

Updaters interpret Actions and handle updating the data in the store.  All Updaters must conform to the following signature:

```go
type Updater func(state interface{}, action Action) interface{}
```

The "state" is the data object that will get updated.  In our case, this would be "Data" that we defined above.  "action" is the Action that is to be processed.  An Updater does NOT have to handle an Action.  Updater returns the updated state object.

Here we will define Updaters to handle our Actions.  We could write one Updater to handle all Actions or multiple Updaters handling each individual Actions.

```go
package updaters

func Add(state interface{}, action Action) interface{} {
  // This only handles the Add Action, so if it is not that type, just return the state we received.
  if action.Type != actions.ActAdd {
    return s
  }

  s := state.(store.Data)
  a := action.Update.(actions.AddUpdate)

  v := s.Calculators[a.ID]
  m := map[string]Calculators{}
  for k, v := s.Calculators {


  s.Operations = boutique.CopyAppendSlice(v, a.Op).([]actions.Operation)
}

func Delete(state interface{}, action Action) interface{} {
 if action.Type != actions.ActAdd {
    return s
  }

  s := state.(store.Data)

}

func Submit(state interface{}, action Action) interface{} {
  if action.Type != actions.ActAdd {
    return s
  }

  s := state.(store.Data)


}
// RunTime updates our state object for ActUpdateRunTime Actions.
func RunTime (state interface{}, action Action) interface{} {
  s := state.(MyState)

  switch action.Type {
  case ActUpdateRunTime:
    s.RunTime = action.Update.RunTime.(time.Time)
  }
  return s
}
```

## Previous works

Boutique is based on the Redux library: [http://redux.js.org](http://redux.js.org)

