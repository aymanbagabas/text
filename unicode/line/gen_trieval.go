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
// LB9 absorption states, chain states, and virtual properties are
// NOT bitflags — they are uint8 indices assigned after flattening,
// used by the state machine.
const (
	XX Class = 0 // LB=XX (Unknown / default) — zero value, no bits set

	BK Class = 1 << iota // LB=BK (Mandatory Break)
	CR                    // LB=CR (Carriage Return)
	LF                    // LB=LF (Line Feed)
	NL                    // LB=NL (Next Line)
	SP                    // LB=SP (Space)
	ZW                    // LB=ZW (Zero Width Space)
	WJ                    // LB=WJ (Word Joiner)
	GL                    // LB=GL (Non-breaking / Glue)
	CL                    // LB=CL (Close Punctuation)
	EX                    // LB=EX (Exclamation/Interrogation)
	IS                    // LB=IS (Infix Numeric Separator)
	SY                    // LB=SY (Symbols Allowing Break After)
	OP                    // LB=OP (Open Punctuation, non-EA)
	QU                    // LB=QU (Quotation, non-Pi/Pf)
	NS                    // LB=NS (Nonstarter)
	HY                    // LB=HY (Hyphen)
	BA                    // LB=BA (Break After)
	BB                    // LB=BB (Break Before)
	B2                    // LB=B2 (Break Opportunity Before and After)
	IN                    // LB=IN (Inseparable)
	AL                    // LB=AL (Alphabetic)
	NU                    // LB=NU (Numeric)
	PR                    // LB=PR (Prefix Numeric)
	PO                    // LB=PO (Postfix Numeric)
	ID                    // LB=ID (Ideographic)
	EB                    // LB=EB (Emoji Base)
	EM                    // LB=EM (Emoji Modifier)
	CB                    // LB=CB (Contingent Break)
	RI                    // LB=RI (Regional Indicator)
	SA                    // LB=SA (Complex Context / South Asian)
	HL                    // LB=HL (Hebrew Letter)
	CJ                    // LB=CJ (Conditional Japanese Starter)
	AK                    // LB=AK (Aksara)
	AP                    // LB=AP (Aksara Pre-base)
	AS                    // LB=AS (Aksara Start)
	VF                    // LB=VF (Virama Final)
	VI                    // LB=VI (Virama)

	// Hangul properties (LB26/LB27).
	JL // LB=JL (Hangul L Jamo)
	JV // LB=JV (Hangul V Jamo)
	JT // LB=JT (Hangul T Jamo)
	H2 // LB=H2 (Hangul LV Syllable)
	H3 // LB=H3 (Hangul LVT Syllable)

	// Synthetic properties (LineBreak + EastAsianWidth/GeneralCategory).
	OP_EA // OP with ea=F/H/W (East Asian Open Punctuation)
	CP    // CP (Close Punctuation, non-EA)
	CP_EA // CP with ea=F/H/W (East Asian Close Punctuation)
	QU_PI // QU with gc=Pi (Initial Quotation)
	QU_PF // QU with gc=Pf (Final Quotation)

	// LB9/LB10 transparent properties.
	Extend // Extend (GCB=Extend, absorbed by LB9)
	ZWJ    // ZWJ (U+200D, absorbed by LB9)
)

// allBaseProperties is the OR of all base property bits.
const allBaseProperties = BK | CR | LF | NL | SP | ZW | WJ | GL |
	CL | EX | IS | SY | OP | QU | NS | HY | BA | BB | B2 | IN |
	AL | NU | PR | PO | ID | EB | EM | CB | RI | SA | HL | CJ |
	AK | AP | AS | VF | VI |
	JL | JV | JT | H2 | H3 |
	OP_EA | CP | CP_EA | QU_PI | QU_PF |
	Extend | ZWJ

// Composite group constants. These OR together related base properties
// for use in break rules (via flat.Expand).

// LB9Excluded lists properties that do NOT participate in LB9 absorption.
// These are either mandatory-break/space/zero-width properties that are
// never absorbers, or the transparent properties being absorbed.
const LB9Excluded = BK | CR | LF | NL | SP | ZW | Extend | ZWJ

// MandatoryBreak groups all hard line break properties (LB4–LB6).
const MandatoryBreak = BK | CR | LF | NL

// AnyOP groups all Open Punctuation variants.
const AnyOP = OP | OP_EA

// AnyCP groups all Close Parenthesis variants.
const AnyCP = CP | CP_EA

// AnyClose groups all closing punctuation (CL + CP variants).
const AnyClose = CL | CP | CP_EA

// AnyQU groups all Quotation variants.
const AnyQU = QU | QU_PI | QU_PF

// Hangul groups all Korean syllable types (LB26/LB27).
const Hangul = JL | JV | JT | H2 | H3

// Aksara groups all aksara-related properties (LB28a).
const Aksara = AK | AS | VF | VI

// AksaraFinal groups aksara properties that can appear on the right of LB28a.
const AksaraFinal = AK | VF

// Ideographic groups ID, Emoji Base, and Emoji Modifier (LB23a).
const Ideographic = ID | EB | EM

// ALLike groups all properties that behave as AL (LB10).
// Unattached Extend/ZWJ (after BK/CR/LF/NL/SP/ZW) and SA resolve to AL.
// XX (zero value) also resolves to AL but cannot be expressed as a bitflag;
// it is added separately via expandAll(0) in gen.go.
const ALLike = AL | SA | Extend | ZWJ
