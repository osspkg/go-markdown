/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package markdown

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// ConvertHTML converts an HTML document or fragment to GitHub Flavored Markdown.
// It maps page title and description to YAML frontmatter, preserves content
// inside unsupported elements, and omits scripts, styles, and comments. It
// carries the converter's input, nesting, and node-count limits. The first call
// freezes the converter configuration, like Convert and Tokenize.
func (c *Converter) ConvertHTML(source string) (string, error) {
	snapshot, err := c.snapshot()
	if err != nil {
		return "", err
	}

	normalized, err := normalizeSource(source)
	if err != nil {
		return "", err
	}
	if snapshot.cfg.maxInputBytes > 0 && len(normalized) > snapshot.cfg.maxInputBytes {
		return "", fmt.Errorf("%w: input bytes %d", ErrLimitExceeded, len(normalized))
	}

	document, err := html.Parse(strings.NewReader(normalized))
	if err != nil {
		return "", fmt.Errorf("%w: parse HTML: %w", ErrLimitExceeded, err)
	}
	if err := checkHTMLNodeLimits(document, 0, snapshot.cfg, new(int)); err != nil {
		return "", err
	}

	converter := htmlMarkdownConverter{
		cfg:      snapshot.cfg,
		handlers: snapshot.htmlHandlers,
		rules:    snapshot.htmlRules,
	}
	head := findHTMLChild(document, "html", "head")
	body := findHTMLChild(document, "html", "body")
	frontmatter := converter.frontmatter(head)
	content, err := converter.flowChildren(body, 1)
	if err != nil {
		return "", err
	}
	content = strings.TrimSpace(content)
	if frontmatter == "" {
		return content, nil
	}
	if content == "" {
		return frontmatter, nil
	}
	return frontmatter + "\n\n" + content, nil
}

type htmlMarkdownConverter struct {
	cfg      config
	handlers map[string]HTMLHandler
	rules    []registeredHTMLRule
}

// HTMLNode is a read-only view of one node in the parsed HTML tree.
type HTMLNode struct {
	node *html.Node
}

// Tag returns the lower-case element name, or an empty string for non-elements.
func (n HTMLNode) Tag() string {
	if n.node == nil || n.node.Type != html.ElementNode {
		return ""
	}
	return n.node.Data
}

// Text returns the concatenated text content of this node and its descendants.
func (n HTMLNode) Text() string {
	if n.node == nil {
		return ""
	}
	return nodeText(n.node)
}

// Attribute returns the value of an element attribute and whether it exists.
func (n HTMLNode) Attribute(name string) (string, bool) {
	if n.node == nil {
		return "", false
	}
	for _, attr := range n.node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val, true
		}
	}
	return "", false
}

// HTMLHandler renders one HTML element as Markdown.
type HTMLHandler func(*HTMLRenderContext, io.Writer, HTMLNode) error

// HTMLRule matches an HTML element and renders matching elements. Rules with
// higher priority run first; equal priorities use reverse registration order.
type HTMLRule struct {
	ID       string
	Priority int
	Match    func(HTMLNode) bool
	Handler  HTMLHandler
}

// HTMLRenderContext provides helpers for custom HTML handlers.
type HTMLRenderContext struct {
	converter *htmlMarkdownConverter
	inline    bool
	depth     int
}

// RenderChildren converts the element's children using the current block or
// inline rendering mode.
func (c *HTMLRenderContext) RenderChildren(w io.Writer, node HTMLNode) error {
	if c == nil || c.converter == nil || node.node == nil {
		return nil
	}
	var output string
	var err error
	if c.inline {
		output, err = c.converter.inlineChildren(node.node, c.depth+1)
	} else {
		output, err = c.converter.flowChildren(node.node, c.depth+1)
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, output)
	return err
}

// WriteText escapes text for literal inclusion in Markdown output.
func (c *HTMLRenderContext) WriteText(w io.Writer, value string) error {
	_, err := io.WriteString(w, escapeMarkdownText(value))
	return err
}

