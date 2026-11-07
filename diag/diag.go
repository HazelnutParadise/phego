// Package diag renders Phego's own diagnostics in the rustc style required by
// AGENTS.md: a stable code, a one-line message, the location as
// file:line:column, the source line with markers under the exact span, and
// note:/help: lines.
//
// PHP-defined output (warnings, fatal errors, exception messages and their
// line numbers) never goes through this package; it stays byte-identical to
// the selected PHP profile.
package diag

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Code is a stable diagnostic identifier such as "PHG1001". A published code
// is never renumbered or reused; docs/diagnostics/codes.md lists them all.
type Code string

// Severity is the level shown before the code. The zero value is Error.
type Severity uint8

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// MarshalText makes JSON and terminal output use the same word.
func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText accepts the words produced by MarshalText.
func (s *Severity) UnmarshalText(b []byte) error {
	switch string(b) {
	case "error":
		*s = Error
	case "warning":
		*s = Warning
	default:
		return fmt.Errorf("diag: unknown severity %q", b)
	}
	return nil
}

// Span locates part of an input.
//
// Line is 1-based; 0 means the input has no line structure, for example a Go
// Config field path such as "Limits.Output", which File then names. Column is
// a 1-based byte column within the line and 0 when unknown; Offset is the
// 0-based byte offset from the start of File and is meaningful only when
// Line > 0. Length is the span length in bytes. Byte units match go/token and
// stay defined for invalid UTF-8; the renderer converts them to terminal
// columns from the source text. A span crossing the end of its line is marked
// to the end of that line.
type Span struct {
	File   string `json:"file"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
	Label  string `json:"label,omitempty"`
}

// Location formats File[:Line[:Column]].
func (s Span) Location() string {
	switch {
	case s.Line <= 0:
		return s.File
	case s.Column <= 0:
		return s.File + ":" + strconv.Itoa(s.Line)
	}
	return s.File + ":" + strconv.Itoa(s.Line) + ":" + strconv.Itoa(s.Column)
}

// Diagnostic is one explicit, located and actionable report.
// Its JSON form carries the same code, location, notes and help as the
// terminal form, for tools that read diagnostics.
type Diagnostic struct {
	Code      Code     `json:"code"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"` // one line saying what is wrong
	Primary   Span     `json:"primary"`
	Secondary []Span   `json:"secondary,omitempty"`
	Notes     []string `json:"notes,omitempty"` // why it is wrong
	Help      []string `json:"help,omitempty"`  // how to fix it; never applied automatically
}

// OneLine is the single-line form, for contexts such as Error() that must
// stay on one line.
func (d Diagnostic) OneLine() string {
	s := d.Severity.String() + "[" + clean(string(d.Code)) + "]: " + clean(d.Message)
	if loc := clean(d.Primary.Location()); loc != "" {
		s += " (" + loc + ")"
	}
	return s
}

// Options controls rendering.
//
// Source returns the text of a file named by a span; spans whose text is
// unavailable are shown by location only. The provider is responsible for
// withholding secrets from source it returns. Color adds ANSI styling; the
// caller decides whether the destination is a terminal and whether NO_COLOR
// applies, because a library cannot know where w ends up.
type Options struct {
	Color  bool
	Source func(file string) ([]byte, bool)
}

const (
	tabWidth      = 4
	maxLineWidth  = 160 // source columns shown per line
	maxSecondary  = 16  // secondary spans drawn before the rest are summarised
	contextBefore = 40  // columns kept before a span when a long line is cut
)

type palette struct{ on bool }

func (p palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}
func (p palette) severity(s Severity, text string) string {
	if s == Warning {
		return p.wrap("1;33", text)
	}
	return p.wrap("1;31", text)
}
func (p palette) gutter(s string) string { return p.wrap("1;34", s) }
func (p palette) bold(s string) string   { return p.wrap("1", s) }

type marked struct {
	span    Span
	primary bool
}

type fileLines struct {
	name  string
	lines map[int][]marked
}

