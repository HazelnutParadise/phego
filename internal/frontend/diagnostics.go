package frontend

import (
	"fmt"
	"slices"
	"strings"
)

const diagnosticWorkPerToken = 128

// diagnoseSyntax replays the release grammar's diagnostic transitions, without building an
// AST, executing PHP or retaining mutable state in the shared grammar data.
// Complete distinguishes a grammar conclusion from an incomplete bounded
// replay. Only (nil, true) means the grammar accepted the input.
func (p *parser) diagnoseSyntax() (diagnostic *SyntaxError, complete bool) {
	states := []uint16{0}
	type tokenRange struct{ start, end int }
	ranges := []tokenRange{{}}
	var brackets []Token
	position, lookahead, scannerLine := 0, -1, 1
	var token Token
	var pending *SyntaxError
	eof := p.eof
	// Each transition is a shift or a reduction. The factor bounds empty-rule
	// chains independently of hostile input, while stack depth is capped too.
	work := diagnosticWorkPerToken * (len(p.toks) + 1)
	for work > 0 {
		// PHP checks immediately after either kind of push, before the next
		// state action or accepting the parse.
		if len(states) >= MaxNesting {
			return &SyntaxError{Line: scannerLine, Message: "memory exhausted"}, true
		}
		work--
		state := int(states[len(states)-1])
		if state == diagnosticFinalState {
			return nil, true
		}
		off := diagnosticStateOffset + 8*state
		start, count, rule := diagnosticU32(off), diagnosticU16(off+4), diagnosticU16(off+6)
		action := -rule
		if count&0x8000 == 0 {
			if lookahead < 0 {
				token = eof
				if position < len(p.toks) {
					token = p.toks[position]
				}
				scannerLine = token.Line + countLines(token.Text)
				if token.Kind == TCloseTag {
					scannerLine = token.Line
				}
				if d, found := p.fatal[position]; found {
					pending = &SyntaxError{Line: d.Line, Message: d.Message, CallerLocation: d.CallerLocation}
					// Heredoc scanning restores the opening-token scanner state;
					// other scanner exceptions stop at their reported line.
					if !d.CallerLocation {
						scannerLine = d.Line
					}
				} else {
					pending = diagnosticBrackets(token, &brackets)
				}
				if pending != nil {
					// PHP returns T_ERROR with an exception pending. Reductions
					// can still replace it with an earlier semantic failure.
					lookahead = diagnosticErrorSymbol
				} else if token.Kind == tEOF {
					lookahead = 0
				} else if token.Kind >= 0 && int(token.Kind) < diagnosticSymbolCount {
					lookahead = diagnosticU16(diagnosticSymbolOffset + 2*int(token.Kind))
				} else {
					return nil, false
				}
			}
			if found, value := diagnosticLookup(diagnosticActionOffset, start, count, lookahead); found {
				action = int(int16(value))
			}
		}
		if action > 0 {
			states = append(states, uint16(action))
			ranges = append(ranges, tokenRange{position, position + 1})
			position++
			lookahead = -1
			continue
		}
		if action == 0 {
			if pending != nil {
				return pending, true
			}
			return diagnosticUnexpected(token, start, count&0x7fff), true
		}
		rule = -action
		if rule <= 0 || rule >= diagnosticRuleCount {
			return nil, false
		}
		ro := diagnosticRuleOffset + 8*rule
		lhs, pop := diagnosticU16(ro), diagnosticU16(ro+2)
		if pop >= len(states) {
			return nil, false
		}
		base := len(ranges) - pop
		span := tokenRange{position, position}
		if pop != 0 {
			span = tokenRange{ranges[base].start, ranges[len(ranges)-1].end}
		}
		guard, operand := diagnosticU16(ro+4), diagnosticU16(ro+6)
		var err *SyntaxError
		switch guard {
		case diagnosticGuardIdentifier:
			r := ranges[base+operand]
			if r.end == r.start+1 && p.toks[r.start].Kind == TOpenTagWithEcho {
				err = &SyntaxError{Line: scannerLine, Message: `Cannot use "<?=" as an identifier`}
			}
		case diagnosticGuardHalt:
			// Only the completed top-level halt statement stops the scanner.
			eof = Token{Kind: tEOF, Line: scannerLine}
			position = len(p.toks)
			lookahead = -1
		case diagnosticGuardInnerHalt:
			err = &SyntaxError{Line: scannerLine, Compile: true, Message: "__HALT_COMPILER() can only be used from the outermost scope"}
		case diagnosticGuardClass, diagnosticGuardAnonymousClass:
			err = validateClassModifiers(slices.Values(p.toks[span.start:span.end]), guard == diagnosticGuardAnonymousClass, scannerLine)
		case diagnosticGuardProperty, diagnosticGuardMethod, diagnosticGuardConstant, diagnosticGuardParameter, diagnosticGuardPropertyHook:
			targets := [...]string{targetProperty, targetMethod, targetConstant, targetParameter, targetPropertyHook}
			r := ranges[base+operand]
			err = validateModifiers(slices.Values(p.toks[r.start:r.end]), targets[guard-diagnosticGuardProperty], scannerLine)
		case diagnosticGuardEncoding:
			// This is an empty mid-rule action after declare '(' const_list
			// ')'. The list is the second stack entry from the top.
			if len(ranges) < 3 {
				return nil, false
			}
			r := ranges[len(ranges)-2]
			if r.start < 0 || r.end > len(p.toks) || r.start >= r.end {
				return nil, false
			}
			var known bool
			err, known = diagnosticEncoding(p.toks[r.start:r.end], scannerLine)
			if !known {
				return nil, false
			}
		case diagnosticGuardNone:
		default:
			return nil, false
		}
		if err != nil {
			return err, true
		}
		ranges = append(ranges[:base], span)
		states = states[:len(states)-pop]
		goff := diagnosticGotoOffset + 8*lhs
		gs, gc, target := diagnosticU32(goff), diagnosticU16(goff+4), diagnosticU16(goff+6)
		if found, value := diagnosticLookup(diagnosticJumpOffset, gs, gc, int(states[len(states)-1])); found {
			target = value
		}
		states = append(states, uint16(target))
	}
	return nil, false
}

