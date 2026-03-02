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
	gen.Repackage("gen_rules.go", "rules.go", "word")
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
	// explicitly set by combined state transitions. Lookahead states
	// (WB6/7, WB7b/7c, WB11/12) wait for a specific follow-up; if it
	// doesn't arrive, the segmenter must rewind. We overwrite Break
	// cells with NoMatch but leave Keep and combined-state cells intact.
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
