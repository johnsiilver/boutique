/*
This program is a simplistic example of using Boutique as a state store.
This is not a practical example, it just shows how Boutique works.

This example spins up 1000 goroutines that sleep for 1-5 seconds.
Our state store simply needs to track how many goroutines are running at a given
time and print that number out as the store is updated.

Boutique allows us to subscribe to state changes and we only receive the
latest update, not all updates.  This again is not a practical example, because
we only have 1 subscriber to our changes.  This would be easier to accomplish
normally with atomic.Value for this counter.
*/
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/johnsiilver/boutique"
)

// State is our state data for Boutique.  This is what data we want to store.
type State struct {
	// Goroutines is how many goroutines we are running.
	Goroutines int
}

// These are our ActionTypes.  This inform us of what kind of change we want
// to do with an Action.
const (
	// ActIncr indicates we are incrementing the Goroutines field.
	ActIncr boutique.ActionType = iota

	// ActDecr indicates we are decrementing the Gorroutines field.
	ActDecr
)

///////////////////////////////////////////////////////////////////////////////
// These are our Action creators.  They create an Action to update the Store
// and are always used with a boutique.Store.Perform() call.
///////////////////////////////////////////////////////////////////////////////

// IncrGoroutines creates an ActIncr boutique.Action.
func IncrGoroutines(n int) boutique.Action {
	return boutique.Action{Type: ActIncr, Update: n}
}

// DecrGoroutines creates and ActDecr boutique.Action.
func DecrGoroutines() boutique.Action {
	return boutique.Action{Type: ActDecr}
}

///////////////////////////////////////////////////////////////////////////////

///////////////////////////////////////////////////////////////////////////////
// This is our single Modifier. It looks at an Action and determines if it
// was meant to handle it. If not, it just returns the State that was passed in.
// Otherwise it makes the modification to the State.  You must be careful not to
// mutate data here. In this example, .Goroutines is not a pointer or reference
// type, so we do not have to worry about this.
///////////////////////////////////////////////////////////////////////////////

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

///////////////////////////////////////////////////////////////////////////////

func main() {
	// Create our new Store with our default State{} object and our only
	// Modifier.  We are not going to define Middleware, so we pass nil.
	store, err := boutique.New(State{}, boutique.NewModifiers(HandleIncrDecr), nil)
	if err != nil {
		panic(err)
	}

	// killPrinter lets us signal our printer goroutine that we no longer need
	// its services.
	killPrinter := make(chan struct{})
	// printerKilled informs us that the printer goroutine has exited.
	printerKilled := make(chan struct{})

	// Write out our goroutine count as it changes.  Include this goroutine
	// in the count.
	store.Perform(IncrGoroutines(1))
	go func() {
		defer close(printerKilled)
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
			case <-killPrinter: // We were told to die.
				return
			}
		}
	}()

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
}
