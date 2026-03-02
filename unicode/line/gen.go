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

// lbMap maps Line_Break property value strings to property indices.
// Entries that need synthetic splitting (OP, CP, QU) get their base index;
// gen.go post-processes them using EastAsianWidth and GeneralCategory.
var lbMap = map[string]uint8{
	"XX":  pXX,
	"BK":  pBK,
	"CR":  pCR,
	"LF":  pLF,
	"NL":  pNL,
	"SP":  pSP,
	"ZW":  pZW,
	"WJ":  pWJ,
	"GL":  pGL,
	"CL":  pCL,
	"EX":  pEX,
	"IS":  pIS,
	"SY":  pSY,
	"OP":  pOP,
	"QU":  pQU,
	"NS":  pNS,
	"HY":  pHY,
	"BA":  pBA,
	"BB":  pBB,
	"B2":  pB2,
	"IN":  pIN,
	"AL":  pAL,
	"NU":  pNU,
	"PR":  pPR,
	"PO":  pPO,
	"ID":  pID,
	"EB":  pEB,
	"EM":  pEM,
	"CB":  pCB,
	"RI":  pRI,
	"SA":  pSA,
	"HL":  pHL,
	"CJ":  pCJ,
	"AK":  pAK,
	"AP":  pAP,
	"AS":  pAS,
	"VF":  pVF,
	"VI":  pVI,
	"CP":  pCP,
	"AI":  pAL,      // LB1: AI → AL
	"SG":  pXX,      // LB1: SG → XX
	"CM":  pExtend,  // LB9: CM treated as Extend; LB10: remaining CM → AL
	"ZWJ": pZWJ,     // LB9: ZWJ
	"H2":  pH2,      // LB26/27: Hangul LV syllable
	"H3":  pH3,      // LB26/27: Hangul LVT syllable
	"JL":  pJL,      // LB26/27: Hangul L Jamo
	"JV":  pJV,      // LB26/27: Hangul V Jamo
	"JT":  pJT,      // LB26/27: Hangul T Jamo
	"HH":  pHY,      // Unambiguous Hyphen → HY
}

func main() {
	gen.Init()
	gen.Repackage("gen_trieval.go", "trieval.go", "line")
	genTables()
}

func genTables() {
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "line")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Line_Break property values.
	ucd.Parse(gen.OpenUCDFile("LineBreak.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		prop, ok := lbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Line_Break value %q", r, val)
		}
		props[r] = prop
	})

	// Step 2: Parse East_Asian_Width for synthetic OP_EA, CP_EA.
	eaw := make([]byte, unicode.MaxRune+1) // 'F', 'H', 'W', 'N', 'A', 'Na'
	ucd.Parse(gen.OpenUCDFile("EastAsianWidth.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		if len(val) > 0 {
			eaw[r] = val[0]
		}
	})

	// Step 3: Parse General_Category for QU_PI, QU_PF.
	gc := make([]string, unicode.MaxRune+1)
	ucd.Parse(gen.OpenUCDFile("UnicodeData.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		gc[r] = p.String(ucd.GeneralCategory)
	})

	// Step 4: Parse Extended_Pictographic from emoji-data.txt.
	// Used for resolving LB30b sub-rule 2:
	//   [\p{Extended_Pictographic}&\p{Cn}] × EM
	// Only unassigned (gc=Cn) Extended_Pictographic characters get EB
	// treatment. Assigned characters keep their original LB property.
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(p *ucd.Parser) {
		if p.String(1) == "Extended_Pictographic" {
			r := p.Rune(0)
			if gc[r] == "" {
				props[r] = pEB
			}
		}
	})

	// Step 5: Create synthetic properties.
	for r := rune(0); r <= unicode.MaxRune; r++ {
		isEA := eaw[r] == 'F' || eaw[r] == 'H' || eaw[r] == 'W'
		switch props[r] {
		case pOP:
			if isEA {
				props[r] = pOP_EA
			}
		case pCP:
			if isEA {
				props[r] = pCP_EA
			}
		case pQU:
			switch gc[r] {
			case "Pi":
				props[r] = pQU_PI
			case "Pf":
				props[r] = pQU_PF
			}
		case pSA:
			// LB1: SA with gc in {Mn, Mc} → CM behavior (resolve as AL per LB10).
			// SA with other gc → keep as SA.
			if gc[r] == "Mn" || gc[r] == "Mc" {
				props[r] = pExtend
			}
		case pCJ:
			// In normal (default) strictness, CJ resolves to NS.
			// CSS loose mode would keep CJ as ID (handled via override trie).
			props[r] = pNS
		}
	}

	// LB9/LB10: Map Extend (GCB=Extend) and ZWJ to their property indices.
	// Characters with GCB=Extend or GCB=ZWJ that don't already have a
	// Line_Break property override get mapped to pExtend/pZWJ.
	ucd.Parse(gen.OpenUCDFile("auxiliary/GraphemeBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		switch val {
		case "Extend":
			// Only override if current property is XX (unassigned) or
			// a property that participates in LB9.
			if props[r] == pXX {
				props[r] = pExtend
			}
		case "ZWJ":
			props[r] = pZWJ
		}
	})

	// Step 6: Build the property trie.
	t := triegen.NewTrie("line")
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if props[r] != pXX {
			t.Insert(r, uint64(props[r]))
		}
	}

	sz, err := t.Gen(w)
	if err != nil {
		log.Fatal(err)
	}
	w.Size += sz

	// Step 7: Build and write the break state table.
	table := segmenter.BuildStateTable(rules, combinedStates, int(propCount))

	// Post-processing: chain state rows default to NoMatch (rewind).
	// Base rules that leaked into chain rows are cleared. Only combined
	// state transitions (already set) and explicit chainOverrides survive.
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

	// Apply chain override transitions after the wipe.
	// Skip cells that already have combined state entries — those take priority.
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

