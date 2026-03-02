// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

import (
	"fmt"
	"log"
	"unicode"

	"golang.org/x/text/internal/gen"
	"golang.org/x/text/internal/segmenter"
	"golang.org/x/text/internal/triegen"
	"golang.org/x/text/internal/ucd"
)

// sbMap maps Sentence_Break property value strings to property indices.
var sbMap = map[string]uint8{
	"Other":     pOther,
	"CR":        pCR,
	"LF":        pLF,
	"Sep":       pSep,
	"Extend":    pExtend,
	"Format":    pFormat,
	"Sp":        pSp,
	"Lower":     pLower,
	"Upper":     pUpper,
	"OLetter":   pOLetter,
	"Numeric":   pNumeric,
	"ATerm":     pATerm,
	"STerm":     pSTerm,
	"SContinue": pSContinue,
	"Close":     pClose,
}

func main() {
	gen.Init()
	gen.Repackage("gen_trieval.go", "trieval.go", "sentence")
	genTables()
}

func genTables() {
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "sentence")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Sentence_Break property values.
	ucd.Parse(gen.OpenUCDFile("auxiliary/SentenceBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		prop, ok := sbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Sentence_Break value %q", r, val)
		}
		props[r] = prop
	})

	// Step 2: Build the property trie.
	t := triegen.NewTrie("sentence")
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if props[r] != pOther {
			t.Insert(r, uint64(props[r]))
		}
	}

	sz, err := t.Gen(w)
	if err != nil {
		log.Fatal(err)
	}
	w.Size += sz

	// Step 3: Build and write the break state table.
	table := segmenter.BuildStateTable(rules, combinedStates, int(propCount))

	// Post-processing: chain state rows default to Break via the explicit
	// chain state break rule. Convert Break cells to NoMatch so the engine
	// rewinds to the Intermediate marker position (the SB11 break point).
	// Cells explicitly set by higher-priority rules (SB8/SB8a → Keep) or
	// by combined states (SB5/chain transitions) are preserved.
	for _, ls := range chainStates {
		row := int(ls) * int(propCount)
		for j := 0; j < int(propCount); j++ {
			if table[row+j] == segmenter.Break {
				table[row+j] = segmenter.NoMatch
			}
		}
	}

	w.WriteComment(
		`breakTable is the sentence break state table.
	breakTable[left*stride + right] encodes the action for (left, right).
	See segmenter.BreakState for the action encoding.`)
	fmt.Fprintf(w, "var breakTable = [...]uint8{")
	for i, v := range table {
		if i%int(propCount) == 0 {
			fmt.Fprintf(w, "\n\t")
		}
		fmt.Fprintf(w, "%d, ", v)
	}
	fmt.Fprintf(w, "\n}\n\n")

	w.WriteComment("stride is the number of columns in breakTable.")
	fmt.Fprintf(w, "const stride = %d\n", propCount)
}

// ---------------------------------------------------------------------------
// Sentence boundary rules (UAX #29, Section 5)
// ---------------------------------------------------------------------------
//
// BuildStateTable applies rules in reverse order: the FIRST rule in the
// slice has the HIGHEST priority. Rules are ordered accordingly.
//
// The sentence break algorithm uses three mechanisms:
//
// 1. IgnoreRule (SB5): Extend/Format absorption. Base properties absorb
//    adjacent Extend/Format characters, keeping the base identity.
//
// 2. ChainRule (SB9/SB10/SB11): Multi-character patterns like
//    SATerm Close* Sp* ParaSep? are handled as chains of Intermediate
//    states so the engine's marker advances past each consumed character.
//    On NoMatch, the engine rewinds to the marker, implementing the SB11
//    break position. The SB8 forward scan uses Index (non-Intermediate)
//    states via IntermFalse so the marker stays at the SB11 position.
//
// 3. Plain rules (SB1–SB8a, SB998): Pairwise left×right rules.

func p(props ...uint8) []uint8 { return props }

