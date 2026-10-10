/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package markdown

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type sourceLine struct {
	start   int
	end     int
	fullEnd int
	text    string
}

type reference struct {
	destination string
	title       string
}

type parser struct {
	source     string
	lines      []sourceLine
	cfg        config
	rules      []registeredRule
	references map[string]reference
	warnings   []Diagnostic
}

func normalizeSource(source string) (string, error) {
	if !utf8.ValidString(source) {
		return "", ErrInvalidUTF8
	}
	if !strings.ContainsAny(source, "\r\x00") {
		return source, nil
	}

	var builder strings.Builder
	builder.Grow(len(source))
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case 0:
			builder.WriteRune('\uFFFD')
		case '\r':
			if i+1 < len(source) && source[i+1] == '\n' {
				i++
			}
			builder.WriteByte('\n')
		default:
			builder.WriteByte(source[i])
		}
	}

	return builder.String(), nil
}

func splitSourceLines(source string) []sourceLine {
	if source == "" {
		return []sourceLine{}
	}

	lines := make([]sourceLine, 0, strings.Count(source, "\n")+1)
	for start := 0; start < len(source); {
		end := strings.IndexByte(source[start:], '\n')
		if end < 0 {
			end = len(source)
		} else {
			end += start
		}

		fullEnd := end
		if fullEnd < len(source) && source[fullEnd] == '\n' {
			fullEnd++
		}
		lines = append(lines, sourceLine{
			start:   start,
			end:     end,
			fullEnd: fullEnd,
			text:    source[start:end],
		})
		start = fullEnd
	}

	return lines
}

func newParser(source string, cfg config, rules []registeredRule) *parser {
	p := &parser{
		source:     source,
		lines:      splitSourceLines(source),
		cfg:        cfg,
		rules:      rules,
		references: map[string]reference{},
		warnings:   []Diagnostic{},
	}
	p.collectReferences()
	return p
}

func (p *parser) parse() (TokenizationResult, error) {
	tokens, err := p.parseBlockRange(0, len(p.source), 0)
	if err != nil {
		return p.handleParseError(err)
	}

	root := makeToken(
		TokenDocument,
		"",
		map[string]string{},
		tokens,
		Span{Start: 0, End: len(p.source)},
	)
	if err := p.checkTokenLimit([]Token{root}); err != nil {
		return p.handleParseError(err)
	}

	return TokenizationResult{
		Tokens:   []Token{root},
		Warnings: append([]Diagnostic{}, p.warnings...),
	}, nil
}

func (p *parser) handleParseError(err error) (TokenizationResult, error) {
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		parseErr = &ParseError{
			Err:    err,
			Phase:  RulePhaseBlock,
			RuleID: TokenDocument,
			Span:   Span{Start: 0, End: len(p.source)},
		}
	}
	if !isBestEffort(p.cfg) {
		return TokenizationResult{}, parseErr
	}

	warnings := append([]Diagnostic{}, p.warnings...)
	warnings = append(warnings, wrapDiagnostic(parseErr, parseErr.Phase, parseErr.RuleID, parseErr.Span))
	fallback := makeToken(
		TokenDocument,
		"",
		map[string]string{},
		[]Token{makeToken(TokenText, p.source, map[string]string{}, []Token{}, Span{Start: 0, End: len(p.source)})},
		Span{Start: 0, End: len(p.source)},
	)
	return TokenizationResult{Tokens: []Token{fallback}, Warnings: warnings}, nil
}

func (p *parser) parseBlockRange(start, end, depth int) ([]Token, error) {
	if depth > p.cfg.maxNestingDepth {
		return nil, fmt.Errorf("%w: nesting depth %d", ErrLimitExceeded, depth)
	}
	if start < 0 || end < start || end > len(p.source) {
		return nil, fmt.Errorf("markdown: invalid block range %d:%d", start, end)
	}
	if start == end {
		return []Token{}, nil
	}

	first := p.firstLineAtOrAfter(start)
	if first >= len(p.lines) {
		return []Token{}, nil
	}
	last := p.firstLineAtOrAfter(end)
	if last > len(p.lines) {
		last = len(p.lines)
	}

	return p.parseBlocks(first, last, depth)
}