func checkHTMLNodeLimits(node *html.Node, depth int, cfg config, count *int) error {
	if depth > cfg.maxNestingDepth {
		return fmt.Errorf("%w: HTML nesting depth %d", ErrLimitExceeded, depth)
	}
	(*count)++
	if *count > cfg.maxTokens {
		return fmt.Errorf("%w: HTML node count %d", ErrLimitExceeded, *count)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := checkHTMLNodeLimits(child, depth+1, cfg, count); err != nil {
			return err
		}
	}
	return nil
}

func findHTMLChild(node *html.Node, parent, child string) *html.Node {
	for current := node.FirstChild; current != nil; current = current.NextSibling {
		if current.Type != html.ElementNode || current.Data != parent {
			continue
		}
		for nested := current.FirstChild; nested != nil; nested = nested.NextSibling {
			if nested.Type == html.ElementNode && nested.Data == child {
				return nested
			}
		}
	}
	return nil
}

func (c *htmlMarkdownConverter) frontmatter(head *html.Node) string {
	if head == nil {
		return ""
	}
	var title, description string
	titleFound, descriptionFound := false, false
	for node := head.FirstChild; node != nil; node = node.NextSibling {
		if node.Type != html.ElementNode {
			continue
		}
		switch node.Data {
		case "title":
			if !titleFound {
				title = strings.TrimSpace(collapseHTMLWhitespace(nodeText(node)))
				titleFound = true
			}
		case "meta":
			if !descriptionFound && strings.EqualFold(attribute(node, "name"), "description") {
				description = strings.TrimSpace(attribute(node, "content"))
				descriptionFound = true
			}
		}
	}
	if title == "" && description == "" {
		return ""
	}
	var output strings.Builder
	output.WriteString("---\n")
	if title != "" {
		output.WriteString("title: ")
		output.WriteString(yamlString(title))
		output.WriteByte('\n')
	}
	if description != "" {
		output.WriteString("description: ")
		output.WriteString(yamlString(description))
		output.WriteByte('\n')
	}
	output.WriteString("---")
	return output.String()
}

func yamlString(value string) string {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(output.String(), "\n")
}

func (c *htmlMarkdownConverter) flowChildren(node *html.Node, depth int) (string, error) {
	if node == nil {
		return "", nil
	}
	blocks := make([]string, 0)
	var inline strings.Builder
	flushInline := func() {
		value := strings.TrimSpace(inline.String())
		if value != "" {
			blocks = append(blocks, value)
		}
		inline.Reset()
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.CommentNode, html.DoctypeNode:
			continue
		case html.TextNode:
			inline.WriteString(collapseHTMLWhitespace(child.Data))
		case html.ElementNode:
			if child.Data == "script" || child.Data == "style" {
				continue
			}
			if child.Data == "input" {
				value, err := c.inline(child, depth+1)
				if err != nil {
					return "", err
				}
				inline.WriteString(value)
				continue
			}
			if !isHTMLMarkdownBlock(child.Data) {
				value, err := c.inline(child, depth+1)
				if err != nil {
					return "", err
				}
				inline.WriteString(value)
				continue
			}
			flushInline()
			value, err := c.block(child, depth+1)
			if err != nil {
				return "", err
			}
			if value = strings.TrimSpace(value); value != "" {
				blocks = append(blocks, value)
			}
		}
	}
	flushInline()
	return strings.Join(blocks, "\n\n"), nil
}

func isHTMLMarkdownBlock(tag string) bool {
	switch tag {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "ul", "ol", "pre", "hr", "table",
		"div", "section", "article", "main", "header", "footer", "nav", "aside", "details", "summary", "figure", "figcaption", "address", "dl", "dt", "dd", "form", "fieldset":
		return true
	default:
		return false
	}
}

