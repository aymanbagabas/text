// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

// Property indices for grapheme cluster break.
//
// Base properties 0–17 correspond to Grapheme_Cluster_Break values from
// the Unicode Character Database, with InCB (Indic_Conjunct_Break) sub-properties
// split out from Extend for rule GB9c. The gen.go generator assigns these
// indices when building the property trie.
//
// Combined states 18–21 are synthetic properties used by the state machine
// to track multi-character lookahead contexts (GB11, GB12/13, GB9c).
//
// SOT and EOT are virtual properties for start-of-text and end-of-text.
const (
	pOther             uint8 = iota // GCB=Other (and InCB=None)
	pCR                             // GCB=CR
	pLF                             // GCB=LF
	pControl                        // GCB=Control
	pExtend                         // GCB=Extend (and InCB=None)
	pZWJ                            // GCB=ZWJ (U+200D)
	pRegionalIndicator              // GCB=Regional_Indicator
	pPrepend                        // GCB=Prepend
	pSpacingMark                    // GCB=SpacingMark
	pL                              // GCB=L (Hangul leading jamo)
	pV                              // GCB=V (Hangul vowel jamo)
	pT                              // GCB=T (Hangul trailing jamo)
	pLV                             // GCB=LV (Hangul LV syllable)
	pLVT                            // GCB=LVT (Hangul LVT syllable)
	pExtPict                        // Extended_Pictographic=Yes
	pInCBLinker                     // InCB=Linker (subset of GCB=Extend; viramas)
	pInCBConsonant                  // InCB=Consonant (subset of GCB=Other; Indic consonants)
	pInCBExtend                     // InCB=Extend (subset of GCB=Extend; combining marks near Indic clusters)

	// Combined states for multi-character lookahead.
	pRI_RI       // after RI × RI (GB12/13: pair consumed)
	pExtPict_Ext // after ExtPict × Extend* (GB11: accumulating extends)
	pExtPict_ZWJ // after ExtPict × Extend* × ZWJ (GB11: ready for next ExtPict)
	pInCB_Linker // after Consonant × [Extend|Linker]* × Linker (GB9c)

	// Virtual properties.
	pSOT      // start of text
	pEOT      // end of text
	propCount // total number of properties (= stride)
)

const lastCodepointProperty = pInCB_Linker