func (p *parser) parseBlocks(first, last, depth int) ([]Token, error) {
	tokens := make([]Token, 0, last-first)
	for i := first; i < last; {
		if isBlankLine(p.lines[i].text) {
			i++
			continue
		}
		if _, _, ok := parseReferenceDefinition(p.lines[i].text); ok {
			i++
			continue
		}

		custom, next, matched, err := p.applyBlockRules(i, last)
		if err != nil {
			return nil, err
		}
		if matched {
			tokens = append(tokens, custom)
			i = next
			continue
		}

		if token, next, ok := p.parseFencedCode(i, last); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := p.parseIndentedCode(i, last); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := p.parseATXHeading(i, last, depth); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok, err := p.parseSetextHeading(i, last, depth); err != nil {
			return nil, err
		} else if ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if isThematicBreak(p.lines[i].text) {
			tokens = append(tokens, makeToken(
				TokenThematicRule,
				"",
				map[string]string{},
				[]Token{},
				Span{Start: p.lines[i].start, End: p.lines[i].end},
			))
			i++
			continue
		}
		if token, next, ok, err := p.parseBlockquote(i, last, depth); err != nil {
			return nil, err
		} else if ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok, err := p.parseList(i, last, depth); err != nil {
			return nil, err
		} else if ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := p.parseTable(i, last, depth); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := p.parseHTMLBlock(i, last); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}

		token, next, err := p.parseParagraph(i, last, depth)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
		i = next
	}

	return tokens, nil
}

func (p *parser) applyBlockRules(index, last int) (Token, int, bool, error) {
	line := p.lines[index]
	for _, registered := range p.rules {
		if registered.rule.Phase != RulePhaseBlock {
			continue
		}

		ctx := &ParseContext{
			source: p.source,
			parser: p,
			phase:  RulePhaseBlock,
		}
		token, consumed, matched, err := registered.rule.Parse(ctx, Cursor{
			source: p.source,
			offset: line.start,
			end:    len(p.source),
		})
		if err != nil {
			wrapped := &ParseError{
				Err:    fmt.Errorf("rule %q: %w", registered.rule.ID, err),
				Phase:  RulePhaseBlock,
				RuleID: registered.rule.ID,
				Span:   Span{Start: line.start, End: line.end},
			}
			if isBestEffort(p.cfg) {
				p.warnings = append(p.warnings, wrapDiagnostic(wrapped, RulePhaseBlock, registered.rule.ID, wrapped.Span))
				continue
			}
			return Token{}, 0, false, wrapped
		}
		if !matched {
			continue
		}
		if consumed <= 0 || line.start+consumed > len(p.source) || !utf8.ValidString(p.source[line.start:line.start+consumed]) {
			wrapped := &ParseError{
				Err:    ErrRuleProgress,
				Phase:  RulePhaseBlock,
				RuleID: registered.rule.ID,
				Span:   Span{Start: line.start, End: line.end},
			}
			if isBestEffort(p.cfg) {
				p.warnings = append(p.warnings, wrapDiagnostic(wrapped, RulePhaseBlock, registered.rule.ID, wrapped.Span))
				continue
			}
			return Token{}, 0, false, wrapped
		}

		consumedEnd := line.start + consumed
		next := p.lineIndexAtStart(consumedEnd)
		if next < 0 || next > last {
			wrapped := &ParseError{
				Err:    fmt.Errorf("%w: block rule must consume complete lines", ErrRuleProgress),
				Phase:  RulePhaseBlock,
				RuleID: registered.rule.ID,
				Span:   Span{Start: line.start, End: consumedEnd},
			}
			if isBestEffort(p.cfg) {
				p.warnings = append(p.warnings, wrapDiagnostic(wrapped, RulePhaseBlock, registered.rule.ID, wrapped.Span))
				continue
			}
			return Token{}, 0, false, wrapped
		}
		token.ID = registered.rule.ID
		token.Attributes = normalizeAttributes(token.Attributes)
		if token.Children == nil {
			token.Children = []Token{}
		}
		token.Span = Span{Start: line.start, End: consumedEnd}
		return token, next, true, nil
	}

	return Token{}, 0, false, nil
}

