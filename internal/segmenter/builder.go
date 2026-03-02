// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

// Rule describes a single segmentation rule in priority order.
// The first matching rule wins.
type Rule struct {
	Left  []uint8 // set of left properties (nil means any)
	Right []uint8 // set of right properties (nil means any)
	Break bool    // true = break, false = no break
}

// CombinedState describes a transition into a combined (synthetic) state.
// Combined states override the normal rule outcome: when left and right match,
// the segmenter enters State as the new left property instead of applying
// the rule table directly.
type CombinedState struct {
	Left   uint8
	Right  uint8
	State  uint8
	Interm bool // if true, emit MakeIntermediate instead of MakeIndex (LB15b only)
}

// BuildStateTable compiles rules and combined states into a flat N×N
// break state table in row-major order.
//
// Rules are applied in priority order; the first match wins.
// Combined states overlay the rule results.
//
// Combined states with Interm=false are emitted as [IndexState].
// Combined states with Interm=true are emitted as [IntermediateState].
//
// Marker movement is controlled at runtime by [RuleData.LastCodepointProperty],
// not by Index vs Intermediate. The caller must order combined state indices:
//   - Absorption states have indices ≤ LastCodepointProperty
//   - Lookahead states have indices > LastCodepointProperty
//
// Intermediate vs Index controls what happens on NoMatch:
//   - Index: rewind to the marker (standard lookahead)
//   - Intermediate: rewind point always advances (LB15b semantics)
func BuildStateTable(rules []Rule, combined []CombinedState, stride int) []BreakState {
	n := stride
	table := make([]BreakState, n*n)

	for i := range table {
		table[i] = Break
	}

	for i := len(rules) - 1; i >= 0; i-- {
		r := &rules[i]
		lefts := r.Left
		rights := r.Right
		if lefts == nil {
			lefts = allProps(n)
		}
		if rights == nil {
			rights = allProps(n)
		}
		var state BreakState
		if r.Break {
			state = Break
		} else {
			state = Keep
		}
		for _, l := range lefts {
			for _, ri := range rights {
				table[int(l)*n+int(ri)] = state
			}
		}
	}

	for _, cs := range combined {
		if cs.Interm {
			table[int(cs.Left)*n+int(cs.Right)] = IntermediateState(cs.State)
		} else {
			table[int(cs.Left)*n+int(cs.Right)] = IndexState(cs.State)
		}
	}

	return table
}

func allProps(n int) []uint8 {
	p := make([]uint8, n)
	for i := range p {
		p[i] = uint8(i)
	}
	return p
}
