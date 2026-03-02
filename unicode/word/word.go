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
// Lookahead states (> lastCodepointProperty) are not indexed because the
// engine resolves them before reporting a boundary property.
var wordTypeTable = [propCount]WordType{
	pALetter:      WordLetter,
	pHebrewLetter: WordLetter,
	pKatakana:     WordLetter,
	pExtendNumLet: WordLetter,
	pNumeric:      WordNumber,
	pExtPict:      WordNone,

	pALetter_ZWJ:      WordLetter,
	pHebrewLetter_ZWJ: WordLetter,
	pKatakana_ZWJ:     WordLetter,
	pExtendNumLet_ZWJ: WordLetter,
	pNumeric_ZWJ:      WordNumber,
	pExtPict_ZWJ:      WordNone,

	pAHL_MidLetter: WordLetter,
	pHL_MidLetter:  WordLetter,
	pNum_MidNum:    WordNumber,
	pHL_DQ:         WordLetter,
	pRI_RI:         WordNone,
}

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
	ASCIIBreak:            false,
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

// Next advances to the next word boundary segment. It returns false when the
// end of input has been reached.
func (w *Segmenter) Next() bool { return w.s.Next() }

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