func (c *htmlMarkdownConverter) block(node *html.Node, depth int) (string, error) {
	if rendered, ok, err := c.custom(node, depth, false); ok || err != nil {
		return rendered, err
	}
	switch node.Data {
	case "p":
		return c.inlineChildren(node, depth+1)
	case "h1", "h2", "h3", "h4", "h5", "h6":
		content, err := c.inlineChildren(node, depth+1)
		if err != nil {
			return "", err
		}
		level := int(node.Data[1] - '0')
		return strings.Repeat("#", level) + " " + strings.TrimSpace(content), nil
	case "blockquote":
		content, err := c.flowChildren(node, depth+1)
		if err != nil || strings.TrimSpace(content) == "" {
			return "", err
		}
		lines := strings.Split(strings.TrimSpace(content), "\n")
		for i := range lines {
			lines[i] = "> " + lines[i]
		}
		return strings.Join(lines, "\n"), nil
	case "ul", "ol":
		return c.list(node, depth+1)
	case "pre":
		return c.codeBlock(node), nil
	case "hr":
		return "---", nil
	case "table":
		return c.table(node, depth+1)
	default:
		return c.flowChildren(node, depth+1)
	}
}

func (c *htmlMarkdownConverter) inline(node *html.Node, depth int) (string, error) {
	if depth > c.cfg.maxNestingDepth {
		return "", fmt.Errorf("%w: HTML nesting depth %d", ErrLimitExceeded, depth)
	}
	if node.Type == html.TextNode {
		return escapeMarkdownText(collapseHTMLWhitespace(node.Data)), nil
	}
	if node.Type != html.ElementNode || node.Data == "script" || node.Data == "style" {
		return "", nil
	}
	if rendered, ok, err := c.custom(node, depth, true); ok || err != nil {
		return rendered, err
	}
	if node.Data == "input" {
		return "", nil
	}
	if node.Data == "br" {
		return "  \n", nil
	}
	if node.Data == "img" {
		alt := escapeMarkdownText(attribute(node, "alt"))
		safeSource, ok := safeURL(attribute(node, "src"))
		if !ok {
			return alt, nil
		}
		destination := markdownDestination(safeSource)
		if destination == "" {
			return alt, nil
		}
		result := "![" + alt + "](<" + destination + ">)"
		if title := attribute(node, "title"); title != "" {
			result = "![" + alt + "](<" + destination + "> " + markdownTitle(title) + ")"
		}
		return result, nil
	}
	if node.Data == "code" {
		return markdownCodeSpan(nodeText(node)), nil
	}
	content, err := c.inlineChildren(node, depth+1)
	if err != nil {
		return "", err
	}
	switch node.Data {
	case "em", "i":
		return wrapMarkdown(content, "*"), nil
	case "strong", "b":
		return wrapMarkdown(content, "**"), nil
	case "del", "s", "strike":
		return wrapMarkdown(content, "~~"), nil
	case "a":
		safeHref, ok := safeURL(attribute(node, "href"))
		if !ok {
			return content, nil
		}
		destination := markdownDestination(safeHref)
		if destination == "" {
			return content, nil
		}
		result := "[" + content + "](<" + destination + ">)"
		if title := attribute(node, "title"); title != "" {
			result = "[" + content + "](<" + destination + "> " + markdownTitle(title) + ")"
		}
		return result, nil
	default:
		return content, nil
	}
}

func (c *htmlMarkdownConverter) custom(node *html.Node, depth int, inline bool) (string, bool, error) {
	if node == nil || node.Type != html.ElementNode {
		return "", false, nil
	}
	view := HTMLNode{node: node}
	context := &HTMLRenderContext{converter: c, inline: inline, depth: depth}
	for _, registered := range c.rules {
		if registered.rule.Match(view) {
			output := new(strings.Builder)
			if err := registered.rule.Handler(context, output, view); err != nil {
				return "", true, fmt.Errorf("markdown: render HTML rule %q: %w", registered.rule.ID, err)
			}
			return output.String(), true, nil
		}
	}
	if handler := c.handlers[node.Data]; handler != nil {
		output := new(strings.Builder)
		if err := handler(context, output, view); err != nil {
			return "", true, fmt.Errorf("markdown: render HTML tag %q: %w", node.Data, err)
		}
		return output.String(), true, nil
	}
	return "", false, nil
}