// Rule macros from UAX #29.
var (
	ParaSep = p(pSep, pCR, pLF)
	SATerm  = p(pATerm, pSTerm, pATerm_XX, pSTerm_XX, pUL_ATerm)
)

// chainStates lists combined states whose rows should default to NoMatch
// instead of Break (post-processing converts Break→NoMatch).
var chainStates = []uint8{
	pATerm_Close, pATerm_Sp, pATerm_Scan, pATerm_Para,
	pSTerm_Close, pSTerm_Sp, pSTerm_Para,
}

// sb5Ignored is the set of properties absorbed by SB5.
var sb5Ignored = p(pExtend, pFormat)

// SB8 scan char sets per entry point. The scan looks past
// ¬(OLetter | Upper | Lower | ParaSep | SATerm) to find Lower.
// Each entry point restricts the set to avoid overriding higher-priority
// rules/chains that combined states would otherwise clobber.
//
// Note: right-side properties are always base properties (from the trie),
// never _XX absorption states, so we only list base properties.
// Extend/Format are handled by SB5 IgnoreRules, not scan entries.
var (
	// From bare ATerm: SB6 handles Numeric, SB8a handles SContinue,
	// chains handle Close/Sp/ParaSep.
	scanFromATerm = p(pOther)

	// From pATerm_Close: SB8a handles SContinue,
	// chain self-loop handles Close, chain handles Sp.
	scanFromClose = p(pOther, pNumeric)

	// From pATerm_Sp: SB8a handles SContinue,
	// chain self-loop handles Sp.
	scanFromSp = p(pOther, pNumeric, pClose)

	// From pATerm_Scan self-loop: no rule/chain conflicts.
	scanFromScan = p(pOther, pNumeric, pSContinue, pClose, pSp)
)

// allATerm lists property indices that behave as ATerm for chain entry.
var allATerm = p(pATerm, pATerm_XX, pUL_ATerm)

// allSTerm lists property indices that behave as STerm for chain entry.
var allSTerm = p(pSTerm, pSTerm_XX)

// rules encodes the UAX #29 sentence boundary rules (SB1–SB998).
//
// IMPORTANT: BuildStateTable applies rules in reverse order, so the FIRST
// rule has the HIGHEST priority. Rules are ordered from highest to lowest
// priority (SB1 first, SB998 last).
//
// Chain state rows (SB8–SB11) default to NoMatch after post-processing,
// so unmatched transitions cause the engine to rewind and break (SB11).
//
// References: https://www.unicode.org/reports/tr29/#Sentence_Boundary_Rules
var rules = []segmenter.Rule{
	// SB1: sot ÷ Any — don't break at start (engine handles sot as left).
	{Left: p(pSOT), Right: nil, Break: false},

	// SB2: Any ÷ eot — break at end.
	{Left: nil, Right: p(pEOT), Break: true},

	// SB3: CR × LF
	{Left: p(pCR), Right: p(pLF), Break: false},

	// SB4: ParaSep ÷ (break after paragraph separators)
	{Left: ParaSep, Right: nil, Break: true},

	// SB11 Para chain states: break on anything. Placed here (above SB5)
	// so that SB4's break-after-ParaSep semantics also apply to the
	// chain states that consumed a ParaSep — Extend/Format must NOT be
	// absorbed past ParaSep.
	{Left: p(pATerm_Para, pSTerm_Para), Right: nil, Break: true},

	// SB5: X (Extend | Format)* → X
	// Handled by IgnoreRule in combinedStates. The residual effect is:
	{Left: nil, Right: sb5Ignored, Break: false},

	// SB6: ATerm × Numeric
	{Left: allATerm, Right: p(pNumeric), Break: false},

	// SB7: (Upper | Lower) ATerm × Upper
	{Left: p(pUL_ATerm), Right: p(pUpper), Break: false},

	// SB8: ATerm Close* Sp* × (¬(OLetter|Upper|Lower|ParaSep|SATerm))* Lower
	// Direct case:
	{Left: allATerm, Right: p(pLower), Break: false},
	// Chain cases:
	{Left: p(pATerm_Close, pATerm_Sp, pATerm_Scan), Right: p(pLower), Break: false},

	// SB8a: SATerm Close* Sp* × (SContinue | SATerm)
	{Left: append(append(p(), allATerm...), pATerm_Close, pATerm_Sp, pATerm_Scan),
		Right: p(pSContinue, pATerm, pSTerm), Break: false},
	{Left: append(append(p(), allSTerm...), pSTerm_Close, pSTerm_Sp),
		Right: p(pSContinue, pATerm, pSTerm), Break: false},

	// SB11: SATerm Close* Sp* ParaSep? ÷
	// Para chain states handled above (before SB5).
	// Bare SATerm: break on characters that don't enter a chain state
	// and aren't handled by SB6/SB7/SB8/SB8a/SB9. Since chain combined
	// states handle Close/Sp/ParaSep/scan-chars, and SB6/SB7/SB8 handle
	// Numeric/Upper/Lower, the remaining cases are: OLetter and Other
	// (for ATerm) plus OLetter, Other, Upper, Lower, Numeric (for STerm).
	{Left: SATerm, Right: nil, Break: true},
	// Non-Para chain state rows: default to Break so that post-processing
	// converts unhandled cells to NoMatch (rewind to SB11 position).
	// SB8/SB8a rules above have higher priority and set Keep for handled
	// right-side properties (Lower, SContinue, ATerm, STerm).
	// Combined states override for SB5 absorption and chain transitions.
	{Left: p(pATerm_Close, pATerm_Sp, pATerm_Scan,
		pSTerm_Close, pSTerm_Sp), Right: nil, Break: true},

	// SB998: Any × Any (do not break) — LAST = lowest priority.
	{Left: nil, Right: nil, Break: false},
}

