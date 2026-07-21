package checker

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type sourceLine struct {
	Number int
	Text   string
}

type blockEntry struct {
	Kind string
	Line int
}

type labelRef struct {
	Name string
	Line int
	Col  int
	Kind string
}

var (
	labelNameRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	longVarRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:[$#])?$`)
	lineNumberRE = regexp.MustCompile(`^\s*([0-9]+)\s+(.*)$`)
	labelStmtRE  = regexp.MustCompile(`(?i)^\s*LABEL\s+([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	useVarRE     = regexp.MustCompile(`(?i)^\s*USEVAR\s+(.+)$`)
	dimStmtRE    = regexp.MustCompile(`(?i)^\s*DIM\s+(.+)$`)
	varStmtRE    = regexp.MustCompile(`(?i)^\s*VAR\s+(.+)$`)
	jumpRE       = regexp.MustCompile(`(?i)\b(GOTO|GOSUB|RESTORE|SOUND)\s+([A-Za-z_][A-Za-z0-9_]*|[0-9]+)\b`)
	forRE        = regexp.MustCompile(`(?i)^\s*FOR\s+([A-Za-z_][A-Za-z0-9_]*)([$#]?)\s*=`)
	nextRE       = regexp.MustCompile(`(?i)^\s*NEXT(?:\s+([A-Za-z_][A-Za-z0-9_]*))?\s*$`)
	callRE       = regexp.MustCompile(`(?i)([A-Za-z_][A-Za-z0-9_]*(?:[$#])?)\s*\(`)
	firstWordRE  = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*(?:[$#])?)`)
	identifierRE = regexp.MustCompile(`(?i)[A-Za-z_][A-Za-z0-9_]*(?:[$#])?`)
)

var reserved = languageReservedWords()

func CheckFile(path string, opts Options) (FileResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileResult{}, err
	}
	defer f.Close()

	lines := []sourceLine{}
	sc := bufio.NewScanner(f)
	// BASICソース中の長いCDATA行などにも対応する。
	sc.Buffer(make([]byte, 4096), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		lines = append(lines, sourceLine{Number: n, Text: strings.TrimSuffix(sc.Text(), "\r")})
	}
	if err := sc.Err(); err != nil {
		return FileResult{}, err
	}

	c := newContext(path, opts)
	c.check(lines)
	sort.SliceStable(c.result.Diagnostics, func(i, j int) bool {
		a, b := c.result.Diagnostics[i], c.result.Diagnostics[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return a.Code < b.Code
	})
	return c.result, nil
}

type context struct {
	result          FileResult
	options         Options
	blocks          []blockEntry
	labels          map[string]int
	lineNumbers     map[string]int
	refs            []labelRef
	declaredVars    map[string]int
	arrays          map[string]int
	classes         map[string]classDefinition
	usedClasses     map[string]struct{}
	validateClasses bool
}

func newContext(path string, opts Options) *context {
	return &context{
		result:          FileResult{Path: path},
		options:         opts,
		labels:          map[string]int{},
		lineNumbers:     map[string]int{},
		declaredVars:    map[string]int{},
		arrays:          map[string]int{},
		classes:         loadClassLibraries(opts.LibraryDirs),
		usedClasses:     map[string]struct{}{},
		validateClasses: len(opts.LibraryDirs) > 0,
	}
}

func (c *context) check(lines []sourceLine) {
	for _, ln := range lines {
		c.checkPhysicalLine(ln)
	}

	// 閉じられていない構造を報告する。
	for i := len(c.blocks) - 1; i >= 0; i-- {
		b := c.blocks[i]
		c.add(b.Line, 1, Error, "KM1001",
			fmt.Sprintf("%s に対応する終端命令がありません", b.Kind))
	}

	// ラベル・行番号の参照解決。
	for _, ref := range c.refs {
		key := strings.ToUpper(ref.Name)
		if _, ok := c.labels[key]; ok {
			continue
		}
		if _, ok := c.lineNumbers[key]; ok {
			continue
		}
		c.add(ref.Line, ref.Col, Error, "KM2001",
			fmt.Sprintf("%s の参照先 %q が定義されていません", ref.Kind, ref.Name))
	}
}

func (c *context) checkPhysicalLine(ln sourceLine) {
	text := ln.Text
	if strings.TrimSpace(text) == "" {
		return
	}

	// 行番号を先に切り離す。
	body := text
	if m := lineNumberRE.FindStringSubmatch(text); m != nil {
		num := m[1]
		if prev, exists := c.lineNumbers[num]; exists {
			c.add(ln.Number, 1, Error, "KM2002",
				fmt.Sprintf("行番号 %s は%d行目でも定義されています", num, prev))
		} else {
			c.lineNumbers[num] = ln.Number
		}
		body = m[2]
	}

	// REM以降は引用符やコロンを含め、すべてコメント本文として扱う。
	code := body
	if pos := remCommentStart(body); pos >= 0 {
		code = body[:pos]
	}
	if hasUnclosedString(code) {
		c.add(ln.Number, firstQuoteColumn(code), Error, "KM0001", "文字列リテラルの閉じる引用符がありません")
	}
	if col := apostropheColumn(code); col > 0 {
		c.add(ln.Number, col, Error, "KM0002", "KM-BASICではアポストロフィーによるコメントは使用できません。REMを使用してください")
	}

	for _, stmt := range splitStatements(body) {
		c.checkStatement(ln.Number, stmt)
	}
}

func (c *context) checkStatement(line int, stmt string) {
	s := strings.TrimSpace(stmt)
	if s == "" {
		return
	}
	// 物理行の検査でKM0002を報告済み。後続を式として解析して連鎖エラーにしない。
	if apostropheColumn(s) > 0 {
		return
	}

	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "REM") &&
		(len(upper) == 3 || unicode.IsSpace(rune(upper[3]))) {
		return
	}

	if m := labelStmtRE.FindStringSubmatch(s); m != nil {
		c.checkLabel(line, m[1])
		return
	}

	if m := useVarRE.FindStringSubmatch(s); m != nil {
		c.checkVariableList(line, m[1], "USEVAR")
		return
	}
	if m := dimStmtRE.FindStringSubmatch(s); m != nil {
		c.checkDimList(line, m[1])
		c.checkKnownNames(line, s)
		return
	}
	if m := varStmtRE.FindStringSubmatch(s); m != nil {
		c.checkVariableList(line, m[1], "VAR")
		return
	}
	if c.checkClassStatement(line, s) {
		return
	}

	for _, m := range jumpRE.FindAllStringSubmatchIndex(s, -1) {
		kind := strings.ToUpper(s[m[2]:m[3]])
		name := s[m[4]:m[5]]
		c.refs = append(c.refs, labelRef{
			Name: name, Line: line, Col: m[4] + 1, Kind: kind,
		})
	}

	c.checkBlocks(line, s)
	c.checkForVariable(line, s)
	c.checkKnownNames(line, s)
	c.checkLongVariableUsage(line, s)
	c.checkStatementTypes(line, s)
}

func (c *context) checkLabel(line int, name string) {
	key := strings.ToUpper(name)
	if _, ok := reserved[key]; ok {
		c.add(line, 1, Error, "KM2003",
			fmt.Sprintf("%q は予約語のためラベル名に使用できません", name))
	}
	if !labelNameRE.MatchString(name) {
		c.add(line, 1, Error, "KM2004", "ラベル名が不正です")
		return
	}

	// type P/PU以外はガイド記載の6文字制限を適用する。
	if c.options.Target != "type-p" && c.options.Target != "type-pu" && len(name) > 6 {
		c.add(line, 1, Error, "KM2005",
			fmt.Sprintf("対象機種 %s ではラベル名は6文字以内です", c.options.Target))
	}

	if prev, exists := c.labels[key]; exists {
		c.add(line, 1, Error, "KM2006",
			fmt.Sprintf("ラベル %q は%d行目でも定義されています", name, prev))
		return
	}
	c.labels[key] = line
}

func (c *context) checkVariableList(line int, raw, kind string) {
	for _, item := range strings.Split(raw, ",") {
		name := strings.TrimSpace(item)
		if name == "" {
			c.add(line, 1, Error, "KM3001", kind+" の変数名が空です")
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, "$"), "#")
		key := strings.ToUpper(base)

		if !longVarRE.MatchString(name) {
			c.add(line, 1, Error, "KM3002",
				fmt.Sprintf("変数名 %q が不正です", name))
			continue
		}
		if _, ok := reserved[key]; ok {
			c.add(line, 1, Error, "KM3003",
				fmt.Sprintf("%q は予約語のため変数名に使用できません", name))
		}
		if kind == "USEVAR" && len(base) == 1 {
			c.add(line, 1, Warning, "KM3004",
				fmt.Sprintf("1文字変数 %q はUSEVAR宣言なしで利用できます", name))
		}
		if kind == "VAR" && len(base) > 1 {
			if _, declared := c.declaredVars[key]; !declared {
				c.add(line, 1, Error, "KM3007", fmt.Sprintf("2文字以上の変数 %q は使用前にUSEVARで宣言してください", name))
			}
		}
		if c.options.Target != "type-p" && c.options.Target != "type-pu" && len(base) > 6 {
			c.add(line, 1, Error, "KM3005",
				fmt.Sprintf("対象機種 %s では長文字変数名は6文字以内です", c.options.Target))
		}
		if prev, exists := c.declaredVars[key]; exists && kind == "USEVAR" {
			c.add(line, 1, Warning, "KM3006",
				fmt.Sprintf("変数 %q は%d行目ですでに宣言されています", name, prev))
		} else if kind == "USEVAR" {
			c.declaredVars[key] = line
		}
	}
}

// checkDimList は DIM の各配列宣言を検査する。
// 例: DIM d(2), table_(10,20), names$(5)
func (c *context) checkDimList(line int, raw string) {
	for _, item := range splitTopLevelCommas(raw) {
		decl := strings.TrimSpace(item)
		if decl == "" {
			c.add(line, 1, Error, "KM3010", "DIM の配列宣言が空です")
			continue
		}

		open := strings.IndexByte(decl, '(')
		if open <= 0 || !strings.HasSuffix(strings.TrimSpace(decl), ")") {
			c.add(line, 1, Error, "KM3011",
				fmt.Sprintf("DIM の配列宣言 %q が不正です", decl))
			continue
		}

		name := strings.TrimSpace(decl[:open])
		dimensions := strings.TrimSpace(decl[open+1 : len(strings.TrimSpace(decl))-1])
		if !longVarRE.MatchString(name) {
			c.add(line, 1, Error, "KM3002",
				fmt.Sprintf("変数名 %q が不正です", name))
			continue
		}

		base := strings.TrimSuffix(strings.TrimSuffix(name, "$"), "#")
		key := strings.ToUpper(base)
		c.arrays[key] = line
		if len(base) > 1 {
			if _, declared := c.declaredVars[key]; !declared {
				c.add(line, 1, Error, "KM3007", fmt.Sprintf("2文字以上の変数 %q は使用前にUSEVARで宣言してください", name))
			}
		}
		if _, ok := reserved[key]; ok {
			c.add(line, 1, Error, "KM3003",
				fmt.Sprintf("%q は予約語のため変数名に使用できません", name))
		}
		if c.options.Target != "type-p" && c.options.Target != "type-pu" && len(base) > 6 {
			c.add(line, 1, Error, "KM3005",
				fmt.Sprintf("対象機種 %s では長文字変数名は6文字以内です", c.options.Target))
		}

		if dimensions == "" {
			c.add(line, 1, Error, "KM3012",
				fmt.Sprintf("配列 %q の次元指定が空です", name))
			continue
		}
		for _, dimension := range splitTopLevelCommas(dimensions) {
			expr := strings.TrimSpace(dimension)
			if expr == "" {
				c.add(line, 1, Error, "KM3013",
					fmt.Sprintf("配列 %q に空の次元があります", name))
				continue
			}
			if typ, err := parseExpression(expr); err != nil {
				c.addExpressionError(line, err)
			} else if typ != typeInteger {
				c.add(line, 1, Error, "KM5005", fmt.Sprintf("配列 %q の次元には整数型が必要です", name))
			}
		}
	}
}

var nonVariableWords = wordSet("TO STEP THEN UNTIL PRIVATE PUBLIC STOP AND OR XOR")

// checkLongVariableUsage は、2文字以上の変数が参照時点ですでにUSEVAR宣言済みか検査する。
func (c *context) checkLongVariableUsage(line int, statement string) {
	masked := maskStrings(statement)
	first := firstWordRE.FindStringIndex(strings.TrimSpace(masked))
	leading := len(masked) - len(strings.TrimLeftFunc(masked, unicode.IsSpace))
	seen := make(map[string]struct{})

	for _, match := range identifierRE.FindAllStringIndex(masked, -1) {
		name := strings.ToUpper(masked[match[0]:match[1]])
		base := strings.TrimSuffix(strings.TrimSuffix(name, "$"), "#")
		if len(base) <= 1 {
			continue
		}
		// 文頭の命令、言語の予約語、FOR等の構文語は変数ではない。
		if first != nil && match[0] == leading+first[0] {
			if _, command := knownStatements[name]; command {
				continue
			}
		}
		if _, keyword := reserved[base]; keyword {
			continue
		}
		if _, keyword := nonVariableWords[base]; keyword {
			continue
		}
		if _, className := c.usedClasses[base]; className {
			continue
		}
		// 16進リテラル、指数表記の一部、クラス名・メソッド名を除外する。
		if match[0] > 0 && (masked[match[0]-1] == '$' || masked[match[0]-1] >= '0' && masked[match[0]-1] <= '9' || masked[match[0]-1] == '.' || masked[match[0]-1] == ':') {
			continue
		}
		previous := previousIdentifier(masked[:match[0]])
		previous = strings.TrimSuffix(strings.TrimSuffix(previous, "$"), "#")
		if previous == "GOTO" || previous == "GOSUB" || previous == "RESTORE" || previous == "SOUND" || previous == "LABEL" || previous == "USECLASS" || previous == "NEW" {
			continue
		}
		if _, duplicate := seen[base]; duplicate {
			continue
		}
		seen[base] = struct{}{}
		if _, declared := c.declaredVars[base]; !declared {
			c.add(line, match[0]+1, Error, "KM3007", fmt.Sprintf("2文字以上の変数 %q は使用前にUSEVARで宣言してください", name))
		}
	}
}

func previousIdentifier(prefix string) string {
	matches := identifierRE.FindAllString(prefix, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.ToUpper(matches[len(matches)-1])
}

func (c *context) checkStatementTypes(line int, statement string) {
	s := strings.TrimSpace(statement)
	u := strings.ToUpper(s)

	if eq := assignmentOperator(s); eq >= 0 {
		leftText, rightText := strings.TrimSpace(s[:eq]), strings.TrimSpace(s[eq+1:])
		left, leftErr := parseExpression(leftText)
		if leftErr != nil {
			c.addExpressionError(line, leftErr)
			return
		}
		right, rightErr := parseExpression(rightText)
		if rightErr != nil {
			c.addExpressionError(line, rightErr)
			return
		}
		if left != right && !(left == typeReal && right == typeInteger) {
			c.add(line, eq+1, Error, "KM5002", fmt.Sprintf("%sの変数に%sの式は代入できません", left, right))
		}
		return
	}

	switch {
	case strings.HasPrefix(u, "IF "):
		if before, ok := beforeKeyword(s[3:], "THEN"); ok {
			c.checkConditionExpression(line, before)
			c.checkInlineIfStatements(line, s[3:])
		}
	case strings.HasPrefix(u, "ELSEIF "):
		if before, ok := beforeKeyword(s[7:], "THEN"); ok {
			c.checkConditionExpression(line, before)
		}
	case strings.HasPrefix(u, "WHILE "):
		c.checkConditionExpression(line, s[6:])
	case strings.HasPrefix(u, "DO WHILE "):
		c.checkConditionExpression(line, s[9:])
	case strings.HasPrefix(u, "DO UNTIL "):
		c.checkConditionExpression(line, s[9:])
	case strings.HasPrefix(u, "LOOP WHILE "):
		c.checkConditionExpression(line, s[11:])
	case strings.HasPrefix(u, "LOOP UNTIL "):
		c.checkConditionExpression(line, s[11:])
	case strings.HasPrefix(u, "FOR "):
		c.checkForExpressions(line, s[4:])
	default:
		c.checkCommandArguments(line, s)
	}
}

func (c *context) checkInlineIfStatements(line int, afterIF string) {
	thenPos := keywordPosition(afterIF, "THEN")
	if thenPos < 0 {
		return
	}
	tail := strings.TrimSpace(afterIF[thenPos+len("THEN"):])
	if tail == "" {
		return
	}
	if elsePos := keywordPosition(tail, "ELSE"); elsePos >= 0 {
		thenStatement := strings.TrimSpace(tail[:elsePos])
		elseStatement := strings.TrimSpace(tail[elsePos+len("ELSE"):])
		if thenStatement != "" {
			c.checkStatementTypes(line, thenStatement)
		}
		if elseStatement != "" {
			c.checkStatementTypes(line, elseStatement)
		}
		return
	}
	c.checkStatementTypes(line, tail)
}

func (c *context) checkConditionExpression(line int, raw string) {
	if _, err := parseExpression(strings.TrimSpace(raw)); err != nil {
		c.addExpressionError(line, err)
	}
}

func (c *context) checkForExpressions(line int, raw string) {
	eq := assignmentOperator(raw)
	if eq < 0 {
		return
	}
	variable := strings.TrimSpace(raw[:eq])
	if typeFromName(strings.ToUpper(variable)) != typeInteger {
		return
	} // KM3101が報告する。
	rest := raw[eq+1:]
	toPos := keywordPosition(rest, "TO")
	if toPos < 0 {
		return
	}
	expressions := []string{rest[:toPos]}
	afterTo := rest[toPos+2:]
	if stepPos := keywordPosition(afterTo, "STEP"); stepPos >= 0 {
		expressions = append(expressions, afterTo[:stepPos], afterTo[stepPos+4:])
	} else {
		expressions = append(expressions, afterTo)
	}
	for _, expression := range expressions {
		typ, err := parseExpression(strings.TrimSpace(expression))
		if err != nil {
			c.addExpressionError(line, err)
			continue
		}
		if typ != typeInteger {
			c.add(line, 1, Error, "KM5006", "FORの初期値・終了値・STEPには整数型が必要です")
		}
	}
}

func (c *context) addExpressionError(line int, err *expressionError) {
	c.add(line, err.Pos+1, Error, err.Code, err.Message)
}

func assignmentOperator(s string) int {
	depth, inString := 0, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString {
				depth--
			}
		case '=':
			if !inString && depth == 0 && (i == 0 || s[i-1] != '<' && s[i-1] != '>' && s[i-1] != '!') {
				left := strings.TrimSpace(s[:i])
				if firstWordRE.MatchString(left) && !strings.ContainsAny(left, " +*/%<>&|") {
					return i
				}
			}
		}
	}
	return -1
}

func beforeKeyword(s, keyword string) (string, bool) {
	pos := keywordPosition(s, keyword)
	if pos < 0 {
		return "", false
	}
	return s[:pos], true
}

func keywordPosition(s, keyword string) int {
	u := strings.ToUpper(s)
	depth, inString := 0, false
	for i := 0; i+len(keyword) <= len(s); i++ {
		switch s[i] {
		case '"':
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString {
				depth--
			}
		}
		if !inString && depth == 0 && u[i:i+len(keyword)] == keyword &&
			(i == 0 || unicode.IsSpace(rune(s[i-1]))) &&
			(i+len(keyword) == len(s) || unicode.IsSpace(rune(s[i+len(keyword)]))) {
			return i
		}
	}
	return -1
}

// splitTopLevelCommas は括弧内のカンマを保持したまま、最上位のカンマだけで分割する。
func splitTopLevelCommas(s string) []string {
	var parts []string
	start, depth := 0, 0
	inString := false
	for i, r := range s {
		switch r {
		case '"':
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString && depth > 0 {
				depth--
			}
		case ',':
			if !inString && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func (c *context) checkBlocks(line int, s string) {
	u := strings.ToUpper(strings.TrimSpace(s))

	switch {
	case strings.HasPrefix(u, "FOR "):
		c.blocks = append(c.blocks, blockEntry{Kind: "FOR", Line: line})

	case nextRE.MatchString(s):
		c.closeBlock(line, "FOR", "NEXT")

	case strings.HasPrefix(u, "WHILE "):
		c.blocks = append(c.blocks, blockEntry{Kind: "WHILE", Line: line})

	case u == "WEND":
		c.closeBlock(line, "WHILE", "WEND")

	case u == "DO" || strings.HasPrefix(u, "DO WHILE ") || strings.HasPrefix(u, "DO UNTIL "):
		c.blocks = append(c.blocks, blockEntry{Kind: "DO", Line: line})

	case u == "LOOP" || strings.HasPrefix(u, "LOOP WHILE ") || strings.HasPrefix(u, "LOOP UNTIL "):
		c.closeBlock(line, "DO", "LOOP")

	case isMultilineIfStart(u):
		c.blocks = append(c.blocks, blockEntry{Kind: "IF", Line: line})

	case strings.HasPrefix(u, "ELSEIF ") || u == "ELSE":
		if !c.hasOpenTop("IF") {
			c.add(line, 1, Error, "KM1002", "ELSE/ELSEIF に対応する複数行IFがありません")
		}

	case u == "ENDIF":
		c.closeBlock(line, "IF", "ENDIF")
	}
}

func (c *context) checkForVariable(line int, s string) {
	m := forRE.FindStringSubmatch(s)
	if m == nil {
		return
	}
	if m[2] == "#" || m[2] == "$" {
		c.add(line, 1, Error, "KM3101", "FORの制御変数には整数型変数を使用してください")
	}
}

// checkKnownNames は未知の命令と、式中の未知の関数呼び出しを検出する。
func (c *context) checkKnownNames(line int, statement string) {
	masked := maskStrings(statement)
	trimmed := strings.TrimSpace(masked)
	if trimmed == "" {
		return
	}

	if m := firstWordRE.FindStringSubmatchIndex(trimmed); m != nil {
		name := strings.ToUpper(trimmed[m[2]:m[3]])
		rest := strings.TrimSpace(trimmed[m[1]:])
		_, known := knownStatements[name]
		if !known && !looksLikeAssignment(rest) && !strings.HasPrefix(rest, ".") && !strings.HasPrefix(rest, "::") {
			col := strings.Index(masked, trimmed[m[2]:m[3]]) + 1
			c.add(line, col, Error, "KM4001", fmt.Sprintf("命令 %q はKM-BASICに実装されていません", name))
		}
	}

	for _, m := range callRE.FindAllStringSubmatchIndex(masked, -1) {
		name := strings.ToUpper(masked[m[2]:m[3]])
		if _, operator := nonVariableWords[name]; operator {
			continue
		}
		if _, ok := knownFunctions[name]; ok {
			continue
		}
		if (m[0] > 0 && masked[m[0]-1] == '.') || (m[0] >= 2 && masked[m[0]-2:m[0]] == "::") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, "$"), "#")
		if len(base) == 1 {
			continue // クラスのフィールドから受け取った配列など、DIM以外の配列参照。
		}
		if _, ok := c.arrays[base]; ok {
			continue
		}
		if c.declaredVars[base] != 0 {
			continue
		}
		c.add(line, m[2]+1, Error, "KM4002", fmt.Sprintf("関数 %q はKM-BASICに実装されていません", name))
	}
}

func looksLikeAssignment(rest string) bool {
	if strings.HasPrefix(rest, "=") {
		return true
	}
	if !strings.HasPrefix(rest, "(") {
		return false
	}
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.HasPrefix(strings.TrimSpace(rest[i+1:]), "=")
			}
		}
	}
	return false
}

func maskStrings(s string) string {
	b := []byte(s)
	inString := false
	for i := 0; i < len(b); i++ {
		if b[i] == '"' {
			if inString && i+1 < len(b) && b[i+1] == '"' {
				b[i], b[i+1] = ' ', ' '
				i++
				continue
			}
			inString = !inString
			b[i] = ' '
		} else if inString {
			b[i] = ' '
		}
	}
	return string(b)
}

func (c *context) closeBlock(line int, want, closer string) {
	if len(c.blocks) == 0 {
		c.add(line, 1, Error, "KM1003",
			fmt.Sprintf("%s に対応する開始命令がありません", closer))
		return
	}
	top := c.blocks[len(c.blocks)-1]
	if top.Kind != want {
		c.add(line, 1, Error, "KM1004",
			fmt.Sprintf("%s の前に%d行目の %s を閉じる必要があります", closer, top.Line, top.Kind))
		return
	}
	c.blocks = c.blocks[:len(c.blocks)-1]
}

func (c *context) hasOpenTop(kind string) bool {
	return len(c.blocks) > 0 && c.blocks[len(c.blocks)-1].Kind == kind
}

func (c *context) add(line, col int, sev Severity, code, message string) {
	c.result.Diagnostics = append(c.result.Diagnostics, Diagnostic{
		Line: line, Column: col, Severity: sev, Code: code, Message: message,
	})
}

func splitStatements(line string) []string {
	var out []string
	start := 0
	inString := false

	for i := 0; i < len(line); i++ {
		if !inString && isREMAtStatementStart(line, start, i) {
			out = append(out, line[start:])
			return out
		}
		switch line[i] {
		case '"':
			// "" は文字列中の引用符として扱う。
			if inString && i+1 < len(line) && line[i+1] == '"' {
				i++
				continue
			}
			inString = !inString
		case ':':
			if !inString {
				if i+1 < len(line) && line[i+1] == ':' {
					i++
					continue
				}
				out = append(out, line[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, line[start:])
	return out
}

// remCommentStart は、文頭（行頭またはコロン直後）のREMの位置を返す。
func remCommentStart(line string) int {
	start, inString := 0, false
	for i := 0; i < len(line); i++ {
		if !inString && isREMAtStatementStart(line, start, i) {
			return i
		}
		switch line[i] {
		case '"':
			if inString && i+1 < len(line) && line[i+1] == '"' {
				i++
				continue
			}
			inString = !inString
		case ':':
			if !inString && i+1 < len(line) && line[i+1] == ':' {
				i++
			} else if !inString {
				start = i + 1
			}
		}
	}
	return -1
}

func isREMAtStatementStart(line string, statementStart, pos int) bool {
	if pos < statementStart || strings.TrimSpace(line[statementStart:pos]) != "" || pos+3 > len(line) || !strings.EqualFold(line[pos:pos+3], "REM") {
		return false
	}
	return pos+3 == len(line) || unicode.IsSpace(rune(line[pos+3]))
}

func apostropheColumn(line string) int {
	inString := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			if inString && i+1 < len(line) && line[i+1] == '"' {
				i++
				continue
			}
			inString = !inString
		case '\'':
			if !inString {
				return i + 1
			}
		}
	}
	return 0
}

func hasUnclosedString(line string) bool {
	inString := false
	for i := 0; i < len(line); i++ {
		if line[i] != '"' {
			continue
		}
		if inString && i+1 < len(line) && line[i+1] == '"' {
			i++
			continue
		}
		inString = !inString
	}
	return inString
}

func firstQuoteColumn(line string) int {
	i := strings.IndexByte(line, '"')
	if i < 0 {
		return 1
	}
	return i + 1
}

func isMultilineIfStart(u string) bool {
	if !strings.HasPrefix(u, "IF ") {
		return false
	}
	// THENの後ろに命令がなければ複数行IF。
	idx := strings.LastIndex(u, " THEN")
	if idx < 0 {
		return false
	}
	return strings.TrimSpace(u[idx+len(" THEN"):]) == ""
}

// 現段階では未使用だが、今後の数値リテラル検査用に保持する。
func parseIntegerLiteral(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToUpper(s), "0X") {
		return strconv.ParseInt(s[2:], 16, 32)
	}
	if strings.HasPrefix(s, "$") {
		return strconv.ParseInt(s[1:], 16, 32)
	}
	return strconv.ParseInt(s, 10, 32)
}
