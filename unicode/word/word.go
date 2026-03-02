// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package word implements Unicode word segmentation as defined by UAX #29.
package word

import "golang.org/x/text/internal/segmenter"

// WordType classifies a word segment.
type WordType uint8

// Word types as defined by UAX #29, with WordNone representing non-word-like
// segments such as whitespace and punctuation.
const (
	WordNone   WordType = iota // not word-like (whitespace, punctuation, etc.)
	WordNumber                 // numeric segment
	WordLetter                 // word segment (letters, CJK ideographs, etc.)
)

// IsWordLike reports whether t represents a word-like segment (Letter or Number).
func (t WordType) IsWordLike() bool { return t != WordNone }

// wordTypeTable maps base and absorption property indices to WordType.
// Only base (0–19) and absorption (20–28) indices appear here. Lookahead
// states (> lastCodepointProperty) never reach BoundaryProperty because
// the engine resolves them via NoMatch/rewind before reporting a break.
var wordTypeTable = func() [propCount]WordType {
	var t [propCount]WordType
	for i := range t {
		switch uint8(i) {
		case pALetter, pHebrewLetter, pKatakana, pExtendNumLet,
			pALetter_ZWJ, pHebrewLetter_ZWJ, pKatakana_ZWJ, pExtendNumLet_ZWJ:
			t[i] = WordLetter
		case pNumeric, pNumeric_ZWJ:
			t[i] = WordNumber
		}
	}
	return t
}()

// trieTable adapts the generated wordTrie to the segmenter.PropertyTable
// interface.
type trieTable struct{ t wordTrie }

func (tt *trieTable) Lookup(b []byte) (uint8, int) { return tt.t.lookup(b) }

var ruleData = &segmenter.RuleData{
	Properties: &trieTable{},
	BreakTable: func() []segmenter.BreakState {
		t := make([]segmenter.BreakState, len(breakTable))
		for i, v := range breakTable {
			t[i] = segmenter.BreakState(v)
		}
		return t
	}(),
	Stride:                stride,
	PropCount:             propCount,
	LastCodepointProperty: lastCodepointProperty,
	SOT:                   pSOT,
	EOT:                   pEOT,
}

// Segmenter iterates over the words in a byte slice.
// The usage pattern is:
//
//	seg := word.NewSegmenter(input)
//	for seg.Next() {
//	    fmt.Println(seg.Bytes())
//	}
type Segmenter struct {
	s *segmenter.Segmenter
}

// NewSegmenter returns a Segmenter that iterates over the words
// in the given input.
func NewSegmenter(input []byte) *Segmenter {
	return &Segmenter{s: segmenter.New(ruleData, input)}
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isUnsafeAfterAlphaNum(b byte) bool {
	switch b {
	case '\'', '"', ',', '.', ':', ';', '_':
		return true
	}
	return isAlphaNum(b)
}

// Next advances to the next word boundary segment. It returns false when the
// end of input has been reached.
func (w *Segmenter) Next() bool {
	input := w.s.Input()
	pos := w.s.End()
	if pos >= len(input) {
		return false
	}

	// ASCII fast path: consume an [a-zA-Z0-9]+ run in a tight loop when
	// followed by EOF or a safe ASCII break (not apostrophe, period, comma,
	// underscore, etc. which need the full state machine for lookahead).
	b := input[pos]
	if b < 0x80 && isAlphaNum(b) {
		end := pos + 1
		for end < len(input) && isAlphaNum(input[end]) {
			end++
		}
		if end >= len(input) || (input[end] < 0x80 && !isUnsafeAfterAlphaNum(input[end])) {
			prop := pALetter
			if input[end-1] >= '0' && input[end-1] <= '9' {
				prop = pNumeric
			}
			w.s.FastForward(end, prop)
			return true
		}
	}
	return w.s.Next()
}

// Bytes returns the current segment as a byte slice.
func (w *Segmenter) Bytes() []byte { return w.s.Bytes() }

// Text returns the current segment as a string.
func (w *Segmenter) Text() string { return w.s.Text() }

// Position returns the byte offsets [start, end) of the current segment.
func (w *Segmenter) Position() (start, end int) { return w.s.Position() }

// WordType returns the classification of the current segment.
func (w *Segmenter) WordType() WordType {
	return wordTypeTable[w.s.BoundaryProperty()]
}

// IsWordLike reports whether the current segment is word-like (letter or number).
func (w *Segmenter) IsWordLike() bool { return w.WordType().IsWordLike() }
