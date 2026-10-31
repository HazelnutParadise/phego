package frontend

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// LexOptions selects the profile-dependent lexing rules.
type LexOptions struct {
	ShortOpenTag bool // INI short_open_tag
	IntWidth     int  // guest integer width: 32 or 64
	MaxBytes     int  // input limit checked before any token is allocated
	MaxTokens    int  // token limit; 0 means len(src)+1, which cannot be exceeded
}

// ErrInputTooLarge reports a source rejected by LexOptions.MaxBytes.
var ErrInputTooLarge = errors.New("source exceeds the configured size limit")

// ErrTooManyTokens reports a source that produced more than MaxTokens tokens.
var ErrTooManyTokens = errors.New("source exceeds the configured token limit")

type state uint8

const (
	stInitial state = iota
	stScripting
	stDoubleQuotes
	stBackquote
	stHeredoc
	stNowdoc
	stLookingForProperty
	stVarOffset
	stLookingForVarname
)

type lexer struct {
	src    string
	pos    int
	line   int
	opts   LexOptions
	states []state
	labels []heredocLabel
	out    []Token
	diags  []Diagnostic
	// braceEnds maps each indexed opening brace to its closing brace; only
	// braces actually seen are stored, so memory follows the brace count.
	braceEnds map[int]int
	// braceBudget counts braces the prescan may still record; every brace
	// it records becomes a token, so exhausting it means MaxTokens will be
	// exceeded and lexing stops early.
	braceBudget int
	overBudget  bool
}

type heredocLabel struct {
	name   string
	marker int // start of the closing marker line, or -1 when unterminated
	indent string
	// unindented records an interpolation that starts a body line without
	// the closing marker's indentation; PHP reports it at the marker.
	unindented bool
}

// Lex tokenizes PHP 8.5.12 source the way token_get_all() does without
// TOKEN_PARSE. Every byte of src belongs to exactly one token. Lexing never
// panics; problems are reported as diagnostics with their line.
func Lex(src []byte, opts LexOptions) ([]Token, []Diagnostic, error) {
	if opts.IntWidth != 32 && opts.IntWidth != 64 {
		return nil, nil, fmt.Errorf("invalid PHP integer width %d: expected 32 or 64", opts.IntWidth)
	}
	if opts.MaxBytes <= 0 || len(src) > opts.MaxBytes {
		return nil, nil, ErrInputTooLarge
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = len(src) + 1
	}
	l := &lexer{src: string(src), line: 1, opts: opts, states: []state{stInitial}, braceBudget: opts.MaxTokens}
	halted := -1
	for l.pos < len(l.src) {
		start, depth := l.pos, len(l.states)
		l.next()
		if len(l.out) > opts.MaxTokens || l.overBudget {
			return nil, nil, ErrTooManyTokens
		}
		if l.pos == start {
			// Only a state pop may consume nothing; anything else would loop.
			if len(l.states) >= depth {
				l.pos = start + 1
				l.emit(TBadCharacter, start)
			}
			continue
		}
		if halted < 0 && len(l.out) > 0 && l.out[len(l.out)-1].Kind == THaltCompiler {
			halted = 0
		} else if halted >= 0 && !isTrivia(l.out[len(l.out)-1].Kind) {
			halted++
			if halted == 3 {
				if l.pos < len(l.src) {
					l.emitRaw(TInlineHTML, l.pos, len(l.src))
					l.pos = len(l.src)
				}
				break
			}
		}
	}
	if len(l.out) > opts.MaxTokens {
		return nil, nil, ErrTooManyTokens
	}
	unterminated := false
	for _, state := range l.states {
		switch state {
		case stDoubleQuotes, stBackquote, stHeredoc, stNowdoc:
			unterminated = true
		}
	}
	if unterminated {
		alreadyReported := false
		for _, d := range l.diags {
			if strings.HasPrefix(d.Message, "Unterminated ") {
				alreadyReported = true
				break
			}
		}
		if !alreadyReported {
			l.diag("Unterminated string at end of file")
		}
	}
	return l.out, l.diags, nil
}

func isTrivia(k Kind) bool { return k == TWhitespace || k == TComment || k == TDocComment }

func (l *lexer) top() state     { return l.states[len(l.states)-1] }
func (l *lexer) push(s state)   { l.states = append(l.states, s) }
func (l *lexer) setTop(s state) { l.states[len(l.states)-1] = s }
func (l *lexer) pop() {
	if len(l.states) > 1 {
		l.states = l.states[:len(l.states)-1]
	}
}

func (l *lexer) emit(k Kind, start int) { l.emitRaw(k, start, l.pos) }

func (l *lexer) emitRaw(k Kind, start, end int) {
	text := l.src[start:end]
	switch {
	case k == TConstantEncapsedString && (strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "b\"") || strings.HasPrefix(text, "B\"")):
		l.checkCodepointEscapes(text)
	case k == TEncapsedAndWhitespace:
		switch l.top() {
		case stDoubleQuotes, stBackquote, stHeredoc:
			l.checkCodepointEscapes(text)
		}
	}
	l.out = append(l.out, Token{Kind: k, Text: text, Line: l.line})
	l.line += countLines(text)
}