// ---------------------------------------------------------------------------
// Line break rules (UAX #14)
// ---------------------------------------------------------------------------
//
// BuildStateTable applies rules in reverse order: the FIRST rule in the
// slice has the HIGHEST priority. Rules are ordered accordingly.
//
// The line break algorithm uses three mechanisms:
//
// 1. IgnoreRule (LB9): Extend/ZWJ absorption. Base properties absorb
//    adjacent Extend/ZWJ characters, keeping the base identity.
//    Exception: BK, CR, LF, NL, SP, ZW do NOT absorb.
//
// 2. ChainRule: Multi-character patterns like ZW SP* ÷ (LB8),
//    OP SP* × (LB14), CL/CP SP* × NS (LB16), etc.
//
// 3. Plain rules (LB4–LB31): Pairwise left×right rules.

func p(props ...uint8) []uint8 { return props }

// lb9Excluded lists properties that do NOT participate in LB9 absorption.
var lb9Excluded = map[uint8]bool{
	pBK: true, pCR: true, pLF: true, pNL: true, pSP: true, pZW: true,
}

// lb9Absorb maps each base property to its absorption target.
// For simplicity, X (Extend|ZWJ)* → X_XX for all participating bases.
var lb9Absorb = []struct {
	base uint8
	xx   uint8
}{
	{pAL, pAL_XX},
	{pHL, pHL_XX},
	{pNU, pNU_XX},
	{pID, pID_XX},
	{pEB, pEB_XX},
	{pRI, pRI_XX},
	{pOP, pOP_XX},
	{pOP_EA, pOP_EA_XX},
	{pCP, pCP_XX},
	{pCP_EA, pCP_EA_XX},
	{pCL, pCL_XX},
	{pBA, pBA_XX},
	{pHY, pHY_XX},
	{pBB, pBB_XX},
	{pB2, pB2_XX},
	{pSY, pSY_XX},
	{pIS, pIS_XX},
	{pPR, pPR_XX},
	{pPO, pPO_XX},
	{pIN, pIN_XX},
	{pGL, pGL_XX},
	{pWJ, pWJ_XX},
	{pNS, pNS_XX},
	{pEX, pEX_XX},
	{pQU, pQU_XX},
	{pQU_PI, pQU_PI_XX},
	{pQU_PF, pQU_PF_XX},
	{pCB, pCB_XX},
	{pSA, pSA_XX},
	{pCJ, pCJ_XX},
	{pAK, pAK_XX},
	{pAP, pAP_XX},
	{pAS, pAS_XX},
	{pVF, pVF_XX},
	{pVI, pVI_XX},
	{pEM, pEM_XX},
	{pJL, pJL_XX},
	{pJV, pJV_XX},
	{pJT, pJT_XX},
	{pH2, pH2_XX},
	{pH3, pH3_XX},
	{pXX, pXX_XX},
}

// lb9Ignored are properties absorbed by LB9.
var lb9Ignored = p(pExtend, pZWJ)

// chainOverride describes explicit transitions for chain state rows that
// survive the NoMatch wipe. Applied AFTER post-processing clears chain rows.
type chainOverride struct {
	Lefts  []uint8
	Rights []uint8
	State  segmenter.BreakState
}

