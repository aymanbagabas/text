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

// Next advances to the next sentence boundary segment. It returns false when
// the end of input has been reached.
func (se *Segmenter) Next() bool { return se.s.Next() }

// Bytes returns the current sentence as a byte slice.
func (se *Segmenter) Bytes() []byte { return se.s.Bytes() }

// Text returns the current sentence as a string.
func (se *Segmenter) Text() string { return se.s.Text() }

// Position returns the byte offsets [start, end) of the current sentence.
func (se *Segmenter) Position() (start, end int) { return se.s.Position() }
