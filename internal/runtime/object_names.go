package runtime

import "strings"

// PHP identifiers fold ASCII bytes only. Unicode case folding aliases names
// PHP keeps distinct, and rune mapping would alter invalid UTF-8 bytes.
func phpNameLower(name string) string {
	for i := range len(name) {
		if name[i] < 'A' || name[i] > 'Z' {
			continue
		}
		var out strings.Builder
		out.Grow(len(name))
		out.WriteString(name[:i])
		for j := i; j < len(name); j++ {
			b := name[j]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			out.WriteByte(b)
		}
		return out.String()
	}
	return name
}

func phpNameEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