func (p *parser) applyInlineRules(offset, end int) (Token, int, bool, error) {
	for _, registered := range p.rules {
		if registered.rule.Phase != RulePhaseInline {
			continue
		}

		ctx := &ParseContext{
			source: p.source,
			parser: p,
			phase:  RulePhaseInline,
		}
		token, consumed, matched, err := registered.rule.Parse(ctx, Cursor{
			source: p.source,
			offset: offset,
			end:    end,
		})
		if err != nil {
			wrapped := &ParseError{
				Err:    fmt.Errorf("rule %q: %w", registered.rule.ID, err),
				Phase:  RulePhaseInline,
				RuleID: registered.rule.ID,
				Span:   Span{Start: offset, End: minInt(end, offset+1)},
			}
			if isBestEffort(p.cfg) {
				p.warnings = append(p.warnings, wrapDiagnostic(wrapped, RulePhaseInline, registered.rule.ID, wrapped.Span))
				continue
			}
			return Token{}, 0, false, wrapped
		}
		if !matched {
			continue
		}
		if consumed <= 0 || offset+consumed > end || !utf8.ValidString(p.source[offset:offset+consumed]) {
			wrapped := &ParseError{
				Err:    ErrRuleProgress,
				Phase:  RulePhaseInline,
				RuleID: registered.rule.ID,
				Span:   Span{Start: offset, End: minInt(end, offset+1)},
			}
			if isBestEffort(p.cfg) {
				p.warnings = append(p.warnings, wrapDiagnostic(wrapped, RulePhaseInline, registered.rule.ID, wrapped.Span))
				continue
			}
			return Token{}, 0, false, wrapped
		}
		token.ID = registered.rule.ID
		token.Attributes = normalizeAttributes(token.Attributes)
		if token.Children == nil {
			token.Children = []Token{}
		}
		token.Span = Span{Start: offset, End: offset + consumed}
		return token, consumed, true, nil
	}

	return Token{}, 0, false, nil
}

func (p *parser) parseFencedCode(index, last int) (Token, int, bool) {
	line := p.lines[index]
	trimmed, indent := trimIndent(line.text)
	if indent > 3 || len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return Token{}, index, false
	}

	marker := trimmed[0]
	markerLen := 0
	for markerLen < len(trimmed) && trimmed[markerLen] == marker {
		markerLen++
	}
	if markerLen < 3 || (marker == '`' && strings.Contains(trimmed[markerLen:], "`")) {
		return Token{}, index, false
	}

	info := strings.TrimSpace(trimmed[markerLen:])
	contentStart := line.fullEnd
	contentEnd := contentStart
	next := index + 1
	for ; next < last; next++ {
		candidate, candidateIndent := trimIndent(p.lines[next].text)
		if candidateIndent <= 3 && hasFence(candidate, marker, markerLen) {
			contentEnd = p.lines[next].start
			break
		}
		contentEnd = p.lines[next].fullEnd
	}
	if contentEnd < contentStart {
		contentEnd = contentStart
	}
	content := p.source[contentStart:contentEnd]
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	attributes := map[string]string{
		AttributeLanguage: "",
	}
	if fields := strings.Fields(info); len(fields) > 0 {
		attributes[AttributeLanguage] = fields[0]
	}
	end := line.end
	if next < last {
		end = p.lines[next].end
		next++
	} else if last > index {
		end = p.lines[last-1].end
	}

	return makeToken(
		TokenCodeBlock,
		content,
		attributes,
		[]Token{},
		Span{Start: line.start, End: end},
	), next, true
}

func hasFence(line string, marker byte, minLength int) bool {
	if len(line) < minLength || line[0] != marker {
		return false
	}

	length := 0
	for length < len(line) && line[length] == marker {
		length++
	}
	return length >= minLength && strings.TrimSpace(line[length:]) == ""
}

