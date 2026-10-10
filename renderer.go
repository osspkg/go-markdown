/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package markdown

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// RenderContext contains the converter configuration and safe helpers for a
// custom Handler. It is valid only for the duration of that handler call.
type RenderContext struct {
	state *renderState
	depth int
}

type renderState struct {
	cfg      config
	handlers map[TokenID]Handler
	source   string
	warnings *[]Diagnostic
}

// Tokenize parses source into a hierarchical token tree. Input is normalized
// to LF before parsing, and spans refer to that normalized source.
func (c *Converter) Tokenize(source string) (TokenizationResult, error) {
	snapshot, err := c.snapshot()
	if err != nil {
		return TokenizationResult{}, err
	}
	return tokenizeWithSnapshot(source, snapshot)
}

func tokenizeWithSnapshot(source string, snapshot converterSnapshot) (TokenizationResult, error) {
	normalized, err := normalizeSource(source)
	if err != nil {
		return TokenizationResult{}, err
	}
	return tokenizeNormalized(normalized, snapshot)
}

func tokenizeNormalized(normalized string, snapshot converterSnapshot) (TokenizationResult, error) {
	if snapshot.cfg.maxInputBytes > 0 && len(normalized) > snapshot.cfg.maxInputBytes {
		return TokenizationResult{}, fmt.Errorf("%w: input bytes %d", ErrLimitExceeded, len(normalized))
	}

	return newParser(normalized, snapshot.cfg, snapshot.rules).parse()
}

