// Package markdown tokenizes CommonMark and GFM documents and renders them as
// HTML through replaceable token handlers.
package markdown

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
)

// TokenID identifies a node in the token tree.
type TokenID string

// Built-in token identifiers are stable names used by the default parser and
// renderer. Applications may define their own identifiers.
const (
	TokenDocument      TokenID = "document"
	TokenParagraph     TokenID = "paragraph"
	TokenHeading       TokenID = "heading"
	TokenBlockquote    TokenID = "blockquote"
	TokenList          TokenID = "list"
	TokenListItem      TokenID = "list_item"
	TokenCodeBlock     TokenID = "code_block"
	TokenCodeSpan      TokenID = "code_span"
	TokenThematicRule  TokenID = "thematic_rule"
	TokenText          TokenID = "text"
	TokenEmphasis      TokenID = "emphasis"
	TokenStrong        TokenID = "strong"
	TokenStrikethrough TokenID = "strikethrough"
	TokenLink          TokenID = "link"
	TokenImage         TokenID = "image"
	TokenAutolink      TokenID = "autolink"
	TokenSoftbreak     TokenID = "softbreak"
	TokenHardbreak     TokenID = "hardbreak"
	TokenHTMLBlock     TokenID = "html_block"
	TokenHTMLInline    TokenID = "html_inline"
	TokenTable         TokenID = "table"
	TokenTableHead     TokenID = "table_head"
	TokenTableBody     TokenID = "table_body"
	TokenTableRow      TokenID = "table_row"
	TokenTableCell     TokenID = "table_cell"
	TokenTaskCheckbox  TokenID = "task_checkbox"
)

// Attribute names used by built-in tokens.
const (
	AttributeDestination = "destination"
	AttributeTitle       = "title"
	AttributeLevel       = "level"
	AttributeOrdered     = "ordered"
	AttributeStart       = "start"
	AttributeLanguage    = "language"
	AttributeAlignment   = "alignment"
	AttributeHeader      = "header"
	AttributeChecked     = "checked"
	AttributeTight       = "tight"
)

// Span identifies a half-open byte range in the normalized UTF-8 source.
type Span struct {
	Start int
	End   int
}

// Token is one node in the hierarchical Markdown token tree. Attributes and
// Children are initialized to empty collections by the tokenizer. Handlers
// must treat the token as read-only.
type Token struct {
	ID         TokenID
	Literal    string
	Attributes map[string]string
	Children   []Token
	Span       Span
}

// RulePhase determines whether a custom rule runs at block or inline level.
type RulePhase uint8

const (
	// RulePhaseUnknown is the zero value for an unspecified rule phase.
	RulePhaseUnknown RulePhase = iota
	// RulePhaseBlock identifies block-level parser rules.
	RulePhaseBlock
	// RulePhaseInline identifies inline-level parser rules.
	RulePhaseInline
	// RulePhaseRender identifies renderer diagnostics.
	RulePhaseRender
)

// ErrorMode controls how parser and handler failures are reported.
type ErrorMode uint8

const (
	// ErrorModeUnknown is the zero value for an unspecified error mode.
	ErrorModeUnknown ErrorMode = iota
	// ErrorModeFailFast stops at the first fatal parser or renderer error.
	ErrorModeFailFast
	// ErrorModeBestEffort records recoverable errors as warnings.
	ErrorModeBestEffort
)

// HTMLFormat selects the formatting of built-in HTML handlers.
type HTMLFormat uint8

const (
	// HTMLFormatUnknown is the zero value for an unspecified HTML format.
	HTMLFormatUnknown HTMLFormat = iota
	// HTMLFormatCompact emits minimal deterministic HTML.
	HTMLFormatCompact
	// HTMLFormatPretty emits deterministic indented HTML.
	HTMLFormatPretty
)

var (
	// ErrFrozen indicates that a converter was modified after first use.
	ErrFrozen = errors.New("markdown: converter is frozen")
	// ErrInvalidUTF8 indicates that the source is not valid UTF-8.
	ErrInvalidUTF8 = errors.New("markdown: source is not valid utf-8")
	// ErrLimitExceeded indicates that a configured resource limit was reached.
	ErrLimitExceeded = errors.New("markdown: configured limit exceeded")
	// ErrRuleProgress indicates that a custom rule returned an invalid length.
	ErrRuleProgress = errors.New("markdown: custom rule made invalid progress")
	// ErrUnknownToken indicates that no handler exists for a token identifier.
	ErrUnknownToken = errors.New("markdown: no handler for token")
)

