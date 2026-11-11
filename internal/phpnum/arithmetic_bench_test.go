package phpnum

import "testing"

var benchPowResult float64

func BenchmarkPow(b *testing.B) {
	for _, tc := range []struct {
		name  string
		x, y  float64
		width int
	}{
		{"i386/integer", 7, -7, 32}, {"i386/fractional", 1.5, 1.5, 32}, {"i386/large", 1e100, -1.5, 32}, {"amd64/fractional", 1.5, 1.5, 64},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchPowResult = Pow(tc.x, tc.y, tc.width)
			}
		})
	}
}