// Render writes tokens to writer and returns non-fatal warnings. The token
// slice may come from Tokenize or be assembled by the caller.
func (c *Converter) Render(writer io.Writer, tokens []Token) ([]Diagnostic, error) {
	if writer == nil {
		return []Diagnostic{}, errors.New("markdown: nil render writer")
	}
	snapshot, err := c.snapshot()
	if err != nil {
		return []Diagnostic{}, err
	}

	warnings := []Diagnostic{}
	state := &renderState{
		cfg:      snapshot.cfg,
		handlers: snapshot.handlers,
		source:   "",
		warnings: &warnings,
	}
	for _, token := range tokens {
		if err := state.renderToken(writer, token, 0); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

// Convert tokenizes source and renders it into a string. Best-effort warnings
// are returned in the result while fatal configuration and input errors are
// returned as error.
func (c *Converter) Convert(source string) (ConversionResult, error) {
	snapshot, err := c.snapshot()
	if err != nil {
		return ConversionResult{}, err
	}

	normalized, err := normalizeSource(source)
	if err != nil {
		return ConversionResult{}, err
	}
	tokenization, err := tokenizeNormalized(normalized, snapshot)
	if err != nil {
		return ConversionResult{}, err
	}
	warnings := append([]Diagnostic{}, tokenization.Warnings...)
	state := &renderState{
		cfg:      snapshot.cfg,
		handlers: snapshot.handlers,
		source:   normalized,
		warnings: &warnings,
	}
	var builder strings.Builder
	for _, token := range tokenization.Tokens {
		if err := state.renderToken(&builder, token, 0); err != nil {
			return ConversionResult{}, err
		}
	}

	return ConversionResult{
		HTML:     builder.String(),
		Warnings: warnings,
	}, nil
}

func (s *renderState) renderToken(writer io.Writer, token Token, depth int) error {
	if depth > s.cfg.maxNestingDepth {
		return s.wrapRenderError(token, fmt.Errorf("%w: rendering depth %d", ErrLimitExceeded, depth))
	}
	handler, ok := s.handlers[token.ID]
	if !ok {
		return s.handleRenderError(
			writer,
			token,
			fmt.Errorf("%w %q", ErrUnknownToken, token.ID),
		)
	}

	var buffer strings.Builder
	ctx := &RenderContext{state: s, depth: depth}
	if err := handler(ctx, &buffer, token); err != nil {
		return s.handleRenderError(writer, token, err)
	}
	if err := writeString(writer, buffer.String()); err != nil {
		return s.wrapRenderError(token, err)
	}
	return nil
}

func (s *renderState) handleRenderError(writer io.Writer, token Token, err error) error {
	wrapped := s.wrapRenderError(token, err)
	if !isBestEffort(s.cfg) {
		return wrapped
	}
	*s.warnings = append(*s.warnings, wrapDiagnostic(wrapped, RulePhaseRender, token.ID, token.Span))
	if err := s.writeFallback(writer, token); err != nil {
		return s.wrapRenderError(token, err)
	}
	return nil
}

func (s *renderState) wrapRenderError(token Token, err error) error {
	return &ParseError{
		Err:    fmt.Errorf("render token %q: %w", token.ID, err),
		Phase:  RulePhaseRender,
		RuleID: token.ID,
		Span:   token.Span,
	}
}

func (s *renderState) writeFallback(writer io.Writer, token Token) error {
	if token.Literal != "" {
		return writeString(writer, html.EscapeString(token.Literal))
	}
	ctx := &RenderContext{state: s, depth: 0}
	for _, child := range token.Children {
		if err := ctx.renderToken(writer, child); err != nil {
			return err
		}
	}
	return nil
}

func (c *RenderContext) renderToken(writer io.Writer, token Token) error {
	return c.state.renderToken(writer, token, c.depth+1)
}

// RenderChildren renders all child tokens in order.
func (c *RenderContext) RenderChildren(writer io.Writer, token Token) error {
	if c == nil || c.state == nil {
		return errors.New("markdown: nil render context")
	}
	for _, child := range token.Children {
		if err := c.renderToken(writer, child); err != nil {
			return err
		}
	}
	return nil
}

// WriteText writes text after decoding Markdown character references and
// escaping HTML-significant characters.
func (c *RenderContext) WriteText(writer io.Writer, value string) error {
	return writeString(writer, html.EscapeString(html.UnescapeString(value)))
}

// WriteAttribute writes an HTML attribute value with HTML escaping.
func (c *RenderContext) WriteAttribute(writer io.Writer, value string) error {
	return writeString(writer, html.EscapeString(value))
}

// SafeURL validates a URL against the renderer's safe scheme policy.
func (c *RenderContext) SafeURL(value string) (string, bool) {
	return safeURL(value)
}

// Source returns the source fragment covered by token when the context was
// created by Convert. It returns an empty string for externally supplied trees.
func (c *RenderContext) Source(token Token) string {
	if c == nil || c.state == nil {
		return ""
	}
	return sourceSpan(c.state.source, token.Span)
}

func (c *RenderContext) openBlock(writer io.Writer, tag string) error {
	if c.state.cfg.htmlFormat == HTMLFormatPretty {
		if err := writeString(writer, strings.Repeat("  ", c.depth)); err != nil {
			return err
		}
	}
	return writeString(writer, tag)
}

func (c *RenderContext) closeBlock(writer io.Writer, tag string) error {
	if c.state.cfg.htmlFormat == HTMLFormatPretty {
		if err := writeString(writer, "\n"); err != nil {
			return err
		}
		if err := writeString(writer, strings.Repeat("  ", c.depth)); err != nil {
			return err
		}
	}
	if err := writeString(writer, tag); err != nil {
		return err
	}
	if c.state.cfg.htmlFormat == HTMLFormatPretty {
		return writeString(writer, "\n")
	}
	return nil
}

func defaultHandlers() map[TokenID]Handler {
	return map[TokenID]Handler{
		TokenDocument:      renderDocument,
		TokenParagraph:     renderParagraph,
		TokenHeading:       renderHeading,
		TokenBlockquote:    renderBlockquote,
		TokenList:          renderList,
		TokenListItem:      renderListItem,
		TokenCodeBlock:     renderCodeBlock,
		TokenCodeSpan:      renderCodeSpan,
		TokenThematicRule:  renderThematicRule,
		TokenText:          renderText,
		TokenEmphasis:      renderElement("em"),
		TokenStrong:        renderElement("strong"),
		TokenStrikethrough: renderElement("del"),
		TokenLink:          renderLink,
		TokenImage:         renderImage,
		TokenAutolink:      renderAutolink,
		TokenSoftbreak:     renderSoftbreak,
		TokenHardbreak:     renderHardbreak,
		TokenHTMLBlock:     renderHTML,
		TokenHTMLInline:    renderHTML,
		TokenTable:         renderTable,
		TokenTableHead:     renderTableHead,
		TokenTableBody:     renderTableBody,
		TokenTableRow:      renderTableRow,
		TokenTableCell:     renderTableCell,
		TokenTaskCheckbox:  renderTaskCheckbox,
	}
}

func renderDocument(ctx *RenderContext, writer io.Writer, token Token) error {
	return ctx.RenderChildren(writer, token)
}

func renderParagraph(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<p>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</p>")
}

func renderHeading(ctx *RenderContext, writer io.Writer, token Token) error {
	level, err := strconv.Atoi(token.Attributes[AttributeLevel])
	if err != nil || level < 1 || level > 6 {
		return errors.New("invalid heading level")
	}
	open := fmt.Sprintf("<h%d>", level)
	closing := fmt.Sprintf("</h%d>", level)
	if err := ctx.openBlock(writer, open); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, closing)
}

func renderBlockquote(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<blockquote>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</blockquote>")
}

func renderList(ctx *RenderContext, writer io.Writer, token Token) error {
	ordered := token.Attributes[AttributeOrdered] == "true"
	tag := "ul"
	open := "<ul>"
	if ordered {
		tag = "ol"
		open = "<ol>"
		start := token.Attributes[AttributeStart]
		if start != "" && start != "1" {
			open = `<ol start="` + html.EscapeString(start) + `">`
		}
	}
	if err := ctx.openBlock(writer, open); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</"+tag+">")
}

func renderListItem(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<li>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</li>")
}

func renderCodeBlock(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<pre><code"); err != nil {
		return err
	}
	language := token.Attributes[AttributeLanguage]
	if language != "" {
		if err := writeString(writer, ` class="language-`+html.EscapeString(language)+`"`); err != nil {
			return err
		}
	}
	if err := writeString(writer, ">"+html.EscapeString(token.Literal)+"</code></pre>"); err != nil {
		return err
	}
	if ctx.state.cfg.htmlFormat == HTMLFormatPretty {
		return writeString(writer, "\n")
	}
	return nil
}

