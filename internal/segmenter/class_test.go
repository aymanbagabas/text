// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

import (
	"testing"
)

// flag is a local bitflag type used to test the generic Flattener.
type flag uint64

func TestFlattenerBasic(t *testing.T) {
	const (
		CR      flag = 1 << iota
		LF
		Control
		Extend
	)

	f := NewFlattener[flag]()
	f.Add(0)       // Other → index 0
	f.Add(CR)      // → index 1
	f.Add(LF)      // → index 2
	f.Add(Control) // → index 3
	f.Add(Extend)  // → index 4

	if f.Index(0) != 0 {
		t.Errorf("Other: got %d; want 0", f.Index(0))
	}
	if f.Index(CR) != 1 {
		t.Errorf("CR: got %d; want 1", f.Index(CR))
	}
	if f.Len() != 5 {
		t.Errorf("Len: got %d; want 5", f.Len())
	}

	// Expand: CR|LF|Control should return indices 1,2,3.
	got := f.Expand(func(c flag) bool { return c&(CR|LF|Control) != 0 })
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("Expand(CR|LF|Control): got %v; want [1 2 3]", got)
	}

	// All returns all indices.
	got = f.All()
	if len(got) != 5 {
		t.Errorf("All: got %d entries; want 5", len(got))
	}
}

func TestFlattenerExpand(t *testing.T) {
	const (
		OP       flag = 1 << iota
		CP
		QU
		eastAsian
		initialQu
		finalQu
	)

	f := NewFlattener[flag]()
	f.Add(0)               // XX → 0
	f.Add(OP)              // OP → 1
	f.Add(OP | eastAsian)  // opEA → 2
	f.Add(CP)              // CP → 3
	f.Add(CP | eastAsian)  // cpEA → 4
	f.Add(QU)              // QU → 5
	f.Add(QU | initialQu)  // quPI → 6
	f.Add(QU | finalQu)    // quPF → 7

	// OP mask should match both OP and opEA.
	got := f.Expand(func(c flag) bool { return c&OP != 0 })
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("Expand(OP): got %v; want [1 2]", got)
	}

	// OP with exclusion of eastAsian should match only OP.
	got = f.Expand(func(c flag) bool { return c&OP != 0 && c&eastAsian == 0 })
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("Expand(OP, !eastAsian): got %v; want [1]", got)
	}

	// OP|eastAsian as a mask matches anything with either bit.
	got = f.Expand(func(c flag) bool { return c&(OP|eastAsian) != 0 })
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 4 {
		t.Errorf("Expand(OP|eastAsian): got %v; want [1 2 4]", got)
	}
}

func TestFlattenerAddIdempotent(t *testing.T) {
	const A flag = 1

	f := NewFlattener[flag]()
	idx1 := f.Add(A)
	idx2 := f.Add(A)

	if idx1 != idx2 {
		t.Errorf("Add(A) returned different indices: %d vs %d", idx1, idx2)
	}
	if f.Len() != 1 {
		t.Errorf("Len: got %d; want 1", f.Len())
	}
}

func TestFlattenerKeys(t *testing.T) {
	const (
		A flag = 1 << iota
		B
		C
	)

	f := NewFlattener[flag]()
	f.Add(A)
	f.Add(B)
	f.Add(C)

	keys := f.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys: got %d; want 3", len(keys))
	}
	if keys[0] != A || keys[1] != B || keys[2] != C {
		t.Errorf("Keys: got %v; want [%d %d %d]", keys, A, B, C)
	}
}

func TestFlattenerIndexPanic(t *testing.T) {
	const A flag = 1

	f := NewFlattener[flag]()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Index on unregistered key should panic")
		}
	}()
	f.Index(A)
}

func TestFlattenerStringKey(t *testing.T) {
	f := NewFlattener[string]()
	f.Add("CR")
	f.Add("LF")
	f.Add("Control")

	if f.Index("CR") != 0 {
		t.Errorf("CR: got %d; want 0", f.Index("CR"))
	}
	if f.Index("LF") != 1 {
		t.Errorf("LF: got %d; want 1", f.Index("LF"))
	}
	if f.Len() != 3 {
		t.Errorf("Len: got %d; want 3", f.Len())
	}

	got := f.Expand(func(s string) bool { return s == "CR" || s == "LF" })
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("Expand: got %v; want [0 1]", got)
	}
}
