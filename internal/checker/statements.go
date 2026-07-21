package checker

import (
	"fmt"
	"strings"
)

type statementSignature struct {
	args     []valueType
	min      int
	max      int // -1は可変長
	repeat   valueType
	anyTypes bool
}

func fixedStatement(types ...valueType) statementSignature {
	return statementSignature{args: types, min: len(types), max: len(types)}
}

func optionalStatement(min int, types ...valueType) statementSignature {
	return statementSignature{args: types, min: min, max: len(types)}
}

func variadicStatement(min int, typ valueType) statementSignature {
	return statementSignature{min: min, max: -1, repeat: typ}
}

var statementSignatures = map[string]statementSignature{
	"ALIGN4": fixedStatement(), "BREAK": fixedStatement(), "CLEAR": fixedStatement(), "CLS": fixedStatement(),
	"CONTINUE": fixedStatement(), "ELSE": fixedStatement(), "END": fixedStatement(), "ENDIF": fixedStatement(),
	"GCLS": fixedStatement(), "IDLE": fixedStatement(), "NTP": fixedStatement(), "WEND": fixedStatement(),
	"BGCOLOR":    fixedStatement(typeInteger, typeInteger, typeInteger),
	"BOXFILL":    optionalStatement(3, typeInteger, typeInteger, typeInteger, typeInteger, typeInteger),
	"CIRCLE":     optionalStatement(2, typeInteger, typeInteger, typeInteger, typeInteger),
	"CIRCLEFILL": optionalStatement(2, typeInteger, typeInteger, typeInteger, typeInteger),
	"COLOR":      fixedStatement(typeInteger), "CORETIMER": fixedStatement(typeInteger),
	"CURSOR": fixedStatement(typeInteger, typeInteger), "DELAYMS": fixedStatement(typeInteger),
	"DELAYUS": fixedStatement(typeInteger), "DRAWCOUNT": fixedStatement(typeInteger),
	"FCLOSE": optionalStatement(0, typeInteger), "FILE": fixedStatement(typeInteger),
	"FOPEN": optionalStatement(2, typeString, typeString, typeInteger),
	"FGET":  fixedStatement(typeInteger, typeInteger), "FPUT": fixedStatement(typeInteger, typeInteger), "FPUTC": fixedStatement(typeInteger),
	"FREMOVE": fixedStatement(typeString), "FRENAME": fixedStatement(typeString, typeString),
	"FSEEK": fixedStatement(typeInteger), "GCOLOR": fixedStatement(typeInteger),
	"GPALETTE": fixedStatement(typeInteger, typeInteger, typeInteger, typeInteger),
	"GPRINT":   optionalStatement(4, typeInteger, typeInteger, typeInteger, typeInteger, typeString),
	"I2C":      optionalStatement(0, typeInteger), "LINE": optionalStatement(3, typeInteger, typeInteger, typeInteger, typeInteger, typeInteger),
	"MKDIR": fixedStatement(typeString), "MUSIC": fixedStatement(typeString),
	"OUT": fixedStatement(typeInteger, typeInteger), "OUT8L": fixedStatement(typeInteger),
	"OUT8H": fixedStatement(typeInteger), "OUT16": fixedStatement(typeInteger),
	"PALETTE":  fixedStatement(typeInteger, typeInteger, typeInteger, typeInteger),
	"PCG":      fixedStatement(typeInteger, typeInteger, typeInteger),
	"PLAYWAVE": optionalStatement(1, typeString, typeInteger),
	"POINT":    fixedStatement(typeInteger, typeInteger),
	"POKE":     fixedStatement(typeInteger, typeInteger), "POKE16": fixedStatement(typeInteger, typeInteger),
	"POKE32":    fixedStatement(typeInteger, typeInteger),
	"PSET":      optionalStatement(2, typeInteger, typeInteger, typeInteger),
	"PUTBMP":    fixedStatement(typeInteger, typeInteger, typeInteger, typeInteger, typeInteger),
	"PWM":       optionalStatement(1, typeInteger, typeInteger, typeInteger),
	"SCROLL":    fixedStatement(typeInteger, typeInteger),
	"SERIAL":    optionalStatement(2, typeInteger, typeInteger, typeInteger),
	"SERIALOUT": fixedStatement(typeInteger), "SETDIR": fixedStatement(typeString),
	"SETTIME": fixedStatement(typeString), "SPI": optionalStatement(1, typeInteger, typeInteger, typeInteger, typeInteger),
	"SYSTEM": fixedStatement(typeInteger, typeInteger), "TIMER": fixedStatement(typeInteger),
	"USEGRAPHIC": optionalStatement(0, typeInteger, typeInteger),
	"USEPCG":     optionalStatement(0, typeInteger), "USETIMER": fixedStatement(typeInteger),
	"WAIT": fixedStatement(typeInteger), "WIDTH": optionalStatement(1, typeInteger, typeInteger),
	"CDATA": variadicStatement(1, typeInteger), "EXEC": variadicStatement(1, typeInteger),
	"SPIWRITE": variadicStatement(1, typeInteger), "SPIWRITEDATA": variadicStatement(2, typeInteger),
	"SPIREADDATA": variadicStatement(2, typeInteger), "SPISWAPDATA": variadicStatement(2, typeInteger),
	"I2CWRITE": variadicStatement(1, typeInteger), "I2CWRITEDATA": variadicStatement(3, typeInteger),
	"I2CREADDATA": variadicStatement(3, typeInteger), "DELETE": variadicStatement(1, typeInteger),
	"DATA":   {min: 1, max: -1, anyTypes: true},
	"RETURN": {min: 0, max: 1, anyTypes: true},
}

