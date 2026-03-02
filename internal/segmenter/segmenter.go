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
// Combined states encode multi-character lookahead rules (e.g. RI×RI pairing,
// ExtPict×Extend*×ZWJ×ExtPict) without backtracking.
package segmenter

// BreakState represents a cell in the break state table.
type BreakState int8

const (
	// Break signals a definite break between left and right.
	Break BreakState = -128
	// Keep signals no break; advance right and continue with right as the new left.
	Keep BreakState = -2
	// NoMatch signals that a combined state did not match; break at the
	// saved marker position (rewind).
	NoMatch BreakState = -1
	// Values ≥ 0 are combined-state indices: enter that state as the new left
	// property and advance right without breaking.
)

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

	PropCount   uint8 // number of base properties (before combined states)
	SOT         uint8 // start-of-text property index
	EOT         uint8 // end-of-text property index
	ComplexProp uint8 // SA property index (for dictionary delegation), 0 if none

	ASCIIBreak bool // if true, an ASCII byte (except CR/LF) followed by ASCII or EOF is always a 1-byte segment
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
	pos   int // end of current segment (updated by Next)
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
	if s.pos >= len(s.input) {
		return false
	}

	s.start = s.pos

	if s.data.ASCIIBreak {
		b := s.input[s.pos]
		if b < 0x80 && b != '\r' && b != '\n' {
			if s.pos+1 >= len(s.input) || s.input[s.pos+1] < 0x80 {
				s.pos++
				return true
			}
		}
	}

	var leftProp uint8
	if s.pos == 0 {
		leftProp = s.data.SOT
		rightProp, size := s.lookup(s.input[s.pos:])
		state := s.data.BreakTable[int(leftProp)*s.data.Stride+int(rightProp)]
		s.pos += size
		switch state {
		case Break:
			return true
		case Keep:
			leftProp = rightProp
		default:
			if state >= 0 {
				leftProp = uint8(state)
			} else {
				leftProp = rightProp
			}
		}
	} else {
		var size int
		leftProp, size = s.lookup(s.input[s.pos:])
		s.pos += size
	}

	marker := s.pos
	inCombined := false

	for s.pos < len(s.input) {
		rightProp, size := s.lookup(s.input[s.pos:])
		state := s.data.BreakTable[int(leftProp)*s.data.Stride+int(rightProp)]

		switch state {
		case Break:
			if s.pos == s.start {
				s.pos += size
			}
			return true

		case Keep:
			leftProp = rightProp
			s.pos += size
			inCombined = false
			marker = s.pos

		case NoMatch:
			s.pos = marker
			if s.pos == s.start {
				_, sz := s.lookup(s.input[s.pos:])
				s.pos += sz
			}
			return true

		default: // state >= 0: enter combined state
			if !inCombined {
				marker = s.pos
				inCombined = true
			}
			leftProp = uint8(state)
			s.pos += size
		}
	}

	// End of text — check EOT rule.
	eotState := s.data.BreakTable[int(leftProp)*s.data.Stride+int(s.data.EOT)]
	if eotState == NoMatch {
		s.pos = marker
		if s.pos == s.start {
			s.pos = len(s.input)
		}
	}
	return true
}

// Bytes returns the current segment as a byte slice.
// It is only valid after [Next] returns true.
func (s *Segmenter) Bytes() []byte {
	return s.input[s.start:s.pos]
}

// Text returns the current segment as a string.
// It is only valid after [Next] returns true.
func (s *Segmenter) Text() string {
	return string(s.input[s.start:s.pos])
}

// Position returns the byte offsets [start, end) of the current segment.
// It is only valid after [Next] returns true.
func (s *Segmenter) Position() (start, end int) {
	return s.start, s.pos
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