// checkCodepointEscapes reports PHP 8.5.12's ParseError for an invalid
// \u{...} escape in an interpolating string segment, on the escape's line.
func (l *lexer) checkCodepointEscapes(text string) {
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 >= len(text) {
			continue
		}
		if text[i+1] != 'u' || i+2 >= len(text) || text[i+2] != '{' {
			i++ // skip the escaped byte, including another backslash
			continue
		}
		j := i + 3
		for j < len(text) && isHexDigit(text[j]) {
			j++
		}
		line := l.line + countLines(text[:i])
		switch {
		case j == i+3 || j >= len(text) || text[j] != '}':
			l.fatal(-1, line, "Invalid UTF-8 codepoint escape sequence")
			return
		case j-(i+3) > 8 || parseHex(text[i+3:j]) > 0x10FFFF:
			l.fatal(-1, line, "Invalid UTF-8 codepoint escape sequence: Codepoint too large")
			return
		}
		i = j
	}
}

func isHexDigit(c byte) bool { return isDigit(c) || (c|0x20 >= 'a' && c|0x20 <= 'f') }

func parseHex(s string) uint64 {
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		d := uint64(c - '0')
		if !isDigit(c) {
			d = uint64(c|0x20-'a') + 10
		}
		v = v*16 + d
	}
	return v
}

func (l *lexer) diag(format string, args ...any) {
	l.diags = append(l.diags, Diagnostic{Line: l.line, Message: fmt.Sprintf(format, args...), Token: len(l.out)})
}

// fatal records a ParseError for the token about to be emitted (or, with
// token >= 0, an already emitted one).
func (l *lexer) fatal(token, line int, format string, args ...any) {
	if token < 0 {
		token = len(l.out)
	}
	l.diags = append(l.diags, Diagnostic{Line: line, Message: fmt.Sprintf(format, args...), Fatal: true, Token: token})
}

// countLines counts \n, \r\n, and lone \r as PHP's scanner does.
func countLines(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			n++
		case '\r':
			if i+1 >= len(s) || s[i+1] != '\n' {
				n++
			}
		}
	}
	return n
}

func (l *lexer) peek(off int) byte {
	if l.pos+off < len(l.src) {
		return l.src[l.pos+off]
	}
	return 0
}

func (l *lexer) hasPrefixFold(s string) bool {
	return len(l.src)-l.pos >= len(s) && strings.EqualFold(l.src[l.pos:l.pos+len(s)], s)
}

func isLabelStart(c byte) bool {
	return c == '_' || (c|0x20 >= 'a' && c|0x20 <= 'z') || c >= 0x80
}