func (p *parser) parseIndentedCode(index, last int) (Token, int, bool) {
	if !hasFourSpaceIndent(p.lines[index].text) {
		return Token{}, index, false
	}

	parts := make([]string, 0, 4)
	next := index
	for next < last {
		line := p.lines[next].text
		if hasFourSpaceIndent(line) {
			parts = append(parts, line[4:])
			next++
			continue
		}
		if isBlankLine(line) {
			parts = append(parts, "")
			next++
			continue
		}
		break
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	literal := strings.Join(parts, "\n")
	if literal != "" {
		literal += "\n"
	}
	return makeToken(
		TokenCodeBlock,
		literal,
		map[string]string{AttributeLanguage: ""},
		[]Token{},
		Span{Start: p.lines[index].start, End: p.lines[next-1].end},
	), next, true
}

func hasFourSpaceIndent(line string) bool {
	return len(line) >= 4 && line[0] == ' ' && line[1] == ' ' && line[2] == ' ' && line[3] == ' '
}

func (p *parser) parseATXHeading(index, _ int, depth int) (Token, int, bool) {
	line := p.lines[index]
	trimmed, indent := trimIndent(line.text)
	if indent > 3 || len(trimmed) == 0 || trimmed[0] != '#' {
		return Token{}, index, false
	}

	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 6 || (level < len(trimmed) && trimmed[level] != ' ' && trimmed[level] != '\t') {
		return Token{}, index, false
	}

	content := strings.TrimSpace(trimmed[level:])
	if strings.HasSuffix(content, "#") {
		trimmedContent := strings.TrimRight(content, "#")
		if strings.HasSuffix(trimmedContent, " ") || strings.HasSuffix(trimmedContent, "\t") {
			content = strings.TrimSpace(trimmedContent)
		}
	}
	contentStart := line.start + indent + level
	for contentStart < line.end && (p.source[contentStart] == ' ' || p.source[contentStart] == '\t') {
		contentStart++
	}
	contentEnd := contentStart + len(content)
	if contentEnd > line.end {
		contentEnd = line.end
	}
	children, err := p.parseInlineRange(contentStart, contentEnd, depth+1)
	if err != nil {
		return Token{}, index, false
	}

	return makeToken(
		TokenHeading,
		"",
		map[string]string{AttributeLevel: fmt.Sprintf("%d", level)},
		children,
		Span{Start: line.start, End: line.end},
	), index + 1, true
}

func (p *parser) parseSetextHeading(index, last, depth int) (Token, int, bool, error) {
	if index+1 >= last || isBlankLine(p.lines[index].text) {
		return Token{}, index, false, nil
	}
	underline := strings.TrimSpace(p.lines[index+1].text)
	level := 0
	if isRepeated(underline, '=') {
		level = 1
	}
	if isRepeated(underline, '-') {
		level = 2
	}
	if level == 0 {
		return Token{}, index, false, nil
	}

	line := p.lines[index]
	children, err := p.parseInlineRange(line.start, line.end, depth+1)
	if err != nil {
		return Token{}, index, false, err
	}

	return makeToken(
		TokenHeading,
		"",
		map[string]string{AttributeLevel: fmt.Sprintf("%d", level)},
		children,
		Span{Start: line.start, End: p.lines[index+1].end},
	), index + 2, true, nil
}

func isRepeated(value string, marker byte) bool {
	if len(value) < 1 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] != marker {
			return false
		}
	}
	return true
}

func (p *parser) parseBlockquote(index, last, depth int) (Token, int, bool, error) {
	if !startsBlockquote(p.lines[index].text) {
		return Token{}, index, false, nil
	}

	parts := make([]string, 0, 4)
	next := index
	for next < last {
		line := p.lines[next].text
		if startsBlockquote(line) {
			parts = append(parts, stripBlockquote(line))
			next++
			continue
		}
		if isBlankLine(line) && next+1 < last && startsBlockquote(p.lines[next+1].text) {
			parts = append(parts, "")
			next++
			continue
		}
		break
	}

	children, warnings, err := p.parseSyntheticBlocks(strings.Join(parts, "\n"), depth+1)
	if err != nil {
		return Token{}, index, false, err
	}
	p.warnings = append(p.warnings, warnings...)
	return makeToken(
		TokenBlockquote,
		"",
		map[string]string{},
		children,
		Span{Start: p.lines[index].start, End: p.lines[next-1].end},
	), next, true, nil
}

