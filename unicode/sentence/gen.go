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

// sbMap maps Sentence_Break property value strings to Class bitflags.
var sbMap = map[string]Class{
	"Other":     Other,
	"CR":        CR,
	"LF":        LF,
	"Sep":       Sep,
	"Extend":    Extend,
	"Format":    Format,
	"Sp":        Sp,
	"Lower":     Lower,
	"Upper":     Upper,
	"OLetter":   OLetter,
	"Numeric":   Numeric,
	"ATerm":     ATerm,
	"STerm":     STerm,
	"SContinue": SContinue,
	"Close":     Close,
}

func main() {
	gen.Init()
	genTables()
}

func genTables() {
	// --- Flattener setup ---
	flat := segmenter.NewFlattener[Class]()
	numBase := flat.AddAllBaseProperties(allBaseProperties) // 15 (Other + 14 base properties)

	idx := flat.Index

	// SB5 absorption states (assigned above base properties).
	pLower_XX := uint8(numBase)
	pUpper_XX := uint8(numBase + 1)
	pOLetter_XX := uint8(numBase + 2)
	pNumeric_XX := uint8(numBase + 3)
	pATerm_XX := uint8(numBase + 4)
	pSTerm_XX := uint8(numBase + 5)
	pSCont_XX := uint8(numBase + 6)
	pClose_XX := uint8(numBase + 7)
	pUL_ATerm := uint8(numBase + 8)
	lastCodepointProperty := pUL_ATerm

	// Chain states for multi-character rules SB8–SB11.
	pATerm_Close := lastCodepointProperty + 1
	pATerm_Sp := lastCodepointProperty + 2
	pATerm_Scan := lastCodepointProperty + 3
	pATerm_Para := lastCodepointProperty + 4
	pSTerm_Close := lastCodepointProperty + 5
	pSTerm_Sp := lastCodepointProperty + 6
	pSTerm_Para := lastCodepointProperty + 7

	// Virtual properties.
	pSOT := lastCodepointProperty + 8
	pEOT := lastCodepointProperty + 9
	propCount := int(lastCodepointProperty + 10)

	// --- Repackage gen_trieval.go → trieval.go ---
	gen.Repackage("gen_trieval.go", "trieval.go", "sentence")

	// --- Generate prop.go (runtime-used constants only) ---
	writeProps(idx, lastCodepointProperty, pSOT, pEOT, uint8(propCount))

	// --- Build trie ---
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "sentence")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Sentence_Break property values.
	ucd.Parse(gen.OpenUCDFile("auxiliary/SentenceBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		cls, ok := sbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Sentence_Break value %q", r, val)
		}
		props[r] = idx(cls)
	})

	// Step 2: Build the property trie.
	t := triegen.NewTrie("sentence")
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if props[r] != 0 {
			t.Insert(r, uint64(props[r]))
		}
	}

	sz, err := t.Gen(w)
	if err != nil {
		log.Fatal(err)
	}
	w.Size += sz

	// Step 3: Build and write the break state table.
	p := func(ps ...uint8) []uint8 { return ps }

	// Expanded SATerm including absorption states.
	allSATerm := p(idx(ATerm), idx(STerm), pATerm_XX, pSTerm_XX, pUL_ATerm)
	allATerm := p(idx(ATerm), pATerm_XX, pUL_ATerm)
	allSTerm := p(idx(STerm), pSTerm_XX)
	flatParaSep := p(idx(Sep), idx(CR), idx(LF))
	sb5Ignored := p(idx(Extend), idx(Format))

	// chainStates lists combined states whose rows should default to NoMatch.
	chainStates := []uint8{
		pATerm_Close, pATerm_Sp, pATerm_Scan, pATerm_Para,
		pSTerm_Close, pSTerm_Sp, pSTerm_Para,
	}

	// SB8 scan char sets per entry point.
	scanFromATerm := p(idx(Other))
	scanFromClose := p(idx(Other), idx(Numeric))
	scanFromSp := p(idx(Other), idx(Numeric), idx(Close))
	scanFromScan := p(idx(Other), idx(Numeric), idx(SContinue), idx(Close), idx(Sp))

	// Build rules.
	var rules []segmenter.Rule

	// SB1: sot ×
	rules = append(rules, segmenter.Rule{Left: p(pSOT), Right: nil, Break: false})
	// SB2: ÷ eot
	rules = append(rules, segmenter.Rule{Left: nil, Right: p(pEOT), Break: true})
	// SB3: CR × LF
	rules = append(rules, segmenter.Rule{Left: p(idx(CR)), Right: p(idx(LF)), Break: false})
	// SB4: ParaSep ÷
	rules = append(rules, segmenter.Rule{Left: flatParaSep, Right: nil, Break: true})

	// SB11 Para chain states: break on anything.
	rules = append(rules, segmenter.Rule{Left: p(pATerm_Para, pSTerm_Para), Right: nil, Break: true})

	// SB5: × (Extend | Format)
	rules = append(rules, segmenter.Rule{Left: nil, Right: sb5Ignored, Break: false})

	// SB6: ATerm × Numeric
	rules = append(rules, segmenter.Rule{Left: allATerm, Right: p(idx(Numeric)), Break: false})

	// SB7: (Upper|Lower) ATerm × Upper
	rules = append(rules, segmenter.Rule{Left: p(pUL_ATerm), Right: p(idx(Upper)), Break: false})

	// SB8: ATerm Close* Sp* × Lower (direct + chain)
	rules = append(rules, segmenter.Rule{Left: allATerm, Right: p(idx(Lower)), Break: false})
	rules = append(rules, segmenter.Rule{
		Left: p(pATerm_Close, pATerm_Sp, pATerm_Scan), Right: p(idx(Lower)), Break: false,
	})

	// SB8a: SATerm Close* Sp* × (SContinue | SATerm)
	rules = append(rules, segmenter.Rule{
		Left:  append(append(p(), allATerm...), pATerm_Close, pATerm_Sp, pATerm_Scan),
		Right: p(idx(SContinue), idx(ATerm), idx(STerm)), Break: false,
	})
	rules = append(rules, segmenter.Rule{
		Left:  append(append(p(), allSTerm...), pSTerm_Close, pSTerm_Sp),
		Right: p(idx(SContinue), idx(ATerm), idx(STerm)), Break: false,
	})

	// SB11: SATerm ÷ (bare, non-chain cases)
	rules = append(rules, segmenter.Rule{Left: allSATerm, Right: nil, Break: true})

	// Chain state rows: default to Break (post-processing converts to NoMatch).
	rules = append(rules, segmenter.Rule{
		Left: p(pATerm_Close, pATerm_Sp, pATerm_Scan,
			pSTerm_Close, pSTerm_Sp), Right: nil, Break: true,
	})

	// SB998: Any × Any (do not break)
	rules = append(rules, segmenter.Rule{Left: nil, Right: nil, Break: false})

	// Build combined states.
	// SB5 absorption: base properties absorb Extend/Format.
	sb5Base := segmenter.ExpandAll(
		segmenter.IgnoreRule{
			Props: p(idx(Lower), pLower_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pLower_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(Upper), pUpper_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pUpper_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(OLetter), pOLetter_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pOLetter_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(Numeric), pNumeric_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pNumeric_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(ATerm), pATerm_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pATerm_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(STerm), pSTerm_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pSTerm_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(SContinue), pSCont_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pSCont_XX },
		},
		segmenter.IgnoreRule{
			Props: p(idx(Close), pClose_XX), Ignored: sb5Ignored,
			Target: func(_, _ uint8) uint8 { return pClose_XX },
		},
	)

	sb5UL := segmenter.IgnoreRule{
		Props: p(pUL_ATerm), Ignored: sb5Ignored,
		Target: func(_, _ uint8) uint8 { return pUL_ATerm },
		Interm: true,
	}.Expand()

	sb5ChainInterm := segmenter.IgnoreRule{
		Props: p(pATerm_Close, pATerm_Sp,
			pSTerm_Close, pSTerm_Sp),
		Ignored: sb5Ignored,
		Target:  func(base, _ uint8) uint8 { return base },
		Interm:  true,
	}.Expand()

	sb5Scan := segmenter.IgnoreRule{
		Props: p(pATerm_Scan), Ignored: sb5Ignored,
		Target: func(base, _ uint8) uint8 { return base },
	}.Expand()

	// SB7: (Upper|Lower) × ATerm → pUL_ATerm
	sb7 := segmenter.ChainRule{
		Entry: p(idx(Upper), idx(Lower), pUpper_XX, pLower_XX),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(ATerm)), State: pUL_ATerm},
		},
	}.Expand()

	// ATerm chains: SB9/SB10/SB11/SB8
	aTermChain := segmenter.ExpandAll(
		segmenter.ChainRule{
			Entry:    allATerm,
			Steps:    []segmenter.ChainStep{{Props: p(idx(Close), pClose_XX), State: pATerm_Close}},
			SelfLoop: p(idx(Close), pClose_XX),
			Interm:   true,
		},
		segmenter.ChainRule{
			Entry:  append(append(p(), allATerm...), pATerm_Close),
			Steps:  []segmenter.ChainStep{{Props: p(idx(Sp)), State: pATerm_Sp}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  p(pATerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: p(idx(Sp)), State: pATerm_Sp}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  append(append(p(), allATerm...), pATerm_Close, pATerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: flatParaSep, State: pATerm_Para}},
			Interm: true,
		},
		// SB8 scan entries.
		segmenter.ChainRule{
			Entry: allATerm,
			Steps: []segmenter.ChainStep{{
				Props: scanFromATerm, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry: p(pATerm_Close),
			Steps: []segmenter.ChainStep{{
				Props: scanFromClose, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry: p(pATerm_Sp),
			Steps: []segmenter.ChainStep{{
				Props: scanFromSp, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry: p(pATerm_Scan),
			Steps: []segmenter.ChainStep{{
				Props: scanFromScan, State: pATerm_Scan, Interm: segmenter.IntermFalse,
			}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  p(pATerm_Scan),
			Steps:  []segmenter.ChainStep{{Props: flatParaSep, State: pATerm_Para}},
			Interm: true,
		},
	)

	// STerm chains: SB8a/SB9/SB10/SB11
	sTermChain := segmenter.ExpandAll(
		segmenter.ChainRule{
			Entry:    allSTerm,
			Steps:    []segmenter.ChainStep{{Props: p(idx(Close), pClose_XX), State: pSTerm_Close}},
			SelfLoop: p(idx(Close), pClose_XX),
			Interm:   true,
		},
		segmenter.ChainRule{
			Entry:  append(append(p(), allSTerm...), pSTerm_Close),
			Steps:  []segmenter.ChainStep{{Props: p(idx(Sp)), State: pSTerm_Sp}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  p(pSTerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: p(idx(Sp)), State: pSTerm_Sp}},
			Interm: true,
		},
		segmenter.ChainRule{
			Entry:  append(append(p(), allSTerm...), pSTerm_Close, pSTerm_Sp),
			Steps:  []segmenter.ChainStep{{Props: flatParaSep, State: pSTerm_Para}},
			Interm: true,
		},
	)

	var combinedStates []segmenter.CombinedState
	combinedStates = append(combinedStates, sb5Base...)
	combinedStates = append(combinedStates, sb5UL...)
	combinedStates = append(combinedStates, sb5ChainInterm...)
	combinedStates = append(combinedStates, sb5Scan...)
	combinedStates = append(combinedStates, sb7...)
	combinedStates = append(combinedStates, aTermChain...)
	combinedStates = append(combinedStates, sTermChain...)

	table := segmenter.BuildStateTable(rules, combinedStates, propCount)

	// Post-processing: chain state rows default to Break via the explicit
	// chain state break rule. Convert Break cells to NoMatch so the engine
	// rewinds to the Intermediate marker position (the SB11 break point).
	for _, ls := range chainStates {
		row := int(ls) * propCount
		for j := 0; j < propCount; j++ {
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
		if i%propCount == 0 {
			fmt.Fprintf(w, "\n\t")
		}
		fmt.Fprintf(w, "%d, ", v)
	}
	fmt.Fprintf(w, "\n}\n\n")

	w.WriteComment("stride is the number of columns in breakTable.")
	fmt.Fprintf(w, "const stride = %d\n", propCount)
}

// writeProps generates prop.go with the runtime-used property constants.
func writeProps(idx func(Class) uint8, lastCodepointProperty, pSOT, pEOT, propCount uint8) {
	w := gen.NewCodeWriter()
	defer w.WriteGoFile("prop.go", "sentence")

	fmt.Fprintf(w, "const (\n")
	fmt.Fprintf(w, "\tpropCount             uint8 = %d\n", propCount)
	fmt.Fprintf(w, "\tlastCodepointProperty uint8 = %d\n", lastCodepointProperty)
	fmt.Fprintf(w, "\tpSOT                  uint8 = %d\n", pSOT)
	fmt.Fprintf(w, "\tpEOT                  uint8 = %d\n", pEOT)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "\tpLower   uint8 = %d\n", idx(Lower))
	fmt.Fprintf(w, "\tpUpper   uint8 = %d\n", idx(Upper))
	fmt.Fprintf(w, "\tpNumeric uint8 = %d\n", idx(Numeric))
	fmt.Fprintf(w, "\tpSp      uint8 = %d\n", idx(Sp))
	fmt.Fprintf(w, ")\n")
}