func isLabelChar(c byte) bool { return isLabelStart(c) || (c >= '0' && c <= '9') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (l *lexer) labelEnd(i int) int {
	for i < len(l.src) && isLabelChar(l.src[i]) {
		i++
	}
	return i
}

func (l *lexer) next() {
	switch l.top() {
	case stInitial:
		l.lexInitial()
	case stScripting:
		l.lexScripting()
	case stDoubleQuotes:
		l.lexInterpolated('"')
	case stBackquote:
		l.lexInterpolated('`')
	case stHeredoc:
		l.lexInterpolated(0)
	case stNowdoc:
		l.lexNowdoc()
	case stLookingForProperty:
		l.lexProperty()
	case stVarOffset:
		l.lexVarOffset()
	case stLookingForVarname:
		l.lexVarname()
	}
}

// openTagAt reports the open tag at i and its length, or 0.
func (l *lexer) openTagAt(i int) (Kind, int) {
	s := l.src[i:]
	if !strings.HasPrefix(s, "<?") {
		return 0, 0
	}
	if strings.HasPrefix(s, "<?=") {
		return TOpenTagWithEcho, 3
	}
	if len(s) >= 5 && strings.EqualFold(s[2:5], "php") {
		if len(s) == 5 {
			return TOpenTag, 5
		}
		switch s[5] {
		case ' ', '\t', '\n':
			return TOpenTag, 6
		case '\r':
			if len(s) > 6 && s[6] == '\n' {
				return TOpenTag, 7
			}
			return TOpenTag, 6
		}
	}
	if l.opts.ShortOpenTag {
		return TOpenTag, 2
	}
	return 0, 0
}

func (l *lexer) lexInitial() {
	start := l.pos
	for i := l.pos; i < len(l.src); i++ {
		if l.src[i] != '<' {
			continue
		}
		if kind, n := l.openTagAt(i); n > 0 {
			if i > start {
				l.emitRaw(TInlineHTML, start, i)
			}
			l.pos = i + n
			l.emit(kind, i)
			l.setTop(stScripting)
			return
		}
	}
	l.pos = len(l.src)
	l.emit(TInlineHTML, start)
}

var threeCharOps = map[string]Kind{
	"<=>": TSpaceship, "===": TIsIdentical, "!==": TIsNotIdentical, "<<=": TSlEqual,
	">>=": TSrEqual, "**=": TPowEqual, "??=": TCoalesceEqual, "...": TEllipsis,
	"?->": TNullsafeObjectOperator,
}

var twoCharOps = map[string]Kind{
	"++": TInc, "--": TDec, "->": TObjectOperator, "=>": TDoubleArrow, "::": TDoubleColon,
	"==": TIsEqual, "!=": TIsNotEqual, "<>": TIsNotEqual, "<=": TIsSmallerOrEqual,
	">=": TIsGreaterOrEqual, "+=": TPlusEqual, "-=": TMinusEqual, "*=": TMulEqual,
	"/=": TDivEqual, ".=": TConcatEqual, "%=": TModEqual, "&=": TAndEqual, "|=": TOrEqual,
	"^=": TXorEqual, "&&": TBooleanAnd, "||": TBooleanOr, "??": TCoalesce, "<<": TSl,
	">>": TSr, "**": TPow, "|>": TPipe,
}

const singleChars = ";:,.[]()|^&+-/*=%!~$<>?@{}\"`"

var casts = map[string]Kind{
	"int": TIntCast, "integer": TIntCast, "bool": TBoolCast, "boolean": TBoolCast,
	"float": TDoubleCast, "double": TDoubleCast, "real": TDoubleCast,
	"string": TStringCast, "binary": TStringCast, "array": TArrayCast, "object": TObjectCast,
	"unset": TUnsetCast, "void": TVoidCast,
}

func (l *lexer) lexScripting() {
	start := l.pos
	c := l.src[l.pos]
	switch {
	case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		for l.pos < len(l.src) && strings.IndexByte(" \t\n\r", l.src[l.pos]) >= 0 {
			l.pos++
		}
		l.emit(TWhitespace, start)
		return
	case c == '?' && l.peek(1) == '>':
		l.pos += 2
		if l.peek(0) == '\n' {
			l.pos++
		} else if l.peek(0) == '\r' {
			l.pos++
			if l.peek(0) == '\n' {
				l.pos++
			}
		}
		l.emit(TCloseTag, start)
		l.setTop(stInitial)
		return
	case c == '#' && l.peek(1) == '[':
		l.pos += 2
		l.emit(TAttribute, start)
		return
	case c == '#' || (c == '/' && l.peek(1) == '/'):
		l.lineComment(start)
		return
	case c == '/' && l.peek(1) == '*':
		l.blockComment(start)
		return
	case c == '$' && isLabelStart(l.peek(1)):
		l.pos = l.labelEnd(l.pos + 1)
		l.emit(TVariable, start)
		return
	case isDigit(c) || (c == '.' && isDigit(l.peek(1))):
		l.number(start)
		return
	case c == '\'':
		l.singleQuoted(start)
		return
	case c == '"':
		l.doubleQuoted(start)
		return
	case c == '`':
		l.pos++
		l.emit('`', start)
		l.push(stBackquote)
		return
	case (c == 'b' || c == 'B') && (l.peek(1) == '\'' || l.peek(1) == '"' || (l.peek(1) == '<' && strings.HasPrefix(l.src[l.pos+1:], "<<<"))):
		l.pos++
		switch l.peek(0) {
		case '\'':
			l.singleQuoted(start)
		case '"':
			l.doubleQuoted(start)
		default:
			if !l.heredocStart(start) {
				l.pos = start
				l.name(start)
			}
		}
		return
	case c == '<' && strings.HasPrefix(l.src[l.pos:], "<<<") && l.heredocStart(start):
		return
	case c == '(':
		if l.cast(start) {
			return
		}
	case isLabelStart(c) || c == '\\':
		l.name(start)
		return
	}
	if len(l.src)-l.pos >= 3 {
		if k, ok := threeCharOps[l.src[l.pos:l.pos+3]]; ok {
			l.pos += 3
			l.emit(k, start)
			if k == TNullsafeObjectOperator {
				l.push(stLookingForProperty)
			}
			return
		}
	}
	if len(l.src)-l.pos >= 2 {
		if k, ok := twoCharOps[l.src[l.pos:l.pos+2]]; ok {
			l.pos += 2
			l.emit(k, start)
			if k == TObjectOperator {
				l.push(stLookingForProperty)
			}
			return
		}
	}
	l.pos++
	switch {
	case c == '{':
		l.emit('{', start)
		l.push(stScripting)
	case c == '}':
		l.emit('}', start)
		l.pop()
	case c == '&':
		if j := l.skipTrivia(l.pos); j < len(l.src) && (l.src[j] == '$' || strings.HasPrefix(l.src[j:], "...")) {
			l.emit(TAmpersandFollowedByVarOrVararg, start)
		} else {
			l.emit(TAmpersandNotFollowedByVarOrVararg, start)
		}
	case strings.IndexByte(singleChars, c) >= 0:
		l.emit(Kind(c), start)
	default:
		l.emit(TBadCharacter, start)
	}
}

// lineComment ends before a newline or a close tag.
func (l *lexer) lineComment(start int) {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\n' || c == '\r' || (c == '?' && l.peek(1) == '>') {
			break
		}
		l.pos++
	}
	l.emit(TComment, start)
}

func (l *lexer) blockComment(start int) {
	kind := TComment
	if strings.HasPrefix(l.src[l.pos:], "/**") && l.pos+3 < len(l.src) && strings.IndexByte(" \t\n\r", l.src[l.pos+3]) >= 0 {
		kind = TDocComment
	}
	end := strings.Index(l.src[l.pos+2:], "*/")
	if end < 0 {
		l.fatal(-1, l.line, "Unterminated comment starting line %d", l.line)
		l.pos = len(l.src)
	} else {
		l.pos += 2 + end + 2
	}
	l.emit(kind, start)
}

func (l *lexer) cast(start int) bool {
	i := l.pos + 1
	for i < len(l.src) && (l.src[i] == ' ' || l.src[i] == '\t') {
		i++
	}
	j := i
	for j < len(l.src) && (l.src[j]|0x20 >= 'a' && l.src[j]|0x20 <= 'z') {
		j++
	}
	word := strings.ToLower(l.src[i:j])
	k, ok := casts[word]
	if !ok {
		return false
	}
	for j < len(l.src) && (l.src[j] == ' ' || l.src[j] == '\t') {
		j++
	}
	if j >= len(l.src) || l.src[j] != ')' {
		return false
	}
	l.pos = j + 1
	if word == "real" {
		l.fatal(-1, l.line, "The (real) cast has been removed, use (float) instead")
	}
	l.emit(k, start)
	return true
}

