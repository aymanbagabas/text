// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

// Class is a bitflag type for sentence break properties.
// Each base property occupies one bit. The zero value represents Other.
type Class uint16

// Base property bitflags for sentence break.
//
// Each property occupies one bit in a Class value. The zero value
// represents Other (SB=Other) — the default property for codepoints
// with no specific class.
//
// SB5 absorption states, chain states, and virtual properties are
// NOT bitflags — they are uint8 indices assigned after flattening,
// used by the state machine.
const (
	Other Class = 0 // SB=Other — zero value, no bits set

	CR        Class = 1 << iota // SB=CR
	LF                          // SB=LF
	Sep                         // SB=Sep
	Extend                      // SB=Extend
	Format                      // SB=Format
	Sp                          // SB=Sp
	Lower                       // SB=Lower
	Upper                       // SB=Upper
	OLetter                     // SB=OLetter
	Numeric                     // SB=Numeric
	ATerm                       // SB=ATerm
	STerm                       // SB=STerm
	SContinue                   // SB=SContinue
	Close                       // SB=Close
)

// allBaseProperties is the OR of all base property bits.
const allBaseProperties = CR | LF | Sep | Extend | Format | Sp |
	Lower | Upper | OLetter | Numeric | ATerm | STerm | SContinue | Close

// ParaSep groups paragraph separator properties.
const ParaSep = Sep | CR | LF

// SATerm groups sentence-terminator properties (base only).
// For expanded SATerm including absorption states, see gen.go.
const SATerm = ATerm | STerm