func startsBlockquote(line string) bool {
	trimmed, indent := trimIndent(line)
	return indent <= 3 && strings.HasPrefix(trimmed, ">")
}

func stripBlockquote(line string) string {
	trimmed, _ := trimIndent(line)
	trimmed = strings.TrimPrefix(trimmed, ">")
	return strings.TrimPrefix(trimmed, " ")
}

func (p *parser) parseList(index, last, depth int) (Token, int, bool, error) {
	ordered, _, start, _, ok := listMarker(p.lines[index].text)
	if !ok {
		return Token{}, index, false, nil
	}
	_, listIndent := trimIndent(p.lines[index].text)

	items := make([]Token, 0, 4)
	next := index
	tight := true
	for next < last {
		itemOrdered, markerWidth, _, content, itemOK := listMarker(p.lines[next].text)
		if !itemOK || itemOrdered != ordered {
			break
		}
		_, itemIndent := trimIndent(p.lines[next].text)
		if itemIndent != listIndent {
			break
		}

		itemLine := p.lines[next]
		parts := []string{content}
		itemEnd := itemLine.end
		next++
		itemHasBlank := false
		for next < last {
			if isBlankLine(p.lines[next].text) {
				blankIndex := next
				next++
				if next >= last {
					break
				}
				nextOrdered, _, _, _, nextItem := listMarker(p.lines[next].text)
				_, nextIndent := trimIndent(p.lines[next].text)
				if nextItem && nextOrdered == ordered && nextIndent == listIndent {
					itemHasBlank = true
					tight = false
					parts = append(parts, "")
					itemEnd = p.lines[blankIndex].end
					break
				}
				if nextIndent == 0 {
					next = blankIndex
					break
				}
				itemHasBlank = true
				tight = false
				parts = append(parts, "")
				itemEnd = p.lines[blankIndex].end
				continue
			}
			if _, _, _, _, nextItem := listMarker(p.lines[next].text); nextItem {
				_, nextIndent := trimIndent(p.lines[next].text)
				if nextIndent > listIndent {
					line := p.lines[next]
					remove := minInt(len(line.text)-len(strings.TrimLeft(line.text, " ")), markerWidth)
					parts = append(parts, line.text[remove:])
					itemEnd = line.end
					next++
					continue
				}
				break
			}
			line := p.lines[next]
			_, indent := trimIndent(line.text)
			if indent == 0 && isBlockStarter(line.text) {
				break
			}
			remove := minInt(len(line.text)-len(strings.TrimLeft(line.text, " ")), markerWidth)
			parts = append(parts, line.text[remove:])
			itemEnd = line.end
			next++
		}

		itemSource := strings.Join(parts, "\n")
		checked, itemSource := extractTaskMarker(itemSource)
		children, warnings, err := p.parseSyntheticBlocks(itemSource, depth+1)
		if err != nil {
			return Token{}, index, false, err
		}
		p.warnings = append(p.warnings, warnings...)
		if checked != "" {
			children = append([]Token{makeToken(
				TokenTaskCheckbox,
				"",
				map[string]string{AttributeChecked: checked},
				[]Token{},
				Span{},
			)}, children...)
		}
		if itemHasBlank {
			tight = false
		}
		items = append(items, makeToken(
			TokenListItem,
			"",
			map[string]string{},
			children,
			Span{Start: itemLine.start, End: itemEnd},
		))
	}
	if tight {
		for i := range items {
			flat := make([]Token, 0, len(items[i].Children))
			for _, child := range items[i].Children {
				if child.ID == TokenParagraph {
					flat = append(flat, child.Children...)
					continue
				}
				flat = append(flat, child)
			}
			items[i].Children = flat
		}
	}

	attributes := map[string]string{
		AttributeOrdered: boolAttribute(ordered),
		AttributeStart:   fmt.Sprintf("%d", start),
		AttributeTight:   boolAttribute(tight),
	}
	return makeToken(
		TokenList,
		"",
		attributes,
		items,
		Span{Start: p.lines[index].start, End: p.lines[next-1].end},
	), next, true, nil
}

