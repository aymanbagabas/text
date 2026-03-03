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

// lbMap maps Line_Break property value strings to Class bitflags.
// Entries that need synthetic splitting (OP, CP, QU) get their base flag;
// gen.go post-processes them using EastAsianWidth and GeneralCategory.
var lbMap = map[string]Class{
	"XX":  XX,
	"BK":  BK,
	"CR":  CR,
	"LF":  LF,
	"NL":  NL,
	"SP":  SP,
	"ZW":  ZW,
	"WJ":  WJ,
	"GL":  GL,
	"CL":  CL,
	"EX":  EX,
	"IS":  IS,
	"SY":  SY,
	"OP":  OP,
	"QU":  QU,
	"NS":  NS,
	"HY":  HY,
	"BA":  BA,
	"BB":  BB,
	"B2":  B2,
	"IN":  IN,
	"AL":  AL,
	"NU":  NU,
	"PR":  PR,
	"PO":  PO,
	"ID":  ID,
	"EB":  EB,
	"EM":  EM,
	"CB":  CB,
	"RI":  RI,
	"SA":  SA,
	"HL":  HL,
	"CJ":  CJ,
	"AK":  AK,
	"AP":  AP,
	"AS":  AS,
	"VF":  VF,
	"VI":  VI,
	"CP":  CP,
	"AI":  AL,     // LB1: AI → AL
	"SG":  XX,     // LB1: SG → XX
	"CM":  Extend, // LB9: CM treated as Extend; LB10: remaining CM → AL
	"ZWJ": ZWJ,    // LB9: ZWJ
	"H2":  H2,     // LB26/27: Hangul LV syllable
	"H3":  H3,     // LB26/27: Hangul LVT syllable
	"JL":  JL,     // LB26/27: Hangul L Jamo
	"JV":  JV,     // LB26/27: Hangul V Jamo
	"JT":  JT,     // LB26/27: Hangul T Jamo
	"HH":  HY,     // Unambiguous Hyphen → HY
}

func main() {
	gen.Init()
	genTables()
}

