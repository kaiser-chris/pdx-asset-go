package state

import (
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/database"
)

// lengths says every animation of a test runs for as many seconds as the map
// gives it.
func lengths(seconds map[string]float64) func(string) (float64, bool) {
	return func(animation string) (float64, bool) {
		length, ok := seconds[animation]

		return length, ok
	}
}

// A state that does not loop ends with its animation and goes on to the next,
// carrying over the time it overshot by.
func TestAdvanceGoesOn(t *testing.T) {
	machine := New([]asset.State{
		{Name: "entry", Animation: "a", Looping: database.Some(false), NextState: "idle"},
		{Name: "idle", Animation: "b", Looping: database.Some(false), NextState: "idle"},
	}, "entry", 1)

	length := lengths(map[string]float64{"a": 2, "b": 3})

	machine.Advance(1.5, length)

	if animation, seconds, _, ok := machine.Playing(); !ok || animation != "a" || seconds != 1.5 {
		t.Errorf("playing %s at %v, want a at 1.5", animation, seconds)
	}

	machine.Advance(1, length)

	if animation, seconds, _, _ := machine.Playing(); animation != "b" || seconds != 0.5 || machine.State() != "idle" {
		t.Errorf("playing %s at %v in %s, want b at 0.5 in idle", animation, seconds, machine.State())
	}

	// Round again into the same state.
	machine.Advance(3, length)

	if animation, seconds, _, _ := machine.Playing(); animation != "b" || seconds != 0.5 {
		t.Errorf("playing %s at %v, want b at 0.5 again", animation, seconds)
	}
}

// A state with no next state holds its last pose, unless it loops; an
// animation whose length is not known yet does not end.
func TestAdvanceHoldsAndLoops(t *testing.T) {
	held := New([]asset.State{{Name: "still", Animation: "a", Looping: database.Some(false)}}, "still", 1)
	held.Advance(5, lengths(map[string]float64{"a": 2}))

	if _, seconds, loop, _ := held.Playing(); seconds != 2 || loop {
		t.Errorf("held at %v, looping %v; want the end, 2", seconds, loop)
	}

	looping := New([]asset.State{{Name: "spin", Animation: "a", Looping: database.Some(true)}}, "spin", 1)
	looping.Advance(5, lengths(map[string]float64{"a": 2}))

	if _, seconds, loop, _ := looping.Playing(); seconds != 5 || !loop {
		t.Errorf("looping at %v, looping %v; want 5, looped by whoever poses it", seconds, loop)
	}

	unknown := New([]asset.State{{Name: "s", Animation: "x", NextState: "s"}}, "s", 1)
	unknown.Advance(5, lengths(nil))

	if _, seconds, _, _ := unknown.Playing(); seconds != 5 {
		t.Errorf("unread animation at %v, want it still playing at 5", seconds)
	}
}

// Going to a name picks one of its states by their chances, and a state of no
// chance never.
func TestPickByChance(t *testing.T) {
	states := []asset.State{
		{Name: "idle", Animation: "often", Chance: database.Some(9.0)},
		{Name: "idle", Animation: "seldom", Chance: database.Some(1.0)},
		{Name: "idle", Animation: "never", Chance: database.Some(0.0)},
	}

	counts := map[string]int{}

	for seed := range uint64(1000) {
		animation, _, _, _ := New(states, "idle", seed).Playing()
		counts[animation]++
	}

	if counts["never"] != 0 || counts["often"] < 800 || counts["seldom"] < 50 {
		t.Errorf("picked %v, want about nine in ten often and never none", counts)
	}

	if _, _, _, ok := New(states, "missing", 1).Playing(); ok {
		t.Error("a machine started in a state the entity lacks plays something")
	}
}

// A state can weigh itself by the state playing when the pick is made.
func TestPickByCurrentState(t *testing.T) {
	machine := New([]asset.State{
		{Name: "start", Animation: "s", NextState: "next"},
		{Name: "next", Animation: "from_start", ChanceFromState: []database.Amount{{Key: "start", Amount: 1}}},
		{Name: "next", Animation: "otherwise", ChanceFromState: []database.Amount{{Key: "elsewhere", Amount: 1}}},
	}, "start", 1)

	machine.Advance(2, lengths(map[string]float64{"s": 1}))

	if animation, _, _, _ := machine.Playing(); animation != "from_start" {
		t.Errorf("playing %s, want the state weighed by start", animation)
	}
}
