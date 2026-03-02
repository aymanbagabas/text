// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package segmenter implements a state machine engine for Unicode text
// segmentation (UAX #29 and UAX #14). It is shared by the grapheme, word,
// sentence, and line break packages.
//
// The engine is data-driven: each segmenter type supplies a [RuleData]
// containing property lookup tables and a pre-built break state table.
// The table is an N×N matrix (left-property × right-property → action),
// where actions are break, keep, no-match (rewind), or enter-combined-state.
//
// Combined states come in two flavours:
//   - Index states (the default) enter a combined state. Marker movement
//     is controlled by [RuleData.LastCodepointProperty]: the marker only
//     advances when the previous state index ≤ LastCodepointProperty.
//   - Intermediate states (bit 0x40 set) always advance the rewind point,
//     regardless of LastCodepointProperty. Used for LB15b in line break.
package segmenter

// BreakState is the element type of a break state table cell.
// It is a type alias so that []int8 (the generated table type) and
// []BreakState are interchangeable, eliminating init-time copies.
type BreakState = int8

const (
	// Break signals a definite break between left and right.
	Break BreakState = -128
	// Keep signals no break; advance right and continue with right as the new left.
	Keep BreakState = -2
	// NoMatch signals that a combined state did not match; break at the
	// saved marker position (rewind).
	NoMatch BreakState = -1

	// Values 0–63 are Index combined states.
	// Values 64–127 are Intermediate combined states (LB15b only).

	intermediateBit BreakState = 0x40
)

// isIntermediate reports whether state is an Intermediate combined state.
func isIntermediate(s int8) bool {
	return s >= 0 && s&intermediateBit != 0
}

// stateIndex extracts the combined-state property index from a combined
// state (either Index or Intermediate). The caller must ensure s >= 0.
func stateIndex(s int8) uint8 {
	return uint8(s &^ intermediateBit)
}

// IndexState returns the BreakState encoding for an Index combined
// state with the given property index.
func IndexState(prop uint8) BreakState { return BreakState(prop) }

// IntermediateState returns the BreakState encoding for an Intermediate
// combined state with the given property index. Used for LB15b.
func IntermediateState(prop uint8) BreakState { return BreakState(prop) | intermediateBit }

// PropertyTable abstracts the trie lookup for codepoint → property index.
type PropertyTable interface {
	Lookup(b []byte) (prop uint8, size int)
}

// RuleData holds the generated tables for one segmenter type.
type RuleData struct {
	// Properties is the base property trie.
	Properties PropertyTable

	// Override is an optional locale/CSS override trie. When non-nil, it is
	// checked first; a non-zero return value overrides the base property.
	Override PropertyTable

	// BreakTable is the N×N state table in row-major order.
	// BreakTable[left*Stride + right] gives the action for (left, right).
	BreakTable []BreakState
	Stride     int // number of columns (= total property count)

	PropCount uint8 // number of base properties (before combined states)

	// LastCodepointProperty is the highest index that counts as a
	// "codepoint property" for marker movement purposes. Indices
	// 0..LastCodepointProperty are base properties + absorption combined
	// states. Indices above are lookahead combined states.
	//
	// When entering a combined state via Index encoding, the walker
	// moves the marker only if the PREVIOUS left property index was
	// ≤ LastCodepointProperty. This means absorption states (which map
	// back to base indices or _ZWJ indices, all ≤ LastCodepointProperty)
	// advance the marker, while lookahead states (> LastCodepointProperty)
	// do not.
	LastCodepointProperty uint8

	SOT         uint8 // start-of-text property index
	EOT         uint8 // end-of-text property index
	ComplexProp uint8 // SA property index (for dictionary delegation), 0 if none
}

// ComplexHandler segments runs of complex-script text (SA property).
// This interface is defined here as a shared contract — the engine itself
// does NOT use it. The per-package segmenters (word, line) that wrap the
// engine are responsible for detecting SA runs and delegating to the
// appropriate ComplexHandler.
type ComplexHandler interface {
	Segment(input []byte) []int
}

// Segmenter iterates over segments in text. The iteration pattern is:
//
//	seg := segmenter.New(data, input)
//	for seg.Next() {
//	    seg.Bytes()  // current segment
//	}
type Segmenter struct {
	data  *RuleData
	input []byte
	start int // start of current segment
	end   int // end of current segment (updated by Next)

	boundaryProp uint8 // property of the left side at the break point
}

// New returns a Segmenter that iterates over segments in input
// according to data.
func New(data *RuleData, input []byte) *Segmenter {
	return &Segmenter{data: data, input: input}
}

