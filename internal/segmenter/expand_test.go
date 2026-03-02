// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

import (
	"testing"
)

func TestIgnoreRuleExpand(t *testing.T) {
	// Model WB4 absorption for ALetter absorbing Extend/Format/ZWJ.
	const (
		pAL     uint8 = 0
		pExt    uint8 = 1
		pFmt    uint8 = 2
		pZWJ    uint8 = 3
		pAL_ZWJ uint8 = 10
	)

	rule := IgnoreRule{
		Props:   []uint8{pAL, pAL_ZWJ},
		Ignored: []uint8{pExt, pFmt, pZWJ},
		Target: func(base, ign uint8) uint8 {
			if ign == pZWJ {
				return pAL_ZWJ
			}
			return pAL
		},
	}

	cs := rule.Expand()

	type key struct{ L, R, S uint8 }
	got := make(map[key]bool)
	for _, c := range cs {
		got[key{c.Left, c.Right, c.State}] = true
	}

	expect := []key{
		{pAL, pExt, pAL},
		{pAL, pFmt, pAL},
		{pAL, pZWJ, pAL_ZWJ},
		{pAL_ZWJ, pExt, pAL},
		{pAL_ZWJ, pFmt, pAL},
		{pAL_ZWJ, pZWJ, pAL_ZWJ},
	}

	for _, e := range expect {
		if !got[e] {
			t.Errorf("missing: Left=%d Right=%d State=%d", e.L, e.R, e.S)
		}
	}
	if len(cs) != len(expect) {
		t.Errorf("got %d entries; want %d", len(cs), len(expect))
	}
}

func TestIgnoreRuleSimple(t *testing.T) {
	// Model SB5 absorption: Lower absorbing Extend/Format.
	const (
		pLow   uint8 = 0
		pExt   uint8 = 1
		pFmt   uint8 = 2
		pLowXX uint8 = 10
	)

	rule := IgnoreRule{
		Props:   []uint8{pLow, pLowXX},
		Ignored: []uint8{pExt, pFmt},
		Target:  func(_, _ uint8) uint8 { return pLowXX },
	}

	cs := rule.Expand()

	type key struct{ L, R, S uint8 }
	got := make(map[key]bool)
	for _, c := range cs {
		got[key{c.Left, c.Right, c.State}] = true
	}

	expect := []key{
		{pLow, pExt, pLowXX},
		{pLow, pFmt, pLowXX},
		{pLowXX, pExt, pLowXX},
		{pLowXX, pFmt, pLowXX},
	}

	for _, e := range expect {
		if !got[e] {
			t.Errorf("missing: Left=%d Right=%d State=%d", e.L, e.R, e.S)
		}
	}
	if len(cs) != len(expect) {
		t.Errorf("got %d entries; want %d", len(cs), len(expect))
	}
}

func TestChainRuleExpand(t *testing.T) {
	const (
		pAH     uint8 = 0
		pAH_ZWJ uint8 = 1
		pMid    uint8 = 2
		pSQ     uint8 = 3
		pExt    uint8 = 4
		pFmt    uint8 = 5
		pZWJ    uint8 = 6
		pAHMid  uint8 = 10
	)

	rule := ChainRule{
		Entry: []uint8{pAH, pAH_ZWJ},
		Steps: []ChainStep{
			{Props: []uint8{pMid, pSQ}, State: pAHMid},
		},
		SelfLoop: []uint8{pExt, pFmt, pZWJ},
	}

	cs := rule.Expand()

	type key struct {
		L, R, S uint8
		I       bool
	}
	got := make(map[key]bool)
	for _, c := range cs {
		got[key{c.Left, c.Right, c.State, c.Interm}] = true
	}

	expect := []key{
		{pAH, pMid, pAHMid, false},
		{pAH, pSQ, pAHMid, false},
		{pAH_ZWJ, pMid, pAHMid, false},
		{pAH_ZWJ, pSQ, pAHMid, false},
		{pAHMid, pExt, pAHMid, false},
		{pAHMid, pFmt, pAHMid, false},
		{pAHMid, pZWJ, pAHMid, false},
	}

	for _, e := range expect {
		if !got[e] {
			t.Errorf("missing: Left=%d Right=%d State=%d Interm=%v", e.L, e.R, e.S, e.I)
		}
	}
	if len(cs) != len(expect) {
		t.Errorf("got %d entries; want %d", len(cs), len(expect))
	}
}

func TestChainRuleInterm(t *testing.T) {
	const (
		pA    uint8 = 0
		pB    uint8 = 1
		pC    uint8 = 2
		pS1   uint8 = 10
		pS2   uint8 = 11
	)

	rule := ChainRule{
		Entry: []uint8{pA},
		Steps: []ChainStep{
			{Props: []uint8{pB}, State: pS1},
			{Props: []uint8{pC}, State: pS2},
		},
		Interm: true,
	}

	cs := rule.Expand()

	type key struct {
		L, R, S uint8
		I       bool
	}
	got := make(map[key]bool)
	for _, c := range cs {
		got[key{c.Left, c.Right, c.State, c.Interm}] = true
	}

	expect := []key{
		{pA, pB, pS1, true},
		{pS1, pC, pS2, true},
	}

	for _, e := range expect {
		if !got[e] {
			t.Errorf("missing: Left=%d Right=%d State=%d Interm=%v", e.L, e.R, e.S, e.I)
		}
	}
	if len(cs) != len(expect) {
		t.Errorf("got %d entries; want %d", len(cs), len(expect))
	}
}

func TestExpandAll(t *testing.T) {
	const (
		pA  uint8 = 0
		pB  uint8 = 1
		pAX uint8 = 10
		pS  uint8 = 11
	)

	ig := IgnoreRule{
		Props:   []uint8{pA},
		Ignored: []uint8{pB},
		Target:  func(_, _ uint8) uint8 { return pAX },
	}
	ch := ChainRule{
		Entry: []uint8{pA},
		Steps: []ChainStep{{Props: []uint8{pB}, State: pS}},
	}

	cs := ExpandAll(ig, ch)
	if len(cs) != 2 {
		t.Errorf("got %d entries; want 2", len(cs))
	}
}