func listMarker(line string) (ordered bool, markerWidth, start int, content string, ok bool) {
	trimmed, indent := trimIndent(line)
	if indent > 3 || trimmed == "" {
		return false, 0, 0, "", false
	}
	if trimmed[0] == '-' || trimmed[0] == '+' || trimmed[0] == '*' {
		if len(trimmed) == 1 || (trimmed[1] != ' ' && trimmed[1] != '\t') {
			return false, 0, 0, "", false
		}
		content = strings.TrimLeft(trimmed[1:], " \t")
		return false, 2, 1, content, true
	}

	digitEnd := 0
	for digitEnd < len(trimmed) && digitEnd < 9 && trimmed[digitEnd] >= '0' && trimmed[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == 0 || digitEnd >= len(trimmed) || (trimmed[digitEnd] != '.' && trimmed[digitEnd] != ')') {
		return false, 0, 0, "", false
	}
	if digitEnd+1 >= len(trimmed) || (trimmed[digitEnd+1] != ' ' && trimmed[digitEnd+1] != '\t') {
		return false, 0, 0, "", false
	}
	start = 0
	for i := 0; i < digitEnd; i++ {
		start = start*10 + int(trimmed[i]-'0')
	}
	content = strings.TrimLeft(trimmed[digitEnd+1:], " \t")
	return true, digitEnd + 2, start, content, true
}

func extractTaskMarker(source string) (string, string) {
	if len(source) < 4 || source[0] != '[' || source[2] != ']' || !isTaskMarkerWhitespace(source[3]) {
		return "", source
	}
	if isTaskMarkerWhitespace(source[1]) {
		return "false", source[4:]
	}
	if source[1] == 'x' || source[1] == 'X' {
		return "true", source[4:]
	}
	return "", source
}

func isTaskMarkerWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\v' || value == '\f'
}

func (p *parser) parseTable(index, last, depth int) (Token, int, bool) {
	if index+1 >= last || !strings.Contains(p.lines[index].text, "|") {
		return Token{}, index, false
	}
	alignments, ok := parseTableDelimiter(p.lines[index+1].text)
	if !ok {
		return Token{}, index, false
	}

	headCells := splitTableCells(p.lines[index].text)
	if len(headCells) == 0 || len(alignments) != len(headCells) {
		return Token{}, index, false
	}
	head, err := p.makeTableRow(headCells, alignments, true, depth+1, p.lines[index].start)
	if err != nil {
		return Token{}, index, false
	}
	bodyRows := make([]Token, 0, 4)
	next := index + 2
	for next < last && !isBlankLine(p.lines[next].text) && strings.Contains(p.lines[next].text, "|") {
		cells := splitTableCells(p.lines[next].text)
		if len(cells) < len(headCells) {
			for len(cells) < len(headCells) {
				cells = append(cells, "")
			}
		}
		if len(cells) > len(headCells) {
			cells = cells[:len(headCells)]
		}
		row, rowErr := p.makeTableRow(cells, alignments, false, depth+1, p.lines[next].start)
		if rowErr != nil {
			return Token{}, index, false
		}
		bodyRows = append(bodyRows, row)
		next++
	}

	headToken := makeToken(TokenTableHead, "", map[string]string{}, []Token{head}, Span{})
	bodyToken := makeToken(TokenTableBody, "", map[string]string{}, bodyRows, Span{})
	children := []Token{headToken}
	if len(bodyRows) > 0 {
		children = append(children, bodyToken)
	}
	return makeToken(
		TokenTable,
		"",
		map[string]string{},
		children,
		Span{Start: p.lines[index].start, End: p.lines[next-1].end},
	), next, true
}