// chainStates lists combined states whose rows should default to NoMatch
// instead of Break (post-processing converts Break→NoMatch).
var chainStates = []uint8{
	pZW_SP,
	pOP_SP, pOP_EA_SP,
	pQU_PI_SP,
	pCL_SP, pCP_SP, pCP_EA_SP,
	pB2_SP,
	pHL_HY,
	pRI_RI,
	pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR,
}

// chainOverrides lists explicit transitions for chain state rows.
// Applied after post-processing wipes all chain rows to NoMatch.
//
// Every chain row gets back LB6 (× BK/CR/LF/NL = Keep) and LB7 (× ZW = Keep)
// because those are higher-priority universal rules that must survive in chain
// contexts. Chain-specific transitions are layered on top.
var chainOverrides = func() []chainOverride {
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

	// universalKeeps: LB6 (× BK/CR/LF/NL), LB7 (× SP/ZW).
	lb6_7 := p(pBK, pCR, pLF, pNL, pZW, pSP)

	// allChains is all chain state properties that need base-rule overrides.
	allChains := p(
		pZW_SP,
		pOP_SP, pOP_EA_SP,
		pQU_PI_SP,
		pCL_SP, pCP_SP, pCP_EA_SP,
		pB2_SP,
		pHL_HY,
		pRI_RI,
		pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR,
	)

	// loserChains: chain states that only match specific right props.
	// These still need universal high-priority keeps: LB6/7, LB11, LB13.
	// (ZW_SP excluded: LB8 "ZW SP* ÷" intentionally breaks before everything.)
	loserChains := p(
		pQU_PI_SP,
		pCL_SP, pCP_SP, pCP_EA_SP,
		pB2_SP,
		pRI_RI,
		pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR,
	)

	// LB13 rights: × CL/CP/EX/IS/SY = Keep.
	lb13Right := func() []uint8 {
		var r []uint8
		r = append(r, allCL...)
		r = append(r, allCP...)
		r = append(r, allEX...)
		r = append(r, allIS...)
		r = append(r, allSY...)
		return r
	}()

	return []chainOverride{
		// Universal: all chain rows × BK/CR/LF/NL/ZW = Keep (LB6/LB7).
		{Lefts: allChains, Rights: lb6_7, State: segmenter.Keep},

		// LB2: RI_RI × EOT = Break. Non-intermediate chain states need
		// explicit Break at EOT to avoid incorrect rewind. (Intermediate
		// states handle EOT correctly via marker advancement.)
		{Lefts: p(pRI_RI), Rights: p(pEOT), State: segmenter.Break},

		// LB11: × WJ = Keep (all loser chains).
		{Lefts: loserChains, Rights: allWJ, State: segmenter.Keep},

		// LB13: × CL/CP/EX/IS/SY = Keep (all loser chains).
		{Lefts: loserChains, Rights: lb13Right, State: segmenter.Keep},

		// LB14: OP_SP/OP_EA_SP × anything = Keep
		{Lefts: p(pOP_SP, pOP_EA_SP), Rights: all, State: segmenter.Keep},

		// LB15: QU_PI_SP × OP/OP_EA = Keep (Unicode 15.0.0: QU SP* × OP)
		{Lefts: p(pQU_PI_SP), Rights: allOP, State: segmenter.Keep},

		// LB16: CL_SP/CP_SP/CP_EA_SP × NS = Keep
		{Lefts: p(pCL_SP, pCP_SP, pCP_EA_SP), Rights: allNS, State: segmenter.Keep},

		// LB17: B2_SP × B2 = Keep
		{Lefts: p(pB2_SP), Rights: allB2, State: segmenter.Keep},

		// LB21a: HL_HY × anything = Keep
		{Lefts: p(pHL_HY), Rights: all, State: segmenter.Keep},

		// LB30a: RI_RI × RI = Break (pair consumed)
		{Lefts: p(pRI_RI), Rights: allRI, State: segmenter.Break},

		// LB25 chain states need base-rule behavior for transitions that
		// don't match the numeric chain. Without these, NoMatch would
		// rewind and break where the base rules say keep.
		//
		// NU_Num acts like NU for non-chain transitions:
		//   LB19: × QU, QU × (all QU keeps)
		//   LB22: × IN
		//   LB21: × BA, × HY, × NS
		//   LB23: NU × (AL|HL)
		//   LB30: NU × OP (non-EA)
		//   LB24: NU(=AL/HL-like) — not needed, NU is not AL/HL
		//   LB12: × GL (when not SP/BA/HY)
		{Lefts: p(pNU_Num, pNU_Close_CL, pNU_Close_CP, pNU_PR), Rights: allQU, State: segmenter.Keep},
		{Lefts: p(pNU_Num), Rights: func() []uint8 {
			var r []uint8
			r = append(r, allIN...)
			r = append(r, allBA...)
			r = append(r, allHY...)
			r = append(r, allNS...)
			r = append(r, allAL_HL...)
			r = append(r, allGL...)
			r = append(r, p(pOP, pOP_XX)...)
			return r
		}(), State: segmenter.Keep},

		// NU_Close acts like CP (non-EA) for non-chain transitions:
		//   LB30: CP × (AL|HL|NU)
		//   LB16: CP × NS (already via loserChains LB13-like keeps)
		//   LB19: × QU (covered above)
		//   LB22: × IN, LB21: × BA/HY/NS, LB12: × GL
		// NU_Close_CP acts like CP (non-EA) for non-chain transitions:
		//   LB30: CP × (AL|HL|NU)
		{Lefts: p(pNU_Close_CP), Rights: func() []uint8 {
			var r []uint8
			r = append(r, allAL_HL...)
			r = append(r, allNU...)
			r = append(r, allIN...)
			r = append(r, allBA...)
			r = append(r, allHY...)
			r = append(r, allNS...)
			r = append(r, allGL...)
			r = append(r, allBB...)
			return r
		}(), State: segmenter.Keep},
		// NU_Close_CL acts like CL: no LB30 keeps, but still needs
		// base rule keeps for transitions other than AL/HL/NU.
		{Lefts: p(pNU_Close_CL), Rights: func() []uint8 {
			var r []uint8
			r = append(r, allIN...)
			r = append(r, allBA...)
			r = append(r, allHY...)
			r = append(r, allNS...)
			r = append(r, allGL...)
			r = append(r, allBB...)
			return r
		}(), State: segmenter.Keep},

		// LB25 (tailored): NU_PR × (OP|HY|NU) = Keep
		// After NU chain → PR/PO → pNU_PR, allow (PO|PR) × OP, (PO|PR) × HY,
		// (PO|PR) × NU from the tailored regex.
		{Lefts: p(pNU_PR), Rights: func() []uint8 {
			var r []uint8
			r = append(r, allOP...)
			r = append(r, allHY...)
			r = append(r, allNU...)
			return r
		}(), State: segmenter.Keep},
	}
}()

