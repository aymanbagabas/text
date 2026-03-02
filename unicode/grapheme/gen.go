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

// gcbMap maps Grapheme_Cluster_Break property value strings to property indices.
var gcbMap = map[string]uint8{
	"Other":              pOther,
	"CR":                 pCR,
	"LF":                 pLF,
	"Control":            pControl,
	"Extend":             pExtend,
	"ZWJ":                pZWJ,
	"Regional_Indicator": pRegionalIndicator,
	"Prepend":            pPrepend,
	"SpacingMark":        pSpacingMark,
	"L":                  pL,
	"V":                  pV,
	"T":                  pT,
	"LV":                 pLV,
	"LVT":                pLVT,
}

func main() {
	gen.Init()
	gen.Repackage("gen_trieval.go", "trieval.go", "grapheme")
	genTables()
}

func genTables() {
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "grapheme")

	gen.WriteUnicodeVersion(w)

	// Per-codepoint property storage. Default is pOther (0).
	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Grapheme_Cluster_Break values.
	ucd.Parse(gen.OpenUCDFile("auxiliary/GraphemeBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		prop, ok := gcbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Grapheme_Cluster_Break value %q", r, val)
		}
		props[r] = prop
	})

	// Step 2: Parse Extended_Pictographic from emoji data.
	// This must run after Step 1 because some Extended_Pictographic characters
	// have GCB=Other, and we want pExtPict to override pOther for them.
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(p *ucd.Parser) {
		if p.String(1) == "Extended_Pictographic" {
			props[p.Rune(0)] = pExtPict
		}
	})

	// Step 3: Parse InCB (Indic_Conjunct_Break) from DerivedCoreProperties.
	// Format: <codepoint> ; InCB ; <value>
	// InCB=Linker is a subset of GCB=Extend (viramas).
	// InCB=Consonant is a subset of GCB=Other (Indic consonants).
	// InCB=Extend is a subset of GCB=Extend (combining marks near Indic clusters).
	// This must run last because InCB overrides the base GCB property for
	// affected codepoints.
	ucd.Parse(gen.OpenUCDFile("DerivedCoreProperties.txt"), func(p *ucd.Parser) {
		if p.String(1) != "InCB" {
			return
		}
		r := p.Rune(0)
		switch p.String(2) {
		case "Linker":
			props[r] = pInCBLinker
		case "Consonant":
			props[r] = pInCBConsonant
		case "Extend":
			props[r] = pInCBExtend
		}
	})

	// Step 4: Build the property trie.
	t := triegen.NewTrie("grapheme")
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

	// Step 5: Build and write the break state table.
	table := segmenter.BuildStateTable(rules, combinedStates, int(propCount))

	w.WriteComment(
		`breakTable is the grapheme cluster break state table.
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
// Grapheme cluster boundary rules (UAX #29)
// ---------------------------------------------------------------------------

func p(props ...uint8) []uint8 { return props }

// rules encodes the UAX #29 grapheme cluster boundary rules (GB3–GB999)
// as input to [segmenter.BuildStateTable]. Rules are listed in priority
// order; the first match wins. Combined states handle GB9c, GB11, and
// GB12/13 lookahead.
//
// References: https://www.unicode.org/reports/tr29/#Grapheme_Cluster_Boundary_Rules
var rules = []segmenter.Rule{
	{Left: p(pSOT), Right: nil, Break: false},                                    // GB1
	{Left: nil, Right: p(pEOT), Break: true},                                     // GB2
	{Left: p(pCR), Right: p(pLF), Break: false},                                  // GB3
	{Left: p(pControl, pCR, pLF), Right: nil, Break: true},                       // GB4
	{Left: nil, Right: p(pControl, pCR, pLF), Break: true},                       // GB5
	{Left: p(pL), Right: p(pL, pV, pLV, pLVT), Break: false},                    // GB6
	{Left: p(pLV, pV), Right: p(pV, pT), Break: false},                          // GB7
	{Left: p(pLVT, pT), Right: p(pT), Break: false},                             // GB8
	{Left: nil, Right: p(pExtend, pZWJ, pInCBExtend, pInCBLinker), Break: false}, // GB9
	{Left: nil, Right: p(pSpacingMark), Break: false},                            // GB9a
	{Left: p(pPrepend), Right: nil, Break: false},                                // GB9b
	{Left: p(pInCB_Linker), Right: p(pInCBConsonant), Break: false},              // GB9c
	{Left: p(pExtPict_ZWJ), Right: p(pExtPict), Break: false},                   // GB11
	{Left: p(pRegionalIndicator), Right: p(pRegionalIndicator), Break: false},    // GB12/13
	{Left: p(pRI_RI), Right: p(pRegionalIndicator), Break: true},                // GB12/13
	{Left: nil, Right: nil, Break: true},                                         // GB999
}

// combinedStates defines transitions into synthetic combined-state properties.
// These overlay the rule table to implement multi-character lookahead without
// backtracking.
var combinedStates = []segmenter.CombinedState{
	// GB12/13: RI × RI → enter pRI_RI (pair consumed; next RI will break).
	{Left: pRegionalIndicator, Right: pRegionalIndicator, State: pRI_RI},

	// GB11: ExtPict × Extend → enter pExtPict_Ext (accumulating extends).
	{Left: pExtPict, Right: pExtend, State: pExtPict_Ext},
	{Left: pExtPict, Right: pInCBExtend, State: pExtPict_Ext},
	// GB11: ExtPict_Ext × Extend → stay in pExtPict_Ext.
	{Left: pExtPict_Ext, Right: pExtend, State: pExtPict_Ext},
	{Left: pExtPict_Ext, Right: pInCBExtend, State: pExtPict_Ext},
	// GB11: ExtPict_Ext × ZWJ → enter pExtPict_ZWJ (ready for next ExtPict).
	{Left: pExtPict_Ext, Right: pZWJ, State: pExtPict_ZWJ},
	// GB11: ExtPict × ZWJ → enter pExtPict_ZWJ (no intervening Extend).
	{Left: pExtPict, Right: pZWJ, State: pExtPict_ZWJ},

	// GB9c: Consonant × Linker → enter pInCB_Linker.
	{Left: pInCBConsonant, Right: pInCBLinker, State: pInCB_Linker},
	// GB9c: InCB_Linker × Extend → stay (absorb extends within the cluster).
	{Left: pInCB_Linker, Right: pInCBExtend, State: pInCB_Linker},
	// GB9c: InCB_Linker × Linker → stay (multiple linkers allowed).
	{Left: pInCB_Linker, Right: pInCBLinker, State: pInCB_Linker},
}
