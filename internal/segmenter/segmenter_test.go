// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

import (
	"fmt"
	"testing"
)

// Test properties: two base properties (A=0, B=1), SOT=2, EOT=3.
const (
	propA   uint8 = 0
	propB   uint8 = 1
	propSOT uint8 = 2
	propEOT uint8 = 3
	tStride       = 4
)

// funcTable adapts a lookup function to the PropertyTable interface.
type funcTable func([]byte) (uint8, int)

func (f funcTable) Lookup(b []byte) (uint8, int) { return f(b) }

func lookupAB(b []byte) (uint8, int) {
	if len(b) == 0 {
		return 0, 0
	}
	if b[0] == 'A' {
		return propA, 1
	}
	return propB, 1
}

// segments collects all segments from a Segmenter as strings.
func segments(seg *Segmenter) []string {
	var out []string
	for seg.Next() {
		out = append(out, seg.Text())
	}
	return out
}

// breaks collects all break positions (end offsets) from a Segmenter.
func breaks(seg *Segmenter) []int {
	var out []int
	for seg.Next() {
		_, end := seg.Position()
		out = append(out, end)
	}
	return out
}

func TestBuildStateTable(t *testing.T) {
	rules := []Rule{
		{Left: []uint8{propA}, Right: []uint8{propA}, Break: false},
		{Left: []uint8{propA}, Right: []uint8{propB}, Break: true},
		{Left: []uint8{propB}, Right: []uint8{propA}, Break: true},
		{Left: []uint8{propB}, Right: []uint8{propB}, Break: false},
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: nil, Right: []uint8{propEOT}, Break: true},
	}

	table := BuildStateTable(rules, nil, tStride)

	if got := table[int(propA)*tStride+int(propA)]; got != Keep {
		t.Errorf("A×A: got %d; want Keep(%d)", got, Keep)
	}
	if got := table[int(propA)*tStride+int(propB)]; got != Break {
		t.Errorf("A×B: got %d; want Break(%d)", got, Break)
	}
	if got := table[int(propB)*tStride+int(propA)]; got != Break {
		t.Errorf("B×A: got %d; want Break(%d)", got, Break)
	}
	if got := table[int(propB)*tStride+int(propB)]; got != Keep {
		t.Errorf("B×B: got %d; want Keep(%d)", got, Keep)
	}
	if got := table[int(propSOT)*tStride+int(propA)]; got != Keep {
		t.Errorf("SOT×A: got %d; want Keep(%d)", got, Keep)
	}
	if got := table[int(propA)*tStride+int(propEOT)]; got != Break {
		t.Errorf("A×EOT: got %d; want Break(%d)", got, Break)
	}
}

func TestSimpleBreaks(t *testing.T) {
	rules := []Rule{
		{Left: []uint8{propA}, Right: []uint8{propA}, Break: false},
		{Left: []uint8{propA}, Right: []uint8{propB}, Break: true},
		{Left: []uint8{propB}, Right: []uint8{propA}, Break: true},
		{Left: []uint8{propB}, Right: []uint8{propB}, Break: false},
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: nil, Right: []uint8{propEOT}, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: funcTable(lookupAB),
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}

	tests := []struct {
		input string
		segs  []string
	}{
		{"", nil},
		{"A", []string{"A"}},
		{"AA", []string{"AA"}},
		{"AB", []string{"A", "B"}},
		{"BA", []string{"B", "A"}},
		{"AAA", []string{"AAA"}},
		{"AABB", []string{"AA", "BB"}},
		{"AABBA", []string{"AA", "BB", "A"}},
		{"ABAB", []string{"A", "B", "A", "B"}},
		{"BBB", []string{"BBB"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			seg := New(data, []byte(tt.input))
			got := segments(seg)
			if !stringsEqual(got, tt.segs) {
				t.Errorf("got %q; want %q", got, tt.segs)
			}
		})
	}
}

