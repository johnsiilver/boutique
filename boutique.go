/*
Package boutique provides an immutable state storage with subscriptions to
changes in the store. It is intended to be a single storage for individual client
data or a single state store for an application not requiring high QPS.

Features and Drawbacks

Features:

 * Immutable data does not require locking outside the store.
 * Subscribing to individual field changes are simple.
 * Data locking is handled by the Store.

Drawbacks:

 * You are giving up static type checks on compile.
 * Internal reflection adds about 1.8x overhead.
 * Must be careful to not mutate data.

Immutability

When we say immutable, we mean that everything gets copied, as Go
does not have immutable objects or types other than strings. This means every
update to a pointer or reference type (map, dict, slice) must make a copy of the
data before changing it, not a mutation.  Because of modern processors, this
copying is quite fast.

Usage structure

Boutique provides storage that is best designed in a modular method:

  └── state
    ├── state.go
    ├── actions
    │   └── actions.go
		├── data
		|   └── data.go
		├── middleware
		|   └── middleware.go
    └── updaters
        └── updaters.go

The files are best organized by using them as follows:

  state.go - Holds the constructor for a boutique.Store for your application
  actions.go - Holds the actions that will be used by the updaters to update the store
	data.go - Holds the definition of your state object
	middleware.go = Holds middleware for acting on changes to your data. This is not required
  updaters.go - Holds all the updaters that are used by the boutique.Store to modify the store's data


  Note: These are all simply suggestions, you can combine this in a single file or name the files whatever you wish.

Example

Please see github.com/johnsiilver/boutique for a complete guide to using this
package.  Its complicated enough to warrant some documentation to guide you
through.
*/
package boutique

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/mohae/deepcopy"
)

// Any is used to indicated to Store.Subscribe() that you want updates for
// any update to the store, not just a field.
const Any = "any"

var (
	publicRE = regexp.MustCompile(`^[A-Z].*`)
)

// Signal is used to signal upstream subscribers that a field in the Store.Store
// has changed.
type Signal struct {
	// Version is the version of the field that was changed.  If Any was passed, it will
	// be the store's version, not a specific field.
	Version uint64

	// Fields are the field names that were updated.  This is only a single name unless
	// Any is used.
	Fields []string

	// State is the new State object.
	State State
}

// FieldChanged loops over Fields to deterimine if "f" exists.
// len(s.Fields) is always small, so a linear search is optimal.
// Only useful if you are subscribed to "Any", as otherwise its a single entry.
func (s Signal) FieldChanged(f string) bool {
	for _, field := range s.Fields {
		if field == f {
			return true
		}
	}
	return false
}

// signaler provides a way to extract the latest Signal from a Store.Subscription.
// It is similar to channel semantics using Get() instead of <-.  If Get()
// returns a false, then the signaler is closed and there are no more Signals
// queued. Even if the signaler is closed, Get() will return a (Signal, true) if
// there are still Signals in the queue, just like a channel.
// signaler is used instead of a channel to allow receiving only the last state
// changes.
type signaler struct {
	done     chan struct{}
	mu       sync.Mutex
	signal   chan Signal
	insertCh chan Signal
}

func newsignaler() *signaler {
	return &signaler{
		done:     make(chan struct{}),
		signal:   make(chan Signal, 1),
		insertCh: make(chan Signal),
	}
}

// ch returns a chan Signal that will recieve Signals whenver a subscriber is
// updated.
func (s *signaler) ch() chan Signal {
	ch := make(chan Signal, 1)

	go func() {
		defer close(ch)

		// oldValue holds a value that is gotten by .get() but could not be
		// inserted on ch.
		var oldValue *Signal

		for {
			// Attempt to get a Signal within a timeout.
			v, ok, timeout := s.get(1 * time.Millisecond)

			switch {
			// signaler.close() was called.
			case !ok:
				return
			// .get()'s timeout was reached.
			case timeout:
				// If we have an oldValue that we tried to send on ch, but was blocked,
				// try again because it is still valid.
				if oldValue != nil {
					select {
					case ch <- *oldValue:
						oldValue = nil
					default:
					}
				}
			// We got a new value.
			default:
				select {
				// Try to put in on ch. If we can, delete oldValue, as it is not valid.
				case ch <- v:
					oldValue = nil
				// There was no room on ch, so store this value and try again if something
				// newer hasn't come along.
				default:
					oldValue = &v
				}
			}
		}
	}()
	return ch
}

