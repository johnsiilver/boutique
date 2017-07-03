package modifiers

import (
	"runtime"
	"sort"
	"testing"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/notifier/state/actions"
	"github.com/johnsiilver/boutique/example/notifier/state/data"
	"github.com/kylelemons/godebug/pretty"
	"github.com/lukechampine/freeze"
)

// supportedOS prevents tests from running on non-unix systems.
// Windows cannot freeze memory.
func supportedOS() bool {
	switch runtime.GOOS {
	case "linux", "darwin":
		return true
	}
	return false
}

func TestTracking(t *testing.T) {
	if !supportedOS() {
		return
	}

	tests := []struct {
		desc   string
		action boutique.Action
		keys   []string
	}{
		{
			desc:   "Add stock",
			action: boutique.Action{Type: actions.ActTrack, Update: data.Stock{Symbol: "googl"}},
			keys:   []string{"aapl", "googl"},
		},
		{
			desc:   "Remove stock",
			action: boutique.Action{Type: actions.ActUntrack, Update: "googl"},
			keys:   []string{"aapl"},
		},
	}

	for _, test := range tests {
		// This validates that we didn't mutate our map.
		tr := map[string]data.Stock{"aapl": data.Stock{}} // bug(https://github.com/lukechampine/freeze/issues/4)
		tr = freeze.Map(tr).(map[string]data.Stock)
		d := data.State{Tracking: tr}

		newState := Tracking(d, test.action)

		keys := []string{}
		for k := range newState.(data.State).Tracking {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if diff := pretty.Compare(test.keys, keys); diff != "" {
			t.Errorf("TestTracking(%s): -want/+got:\n%s", test.desc, diff)
		}
	}
}

func TestChangePoint(t *testing.T) {
	if !supportedOS() {
		return
	}

	const (
		apple = "aapl"
		val   = 3.40
	)

	tests := []struct {
		desc   string
		update data.Stock
		want   data.Stock
	}{
		{
			desc:   "Change buy",
			update: data.Stock{Symbol: apple, Buy: val},
			want:   data.Stock{Symbol: apple, Buy: val},
		},
		{
			desc:   "Change sell",
			update: data.Stock{Symbol: apple, Sell: val},
			want:   data.Stock{Symbol: apple, Sell: val},
		},
		{
			desc:   "Change current",
			update: data.Stock{Symbol: apple, Current: val},
			want:   data.Stock{Symbol: apple, Current: val},
		},
	}

	for _, test := range tests {
		// This validates that we didn't mutate our map.
		tr := map[string]data.Stock{apple: data.Stock{Symbol: apple}} // bug(https://github.com/lukechampine/freeze/issues/4)
		tr = freeze.Map(tr).(map[string]data.Stock)
		d := data.State{Tracking: tr}

		newState := ChangePoint(d, boutique.Action{Type: actions.ActChangePoint, Update: test.update})

		if diff := pretty.Compare(test.want, newState.(data.State).Tracking[apple]); diff != "" {
			t.Errorf("TestChangePoint(%s): -want/+got:\n%s", test.desc, diff)
		}
	}
}
