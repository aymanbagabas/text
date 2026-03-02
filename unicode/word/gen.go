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

// wbMap maps Word_Break property value strings to property indices.
var wbMap = map[string]uint8{
	"Other":              pOther,
	"CR":                 pCR,
	"LF":                 pLF,
	"Newline":            pNewline,
	"Extend":             pExtend,
	"ZWJ":                pZWJ,
	"Format":             pFormat,
	"Regional_Indicator": pRegionalIndicator,
	"Katakana":           pKatakana,
	"Hebrew_Letter":      pHebrewLetter,
	"ALetter":            pALetter,
	"Single_Quote":       pSingleQuote,
	"Double_Quote":       pDoubleQuote,
	"MidLetter":          pMidLetter,
	"MidNum":             pMidNum,
	"MidNumLet":          pMidNumLet,
	"Numeric":            pNumeric,
	"ExtendNumLet":       pExtendNumLet,
	"WSegSpace":          pWSegSpace,
}

func main() {
	gen.Init()
	gen.Repackage("gen_trieval.go", "trieval.go", "word")
	genTables()
}

func genTables() {
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "word")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Word_Break property values.
	ucd.Parse(gen.OpenUCDFile("auxiliary/WordBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		prop, ok := wbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Word_Break value %q", r, val)
		}
		props[r] = prop
	})

	// Step 2: Parse Extended_Pictographic from emoji data (needed for WB3c).
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(p *ucd.Parser) {
		if p.String(1) == "Extended_Pictographic" {
			r := p.Rune(0)
			if props[r] == pOther {
				props[r] = pExtPict
			}
		}
	})

	// Step 3: Build the property trie.
	t := triegen.NewTrie("word")
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

	// Step 4: Build and write the break state table.
	table := segmenter.BuildStateTable(rules, combinedStates, int(propCount))

	// Fill lookahead state rows with NoMatch (rewind) for cells not
	// explicitly set by combined state transitions.
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
	fmt.Fprintf(w, "const stride = %d\n", propCount)
}

// ---------------------------------------------------------------------------
// Word boundary rules (UAX #29)
// ---------------------------------------------------------------------------

func p(props ...uint8) []uint8 { return props }

// wb4Absorb maps each base property that participates in WB4 absorption
// to its _ZWJ and Extend/Format targets.
var wb4Absorb = []struct {
	base   uint8
	zwj    uint8
	extFmt uint8
}{
	{pALetter, pALetter_ZWJ, pALetter},
	{pHebrewLetter, pHebrewLetter_ZWJ, pHebrewLetter},
	{pNumeric, pNumeric_ZWJ, pNumeric},
	{pKatakana, pKatakana_ZWJ, pKatakana},
	{pExtendNumLet, pExtendNumLet_ZWJ, pExtendNumLet},
	{pRegionalIndicator, pRegionalIndicator_ZWJ, pRegionalIndicator},
	{pExtPict, pExtPict_ZWJ, pExtPict},
	{pWSegSpace, pWSegSpace_ZWJ, pWSegSpace_XX},
	{pWSegSpace_XX, pWSegSpace_ZWJ, pWSegSpace_XX},
}

var wb4Ignored = []uint8{pExtend, pFormat, pZWJ}

var wb4Combined = func() []segmenter.CombinedState {
	var cs []segmenter.CombinedState
	for _, a := range wb4Absorb {
		zwj, extFmt := a.zwj, a.extFmt
		cs = append(cs, segmenter.IgnoreRule{
			Props:   []uint8{a.base, a.zwj},
			Ignored: wb4Ignored,
			Target: func(_, ign uint8) uint8 {
				if ign == pZWJ {
					return zwj
				}
				return extFmt
			},
		}.Expand()...)
	}
	return cs
}()

var lookaheadStates = []uint8{pAHL_MidLetter, pHL_MidLetter, pNum_MidNum, pHL_DQ}

var allZWJ = p(pZWJ,
	pALetter_ZWJ, pHebrewLetter_ZWJ, pNumeric_ZWJ,
	pKatakana_ZWJ, pExtendNumLet_ZWJ, pRegionalIndicator_ZWJ,
	pExtPict_ZWJ, pWSegSpace_ZWJ,
)

var (
	AHLetter   = p(pALetter, pHebrewLetter, pALetter_ZWJ, pHebrewLetter_ZWJ)
	MidNumLetQ = p(pMidNumLet, pSingleQuote)
)