// allOP groups all OP variants (base and absorption).
var allOP = p(pOP, pOP_EA, pOP_XX, pOP_EA_XX)

// allCP groups all CP variants (base and absorption).
var allCP = p(pCP, pCP_EA, pCP_XX, pCP_EA_XX)

// allCL groups all CL variants.
var allCL = p(pCL, pCL_XX)

// allAL groups all AL-like properties (including absorption states).
// LB10: unattached Extend/ZWJ (after BK/CR/LF/NL/SP/ZW) resolve to AL.
var allAL = p(pAL, pAL_XX, pSA, pSA_XX, pXX, pXX_XX, pExtend, pZWJ)

// allHL groups all HL variants.
var allHL = p(pHL, pHL_XX)

// allAL_HL groups AL and HL together (for rules that treat them alike).
var allAL_HL = append(append(p(), allAL...), allHL...)

// allNU groups all NU variants.
var allNU = p(pNU, pNU_XX)

// allPR groups all PR variants.
var allPR = p(pPR, pPR_XX)

// allPO groups all PO variants.
var allPO = p(pPO, pPO_XX)

// allID groups all ID variants.
var allID = p(pID, pID_XX)

// allEB groups all EB variants.
var allEB = p(pEB, pEB_XX)

// allEM groups all EM variants.
var allEM = p(pEM, pEM_XX)

// allIS groups all IS variants.
var allIS = p(pIS, pIS_XX)

// allSY groups all SY variants.
var allSY = p(pSY, pSY_XX)

// allHY groups all HY variants.
var allHY = p(pHY, pHY_XX)

// allBA groups all BA variants.
var allBA = p(pBA, pBA_XX)

// allBB groups all BB variants.
var allBB = p(pBB, pBB_XX)

// allIN groups all IN variants.
var allIN = p(pIN, pIN_XX)

// allNS groups all NS variants.
var allNS = p(pNS, pNS_XX)

// allRI groups all RI variants.
var allRI = p(pRI, pRI_XX)