// Get returns a Signal and a bool.  If the bool is false, then the signaler
// has been closed and has no more entries, the Signal will be the zero value.
// Otherwise it will contain the latest Signal.
func (s *signaler) get(timeout time.Duration) (signal Signal, ok bool, timedout bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

top:
	// Return a signal if there is one or take in a signal someone wants inserted.
	select {
	// signaler has been closed, but if there is still a Signal left, send it.
	// If not, let the user know.
	case <-s.done:
		select {
		case sig := <-s.signal:
			return sig, true, false
		default:
			return Signal{}, false, false
		}
	case <-time.After(timeout):
		return Signal{}, true, true
	// We have a signal to give them, so give it to them.
	case sig := <-s.signal:
		return sig, true, false
	// They want to insert a new signal.
	case i := <-s.insertCh:
		select {
		// There is room, put it in.
		case s.signal <- i:
			goto top
		// There isn't room, remove the older Signal and put the new one in.
		default:
			_ = <-s.signal
			s.signal <- i
			goto top
		}
	}
	panic("should never get here")
}

// close closes the signaler.
func (s *signaler) close() {
	close(s.done)
}

// insert puts a new Signal out.  This is not thread safe.
func (s *signaler) insert(sig Signal) {
	select {
	case <-s.done:
		panic("inserting on closed signaler")
	default:
		s.insertCh <- sig
	}
}

// ActionType indicates what type of Action this is.
type ActionType int

func (a ActionType) isActionType() bool {
	return true
}

// Action represents an action to take on the Store.
type Action struct {
	// Type should be an enumerated constant representing the type of Action.
	// It is valuable to use http://golang.org/x/tools/cmd/stringer to allow
	// for string representation.
	Type ActionType

	// Update holds the values to alter in the Update.
	Update interface{}
}

// Modifier takes in the existing state and an action to perform on the state.
// The result will be the new state.
// Implementation of an Modifier must be careful to not mutate "state", it must
// work on a copy only. If you are changing a reference type contained in
// state, you must make a copy of that reference first and then manipulate
// the copy, storing it in the new state object.
type Modifier func(state interface{}, action Action) interface{}

// Modifiers provides the internals the ability to use the Modifier.
type Modifiers struct {
	updater Modifier
}

// NewModifiers creates a new Modifiers with the Modifiers provided.
func NewModifiers(updaters ...Modifier) Modifiers {
	return Modifiers{updater: combineModifier(updaters...)}
}

// run calls the updater on state/action.
func (m Modifiers) run(state interface{}, action Action) interface{} {
	return m.updater(state, action)
}

// State holds the state data.
type State struct {
	// Version is the version of the state this represents.  Each change updates
	// this version number.
	Version uint64

	// FieldVersions holds the version each field is at. This allows us to track
	// individual field updates.
	FieldVersions map[string]uint64

	// Data is the state data.  They type is some type of struct.
	Data interface{}
}

// IsZero indicates that the State isn't set.
func (s State) IsZero() bool {
	if s.Data == nil {
		return true
	}
	return false
}

// GetState returns the state of the Store.
type GetState func() State

// MWArgs are the arguments to a Middleware implmentor.
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

// Middleware provides a function that is called before the state is written.  The Action that
// is being applied is passed, with the newData that is going to be commited, a method to get the current state,
// and committed which will close when newData is committed. It returns either a changed version of newData or
// nil if newData is unchanged.  It returns an indicator if we should stop processing middleware but continue
// with our commit of the newData.  And it returns an error if we should not commit.
// Finally the "wg" WaitGroup that is passed must have .Done() called when the Middleware finishes.
// "committed" can be ignored unless the middleware wants to spin off a goroutine that does something after
// the data is committed.  If the data is not committed because another Middleware returns an error, the channel will
// be closed with an empty state. This ability allow Middleware that performs things such as logging the final result.
// If using this ability, do not call wg.Done() until all processing is done.
type Middleware func(args *MWArgs) (changedData interface{}, stop bool, err error)

// combineModifier takes multiple Modifiers and combines them into a
// single instance.
// Note: We do not provide any safety here. If you
func combineModifier(updaters ...Modifier) Modifier {
	return func(state interface{}, action Action) interface{} {
		if err := validateState(state); err != nil {
			panic(err)
		}

		for _, u := range updaters {
			state = u(state, action)
		}
		return state
	}
}

// validateState validates that state is actually a Struct.
func validateState(state interface{}) error {
	if reflect.TypeOf(state).Kind() != reflect.Struct {
		return fmt.Errorf("a state may only be of type struct, which does not include *struct, was: %s", reflect.TypeOf(state).Kind())
	}
	return nil
}

// subscribers holds a mapping of field names to channels that will receive
// an update when the field name changes. A special field "any" will be updated
// for any change.
type subscribers map[string][]subscriber

