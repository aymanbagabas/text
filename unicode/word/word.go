// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package word implements Unicode word segmentation as defined by UAX #29.
package word

import (
	"golang.org/x/text/internal/segmenter"
	"golang.org/x/text/language"
	"unicode/utf8"
)

// WordType classifies a word segment.
type WordType uint8

const (
	WordNone   WordType = iota // not word-like (whitespace, punctuation, etc.)
	WordNumber                 // numeric segment
	WordLetter                 // word segment (letters, CJK ideographs, etc.)
)

// IsWordLike reports whether t represents a word-like segment (Letter or Number).
func IsWordLike(t WordType) bool { return t != WordNone }

// wordTypeTable maps simple property indices to WordType.
// Indices correspond to the Property constants in trieval.go.
var wordTypeTable = [...]WordType{
	Katakana:      WordLetter,
	Hebrew_Letter: WordLetter,
	ALetter:       WordLetter,
	Numeric:       WordNumber,
	ExtendNumLet:  WordLetter,
	SA:            WordLetter,
}

// Segmenter iterates over the words in a byte slice.
type Segmenter struct {
	s *segmenter.Segmenter
}

type options struct {
	locale language.Tag
}

// Option configures a [Segmenter].
type Option func(*options)

// WithLocale sets the locale for locale-tailored segmentation.
// Supported locales: Finnish (fi), Swedish (sv).
func WithLocale(t language.Tag) Option {
	return func(o *options) { o.locale = t }
}

// finnishOverride remaps colon characters from MidLetter to Other for
// Finnish and Swedish word segmentation. In standard UAX #29, colon is
// MidLetter, so "EU:ssa" is one word. Finnish/Swedish treat colon as a
// word break.
func finnishOverride(input []byte) (uint8, int) {
	r, sz := utf8.DecodeRune(input)
	switch r {
	case ':', '\uFE55', '\uFF1A':
		return Other, sz
	}
	return 0, -1
}

// NewSegmenter returns a Segmenter that iterates over the words
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
		case language.MustParseBase("fi"), language.MustParseBase("sv"):
			seg.SetOverrideLookup(finnishOverride)
		}
	}
	return &Segmenter{s: seg}
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

	b := input[pos]
	if b < 0x80 && isAlphaNum(b) {
		end := pos + 1
		for end < len(input) && isAlphaNum(input[end]) {
			end++
		}
		if end >= len(input) || (input[end] < 0x80 && !isUnsafeAfterAlphaNum(input[end])) {
			prop := ALetter
			if input[end-1] >= '0' && input[end-1] <= '9' {
				prop = Numeric
			}
			w.s.FastForward(end, uint8(prop))
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
	p := w.s.BoundaryProperty()
	if int(p) < len(wordTypeTable) {
		return wordTypeTable[p]
	}
	return WordNone
}

// IsWordLike reports whether the current segment is word-like (letter or number).
func (w *Segmenter) IsWordLike() bool { return IsWordLike(w.WordType()) }
