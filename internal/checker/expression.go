package checker

import (
	"fmt"
	"strings"
	"unicode"
)

type valueType uint8

const (
	typeInvalid valueType = iota
	typeInteger
	typeReal
	typeString
)

func (t valueType) String() string {
	switch t {
	case typeInteger:
		return "整数型"
	case typeReal:
		return "実数型"
	case typeString:
		return "文字列型"
	default:
		return "不明な型"
	}
}

type expressionError struct {
	Pos     int
	Code    string
	Message string
}

func (e *expressionError) Error() string { return e.Message }

type exprTokenKind uint8

const (
	tokEOF exprTokenKind = iota
	tokIdent
	tokInteger
	tokReal
	tokString
	tokOperator
	tokLParen
	tokRParen
	tokComma
)

type exprToken struct {
	kind exprTokenKind
	text string
	pos  int
}

type expressionParser struct {
	tokens []exprToken
	index  int
}

type functionSignature struct {
	result valueType
	args   []valueType
	min    int
}

var functionSignatures = map[string]functionSignature{
	"INT": {typeInteger, []valueType{typeReal}, 1}, "FLOAT#": {typeReal, []valueType{typeInteger}, 1},
	"VAL": {typeInteger, []valueType{typeString}, 1}, "VAL#": {typeReal, []valueType{typeString}, 1},
	"ASC": {typeInteger, []valueType{typeString}, 1}, "CHR$": {typeString, []valueType{typeInteger}, 1},
	"FLOAT$": {typeString, []valueType{typeReal}, 1}, "DEC$": {typeString, []valueType{typeInteger}, 1},
	"HEX$": {typeString, []valueType{typeInteger, typeInteger}, 1}, "SPRINTF$": {typeString, []valueType{typeString, typeReal}, 2},
	"LEN": {typeInteger, []valueType{typeString}, 1}, "STRCMP": {typeInteger, []valueType{typeString, typeString}, 2},
	"STRNCMP": {typeInteger, []valueType{typeString, typeString, typeInteger}, 3},
	"ABS":     {typeInteger, []valueType{typeInteger}, 1}, "FABS#": {typeReal, []valueType{typeReal}, 1},
	"ACOS#": {typeReal, []valueType{typeReal}, 1}, "ASIN#": {typeReal, []valueType{typeReal}, 1},
	"ATAN#": {typeReal, []valueType{typeReal}, 1}, "ATAN2#": {typeReal, []valueType{typeReal, typeReal}, 2},
	"CEIL#": {typeReal, []valueType{typeReal}, 1}, "COS#": {typeReal, []valueType{typeReal}, 1},
	"COSH#": {typeReal, []valueType{typeReal}, 1}, "EXP#": {typeReal, []valueType{typeReal}, 1},
	"FLOOR#": {typeReal, []valueType{typeReal}, 1}, "FMOD#": {typeReal, []valueType{typeReal, typeReal}, 2},
	"LOG#": {typeReal, []valueType{typeReal}, 1}, "LOG10#": {typeReal, []valueType{typeReal}, 1},
	"MODF#": {typeReal, []valueType{typeReal}, 1}, "POW#": {typeReal, []valueType{typeReal, typeReal}, 2},
	"SIN#": {typeReal, []valueType{typeReal}, 1}, "SINH#": {typeReal, []valueType{typeReal}, 1},
	"SQRT#": {typeReal, []valueType{typeReal}, 1}, "TAN#": {typeReal, []valueType{typeReal}, 1},
	"TANH#": {typeReal, []valueType{typeReal}, 1},
}

func parseExpression(s string) (valueType, *expressionError) {
	tokens, err := lexExpression(s)
	if err != nil {
		return typeInvalid, err
	}
	p := &expressionParser{tokens: tokens}
	t, err := p.parseBinary(1)
	if err != nil {
		return typeInvalid, err
	}
	if p.peek().kind != tokEOF {
		return typeInvalid, syntaxError(p.peek(), fmt.Sprintf("式の末尾に解釈できない %q があります", p.peek().text))
	}
	return t, nil
}

