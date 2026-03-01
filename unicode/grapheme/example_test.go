// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grapheme_test

import (
	"fmt"

	"golang.org/x/text/unicode/grapheme"
)

func ExampleSegmenter() {
	input := []byte("Hello, 世界! 👨‍👩‍👧‍👦")

	seg := grapheme.NewSegmenter(input)
	for seg.Next() {
		start, end := seg.Position()
		fmt.Printf("[%d:%d] %q\n", start, end, seg.Text())
	}
	// Output:
	// [0:1] "H"
	// [1:2] "e"
	// [2:3] "l"
	// [3:4] "l"
	// [4:5] "o"
	// [5:6] ","
	// [6:7] " "
	// [7:10] "世"
	// [10:13] "界"
	// [13:14] "!"
	// [14:15] " "
	// [15:40] "👨\u200d👩\u200d👧\u200d👦"
}