// Diagnostic carries a non-fatal conversion issue with its source location.
type Diagnostic struct {
	Err     error
	Phase   RulePhase
	TokenID TokenID
	Span    Span
}

// Error implements error.
func (d Diagnostic) Error() string {
	if d.Err == nil {
		return "markdown: conversion warning"
	}

	return d.Err.Error()
}

// Unwrap exposes the underlying conversion issue.
func (d Diagnostic) Unwrap() error {
	return d.Err
}

// ParseError adds parser phase, token, and source location to an error.
type ParseError struct {
	Err    error
	Phase  RulePhase
	RuleID TokenID
	Span   Span
}

// Error implements error.
func (e *ParseError) Error() string {
	if e == nil {
		return "markdown: parse error"
	}

	return fmt.Sprintf(
		"markdown: %s rule %q at bytes %d:%d: %v",
		phaseName(e.Phase),
		e.RuleID,
		e.Span.Start,
		e.Span.End,
		e.Err,
	)
}

// Unwrap exposes the underlying parser error.
func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

// Rule is an extensible Markdown parser rule. Parse must return matched=false
// and consumed=0 when the current cursor does not contain the rule syntax.
// When matched is true, consumed must be positive and end on a UTF-8 boundary.
type Rule struct {
	ID       TokenID
	Phase    RulePhase
	Priority int
	Parse    func(*ParseContext, Cursor) (Token, int, bool, error)
}

// Handler renders one token to the supplied writer. RenderContext provides
// escaping and recursive child rendering helpers.
type Handler func(*RenderContext, io.Writer, Token) error

// TokenizationResult contains a complete token tree and non-fatal warnings.
type TokenizationResult struct {
	Tokens   []Token
	Warnings []Diagnostic
}

// ConversionResult contains the rendered HTML and non-fatal warnings.
type ConversionResult struct {
	HTML     string
	Warnings []Diagnostic
}

type config struct {
	errorMode       ErrorMode
	htmlFormat      HTMLFormat
	unsafeRawHTML   bool
	maxInputBytes   int
	maxNestingDepth int
	maxTokens       int
}

// Option configures a Converter during construction.
type Option func(*config) error

// WithErrorMode selects fail-fast or best-effort conversion.
func WithErrorMode(mode ErrorMode) Option {
	return func(cfg *config) error {
		if mode != ErrorModeFailFast && mode != ErrorModeBestEffort {
			return fmt.Errorf("markdown: invalid error mode %d", mode)
		}

		cfg.errorMode = mode
		return nil
	}
}

// WithHTMLFormat selects compact or pretty built-in HTML output.
func WithHTMLFormat(format HTMLFormat) Option {
	return func(cfg *config) error {
		if format != HTMLFormatCompact && format != HTMLFormatPretty {
			return fmt.Errorf("markdown: invalid html format %d", format)
		}

		cfg.htmlFormat = format
		return nil
	}
}

// WithUnsafeRawHTML allows raw HTML blocks and inline HTML to pass through.
// User-provided handlers are trusted code regardless of this option.
func WithUnsafeRawHTML() Option {
	return func(cfg *config) error {
		cfg.unsafeRawHTML = true
		return nil
	}
}

// WithMaxInputBytes limits the normalized source size. A non-positive value
// disables this limit.
func WithMaxInputBytes(limit int) Option {
	return func(cfg *config) error {
		if limit < 0 {
			return fmt.Errorf("markdown: invalid input limit %d", limit)
		}

		cfg.maxInputBytes = limit
		return nil
	}
}

// WithMaxNestingDepth limits recursive block, inline, and rendering depth.
func WithMaxNestingDepth(limit int) Option {
	return func(cfg *config) error {
		if limit <= 0 {
			return fmt.Errorf("markdown: invalid nesting limit %d", limit)
		}

		cfg.maxNestingDepth = limit
		return nil
	}
}

// WithMaxTokens limits the number of tokens created by one tokenization.
func WithMaxTokens(limit int) Option {
	return func(cfg *config) error {
		if limit <= 0 {
			return fmt.Errorf("markdown: invalid token limit %d", limit)
		}

		cfg.maxTokens = limit
		return nil
	}
}

type registeredRule struct {
	rule  Rule
	order uint64
}

type converterSnapshot struct {
	cfg      config
	handlers map[TokenID]Handler
	rules    []registeredRule
}

// Converter tokenizes Markdown and renders its tokens through registered
// handlers. Configure a Converter before its first operation; after that it
// may be used concurrently.
type Converter struct {
	mu       sync.RWMutex
	cfg      config
	handlers map[TokenID]Handler
	rules    []registeredRule
	nextRule uint64
	used     bool
}

