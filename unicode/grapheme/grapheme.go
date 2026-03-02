// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package grapheme implements Unicode grapheme cluster segmentation
// as defined by UAX #29.
package grapheme

import "golang.org/x/text/internal/segmenter"

// trieTable adapts the generated graphemeTrie to the segmenter.PropertyTable
// interface.
type trieTable struct{ t graphemeTrie }

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

	// This optimization is valid for grapheme clusters because all ASCII bytes
	// are in the same property (Other) and have no special break rules except
	// around CR/LF, which are handled by the break state table.
	ASCIIBreak: true,
}

// Segmenter iterates over the grapheme clusters in a byte slice.
// The usage pattern is:
//
//	seg := grapheme.NewSegmenter(input)
//	for seg.Next() {
//	    fmt.Println(seg.Bytes())
//	}
type Segmenter struct {
	s *segmenter.Segmenter
}

// NewSegmenter returns a Segmenter that iterates over the grapheme clusters
// in the given input.
func NewSegmenter(input []byte) *Segmenter {
	return &Segmenter{s: segmenter.New(ruleData, input)}
}

// Next advances to the next grapheme cluster. It returns false when the
// end of input has been reached.
func (g *Segmenter) Next() bool { return g.s.Next() }

// Bytes returns the current grapheme cluster as a byte slice.
func (g *Segmenter) Bytes() []byte { return g.s.Bytes() }

// Text returns the current grapheme cluster as a string.
func (g *Segmenter) Text() string { return g.s.Text() }

// Position returns the byte offsets [start, end) of the current grapheme
// cluster.
func (g *Segmenter) Position() (start, end int) { return g.s.Position() }