func TestBytesAndPosition(t *testing.T) {
	rules := []Rule{
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: funcTable(lookupAB),
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}

	seg := New(data, []byte("AB"))
	if !seg.Next() {
		t.Fatal("expected first segment")
	}
	if got := string(seg.Bytes()); got != "A" {
		t.Errorf("Bytes: got %q; want %q", got, "A")
	}
	if got := seg.Text(); got != "A" {
		t.Errorf("Text: got %q; want %q", got, "A")
	}
	start, end := seg.Position()
	if start != 0 || end != 1 {
		t.Errorf("Position: got (%d,%d); want (0,1)", start, end)
	}

	if !seg.Next() {
		t.Fatal("expected second segment")
	}
	start, end = seg.Position()
	if start != 1 || end != 2 {
		t.Errorf("Position: got (%d,%d); want (1,2)", start, end)
	}

	if seg.Next() {
		t.Error("expected end of input")
	}
}

func TestWildcardRules(t *testing.T) {
	rules := []Rule{
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: []uint8{propA}, Right: []uint8{propA}, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: funcTable(lookupAB),
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}

	seg := New(data, []byte("AABA"))
	got := segments(seg)
	want := []string{"AA", "B", "A"}
	if !stringsEqual(got, want) {
		t.Errorf("got %q; want %q", got, want)
	}
}