// skipTrivia returns the index after whitespace and comments starting at i.
func (l *lexer) skipTrivia(i int) int {
	for i < len(l.src) {
		switch {
		case strings.IndexByte(" \t\n\r", l.src[i]) >= 0:
			i++
		case l.src[i] == '#' && (i+1 >= len(l.src) || l.src[i+1] != '['), strings.HasPrefix(l.src[i:], "//"):
			for i < len(l.src) && l.src[i] != '\n' && l.src[i] != '\r' {
				i++
			}
		case strings.HasPrefix(l.src[i:], "/*"):
			end := strings.Index(l.src[i+2:], "*/")
			if end < 0 {
				return i
			}
			i += 2 + end + 2
		default:
			return i
		}
	}
	return i
}

func (l *lexer) name(start int) {
	i := l.pos
	leading := l.src[i] == '\\'
	if leading {
		i++
		if i >= len(l.src) || !isLabelStart(l.src[i]) {
			l.pos = i
			l.emit(TNsSeparator, start)
			return
		}
	}
	i = l.labelEnd(i)
	first := l.src[l.pos:i]
	segments := 1
	for i+1 < len(l.src) && l.src[i] == '\\' && isLabelStart(l.src[i+1]) {
		i = l.labelEnd(i + 1)
		segments++
	}
	if leading {
		l.pos = i
		l.emit(TNameFullyQualified, start)
		return
	}
	if segments > 1 {
		l.pos = i
		if strings.EqualFold(first, "namespace") {
			l.emit(TNameRelative, start)
		} else {
			l.emit(TNameQualified, start)
		}
		return
	}
	l.pos = i
	lower := strings.ToLower(first)
	switch lower {
	case "yield":
		j := l.skipTrivia(i)
		if j > i && len(l.src)-j >= 4 && strings.EqualFold(l.src[j:j+4], "from") && (j+4 >= len(l.src) || !isLabelChar(l.src[j+4])) {
			l.pos = j + 4
			l.emit(TYieldFrom, start)
			return
		}
	case "enum":
		j := l.skipTrivia(i)
		if j > i && j < len(l.src) && isLabelStart(l.src[j]) {
			word := strings.ToLower(l.src[j:l.labelEnd(j)])
			if word != "extends" && word != "implements" {
				l.emit(TEnum, start)
				return
			}
		}
		l.emit(TString, start)
		return
	case "public", "protected", "private":
		if len(l.src)-i >= 5 && strings.EqualFold(l.src[i:i+5], "(set)") {
			l.pos = i + 5
			l.emit(map[string]Kind{"public": TPublicSet, "protected": TProtectedSet, "private": TPrivateSet}[lower], start)
			return
		}
	}
	if k, ok := keywords[lower]; ok {
		l.emit(k, start)
		return
	}
	l.emit(TString, start)
}

func (l *lexer) number(start int) {
	src := l.src
	i := l.pos
	digitsWith := func(i int, ok func(byte) bool) int {
		if i >= len(src) || !ok(src[i]) {
			return i
		}
		for i < len(src) {
			if ok(src[i]) {
				i++
			} else if src[i] == '_' && i+1 < len(src) && ok(src[i+1]) {
				i += 2
			} else {
				break
			}
		}
		return i
	}
	isHex := func(c byte) bool { return isDigit(c) || (c|0x20 >= 'a' && c|0x20 <= 'f') }
	isBin := func(c byte) bool { return c == '0' || c == '1' }
	isOct := func(c byte) bool { return c >= '0' && c <= '7' }
	if src[i] == '0' && i+2 < len(src) {
		var ok func(byte) bool
		base := 0
		switch src[i+1] | 0x20 {
		case 'x':
			ok, base = isHex, 16
		case 'b':
			ok, base = isBin, 2
		case 'o':
			ok, base = isOct, 8
		}
		if ok != nil && ok(src[i+2]) {
			end := digitsWith(i+2, ok)
			l.pos = end
			l.emit(l.intKind(src[i+2:end], base), start)
			return
		}
	}
	end := digitsWith(i, isDigit)
	float := false
	if end < len(src) && src[end] == '.' {
		// Longest match, as PHP's scanner does: "1...2" is "1." "." ".2".
		float = true
		end = digitsWith(end+1, isDigit)
	}
	if end < len(src) && src[end]|0x20 == 'e' {
		j := end + 1
		if j < len(src) && (src[j] == '+' || src[j] == '-') {
			j++
		}
		if j < len(src) && isDigit(src[j]) {
			float = true
			end = digitsWith(j, isDigit)
		}
	}
	l.pos = end
	if float {
		l.emit(TDNumber, start)
		return
	}
	text := src[i:end]
	if len(text) > 1 && text[0] == '0' {
		if strings.ContainsAny(text, "89") {
			l.fatal(-1, l.line, "Invalid numeric literal")
		}
		l.emit(l.intKind(text[1:], 8), start)
		return
	}
	l.emit(l.intKind(text, 10), start)
}

