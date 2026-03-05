// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package grapheme implements Unicode grapheme cluster segmentation
// as defined by UAX #29.
package grapheme

import (
	"golang.org/x/text/internal/segmenter"
	"golang.org/x/text/language"
)

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

type options struct {
	locale language.Tag
}

// Option configures a [Segmenter].
type Option func(*options)

// WithLocale sets the locale for locale-tailored segmentation.
func WithLocale(t language.Tag) Option {
	return func(o *options) { o.locale = t }
}

// NewSegmenter returns a Segmenter that iterates over the grapheme clusters
// in the given input.
func NewSegmenter(input []byte, opts ...Option) *Segmenter {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	_ = o
	return &Segmenter{s: segmenter.New(&ruleData, input)}
}

// Next advances to the next grapheme cluster. It returns false when the
// end of input has been reached.
func (g *Segmenter) Next() bool {
	input := g.s.Input()
	pos := g.s.End()
	if pos < len(input) {
		b := input[pos]
		if b < 0x80 && b != '\r' && b != '\n' {
			if pos+1 >= len(input) || input[pos+1] < 0x80 {
				g.s.FastForward(pos+1, 0)
				return true
			}
		}
	}
	return g.s.Next()
}

// Bytes returns the current grapheme cluster as a byte slice.
func (g *Segmenter) Bytes() []byte { return g.s.Bytes() }

// Text returns the current grapheme cluster as a string.
func (g *Segmenter) Text() string { return g.s.Text() }

// Position returns the byte offsets [start, end) of the current grapheme
// cluster.
func (g *Segmenter) Position() (start, end int) { return g.s.Position() }