func genTables() {
	// --- Flattener setup ---
	flat := segmenter.NewFlattener[Class]()
	flat.AddAllBaseProperties(allBaseProperties)

	// Register multi-bit combinations (base property + orthogonal trait).
	flat.Add(OP | EastAsian) // East Asian open punctuation
	flat.Add(CP | EastAsian) // East Asian close parenthesis
	flat.Add(QU | Pi)        // Quotation with gc=Pi
	flat.Add(QU | Pf)        // Quotation with gc=Pf

	idx := flat.Index

	// --- LB9 absorption states ---
	// For each registered key that participates in LB9, assign a _XX state.
	// Excluded: BK, CR, LF, NL, SP, ZW, Extend, ZWJ (never absorbers).
	// Trait flags (EastAsian, Pi, Pf) are NOT in this mask — multi-bit keys
	// like OP|EastAsian still participate in LB9 via their base property.
	const lb9Excluded = BK | CR | LF | NL | SP | ZW | Extend | ZWJ
	xxOfMap := make(map[Class]uint8) // registered key → absorption state index
	nextIdx := uint8(flat.Len())     // starts after all registered keys
	for _, cls := range flat.Keys() {
		if cls == 0 { // XX handled separately below
			continue
		}
		if cls&lb9Excluded != 0 {
			continue
		}
		xxOfMap[cls] = nextIdx
		nextIdx++
	}
	// XX also absorbs (LB10: unattached Extend/ZWJ → AL behavior).
	xxOfMap[XX] = nextIdx
	nextIdx++

	lastCodepointProperty := nextIdx - 1

	// expandAll returns the uint8 indices of all registered keys
	// matching mask, plus their LB9 absorption states (if any).
	// mask == 0 is a special case: it matches only the XX (zero) key.
	expandAll := func(mask Class) []uint8 {
		var r []uint8
		for _, cls := range flat.Keys() {
			if cls == 0 {
				if mask&XX == 0 && mask != 0 {
					continue
				}
			} else if cls&mask == 0 {
				continue
			}
			r = append(r, idx(cls))
			if xx, ok := xxOfMap[cls]; ok {
				r = append(r, xx)
			}
		}
		return r
	}

	// --- Chain/lookahead states ---
	pZW_SP := nextIdx
	nextIdx++
	pOP_SP := nextIdx
	nextIdx++
	pOP_EA_SP := nextIdx
	nextIdx++
	pQU_SP := nextIdx
	nextIdx++
	pCL_SP := nextIdx
	nextIdx++
	pCP_SP := nextIdx
	nextIdx++
	pCP_EA_SP := nextIdx
	nextIdx++
	pB2_SP := nextIdx
	nextIdx++
	pHL_HY := nextIdx
	nextIdx++
	pRI_RI := nextIdx
	nextIdx++
	pNU_Num := nextIdx
	nextIdx++
	pNU_Close_CL := nextIdx
	nextIdx++
	pNU_Close_CP := nextIdx
	nextIdx++
	pNU_PR := nextIdx
	nextIdx++

	// Virtual properties.
	pSOT := nextIdx
	nextIdx++
	pEOT := nextIdx
	nextIdx++
	propCount := nextIdx

	// --- Repackage gen_trieval.go → trieval.go ---
	gen.Repackage("gen_trieval.go", "trieval.go", "line")

	// --- Generate prop.go (runtime-used constants only) ---
	writeProps(idx, lastCodepointProperty, pSOT, pEOT, propCount)

	// --- Build trie ---
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "line")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Line_Break property values.
	ucd.Parse(gen.OpenUCDFile("LineBreak.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		cls, ok := lbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Line_Break value %q", r, val)
		}
		props[r] = idx(cls)
	})

	// Step 2: Parse East_Asian_Width.
	eaw := make([]byte, unicode.MaxRune+1)
	ucd.Parse(gen.OpenUCDFile("EastAsianWidth.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		if len(val) > 0 {
			eaw[r] = val[0]
		}
	})

	// Step 3: Parse General_Category.
	gc := make([]string, unicode.MaxRune+1)
	ucd.Parse(gen.OpenUCDFile("UnicodeData.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		gc[r] = p.String(ucd.GeneralCategory)
	})

	// Step 4: Parse Extended_Pictographic from emoji-data.txt.
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(p *ucd.Parser) {
		if p.String(1) == "Extended_Pictographic" {
			r := p.Rune(0)
			if gc[r] == "" {
				props[r] = idx(EB)
			}
		}
	})

	// Step 5: Create synthetic properties by combining base + trait flags.
	for r := rune(0); r <= unicode.MaxRune; r++ {
		isEA := eaw[r] == 'F' || eaw[r] == 'H' || eaw[r] == 'W'
		switch props[r] {
		case idx(OP):
			if isEA {
				props[r] = idx(OP | EastAsian)
			}
		case idx(CP):
			if isEA {
				props[r] = idx(CP | EastAsian)
			}
		case idx(QU):
			switch gc[r] {
			case "Pi":
				props[r] = idx(QU | Pi)
			case "Pf":
				props[r] = idx(QU | Pf)
			}
		case idx(SA):
			// LB1: SA with gc in {Mn, Mc} → CM behavior (resolve as AL per LB10).
			if gc[r] == "Mn" || gc[r] == "Mc" {
				props[r] = idx(Extend)
			}
		case idx(CJ):
			// In normal (default) strictness, CJ resolves to NS.
			props[r] = idx(NS)
		}
	}

	// LB9/LB10: Map Extend (GCB=Extend) and ZWJ to their property indices.
	ucd.Parse(gen.OpenUCDFile("auxiliary/GraphemeBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		switch val {
		case "Extend":
			if props[r] == idx(XX) {
				props[r] = idx(Extend)
			}
		case "ZWJ":
			props[r] = idx(ZWJ)
		}
	})

	// Step 6: Build the property trie.
	t := triegen.NewTrie("line")
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if props[r] != idx(XX) {
			t.Insert(r, uint64(props[r]))
		}
	}

	sz, err := t.Gen(w)
	if err != nil {
		log.Fatal(err)
	}
	w.Size += sz

	// --- Helper functions ---
	p := func(ps ...uint8) []uint8 { return ps }
	e := expandAll
	allXX := e(0) // XX + pXX_XX (zero value can't be expressed in bitflag masks)

	// x returns the exact index + absorption state for a single registered key.
	x := func(cls Class) []uint8 {
		r := []uint8{idx(cls)}
		if xx, ok := xxOfMap[cls]; ok {
			r = append(r, xx)
		}
		return r
	}

	// Property groups via expandAll.
	// e(OP) returns both OP and OP|EastAsian (plus their absorption states).
	// e(CP) returns both CP and CP|EastAsian.
	// e(QU) returns QU, QU|Pi, and QU|Pf.
	allOP := e(OP)
	allCP := e(CP)
	allCL := e(CL)
	allClose := e(CL | CP) // CL, CP, CP|EastAsian
	allQU := e(QU)

	allHL := e(HL)
	allALLike := append(e(AL|SA|Extend|ZWJ|HL), allXX...) // ALLike + HL + XX

	allNU := e(NU)
	allPR := e(PR)
	allPO := e(PO)
	allEB := e(EB)
	allEM := e(EM)
	allIS := e(IS)
	allSY := e(SY)
	allBB := e(BB)
	allIN := e(IN)
	allNS := e(NS)
	allRI := e(RI)
	allCB := e(CB)
	allGL := e(GL)
	allWJ := e(WJ)
	allB2 := e(B2)
	allEX := e(EX)

	allAP := e(AP)
	allAksara := e(AK | AS | VF | VI)
	allAksaraFinal := e(AK | VF)

	allJL := e(JL)
	allJT := e(JT)
	allHangul := e(JL | JV | JT | H2 | H3)

	allIdeographic := e(ID | EB | EM)

	// --- LB9 absorption ---
	lb9Ignored := p(idx(Extend), idx(ZWJ))

	var combinedStates []segmenter.CombinedState

	for cls, xx := range xxOfMap {
		xx := xx // capture for closure
		combinedStates = append(combinedStates, segmenter.IgnoreRule{
			Props:   p(idx(cls), xx),
			Ignored: lb9Ignored,
			Target:  func(_, _ uint8) uint8 { return xx },
		}.Expand()...)
	}

	// --- Chain rules ---

	// LB8: ZW SP* ÷
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: p(idx(ZW)),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pZW_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pZW_SP, Right: idx(SP), State: pZW_SP, Interm: true})

	// LB14: OP SP* ×
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: x(OP),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pOP_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pOP_SP, Right: idx(SP), State: pOP_SP, Interm: true})
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: x(OP | EastAsian),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pOP_EA_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pOP_EA_SP, Right: idx(SP), State: pOP_EA_SP, Interm: true})

	// LB15: QU SP* × OP
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: allQU,
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pQU_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pQU_SP, Right: idx(SP), State: pQU_SP, Interm: true})

	// LB16: (CL|CP) SP* × NS
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: allCL,
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pCL_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pCL_SP, Right: idx(SP), State: pCL_SP, Interm: true})

	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: x(CP),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pCP_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pCP_SP, Right: idx(SP), State: pCP_SP, Interm: true})

	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: x(CP | EastAsian),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pCP_EA_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pCP_EA_SP, Right: idx(SP), State: pCP_EA_SP, Interm: true})

	// LB17: B2 SP* × B2
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: allB2,
		Steps: []segmenter.ChainStep{
			{Props: p(idx(SP)), State: pB2_SP},
		},
		Interm: true,
	}.Expand()...)
	combinedStates = append(combinedStates, segmenter.CombinedState{Left: pB2_SP, Right: idx(SP), State: pB2_SP, Interm: true})

	// LB21a: HL (HY|BA) ×
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: allHL,
		Steps: []segmenter.ChainStep{
			{Props: e(HY | BA), State: pHL_HY},
		},
		Interm: true,
	}.Expand()...)

	// LB30a: RI × RI (paired)
	combinedStates = append(combinedStates, segmenter.ChainRule{
		Entry: allRI,
		Steps: []segmenter.ChainStep{
			{Props: p(idx(RI)), State: pRI_RI},
		},
	}.Expand()...)

	// LB25 (tailored): NU (NU|SY|IS)* × (NU|SY|IS|CL|CP)
	nuBodyRight := e(NU | SY | IS)
	nuCLRight := allCL
	nuCPRight := allCP
	for _, l := range allNU {
		for _, r := range nuBodyRight {
			combinedStates = append(combinedStates, segmenter.CombinedState{Left: l, Right: r, State: pNU_Num, Interm: true})
		}
		for _, r := range nuCLRight {
			combinedStates = append(combinedStates, segmenter.CombinedState{Left: l, Right: r, State: pNU_Close_CL, Interm: true})
		}
		for _, r := range nuCPRight {
			combinedStates = append(combinedStates, segmenter.CombinedState{Left: l, Right: r, State: pNU_Close_CP, Interm: true})
		}
	}
	for _, r := range nuBodyRight {
		combinedStates = append(combinedStates, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_Num, Interm: true})
	}
	for _, r := range nuCLRight {
		combinedStates = append(combinedStates, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_Close_CL, Interm: true})
	}
	for _, r := range nuCPRight {
		combinedStates = append(combinedStates, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_Close_CP, Interm: true})
	}

	nuPRRight := e(PO | PR)
	for _, l := range allNU {
		for _, r := range nuPRRight {
			combinedStates = append(combinedStates, segmenter.CombinedState{Left: l, Right: r, State: pNU_PR, Interm: true})
		}
	}
	for _, r := range nuPRRight {
		combinedStates = append(combinedStates, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_PR, Interm: true})
		combinedStates = append(combinedStates, segmenter.CombinedState{Left: pNU_Close_CL, Right: r, State: pNU_PR, Interm: true})
		combinedStates = append(combinedStates, segmenter.CombinedState{Left: pNU_Close_CP, Right: r, State: pNU_PR, Interm: true})
	}

	// --- Rules ---
	rules := []segmenter.Rule{
		// LB1: sot — don't break at start.
		{Left: p(pSOT), Right: nil, Break: false},

		// LB2: Any ÷ eot — break at end.
		{Left: nil, Right: p(pEOT), Break: true},

		// LB4: BK ÷
		{Left: p(idx(BK)), Right: nil, Break: true},

		// LB5: CR × LF
		{Left: p(idx(CR)), Right: p(idx(LF)), Break: false},
		// LB5: CR ÷, LF ÷, NL ÷
		{Left: p(idx(CR), idx(LF), idx(NL)), Right: nil, Break: true},

		// LB6: × (BK | CR | LF | NL)
		{Left: nil, Right: p(idx(BK), idx(CR), idx(LF), idx(NL)), Break: false},

		// LB7: × (SP | ZW)
		{Left: nil, Right: p(idx(SP), idx(ZW)), Break: false},

		// LB8: ZW SP* ÷ — chain rule handles the SP* case; base case ZW ÷ here.
		{Left: p(idx(ZW)), Right: nil, Break: true},

		// LB8a: ZWJ × — keep after ZWJ.
		{Left: p(idx(ZWJ)), Right: nil, Break: false},

		// LB11: × WJ, WJ ×
		{Left: nil, Right: allWJ, Break: false},
		{Left: allWJ, Right: nil, Break: false},

		// LB12: GL ×
		{Left: allGL, Right: nil, Break: false},

		// LB12a: [^SP BA HY] × GL
		{Left: e(SP | BA | HY), Right: allGL, Break: true},
		{Left: nil, Right: allGL, Break: false},

		// LB13 (tailored per Example 7): [^NU] × CL/CP/IS/SY, × EX.
		{Left: nil, Right: allEX, Break: false},
		{Left: allNU, Right: allClose, Break: true},
		{Left: nil, Right: e(CL | CP | IS | SY), Break: false},

		// LB14: OP SP* × — base rule for zero SPs.
		{Left: allOP, Right: nil, Break: false},

		// LB16: (CL|CP) SP* × NS — base rule for zero SPs.
		{Left: allClose, Right: allNS, Break: false},

		// LB17: B2 SP* × B2 — direct case B2 × B2.
		{Left: allB2, Right: allB2, Break: false},

		// LB18: SP ÷
		{Left: p(idx(SP)), Right: nil, Break: true},

		// LB19a: × QU ; QU ×
		{Left: nil, Right: allQU, Break: false},
		{Left: allQU, Right: nil, Break: false},

		// LB20: ÷ CB, CB ÷
		{Left: nil, Right: allCB, Break: true},
		{Left: allCB, Right: nil, Break: true},

		// LB21: × BA, × HY, × NS, BB ×
		{Left: nil, Right: e(BA | HY | NS), Break: false},
		{Left: allBB, Right: nil, Break: false},

		// LB21b: SY × HL
		{Left: allSY, Right: allHL, Break: false},

		// LB22: × IN
		{Left: nil, Right: allIN, Break: false},

		// LB23: (AL|HL) × NU, NU × (AL|HL)
		{Left: allALLike, Right: allNU, Break: false},
		{Left: allNU, Right: allALLike, Break: false},

		// LB23a: PR × (ID|EB|EM), (ID|EB|EM) × PO
		{Left: allPR, Right: allIdeographic, Break: false},
		{Left: allIdeographic, Right: allPO, Break: false},

		// LB24: (PR|PO) × (AL|HL), (AL|HL) × (PR|PO)
		{Left: e(PR | PO), Right: allALLike, Break: false},
		{Left: allALLike, Right: e(PR | PO), Break: false},

		// LB25 (tailored per Example 7):
		{Left: e(PO | PR), Right: allNU, Break: false},
		{Left: e(OP | HY), Right: allNU, Break: false},
		{Left: allNU, Right: e(NU | SY | IS), Break: false},
		{Left: allNU, Right: e(PO | PR), Break: false},

		// LB26: Do not break a Korean syllable.
		{Left: allJL, Right: e(JL | JV | H2 | H3), Break: false},
		{Left: e(JV | H2), Right: e(JV | JT), Break: false},
		{Left: e(JT | H3), Right: allJT, Break: false},

		// LB27: Treat Korean Syllable Block the same as ID.
		{Left: allHangul, Right: allPO, Break: false},
		{Left: allPR, Right: allHangul, Break: false},

		// LB28: (AL|HL) × (AL|HL)
		{Left: allALLike, Right: allALLike, Break: false},

		// LB28a: AP × (AK|AS|VF|VI), (AK|AS|VF|VI) × (AK|VF)
		{Left: allAP, Right: allAksara, Break: false},
		{Left: allAksara, Right: allAksaraFinal, Break: false},

		// LB29: IS × (AL|HL)
		{Left: allIS, Right: allALLike, Break: false},

		// LB30: (AL|HL|NU) × OP (non-EA), CP (non-EA) × (AL|HL|NU)
		{Left: append(allALLike, allNU...), Right: x(OP), Break: false},
		{Left: x(CP), Right: append(allALLike, allNU...), Break: false},

		// LB30b: EB × EM
		{Left: allEB, Right: allEM, Break: false},

		// LB31: ALL ÷ ALL (default break)
		{Left: nil, Right: nil, Break: true},
	}

	// Step 7: Build and write the break state table.
	table := segmenter.BuildStateTable(rules, combinedStates, int(propCount))

	// Post-processing: chain state rows default to NoMatch (rewind).
	chainStates := []uint8{
		pZW_SP,
		pOP_SP, pOP_EA_SP,
		pQU_SP,
		pCL_SP, pCP_SP, pCP_EA_SP,
		pB2_SP,
		pHL_HY,
		pRI_RI,
		pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR,
	}

	csSet := make(map[[2]int]bool)
	for _, cs := range combinedStates {
		csSet[[2]int{int(cs.Left), int(cs.Right)}] = true
	}
	for _, ls := range chainStates {
		row := int(ls) * int(propCount)
		for j := 0; j < int(propCount); j++ {
			if csSet[[2]int{int(ls), j}] {
				continue
			}
			table[row+j] = segmenter.NoMatch
		}
	}

	// Chain overrides: explicit transitions for chain state rows.
	type chainOverride struct {
		Lefts  []uint8
		Rights []uint8
		State  segmenter.BreakState
	}

	// Build "all non-SOT/EOT properties" for wildcard rights.
	all := func() []uint8 {
		var r []uint8
		for i := uint8(0); i < propCount; i++ {
			if i != pSOT && i != pEOT {
				r = append(r, i)
			}
		}
		return r
	}()

	lb6_7 := e(BK | CR | LF | NL | ZW | SP)

	allChains := p(
		pZW_SP,
		pOP_SP, pOP_EA_SP,
		pQU_SP,
		pCL_SP, pCP_SP, pCP_EA_SP,
		pB2_SP,
		pHL_HY,
		pRI_RI,
		pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR,
	)

	loserChains := p(
		pQU_SP,
		pCL_SP, pCP_SP, pCP_EA_SP,
		pB2_SP,
		pRI_RI,
		pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR,
	)

	lb13Right := e(CL | CP | EX | IS | SY)

	chainOverrides := []chainOverride{
		{Lefts: allChains, Rights: lb6_7, State: segmenter.Keep},
		{Lefts: p(pRI_RI), Rights: p(pEOT), State: segmenter.Break},
		{Lefts: loserChains, Rights: allWJ, State: segmenter.Keep},
		{Lefts: loserChains, Rights: lb13Right, State: segmenter.Keep},
		{Lefts: p(pOP_SP, pOP_EA_SP), Rights: all, State: segmenter.Keep},
		{Lefts: p(pQU_SP), Rights: allOP, State: segmenter.Keep},
		{Lefts: p(pCL_SP, pCP_SP, pCP_EA_SP), Rights: allNS, State: segmenter.Keep},
		{Lefts: p(pB2_SP), Rights: allB2, State: segmenter.Keep},
		{Lefts: p(pHL_HY), Rights: all, State: segmenter.Keep},
		{Lefts: p(pRI_RI), Rights: allRI, State: segmenter.Break},
		{Lefts: p(pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR), Rights: allQU, State: segmenter.Keep},
		{Lefts: p(pNU_Num), Rights: append(e(IN|BA|HY|NS|AL|SA|Extend|ZWJ|HL|GL|OP), allXX...), State: segmenter.Keep},
		{Lefts: p(pNU_Close_CP), Rights: append(e(AL|SA|Extend|ZWJ|HL|NU|IN|BA|HY|NS|GL|BB), allXX...), State: segmenter.Keep},
		{Lefts: p(pNU_Close_CL), Rights: e(IN | BA | HY | NS | GL | BB), State: segmenter.Keep},
		{Lefts: p(pNU_PR), Rights: e(OP | HY | NU), State: segmenter.Keep},
	}

	// Apply chain override transitions after the wipe.
	for _, co := range chainOverrides {
		for _, l := range co.Lefts {
			for _, r := range co.Rights {
				if csSet[[2]int{int(l), int(r)}] {
					continue
				}
				table[int(l)*int(propCount)+int(r)] = co.State
			}
		}
	}

	w.WriteComment(
		`breakTable is the line break state table.
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

// writeProps generates prop.go with the runtime-used property constants.
func writeProps(idx func(Class) uint8, lastCodepointProperty, pSOT, pEOT, propCount uint8) {
	w := gen.NewCodeWriter()
	defer w.WriteGoFile("prop.go", "line")

	fmt.Fprintf(w, "const (\n")
	fmt.Fprintf(w, "\tpropCount             uint8 = %d\n", propCount)
	fmt.Fprintf(w, "\tlastCodepointProperty uint8 = %d\n", lastCodepointProperty)
	fmt.Fprintf(w, "\tpSOT                  uint8 = %d\n", pSOT)
	fmt.Fprintf(w, "\tpEOT                  uint8 = %d\n", pEOT)
	fmt.Fprintf(w, "\tpSA                   uint8 = %d\n", idx(SA))
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "\tpBK uint8 = %d\n", idx(BK))
	fmt.Fprintf(w, "\tpCR uint8 = %d\n", idx(CR))
	fmt.Fprintf(w, "\tpLF uint8 = %d\n", idx(LF))
	fmt.Fprintf(w, "\tpNL uint8 = %d\n", idx(NL))
	fmt.Fprintf(w, ")\n")
}