// combinedStates wires up SB5 absorption, SB7 context, and the SB8–SB11 chains.
var combinedStates = func() []segmenter.CombinedState {
	// --- SB5 absorption: base properties absorb Extend/Format ---
	sb5Base := segmenter.ExpandAll(
		segmenter.IgnoreRule{
			Props: p(pLower, pLower_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pLower_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pUpper, pUpper_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pUpper_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pOLetter, pOLetter_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pOLetter_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pNumeric, pNumeric_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pNumeric_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pATerm, pATerm_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pATerm_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pSTerm, pSTerm_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pSTerm_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pSContinue, pSCont_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pSCont_XX },
		},
		segmenter.IgnoreRule{
			Props: p(pClose, pClose_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pClose_XX },
		},
	)

	// SB5 on pUL_ATerm: absorb Extend/Format, stay in pUL_ATerm.
	sb5UL := segmenter.IgnoreRule{
		Props: p(pUL_ATerm), Ignored: sb5Ignored,
		Target: func(_, _ uint8) uint8 { return pUL_ATerm },
		Interm: true,
	}.Expand()

	// SB5 on Intermediate chain states: absorb Extend/Format.
	// Note: pATerm_Para and pSTerm_Para are excluded because SB4 says
	// ParaSep always breaks — absorbing Extend/Format past ParaSep is wrong.
	sb5ChainInterm := segmenter.IgnoreRule{
		Props: p(pATerm_Close, pATerm_Sp,
			pSTerm_Close, pSTerm_Sp),
		Ignored: sb5Ignored,
		Target:  func(base, _ uint8) uint8 { return base },
		Interm:  true,
	}.Expand()

	// SB5 on scan state: absorb but don't advance marker.
	sb5Scan := segmenter.IgnoreRule{
		Props: p(pATerm_Scan), Ignored: sb5Ignored,
		Target: func(base, _ uint8) uint8 { return base },
	}.Expand()

	// --- SB7 context: (Upper|Lower) × ATerm → pUL_ATerm ---
	sb7 := segmenter.ChainRule{
		Entry: p(pUpper, pLower, pUpper_XX, pLower_XX),
		Steps: []segmenter.ChainStep{
			{Props: p(pATerm), State: pUL_ATerm},
		},
	}.Expand()

	// --- ATerm chains: SB9/SB10/SB11/SB8 ---
	//
	// SATerm Close* Sp* ParaSep?
	// ─────────────────────────
	// Close/Sp/ParaSep use Intermediate states so the marker advances
	// past each consumed character, giving the correct SB11 break position.
	//
	// SB8 scan uses Index (IntermFalse) so the marker does NOT advance
	// past scan characters. If the scan fails (no Lower found), the
	// engine rewinds to the SB11 position (after Close* Sp*).

	aTermChain := segmenter.ExpandAll(
		// ATerm × Close → pATerm_Close
		segmenter.ChainRule{
			Entry:    allATerm,
			Steps:    []segmenter.ChainStep{{Props: p(pClose, pClose_XX), State: pATerm_Close}},
			SelfLoop: p(pClose, pClose_XX),
			Interm:   true,
		},
		// ATerm/ATerm_Close × Sp → pATerm_Sp
		segmenter.ChainRule{
			Entry:  append(append(p(), allATerm...), pATerm_Close),
			Steps:  []segmenter.ChainStep{{Props: p(pSp), State: pATerm_Sp}},
			Interm: true,
		},
		// pATerm_Sp × Sp → pATerm_Sp (self-loop)
		segmenter.ChainRule{
			Entry:  p(pATerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: p(pSp), State: pATerm_Sp}},
			Interm: true,
		},
		// ATerm/Close/Sp × ParaSep → pATerm_Para
		segmenter.ChainRule{
			Entry:  append(append(p(), allATerm...), pATerm_Close, pATerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: ParaSep, State: pATerm_Para}},
			Interm: true,
		},
		// SB8 scan entries with per-entry-point restricted scan char sets.
		// From bare ATerm: only pOther (SB6/SB8a/chains handle the rest).
		segmenter.ChainRule{
			Entry: allATerm,
			Steps: []segmenter.ChainStep{{
				Props: scanFromATerm, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		// From pATerm_Close: pOther, pNumeric.
		segmenter.ChainRule{
			Entry: p(pATerm_Close),
			Steps: []segmenter.ChainStep{{
				Props: scanFromClose, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		// From pATerm_Sp: pOther, pNumeric, pClose.
		segmenter.ChainRule{
			Entry: p(pATerm_Sp),
			Steps: []segmenter.ChainStep{{
				Props: scanFromSp, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		// SB8 scan self-loop: full set (no conflicts from scan state).
		segmenter.ChainRule{
			Entry: p(pATerm_Scan),
			Steps: []segmenter.ChainStep{{
				Props: scanFromScan, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		// scan × ParaSep → pATerm_Para (Intermediate)
		segmenter.ChainRule{
			Entry:  p(pATerm_Scan),
			Steps:  []segmenter.ChainStep{{Props: ParaSep, State: pATerm_Para}},
			Interm: true,
		},
	)

	// --- STerm chains: SB8a/SB9/SB10/SB11 (no SB8 scan) ---
	sTermChain := segmenter.ExpandAll(
		segmenter.ChainRule{
			Entry:    allSTerm,
			Steps:    []segmenter.ChainStep{{Props: p(pClose, pClose_XX), State: pSTerm_Close}},
			SelfLoop: p(pClose, pClose_XX),
			Interm:   true,
		},
		segmenter.ChainRule{
			Entry:  append(append(p(), allSTerm...), pSTerm_Close),
			Steps:  []segmenter.ChainStep{{Props: p(pSp), State: pSTerm_Sp}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  p(pSTerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: p(pSp), State: pSTerm_Sp}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  append(append(p(), allSTerm...), pSTerm_Close, pSTerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: ParaSep, State: pSTerm_Para}},
			Interm: true,
		},
	)

	var cs []segmenter.CombinedState
	cs = append(cs, sb5Base...)
	cs = append(cs, sb5UL...)
	cs = append(cs, sb5ChainInterm...)
	cs = append(cs, sb5Scan...)
	cs = append(cs, sb7...)
	cs = append(cs, aTermChain...)
	cs = append(cs, sTermChain...)
	return cs
}()


