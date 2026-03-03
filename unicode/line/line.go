// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package line implements Unicode line break segmentation as defined by UAX #14.
//
// A [Segmenter] iterates over the line break opportunities in a byte slice,
// returning segments between mandatory or allowed break positions.
package line

import "golang.org/x/text/internal/segmenter"

// Segmenter iterates over the line break segments in a byte slice.
// The usage pattern is:
//
//	seg := line.NewSegmenter(input)
//	for seg.Next() {
//	    fmt.Println(seg.Bytes())
//	}
type Segmenter struct {
	s *segmenter.Segmenter
}

// NewSegmenter returns a Segmenter that iterates over the line break
// segments in the given input.
func NewSegmenter(input []byte) *Segmenter {
	return &Segmenter{s: segmenter.New(&ruleData, input)}
}

// Next advances to the next line break segment. It returns false when the
// end of input has been reached.
func (l *Segmenter) Next() bool {
	return l.s.Next()
}

// Bytes returns the current segment as a byte slice.
func (l *Segmenter) Bytes() []byte { return l.s.Bytes() }

// Text returns the current segment as a string.
func (l *Segmenter) Text() string { return l.s.Text() }

// Position returns the byte offsets [start, end) of the current segment.
func (l *Segmenter) Position() (start, end int) { return l.s.Position() }

// MustBreak returns whether there is a mandatory break at the current
// position. This is true for hard line breaks such as U+000A (LF) and U+000D
// (CR), but not for soft line breaks such as spaces.
func (l *Segmenter) MustBreak() bool {
	p := l.s.BoundaryProperty()
	return p == uint8(BK) || p == uint8(CR) || p == uint8(LF) || p == uint8(NL)
}