// Reuse the expression parser and PHP literal classification for the rare
// encoding declaration. No declaration body or execution state is retained.
func diagnosticEncoding(tokens []Token, line int) (diagnostic *SyntaxError, complete bool) {
	// The grammar has already accepted const_list. Avoid constructing another
	// tree for declarations such as ticks that have no parse-time value check.
	hasEncoding := false
	for _, token := range tokens {
		if token.Kind == TString && strings.EqualFold(token.Text, "encoding") {
			hasEncoding = true
			break
		}
	}
	if !hasEncoding {
		return nil, true
	}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(parseFailure); !ok {
				panic(r)
			}
			diagnostic, complete = nil, false
		}
	}()
	q := &parser{toks: tokens, eof: Token{Kind: tEOF, Line: line}}
	for {
		name := q.identifier()
		q.expect('=')
		value := q.expr()
		if strings.EqualFold(name.Text, "encoding") && !encodingLiteral(value) {
			return &SyntaxError{Line: line, Message: "Encoding must be a literal", Compile: true}, true
		}
		if !q.accept(',') {
			return nil, q.pos == len(tokens)
		}
	}
}

func diagnosticU16(at int) int { return int(diagnosticData[at]) | int(diagnosticData[at+1])<<8 }
func diagnosticU32(at int) int { return diagnosticU16(at) | diagnosticU16(at+2)<<16 }
func diagnosticLookup(base, start, count, key int) (bool, int) {
	low, high := 0, count
	for low < high {
		mid := low + (high-low)/2
		k := diagnosticU16(base + 4*(start+mid))
		if k < key {
			low = mid + 1
		} else {
			high = mid
		}
	}
	if low < count && diagnosticU16(base+4*(start+low)) == key {
		return true, diagnosticU16(base + 4*(start+low) + 2)
	}
	return false, 0
}
func diagnosticLabel(symbol int) string {
	at := diagnosticNameOffset + 6*symbol
	start, size := diagnosticU32(at), diagnosticU16(at+4)
	return diagnosticData[start : start+size]
}
func diagnosticTokenLabel(t Token) string {
	if t.Kind == tEOF {
		return diagnosticLabel(0)
	}
	if t.Kind >= 0 && int(t.Kind) < diagnosticSymbolCount {
		return diagnosticLabel(diagnosticU16(diagnosticSymbolOffset + 2*int(t.Kind)))
	}
	return "invalid token"
}