type subscriber struct {
	id  int
	sig *signaler
}

type stateChange struct {
	old, new         interface{}
	newVersion       uint64
	newFieldVersions map[string]uint64
	changed          []string
}

// CancelFunc is used to cancel a subscription
type CancelFunc func()

func cancelFunc(c *Store, field string, id int) CancelFunc {
	return func() {
		c.smu.Lock()
		defer c.smu.Unlock()

		v := c.subscribers[field]
		if len(v) == 1 {
			v[0].sig.close()
			delete(c.subscribers, field)
			return
		}

		l := make([]subscriber, 0, len(v)-1)
		for _, s := range v {
			if s.id == id {
				s.sig.close()
				continue
			}
			l = append(l, s)
		}
		c.subscribers[field] = l
	}
}

type performOptions struct {
	committed *sync.WaitGroup
	subscribe chan State
	noUpdate  bool
}

// PerformOption is an optional arguement to Store.Peform() calls.
type PerformOption func(p *performOptions)

// WaitForCommit passed a WaitGroup that will be decremented by 1 when a
// Perform is completed.  This option allows you to use goroutines to call
// Perform, continue on and then wait for the commit to be completed.
// Because you cannot increment the WaitGroup before the Perform, you must wait
// until Peform() is completed. If doing Perform in a
func WaitForCommit(wg *sync.WaitGroup) PerformOption {
	return func(p *performOptions) {
		p.committed = wg
	}
}

// WaitForSubscribers passes a channel that will receive the State from a change
// once all subscribers have been updated with this state. This channel should
// generally have a buffer of 1. If not, the Perform() will return an error.
// If the channel is full when it tries to update the channel, no update will
// be sent.
func WaitForSubscribers(ch chan State) PerformOption {
	return func(p *performOptions) {
		p.subscribe = ch
	}
}

// NoUpdate indicates that when this Perform() is run, no subscribers affected
// should receive an update for this change.
func NoUpdate() PerformOption {
	return func(p *performOptions) {
		p.noUpdate = true
	}
}

// Store provides access to the single data store for the application.
// The Store is thread-safe.
type Store struct {
	// mod holds all the state modifiers.
	mod Modifiers

	// middle holds all the Middleware we must apply.
	middle []Middleware

	// pmu prevents concurrent Perform() calls.
	pmu sync.Mutex

	// state is current state of the Store. Its value is a interface{}, so we
	// don't know the type, but it is guarenteed to be a struct.
	state atomic.Value

	// version holds the version of the store.  This will be the same as .state.Version.
	version atomic.Value // uint64

	// fieldVersions is the versions of each field. This will be teh same as .state.FieldVersions.
	fieldVersions atomic.Value // map[string]uint64

	// smu protects subscribers and sid.
	smu sync.RWMutex

	// subscribers holds the map of subscribers for different fields.
	subscribers subscribers

	// sid is an id for a subscriber.
	sid int
}

// New is the constructor for Store. initialState should be a struct that is
// used for application's state. All Modifiers in mod must return the same struct
// that initialState contains or you will receive a panic.
func New(initialState interface{}, mod Modifiers, middle []Middleware) (*Store, error) {
	if err := validateState(initialState); err != nil {
		return nil, err
	}

	if mod.updater == nil {
		return nil, fmt.Errorf("mod must contain at least one Modifier")
	}

	fieldVersions := map[string]uint64{}
	for _, f := range fieldList(initialState) {
		fieldVersions[f] = 0
	}

	s := &Store{mod: mod, subscribers: subscribers{}, middle: middle}
	s.state.Store(State{Version: 0, FieldVersions: fieldVersions, Data: initialState})
	s.version.Store(uint64(0))
	s.fieldVersions.Store(fieldVersions)

	return s, nil
}

// Perform performs an Action on the Store's state.
func (s *Store) Perform(a Action, options ...PerformOption) error {
	opts := &performOptions{}
	for _, opt := range options {
		opt(opts)
	}

	defer func() {
		if opts.committed != nil {
			opts.committed.Done()
		}
	}()

	s.pmu.Lock()
	defer s.pmu.Unlock()

	state := s.state.Load().(State)
	n := s.mod.run(state.Data, a)

	var (
		commitChans []chan State
		err         error
	)

	middleWg := &sync.WaitGroup{}
	middleWg.Add(len(s.middle))
	n, commitChans, err = s.processMiddleware(a, n, middleWg)
	if err != nil {
		for _, ch := range commitChans {
			close(ch)
		}
		return err
	}

	s.perform(state, n, commitChans, opts)

	done := make(chan struct{})
	timer := time.NewTimer(5 * time.Second)
	go func() {
		middleWg.Wait()
		close(done)
	}()

	// This helps users diagnose misbehaving middleware.
	for {
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			glog.Infof("middleware is taking longer that 5 seconds, did you call wg.Done()?")
			continue
		}
		break
	}

	return nil
}