// allCB groups all CB variants.
var allCB = p(pCB, pCB_XX)

// allGL groups all GL variants.
var allGL = p(pGL, pGL_XX)

// allWJ groups all WJ variants.
var allWJ = p(pWJ, pWJ_XX)

// allB2 groups all B2 variants.
var allB2 = p(pB2, pB2_XX)

// allQU groups all QU variants (base, Pi, Pf, and their absorptions).
var allQU = p(pQU, pQU_PI, pQU_PF, pQU_XX, pQU_PI_XX, pQU_PF_XX)

// allQU_PI groups initial quote variants.
var allQU_PI = p(pQU_PI, pQU_PI_XX)

// allQU_PF groups final quote variants.
var allQU_PF = p(pQU_PF, pQU_PF_XX)

// allEX groups all EX variants.
var allEX = p(pEX, pEX_XX)

// allAK groups all AK variants.
var allAK = p(pAK, pAK_XX)

// allAP groups all AP variants.
var allAP = p(pAP, pAP_XX)

// allAS groups all AS variants.
var allAS = p(pAS, pAS_XX)

// allVF groups all VF variants.
var allVF = p(pVF, pVF_XX)

// allVI groups all VI variants.
var allVI = p(pVI, pVI_XX)

// allCJ groups all CJ variants.
var allCJ = p(pCJ, pCJ_XX)

// allJL groups all JL variants.
var allJL = p(pJL, pJL_XX)

// allJV groups all JV variants.
var allJV = p(pJV, pJV_XX)

// allJT groups all JT variants.
var allJT = p(pJT, pJT_XX)

// allH2 groups all H2 variants.
var allH2 = p(pH2, pH2_XX)

// allH3 groups all H3 variants.
var allH3 = p(pH3, pH3_XX)

// allHangul groups Hangul syllable types that act as ID for LB27.
var allHangul = func() []uint8 {
	var r []uint8
	r = append(r, allJL...)
	r = append(r, allJV...)
	r = append(r, allJT...)
	r = append(r, allH2...)
	r = append(r, allH3...)
	return r
}()