// Render writes d in the rustc layout and always ends with a newline. It
// never writes terminal control characters taken from its input.
func Render(w io.Writer, d Diagnostic, opt Options) error {
	p := palette{opt.Color}
	text := map[string][]string{}
	lineText := func(file string, n int) (string, bool) {
		if n <= 0 || opt.Source == nil {
			return "", false
		}
		ls, ok := text[file]
		if !ok {
			if src, found := opt.Source(file); found {
				ls = strings.Split(string(src), "\n")
				for i := range ls {
					ls[i] = strings.TrimSuffix(ls[i], "\r")
				}
			}
			text[file] = ls
		}
		if n > len(ls) {
			return "", false
		}
		return ls[n-1], true
	}

	// A span with Offset but no Column gets its column from the source text,
	// when the offset falls inside the span's line.
	starts := map[string][]int{}
	resolve := func(sp Span) Span {
		if sp.Line <= 0 || sp.Column > 0 || opt.Source == nil {
			return sp
		}
		ls, ok := starts[sp.File]
		if !ok {
			if src, found := opt.Source(sp.File); found {
				ls = []int{0}
				for i, c := range src {
					if c == '\n' {
						ls = append(ls, i+1)
					}
				}
			}
			starts[sp.File] = ls
		}
		if sp.Line > len(ls) {
			return sp
		}
		start := ls[sp.Line-1]
		line, _ := lineText(sp.File, sp.Line)
		if sp.Offset >= start && sp.Offset <= start+len(line) {
			sp.Column = sp.Offset - start + 1
		}
		return sp
	}
	d.Primary = resolve(d.Primary)
	secondary := make([]Span, len(d.Secondary))
	for i, sp := range d.Secondary {
		secondary[i] = resolve(sp)
	}
	omitted := 0
	if len(secondary) > maxSecondary {
		omitted = len(secondary) - maxSecondary
		secondary = secondary[:maxSecondary]
	}
	all := []marked{{d.Primary, true}}
	for _, s := range secondary {
		all = append(all, marked{s, false})
	}
	var files []*fileLines
	var unshown []marked
	width := 1
	for _, m := range all {
		if _, ok := lineText(m.span.File, m.span.Line); !ok || m.span.Column <= 0 {
			if m.span.Location() != "" && !m.primary {
				unshown = append(unshown, m)
			}
			continue
		}
		var f *fileLines
		for _, x := range files {
			if x.name == m.span.File {
				f = x
			}
		}
		if f == nil {
			f = &fileLines{name: m.span.File, lines: map[int][]marked{}}
			files = append(files, f)
		}
		f.lines[m.span.Line] = append(f.lines[m.span.Line], m)
		width = max(width, len(strconv.Itoa(m.span.Line)))
	}
	pad := strings.Repeat(" ", width)

	var b bytes.Buffer
	line := func(s string) { b.WriteString(strings.TrimRight(s, " ") + "\n") }
	line(p.severity(d.Severity, d.Severity.String()+"["+clean(string(d.Code))+"]") + p.bold(": "+clean(d.Message)))
	if loc := d.Primary.Location(); loc != "" {
		line(pad + p.gutter("--> ") + clean(loc))
	}
	shownPrimary := false
	for fi, f := range files {
		if fi > 0 || !f.hasPrimary() {
			if fi > 0 {
				line(pad + p.gutter(" |"))
			}
			line(pad + p.gutter("::: ") + clean(f.first().Location()))
		}
		line(pad + p.gutter(" |"))
		nums := make([]int, 0, len(f.lines))
		for n := range f.lines {
			nums = append(nums, n)
		}
		slices.Sort(nums)
		for i, n := range nums {
			if i > 0 && n > nums[i-1]+1 {
				line(p.gutter("..."))
			}
			src, _ := lineText(f.name, n)
			ms := f.lines[n]
			slices.SortStableFunc(ms, func(a, b marked) int { return a.span.Column - b.span.Column })
			shown, cut := window(src, ms[0].span.Column)
			line(p.gutter(fmt.Sprintf("%*d |", width, n)) + " " + shown)
			for _, m := range ms {
				start, size := markerRange(src, m.span.Column, m.span.Length)
				start -= cut
				mark := "-"
				color := p.gutter
				if m.primary {
					mark, shownPrimary = "^", true
					color = func(s string) string { return p.severity(d.Severity, s) }
				}
				out := strings.Repeat(mark, size)
				if m.span.Label != "" {
					out += " " + clean(m.span.Label)
				}
				line(pad + p.gutter(" |") + " " + strings.Repeat(" ", max(start, 0)) + color(out))
			}
		}
	}
	for _, m := range unshown {
		line(pad + p.gutter("::: ") + clean(m.span.Location()))
	}
	notes := slices.Clone(d.Notes)
	if !shownPrimary && d.Primary.Label != "" {
		notes = append([]string{d.Primary.Label}, notes...)
	}
	for _, m := range unshown {
		if m.span.Label != "" {
			notes = append(notes, m.span.Location()+": "+m.span.Label)
		}
	}
	if omitted > 0 {
		notes = append(notes, fmt.Sprintf("%d more related locations omitted", omitted))
	}
	if len(files) > 0 && len(notes)+len(d.Help) > 0 {
		line(pad + p.gutter(" |"))
	}
	for _, n := range notes {
		line(pad + p.gutter(" = ") + p.bold("note") + ": " + clean(n))
	}
	for _, h := range d.Help {
		line(pad + p.gutter(" = ") + p.bold("help") + ": " + clean(h))
	}
	_, err := w.Write(b.Bytes())
	return err
}