// New creates a configured Converter with the built-in parser and HTML
// handlers. The default mode is fail-fast with compact HTML and bounded input.
func New(opts ...Option) (*Converter, error) {
	cfg := config{
		errorMode:       ErrorModeFailFast,
		htmlFormat:      HTMLFormatCompact,
		maxInputBytes:   16 << 20,
		maxNestingDepth: 128,
		maxTokens:       1_000_000,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("markdown: nil option")
		}
		if err := opt(&cfg); err != nil {
			return nil, fmt.Errorf("markdown: configure converter: %w", err)
		}
	}

	return &Converter{
		cfg:      cfg,
		handlers: defaultHandlers(),
		rules:    []registeredRule{},
	}, nil
}

// RegisterHandler installs or replaces the handler for id. The most recent
// registration wins, provided the converter has not been frozen.
func (c *Converter) RegisterHandler(id TokenID, handler Handler) error {
	if c == nil {
		return errors.New("markdown: nil converter")
	}
	if id == "" {
		return errors.New("markdown: empty token id")
	}
	if handler == nil {
		return fmt.Errorf("markdown: nil handler for token %q", id)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used {
		return ErrFrozen
	}
	if c.handlers == nil {
		return errors.New("markdown: converter is not initialized; use New")
	}
	c.handlers[id] = handler
	return nil
}

// RegisterRule adds a block or inline parser rule. Rules with higher priority
// run first; equal priorities use the most recently registered rule first.
func (c *Converter) RegisterRule(rule Rule) error {
	if c == nil {
		return errors.New("markdown: nil converter")
	}
	if rule.ID == "" {
		return errors.New("markdown: empty rule token id")
	}
	if rule.Phase != RulePhaseBlock && rule.Phase != RulePhaseInline {
		return fmt.Errorf("markdown: invalid rule phase %d", rule.Phase)
	}
	if rule.Parse == nil {
		return fmt.Errorf("markdown: nil parser for rule %q", rule.ID)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used {
		return ErrFrozen
	}
	if c.handlers == nil {
		return errors.New("markdown: converter is not initialized; use New")
	}
	c.nextRule++
	c.rules = append(c.rules, registeredRule{rule: rule, order: c.nextRule})
	return nil
}

func (c *Converter) snapshot() (converterSnapshot, error) {
	if c == nil {
		return converterSnapshot{}, errors.New("markdown: nil converter")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handlers == nil {
		return converterSnapshot{}, errors.New("markdown: converter is not initialized; use New")
	}
	c.used = true

	handlers := make(map[TokenID]Handler, len(c.handlers))
	for id, handler := range c.handlers {
		handlers[id] = handler
	}
	rules := append([]registeredRule{}, c.rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].rule.Phase != rules[j].rule.Phase {
			return rules[i].rule.Phase < rules[j].rule.Phase
		}
		if rules[i].rule.Priority != rules[j].rule.Priority {
			return rules[i].rule.Priority > rules[j].rule.Priority
		}
		return rules[i].order > rules[j].order
	})

	return converterSnapshot{
		cfg:      c.cfg,
		handlers: handlers,
		rules:    rules,
	}, nil
}

func phaseName(phase RulePhase) string {
	switch phase {
	case RulePhaseBlock:
		return "block"
	case RulePhaseInline:
		return "inline"
	case RulePhaseRender:
		return "render"
	default:
		return "unknown"
	}
}

func wrapDiagnostic(err error, phase RulePhase, id TokenID, span Span) Diagnostic {
	return Diagnostic{
		Err:     err,
		Phase:   phase,
		TokenID: id,
		Span:    span,
	}
}

func isBestEffort(cfg config) bool {
	return cfg.errorMode == ErrorModeBestEffort
}

func sourceSpan(source string, span Span) string {
	start := span.Start
	end := span.End
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(source) {
		start = len(source)
	}
	if end > len(source) {
		end = len(source)
	}
	return source[start:end]
}

func normalizeAttributes(attributes map[string]string) map[string]string {
	if attributes == nil {
		return map[string]string{}
	}

	return attributes
}

func makeToken(id TokenID, literal string, attributes map[string]string, children []Token, span Span) Token {
	if children == nil {
		children = []Token{}
	}

	return Token{
		ID:         id,
		Literal:    literal,
		Attributes: normalizeAttributes(attributes),
		Children:   children,
		Span:       span,
	}
}

func boolAttribute(value bool) string {
	if value {
		return "true"
	}

	return "false"
}
