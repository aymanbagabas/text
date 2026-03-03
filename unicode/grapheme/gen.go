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

// gcbMap maps Grapheme_Cluster_Break property value strings to Class bitflags.
var gcbMap = map[string]Class{
	"Other":              Other,
	"CR":                 CR,
	"LF":                 LF,
	"Control":            Control,
	"Extend":             Extend,
	"ZWJ":                ZWJ,
	"Regional_Indicator": RI,
	"Prepend":            Prepend,
	"SpacingMark":        SpacingMark,
	"L":                  L,
	"V":                  V,
	"T":                  T,
	"LV":                 LV,
	"LVT":                LVT,
}

func main() {
	gen.Init()
	genTables()
}

func genTables() {
	// --- Flattener setup ---
	// Register all base properties. No modifiers for grapheme.
	flat := segmenter.NewFlattener[Class]()
	numBase := flat.AddAllBaseProperties(allBaseProperties) // 18 (Other + 17 base properties)

	// Combined state indices (assigned above base properties).
	pRI_RI := uint8(numBase)
	pExtPict_Ext := uint8(numBase + 1)
	pExtPict_ZWJ := uint8(numBase + 2)
	pInCB_Linker := uint8(numBase + 3)

	lastCodepointProperty := pInCB_Linker

	// Virtual properties.
	pSOT := uint8(numBase + 4)
	pEOT := uint8(numBase + 5)
	propCount := int(numBase + 6)

	// --- Property lookup helper ---
	idx := flat.Index

	// --- Repackage gen_trieval.go → trieval.go ---
	gen.Repackage("gen_trieval.go", "trieval.go", "grapheme")

	// --- Generate props.go (runtime-used constants only) ---
	writeProps(lastCodepointProperty, pSOT, pEOT, uint8(propCount))

	// --- Build trie ---
	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "grapheme")

	gen.WriteUnicodeVersion(w)

	props := make([]uint8, unicode.MaxRune+1)

	// Step 1: Parse Grapheme_Cluster_Break values.
	ucd.Parse(gen.OpenUCDFile("auxiliary/GraphemeBreakProperty.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		val := p.String(1)
		cls, ok := gcbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Grapheme_Cluster_Break value %q", r, val)
		}
		props[r] = idx(cls)
	})

	// Step 2: Parse Extended_Pictographic from emoji data.
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(p *ucd.Parser) {
		if p.String(1) == "Extended_Pictographic" {
			props[p.Rune(0)] = idx(ExtPict)
		}
	})

	// Step 3: Parse InCB (Indic_Conjunct_Break) from DerivedCoreProperties.
	ucd.Parse(gen.OpenUCDFile("DerivedCoreProperties.txt"), func(p *ucd.Parser) {
		if p.String(1) != "InCB" {
			return
		}
		r := p.Rune(0)
		switch p.String(2) {
		case "Linker":
			props[r] = idx(InCBLinker)
		case "Consonant":
			props[r] = idx(InCBConsonant)
		case "Extend":
			props[r] = idx(InCBExtend)
		}
	})

	// Step 4: Build the property trie.
	t := triegen.NewTrie("grapheme")
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

	// Step 5: Build and write the break state table.

	// Flatten ClassRules to uint8-based Rules.
	flatRules := flat.FlattenRules(classRules)

	// Build the full rule list with virtual and combined-state rules.
	rules := []segmenter.Rule{
		{Left: []uint8{pSOT}, Right: nil, Break: false}, // GB1
		{Left: nil, Right: []uint8{pEOT}, Break: true},  // GB2
	}
	rules = append(rules, flatRules...) // GB3–GB9b, GB999

	// Insert combined-state rules before GB999 (last entry).
	gb999 := rules[len(rules)-1]
	rules = rules[:len(rules)-1]
	rules = append(rules,
		segmenter.Rule{Left: []uint8{pInCB_Linker}, Right: []uint8{idx(InCBConsonant)}, Break: false}, // GB9c
		segmenter.Rule{Left: []uint8{pExtPict_ZWJ}, Right: []uint8{idx(ExtPict)}, Break: false},       // GB11
		segmenter.Rule{Left: []uint8{idx(RI)}, Right: []uint8{idx(RI)}, Break: false},                 // GB12/13
		segmenter.Rule{Left: []uint8{pRI_RI}, Right: []uint8{idx(RI)}, Break: true},                   // GB12/13
		gb999, // GB999
	)

	combinedStates := []segmenter.CombinedState{
		// GB12/13: RI × RI → enter pRI_RI (pair consumed; next RI will break).
		{Left: idx(RI), Right: idx(RI), State: pRI_RI},

		// GB11: ExtPict × Extend → enter pExtPict_Ext (accumulating extends).
		{Left: idx(ExtPict), Right: idx(Extend), State: pExtPict_Ext},
		{Left: idx(ExtPict), Right: idx(InCBExtend), State: pExtPict_Ext},
		// GB11: ExtPict_Ext × Extend → stay in pExtPict_Ext.
		{Left: pExtPict_Ext, Right: idx(Extend), State: pExtPict_Ext},
		{Left: pExtPict_Ext, Right: idx(InCBExtend), State: pExtPict_Ext},
		// GB11: ExtPict_Ext × ZWJ → enter pExtPict_ZWJ (ready for next ExtPict).
		{Left: pExtPict_Ext, Right: idx(ZWJ), State: pExtPict_ZWJ},
		// GB11: ExtPict × ZWJ → enter pExtPict_ZWJ (no intervening Extend).
		{Left: idx(ExtPict), Right: idx(ZWJ), State: pExtPict_ZWJ},

		// GB9c: Consonant × Linker → enter pInCB_Linker.
		{Left: idx(InCBConsonant), Right: idx(InCBLinker), State: pInCB_Linker},
		// GB9c: InCB_Linker × Extend → stay (absorb extends within the cluster).
		{Left: pInCB_Linker, Right: idx(InCBExtend), State: pInCB_Linker},
		// GB9c: InCB_Linker × Linker → stay (multiple linkers allowed).
		{Left: pInCB_Linker, Right: idx(InCBLinker), State: pInCB_Linker},
	}

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
	fmt.Fprintf(w, "const stride = %d\n", int(propCount))
}

