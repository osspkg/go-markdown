package markdown

import (
	"unicode/utf8"
)

// Cursor is a read-only view of the parser input at one byte offset. A custom
// rule reports how many bytes it consumed instead of mutating the cursor.
type Cursor struct {
	source string
	offset int
	end    int
}

// Position returns the current zero-based byte offset.
func (c Cursor) Position() int {
	return c.offset
}

// Remaining returns the unconsumed input in the cursor range.
func (c Cursor) Remaining() string {
	return c.source[c.offset:c.end]
}

// AtEnd reports whether the cursor has no remaining input.
func (c Cursor) AtEnd() bool {
	return c.offset >= c.end
}

// Peek returns the next rune without advancing the cursor.
func (c Cursor) Peek() (rune, bool) {
	if c.AtEnd() {
		return 0, false
	}

	r, _ := utf8.DecodeRuneInString(c.source[c.offset:c.end])
	return r, true
}

// Slice returns the first size bytes without advancing the cursor. It fails
// when size crosses the cursor boundary or a UTF-8 rune boundary.
func (c Cursor) Slice(size int) (string, bool) {
	if size < 0 || size > c.end-c.offset {
		return "", false
	}
	if size > 0 && !utf8.ValidString(c.source[c.offset:c.offset+size]) {
		return "", false
	}

	return c.source[c.offset : c.offset+size], true
}

// Source returns the complete normalized source visible to a custom rule.
func (c Cursor) Source() string {
	return c.source
}

// ParseContext exposes source-aware recursive parsing helpers to custom rules.
type ParseContext struct {
	source string
	parser *parser
	phase  RulePhase
}

// Source returns the normalized UTF-8 source.
func (c *ParseContext) Source() string {
	if c == nil {
		return ""
	}

	return c.source
}

// Phase returns the phase in which the current rule is running.
func (c *ParseContext) Phase() RulePhase {
	if c == nil {
		return RulePhaseUnknown
	}

	return c.phase
}

// ParseInline parses a source range as inline Markdown. The range must be a
// valid half-open byte range in the normalized source.
func (c *ParseContext) ParseInline(start, end int) ([]Token, error) {
	if c == nil || c.parser == nil {
		return []Token{}, nil
	}

	return c.parser.parseInlineRange(start, end, 0)
}

// ParseBlocks parses a source range as block Markdown. The range must be a
// valid half-open byte range in the normalized source.
func (c *ParseContext) ParseBlocks(start, end int) ([]Token, error) {
	if c == nil || c.parser == nil {
		return []Token{}, nil
	}

	return c.parser.parseBlockRange(start, end, 0)
}
