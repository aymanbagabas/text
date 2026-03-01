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
	gen.Repackage("gen_rules.go", "rules.go", "grapheme")
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
	fmt.Fprintf(w, "var breakTable = [...]int8{")
	for i, v := range table {
		if i%int(propCount) == 0 {
			fmt.Fprintf(w, "\n\t")
		}
		fmt.Fprintf(w, "%d, ", int8(v))
	}
	fmt.Fprintf(w, "\n}\n\n")

	w.WriteComment("stride is the number of columns in breakTable.")
	fmt.Fprintf(w, "const stride = %d\n", propCount)
}