// ---------------------------------------------------------------------------
// Grapheme cluster boundary rules (UAX #29) — using Class bitflags
// ---------------------------------------------------------------------------

// classRules encodes the UAX #29 grapheme cluster boundary rules (GB3–GB999)
// using Class bitflags. GB1 (SOT), GB2 (EOT), and combined-state rules
// (GB9c, GB11, GB12/13) are added separately since they reference uint8
// state indices.
//
// References: https://www.unicode.org/reports/tr29/#Grapheme_Cluster_Boundary_Rules
// writeProps generates props.go with the runtime-used property constants.
func writeProps(lastCodepointProperty, pSOT, pEOT, propCount uint8) {
	w := gen.NewCodeWriter()
	defer w.WriteGoFile("prop.go", "grapheme")

	fmt.Fprintf(w, "const (\n")
	fmt.Fprintf(w, "\tpropCount             uint8 = %d\n", propCount)
	fmt.Fprintf(w, "\tlastCodepointProperty uint8 = %d\n", lastCodepointProperty)
	fmt.Fprintf(w, "\tpSOT                  uint8 = %d\n", pSOT)
	fmt.Fprintf(w, "\tpEOT                  uint8 = %d\n", pEOT)
	fmt.Fprintf(w, ")\n")
}

var classRules = []segmenter.ClassRule[Class]{
	{Left: CR, Right: LF, Break: false},                           // GB3
	{Left: Control | CR | LF, Break: true},                        // GB4
	{Right: Control | CR | LF, Break: true},                       // GB5
	{Left: L, Right: L | V | LV | LVT, Break: false},              // GB6
	{Left: LV | V, Right: V | T, Break: false},                    // GB7
	{Left: LVT | T, Right: T, Break: false},                       // GB8
	{Right: Extend | ZWJ | InCBExtend | InCBLinker, Break: false}, // GB9
	{Right: SpacingMark, Break: false},                            // GB9a
	{Left: Prepend, Break: false},                                 // GB9b
	{Break: true},                                                 // GB999
}