// rules encodes the UAX #14 line break rules (LB1–LB31).
//
// IMPORTANT: BuildStateTable applies rules in reverse order, so the FIRST
// rule has the HIGHEST priority.
//
// LB9 absorption and chain rules are handled by combinedStates.
// LB10: "Treat any remaining Combining_Mark or ZWJ as AL" — handled by
// mapping unattached Extend/ZWJ to AL in the rule table (they get pExtend/pZWJ
// which participate in rules as if they were AL).
//
// References: https://www.unicode.org/reports/tr29/#Line_Breaking
// and https://www.unicode.org/reports/tr14/
var rules = []segmenter.Rule{
	// LB1: sot — don't break at start.
	{Left: p(pSOT), Right: nil, Break: false},

	// LB2: Any ÷ eot — break at end.
	{Left: nil, Right: p(pEOT), Break: true},

	// LB4: BK ÷
	{Left: p(pBK), Right: nil, Break: true},

	// LB5: CR × LF
	{Left: p(pCR), Right: p(pLF), Break: false},
	// LB5: CR ÷, LF ÷, NL ÷
	{Left: p(pCR, pLF, pNL), Right: nil, Break: true},

	// LB6: × (BK | CR | LF | NL)
	{Left: nil, Right: p(pBK, pCR, pLF, pNL), Break: false},

	// LB7: × (SP | ZW)
	{Left: nil, Right: p(pSP, pZW), Break: false},

	// LB8: ZW SP* ÷ — chain rule handles the SP* case; base case ZW ÷ here.
	{Left: p(pZW), Right: nil, Break: true},

	// LB8a: ZWJ × — keep after ZWJ (part of LB9 absorption, but also
	// needs explicit rule for unattached ZWJ).
	{Left: p(pZWJ), Right: nil, Break: false},

	// LB9: handled by IgnoreRule in combinedStates.
	// LB10: Extend/ZWJ treated as AL — the rules below use pExtend/pZWJ
	// in positions where AL would appear, giving them AL-like behavior.

	// LB11: × WJ, WJ ×
	{Left: nil, Right: allWJ, Break: false},
	{Left: allWJ, Right: nil, Break: false},

	// LB12: GL ×
	{Left: allGL, Right: nil, Break: false},

	// LB12a: [^SP BA HY] × GL
	// Specific rule first (higher priority): SP/BA/HY × GL = Break.
	{Left: append(append(p(pSP), allBA...), allHY...), Right: allGL, Break: true},
	// Default: × GL = Keep.
	{Left: nil, Right: allGL, Break: false},

	// LB13 (tailored per Example 7): [^NU] × CL/CP/IS/SY, × EX.
	// NU is excluded from the left side for CL/CP so that
	// NU × CL/CP falls through to LB25 chain handling.
	// NU × IS/SY is kept by LB25 pairwise, so IS/SY stay universal here.
	// EX remains universal.
	{Left: nil, Right: allEX, Break: false},
	// NU × (CL|CP) = Break (override), so numeric context chain handles it.
	{Left: allNU, Right: append(append(p(), allCL...), allCP...), Break: true},
	// × (CL|CP|IS|SY) = Keep (base).
	{Left: nil, Right: append(append(append(allCL, allCP...), allIS...), allSY...), Break: false},

	// LB14: OP SP* × — chain handles SP* case; base rule for zero SPs.
	{Left: allOP, Right: nil, Break: false},
	// LB15a: QU_PI SP* × — chain handles SP* case; base rule not needed
	// because QU × anything is already covered by LB19a.
	// LB15b: × QU_PF — handled by chain (intermediate).
	// LB16: (CL|CP) SP* × NS — chain rule handles SP* case; base rule for zero SPs.
	{Left: append(append(p(), allCL...), allCP...), Right: allNS, Break: false},
	// LB17: B2 SP* × B2 — chain rule handles SP* case.

	// LB17: B2 SP* × B2 — chain rule handles SP* case.
	// Direct case B2 × B2 (no SP) needs explicit rule.
	{Left: allB2, Right: allB2, Break: false},

	// LB18: SP ÷
	{Left: p(pSP), Right: nil, Break: true},

	// LB19a: × QU ; QU × (simplified: treat all QU variants)
	{Left: nil, Right: allQU, Break: false},
	{Left: allQU, Right: nil, Break: false},

	// LB20: ÷ CB, CB ÷
	{Left: nil, Right: allCB, Break: true},
	{Left: allCB, Right: nil, Break: true},

	// LB21: × BA, × HY, × NS, BB ×
	{Left: nil, Right: append(append(allBA, allHY...), allNS...), Break: false},
	{Left: allBB, Right: nil, Break: false},

	// LB21a: HL (HY|BA) × — handled by chain rule.

	// LB21b: SY × HL
	{Left: allSY, Right: allHL, Break: false},

	// LB22: × IN
	{Left: nil, Right: allIN, Break: false},

	// LB23: (AL|HL) × NU, NU × (AL|HL)
	{Left: allAL_HL, Right: allNU, Break: false},
	{Left: allNU, Right: allAL_HL, Break: false},

	// LB23a: PR × (ID|EB|EM), (ID|EB|EM) × PO
	{Left: allPR, Right: func() []uint8 {
		var r []uint8
		r = append(r, allID...)
		r = append(r, allEB...)
		r = append(r, allEM...)
		return r
	}(), Break: false},
	{Left: func() []uint8 {
		var r []uint8
		r = append(r, allID...)
		r = append(r, allEB...)
		r = append(r, allEM...)
		return r
	}(), Right: allPO, Break: false},

	// LB24: (PR|PO) × (AL|HL), (AL|HL) × (PR|PO)
	{Left: append(append(p(), allPR...), allPO...), Right: allAL_HL, Break: false},
	{Left: allAL_HL, Right: append(append(p(), allPR...), allPO...), Break: false},

	// LB25 (tailored per Example 7): numeric context regex
	// (PR|PO)? (OP|HY)? NU (NU|SY|IS)* (CL|CP)? (PR|PO)?
	//
	// Pairwise rules (handled here):
	//   (PO|PR) × NU  (direct prefix → numeric)
	//   (OP|HY) × NU
	//   NU × (NU|SY|IS)
	//
	// Note: (PO|PR) × OP and (PO|PR) × HY are NOT pairwise because the
	// tailored regex requires them to be followed by NU. These are handled
	// by the OP_SP chain (LB14) when NU follows, or by a PO/PR-specific
	// chain for the HY case.
	//
	// Chain rules (in combinedStates):
	//   NU (NU|SY|IS)* × (NU|SY|IS|CL|CP)  → pNU_Num
	//   NU (NU|SY|IS)* (CL|CP)? × (PO|PR)  → pNU_Num / pNU_Close
	{Left: append(append(p(), allPO...), allPR...), Right: allNU, Break: false},
	{Left: append(append(p(), allOP...), allHY...), Right: allNU, Break: false},
	{Left: allNU, Right: append(append(allNU, allSY...), allIS...), Break: false},
	// NU × (PO|PR): zero-length body (from last subrule with empty body/close)
	{Left: allNU, Right: append(append(p(), allPO...), allPR...), Break: false},

	// LB26: Do not break a Korean syllable.
	// JL × (JL|JV|H2|H3)
	{Left: allJL, Right: func() []uint8 {
		var r []uint8
		r = append(r, allJL...)
		r = append(r, allJV...)
		r = append(r, allH2...)
		r = append(r, allH3...)
		return r
	}(), Break: false},
	// (JV|H2) × (JV|JT)
	{Left: append(append(p(), allJV...), allH2...), Right: append(append(p(), allJV...), allJT...), Break: false},
	// (JT|H3) × JT
	{Left: append(append(p(), allJT...), allH3...), Right: allJT, Break: false},

	// LB27: Treat Korean Syllable Block the same as ID.
	// (JL|JV|JT|H2|H3) × PO
	{Left: allHangul, Right: allPO, Break: false},
	// PR × (JL|JV|JT|H2|H3)
	{Left: allPR, Right: allHangul, Break: false},

	// LB28: (AL|HL) × (AL|HL)
	{Left: allAL_HL, Right: allAL_HL, Break: false},

	// LB28a: AP × (AK|AS|VF|VI), (AK|AS|VF|VI) × (AK|VF)
	{Left: allAP, Right: func() []uint8 {
		var r []uint8
		r = append(r, allAK...)
		r = append(r, allAS...)
		r = append(r, allVF...)
		r = append(r, allVI...)
		return r
	}(), Break: false},
	{Left: func() []uint8 {
		var r []uint8
		r = append(r, allAK...)
		r = append(r, allAS...)
		r = append(r, allVF...)
		r = append(r, allVI...)
		return r
	}(), Right: append(append(p(), allAK...), allVF...), Break: false},

	// LB29: IS × (AL|HL)
	{Left: allIS, Right: allAL_HL, Break: false},

	// LB30: (AL|HL|NU) × OP (non-EA), CP (non-EA) × (AL|HL|NU)
	{Left: append(append(allAL_HL, allNU...), pExtend, pZWJ), Right: p(pOP, pOP_XX), Break: false},
	{Left: p(pCP, pCP_XX), Right: append(append(allAL_HL, allNU...), pExtend, pZWJ), Break: false},

	// LB30a: RI × RI — handled by chain rule (paired).

	// LB30b: EB × EM
	{Left: allEB, Right: allEM, Break: false},

	// LB31: ALL ÷ ALL (default break)
	{Left: nil, Right: nil, Break: true},
}