func parseTableDelimiter(line string) ([]string, bool) {
	cells := splitTableCells(line)
	if len(cells) == 0 {
		return []string{}, false
	}
	alignments := make([]string, 0, len(cells))
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if len(cell) < 3 {
			return []string{}, false
		}
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		body := strings.Trim(cell, ":")
		if len(body) < 3 || !isRepeated(body, '-') {
			return []string{}, false
		}
		switch {
		case left && right:
			alignments = append(alignments, "center")
		case left:
			alignments = append(alignments, "left")
		case right:
			alignments = append(alignments, "right")
		default:
			alignments = append(alignments, "")
		}
	}
	return alignments, true
}

func splitTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	if strings.HasSuffix(line, "|") && !isEscaped(line, len(line)-1) {
		line = line[:len(line)-1]
	}

	cells := make([]string, 0, 4)
	start := 0
	escaped := false
	for i := 0; i < len(line); i++ {
		if escaped {
			escaped = false
			continue
		}
		if line[i] == '\\' {
			escaped = true
			continue
		}
		if line[i] == '|' {
			cells = append(cells, strings.TrimSpace(strings.ReplaceAll(line[start:i], "\\|", "|")))
			start = i + 1
		}
	}
	cells = append(cells, strings.TrimSpace(strings.ReplaceAll(line[start:], "\\|", "|")))
	return cells
}

func isEscaped(value string, index int) bool {
	count := 0
	for index > 0 && value[index-1] == '\\' {
		count++
		index--
	}
	return count%2 == 1
}

func (p *parser) makeTableRow(cells, alignments []string, header bool, depth, offset int) (Token, error) {
	children := make([]Token, 0, len(cells))
	for i, cell := range cells {
		cellParser := newParser(cell, p.cfg, p.rules)
		cellParser.references = p.references
		inline, err := cellParser.parseInlineRange(0, len(cell), depth)
		if err != nil {
			return Token{}, err
		}
		p.warnings = append(p.warnings, cellParser.warnings...)
		alignment := ""
		if i < len(alignments) {
			alignment = alignments[i]
		}
		attributes := map[string]string{AttributeAlignment: alignment}
		if header {
			attributes[AttributeHeader] = "true"
		}
		children = append(children, makeToken(
			TokenTableCell,
			"",
			attributes,
			inline,
			Span{Start: offset, End: offset + len(cell)},
		))
	}
	return makeToken(TokenTableRow, "", map[string]string{}, children, Span{}), nil
}

func (p *parser) parseHTMLBlock(index, last int) (Token, int, bool) {
	if !isHTMLStart(p.lines[index].text) {
		return Token{}, index, false
	}

	next := index + 1
	for next < last && !isBlankLine(p.lines[next].text) {
		next++
	}
	start := p.lines[index].start
	end := p.lines[next-1].end
	return makeToken(
		TokenHTMLBlock,
		p.source[start:end],
		map[string]string{},
		[]Token{},
		Span{Start: start, End: end},
	), next, true
}

func isHTMLStart(line string) bool {
	trimmed, indent := trimIndent(line)
	if indent > 3 || !strings.HasPrefix(trimmed, "<") {
		return false
	}
	if strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, "<?") || strings.HasPrefix(trimmed, "<![") || strings.HasPrefix(trimmed, "<!") {
		return true
	}
	end := strings.IndexByte(trimmed, '>')
	if end < 0 {
		return false
	}
	tag := trimmed[1:end]
	tag = strings.TrimPrefix(tag, "/")
	return looksLikeHTML(tag)
}

func (p *parser) parseParagraph(index, last, depth int) (Token, int, error) {
	start := p.lines[index].start
	next := index + 1
	for next < last {
		if isBlankLine(p.lines[next].text) {
			break
		}
		if isBlockStarter(p.lines[next].text) {
			break
		}
		if next+1 < last && strings.Contains(p.lines[next].text, "|") {
			if _, ok := parseTableDelimiter(p.lines[next+1].text); ok {
				break
			}
		}
		next++
	}
	end := p.lines[next-1].end
	children, err := p.parseInlineRange(start, end, depth+1)
	if err != nil {
		return Token{}, index, err
	}
	return makeToken(
		TokenParagraph,
		"",
		map[string]string{},
		children,
		Span{Start: start, End: end},
	), next, nil
}

