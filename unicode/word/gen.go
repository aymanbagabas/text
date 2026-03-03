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

// wbMap maps Word_Break property value strings to Class bitflags.
var wbMap = map[string]Class{
	"Other":              Other,
	"CR":                 CR,
	"LF":                 LF,
	"Newline":            Newline,
	"Extend":             Extend,
	"ZWJ":                ZWJ,
	"Format":             Format,
	"Regional_Indicator": RI,
	"Katakana":           Katakana,
	"Hebrew_Letter":      HebrewLetter,
	"ALetter":            ALetter,
	"Single_Quote":       SingleQuote,
	"Double_Quote":       DoubleQuote,
	"MidLetter":          MidLetter,
	"MidNum":             MidNum,
	"MidNumLet":          MidNumLet,
	"Numeric":            Numeric,
	"ExtendNumLet":       ExtendNumLet,
	"WSegSpace":          WSegSpace,
}

func main() {
	gen.Init()
	genTables()
}

func genTables() {
	// --- Flattener setup ---
	flat := segmenter.NewFlattener[Class]()
	numBase := flat.AddAllBaseProperties(allBaseProperties) // 20 (Other + 19 base properties)

	idx := flat.Index

	// WB4 absorption states (assigned above base properties).
	pWSegSpace_XX := uint8(numBase)
	pALetter_ZWJ := uint8(numBase + 1)
	pHebrewLetter_ZWJ := uint8(numBase + 2)
	pNumeric_ZWJ := uint8(numBase + 3)
	pKatakana_ZWJ := uint8(numBase + 4)
	pExtendNumLet_ZWJ := uint8(numBase + 5)
	pRegionalIndicator_ZWJ := uint8(numBase + 6)
	pExtPict_ZWJ := uint8(numBase + 7)
	pWSegSpace_ZWJ := uint8(numBase + 8)
	lastCodepointProperty := pWSegSpace_ZWJ

	// Lookahead states.
	pAHL_MidLetter := lastCodepointProperty + 1
	pHL_MidLetter := lastCodepointProperty + 2
	pNum_MidNum := lastCodepointProperty + 3
	pHL_DQ := lastCodepointProperty + 4
	pRI_RI := lastCodepointProperty + 5

	// Virtual properties.
	pSOT := lastCodepointProperty + 6
	pEOT := lastCodepointProperty + 7
	propCount := int(lastCodepointProperty + 8)

	// --- Repackage gen_trieval.go → trieval.go ---
	gen.Repackage("gen_trieval.go", "trieval.go", "word")

	// --- Generate prop.go (runtime-used constants only) ---
	writeProps(idx, lastCodepointProperty,
		pALetter_ZWJ, pHebrewLetter_ZWJ, pNumeric_ZWJ,
		pKatakana_ZWJ, pExtendNumLet_ZWJ,
		pSOT, pEOT, uint8(propCount))

	// --- Build trie ---
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "word")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Word_Break property values.
	ucd.Parse(gen.OpenUCDFile("auxiliary/WordBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		cls, ok := wbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Word_Break value %q", r, val)
		}
		props[r] = idx(cls)
	})

	// Step 2: Parse Extended_Pictographic from emoji data (needed for WB3c).
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(p *ucd.Parser) {
		if p.String(1) == "Extended_Pictographic" {
			r := p.Rune(0)
			if props[r] == idx(Other) {
				props[r] = idx(ExtPict)
			}
		}
	})

	// Step 3: Build the property trie.
	t := triegen.NewTrie("word")
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

	// Step 4: Build and write the break state table.

	// WB4 absorption helpers.
	p := func(ps ...uint8) []uint8 { return ps }

	wb4Absorb := []struct {
		base   uint8
		zwj    uint8
		extFmt uint8
	}{
		{idx(ALetter), pALetter_ZWJ, idx(ALetter)},
		{idx(HebrewLetter), pHebrewLetter_ZWJ, idx(HebrewLetter)},
		{idx(Numeric), pNumeric_ZWJ, idx(Numeric)},
		{idx(Katakana), pKatakana_ZWJ, idx(Katakana)},
		{idx(ExtendNumLet), pExtendNumLet_ZWJ, idx(ExtendNumLet)},
		{idx(RI), pRegionalIndicator_ZWJ, idx(RI)},
		{idx(ExtPict), pExtPict_ZWJ, idx(ExtPict)},
		{idx(WSegSpace), pWSegSpace_ZWJ, pWSegSpace_XX},
		{pWSegSpace_XX, pWSegSpace_ZWJ, pWSegSpace_XX},
	}

	wb4Ignored := p(idx(Extend), idx(Format), idx(ZWJ))

	var wb4Combined []segmenter.CombinedState
	for _, a := range wb4Absorb {
		zwj, extFmt := a.zwj, a.extFmt
		wb4Combined = append(wb4Combined, segmenter.IgnoreRule{
			Props:   []uint8{a.base, a.zwj},
			Ignored: wb4Ignored,
			Target: func(_, ign uint8) uint8 {
				if ign == idx(ZWJ) {
					return zwj
				}
				return extFmt
			},
		}.Expand()...)
	}

	wb4SelfLoop := p(idx(Extend), idx(Format), idx(ZWJ))

	allZWJ := p(idx(ZWJ),
		pALetter_ZWJ, pHebrewLetter_ZWJ, pNumeric_ZWJ,
		pKatakana_ZWJ, pExtendNumLet_ZWJ, pRegionalIndicator_ZWJ,
		pExtPict_ZWJ, pWSegSpace_ZWJ,
	)

	flatAHLetter := p(idx(ALetter), idx(HebrewLetter), pALetter_ZWJ, pHebrewLetter_ZWJ)

	// Flatten ClassRules.
	flatRules := flat.FlattenRules(classRules)

	// Build the full rule list with virtual, combined-state, and absorption rules.
	var rules []segmenter.Rule
	rules = append(rules, segmenter.Rule{Left: p(pSOT), Right: nil, Break: false})              // WB1
	rules = append(rules, segmenter.Rule{Left: nil, Right: p(pEOT), Break: true})                // WB2

	// WB3: CR × LF (from classRules[0])
	rules = append(rules, flatRules[0])
	// WB3a: (Newline | CR | LF) ÷ (from classRules[1])
	rules = append(rules, flatRules[1])
	// WB3b: ÷ (Newline | CR | LF) (from classRules[2])
	rules = append(rules, flatRules[2])

	// WB3c: ZWJ × ExtPict — use allZWJ (includes _ZWJ absorption states)
	rules = append(rules, segmenter.Rule{Left: allZWJ, Right: p(idx(ExtPict)), Break: false})

	// WB3d: WSegSpace × WSegSpace
	rules = append(rules, segmenter.Rule{Left: p(idx(WSegSpace)), Right: p(idx(WSegSpace)), Break: false})

	// WB4: × (Extend | Format | ZWJ)
	rules = append(rules, segmenter.Rule{Left: nil, Right: p(idx(Extend), idx(Format), idx(ZWJ)), Break: false})

	// WB5: AHLetter × AHLetter (from classRules[3])
	rules = append(rules, segmenter.Rule{Left: flatAHLetter, Right: p(idx(ALetter), idx(HebrewLetter)), Break: false})

	// WB7: AHL_MidLetter/HL_MidLetter × AHLetter
	rules = append(rules, segmenter.Rule{
		Left: p(pAHL_MidLetter, pHL_MidLetter), Right: p(idx(ALetter), idx(HebrewLetter)), Break: false,
	})

	// WB7a: HebrewLetter × SingleQuote
	rules = append(rules, segmenter.Rule{
		Left: p(idx(HebrewLetter), pHebrewLetter_ZWJ), Right: p(idx(SingleQuote)), Break: false,
	})

	// WB7c: HL_DQ × HebrewLetter
	rules = append(rules, segmenter.Rule{
		Left: p(pHL_DQ), Right: p(idx(HebrewLetter)), Break: false,
	})

	// WB8: Numeric × Numeric
	rules = append(rules, segmenter.Rule{
		Left: p(idx(Numeric), pNumeric_ZWJ), Right: p(idx(Numeric)), Break: false,
	})

	// WB9: AHLetter × Numeric
	rules = append(rules, segmenter.Rule{Left: flatAHLetter, Right: p(idx(Numeric)), Break: false})

	// WB10: Numeric × AHLetter
	rules = append(rules, segmenter.Rule{
		Left: p(idx(Numeric), pNumeric_ZWJ), Right: p(idx(ALetter), idx(HebrewLetter)), Break: false,
	})

	// WB11: Num_MidNum × Numeric
	rules = append(rules, segmenter.Rule{Left: p(pNum_MidNum), Right: p(idx(Numeric)), Break: false})

	// WB13: Katakana × Katakana
	rules = append(rules, segmenter.Rule{
		Left: p(idx(Katakana), pKatakana_ZWJ), Right: p(idx(Katakana)), Break: false,
	})

	// WB13a: (ALetter|HebrewLetter|Numeric|Katakana|ExtendNumLet + _ZWJ) × ExtendNumLet
	rules = append(rules, segmenter.Rule{
		Left: p(idx(ALetter), idx(HebrewLetter), idx(Numeric), idx(Katakana), idx(ExtendNumLet),
			pALetter_ZWJ, pHebrewLetter_ZWJ, pNumeric_ZWJ,
			pKatakana_ZWJ, pExtendNumLet_ZWJ),
		Right: p(idx(ExtendNumLet)), Break: false,
	})

	// WB13b: ExtendNumLet × (ALetter|HebrewLetter|Numeric|Katakana)
	rules = append(rules, segmenter.Rule{
		Left: p(idx(ExtendNumLet), pExtendNumLet_ZWJ),
		Right: p(idx(ALetter), idx(HebrewLetter), idx(Numeric), idx(Katakana)), Break: false,
	})

	// WB15/16: RI × RI (keep), RI_RI × RI (break)
	rules = append(rules, segmenter.Rule{
		Left: p(idx(RI), pRegionalIndicator_ZWJ), Right: p(idx(RI)), Break: false,
	})
	rules = append(rules, segmenter.Rule{
		Left: p(pRI_RI), Right: p(idx(RI)), Break: true,
	})

	// WB999: Any ÷ Any
	rules = append(rules, segmenter.Rule{Break: true})

	// Build combined states.
	var combinedStates []segmenter.CombinedState
	combinedStates = append(combinedStates, wb4Combined...)

	flatMidNumLetQ := p(idx(MidNumLet), idx(SingleQuote))

	combinedStates = append(combinedStates, segmenter.ChainRule{ // WB6: ALetter × (MidLetter|MidNumLetQ)
		Entry: p(idx(ALetter), pALetter_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: append(p(idx(MidLetter)), flatMidNumLetQ...), State: pAHL_MidLetter},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	combinedStates = append(combinedStates, segmenter.ChainRule{ // WB6: HebrewLetter × (MidLetter|MidNumLet)
		Entry: p(idx(HebrewLetter), pHebrewLetter_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(MidLetter), idx(MidNumLet)), State: pHL_MidLetter},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	combinedStates = append(combinedStates, segmenter.ChainRule{ // WB7b: HebrewLetter × Double_Quote
		Entry: p(idx(HebrewLetter), pHebrewLetter_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(DoubleQuote)), State: pHL_DQ},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	combinedStates = append(combinedStates, segmenter.ChainRule{ // WB12: Numeric × (MidNum|MidNumLetQ)
		Entry: p(idx(Numeric), pNumeric_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: append(p(idx(MidNum)), flatMidNumLetQ...), State: pNum_MidNum},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	combinedStates = append(combinedStates, segmenter.ChainRule{ // WB15/16: RI × RI
		Entry: p(idx(RI), pRegionalIndicator_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: p(idx(RI)), State: pRI_RI},
		},
	}.Expand()...)

	table := segmenter.BuildStateTable(rules, combinedStates, int(propCount))

	// Fill lookahead state rows with NoMatch (rewind).
	lookaheadStates := []uint8{pAHL_MidLetter, pHL_MidLetter, pNum_MidNum, pHL_DQ}
	for _, ls := range lookaheadStates {
		row := int(ls) * int(propCount)
		for j := 0; j < int(propCount); j++ {
			if table[row+j] == segmenter.Break {
				table[row+j] = segmenter.NoMatch
			}
		}
	}

	w.WriteComment(
		`breakTable is the word break state table.
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
	fmt.Fprintf(w, "const stride = %d\n", int(propCount))
}

// ---------------------------------------------------------------------------
// Word boundary rules (UAX #29) — using Class bitflags
// ---------------------------------------------------------------------------

// classRules encodes the core word boundary rules using Class bitflags.
// Rules involving combined states (WB4 absorption, WB6/7, WB7b/c, WB11/12,
// WB15/16) are assembled separately in genTables since they reference
// uint8 state indices.
//
// References: https://www.unicode.org/reports/tr29/#Word_Boundary_Rules
var classRules = []segmenter.ClassRule[Class]{
	{Left: CR, Right: LF, Break: false},                    // WB3
	{Left: Newline | CR | LF, Break: true},                 // WB3a
	{Right: Newline | CR | LF, Break: true},                // WB3b
	{Left: AHLetter, Right: AHLetter, Break: false},        // WB5
}

// writeProps generates prop.go with the runtime-used property constants.
func writeProps(idx func(Class) uint8, lastCodepointProperty,
	pALetter_ZWJ, pHebrewLetter_ZWJ, pNumeric_ZWJ,
	pKatakana_ZWJ, pExtendNumLet_ZWJ,
	pSOT, pEOT, propCount uint8) {

	w := gen.NewCodeWriter()
	defer w.WriteGoFile("prop.go", "word")

	fmt.Fprintf(w, "const (\n")
	fmt.Fprintf(w, "\tpropCount             uint8 = %d\n", propCount)
	fmt.Fprintf(w, "\tlastCodepointProperty uint8 = %d\n", lastCodepointProperty)
	fmt.Fprintf(w, "\tpSOT                  uint8 = %d\n", pSOT)
	fmt.Fprintf(w, "\tpEOT                  uint8 = %d\n", pEOT)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "\tpALetter          uint8 = %d\n", idx(ALetter))
	fmt.Fprintf(w, "\tpHebrewLetter     uint8 = %d\n", idx(HebrewLetter))
	fmt.Fprintf(w, "\tpKatakana         uint8 = %d\n", idx(Katakana))
	fmt.Fprintf(w, "\tpExtendNumLet     uint8 = %d\n", idx(ExtendNumLet))
	fmt.Fprintf(w, "\tpNumeric          uint8 = %d\n", idx(Numeric))
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "\tpALetter_ZWJ      uint8 = %d\n", pALetter_ZWJ)
	fmt.Fprintf(w, "\tpHebrewLetter_ZWJ uint8 = %d\n", pHebrewLetter_ZWJ)
	fmt.Fprintf(w, "\tpNumeric_ZWJ      uint8 = %d\n", pNumeric_ZWJ)
	fmt.Fprintf(w, "\tpKatakana_ZWJ     uint8 = %d\n", pKatakana_ZWJ)
	fmt.Fprintf(w, "\tpExtendNumLet_ZWJ uint8 = %d\n", pExtendNumLet_ZWJ)
	fmt.Fprintf(w, ")\n")
}
