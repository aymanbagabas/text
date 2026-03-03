// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

// Class is a bitflag type for line break properties.
// Each base property occupies one bit. The zero value represents XX (Unknown).
type Class uint64

// Base property bitflags for line break (UAX #14).
//
// Each property occupies one bit in a Class value. The zero value
// represents XX (LB=XX) — the default property for codepoints
// with no specific class.
//
// The order follows Table 1 of UAX #14 (Unicode Line Breaking Algorithm).
// Properties resolved at parse time (CM→Extend, SG→XX, AI→AL, HH→HY) are
// omitted — they are handled in lbMap. CJ is included because it needs a
// distinct index for the CJ→NS resolution in Step 5.
//
// LB9 absorption states, chain states, and virtual properties are
// NOT bitflags — they are uint8 indices assigned after flattening,
// used by the state machine.
const (
	XX Class = 0 // LB=XX (Unknown / default) — zero value, no bits set

	// Non-tailorable line breaking classes.
	BK Class = 1 << iota // LB=BK (Mandatory Break)
	CR                   // LB=CR (Carriage Return)
	LF                   // LB=LF (Line Feed)
	NL                   // LB=NL (Next Line)
	WJ                   // LB=WJ (Word Joiner)
	ZW                   // LB=ZW (Zero Width Space)
	GL                   // LB=GL (Non-breaking / Glue)
	SP                   // LB=SP (Space)

	// Break opportunities.
	B2 // LB=B2 (Break Opportunity Before and After)
	BA // LB=BA (Break After)
	BB // LB=BB (Break Before)
	HY // LB=HY (Hyphen)
	CB // LB=CB (Contingent Break)

	// Characters prohibiting certain breaks.
	CL // LB=CL (Close Punctuation)
	CP // LB=CP (Close Parenthesis)
	EX // LB=EX (Exclamation/Interrogation)
	IN // LB=IN (Inseparable)
	NS // LB=NS (Nonstarter)
	OP // LB=OP (Open Punctuation)
	QU // LB=QU (Quotation)

	// Numeric context.
	IS // LB=IS (Infix Numeric Separator)
	NU // LB=NU (Numeric)
	PO // LB=PO (Postfix Numeric)
	PR // LB=PR (Prefix Numeric)
	SY // LB=SY (Symbols Allowing Break After)

	// Other characters.
	AK // LB=AK (Aksara)
	AL // LB=AL (Alphabetic)
	AP // LB=AP (Aksara Pre-base)
	AS // LB=AS (Aksara Start)
	CJ // LB=CJ (Conditional Japanese Starter, resolved to NS or ID)
	EB // LB=EB (Emoji Base)
	EM // LB=EM (Emoji Modifier)
	H2 // LB=H2 (Hangul LV Syllable)
	H3 // LB=H3 (Hangul LVT Syllable)
	HL // LB=HL (Hebrew Letter)
	ID // LB=ID (Ideographic)
	JL // LB=JL (Hangul L Jamo)
	JV // LB=JV (Hangul V Jamo)
	JT // LB=JT (Hangul T Jamo)
	RI // LB=RI (Regional Indicator)
	SA // LB=SA (Complex Context / South Asian)
	VF // LB=VF (Virama Final)
	VI // LB=VI (Virama)

	// Orthogonal trait flags. These are not UCD Line_Break values;
	// they are combined with base properties at parse time.
	EastAsian // EastAsianWidth ∈ {F, H, W}
	Pi        // GeneralCategory = Pi (Initial Punctuation)
	Pf        // GeneralCategory = Pf (Final Punctuation)
	ExtPict   // Extended_Pictographic (emoji/emoji-data.txt)

	// LB9/LB10 transparent properties.
	Extend // Extend (GCB=Extend, absorbed by LB9)
	ZWJ    // ZWJ (U+200D, absorbed by LB9)
)

// allBaseProperties is the OR of all single-bit property flags that represent
// standalone codepoint properties. Trait flags (EastAsian, Pi, Pf) are excluded
// because they only appear combined with base properties (e.g. OP|EastAsian).
const allBaseProperties = BK | CR | LF | NL | WJ | ZW | GL | SP |
	B2 | BA | BB | HY | CB |
	CL | CP | EX | IN | NS | OP | QU |
	IS | NU | PO | PR | SY |
	AK | AL | AP | AS | EB | EM | H2 | H3 | HL | ID |
	JL | JV | JT | RI | SA | VF | VI | CJ |
	Extend | ZWJ
