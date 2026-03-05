// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sentence implements Unicode sentence segmentation as defined by UAX #29.
package sentence

import (
	"golang.org/x/text/internal/segmenter"
	"golang.org/x/text/language"
	"unicode/utf8"
)

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

type options struct {
	locale language.Tag
}

// Option configures a [Segmenter].
type Option func(*options)

// WithLocale sets the locale for locale-tailored segmentation.
// Supported locales: Greek (el).
func WithLocale(t language.Tag) Option {
	return func(o *options) { o.locale = t }
}

// greekOverride remaps U+003B (semicolon) and U+037E (Greek question mark)
// to STerm for Greek sentence segmentation. In standard UAX #29 these are
// Other; Greek uses them as sentence terminators.
func greekOverride(input []byte) (uint8, int) {
	r, sz := utf8.DecodeRune(input)
	switch r {
	case ';', '\u037E':
		return STerm, sz
	}
	return 0, -1
}

// NewSegmenter returns a Segmenter that iterates over the sentences
// in the given input.
func NewSegmenter(input []byte, opts ...Option) *Segmenter {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	seg := segmenter.New(&ruleData, input)
	if o.locale != (language.Tag{}) {
		base, _ := o.locale.Base()
		switch base {
		case language.MustParseBase("el"):
			seg.SetOverrideLookup(greekOverride)
		}
	}
	return &Segmenter{s: seg}
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
		return uint8(Lower)
	}
	if b >= 'A' && b <= 'Z' {
		return uint8(Upper)
	}
	if b >= '0' && b <= '9' {
		return uint8(Numeric)
	}
	return uint8(Sp)
}

// Next advances to the next sentence boundary segment. It returns false when
// the end of input has been reached.
func (se *Segmenter) Next() bool {
	input := se.s.Input()
	pos := se.s.End()
	if pos >= len(input) {
		return false
	}

	end := pos
	for end < len(input) && isSafeASCII(input[end]) {
		end++
	}

	if end >= len(input) {
		se.s.FastForward(end, asciiProp(input[end-1]))
		return true
	}

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