func (s *Store) processMiddleware(a Action, newData interface{}, wg *sync.WaitGroup) (data interface{}, commitChans []chan State, err error) {
	commitChans = make([]chan State, len(s.middle))
	for i := 0; i < len(commitChans); i++ {
		commitChans[i] = make(chan State, 1)
	}

	for i, m := range s.middle {
		cd, stop, err := m(&MWArgs{Action: a, NewData: newData, GetState: s.State, Committed: commitChans[i], WG: wg})
		if err != nil {
			return nil, nil, err
		}

		if cd != nil {
			newData = cd
		}

		if stop {
			break
		}
	}
	return newData, commitChans, nil
}

func (s *Store) perform(state State, n interface{}, commitChans []chan State, opts *performOptions) {
	changed := fieldsChanged(state.Data, n)

	// This can happen if middleware interferes.
	if len(changed) == 0 {
		return
	}

	// Copy the field versions so that its safe between loaded states.
	fieldVersions := make(map[string]uint64, len(state.FieldVersions))
	for k, v := range state.FieldVersions {
		fieldVersions[k] = v
	}

	// Update the field versions that had changed.
	for _, k := range changed {
		fieldVersions[k] = fieldVersions[k] + 1
	}
	sort.Strings(changed)

	sc := stateChange{
		old:              state.Data,
		new:              n,
		newVersion:       state.Version + 1,
		newFieldVersions: fieldVersions,
		changed:          changed,
	}

	writtenState := s.write(sc, opts)

	for _, ch := range commitChans {
		ch <- writtenState
	}
}

// write processes the change in state.
func (s *Store) write(sc stateChange, opts *performOptions) State {
	state := State{Data: sc.new, Version: sc.newVersion, FieldVersions: sc.newFieldVersions}
	s.state.Store(state)
	s.version.Store(sc.newVersion)
	s.fieldVersions.Store(sc.newFieldVersions)

	if opts.noUpdate {
		return state
	}

	s.smu.RLock()
	defer s.smu.RUnlock()

	if len(s.subscribers) > 0 {
		go s.cast(sc, state, opts)
	}
	return state
}

// Subscribe creates a subscriber to be notified when a field is updated.
// The notification comes over the returned channel.  If the field is set to
// the Any enumerator, any field change in the state data sends an update.
// CancelFunc() can be called to cancel the subscription. On cancel, chan Signal
// will be closed after the last entry is pulled from the channel.
// The returned channel is guarenteed to have the latest data at the time the
// Signal is returned. If you pull off the channel slower than the sender,
// you will still receive the latest data when you pull off the channel.
// It is not guarenteed that you will see every update, only the latest.
// If you need every update, you need to write middleware.
func (s *Store) Subscribe(field string) (chan Signal, CancelFunc, error) {
	if field != Any && !publicRE.MatchString(field) {
		return nil, nil, fmt.Errorf("cannot subscribe to a field that is not public: %s", field)
	}

	if field != Any && !fieldExist(field, s.State().Data) {
		return nil, nil, fmt.Errorf("cannot subscribe to non-existing field: %s", field)
	}

	sig := newsignaler()

	s.smu.Lock()
	defer s.smu.Unlock()
	defer func() { s.sid++ }()

	if v, ok := s.subscribers[field]; ok {
		s.subscribers[field] = append(v, subscriber{id: s.sid, sig: sig})
	} else {
		s.subscribers[field] = []subscriber{
			{id: s.sid, sig: sig},
		}
	}
	return sig.ch(), cancelFunc(s, field, s.sid), nil
}

// State returns the current stored state.
func (s *Store) State() State {
	return s.state.Load().(State)
}

// Version returns the current version of the Store.
func (s *Store) Version() uint64 {
	return s.version.Load().(uint64)
}

// FieldVersion returns the current version of field "f". If "f" doesn't exist,
// the default value 0 will be returned.
func (s *Store) FieldVersion(f string) uint64 {
	return s.fieldVersions.Load().(map[string]uint64)[f]
}

