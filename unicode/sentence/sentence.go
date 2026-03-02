// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sentence implements Unicode sentence segmentation as defined by UAX #29.
package sentence

import "golang.org/x/text/internal/segmenter"

// trieTable adapts the generated sentenceTrie to the segmenter.PropertyTable
// interface.
type trieTable struct{ t sentenceTrie }

func (tt *trieTable) Lookup(b []byte) (uint8, int) { return tt.t.lookup(b) }

var ruleData = &segmenter.RuleData{
	Properties:            &trieTable{},
	BreakTable:            breakTable[:],
	Stride:                stride,
	PropCount:             propCount,
	LastCodepointProperty: lastCodepointProperty,
	SOT:                   pSOT,
	EOT:                   pEOT,
}

// Segmenter iterates over the sentences in a byte slice.
// The usage pattern is:
//
//	seg := sentence.NewSegmenter(input)
//	for seg.Next() {
//	    fmt.Println(seg.Bytes())
//	}
type Segmenter struct {
	s *segmenter.Segmenter
}

// NewSegmenter returns a Segmenter that iterates over the sentences
// in the given input.
func NewSegmenter(input []byte) *Segmenter {
	return &Segmenter{s: segmenter.New(ruleData, input)}
}

// isSafeASCII reports whether b is an ASCII byte that never participates in
// sentence break rules: letters, digits, and space. All other ASCII bytes
// (punctuation, control characters) may be terminators, closers, paragraph
// separators, or other rule-relevant properties.
func isSafeASCII(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == ' '
}

// asciiProp returns the sentence break property for a safe ASCII byte.
func asciiProp(b byte) uint8 {
	if b >= 'a' && b <= 'z' {
		return pLower
	}
	if b >= 'A' && b <= 'Z' {
		return pUpper
	}
	if b >= '0' && b <= '9' {
		return pNumeric
	}
	return pSp // space
}

// Next advances to the next sentence boundary segment. It returns false when
// the end of input has been reached.
func (se *Segmenter) Next() bool {
	input := se.s.Input()
	pos := se.s.End()
	if pos >= len(input) {
		return false
	}

	// ASCII fast path: scan past contiguous safe ASCII bytes ([a-zA-Z0-9 ]).
	// These never trigger sentence breaks between each other.
	end := pos
	for end < len(input) && isSafeASCII(input[end]) {
		end++
	}

	if end >= len(input) {
		// Entire remaining input is safe ASCII — one sentence.
		se.s.FastForward(end, asciiProp(input[end-1]))
		return true
	}

	// Back up one byte so the engine has correct leftProp context. The
	// engine's Next will read the backed-up byte via trie lookup (a single
	// array index for ASCII).
	if end > pos {
		se.s.SetEnd(end - 1)
	}

	ok := se.s.Next()
	if ok && end > pos {
		se.s.SetStart(pos)
	}
	return ok
}

// Bytes returns the current sentence as a byte slice.
func (se *Segmenter) Bytes() []byte { return se.s.Bytes() }

// Text returns the current sentence as a string.
func (se *Segmenter) Text() string { return se.s.Text() }

// Position returns the byte offsets [start, end) of the current sentence.
func (se *Segmenter) Position() (start, end int) { return se.s.Position() }
