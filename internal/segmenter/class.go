// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package segmenter

// Flattener maps keys of type K to uint8 indices for the state table.
// Each unique key gets a distinct uint8 index. K is typically a bitflag
// type (e.g. uint64) but can be any comparable type.
type Flattener[K comparable] struct {
	keys      map[K]uint8
	ordered   []K
	nextIndex uint8
}

// NewFlattener creates a Flattener for keys of type K.
func NewFlattener[K comparable]() *Flattener[K] {
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