// intKind returns T_LNUMBER when the digits fit the profile's integer width.
func (l *lexer) intKind(digits string, base int) Kind {
	limit := uint64(math.MaxInt64)
	if l.opts.IntWidth == 32 {
		limit = math.MaxInt32
	}
	var v uint64
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		if c == '_' {
			continue
		}
		d := uint64(0)
		switch {
		case isDigit(c):
			d = uint64(c - '0')
		default:
			d = uint64(c|0x20-'a') + 10
		}
		if d >= uint64(base) {
			// Octal literal with 8 or 9: PHP rejects it as an invalid
			// numeric literal; the token remains a number.
			return TLNumber
		}
		if v > (limit-d)/uint64(base) {
			return TDNumber
		}
		v = v*uint64(base) + d
	}
	return TLNumber
}

func (l *lexer) singleQuoted(start int) {
	i := l.pos + 1
	for i < len(l.src) {
		switch l.src[i] {
		case '\\':
			i += 2
			continue
		case '\'':
			l.pos = i + 1
			l.emit(TConstantEncapsedString, start)
			return
		}
		i++
	}
	l.pos = len(l.src)
	l.diag("Unterminated single-quoted string")
	l.emit(TEncapsedAndWhitespace, start)
}

// interpolationAt reports whether an interpolation begins at i.
func (l *lexer) interpolationAt(i int) bool {
	if i+1 >= len(l.src) {
		return false
	}
	c, n := l.src[i], l.src[i+1]
	return (c == '$' && (isLabelStart(n) || n == '{')) || (c == '{' && n == '$')
}

func (l *lexer) doubleQuoted(start int) {
	i := l.pos + 1
	for i < len(l.src) {
		switch {
		case l.src[i] == '\\':
			i += 2
			continue
		case l.src[i] == '"':
			l.pos = i + 1
			l.emit(TConstantEncapsedString, start)
			return
		case l.interpolationAt(i):
			l.pos++
			l.emit('"', start)
			l.push(stDoubleQuotes)
			return
		}
		i++
	}
	// Unterminated: PHP emits the quote and scans the rest as a string body.
	l.pos++
	l.emit('"', start)
	l.push(stDoubleQuotes)
}

// heredocStart recognizes <<< [ \t]* (LABEL | "LABEL" | 'LABEL') NEWLINE.
func (l *lexer) heredocStart(start int) bool {
	i := l.pos + 3
	for i < len(l.src) && (l.src[i] == ' ' || l.src[i] == '\t') {
		i++
	}
	quote := byte(0)
	if i < len(l.src) && (l.src[i] == '"' || l.src[i] == '\'') {
		quote = l.src[i]
		i++
	}
	if i >= len(l.src) || !isLabelStart(l.src[i]) {
		return false
	}
	end := l.labelEnd(i)
	name := l.src[i:end]
	if quote != 0 {
		if end >= len(l.src) || l.src[end] != quote {
			return false
		}
		end++
	}
	switch {
	case end < len(l.src) && l.src[end] == '\n':
		end++
	case end < len(l.src) && l.src[end] == '\r':
		end++
		if end < len(l.src) && l.src[end] == '\n' {
			end++
		}
	default:
		return false
	}
	l.pos = end
	l.emit(TStartHeredoc, start)
	l.labels = append(l.labels, heredocLabel{name: name, marker: -1})
	// Find the closing marker once; body segments reuse it instead of
	// rescanning the remaining source after every interpolation.
	label := &l.labels[len(l.labels)-1]
	_, label.marker = l.heredocBodyLimit(l.pos, quote != '\'')
	if label.marker >= 0 {
		m := label.marker
		for m < len(l.src) && (l.src[m] == ' ' || l.src[m] == '\t') {
			m++
		}
		label.indent = l.src[label.marker:m]
	}
	if quote == '\'' {
		l.push(stNowdoc)
	} else {
		l.push(stHeredoc)
	}
	// An immediately closing marker yields no body token.
	if n := l.closingMarkerAt(l.pos); n > 0 {
		l.endHeredoc(l.pos, n)
	}
	return true
}

// closingMarkerAt returns the length of indentation plus label when the
// current heredoc's closing marker starts at line start i.
func (l *lexer) closingMarkerAt(i int) int {
	if len(l.labels) == 0 {
		return 0
	}
	name := l.labels[len(l.labels)-1].name
	j := i
	for j < len(l.src) && (l.src[j] == ' ' || l.src[j] == '\t') {
		j++
	}
	if !strings.HasPrefix(l.src[j:], name) {
		return 0
	}
	if k := j + len(name); k < len(l.src) && isLabelChar(l.src[k]) {
		return 0
	}
	return j + len(name) - i
}

func (l *lexer) endHeredoc(at, n int) {
	l.pos = at + n
	indent := l.src[at : at+n-len(l.labels[len(l.labels)-1].name)]
	if strings.Contains(indent, " ") && strings.Contains(indent, "\t") {
		l.fatal(-1, l.line, "Invalid indentation - tabs and spaces cannot be mixed")
	} else if l.labels[len(l.labels)-1].unindented {
		l.fatal(-1, l.line, "Invalid body indentation level (expecting an indentation level of at least %d)", len(indent))
	}
	l.emit(TEndHeredoc, at)
	l.labels = l.labels[:len(l.labels)-1]
	l.pop()
}

