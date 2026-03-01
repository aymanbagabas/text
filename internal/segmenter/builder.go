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
	Left  uint8
	Right uint8
	State uint8
}

// BuildStateTable compiles rules and combined states into a flat N×N
// break state table. The stride is the total property count (base +
// combined + SOT + EOT). Rules are applied in order; the first match wins.
// Combined states overlay the rule results with combined-state indices.
func BuildStateTable(rules []Rule, combined []CombinedState, stride int) []BreakState {
	n := stride
	table := make([]BreakState, n*n)

	// Initialize all cells to Break (default: GB999 / WB999 / etc.).
	for i := range table {
		table[i] = Break
	}

	// Apply rules in reverse priority order so that earlier rules overwrite later ones.
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

	// Apply combined state transitions. These override the rule-derived
	// value with a combined-state index (≥ 0).
	for _, cs := range combined {
		table[int(cs.Left)*n+int(cs.Right)] = BreakState(cs.State)
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
