package ir

import (
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// numberLiteral decodes a T_LNUMBER or T_DNUMBER token. The lexer has
// already chosen the token kind for the profile's integer width.
func numberLiteral(text string, kind frontend.Kind) *Literal {
	digits := strings.ReplaceAll(text, "_", "")
	if kind == frontend.TLNumber {
		base, body := numberBase(digits)
		v, _ := strconv.ParseUint(body, base, 64)
		return &Literal{Kind: LitInt, Int: int64(v)}
	}
	if base, body := numberBase(digits); base != 10 {
		// zend_hex_strtod, zend_oct_strtod and zend_bin_strtod accumulate
		// digit by digit in a double, rounding at every step. The octal and
		// binary forms compute value*base + c - '0' from the character
		// code, which rounds twice; multiplying by a power of two is exact.
		v := 0.0
		for i := 0; i < len(body); i++ {
			if base == 16 {
				d, _ := strconv.ParseUint(body[i:i+1], 16, 8)
				v = float64(v*16) + float64(d)
				continue
			}
			t := float64(v*float64(base)) + float64(body[i])
			v = t - '0'
		}
		return &Literal{Kind: LitFloat, Float: v}
	}
	v, err := strconv.ParseFloat(digits, 64)
	if err != nil && !math.IsInf(v, 0) {
		v = 0
	}
	return &Literal{Kind: LitFloat, Float: v}
}

func numberBase(text string) (int, string) {
	if len(text) > 1 && text[0] == '0' {
		switch text[1] {
		case 'x', 'X':
			return 16, text[2:]
		case 'b', 'B':
			return 2, text[2:]
		case 'o', 'O':
			return 8, text[2:]
		}
		if !strings.ContainsAny(text, ".eE") {
			return 8, text[1:]
		}
	}
	return 10, text
}

// stringDecoder decodes string literal bytes and reports the compile
// warnings PHP raises while doing so.
type stringDecoder struct {
	warn func(line int, message string)
}

// constant decodes a T_CONSTANT_ENCAPSED_STRING token.
func (d stringDecoder) constant(text string, line int) string {
	if text != "" && (text[0] == 'b' || text[0] == 'B') {
		text = text[1:]
	}
	body := text[1 : len(text)-1]
	if text[0] == '\'' {
		if !strings.Contains(body, "\\") {
			return body
		}
		var out strings.Builder
		for i := 0; i < len(body); i++ {
			if body[i] == '\\' && i+1 < len(body) && (body[i+1] == '\\' || body[i+1] == '\'') {
				i++
			}
			out.WriteByte(body[i])
		}
		return out.String()
	}
	return d.escapes(body, '"', line)
}

// escapes applies zend_scan_escape_string for the given quote type: '"'
// for double quotes, '`' for shell strings and 0 for heredocs.
func (d stringDecoder) escapes(s string, quote byte, line int) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			out.WriteByte(c)
			if c == '\n' || (c == '\r' && (i+1 == len(s) || s[i+1] != '\n')) {
				line++
			}
			continue
		}
		i++
		if i >= len(s) {
			out.WriteByte('\\')
			break
		}
		c = s[i]
		switch c {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'f':
			out.WriteByte('\f')
		case 'v':
			out.WriteByte('\v')
		case 'e':
			out.WriteByte(0x1b)
		case '"', '`':
			if c != quote {
				out.WriteByte('\\')
			}
			out.WriteByte(c)
		case '\\', '$':
			out.WriteByte(c)
		case 'x', 'X':
			if i+1 < len(s) && isHex(s[i+1]) {
				j := i + 2
				if j < len(s) && isHex(s[j]) {
					j++
				}
				v, _ := strconv.ParseUint(s[i+1:j], 16, 8)
				out.WriteByte(byte(v))
				i = j - 1
			} else {
				out.WriteByte('\\')
				out.WriteByte(c)
			}
		case 'u':
			// The lexer has rejected invalid \u{...} sequences.
			if i+1 < len(s) && s[i+1] == '{' {
				end := strings.IndexByte(s[i:], '}') + i
				v, _ := strconv.ParseUint(s[i+2:end], 16, 32)
				out.WriteString(encodeUTF8(rune(v)))
				i = end
			} else {
				out.WriteString(`\u`)
			}
		default:
			if !isOct(c) {
				out.WriteByte('\\')
				out.WriteByte(c)
				break
			}
			j := i + 1
			for j < len(s) && j < i+3 && isOct(s[j]) {
				j++
			}
			if j-i == 3 && c > '3' && d.warn != nil {
				d.warn(line, "Octal escape sequence overflow \\"+s[i:j]+" is greater than \\377")
			}
			v, _ := strconv.ParseUint(s[i:j], 8, 16)
			out.WriteByte(byte(v))
			i = j - 1
		}
		if c == '\n' || (c == '\r' && (i+1 == len(s) || s[i+1] != '\n')) {
			line++
		}
	}
	return out.String()
}

// encodeUTF8 encodes a code point as PHP does, including surrogates that
// Go's encoder would replace.
func encodeUTF8(r rune) string {
	switch {
	case r < 0x80:
		return string([]byte{byte(r)})
	case r <= 0x7FF:
		return string([]byte{byte(r>>6) + 0xC0, byte(r&0x3F) + 0x80})
	case r <= 0xFFFF:
		return string([]byte{byte(r>>12) + 0xE0, byte((r>>6)&0x3F) + 0x80, byte(r&0x3F) + 0x80})
	}
	return string([]byte{byte(r>>18) + 0xF0, byte((r>>12)&0x3F) + 0x80, byte((r>>6)&0x3F) + 0x80, byte(r&0x3F) + 0x80})
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'f' }
func isOct(c byte) bool { return c >= '0' && c <= '7' }

// stripIndentation removes heredoc body indentation from one string token,
// as strip_multiline_string_indentation. atStart tells whether the token
// begins a line; atEnd whether the closing marker follows it.
func stripIndentation(s string, indent int, atStart, atEnd bool) string {
	if indent == 0 {
		return s
	}
	var out strings.Builder
	i := 0
	if !atStart {
		nl, n := nextNewline(s, 0)
		if nl < 0 {
			return s
		}
		out.WriteString(s[:nl+n])
		i = nl + n
	}
	for i <= len(s) {
		nl, n := nextNewline(s, i)
		lineEnd := nl
		if nl < 0 && atEnd {
			lineEnd = len(s)
		}
		for skip := 0; skip < indent && i != lineEnd && i < len(s) && (s[i] == ' ' || s[i] == '\t'); skip++ {
			i++
		}
		if i == len(s) {
			break
		}
		if nl < 0 {
			out.WriteString(s[i:])
			break
		}
		out.WriteString(s[i : nl+n])
		i = nl + n
	}
	return out.String()
}

func nextNewline(s string, from int) (int, int) {
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '\n':
			return i, 1
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				return i, 2
			}
			return i, 1
		}
	}
	return -1, 0
}

// heredocIndent returns the indentation width of a closing marker token.
func heredocIndent(end string) int {
	n := 0
	for n < len(end) && (end[n] == ' ' || end[n] == '\t') {
		n++
	}
	return n
}

// trimFinalNewline removes the newline before a heredoc closing marker.
func trimFinalNewline(s string) string {
	switch {
	case strings.HasSuffix(s, "\r\n"):
		return s[:len(s)-2]
	case strings.HasSuffix(s, "\n"), strings.HasSuffix(s, "\r"):
		return s[:len(s)-1]
	}
	return s
}