func (c *context) checkCommandArguments(line int, statement string) {
	name, raw, ok := commandAndArguments(statement)
	if !ok {
		return
	}
	if name == "PRINT" || name == "FPRINT" {
		c.checkPrintArguments(line, raw)
		return
	}
	if name == "GOTO" || name == "GOSUB" || name == "RESTORE" || name == "SOUND" {
		c.checkLabelCommandArguments(line, name, raw)
		return
	}
	sig, ok := statementSignatures[name]
	if !ok {
		return // 制御構文、ラベル、クラス構文などは専用検査が担当する。
	}
	args := statementArguments(raw)
	count := len(args)
	if strings.TrimSpace(raw) == "" {
		count = 0
		args = nil
	}
	// GPRINTの座標対省略は先頭の「,」1つで表される。
	if name == "GPRINT" && len(args) == 4 && strings.TrimSpace(args[0]) == "" {
		args = append([]string{"", ""}, args[1:]...)
		count = 4
	}
	if count < sig.min || sig.max >= 0 && count > sig.max {
		expected := fmt.Sprintf("%d個", sig.min)
		if sig.max < 0 {
			expected = fmt.Sprintf("%d個以上", sig.min)
		} else if sig.min != sig.max {
			expected = fmt.Sprintf("%d～%d個", sig.min, sig.max)
		}
		c.add(line, 1, Error, "KM5101", fmt.Sprintf("命令 %s の引数は%s必要ですが、%d個指定されています", name, expected, count))
		return
	}
	for i, rawArg := range args {
		arg := strings.TrimSpace(rawArg)
		if arg == "" {
			continue // 座標など、途中の引数を省略できる命令がある。
		}
		actual, err := parseExpression(arg)
		if err != nil {
			c.addExpressionError(line, err)
			continue
		}
		if sig.anyTypes {
			continue
		}
		expected := sig.repeat
		if i < len(sig.args) {
			expected = sig.args[i]
		}
		if expected != typeInvalid && actual != expected {
			c.add(line, 1, Error, "KM5102", fmt.Sprintf("命令 %s の第%d引数は%sですが、%sが指定されています", name, i+1, expected, actual))
		}
	}
}

func (c *context) checkLabelCommandArguments(line int, name, raw string) {
	args := statementArguments(raw)
	if strings.TrimSpace(raw) == "" {
		args = nil
	}
	max := 1
	if name == "GOSUB" {
		max = -1
	}
	if len(args) < 1 || max >= 0 && len(args) > max || len(args) > 0 && strings.TrimSpace(args[0]) == "" {
		expected := "1"
		if max < 0 {
			expected = "1個以上"
		}
		c.add(line, 1, Error, "KM5101", fmt.Sprintf("命令 %s の引数は%s必要ですが、%d個指定されています", name, expected, len(args)))
		return
	}
	// GOSUBの第2引数以降は任意型の式として検査する。
	for _, rawArg := range args[1:] {
		if _, err := parseExpression(strings.TrimSpace(rawArg)); err != nil {
			c.addExpressionError(line, err)
		}
	}
}

func commandAndArguments(statement string) (string, string, bool) {
	s := strings.TrimSpace(statement)
	m := firstWordRE.FindStringIndex(s)
	if m == nil {
		return "", "", false
	}
	name := strings.ToUpper(s[m[0]:m[1]])
	if _, known := knownStatements[name]; !known {
		return "", "", false
	}
	return name, strings.TrimSpace(s[m[1]:]), true
}

func statementArguments(raw string) []string { return splitTopLevelOn(raw, ',') }

func splitTopLevelOn(s string, separator byte) []string {
	var parts []string
	start, depth, inString := 0, 0, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			if inString && i+1 < len(s) && s[i+1] == '"' {
				i++
				continue
			}
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString {
				depth--
			}
		default:
			if s[i] == separator && !inString && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

func (c *context) checkPrintArguments(line int, raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	for _, arg := range splitPrintArguments(raw) {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		if _, err := parseExpression(strings.TrimSpace(arg)); err != nil {
			c.addExpressionError(line, err)
		}
	}
}

func splitPrintArguments(s string) []string {
	var parts []string
	start, depth, inString := 0, 0, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			if inString && i+1 < len(s) && s[i+1] == '"' {
				i++
				continue
			}
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString {
				depth--
			}
		case ',', ';':
			if !inString && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}
