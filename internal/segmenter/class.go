// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

import "math/bits"

// Bitflag is the constraint for bitflag Class types used by segmenter
// code generators. Each base property occupies one bit.
type Bitflag interface {
	~uint16 | ~uint32 | ~uint64
}

// ClassRule describes a break rule using bitflag Class types.
// Zero Left/Right means "Any".
type ClassRule[K Bitflag] struct {
	Left, Right K
	Break       bool
}

// Flattener maps bitflag keys of type K to uint8 indices for the state table.
// Each unique key gets a distinct uint8 index.
type Flattener[K Bitflag] struct {
	keys      map[K]uint8
	ordered   []K
	nextIndex uint8
}

// NewFlattener creates a Flattener for bitflag Class types.
func NewFlattener[K Bitflag]() *Flattener[K] {
	return &Flattener[K]{
		keys: make(map[K]uint8),
	}
}

// Add registers a key and returns its uint8 index.
// Calling Add multiple times with the same key returns the same index.
func (f *Flattener[K]) Add(k K) uint8 {
	if idx, ok := f.keys[k]; ok {
		return idx
	}
	idx := f.nextIndex
	f.keys[k] = idx
	f.ordered = append(f.ordered, k)
	f.nextIndex++
	return idx
}

// Index returns the uint8 index for a previously-registered key.
// It panics if k was not registered via Add.
func (f *Flattener[K]) Index(k K) uint8 {
	idx, ok := f.keys[k]
	if !ok {
		panic("segmenter: key not registered with Flattener.Add")
	}
	return idx
}

// Expand returns the uint8 indices of all registered keys for which
// match returns true.
func (f *Flattener[K]) Expand(match func(K) bool) []uint8 {
	var result []uint8
	for i, k := range f.ordered {
		if match(k) {
			result = append(result, uint8(i))
		}
	}
	return result
}

// All returns the uint8 indices of every registered key.
func (f *Flattener[K]) All() []uint8 {
	result := make([]uint8, len(f.ordered))
	for i := range f.ordered {
		result[i] = uint8(i)
	}
	return result
}

// Len returns the number of registered keys.
func (f *Flattener[K]) Len() int {
	return len(f.ordered)
}

// Keys returns all registered keys in index order.
func (f *Flattener[K]) Keys() []K {
	result := make([]K, len(f.ordered))
	copy(result, f.ordered)
	return result
}

// AddAllBaseProperties registers Other (zero) and all single-bit properties
// from allBits, iterating bits LSB-first for deterministic index assignment.
// It returns the total number of registered keys.
func (f *Flattener[K]) AddAllBaseProperties(allBits K) int {
	f.Add(0) // any i.e. Other and XX properties
	for remaining := allBits; remaining != 0; {
		bit := K(1) << uint(bits.TrailingZeros64(uint64(remaining)))
		f.Add(bit)
		remaining &^= bit
	}
	return f.Len()
}

// FlattenRules converts ClassRules to Rule slices using the flattener's
// Expand method to map bitmask predicates to uint8 index slices.
func (f *Flattener[K]) FlattenRules(rules []ClassRule[K]) []Rule {
	var out []Rule
	for _, cr := range rules {
		r := Rule{Break: cr.Break}
		if cr.Left != 0 {
			r.Left = f.Expand(func(c K) bool { return c&cr.Left != 0 })
		}
		if cr.Right != 0 {
			r.Right = f.Expand(func(c K) bool { return c&cr.Right != 0 })
		}
		out = append(out, r)
	}
	return out
}