// checkIndent applies PHP 8.5.12's flexible heredoc rule to the body segment
// src[start:end]: every line the segment begins must start with the closing
// marker's indentation, using the same character, except whitespace-only
// lines. A segment that does not begin a line skips its first line.
func (l *lexer) checkIndent(start, end int, atLineStart bool) {
	label := l.labels[len(l.labels)-1]
	indent := label.indent
	if indent == "" || label.marker < 0 {
		return
	}
	spaces := indent[0] == ' '
	line := l.line
	i := start
	nextLine := func() bool {
		for i < end && l.src[i] != '\n' && l.src[i] != '\r' {
			i++
		}
		if i >= end {
			return false
		}
		if l.src[i] == '\r' && i+1 < end && l.src[i+1] == '\n' {
			i++
		}
		i++
		line++
		return true
	}
	if !atLineStart && !nextLine() {
		return
	}
	for i < end || (i == end && end < label.marker) {
		for k := 0; k < len(indent); k++ {
			c := byte(0)
			if i < len(l.src) {
				c = l.src[i]
			}
			if i < end && (c == '\n' || c == '\r') {
				break // whitespace-only line
			}
			if i >= end || (c != ' ' && c != '\t') {
				l.fatal(-1, line, "Invalid body indentation level (expecting an indentation level of at least %d)", len(indent))
				return
			}
			if (c == ' ') != spaces {
				l.fatal(-1, line, "Invalid indentation - tabs and spaces cannot be mixed")
				return
			}
			i++
		}
		if i >= end || !nextLine() {
			return
		}
	}
}

// bodyEnd finds where a heredoc body segment stops: before the newline that
// precedes the closing marker. It returns that index and the marker start.
func (l *lexer) heredocBodyLimit(from int, interpolates bool) (int, int) {
	for i := from; i < len(l.src); i++ {
		c := l.src[i]
		if interpolates {
			if c == '\\' && i+1 < len(l.src) && l.src[i+1] != '\n' && l.src[i+1] != '\r' {
				i++
				continue
			}
			// Interpolated code is not body text; skip it so a marker-like
			// line inside it is not taken as the end.
			if c == '{' && i+1 < len(l.src) && l.src[i+1] == '$' {
				i = l.skipBraces(i)
				continue
			}
			if c == '$' && i+1 < len(l.src) && l.src[i+1] == '{' {
				i = l.skipBraces(i + 1)
				continue
			}
		}
		if c != '\n' && c != '\r' {
			continue
		}
		lineStart := i + 1
		if c == '\r' && lineStart < len(l.src) && l.src[lineStart] == '\n' {
			lineStart++
		}
		if n := l.closingMarkerAt(lineStart); n > 0 {
			return i, lineStart
		}
		i = lineStart - 1
	}
	return len(l.src), -1
}

// skipBraces returns the index of the brace closing the one at i, ignoring
// braces inside quoted strings, or the end of input.
func (l *lexer) skipBraces(i int) int {
	if i >= 0 && i < len(l.src) && l.src[i] == '{' {
		if _, ok := l.braceEnds[i]; !ok && !l.overBudget {
			l.indexBraces(i)
		}
		if end, ok := l.braceEnds[i]; ok {
			return end
		}
		// The prescan ran out of budget; lexing will stop with
		// ErrTooManyTokens, so treat the rest of the input as skipped.
		return len(l.src)
	}
	depth := 0
	for ; i < len(l.src); i++ {
		switch c := l.src[i]; c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		case '\'', '"':
			for i++; i < len(l.src) && l.src[i] != c; i++ {
				if l.src[i] == '\\' {
					i++
				}
			}
		}
	}
	return len(l.src)
}

// recordBrace charges one brace to the prescan budget.
func (l *lexer) recordBrace() bool {
	if l.braceBudget <= 0 {
		l.overBudget = true
		return false
	}
	l.braceBudget--
	return true
}

