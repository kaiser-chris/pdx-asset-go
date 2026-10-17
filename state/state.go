// Package state plays the states of an entity, as its .asset definition
// writes them:
//
//	state = { name = "idle_entry" animation = "body_idle1_1" looping = no next_state = "idle1" }
//	state = { name = "idle1" animation = "body_idle1_1" chance = 10 looping = no next_state = "idle1" }
//	state = { name = "idle1" animation = "body_idle1_2" chance = 5 looping = no next_state = "idle1" }
//
// A state plays an animation of the entity's mesh. Several states can have
// one name, and going to that name picks one of them by its chance. A state
// that does not loop ends when its animation does, or when its time is up,
// and goes on to its next state; one with no next state holds its last pose.
// That is how a portrait's idle goes on from one animation to another
// without repeating itself.
//
// A Machine knows nothing of the animations themselves, only their ids: how
// long each runs it is told, and what it says to play is posed by whoever
// asked.
package state

import (
	"math/rand/v2"

	"github.com/kaiser-chris/pdx-parser-go/asset"
)

// Machine plays the states of one entity.
type Machine struct {
	states []asset.State
	random *rand.Rand

	// current is the state playing, nil when there is none, and elapsed how
	// far into it the machine is, in seconds of its animation.
	current *asset.State
	elapsed float64
}

// New starts a machine in a state, by name, picking one of the states of
// that name by their chances. The seed makes the picks repeat for the same
// seed. A machine started in a state the entity does not have plays nothing.
func New(states []asset.State, start string, seed uint64) *Machine {
	machine := &Machine{states: states, random: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
	machine.current = machine.pick(start)

	return machine
}

// Playing is what the machine plays now: the id of the animation, how far
// into it, in seconds, and whether it loops. It reports false when the
// machine plays nothing.
func (m *Machine) Playing() (animation string, seconds float64, loop bool, ok bool) {
	if m == nil || m.current == nil {
		return "", 0, false, false
	}

	return m.current.Animation, m.elapsed, m.current.Looping.Or(false), true
}

// State is the name of the state playing, empty when there is none.
func (m *Machine) State() string {
	if m == nil || m.current == nil {
		return ""
	}

	return m.current.Name
}

// Advance moves the machine on by a time, in seconds, going on to the next
// state when the one playing ends. length says how long an animation runs,
// by its id, and false for one that is not known yet, such as one still
// being read: a state whose animation is not known does not end, unless its
// own time is up.
func (m *Machine) Advance(seconds float64, length func(animation string) (float64, bool)) {
	if m == nil || m.current == nil || seconds <= 0 {
		return
	}

	m.elapsed += seconds * m.current.AnimationSpeed.Or(1)

	// A short state could end several times over in one long step; it goes
	// on no further than a state per state the entity has, so that states
	// that take no time cannot keep it going for ever.
	for range len(m.states) + 1 {
		duration, ok := m.duration(length)
		if !ok || m.elapsed < duration {
			return
		}

		if m.current.NextState == "" {
			if !m.current.Looping.Or(false) {
				m.elapsed = duration
			}

			return
		}

		next := m.pick(m.current.NextState)
		if next == nil {
			m.elapsed = duration

			return
		}

		m.elapsed -= duration
		m.current = next
	}
}

// duration is how long the state playing lasts: its own time when it has
// one, otherwise as long as its animation runs, unless it loops, which goes
// on for as long as nothing moves it on.
func (m *Machine) duration(length func(animation string) (float64, bool)) (float64, bool) {
	if time, ok := m.current.StateTime.Get(); ok {
		return time, true
	}

	if m.current.Looping.Or(false) {
		return 0, false
	}

	seconds, ok := length(m.current.Animation)
	if !ok || seconds <= 0 {
		return 0, false
	}

	return seconds, true
}

// pick picks one of the states of a name by their chances, as weighed from
// the state playing. It returns nil when there is no such state, or none has
// any chance.
func (m *Machine) pick(name string) *asset.State {
	var (
		candidates []*asset.State
		weights    []float64
		total      float64
	)

	for index := range m.states {
		state := &m.states[index]
		if state.Name != name {
			continue
		}

		weight := m.chance(state)
		if weight <= 0 {
			continue
		}

		candidates = append(candidates, state)
		weights = append(weights, weight)
		total += weight
	}

	if len(candidates) == 0 {
		return nil
	}

	point := m.random.Float64() * total

	for index, weight := range weights {
		if point < weight {
			return candidates[index]
		}

		point -= weight
	}

	return candidates[len(candidates)-1]
}

// chance is how likely a state is to be picked: by the state playing, where
// the state weighs it so, otherwise its plain chance, and a state that gives
// none as likely as any other.
func (m *Machine) chance(state *asset.State) float64 {
	if len(state.ChanceFromState) > 0 {
		if m.current != nil {
			for _, from := range state.ChanceFromState {
				if from.Key == m.current.Name {
					return from.Amount
				}
			}
		}

		return state.ChanceDefault.Or(0)
	}

	return state.Chance.Or(1)
}