func isBlockStarter(line string) bool {
	trimmed, indent := trimIndent(line)
	if indent > 3 || trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, ">") || isHTMLStart(line) || isThematicBreak(line) {
		return true
	}
	if trimmed[0] == '#' {
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		return level <= 6 && (level == len(trimmed) || trimmed[level] == ' ' || trimmed[level] == '\t')
	}
	if _, _, _, _, ok := listMarker(line); ok {
		return true
	}
	if len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
		marker := trimmed[0]
		length := 0
		for length < len(trimmed) && trimmed[length] == marker {
			length++
		}
		return length >= 3 && (marker != '`' || !strings.Contains(trimmed[length:], "`"))
	}
	return false
}

func isThematicBreak(line string) bool {
	trimmed, indent := trimIndent(line)
	if indent > 3 || trimmed == "" {
		return false
	}
	marker := trimmed[0]
	if marker != '*' && marker != '-' && marker != '_' {
		return false
	}
	count := 0
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == marker {
			count++
			continue
		}
		if trimmed[i] != ' ' && trimmed[i] != '\t' {
			return false
		}
	}
	return count >= 3
}

func trimIndent(line string) (string, int) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	return line[indent:], indent
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}

func (p *parser) collectReferences() {
	for _, line := range p.lines {
		label, ref, ok := parseReferenceDefinition(line.text)
		if ok {
			p.references[label] = ref
		}
	}
}

func parseReferenceDefinition(line string) (string, reference, bool) {
	trimmed, indent := trimIndent(line)
	if indent > 3 || !strings.HasPrefix(trimmed, "[") {
		return "", reference{}, false
	}
	closing := strings.Index(trimmed, "]:")
	if closing <= 1 {
		return "", reference{}, false
	}
	label := normalizeReferenceLabel(trimmed[1:closing])
	rest := strings.TrimSpace(trimmed[closing+2:])
	if label == "" || rest == "" {
		return "", reference{}, false
	}
	destination := ""
	if strings.HasPrefix(rest, "<") {
		end := strings.IndexByte(rest, '>')
		if end < 0 {
			return "", reference{}, false
		}
		destination = rest[1:end]
		rest = strings.TrimSpace(rest[end+1:])
	} else {
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return "", reference{}, false
		}
		destination = fields[0]
		rest = strings.TrimSpace(rest[len(fields[0]):])
	}
	title := parseLinkTitle(rest)
	return label, reference{destination: destination, title: title}, true
}

func normalizeReferenceLabel(label string) string {
	return strings.Join(strings.Fields(strings.ToLower(label)), " ")
}

func parseLinkTitle(value string) string {
	if len(value) < 2 {
		return ""
	}
	if (value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '(' && value[len(value)-1] == ')') {
		return value[1 : len(value)-1]
	}
	return ""
}

func (p *parser) parseSyntheticBlocks(source string, depth int) ([]Token, []Diagnostic, error) {
	child := newParser(source, p.cfg, p.rules)
	child.references = p.references
	tokens, err := child.parseBlockRange(0, len(source), depth)
	return tokens, child.warnings, err
}

func (p *parser) firstLineAtOrAfter(offset int) int {
	if offset <= 0 {
		return 0
	}
	for i, line := range p.lines {
		if line.start >= offset {
			return i
		}
	}
	return len(p.lines)
}

func (p *parser) lineIndexAtStart(offset int) int {
	if offset == len(p.source) {
		return len(p.lines)
	}
	for i, line := range p.lines {
		if line.start == offset {
			return i
		}
	}
	return -1
}

func (p *parser) checkTokenLimit(tokens []Token) error {
	count := 0
	var visit func([]Token) bool
	visit = func(nodes []Token) bool {
		for _, token := range nodes {
			count++
			if count > p.cfg.maxTokens {
				return false
			}
			if !visit(token.Children) {
				return false
			}
		}
		return true
	}
	if !visit(tokens) {
		return fmt.Errorf("%w: token count %d", ErrLimitExceeded, count)
	}
	return nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