// rules encodes the UAX #29 word boundary rules (WB1–WB999).
// References: https://www.unicode.org/reports/tr29/#Word_Boundary_Rules
var rules = []segmenter.Rule{
	{Left: p(pSOT), Right: nil, Break: false},                                                 // WB1
	{Left: nil, Right: p(pEOT), Break: true},                                                  // WB2
	{Left: p(pCR), Right: p(pLF), Break: false},                                               // WB3
	{Left: p(pNewline, pCR, pLF), Right: nil, Break: true},                                    // WB3a
	{Left: nil, Right: p(pNewline, pCR, pLF), Break: true},                                    // WB3b
	{Left: allZWJ, Right: p(pExtPict), Break: false},                                          // WB3c
	{Left: p(pWSegSpace), Right: p(pWSegSpace), Break: false},                                 // WB3d
	{Left: nil, Right: p(pExtend, pFormat, pZWJ), Break: false},                               // WB4
	{Left: AHLetter, Right: p(pALetter, pHebrewLetter), Break: false},                         // WB5
	{Left: p(pAHL_MidLetter, pHL_MidLetter), Right: p(pALetter, pHebrewLetter), Break: false}, // WB7
	{Left: p(pHebrewLetter, pHebrewLetter_ZWJ), Right: p(pSingleQuote), Break: false},         // WB7a
	{Left: p(pHL_DQ), Right: p(pHebrewLetter), Break: false},                                  // WB7c
	{Left: p(pNumeric, pNumeric_ZWJ), Right: p(pNumeric), Break: false},                       // WB8
	{Left: AHLetter, Right: p(pNumeric), Break: false},                                        // WB9
	{Left: p(pNumeric, pNumeric_ZWJ), Right: p(pALetter, pHebrewLetter), Break: false},        // WB10
	{Left: p(pNum_MidNum), Right: p(pNumeric), Break: false},                                  // WB11
	{Left: p(pKatakana, pKatakana_ZWJ), Right: p(pKatakana), Break: false},                    // WB13
	{
		Left: p(pALetter, pHebrewLetter, pNumeric, pKatakana, pExtendNumLet, // WB13a
			pALetter_ZWJ, pHebrewLetter_ZWJ, pNumeric_ZWJ,
			pKatakana_ZWJ, pExtendNumLet_ZWJ),
		Right: p(pExtendNumLet), Break: false,
	},
	{Left: p(pExtendNumLet, pExtendNumLet_ZWJ), // WB13b
		Right: p(pALetter, pHebrewLetter, pNumeric, pKatakana), Break: false},
	{Left: p(pRegionalIndicator, pRegionalIndicator_ZWJ), Right: p(pRegionalIndicator), Break: false}, // WB15/16
	{Left: p(pRI_RI), Right: p(pRegionalIndicator), Break: true},                                      // WB15/16
	{Left: nil, Right: nil, Break: true},                                                              // WB999
}

var wb4SelfLoop = []uint8{pExtend, pFormat, pZWJ}

// combinedStates defines all state transitions for WB4 absorption and
// lookahead rules (WB6/7, WB7b/7c, WB11/12, WB15/16).
var combinedStates = func() []segmenter.CombinedState {
	cs := append([]segmenter.CombinedState{}, wb4Combined...)

	cs = append(cs, segmenter.ChainRule{ // WB6: ALetter × (MidLetter|MidNumLetQ)
		Entry: p(pALetter, pALetter_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: append(p(pMidLetter), MidNumLetQ...), State: pAHL_MidLetter},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	cs = append(cs, segmenter.ChainRule{ // WB6: HebrewLetter × (MidLetter|MidNumLet)
		Entry: p(pHebrewLetter, pHebrewLetter_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: p(pMidLetter, pMidNumLet), State: pHL_MidLetter},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	cs = append(cs, segmenter.ChainRule{ // WB7b: HebrewLetter × Double_Quote
		Entry: p(pHebrewLetter, pHebrewLetter_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: p(pDoubleQuote), State: pHL_DQ},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	cs = append(cs, segmenter.ChainRule{ // WB12: Numeric × (MidNum|MidNumLetQ)
		Entry: p(pNumeric, pNumeric_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: append(p(pMidNum), MidNumLetQ...), State: pNum_MidNum},
		},
		SelfLoop: wb4SelfLoop,
	}.Expand()...)

	cs = append(cs, segmenter.ChainRule{ // WB15/16: RI × RI
		Entry: p(pRegionalIndicator, pRegionalIndicator_ZWJ),
		Steps: []segmenter.ChainStep{
			{Props: p(pRegionalIndicator), State: pRI_RI},
		},
	}.Expand()...)

	return cs
}()
