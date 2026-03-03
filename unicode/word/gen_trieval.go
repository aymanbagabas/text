// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

// Class is a bitflag type for word break properties.
// Each base property occupies one bit. The zero value represents Other.
type Class uint32

// Base property bitflags for word break.
//
// Each property occupies one bit in a Class value. The zero value
// represents Other (WB=Other) — the default property for codepoints
// with no specific class.
//
// WB4 absorption states, lookahead states, and virtual properties are
// NOT bitflags — they are uint8 indices assigned after flattening,
// used by the state machine.
const (
	Other Class = 0 // WB=Other — zero value, no bits set

	CR           Class = 1 << iota // WB=CR
	LF                             // WB=LF
	Newline                        // WB=Newline
	Extend                         // WB=Extend
	ZWJ                            // WB=ZWJ
	RI                             // WB=Regional_Indicator
	Format                         // WB=Format
	Katakana                       // WB=Katakana
	HebrewLetter                   // WB=Hebrew_Letter
	ALetter                        // WB=ALetter
	SingleQuote                    // WB=Single_Quote
	DoubleQuote                    // WB=Double_Quote
	MidNumLet                      // WB=MidNumLet
	MidLetter                      // WB=MidLetter
	MidNum                         // WB=MidNum
	Numeric                        // WB=Numeric
	ExtendNumLet                   // WB=ExtendNumLet
	ExtPict                        // Extended_Pictographic=Yes
	WSegSpace                      // WB=WSegSpace
)

// allBaseProperties is the OR of all base property bits.
const allBaseProperties = CR | LF | Newline | Extend | ZWJ | Format | RI |
	Katakana | HebrewLetter | ALetter | SingleQuote | DoubleQuote |
	MidLetter | MidNum | MidNumLet | Numeric | ExtendNumLet | ExtPict | WSegSpace

// Natural groupings using OR — replaces ad-hoc p() helpers.
const (
	AHLetter   = ALetter | HebrewLetter
	MidNumLetQ = MidNumLet | SingleQuote
)
