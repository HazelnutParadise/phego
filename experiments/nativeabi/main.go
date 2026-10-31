//go:build phego_native_probe && cgo

// The native ABI probe is an isolated experiment, not a PHP execution backend.
package main

/*
#include <stdint.h>
uint64_t phego_probe_xor(uint64_t left, uint64_t right);
*/
import "C"

import (
	"fmt"
	"os"
)

func main() {
	cases := []struct{ left, right uint64 }{
		{0, 0}, {0xf0, 0x3c}, {^uint64(0), 0},
		{^uint64(0), ^uint64(0)}, {uint64(1) << 63, 1},
	}
	for _, c := range cases {
		got := uint64(C.phego_probe_xor(C.uint64_t(c.left), C.uint64_t(c.right)))
		want := c.left ^ c.right
		if got != want {
			fmt.Fprintf(os.Stderr, "error: native XOR for %#x and %#x returned %#x; expected %#x\n", c.left, c.right, got, want)
			os.Exit(1)
		}
	}
	fmt.Printf("native ABI numeric probe: %d cases passed\n", len(cases))
}