// combinedStates wires up LB9 absorption and all chain rules.
var combinedStates = func() []segmenter.CombinedState {
	var cs []segmenter.CombinedState

	// --- LB9: X (Extend|ZWJ)* → X_XX ---
	// For each base property that participates, absorb Extend/ZWJ.
	for _, a := range lb9Absorb {
		xx := a.xx
		cs = append(cs, segmenter.IgnoreRule{
			Props:   p(a.base, a.xx),
			Ignored: lb9Ignored,
			Target:  func(_, _ uint8) uint8 { return xx },
		}.Expand()...)
	}

	// --- LB8: ZW SP* ÷ ---
	cs = append(cs, segmenter.ChainRule{
		Entry: p(pZW),
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pZW_SP},
		},
		Interm: true,
	}.Expand()...)
	// ZW_SP self-loop on SP.
	cs = append(cs, segmenter.CombinedState{Left: pZW_SP, Right: pSP, State: pZW_SP, Interm: true})

	// --- LB14: OP SP* × ---
	cs = append(cs, segmenter.ChainRule{
		Entry: p(pOP, pOP_XX),
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pOP_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pOP_SP, Right: pSP, State: pOP_SP, Interm: true})
	cs = append(cs, segmenter.ChainRule{
		Entry: p(pOP_EA, pOP_EA_XX),
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pOP_EA_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pOP_EA_SP, Right: pSP, State: pOP_EA_SP, Interm: true})

	// --- LB15: QU SP* × OP (Unicode 15.0.0 version) ---
	// After QU, skip spaces, then keep before OP.
	cs = append(cs, segmenter.ChainRule{
		Entry: allQU,
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pQU_PI_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pQU_PI_SP, Right: pSP, State: pQU_PI_SP, Interm: true})

	// LB15b is not used in Unicode 15.0.0. The QU_PF intermediate lookahead
	// is part of LB15a/15b split introduced in Unicode 15.1+.
	// For 15.0.0, QU_PF is treated as regular QU via LB19: × QU; QU ×.

	// --- LB16: (CL|CP) SP* × NS ---
	cs = append(cs, segmenter.ChainRule{
		Entry: allCL,
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pCL_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pCL_SP, Right: pSP, State: pCL_SP, Interm: true})

	cs = append(cs, segmenter.ChainRule{
		Entry: p(pCP, pCP_XX),
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pCP_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pCP_SP, Right: pSP, State: pCP_SP, Interm: true})

	cs = append(cs, segmenter.ChainRule{
		Entry: p(pCP_EA, pCP_EA_XX),
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pCP_EA_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pCP_EA_SP, Right: pSP, State: pCP_EA_SP, Interm: true})

	// --- LB17: B2 SP* × B2 ---
	cs = append(cs, segmenter.ChainRule{
		Entry: allB2,
		Steps: []segmenter.ChainStep{
			{Props: p(pSP), State: pB2_SP},
		},
		Interm: true,
	}.Expand()...)
	cs = append(cs, segmenter.CombinedState{Left: pB2_SP, Right: pSP, State: pB2_SP, Interm: true})

	// --- LB21a: HL (HY|BA) × ---
	cs = append(cs, segmenter.ChainRule{
		Entry: allHL,
		Steps: []segmenter.ChainStep{
			{Props: append(append(p(), allHY...), allBA...), State: pHL_HY},
		},
		Interm: true,
	}.Expand()...)

	// --- LB30a: RI × RI (paired) ---
	cs = append(cs, segmenter.ChainRule{
		Entry: allRI,
		Steps: []segmenter.ChainStep{
			{Props: p(pRI), State: pRI_RI},
		},
	}.Expand()...)

	// --- LB25 (tailored): NU (NU|SY|IS)* × (NU|SY|IS|CL|CP) ---
	// NU × (NU|SY|IS) enters pNU_Num via combined state.
	// The base pairwise rule already handles NU × (NU|SY|IS) = Keep,
	// but we override to enter the chain state.
	nuBodyRight := func() []uint8 {
		var r []uint8
		r = append(r, allNU...)
		r = append(r, allSY...)
		r = append(r, allIS...)
		return r
	}()
	nuCLRight := append(p(), allCL...)
	nuCPRight := append(p(), allCP...)
	for _, l := range allNU {
		for _, r := range nuBodyRight {
			cs = append(cs, segmenter.CombinedState{Left: l, Right: r, State: pNU_Num, Interm: true})
		}
		for _, r := range nuCLRight {
			cs = append(cs, segmenter.CombinedState{Left: l, Right: r, State: pNU_Close_CL, Interm: true})
		}
		for _, r := range nuCPRight {
			cs = append(cs, segmenter.CombinedState{Left: l, Right: r, State: pNU_Close_CP, Interm: true})
		}
	}
	// pNU_Num self-loops on NU/SY/IS.
	for _, r := range nuBodyRight {
		cs = append(cs, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_Num, Interm: true})
	}
	// pNU_Num × CL → pNU_Close_CL, pNU_Num × CP → pNU_Close_CP.
	for _, r := range nuCLRight {
		cs = append(cs, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_Close_CL, Interm: true})
	}
	for _, r := range nuCPRight {
		cs = append(cs, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_Close_CP, Interm: true})
	}

	// LB25: NU/NU_Num/NU_Close × (PO|PR) → pNU_PR.
	// This tracks that we're in a numeric context followed by PO/PR,
	// so that (PO|PR) × OP/HY/NU is allowed per the tailored regex.
	nuPRRight := func() []uint8 {
		var r []uint8
		r = append(r, allPO...)
		r = append(r, allPR...)
		return r
	}()
	for _, l := range allNU {
		for _, r := range nuPRRight {
			cs = append(cs, segmenter.CombinedState{Left: l, Right: r, State: pNU_PR, Interm: true})
		}
	}
	for _, r := range nuPRRight {
		cs = append(cs, segmenter.CombinedState{Left: pNU_Num, Right: r, State: pNU_PR, Interm: true})
		cs = append(cs, segmenter.CombinedState{Left: pNU_Close_CL, Right: r, State: pNU_PR, Interm: true})
		cs = append(cs, segmenter.CombinedState{Left: pNU_Close_CP, Right: r, State: pNU_PR, Interm: true})
	}

	return cs
}()