// indexBraces scans PHP code and nested string bodies in one pass. Braces in
// string text and comments do not close an interpolation; interpolated code
// inside a string is scanned as code again.
func (l *lexer) indexBraces(from int) {
	if l.braceEnds == nil {
		l.braceEnds = make(map[int]int)
	}
	type frame struct {
		mode   byte // c: code, q: quoted body, h: heredoc, i: inline HTML
		quote  byte
		marker string
		nowdoc bool
		base   int
	}
	frames := []frame{{mode: 'c', base: 0}}
	stack := make([]int, 0)
	for i := from; i < len(l.src); {
		f := frames[len(frames)-1]
		if f.mode == 'i' {
			if l.src[i] == '<' {
				if _, n := l.openTagAt(i); n > 0 {
					frames = frames[:len(frames)-1]
					i += n
					continue
				}
			}
			i++
			continue
		}
		if f.mode == 'h' {
			if i == 0 || l.src[i-1] == '\n' || l.src[i-1] == '\r' {
				j := i
				for j < len(l.src) && (l.src[j] == ' ' || l.src[j] == '\t') {
					j++
				}
				if strings.HasPrefix(l.src[j:], f.marker) {
					end := j + len(f.marker)
					if end == len(l.src) || !isLabelChar(l.src[end]) {
						frames = frames[:len(frames)-1]
						i = end
						continue
					}
				}
			}
			if !f.nowdoc && l.src[i] == '\\' && i+1 < len(l.src) && l.src[i+1] != '\n' && l.src[i+1] != '\r' {
				i += 2
				continue
			}
			if !f.nowdoc && i+1 < len(l.src) && ((l.src[i] == '{' && l.src[i+1] == '$') || (l.src[i] == '$' && l.src[i+1] == '{')) {
				if l.src[i] == '$' {
					i++
				}
				frames = append(frames, frame{mode: 'c', base: len(stack)})
				if !l.recordBrace() {
					return
				}
				stack = append(stack, i)
				i++
				continue
			}
			i++
			continue
		}
		if f.mode == 'q' {
			if l.src[i] == '\\' && i+1 < len(l.src) {
				i += 2
				continue
			}
			if l.src[i] == f.quote {
				frames = frames[:len(frames)-1]
				i++
				continue
			}
			if i+1 < len(l.src) && ((l.src[i] == '{' && l.src[i+1] == '$') || (l.src[i] == '$' && l.src[i+1] == '{')) {
				if l.src[i] == '$' {
					i++
				}
				frames = append(frames, frame{mode: 'c', base: len(stack)})
				if !l.recordBrace() {
					return
				}
				stack = append(stack, i)
				i++
				continue
			}
			i++
			continue
		}
		switch c := l.src[i]; c {
		case '{':
			if !l.recordBrace() {
				return
			}
			stack = append(stack, i)
			i++
		case '}':
			if len(stack) == 0 {
				i++
				continue
			}
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			l.braceEnds[start] = i
			i++
			if len(stack) == f.base {
				if len(frames) == 1 {
					return
				}
				frames = frames[:len(frames)-1]
			}
		case '\'':
			i++
			for i < len(l.src) && l.src[i] != '\'' {
				if l.src[i] == '\\' && i+1 < len(l.src) {
					i++
				}
				i++
			}
			if i < len(l.src) {
				i++
			}
		case '"', '`':
			frames = append(frames, frame{mode: 'q', quote: c})
			i++
		case '/', '#':
			if c == '/' && i+1 < len(l.src) && l.src[i+1] == '*' {
				i += 2
				for i+1 < len(l.src) && !(l.src[i] == '*' && l.src[i+1] == '/') {
					i++
				}
				if i+1 < len(l.src) {
					i += 2
				} else {
					i = len(l.src)
				}
			} else if c == '#' && (i+1 >= len(l.src) || l.src[i+1] != '[') || c == '/' && i+1 < len(l.src) && l.src[i+1] == '/' {
				for i < len(l.src) && l.src[i] != '\n' && l.src[i] != '\r' && !(l.src[i] == '?' && i+1 < len(l.src) && l.src[i+1] == '>') {
					i++
				}
			} else {
				i++
			}
		case '?':
			if i+1 < len(l.src) && l.src[i+1] == '>' {
				frames = append(frames, frame{mode: 'i'})
				i += 2
			} else {
				i++
			}
		case '<':
			name, end, nowdoc, ok := l.braceHeredocStart(i)
			if ok {
				frames = append(frames, frame{mode: 'h', marker: name, nowdoc: nowdoc})
				i = end
			} else {
				i++
			}
		default:
			i++
		}
	}
	for _, start := range stack {
		l.braceEnds[start] = len(l.src)
	}
}

// braceHeredocStart recognizes a heredoc opener without changing lexer state.
func (l *lexer) braceHeredocStart(start int) (string, int, bool, bool) {
	if !strings.HasPrefix(l.src[start:], "<<<") {
		return "", start, false, false
	}
	i := start + 3
	for i < len(l.src) && (l.src[i] == ' ' || l.src[i] == '\t') {
		i++
	}
	quote := byte(0)
	if i < len(l.src) && (l.src[i] == '\'' || l.src[i] == '"') {
		quote = l.src[i]
		i++
	}
	if i >= len(l.src) || !isLabelStart(l.src[i]) {
		return "", start, false, false
	}
	end := l.labelEnd(i)
	name := l.src[i:end]
	if quote != 0 {
		if end >= len(l.src) || l.src[end] != quote {
			return "", start, false, false
		}
		end++
	}
	if end >= len(l.src) || (l.src[end] != '\n' && l.src[end] != '\r') {
		return "", start, false, false
	}
	if l.src[end] == '\r' && end+1 < len(l.src) && l.src[end+1] == '\n' {
		end++
	}
	return name, end + 1, quote == '\'', true
}

func (l *lexer) lexNowdoc() {
	start := l.pos
	marker := l.labels[len(l.labels)-1].marker
	if marker >= 0 && marker < l.pos {
		_, marker = l.heredocBodyLimit(l.pos, false)
	}
	if marker < 0 {
		l.diag("Unterminated nowdoc")
		l.pos = len(l.src)
		l.emit(TEncapsedAndWhitespace, start)
		return
	}
	l.pos = marker
	l.checkIndent(start, marker, true)
	l.emitRaw(TEncapsedAndWhitespace, start, marker)
	l.endHeredoc(marker, l.closingMarkerAt(marker))
}