func (c *htmlMarkdownConverter) inlineChildren(node *html.Node, depth int) (string, error) {
	var output strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		value, err := c.inline(child, depth+1)
		if err != nil {
			return "", err
		}
		output.WriteString(value)
	}
	return output.String(), nil
}

func (c *htmlMarkdownConverter) list(node *html.Node, depth int) (string, error) {
	ordered := node.Data == "ol"
	start, err := strconv.Atoi(attribute(node, "start"))
	if err != nil || start < 1 {
		start = 1
	}
	items := make([]string, 0)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "li" {
			continue
		}
		content, handled, err := c.custom(child, depth+1, false)
		if !handled && err == nil {
			content, err = c.flowChildren(child, depth+1)
		}
		if err != nil {
			return "", err
		}
		marker := "- "
		if ordered {
			marker = strconv.Itoa(start) + ". "
			start++
		}
		if checked, ok := taskCheckbox(child); ok {
			if checked {
				marker += "[x] "
			} else {
				marker += "[ ] "
			}
		}
		lines := strings.Split(strings.TrimSpace(content), "\n")
		if len(lines) == 1 && lines[0] == "" {
			items = append(items, strings.TrimRight(marker, " "))
			continue
		}
		indent := strings.Repeat(" ", len(marker))
		for i := range lines {
			if i == 0 {
				lines[i] = marker + lines[i]
			} else if lines[i] != "" {
				lines[i] = indent + lines[i]
			}
		}
		items = append(items, strings.Join(lines, "\n"))
	}
	return strings.Join(items, "\n"), nil
}

func taskCheckbox(item *html.Node) (bool, bool) {
	for child := item.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode && strings.TrimSpace(child.Data) == "" {
			continue
		}
		if child.Type != html.ElementNode || child.Data != "input" || !strings.EqualFold(attribute(child, "type"), "checkbox") {
			return false, false
		}
		return hasAttribute(child, "checked"), true
	}
	return false, false
}

func (c *htmlMarkdownConverter) codeBlock(node *html.Node) string {
	content := nodeText(node)
	longest := longestRun(content, '`')
	fence := strings.Repeat("`", max(3, longest+1))
	language := ""
	if node.FirstChild != nil && node.FirstChild.Type == html.ElementNode && node.FirstChild.Data == "code" {
		for _, class := range strings.Fields(attribute(node.FirstChild, "class")) {
			if strings.HasPrefix(class, "language-") {
				language = markdownCodeLanguage(strings.TrimPrefix(class, "language-"))
				break
			}
			if strings.HasPrefix(class, "lang-") {
				language = markdownCodeLanguage(strings.TrimPrefix(class, "lang-"))
				break
			}
		}
	}
	var output strings.Builder
	output.WriteString(fence)
	output.WriteString(language)
	output.WriteByte('\n')
	output.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		output.WriteByte('\n')
	}
	output.WriteString(fence)
	return output.String()
}

func markdownCodeLanguage(value string) string {
	if value == "" {
		return ""
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && !strings.ContainsRune("_+-.", character) {
			return ""
		}
	}
	return value
}

