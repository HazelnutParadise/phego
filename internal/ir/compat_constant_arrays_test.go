package ir

import (
	"errors"
	"testing"
)

func TestCompatibilityConstantArrays(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, tc := range []struct{ expression, message string }{
			{`["0"=>1] === [0=>1]`, `Cannot use "::class" on true`},
			{`["01"=>1] === [1=>1]`, `Cannot use "::class" on false`},
			{`[-2=>1,2] === [-2=>1,-1=>2]`, `Cannot use "::class" on true`},
			{`([0=>1]+[0=>2,1=>3]) === [1,3]`, `Cannot use "::class" on true`},
			{`[1,2] === [1=>2,0=>1]`, `Cannot use "::class" on false`},
			{`[1,2] == [1=>2,0=>1]`, `Cannot use "::class" on true`},
			{`[[1]] == [["1"]]`, `Cannot use "::class" on true`},
			{`[] < [0]`, `Cannot use "::class" on true`},
			{`[0] > []`, `Cannot use "::class" on true`},
			{`[0=>1] > [1=>1]`, `Cannot use "::class" on false`},
			{`[0=>1] < [1=>1]`, `Cannot use "::class" on false`},
			{`[...[], ...[1,2]] === [1,2]`, `Cannot use "::class" on true`},
			{`[...["a"=>1],"a"=>2] === ["a"=>2]`, `Cannot use "::class" on true`},
			{`[1.0=>1] === [1=>1]`, `Cannot use "::class" on true`},
			{`[1.5=>1] === [1=>1]`, ``},
			{`[null=>1] === [""=>1]`, ``},
			{`[[]=>1]+[]`, `Illegal offset type`},
			{`[[]=>$x]+[]`, ``},
			{`[...1]`, `Only arrays and Traversables can be unpacked, int given`},
			{`![]`, `Cannot use "::class" on true`},
			{`![[0]]`, `Cannot use "::class" on false`},
			{`[1] && true`, `Cannot use "::class" on true`},
			{`[] || false`, `Cannot use "::class" on false`},
		} {
			_, err := lowerSource("<?php ("+tc.expression+")::class;", width)
			var ce *CompileError
			if tc.message == "" {
				if err != nil {
					t.Errorf("%d %s: %v", width, tc.expression, err)
				}
				continue
			}
			if !errors.As(err, &ce) || ce.Line != 1 || ce.Message != tc.message {
				t.Errorf("%d %s: got %v; want %s", width, tc.expression, err, tc.message)
			}
		}
	}
}

func TestCompatibilityConstantArrayKeys(t *testing.T) {
	for _, tc := range []struct {
		width               int
		expression, message string
	}{
		{32, `["a"=>1,-2=>2,3] === ["a"=>1,-2=>2,-1=>3]`, `Cannot use "::class" on true`},
		{32, `["-0"=>1] === [0=>1]`, `Cannot use "::class" on false`},
		{32, `[[]=>1]`, `Illegal offset type`},
		{32, `[[]=>1,$x]`, ``},
		{32, `[1.5=>1,[]=>1]`, ``},
		{32, `[[]=>1,1.5=>1]`, `Illegal offset type`},
		{32, `["+1"=>1] === [1=>1]`, `Cannot use "::class" on false`},
		{32, `["2147483648"=>1] === [2147483648=>1]`, ``},
		{32, `["9223372036854775808"=>1] === [9223372036854775808=>1]`, ``},
		{32, `[2147483647=>1,2]`, ``},
		{32, `[9223372036854775807=>1,2]`, ``},
		{32, `["a"=>1,"b"=>2,"a"=>3] === ["a"=>3,"b"=>2]`, `Cannot use "::class" on true`},
		{32, `[...( ["a"=>1]+["a"=>2,"b"=>3] )] === ["a"=>1,"b"=>3]`, `Cannot use "::class" on true`},
		{32, `[...( [-2=>1]+[-1=>2] )] === [1,2]`, `Cannot use "::class" on true`},
		{64, `["a"=>1,-2=>2,3] === ["a"=>1,-2=>2,-1=>3]`, `Cannot use "::class" on true`},
		{64, `["-0"=>1] === [0=>1]`, `Cannot use "::class" on false`},
		{64, `[[]=>1]`, `Illegal offset type`},
		{64, `[[]=>1,$x]`, ``},
		{64, `[1.5=>1,[]=>1]`, ``},
		{64, `[[]=>1,1.5=>1]`, `Illegal offset type`},
		{64, `["+1"=>1] === [1=>1]`, `Cannot use "::class" on false`},
		{64, `["2147483648"=>1] === [2147483648=>1]`, `Cannot use "::class" on true`},
		{64, `["9223372036854775808"=>1] === [9223372036854775808=>1]`, ``},
		{64, `[2147483647=>1,2]`, `Cannot use "::class" on array`},
		{64, `[9223372036854775807=>1,2]`, ``},
		{64, `["a"=>1,"b"=>2,"a"=>3] === ["a"=>3,"b"=>2]`, `Cannot use "::class" on true`},
		{64, `[...( ["a"=>1]+["a"=>2,"b"=>3] )] === ["a"=>1,"b"=>3]`, `Cannot use "::class" on true`},
		{64, `[...( [-2=>1]+[-1=>2] )] === [1,2]`, `Cannot use "::class" on true`},
	} {
		_, err := lowerSource("<?php ("+tc.expression+")::class;", tc.width)
		var ce *CompileError
		if tc.message == "" {
			if err != nil {
				t.Errorf("%d %s: %v", tc.width, tc.expression, err)
			}
			continue
		}
		if !errors.As(err, &ce) || ce.Line != 1 || ce.Message != tc.message {
			t.Errorf("%d %s: %v; want %s", tc.width, tc.expression, err, tc.message)
		}
	}
}
