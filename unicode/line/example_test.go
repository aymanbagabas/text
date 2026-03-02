// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package line_test

import (
	"fmt"

	"golang.org/x/text/unicode/line"
)

func ExampleNewSegmenter() {
	input := []byte("Hello, world! How are you?")
	seg := line.NewSegmenter(input)
	for seg.Next() {
		start, end := seg.Position()
		fmt.Printf("[%d:%d] %q\n", start, end, seg.Text())
	}

	// Output:
	// [0:7] "Hello, "
	// [7:14] "world! "
	// [14:18] "How "
	// [18:22] "are "
	// [22:26] "you?"
}