// The second result marks an embedded NUL in displayed token content. PHP
// formats that content as a C string and leaves its expected-token suffix hidden.
func diagnosticDescription(t Token) (string, bool) {
	label := diagnosticTokenLabel(t)
	if t.Kind == tEOF {
		return "end of file", false
	}
	if label == "amp" {
		return `token "&"`, false
	}
	if label == "'\"'" {
		return "double-quote mark", false
	}
	if len(label) >= 2 && label[0] == '\'' {
		return "token \"" + label[1:len(label)-1] + "\"", false
	}
	if t.Kind == TBadCharacter && len(t.Text) == 1 {
		return fmt.Sprintf("character 0x%02X", t.Text[0]), false
	}
	content := t.Text
	if end := strings.IndexByte(content, '\n'); end >= 0 {
		content = content[:end]
	}
	if t.Kind == TConstantEncapsedString && len(content) > 0 {
		if content[0] == '\'' {
			label = "single-quoted string"
		} else if content[0] == '"' {
			label = "double-quoted string"
		}
	}
	if len(content) > 0 && (content[0] == '\'' || content[0] == '"') {
		content = content[1:]
	}
	if len(content) > 0 && (content[len(content)-1] == '\'' || content[len(content)-1] == '"') {
		content = content[:len(content)-1]
	}
	truncated := len(content) > 33
	if truncated {
		content = content[:30]
	}
	nul := strings.IndexByte(content, 0)
	if nul >= 0 {
		content = content[:nul]
	}
	if truncated {
		content += "..."
	}
	return label + " \"" + content + "\"", nul >= 0
}

func diagnosticUnexpected(t Token, start, count int) *SyntaxError {
	line := t.Line + countLines(t.Text)
	if t.Kind == TCloseTag || t.Kind == TEncapsedAndWhitespace && (strings.HasPrefix(t.Text, "'") || strings.HasPrefix(t.Text, "b'") || strings.HasPrefix(t.Text, "B'")) {
		line = t.Line
	}
	description, nul := diagnosticDescription(t)
	message := "syntax error, unexpected " + description
	if nul {
		return &SyntaxError{Line: line, Message: message}
	}
	expected := make([]string, 0, 4)
	for i := 0; i < count; i++ {
		at := diagnosticActionOffset + 4*(start+i)
		symbol, action := diagnosticU16(at), int16(diagnosticU16(at+2))
		if symbol == 1 || action == 0 {
			continue
		}
		if len(expected) == 4 {
			expected = nil
			break
		}
		label := diagnosticLabel(symbol)
		if label == "amp" {
			label = `token "&"`
		}
		if len(label) >= 2 && label[0] == '\'' {
			label = "\"" + label[1:len(label)-1] + "\""
		}
		expected = append(expected, label)
	}
	if len(expected) > 0 {
		message += ", expecting " + strings.Join(expected, " or ")
	}
	return &SyntaxError{Line: line, Message: message}
}

func diagnosticBrackets(t Token, stack *[]Token) *SyntaxError {
	opening := Kind(0)
	switch t.Kind {
	case '(', '[', '{':
		opening = t.Kind
	case TAttribute:
		opening = '['
	case TCurlyOpen, TDollarOpenCurlyBraces:
		opening = '{'
	}
	if opening != 0 {
		*stack = append(*stack, Token{Kind: opening, Line: t.Line})
		return nil
	}
	if t.Kind != ')' && t.Kind != ']' && t.Kind != '}' && t.Kind != tEOF {
		return nil
	}
	if len(*stack) == 0 {
		if t.Kind == tEOF {
			return nil
		}
		return &SyntaxError{Line: t.Line, Message: fmt.Sprintf("Unmatched '%c'", t.Kind)}
	}
	top := (*stack)[len(*stack)-1]
	if top.Kind == '(' && t.Kind == ')' || top.Kind == '[' && t.Kind == ']' || top.Kind == '{' && t.Kind == '}' {
		*stack = (*stack)[:len(*stack)-1]
		return nil
	}
	message := fmt.Sprintf("Unclosed '%c'", top.Kind)
	if top.Line != t.Line {
		message += fmt.Sprintf(" on line %d", top.Line)
	}
	if t.Kind != tEOF {
		message += fmt.Sprintf(" does not match '%c'", t.Kind)
	}
	return &SyntaxError{Line: t.Line, Message: message}
}
