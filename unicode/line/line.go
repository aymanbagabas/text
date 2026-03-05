// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package line implements Unicode line break segmentation as defined by UAX #14.
//
// A [Segmenter] iterates over the line break opportunities in a byte slice,
// returning segments between mandatory or allowed break positions.
//
// CSS line-break and word-break properties are supported via [WithStrictness]
// and [WithWordBreak] options.
package line

import (
	"golang.org/x/text/internal/segmenter"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/grapheme"
)

// Strictness controls the strictness of line-breaking rules,
// corresponding to the CSS line-break property.
//
// See https://drafts.csswg.org/css-text-3/#line-break-property.
type Strictness uint8

const (
	// Strict uses the most stringent set of line-breaking rules.
	// This is the default behavior of the Unicode Line Breaking Algorithm,
	// resolving class CJ to NS (no extra break opportunities at
	// conditional Japanese starters).
	Strict Strictness = iota

	// Normal uses the most common set of line-breaking rules.
	// CJ class codepoints are treated as ID (ideographic), allowing
	// breaks before them.
	Normal

	// Loose uses the least restrictive set of line-breaking rules.
	// CJ class codepoints are treated as ID (ideographic), allowing
	// breaks before them.
	Loose

	// Anywhere allows breaks after every typographic character unit,
	// disregarding prohibitions against line breaks. Only mandatory
	// break rules (LB4, LB5) are preserved.
	Anywhere
)

// WordBreak controls line break opportunities between letters,
// corresponding to the CSS word-break property.
//
// See https://drafts.csswg.org/css-text-3/#word-break-property.
type WordBreak uint8

const (
	// WordNormal breaks words according to their customary rules.
	WordNormal WordBreak = iota

	// WordBreakAll treats all characters as having soft wrap
	// opportunities by remapping alphabetic (AL) and complex context
	// (SA) characters to ID (ideographic).
	WordBreakAll

	// WordKeepAll suppresses soft wrap opportunities between
	// typographic letter units by remapping ideographic (ID) and
	// conditional Japanese starter (CJ) characters to AL (alphabetic).
	WordKeepAll
)

// Segmenter iterates over the line break segments in a byte slice.
// The usage pattern is:
//
//	seg := line.NewSegmenter(input)
//	for seg.Next() {
//	    fmt.Println(seg.Bytes())
//	}
type Segmenter struct {
	s        *segmenter.Segmenter
	gs       *grapheme.Segmenter
	anywhere bool
}

type options struct {
	locale     language.Tag
	strictness Strictness
	wordBreak  WordBreak
}

// Option configures a [Segmenter].
type Option func(*options)

// WithLocale sets the locale for locale-tailored segmentation.
func WithLocale(t language.Tag) Option {
	return func(o *options) { o.locale = t }
}

// WithStrictness sets the CSS line-break strictness level.
// The default is [Strict].
func WithStrictness(s Strictness) Option {
	return func(o *options) { o.strictness = s }
}

// WithWordBreak sets the CSS word-break behavior.
// The default is [WordNormal].
func WithWordBreak(wb WordBreak) Option {
	return func(o *options) { o.wordBreak = wb }
}

// NewSegmenter returns a Segmenter that iterates over the line break
// segments in the given input.
func NewSegmenter(input []byte, opts ...Option) *Segmenter {
	var o options
	for _, fn := range opts {
		fn(&o)
	}

	seg := segmenter.New(&ruleData, input)

	if override := buildOverride(o.strictness, o.wordBreak); override != nil {
		seg.SetOverrideLookup(override)
	}

	l := &Segmenter{s: seg, anywhere: o.strictness == Anywhere}
	if l.anywhere {
		l.gs = grapheme.NewSegmenter(input)
	}
	return l
}

// buildOverride returns a composed property override function for the given
// CSS settings, or nil if no overrides are needed (Strict + WordNormal).
func buildOverride(strictness Strictness, wb WordBreak) func(uint8, rune) uint8 {
	needStrictness := strictness == Normal || strictness == Loose
	needWordBreak := wb != WordNormal

	if !needStrictness && !needWordBreak {
		return nil
	}

	return func(prop uint8, r rune) uint8 {
		if needStrictness {
			if prop == CJ {
				prop = ID
			}
		}

		switch wb {
		case WordBreakAll:
			if prop == AL || prop == AI || prop == SA {
				prop = ID
			}
		case WordKeepAll:
			if prop == ID || prop == ID_ExtPict || prop == CJ {
				prop = AL
			}
		}

		return prop
	}
}

// Next advances to the next line break segment. It returns false when the
// end of input has been reached.
func (l *Segmenter) Next() bool {
	if l.anywhere {
		return l.nextAnywhere()
	}
	return l.s.Next()
}

// nextAnywhere implements CSS line-break: anywhere by breaking after every
// extended grapheme cluster (typographic character unit per CSS Text 3).
func (l *Segmenter) nextAnywhere() bool {
	return l.gs.Next()
}

// Bytes returns the current segment as a byte slice.
func (l *Segmenter) Bytes() []byte {
	if l.anywhere {
		return l.gs.Bytes()
	}
	return l.s.Bytes()
}

// Text returns the current segment as a string.
func (l *Segmenter) Text() string {
	if l.anywhere {
		return l.gs.Text()
	}
	return l.s.Text()
}

// Position returns the byte offsets [start, end) of the current segment.
func (l *Segmenter) Position() (start, end int) {
	if l.anywhere {
		return l.gs.Position()
	}
	return l.s.Position()
}

// MustBreak returns whether there is a mandatory break at the current
// position. This is true for hard line breaks such as U+000A (LF) and U+000D
// (CR), but not for soft line breaks such as spaces.
func (l *Segmenter) MustBreak() bool {
	var p uint8
	if l.anywhere {
		b := l.gs.Bytes()
		if len(b) > 0 {
			p, _ = ruleData.PropertyLookup(b)
		}
	} else {
		p = l.s.BoundaryProperty()
	}
	return p == uint8(BK) || p == uint8(CR) || p == uint8(LF) || p == uint8(NL)
}