// cast updates subscribers for data changes.
func (s *Store) cast(sc stateChange, state State, opts *performOptions) {
	s.smu.RLock()
	defer s.smu.RUnlock()

	wg := &sync.WaitGroup{}
	for _, field := range sc.changed {
		if v, ok := s.subscribers[field]; ok {
			for _, sub := range v {
				wg.Add(1)
				go signal(Signal{Version: sc.newFieldVersions[field], State: state, Fields: []string{field}}, sub.sig, wg, opts)
			}
		}
	}

	for _, sub := range s.subscribers["any"] {
		wg.Add(1)
		go signal(Signal{Version: sc.newVersion, State: state, Fields: sc.changed}, sub.sig, wg, opts)
	}

	wg.Wait()
	if opts.subscribe != nil {
		select {
		case opts.subscribe <- state:
			// Do nothing
		default:
			glog.Errorf("someone passed a WaitForSubscribers with a full channel")
		}
	}
}

// signal sends a Signa on a Signler.
func signal(sig Signal, signaler *signaler, wg *sync.WaitGroup, opts *performOptions) {
	defer wg.Done()

	signaler.insert(sig)
}

// fieldExists returns true if the field exists in "i".  This will panic if
// "i" is not a struct.
func fieldExist(f string, i interface{}) bool {
	return reflect.ValueOf(i).FieldByName(f).IsValid()
}

// fieldsChanged detects if a field changed between a and z. It reports that
// field name in the return. It is assumed a and z are the same type, if not
// this will not work correctly.
func fieldsChanged(a, z interface{}) []string {
	r := []string{}

	av := reflect.ValueOf(a)
	zv := reflect.ValueOf(z)

	for i := 0; i < av.NumField(); i++ {
		if av.Field(i).CanInterface() {
			if !reflect.DeepEqual(av.Field(i).Interface(), zv.Field(i).Interface()) {
				r = append(r, av.Type().Field(i).Name)
			}
		}
	}
	return r
}

// FieldList takes in a struct and returns a list of all its field names.
// This will panic if "st" is not a struct.
func fieldList(st interface{}) []string {
	v := reflect.TypeOf(st)
	sl := make([]string, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		sl[i] = v.Field(i).Name
	}
	return sl
}

// ShallowCopy makes a copy of a value. On pointers or references, you will
// get a copy of the pointer, not of the underlying value.
func ShallowCopy(i interface{}) interface{} {
	return i
}

// DeepCopy makes a DeepCopy of all elements in "from" into "to".
// "to" must be a pointer to the type of "from".
// "from" and "to" must be the same type.
// Private fields will not be copied.  It this is needed, you should handle
// the copy method yourself.
func DeepCopy(from, to interface{}) error {
	if reflect.TypeOf(to).Kind() != reflect.Ptr {
		return fmt.Errorf("DeepCopy: to must be a pointer")
	}

	cpy := deepcopy.Copy(from)
	glog.Infof("%v", reflect.ValueOf(to).CanSet())
	reflect.ValueOf(to).Elem().Set(reflect.ValueOf(cpy))

	return nil
}

// CopyAppendSlice takes a slice, copies the slice into a new slice and appends
// item to the new slice.  If slice is not actually a slice or item is not the
// same type as []Type, then this will panic.
// This is simply a convenience function for copying then appending to a slice.
// It is faster to do this by hand without the reflection.
// This is also not a deep copy, its simply copies the underlying array.
func CopyAppendSlice(slice interface{}, item interface{}) interface{} {
	i, err := copyAppendSlice(slice, item)
	if err != nil {
		panic(err)
	}
	return i
}

// copyAppendSlice implements CopyAppendSlice, but with an error if there is
// a type mismatch. This makes it easier to test.
func copyAppendSlice(slice interface{}, item interface{}) (interface{}, error) {
	t := reflect.TypeOf(slice)
	if t.Kind() != reflect.Slice {
		return nil, fmt.Errorf("CopyAppendSlice 'slice' argument was a %s", reflect.TypeOf(slice).Kind())
	}
	if t.Elem().Kind() != reflect.TypeOf(item).Kind() {
		return nil, fmt.Errorf("CopyAppendSlice item is of type %s, but slice is of type %s", t.Elem(), reflect.TypeOf(item).Kind())
	}

	slicev := reflect.ValueOf(slice)
	var newcap, newlen int
	if slicev.Len() == slicev.Cap() {
		newcap = slicev.Len() + 1
		newlen = newcap
	} else {
		newlen = slicev.Len() + 1
		newcap = slicev.Cap()
	}

	ns := reflect.MakeSlice(slicev.Type(), newlen, newcap)

	reflect.Copy(ns, slicev)

	ns.Index(newlen - 1).Set(reflect.ValueOf(item))
	return ns.Interface(), nil
}