// Combined state test: model RI×RI pairing.
func TestCombinedStateRI(t *testing.T) {
	const (
		pRI    uint8 = 0
		pOther uint8 = 1
		pSOT   uint8 = 2
		pEOT   uint8 = 3
		pRI_RI uint8 = 4
		stride       = 5
	)

	lookup := funcTable(func(b []byte) (uint8, int) {
		if b[0] == 'R' {
			return pRI, 1
		}
		return pOther, 1
	})

	rules := []Rule{
		{Left: []uint8{pRI}, Right: []uint8{pRI}, Break: false},
		{Left: []uint8{pRI_RI}, Right: []uint8{pRI}, Break: true},
		{Left: []uint8{pSOT}, Right: nil, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	combined := []CombinedState{
		{Left: pRI, Right: pRI, State: pRI_RI},
	}
	table := BuildStateTable(rules, combined, stride)
	data := &RuleData{
		Properties: lookup,
		BreakTable: table,
		Stride:     stride,
		PropCount:  2,
		SOT:        pSOT,
		EOT:        pEOT,
	}

	tests := []struct {
		input string
		segs  []string
	}{
		{"RR", []string{"RR"}},
		{"RRR", []string{"RR", "R"}},
		{"RRRR", []string{"RR", "RR"}},
		{"RRRRR", []string{"RR", "RR", "R"}},
		{"RRxRR", []string{"RR", "x", "RR"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			seg := New(data, []byte(tt.input))
			got := segments(seg)
			if !stringsEqual(got, tt.segs) {
				t.Errorf("got %q; want %q", got, tt.segs)
			}
		})
	}
}

// Combined state with NoMatch: model WB6/WB7 (AHLetter × MidLetter × AHLetter).
func TestCombinedStateNoMatch(t *testing.T) {
	const (
		pAH     uint8 = 0
		pMid    uint8 = 1
		pOther  uint8 = 2
		pSOT    uint8 = 3
		pEOT    uint8 = 4
		pAH_Mid uint8 = 5
		stride        = 6
	)

	lookup := funcTable(func(b []byte) (uint8, int) {
		switch b[0] {
		case 'a':
			return pAH, 1
		case '.':
			return pMid, 1
		default:
			return pOther, 1
		}
	})

	rules := []Rule{
		{Left: []uint8{pAH}, Right: []uint8{pAH}, Break: false},
		{Left: []uint8{pAH_Mid}, Right: []uint8{pAH}, Break: false},
		{Left: []uint8{pAH_Mid}, Right: nil, Break: true},
		{Left: []uint8{pSOT}, Right: nil, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	combined := []CombinedState{
		{Left: pAH, Right: pMid, State: pAH_Mid},
	}

	table := BuildStateTable(rules, combined, stride)

	// Override AH_Mid × non-AH to NoMatch (rewind) instead of Break.
	for right := range stride {
		if uint8(right) != pAH {
			table[int(pAH_Mid)*stride+right] = NoMatch
		}
	}

	data := &RuleData{
		Properties: lookup,
		BreakTable: table,
		Stride:     stride,
		PropCount:  3,
		SOT:        pSOT,
		EOT:        pEOT,
	}

	tests := []struct {
		input string
		segs  []string
	}{
		{"a.a", []string{"a.a"}},
		{"a.x", []string{"a", ".", "x"}},
		{"a.a.a", []string{"a.a.a"}},
		{"a.", []string{"a", "."}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			seg := New(data, []byte(tt.input))
			got := segments(seg)
			if !stringsEqual(got, tt.segs) {
				t.Errorf("got %q; want %q", got, tt.segs)
			}
		})
	}
}

func TestEmptyInput(t *testing.T) {
	rules := []Rule{{Left: nil, Right: nil, Break: true}}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: funcTable(lookupAB),
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}
	seg := New(data, nil)
	if seg.Next() {
		t.Error("expected false for empty input")
	}
	if seg.Next() {
		t.Error("expected false on second call")
	}
}

func TestNextAfterExhausted(t *testing.T) {
	rules := []Rule{
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: funcTable(lookupAB),
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}
	seg := New(data, []byte("A"))
	if !seg.Next() {
		t.Fatal("expected first segment")
	}
	if got := seg.Text(); got != "A" {
		t.Errorf("got %q; want %q", got, "A")
	}
	if seg.Next() {
		t.Error("expected false after exhaustion")
	}
	if seg.Next() {
		t.Error("expected false on third call")
	}
}

func TestRulePriority(t *testing.T) {
	rules := []Rule{
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: []uint8{propA}, Right: []uint8{propB}, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: funcTable(lookupAB),
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}

	seg := New(data, []byte("AB"))
	got := segments(seg)
	want := []string{"AB"}
	if !stringsEqual(got, want) {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestMultiByteRunes(t *testing.T) {
	lookup := funcTable(func(b []byte) (uint8, int) {
		if b[0] < 0x80 {
			return propA, 1
		}
		if b[0] >= 0xC0 && b[0] < 0xE0 && len(b) >= 2 {
			return propB, 2
		}
		if b[0] >= 0xE0 && b[0] < 0xF0 && len(b) >= 3 {
			return propB, 3
		}
		if b[0] >= 0xF0 && len(b) >= 4 {
			return propB, 4
		}
		return propA, 1
	})

	rules := []Rule{
		{Left: []uint8{propA}, Right: []uint8{propA}, Break: false},
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: lookup,
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}

	input := "aé" // 'a' (1 byte) + 'é' (2 bytes U+00E9)
	seg := New(data, []byte(input))
	got := segments(seg)
	// a=propA, é=propB → A÷B at byte 1, then B→EOT at byte 3.
	want := []string{"a", "é"}
	if !stringsEqual(got, want) {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestOverrideTable(t *testing.T) {
	// Base: all bytes map to propA.
	// Override: 'B' maps to propB, everything else returns 0 (no override).
	base := funcTable(func(b []byte) (uint8, int) {
		return propA, 1
	})
	override := funcTable(func(b []byte) (uint8, int) {
		if b[0] == 'B' {
			return propB, 1
		}
		return 0, 1 // 0 = not overridden
	})

	rules := []Rule{
		{Left: []uint8{propA}, Right: []uint8{propA}, Break: false},
		{Left: []uint8{propSOT}, Right: nil, Break: false},
		{Left: nil, Right: nil, Break: true},
	}
	table := BuildStateTable(rules, nil, tStride)
	data := &RuleData{
		Properties: base,
		Override:   override,
		BreakTable: table,
		Stride:     tStride,
		PropCount:  2,
		SOT:        propSOT,
		EOT:        propEOT,
	}

	// Without override, "AABA" would be all propA → one segment.
	// With override, B becomes propB → "AA|B|A".
	seg := New(data, []byte("AABA"))
	got := segments(seg)
	want := []string{"AA", "B", "A"}
	if !stringsEqual(got, want) {
		t.Errorf("got %q; want %q", got, want)
	}

	// Without override: all propA → single segment.
	data.Override = nil
	seg = New(data, []byte("AABA"))
	got = segments(seg)
	want = []string{"AABA"}
	if !stringsEqual(got, want) {
		t.Errorf("without override: got %q; want %q", got, want)
	}
}

func stringsEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