func (f *fileLines) hasPrimary() bool {
	for _, ms := range f.lines {
		for _, m := range ms {
			if m.primary {
				return true
			}
		}
	}
	return false
}

func (f *fileLines) first() Span {
	best := Span{Line: -1}
	for _, ms := range f.lines {
		for _, m := range ms {
			if best.Line < 0 || m.span.Line < best.Line || (m.span.Line == best.Line && m.span.Column < best.Column) {
				best = m.span
			}
		}
	}
	return best
}

// glyph is one displayed unit of a source line.
type glyph struct {
	at, size int    // byte range in the line
	text     string // what is printed
	width    int    // terminal columns
}

func glyphs(src string) []glyph {
	var gs []glyph
	for i := 0; i < len(src); {
		r, n := utf8.DecodeRuneInString(src[i:])
		g := glyph{at: i, size: n, text: string(r), width: runeWidth(r)}
		switch {
		case r == '\t':
			g.text = strings.Repeat(" ", tabWidth)
		case r == utf8.RuneError && n == 1, isControl(r):
			g.text, g.width = "�", 1
		}
		gs = append(gs, g)
		i += n
	}
	return gs
}

// markerRange maps a 1-based byte column and byte length to a display offset
// and marker width, clamped to one column past the end of the line.
func markerRange(src string, column, length int) (start, size int) {
	lo, hi := column-1, column-1+max(length, 0)
	end := 0
	for _, g := range glyphs(src) {
		switch {
		case g.at+g.size <= lo:
			start += g.width
		case g.at < hi:
			size += g.width
		}
		end += g.width
	}
	if lo >= len(src) {
		start = end + (lo - len(src))
	}
	return start, max(size, 1)
}

// window cuts a long line around the first span, returning the printed text
// and how many display columns the markers must shift left.
func window(src string, column int) (string, int) {
	gs := glyphs(src)
	total := 0
	for _, g := range gs {
		total += g.width
	}
	var b strings.Builder
	if total <= maxLineWidth {
		for _, g := range gs {
			b.WriteString(g.text)
		}
		return b.String(), 0
	}
	start, _ := markerRange(src, column, 0)
	from := max(start-contextBefore, 0)
	col, dropped := 0, 0
	if from > 0 {
		b.WriteString("...")
	}
	for _, g := range gs {
		if col < from {
			col += g.width
			dropped = col
			continue
		}
		if col-dropped >= maxLineWidth {
			b.WriteString("...")
			break
		}
		b.WriteString(g.text)
		col += g.width
	}
	if from > 0 {
		dropped -= 3 // the leading "..." occupies three columns
	}
	return b.String(), dropped
}

// clean replaces control characters and invalid UTF-8 so input cannot drive
// the terminal.
func clean(s string) string {
	if !strings.ContainsFunc(s, isControl) && utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if isControl(r) || r == utf8.RuneError {
			r = '�'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isControl(r rune) bool {
	return r != '\t' && (r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0))
}

// runeWidth is the terminal column count of r: East Asian wide and fullwidth
// characters take two columns, as in rustc's unicode-width.
func runeWidth(r rune) int {
	switch {
	case r == '\t':
		return tabWidth
	case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF && r != 0x303F,
		r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF, r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6, r >= 0x1F300 && r <= 0x1F64F,
		r >= 0x1F900 && r <= 0x1F9FF, r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}