func renderCodeSpan(_ *RenderContext, writer io.Writer, token Token) error {
	return writeString(writer, "<code>"+html.EscapeString(token.Literal)+"</code>")
}

func renderThematicRule(ctx *RenderContext, writer io.Writer, _ Token) error {
	if err := ctx.openBlock(writer, "<hr>"); err != nil {
		return err
	}
	if ctx.state.cfg.htmlFormat == HTMLFormatPretty {
		return writeString(writer, "\n")
	}
	return nil
}

func renderText(ctx *RenderContext, writer io.Writer, token Token) error {
	return ctx.WriteText(writer, token.Literal)
}

func renderElement(tag string) Handler {
	return func(ctx *RenderContext, writer io.Writer, token Token) error {
		if err := writeString(writer, "<"+tag+">"); err != nil {
			return err
		}
		if err := ctx.RenderChildren(writer, token); err != nil {
			return err
		}
		return writeString(writer, "</"+tag+">")
	}
}

func renderLink(ctx *RenderContext, writer io.Writer, token Token) error {
	destination := token.Attributes[AttributeDestination]
	safe, ok := ctx.SafeURL(destination)
	if !ok {
		return ctx.RenderChildren(writer, token)
	}
	if err := writeString(writer, `<a href="`+html.EscapeString(safe)+`"`); err != nil {
		return err
	}
	if title := token.Attributes[AttributeTitle]; title != "" {
		if err := writeString(writer, ` title="`+html.EscapeString(title)+`"`); err != nil {
			return err
		}
	}
	if err := writeString(writer, ">"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return writeString(writer, "</a>")
}

func renderAutolink(ctx *RenderContext, writer io.Writer, token Token) error {
	return renderLink(ctx, writer, Token{
		ID:         TokenLink,
		Literal:    token.Literal,
		Attributes: token.Attributes,
		Children:   []Token{makeToken(TokenText, token.Literal, map[string]string{}, []Token{}, token.Span)},
		Span:       token.Span,
	})
}

func renderImage(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := writeString(writer, "<img"); err != nil {
		return err
	}
	if destination, ok := ctx.SafeURL(token.Attributes[AttributeDestination]); ok {
		if err := writeString(writer, ` src="`+html.EscapeString(destination)+`"`); err != nil {
			return err
		}
	}
	if err := writeString(writer, ` alt="`+html.EscapeString(token.Literal)+`"`); err != nil {
		return err
	}
	if title := token.Attributes[AttributeTitle]; title != "" {
		if err := writeString(writer, ` title="`+html.EscapeString(title)+`"`); err != nil {
			return err
		}
	}
	return writeString(writer, ">")
}

func renderSoftbreak(_ *RenderContext, writer io.Writer, _ Token) error {
	return writeString(writer, "\n")
}

func renderHardbreak(ctx *RenderContext, writer io.Writer, _ Token) error {
	if err := writeString(writer, "<br>"); err != nil {
		return err
	}
	if ctx.state.cfg.htmlFormat == HTMLFormatPretty {
		return writeString(writer, "\n")
	}
	return nil
}

func renderHTML(ctx *RenderContext, writer io.Writer, token Token) error {
	if ctx.state.cfg.unsafeRawHTML {
		return writeString(writer, filterDisallowedHTML(token.Literal))
	}
	return ctx.WriteText(writer, token.Literal)
}

func filterDisallowedHTML(value string) string {
	const (
		lessThanEntity = "&lt;"
		disallowedTags = "title textarea style xmp iframe noembed noframes script plaintext"
	)

	var builder strings.Builder
	last := 0
	for position := 0; position < len(value); position++ {
		if value[position] != '<' {
			continue
		}

		tagStart := position + 1
		if tagStart < len(value) && value[tagStart] == '/' {
			tagStart++
		}
		for _, tag := range strings.Fields(disallowedTags) {
			end := tagStart + len(tag)
			if end > len(value) || !strings.EqualFold(value[tagStart:end], tag) {
				continue
			}
			if end < len(value) && value[end] != '>' && value[end] != '/' && !isWhitespaceByte(value[end]) {
				continue
			}

			builder.WriteString(value[last:position])
			builder.WriteString(lessThanEntity)
			last = position + 1
			position = end - 1
			break
		}
	}
	if last == 0 {
		return value
	}
	builder.WriteString(value[last:])
	return builder.String()
}

func renderTable(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<table>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</table>")
}

func renderTableHead(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<thead>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</thead>")
}

func renderTableBody(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<tbody>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</tbody>")
}

func renderTableRow(ctx *RenderContext, writer io.Writer, token Token) error {
	if err := ctx.openBlock(writer, "<tr>"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return ctx.closeBlock(writer, "</tr>")
}

func renderTableCell(ctx *RenderContext, writer io.Writer, token Token) error {
	tag := "td"
	if token.Attributes[AttributeHeader] == "true" {
		tag = "th"
	}
	if err := writeString(writer, "<"+tag); err != nil {
		return err
	}
	if alignment := token.Attributes[AttributeAlignment]; alignment != "" {
		if err := writeString(writer, ` align="`+html.EscapeString(alignment)+`"`); err != nil {
			return err
		}
	}
	if err := writeString(writer, ">"); err != nil {
		return err
	}
	if err := ctx.RenderChildren(writer, token); err != nil {
		return err
	}
	return writeString(writer, "</"+tag+">")
}

func renderTaskCheckbox(_ *RenderContext, writer io.Writer, token Token) error {
	checked := ""
	if token.Attributes[AttributeChecked] == "true" {
		checked = " checked"
	}
	return writeString(writer, `<input type="checkbox" disabled`+checked+`> `)
}

func safeURL(value string) (string, bool) {
	value = strings.TrimSpace(html.UnescapeString(value))
	if value == "" {
		return "", true
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "", "http", "https", "mailto", "tel", "xmpp":
		return value, true
	default:
		return "", false
	}
}

func writeString(writer io.Writer, value string) error {
	if value == "" {
		return nil
	}
	written, err := io.WriteString(writer, value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}