// lexInterpolated scans double-quoted, backquoted (end '`'), or heredoc
// (end 0) bodies.
func (l *lexer) lexInterpolated(end byte) {
	start := l.pos
	if end != 0 && l.src[l.pos] == end {
		l.pos++
		l.emit(Kind(end), start)
		l.pop()
		return
	}
	if end == 0 {
		if n := l.closingMarkerAt(l.pos); n > 0 && (l.pos == 0 || l.src[l.pos-1] == '\n' || l.src[l.pos-1] == '\r') {
			l.endHeredoc(l.pos, n)
			return
		}
	}
	if l.interpolationAt(l.pos) {
		if end == 0 && (l.pos == 0 || l.src[l.pos-1] == '\n' || l.src[l.pos-1] == '\r') {
			if label := &l.labels[len(l.labels)-1]; label.indent != "" {
				label.unindented = true
			}
		}
		l.interpolation(start)
		return
	}
	limit := len(l.src)
	if end == 0 {
		label := &l.labels[len(l.labels)-1]
		if label.marker >= 0 && label.marker < l.pos {
			// The cached marker sat inside an interpolated expression, which
			// PHP lexes as code; search again from here.
			_, label.marker = l.heredocBodyLimit(l.pos, true)
		}
		if label.marker >= 0 {
			// The newline before the marker belongs to the body.
			limit = label.marker
		}
	}
	i := l.pos
	for i < limit {
		c := l.src[i]
		if c == '\\' && i+1 < limit {
			i += 2
			continue
		}
		if (end != 0 && c == end) || l.interpolationAt(i) {
			break
		}
		i++
	}
	if i > limit {
		i = limit
	}
	l.pos = i
	if end == 0 {
		atLineStart := start > 0 && (l.src[start-1] == '\n' || l.src[start-1] == '\r')
		l.checkIndent(start, i, atLineStart)
	}
	l.emit(TEncapsedAndWhitespace, start)
	if l.pos >= len(l.src) && end != 0 {
		l.diag("Unterminated string")
	}
}

func (l *lexer) interpolation(start int) {
	switch {
	case l.src[l.pos] == '{':
		l.pos++
		l.emit(TCurlyOpen, start)
		l.push(stScripting)
	case l.peek(1) == '{':
		l.pos += 2
		l.emit(TDollarOpenCurlyBraces, start)
		l.push(stLookingForVarname)
	default:
		l.pos = l.labelEnd(l.pos + 1)
		l.emit(TVariable, start)
		switch {
		case l.peek(0) == '[':
			l.push(stVarOffset)
		case l.peek(0) == '-' && l.peek(1) == '>' && isLabelStart(l.peek(2)):
			l.push(stLookingForProperty)
		case l.peek(0) == '?' && l.peek(1) == '-' && l.peek(2) == '>' && isLabelStart(l.peek(3)):
			l.push(stLookingForProperty)
		}
	}
}

func (l *lexer) lexProperty() {
	start := l.pos
	inString := len(l.states) >= 2 && l.states[len(l.states)-2] != stScripting
	c := l.src[l.pos]
	switch {
	case !inString && strings.IndexByte(" \t\n\r", c) >= 0:
		for l.pos < len(l.src) && strings.IndexByte(" \t\n\r", l.src[l.pos]) >= 0 {
			l.pos++
		}
		l.emit(TWhitespace, start)
	case !inString && (c == '#' && l.peek(1) != '[' || c == '/' && l.peek(1) == '/'):
		l.lineComment(start)
	case !inString && c == '/' && l.peek(1) == '*':
		l.blockComment(start)
	case c == '-' && l.peek(1) == '>':
		l.pos += 2
		l.emit(TObjectOperator, start)
	case c == '?' && l.peek(1) == '-' && l.peek(2) == '>':
		l.pos += 3
		l.emit(TNullsafeObjectOperator, start)
	case isLabelStart(c):
		l.pos = l.labelEnd(l.pos)
		l.emit(TString, start)
		l.pop()
	default:
		l.pop()
	}
}

var varOffsetNumbers = []*regexp.Regexp{
	regexp.MustCompile(`^0[xX][0-9a-fA-F]+(?:_[0-9a-fA-F]+)*`),
	regexp.MustCompile(`^0[bB][01]+(?:_[01]+)*`),
	regexp.MustCompile(`^0[oO][0-7]+(?:_[0-7]+)*`),
	regexp.MustCompile(`^[0-9]+(?:_[0-9]+)*`),
}

func varOffsetNumber(s string) int {
	best := 0
	for _, re := range varOffsetNumbers {
		if m := re.FindStringIndex(s); m != nil && m[1] > best {
			best = m[1]
		}
	}
	return best
}

func (l *lexer) lexVarOffset() {
	start := l.pos
	c := l.src[l.pos]
	switch {
	case isDigit(c):
		// PHP takes the longest decimal, hex, binary, or octal literal here;
		// anything after it starts the next token.
		l.pos = start + varOffsetNumber(l.src[start:])
		l.emit(TNumString, start)
	case c == '$' && isLabelStart(l.peek(1)):
		l.pos = l.labelEnd(l.pos + 1)
		l.emit(TVariable, start)
	case isLabelStart(c):
		l.pos = l.labelEnd(l.pos)
		l.emit(TString, start)
	case c == ']':
		l.pos++
		l.emit(']', start)
		l.pop()
	case strings.IndexByte(singleChars, c) >= 0:
		l.pos++
		l.emit(Kind(c), start)
	default:
		// Whitespace, quotes, and other bytes end the offset with an empty
		// string-body token, and the body resumes at this byte.
		l.emit(TEncapsedAndWhitespace, start)
		l.pop()
	}
}

func (l *lexer) lexVarname() {
	start := l.pos
	if isLabelStart(l.src[l.pos]) {
		end := l.labelEnd(l.pos)
		if end < len(l.src) && (l.src[end] == '[' || l.src[end] == '}') {
			l.pos = end
			l.emit(TStringVarname, start)
			l.setTop(stScripting)
			return
		}
	}
	l.setTop(stScripting)
	l.lexScripting()
}