func (c *htmlMarkdownConverter) table(node *html.Node, depth int) (string, error) {
	rows := make([]*html.Node, 0)
	var collect func(*html.Node)
	collect = func(current *html.Node) {
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && child.Data == "table" {
				continue
			}
			if child.Type == html.ElementNode && child.Data == "tr" {
				rows = append(rows, child)
				continue
			}
			collect(child)
		}
	}
	collect(node)
	if len(rows) == 0 {
		return "", nil
	}
	headerIndex := 0
	for i, row := range rows {
		if row.Parent != nil && row.Parent.Type == html.ElementNode && row.Parent.Data == "thead" {
			headerIndex = i
			break
		}
	}
	cells := make([][]string, len(rows))
	alignments := make([]string, 0)
	for i, row := range rows {
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type != html.ElementNode || (cell.Data != "th" && cell.Data != "td") {
				continue
			}
			content, err := c.inline(cell, depth+1)
			if err != nil {
				return "", err
			}
			content = strings.TrimSpace(content)
			content = strings.ReplaceAll(content, "\n", "<br>")
			cells[i] = append(cells[i], content)
			if i == headerIndex {
				alignments = append(alignments, tableAlignment(cell))
			}
		}
	}
	columns := len(cells[headerIndex])
	if columns == 0 {
		return "", nil
	}
	for i := range cells {
		if len(cells[i]) < columns {
			cells[i] = append(cells[i], make([]string, columns-len(cells[i]))...)
		}
		if len(cells[i]) > columns {
			cells[i] = cells[i][:columns]
		}
	}
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, markdownTableRow(cells[headerIndex]))
	delimiters := make([]string, columns)
	for i := range delimiters {
		switch alignments[i] {
		case "left":
			delimiters[i] = ":---"
		case "center":
			delimiters[i] = ":---:"
		case "right":
			delimiters[i] = "---:"
		default:
			delimiters[i] = "---"
		}
	}
	lines = append(lines, markdownTableRow(delimiters))
	for i, row := range cells {
		if i == headerIndex {
			continue
		}
		lines = append(lines, markdownTableRow(row))
	}
	return strings.Join(lines, "\n"), nil
}

func markdownTableRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func tableAlignment(node *html.Node) string {
	align := strings.ToLower(strings.TrimSpace(attribute(node, "align")))
	if align == "left" || align == "center" || align == "right" {
		return align
	}
	style := strings.ToLower(attribute(node, "style"))
	for _, declaration := range strings.Split(style, ";") {
		name, value, ok := strings.Cut(declaration, ":")
		if ok && strings.TrimSpace(name) == "text-align" {
			align = strings.TrimSpace(value)
			if align == "left" || align == "center" || align == "right" {
				return align
			}
		}
	}
	return ""
}

func nodeText(node *html.Node) string {
	var output strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			output.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return output.String()
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func hasAttribute(node *html.Node, name string) bool {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return true
		}
	}
	return false
}

func collapseHTMLWhitespace(value string) string {
	var output strings.Builder
	space := false
	for _, character := range value {
		if unicode.IsSpace(character) {
			space = true
			continue
		}
		if space {
			output.WriteByte(' ')
		}
		space = false
		output.WriteRune(character)
	}
	if space {
		output.WriteByte(' ')
	}
	return output.String()
}

func escapeMarkdownText(value string) string {
	const punctuation = "\\`*_{}[]()#+-.!|~<>"
	var output strings.Builder
	output.Grow(len(value))
	for _, character := range value {
		if strings.ContainsRune(punctuation, character) {
			output.WriteByte('\\')
		}
		output.WriteRune(character)
	}
	return output.String()
}

func wrapMarkdown(content, marker string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return marker + content + marker
}

func markdownCodeSpan(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	fence := strings.Repeat("`", longestRun(value, '`')+1)
	if fence == "" {
		fence = "`"
	}
	padding := strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") || (strings.HasPrefix(value, " ") && strings.HasSuffix(value, " ") && strings.TrimSpace(value) != "")
	if padding {
		value = " " + value + " "
	}
	return fence + value + fence
}

func markdownTitle(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return `"` + value + `"`
}

func longestRun(value string, marker rune) int {
	longest, run := 0, 0
	for _, character := range value {
		if character == marker {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return longest
}

func markdownDestination(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, " ", "%20")
	value = strings.ReplaceAll(value, "\t", "%09")
	value = strings.ReplaceAll(value, "\n", "%0A")
	value = strings.ReplaceAll(value, "<", "%3C")
	value = strings.ReplaceAll(value, ">", "%3E")
	return value
}
