// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

// Expandable is implemented by rule types that can expand into
// low-level CombinedState entries for BuildStateTable.
type Expandable interface {
	Expand() []CombinedState
}

// IgnoreRule describes SB5/WB4-style absorption where certain properties
// are transparent between meaningful characters. For each property in
// Props and each property in Ignored, the expansion produces:
//
//	Props[i] × Ignored[j] → Target(Props[i], Ignored[j])
//
// Target is a caller-supplied function that maps (base, ignored) to the
// target combined state index. For example, in word break, ALetter
// absorbing Extend maps to ALetter (itself), while ALetter absorbing
// ZWJ maps to ALetter_ZWJ (a different index).
type IgnoreRule struct {
	// Props lists the properties that absorb ignored characters.
	Props []uint8
	// Ignored lists the properties to absorb (e.g., Extend, Format, ZWJ).
	Ignored []uint8
	// Target maps (base property, ignored property) → target combined state.
	Target func(base, ignored uint8) uint8
	// Interm, if true, emits Intermediate combined states.
	Interm bool
}

func (r IgnoreRule) Expand() []CombinedState {
	var cs []CombinedState
	for _, base := range r.Props {
		for _, ign := range r.Ignored {
			target := r.Target(base, ign)
			cs = append(cs, CombinedState{
				Left:   base,
				Right:  ign,
				State:  target,
				Interm: r.Interm,
			})
		}
	}
	return cs
}

// ChainStep describes one position in a multi-character chain pattern.
type ChainStep struct {
	// Props lists the acceptable properties at this position.
	Props []uint8
	// State is the combined-state index assigned to this step.
	State uint8
}

// ChainRule describes a multi-character lookahead pattern such as:
//
//	AHLetter × (MidLetter|MidNumLetQ) × AHLetter
//
// Entry lists the left-side properties that start the chain. Steps
// describes intermediate positions; each step's Props are right-side
// properties that advance the chain into the step's State. The final
// right-hand match is described by a completing Rule in the rules list,
// not by ChainRule.
//
// SelfLoop, if set, adds transparency transitions within each chain
// state (e.g., Extend/Format/ZWJ for WB4).
type ChainRule struct {
	Entry    []uint8
	Steps    []ChainStep
	SelfLoop []uint8
	Interm   bool
}

func (r ChainRule) Expand() []CombinedState {
	var cs []CombinedState

	for i, step := range r.Steps {
		var lefts []uint8
		if i == 0 {
			lefts = r.Entry
		} else {
			lefts = []uint8{r.Steps[i-1].State}
		}

		for _, l := range lefts {
			for _, right := range step.Props {
				cs = append(cs, CombinedState{
					Left:   l,
					Right:  right,
					State:  step.State,
					Interm: r.Interm,
				})
			}
		}

		for _, sl := range r.SelfLoop {
			cs = append(cs, CombinedState{
				Left:   step.State,
				Right:  sl,
				State:  step.State,
				Interm: r.Interm,
			})
		}
	}

	return cs
}

// ExpandAll expands a slice of Expandable rules into CombinedState entries.
func ExpandAll(rules ...Expandable) []CombinedState {
	var cs []CombinedState
	for _, r := range rules {
		cs = append(cs, r.Expand()...)
	}
	return cs
}

// Compile interface assertions.
var (
	_ Expandable = IgnoreRule{}
	_ Expandable = ChainRule{}
)
