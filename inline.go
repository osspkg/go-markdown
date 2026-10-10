/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package markdown

import (
	"fmt"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (p *parser) parseInlineRange(start, end, depth int) ([]Token, error) {
	if depth > p.cfg.maxNestingDepth {
		return nil, fmt.Errorf("%w: nesting depth %d", ErrLimitExceeded, depth)
	}
	if start < 0 || end < start || end > len(p.source) {
		return nil, fmt.Errorf("markdown: invalid inline range %d:%d", start, end)
	}

	tokens := make([]Token, 0, 4)
	textStart := start
	flushText := func(position int) {
		if position <= textStart {
			return
		}
		appendInlineText(&tokens, html.UnescapeString(p.source[textStart:position]), Span{
			Start: textStart,
			End:   position,
		})
	}

	for i := start; i < end; {
		if token, consumed, matched, err := p.applyInlineRules(i, end); err != nil {
			return nil, err
		} else if matched {
			flushText(i)
			tokens = append(tokens, token)
			i += consumed
			textStart = i
			continue
		}

		if p.source[i] == '\n' {
			flushText(i)
			spaces := trimTrailingTextSpaces(&tokens)
			breakToken := TokenSoftbreak
			if spaces >= 2 {
				breakToken = TokenHardbreak
			}
			tokens = append(tokens, makeToken(
				breakToken,
				"",
				map[string]string{},
				[]Token{},
				Span{Start: i, End: i + 1},
			))
			i++
			textStart = i
			continue
		}

		if p.source[i] == '\\' && i+1 < end {
			if p.source[i+1] == '\n' {
				flushText(i)
				tokens = append(tokens, makeToken(
					TokenHardbreak,
					"",
					map[string]string{},
					[]Token{},
					Span{Start: i, End: i + 2},
				))
				i += 2
				textStart = i
				continue
			}
			if isEscapable(p.source[i+1]) {
				flushText(i)
				appendInlineText(&tokens, string(p.source[i+1]), Span{Start: i, End: i + 2})
				i += 2
				textStart = i
				continue
			}
		}

		if p.source[i] == '`' {
			if token, next, ok := p.parseCodeSpan(i, end); ok {
				flushText(i)
				tokens = append(tokens, token)
				i = next
				textStart = i
				continue
			}
		}

		if p.source[i] == '!' || p.source[i] == '[' {
			if token, next, ok, err := p.parseLink(i, end, depth); err != nil {
				return nil, err
			} else if ok {
				flushText(i)
				tokens = append(tokens, token)
				i = next
				textStart = i
				continue
			}
		}

		if p.source[i] == '<' {
			if token, next, ok := p.parseAngleToken(i, end); ok {
				flushText(i)
				tokens = append(tokens, token)
				i = next
				textStart = i
				continue
			}
		}

		if token, next, ok, err := p.parseDelimited(i, end, depth); err != nil {
			return nil, err
		} else if ok {
			flushText(i)
			tokens = append(tokens, token)
			i = next
			textStart = i
			continue
		}

		if token, next, ok := p.parseAutolinkLiteral(i, end); ok {
			flushText(i)
			tokens = append(tokens, token)
			i = next
			textStart = i
			continue
		}

		_, size := utf8.DecodeRuneInString(p.source[i:end])
		if size == 0 {
			size = 1
		}
		i += size
	}
	flushText(end)
	return tokens, nil
}

func appendInlineText(tokens *[]Token, text string, span Span) {
	if text == "" {
		return
	}
	if len(*tokens) > 0 {
		last := &(*tokens)[len(*tokens)-1]
		if last.ID == TokenText && last.Span.End == span.Start {
			last.Literal += text
			last.Span.End = span.End
			return
		}
	}
	*tokens = append(*tokens, makeToken(TokenText, text, map[string]string{}, []Token{}, span))
}

func trimTrailingTextSpaces(tokens *[]Token) int {
	if len(*tokens) == 0 {
		return 0
	}
	last := &(*tokens)[len(*tokens)-1]
	if last.ID != TokenText {
		return 0
	}
	spaces := 0
	for spaces < len(last.Literal) {
		value := last.Literal[len(last.Literal)-1-spaces]
		if value != ' ' && value != '\t' {
			break
		}
		spaces++
	}
	if spaces > 0 {
		last.Literal = last.Literal[:len(last.Literal)-spaces]
		last.Span.End -= spaces
		if last.Literal == "" {
			*tokens = (*tokens)[:len(*tokens)-1]
		}
	}
	return spaces
}

func (p *parser) parseCodeSpan(start, end int) (Token, int, bool) {
	run := 0
	for start+run < end && p.source[start+run] == '`' {
		run++
	}
	closing := start + run
	for closing < end {
		if p.source[closing] != '`' {
			closing++
			continue
		}
		candidate := closing
		for candidate < end && p.source[candidate] == '`' {
			candidate++
		}
		if candidate-closing == run {
			literal := strings.ReplaceAll(p.source[start+run:closing], "\n", " ")
			if len(literal) >= 2 && literal[0] == ' ' && literal[len(literal)-1] == ' ' && strings.TrimSpace(literal) != "" {
				literal = literal[1 : len(literal)-1]
			}
			return makeToken(
				TokenCodeSpan,
				literal,
				map[string]string{},
				[]Token{},
				Span{Start: start, End: candidate},
			), candidate, true
		}
		closing = candidate
	}
	return Token{}, start, false
}

func (p *parser) parseLink(start, end, depth int) (Token, int, bool, error) {
	image := p.source[start] == '!'
	bracket := start
	if image {
		if start+1 >= end || p.source[start+1] != '[' {
			return Token{}, start, false, nil
		}
		bracket++
	}
	if p.source[bracket] != '[' {
		return Token{}, start, false, nil
	}
	closing := findClosingBracket(p.source, bracket, end)
	if closing < 0 {
		return Token{}, start, false, nil
	}

	destination := ""
	title := ""
	next := closing + 1
	if next < end && p.source[next] == '(' {
		var ok bool
		destination, title, next, ok = parseInlineLinkDestination(p.source, next, end)
		if !ok {
			return Token{}, start, false, nil
		}
	} else if next < end && p.source[next] == '[' {
		refClose := strings.IndexByte(p.source[next+1:end], ']')
		if refClose < 0 {
			return Token{}, start, false, nil
		}
		refClose += next + 1
		label := p.source[next+1 : refClose]
		if label == "" {
			label = p.source[bracket+1 : closing]
		}
		ref, found := p.references[normalizeReferenceLabel(label)]
		if !found {
			return Token{}, start, false, nil
		}
		destination, title, next = ref.destination, ref.title, refClose+1
	} else {
		ref, found := p.references[normalizeReferenceLabel(p.source[bracket+1:closing])]
		if !found {
			return Token{}, start, false, nil
		}
		destination, title = ref.destination, ref.title
	}

	attributes := map[string]string{
		AttributeDestination: html.UnescapeString(destination),
		AttributeTitle:       html.UnescapeString(title),
	}
	if image {
		return makeToken(
			TokenImage,
			html.UnescapeString(p.source[bracket+1:closing]),
			attributes,
			[]Token{},
			Span{Start: start, End: next},
		), next, true, nil
	}
	children, err := p.parseInlineRange(bracket+1, closing, depth+1)
	if err != nil {
		return Token{}, start, false, err
	}
	return makeToken(TokenLink, "", attributes, children, Span{Start: start, End: next}), next, true, nil
}

func parseInlineLinkDestination(source string, open, end int) (string, string, int, bool) {
	position := open + 1
	for position < end && (source[position] == ' ' || source[position] == '\t' || source[position] == '\n') {
		position++
	}
	destination := ""
	if position < end && source[position] == '<' {
		closing := strings.IndexByte(source[position+1:end], '>')
		if closing < 0 {
			return "", "", open, false
		}
		closing += position + 1
		destination = source[position+1 : closing]
		position = closing + 1
	} else {
		start := position
		parentheses := 0
	destination:
		for position < end {
			switch source[position] {
			case '\\':
				if position+1 < end {
					position += 2
					continue
				}
				break destination
			case '(':
				parentheses++
				position++
				continue
			case ')':
				if parentheses == 0 {
					break destination
				}
				parentheses--
				position++
				continue
			case ' ', '\t', '\n':
				break destination
			default:
				position++
				continue
			}
		}
		destination = source[start:position]
	}
	for position < end && (source[position] == ' ' || source[position] == '\t' || source[position] == '\n') {
		position++
	}
	title := ""
	if position < end && source[position] != ')' {
		var ok bool
		title, position, ok = parseInlineLinkTitle(source, position, end)
		if !ok {
			return "", "", open, false
		}
	}
	if position >= end || source[position] != ')' {
		return "", "", open, false
	}
	return destination, title, position + 1, true
}

func parseInlineLinkTitle(source string, position, end int) (string, int, bool) {
	start := position
	closing := byte(0)
	switch quote := source[position]; quote {
	case '"', '\'':
		closing = quote
	case '(':
		closing = ')'
	default:
		return "", position, false
	}
	position++
	for position < end && source[position] != closing {
		position++
	}
	if position >= end {
		return "", position, false
	}
	title := source[start+1 : position]
	position++
	for position < end && (source[position] == ' ' || source[position] == '\t' || source[position] == '\n') {
		position++
	}
	return title, position, true
}

func findClosingBracket(source string, start, end int) int {
	depth := 0
	for i := start + 1; i < end; i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		switch source[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func (p *parser) parseAngleToken(start, end int) (Token, int, bool) {
	closing := strings.IndexByte(p.source[start+1:end], '>')
	if closing < 0 {
		return Token{}, start, false
	}
	closing += start + 1
	value := p.source[start+1 : closing]
	if destination, ok := angleDestination(value); ok {
		return makeToken(
			TokenAutolink,
			value,
			map[string]string{AttributeDestination: destination},
			[]Token{},
			Span{Start: start, End: closing + 1},
		), closing + 1, true
	}
	if looksLikeHTML(value) {
		return makeToken(
			TokenHTMLInline,
			p.source[start:closing+1],
			map[string]string{},
			[]Token{},
			Span{Start: start, End: closing + 1},
		), closing + 1, true
	}
	return Token{}, start, false
}

func angleDestination(value string) (string, bool) {
	if isURIAutolink(value) {
		return value, true
	}
	if looksEmail(value) {
		return "mailto:" + value, true
	}
	return "", false
}

func isURIAutolink(value string) bool {
	colon := strings.IndexByte(value, ':')
	if colon < 2 || colon > 32 || colon == len(value)-1 {
		return false
	}
	if !isASCIIAlphaNumeric(value[0]) || value[0] >= '0' && value[0] <= '9' {
		return false
	}
	for i := 1; i < colon; i++ {
		character := value[i]
		if !isASCIIAlphaNumeric(character) && character != '+' && character != '.' && character != '-' {
			return false
		}
	}
	return !strings.ContainsAny(value[colon+1:], " <>\t\r\n")
}

func looksLikeHTML(value string) bool {
	if value == "" || strings.HasPrefix(value, "!") || strings.HasPrefix(value, "?") {
		return true
	}
	closing := false
	if value[0] == '/' {
		closing = true
		value = value[1:]
	}
	if value == "" || !isASCIIAlpha(value[0]) {
		return false
	}
	position := 1
	for position < len(value) && (isASCIIAlphaNumeric(value[position]) || value[position] == '-') {
		position++
	}
	if position == len(value) || isWhitespaceByte(value[position]) {
		return true
	}
	return !closing && value[position] == '/'
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func (p *parser) parseDelimited(start, end, depth int) (Token, int, bool, error) {
	marker := p.source[start]
	length := 1
	if marker == '~' {
		if start > 0 && p.source[start-1] == marker {
			return Token{}, start, false, nil
		}
		length = delimiterRunLength(p.source, start, end, marker)
		if length > 2 {
			return Token{}, start, false, nil
		}
	} else if (marker == '*' || marker == '_') && start+1 < end && p.source[start+1] == marker {
		length = 2
	}
	if marker != '*' && marker != '_' && marker != '~' {
		return Token{}, start, false, nil
	}
	if marker == '_' && start > 0 && isWordRune(rune(p.source[start-1])) {
		return Token{}, start, false, nil
	}

	var closing int
	if marker == '~' {
		closing = findTildeDelimiter(p.source, start+length, end, length)
	} else {
		closing = findDelimiter(p.source, start+length, end, marker, length)
	}
	if closing < 0 || closing == start+length {
		return Token{}, start, false, nil
	}
	if marker == '_' && closing+length < end && isWordRune(rune(p.source[closing+length])) {
		return Token{}, start, false, nil
	}
	children, err := p.parseInlineRange(start+length, closing, depth+1)
	if err != nil {
		return Token{}, start, false, err
	}
	id := TokenEmphasis
	if marker == '~' {
		id = TokenStrikethrough
	} else if length == 2 {
		id = TokenStrong
	}
	return makeToken(id, "", map[string]string{}, children, Span{Start: start, End: closing + length}), closing + length, true, nil
}

func findDelimiter(source string, start, end int, marker byte, length int) int {
	for i := start; i+length <= end; i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == marker && (length == 1 || (i+1 < end && source[i+1] == marker)) {
			return i
		}
	}
	return -1
}

func delimiterRunLength(source string, start, end int, marker byte) int {
	position := start
	for position < end && source[position] == marker {
		position++
	}
	return position - start
}

func findTildeDelimiter(source string, start, end, length int) int {
	for i := start; i < end; {
		if source[i] == '\\' {
			i += 2
			continue
		}
		if source[i] != '~' {
			i++
			continue
		}

		runLength := delimiterRunLength(source, i, end, '~')
		if runLength == length {
			return i
		}
		i += runLength
	}
	return -1
}

func (p *parser) parseAutolinkLiteral(start, end int) (Token, int, bool) {
	value := p.source[start:end]
	prefix := ""
	switch {
	case strings.HasPrefix(value, "http://"):
		prefix = "http://"
	case strings.HasPrefix(value, "https://"):
		prefix = "https://"
	case strings.HasPrefix(value, "mailto:"):
		prefix = "mailto:"
	case strings.HasPrefix(value, "xmpp:"):
		prefix = "xmpp:"
	case strings.HasPrefix(value, "www."):
		prefix = "www."
	default:
		if isAutolinkBoundary(p.source, start) {
			if looksEmailStart(value) {
				prefix = "email"
			}
		}
	}
	if prefix == "" {
		return Token{}, start, false
	}
	if !isAutolinkBoundary(p.source, start) {
		return Token{}, start, false
	}

	length := 0
	for length < len(value) && !isURLTerminator(value[length]) {
		length++
	}
	if (prefix == "email" || prefix == "mailto:" || prefix == "xmpp:") && length == 0 {
		return Token{}, start, false
	}
	literal := value[:length]
	literal = trimAutolinkPunctuation(literal)
	if prefix != "email" && len(literal) == len(prefix) {
		return Token{}, start, false
	}
	if (prefix == "mailto:" || prefix == "xmpp:") && !isGFMBareEmail(strings.TrimPrefix(literal, prefix)) {
		return Token{}, start, false
	}
	destination := literal
	if prefix == "www." {
		destination = "http://" + literal
	}
	if prefix == "email" {
		if !isGFMBareEmail(literal) {
			return Token{}, start, false
		}
		destination = "mailto:" + literal
	}
	return makeToken(
		TokenAutolink,
		literal,
		map[string]string{AttributeDestination: destination},
		[]Token{},
		Span{Start: start, End: start + len(literal)},
	), start + len(literal), true
}

func isEscapable(value byte) bool {
	return strings.ContainsRune(`\\`+"`*{}[]()#+-.!_>~|", rune(value))
}

func isWordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value)
}

func isWhitespaceByte(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n'
}

func isAutolinkBoundary(source string, start int) bool {
	if start == 0 {
		return true
	}
	previous := source[start-1]
	return isWhitespaceByte(previous) || strings.ContainsRune("*_~(", rune(previous))
}

func isURLTerminator(value byte) bool {
	return isWhitespaceByte(value) || value == '<' || value == '>' || value == '"'
}

func trimAutolinkPunctuation(value string) string {
	for len(value) > 0 {
		last := value[len(value)-1]
		if strings.ContainsRune(".,:;!?", rune(last)) {
			value = value[:len(value)-1]
			continue
		}
		if last == ')' && strings.Count(value, ")") > strings.Count(value, "(") {
			value = value[:len(value)-1]
			continue
		}
		if last == ']' && strings.Count(value, "]") > strings.Count(value, "[") {
			value = value[:len(value)-1]
			continue
		}
		if last == '}' && strings.Count(value, "}") > strings.Count(value, "{") {
			value = value[:len(value)-1]
			continue
		}
		break
	}
	return value
}

func looksEmailStart(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] == '@' {
			return i > 0
		}
		if !isEmailLiteralByte(value[i]) {
			return false
		}
	}
	return false
}

func isEmailLiteralByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		strings.ContainsRune("._+-", rune(value))
}

func looksEmail(value string) bool {
	at := strings.IndexByte(value, '@')
	if at <= 0 || at == len(value)-1 || strings.IndexByte(value[at+1:], '@') >= 0 {
		return false
	}
	for i := 0; i < at; i++ {
		if !strings.ContainsRune(".!#$%&'*+/=?^_`{|}~-", rune(value[i])) && !isASCIIAlphaNumeric(value[i]) {
			return false
		}
	}
	for _, label := range strings.Split(value[at+1:], ".") {
		if len(label) == 0 || len(label) > 63 || !isASCIIAlphaNumeric(label[0]) || !isASCIIAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for i := 1; i < len(label)-1; i++ {
			if !isASCIIAlphaNumeric(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func isGFMBareEmail(value string) bool {
	at := strings.IndexByte(value, '@')
	if at <= 0 || at == len(value)-1 || strings.IndexByte(value[at+1:], '@') >= 0 {
		return false
	}
	for i := 0; i < at; i++ {
		if !isASCIIAlphaNumeric(value[i]) && !strings.ContainsRune("._+-", rune(value[i])) {
			return false
		}
	}
	domain := value[at+1:]
	if !strings.ContainsRune(domain, '.') || domain[len(domain)-1] == '-' || domain[len(domain)-1] == '_' {
		return false
	}
	for i := 0; i < len(domain); i++ {
		if !isASCIIAlphaNumeric(domain[i]) && !strings.ContainsRune("._-", rune(domain[i])) {
			return false
		}
		if domain[i] == '.' && (i == 0 || domain[i-1] == '.') {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}
