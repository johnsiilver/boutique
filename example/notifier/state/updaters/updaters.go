// Package updaters holds all the boutique.Updaters and the boutique.Modifer for the state store.
package updaters

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/notifier/state/actions"
	"github.com/johnsiilver/boutique/example/notifier/state/data"
)

// Modifiers is a boutique.Modifiers made up of all Modifier(s) in this file.
var Modifiers = boutique.NewModifiers(Tracking, ChangePoint)

// Tracking handles an Action of type action.ActTrack or action.ActUntrack.
func Tracking(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActTrack:
		msg := action.Update.(data.Stock)

		if _, ok := s.Tracking[msg.Symbol]; ok {
			break
		}

		to := map[string]data.Stock{}
		if err := boutique.DeepCopy(s.Tracking, &to); err != nil {
			panic(err)
		}

		to[msg.Symbol] = msg

		s.Tracking = to
	case actions.ActUntrack:
		symbol := action.Update.(string)

		if _, ok := s.Tracking[symbol]; !ok {
			break
		}

		to := map[string]data.Stock{}
		if err := boutique.DeepCopy(s.Tracking, &to); err != nil {
			panic(err)
		}

		delete(to, symbol)
		s.Tracking = to
	}
	return s
}

// ChangePoint handles an Action of type action.ActChangePoint.
func ChangePoint(state interface{}, action boutique.Action) interface{} {
	s := state.(data.State)

	switch action.Type {
	case actions.ActChangePoint:
		u := action.Update.(data.Stock)
		var (
			v  data.Stock
			ok bool
		)
		if v, ok = s.Tracking[u.Symbol]; !ok {
			break
		}

		switch {
		case u.Current > 0:
			to := map[string]data.Stock{}
			if err := boutique.DeepCopy(s.Tracking, &to); err != nil {
				panic(err)
			}
			v.Current = u.Current
			to[u.Symbol] = v
			s.Tracking = to
		case u.Buy > 0:
			to := map[string]data.Stock{}
			if err := boutique.DeepCopy(s.Tracking, &to); err != nil {
				panic(err)
			}
			v.Buy = u.Buy
			to[u.Symbol] = v
			s.Tracking = to
		case u.Sell > 0:
			to := map[string]data.Stock{}
			if err := boutique.DeepCopy(s.Tracking, &to); err != nil {
				panic(err)
			}
			v.Sell = u.Sell
			to[u.Symbol] = v
			s.Tracking = to
		}
	}
	return s
}