// Next advances to the next segment. It returns false when the end of
// input has been reached. After Next returns true, [Bytes], [Text], and
// [Position] describe the current segment.
func (s *Segmenter) Next() bool {
	if s.end >= len(s.input) {
		return false
	}

	s.start = s.end

	var leftProp uint8
	if s.end == 0 {
		leftProp = s.data.SOT
		rightProp, size := s.lookup(s.input[s.end:])
		state := s.data.BreakTable[int(leftProp)*s.data.Stride+int(rightProp)]
		s.end += size
		switch state {
		case Break:
			s.boundaryProp = leftProp
			return true
		case Keep:
			leftProp = rightProp
		default:
			if state >= 0 {
				leftProp = stateIndex(state)
			} else {
				leftProp = rightProp
			}
		}
	} else {
		var size int
		leftProp, size = s.lookup(s.input[s.end:])
		s.end += size
	}

	marker := s.end
	markerLeftProp := leftProp

	for s.end < len(s.input) {
		rightProp, size := s.lookup(s.input[s.end:])
		state := s.data.BreakTable[int(leftProp)*s.data.Stride+int(rightProp)]

		switch state {
		case Break:
			if s.end == s.start {
				s.end += size
			}
			s.boundaryProp = leftProp
			return true

		case Keep:
			leftProp = rightProp
			s.end += size
			marker = s.end
			markerLeftProp = rightProp

		case NoMatch:
			s.end = marker
			s.boundaryProp = markerLeftProp
			if s.end == s.start {
				_, sz := s.lookup(s.input[s.end:])
				s.end += sz
			}
			return true

		default: // state >= 0: enter combined state
			idx := stateIndex(state)
			if isIntermediate(state) {
				marker = s.end + size
				if leftProp <= s.data.LastCodepointProperty {
					markerLeftProp = idx
				}
			} else {
				if leftProp <= s.data.LastCodepointProperty {
					marker = s.end
					markerLeftProp = idx
				}
			}
			leftProp = idx
			s.end += size
		}
	}

	// End of text — check EOT rule.
	eotState := s.data.BreakTable[int(leftProp)*s.data.Stride+int(s.data.EOT)]
	if eotState == NoMatch {
		s.boundaryProp = markerLeftProp
		s.end = marker
		if s.end == s.start {
			s.end = len(s.input)
		}
	} else {
		s.boundaryProp = leftProp
	}
	return true
}

// Bytes returns the current segment as a byte slice.
// It is only valid after [Next] returns true.
func (s *Segmenter) Bytes() []byte {
	return s.input[s.start:s.end]
}

// Text returns the current segment as a string.
// It is only valid after [Next] returns true.
func (s *Segmenter) Text() string {
	return string(s.input[s.start:s.end])
}

// Position returns the byte offsets [start, end) of the current segment.
// It is only valid after [Next] returns true.
func (s *Segmenter) Position() (start, end int) {
	return s.start, s.end
}

// BoundaryProperty returns the property index of the left side at the break
// point, after [Next] returns true. This is used by the word segmenter to
// derive WordType (letter, number, or none). The engine tracks this
// generically; interpretation is up to the per-package segmenter.
func (s *Segmenter) BoundaryProperty() uint8 {
	return s.boundaryProp
}

// End returns the end position of the last segment (= start of the next).
func (s *Segmenter) End() int { return s.end }

// SetEnd sets the end position. Used by wrapper-level fast paths that
// scan ahead through known-safe bytes before calling [Next].
func (s *Segmenter) SetEnd(pos int) { s.end = pos }

// SetStart sets the start position of the current segment. Used by
// wrapper-level fast paths to fix up the segment start after [Next]
// when bytes were skipped via [SetEnd] before the call.
func (s *Segmenter) SetStart(pos int) { s.start = pos }

// Input returns the input byte slice.
func (s *Segmenter) Input() []byte { return s.input }

// FastForward sets the current segment to [pos, end) with the given boundary
// property, without running the state machine. Used by per-package fast paths
// (e.g., word's ASCII fast path) that can determine the segment boundary
// without Unicode property lookups.
func (s *Segmenter) FastForward(end int, prop uint8) {
	s.start = s.end
	s.end = end
	s.boundaryProp = prop
}

// lookup resolves a codepoint's property, checking the override trie first.
func (s *Segmenter) lookup(b []byte) (prop uint8, size int) {
	if s.data.Override != nil {
		prop, size = s.data.Override.Lookup(b)
		if prop != 0 {
			return prop, size
		}
	}
	return s.data.Properties.Lookup(b)
}
