// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package word_test

import (
	"fmt"

	"golang.org/x/text/unicode/word"
)

func ExampleSegmenter() {
	input := []byte("Hello, World!")

	seg := word.NewSegmenter(input)
	for seg.Next() {
		fmt.Printf("%q\n", seg.Text())
	}
	// Output:
	// "Hello"
	// ","
	// " "
	// "World"
	// "!"
}
