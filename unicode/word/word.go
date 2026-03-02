// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package word implements Unicode word segmentation as defined by UAX #29.
package word

import "golang.org/x/text/internal/segmenter"

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