func lexExpression(s string) ([]exprToken, *expressionError) {
	var out []exprToken
	for i := 0; i < len(s); {
		if unicode.IsSpace(rune(s[i])) {
			i++
			continue
		}
		start := i
		switch ch := s[i]; {
		case ch == '"':
			i++
			for i < len(s) {
				if s[i] == '"' {
					if i+1 < len(s) && s[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			if i > len(s) || s[i-1] != '"' {
				return nil, &expressionError{start, "KM5001", "文字列リテラルが閉じられていません"}
			}
			out = append(out, exprToken{tokString, s[start:i], start})
		case ch >= '0' && ch <= '9' || ch == '$':
			if ch == '$' {
				i++
				for i < len(s) && isHex(s[i]) {
					i++
				}
				if i == start+1 {
					return nil, &expressionError{start, "KM5001", "16進整数が不正です"}
				}
				out = append(out, exprToken{tokInteger, s[start:i], start})
				continue
			}
			i++
			if ch == '0' && i < len(s) && (s[i] == 'x' || s[i] == 'X') {
				i++
				for i < len(s) && isHex(s[i]) {
					i++
				}
				out = append(out, exprToken{tokInteger, s[start:i], start})
				continue
			}
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			kind := tokInteger
			if i < len(s) && s[i] == '.' {
				kind = tokReal
				i++
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
			}
			if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
				kind = tokReal
				i++
				if i < len(s) && (s[i] == '+' || s[i] == '-') {
					i++
				}
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
			}
			out = append(out, exprToken{kind, s[start:i], start})
		case isIdentStart(ch):
			i++
			for i < len(s) && (isIdentStart(s[i]) || s[i] >= '0' && s[i] <= '9') {
				i++
			}
			if i < len(s) && (s[i] == '$' || s[i] == '#') {
				i++
			}
			// オブジェクトのフィールド/メソッドとクラスのSTATIC参照。
			for i < len(s) {
				separatorLength := 0
				if s[i] == '.' {
					separatorLength = 1
				} else if i+1 < len(s) && s[i:i+2] == "::" {
					separatorLength = 2
				}
				if separatorLength == 0 || i+separatorLength >= len(s) || !isIdentStart(s[i+separatorLength]) {
					break
				}
				i += separatorLength + 1
				for i < len(s) && (isIdentStart(s[i]) || s[i] >= '0' && s[i] <= '9') {
					i++
				}
				if i < len(s) && (s[i] == '$' || s[i] == '#') {
					i++
				}
			}
			text := strings.ToUpper(s[start:i])
			if text == "AND" || text == "OR" || text == "XOR" {
				out = append(out, exprToken{tokOperator, text, start})
			} else {
				out = append(out, exprToken{tokIdent, text, start})
			}
		case ch == '(':
			i++
			out = append(out, exprToken{tokLParen, "(", start})
		case ch == ')':
			i++
			out = append(out, exprToken{tokRParen, ")", start})
		case ch == ',':
			i++
			out = append(out, exprToken{tokComma, ",", start})
		default:
			i++
			if i < len(s) && ((ch == '<' && (s[i] == '<' || s[i] == '=')) || (ch == '>' && (s[i] == '>' || s[i] == '=')) || (ch == '!' && s[i] == '=')) {
				i++
			}
			text := s[start:i]
			if !strings.Contains("+-*/%<>=!&", string(ch)) {
				return nil, &expressionError{start, "KM5001", fmt.Sprintf("式中の文字 %q を解釈できません", ch)}
			}
			out = append(out, exprToken{tokOperator, text, start})
		}
	}
	out = append(out, exprToken{kind: tokEOF, pos: len(s)})
	return out, nil
}

func (p *expressionParser) parseBinary(minPrec int) (valueType, *expressionError) {
	left, err := p.parseUnary()
	if err != nil {
		return typeInvalid, err
	}
	for {
		op := p.peek()
		prec := operatorPrecedence(op.text)
		if op.kind != tokOperator || prec < minPrec {
			break
		}
		p.index++
		right, err := p.parseBinary(prec + 1)
		if err != nil {
			return typeInvalid, err
		}
		resultType, compatible := commonOperandType(left, right)
		if !compatible {
			return typeInvalid, &expressionError{op.pos, "KM5003", fmt.Sprintf("演算子 %s の左右で型が異なります（%sと%s）", op.text, left, right)}
		}
		if !operatorAllowed(op.text, resultType) {
			return typeInvalid, &expressionError{op.pos, "KM5003", fmt.Sprintf("演算子 %s は%sには使用できません", op.text, resultType)}
		}
		left = resultType
	}
	return left, nil
}

// commonOperandType は二項演算時の共通型を返す。
// 整数と実数の組み合わせは実数へ昇格し、文字列との混在は許可しない。
func commonOperandType(left, right valueType) (valueType, bool) {
	if left == right {
		return left, true
	}
	if left == typeInteger && right == typeReal || left == typeReal && right == typeInteger {
		return typeReal, true
	}
	return typeInvalid, false
}

func (p *expressionParser) parseUnary() (valueType, *expressionError) {
	if p.peek().kind == tokOperator && (p.peek().text == "+" || p.peek().text == "-" || p.peek().text == "&") {
		op := p.take()
		if op.text == "&" {
			next := p.peek()
			if next.kind != tokIdent {
				return typeInvalid, &expressionError{op.pos, "KM5003", "単項演算子 & の後ろには整数型変数が必要です"}
			}
			if _, isFunction := knownFunctions[next.text]; isFunction {
				return typeInvalid, &expressionError{op.pos, "KM5003", "単項演算子 & は関数には使用できません"}
			}
		}
		t, err := p.parseUnary()
		if err != nil {
			return typeInvalid, err
		}
		if t == typeString || (op.text == "&" && t != typeInteger) {
			return typeInvalid, &expressionError{op.pos, "KM5003", fmt.Sprintf("単項演算子 %s は%sには使用できません", op.text, t)}
		}
		return t, nil
	}
	return p.parsePrimary()
}

func (p *expressionParser) parsePrimary() (valueType, *expressionError) {
	tok := p.take()
	switch tok.kind {
	case tokInteger:
		return typeInteger, nil
	case tokReal:
		return typeReal, nil
	case tokString:
		return typeString, nil
	case tokLParen:
		t, err := p.parseBinary(1)
		if err != nil {
			return typeInvalid, err
		}
		if p.peek().kind != tokRParen {
			return typeInvalid, syntaxError(p.peek(), "閉じ括弧がありません")
		}
		p.index++
		return t, nil
	case tokIdent:
		result := typeFromName(tok.text)
		if p.peek().kind != tokLParen {
			return result, nil
		}
		p.index++
		var args []valueType
		if p.peek().kind != tokRParen {
			for {
				t, err := p.parseBinary(1)
				if err != nil {
					return typeInvalid, err
				}
				args = append(args, t)
				if p.peek().kind != tokComma {
					break
				}
				p.index++
			}
		}
		if p.peek().kind != tokRParen {
			return typeInvalid, syntaxError(p.peek(), "関数または配列の閉じ括弧がありません")
		}
		p.index++
		if sig, ok := functionSignatures[tok.text]; ok {
			if len(args) < sig.min || len(args) > len(sig.args) {
				return typeInvalid, &expressionError{tok.pos, "KM5004", fmt.Sprintf("関数 %s の引数個数が不正です", tok.text)}
			}
			for i, actual := range args {
				if actual != sig.args[i] {
					return typeInvalid, &expressionError{tok.pos, "KM5004", fmt.Sprintf("関数 %s の第%d引数は%sですが、%sが指定されています", tok.text, i+1, sig.args[i], actual)}
				}
			}
			return sig.result, nil
		}
		// 関数名は接尾辞で戻り値型が決まる。配列添字も常に整数型。
		if _, isFunction := knownFunctions[tok.text]; !isFunction {
			for _, arg := range args {
				if arg != typeInteger {
					return typeInvalid, &expressionError{tok.pos, "KM5005", fmt.Sprintf("配列 %s の添字には整数型が必要です", tok.text)}
				}
			}
		}
		return result, nil
	default:
		return typeInvalid, syntaxError(tok, "式が必要です")
	}
}

func (p *expressionParser) peek() exprToken { return p.tokens[p.index] }
func (p *expressionParser) take() exprToken {
	t := p.peek()
	if t.kind != tokEOF {
		p.index++
	}
	return t
}
func syntaxError(t exprToken, message string) *expressionError {
	return &expressionError{t.pos, "KM5001", message}
}
func isIdentStart(ch byte) bool { return ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' }
func isHex(ch byte) bool {
	return ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F'
}
func typeFromName(name string) valueType {
	if strings.HasSuffix(name, "$") {
		return typeString
	}
	if strings.HasSuffix(name, "#") {
		return typeReal
	}
	return typeInteger
}

func operatorPrecedence(op string) int {
	switch strings.ToUpper(op) {
	case "OR":
		return 1
	case "AND":
		return 2
	case "XOR":
		return 3
	case "=", "!=":
		return 4
	case "<", "<=", ">", ">=":
		return 5
	case "<<", ">>":
		return 6
	case "+", "-":
		return 7
	case "*", "/", "%":
		return 8
	default:
		return 0
	}
}

func operatorAllowed(op string, typ valueType) bool {
	op = strings.ToUpper(op)
	switch typ {
	case typeString:
		return op == "+" || op == "=" || op == "!=" || op == "<" || op == "<=" || op == ">" || op == ">=" || op == "AND" || op == "OR"
	case typeReal:
		return op != "%" && op != "<<" && op != ">>" && op != "XOR"
	case typeInteger:
		return true
	}
	return false
}
