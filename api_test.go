package markdown_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"go.osspkg.com/markdown"
)

func TestUnit_Converter_ConvertCommonMark(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic inline markdown",
			input:    "# Hello\n\nThis is **bold**, *em* and [a link](https://example.com).",
			expected: `<h1>Hello</h1><p>This is <strong>bold</strong>, <em>em</em> and <a href="https://example.com">a link</a>.</p>`,
		},
		{
			name:     "blocks",
			input:    "> quote\n\n- one\n- two\n\n```go\nfmt.Println(\"x\")\n```",
			expected: "<blockquote><p>quote</p></blockquote><ul><li>one</li><li>two</li></ul><pre><code class=\"language-go\">fmt.Println(&#34;x&#34;)\n</code></pre>",
		},
		{
			name:     "setext and breaks",
			input:    "Title\n=====\n\nline  \nbreak",
			expected: `<h1>Title</h1><p>line<br>break</p>`,
		},
		{
			name:     "reference link",
			input:    "[docs][ref]\n\n[ref]: https://example.com \"Example\"",
			expected: `<p><a href="https://example.com" title="Example">docs</a></p>`,
		},
		{
			name:     "balanced link destination",
			input:    "[docs](https://example.com/a_(b))",
			expected: `<p><a href="https://example.com/a_(b)">docs</a></p>`,
		},
		{
			name:     "indented code",
			input:    "    code <tag>\n    next",
			expected: "<pre><code>code &lt;tag&gt;\nnext\n</code></pre>",
		},
		{
			name:     "nested list",
			input:    "- outer\n  - inner",
			expected: "<ul><li>outer<ul><li>inner</li></ul></li></ul>",
		},
	}

	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, convertErr := converter.Convert(tt.input)
			if convertErr != nil {
				t.Fatalf("Convert() error = %v", convertErr)
			}
			if result.HTML != tt.expected {
				t.Errorf("Convert() = %q, want %q", result.HTML, tt.expected)
			}
			if len(result.Warnings) != 0 {
				t.Errorf("Convert() warnings = %v, want none", result.Warnings)
			}
		})
	}
}

func TestUnit_Converter_ConvertGFM(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert("| Name | Age |\n| :--- | ---: |\n| Ada | 36 |\n\n- [x] done\n- [ ] todo\n\n~~old~~ www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	expected := `<table><thead><tr><th align="left">Name</th><th align="right">Age</th></tr></thead><tbody><tr><td align="left">Ada</td><td align="right">36</td></tr></tbody></table><ul><li><input type="checkbox" disabled checked> done</li><li><input type="checkbox" disabled> todo</li></ul><p><del>old</del> <a href="http://www.example.com">www.example.com</a></p>`
	if result.HTML != expected {
		t.Errorf("Convert() = %q, want %q", result.HTML, expected)
	}
}

func TestUnit_Converter_GFMEdgeCases(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "mismatched table columns stay a paragraph",
			input:    "| A | B |\n| --- |\n| x |",
			expected: "<p>| A | B |\n| --- |\n| x |</p>",
		},
		{
			name:     "single tilde strikethrough",
			input:    "~old~",
			expected: "<p><del>old</del></p>",
		},
		{
			name:     "double tilde strikethrough",
			input:    "~~old~~",
			expected: "<p><del>old</del></p>",
		},
		{
			name:     "three tildes are literal",
			input:    "This will ~~~not~~~ strike.",
			expected: "<p>This will ~~~not~~~ strike.</p>",
		},
		{
			name:     "email autolink rejects plus in domain",
			input:    "hello@mail+xyz.example",
			expected: "<p>hello@mail+xyz.example</p>",
		},
		{
			name:     "protocol email autolinks",
			input:    "mailto:foo@bar.example xmpp:foo@bar.example",
			expected: `<p><a href="mailto:foo@bar.example">mailto:foo@bar.example</a> <a href="xmpp:foo@bar.example">xmpp:foo@bar.example</a></p>`,
		},
		{
			name:     "tab inside task marker is unchecked",
			input:    "- [\t] item",
			expected: `<ul><li><input type="checkbox" disabled> item</li></ul>`,
		},
		{
			name:     "NUL is replaced with replacement character",
			input:    "a\x00b",
			expected: "<p>a�b</p>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, convertErr := converter.Convert(tt.input)
			if convertErr != nil {
				t.Fatalf("Convert() error = %v", convertErr)
			}
			if result.HTML != tt.expected {
				t.Errorf("Convert(%q) = %q, want %q", tt.input, result.HTML, tt.expected)
			}
		})
	}
}

func TestUnit_Converter_Tokenize(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	result, err := converter.Tokenize("# title")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tokens) != 1 || result.Tokens[0].ID != markdown.TokenDocument {
		t.Fatalf("Tokens = %#v, want document root", result.Tokens)
	}
	children := result.Tokens[0].Children
	if len(children) != 1 || children[0].ID != markdown.TokenHeading {
		t.Fatalf("document children = %#v, want heading", children)
	}
	if children[0].Attributes[markdown.AttributeLevel] != "1" {
		t.Errorf("heading level = %q, want 1", children[0].Attributes[markdown.AttributeLevel])
	}
}

func TestUnit_Converter_NormalizesCRLFAndPreservesUnicode(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert("# Привет\r\n\r\n世界")
	if err != nil {
		t.Fatal(err)
	}
	if result.HTML != "<h1>Привет</h1><p>世界</p>" {
		t.Errorf("normalized Unicode HTML = %q", result.HTML)
	}
	tokens, err := converter.Tokenize("# Привет\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := tokens.Tokens[0].Span.End; got != len("# Привет\n") {
		t.Errorf("normalized root span end = %d, want %d", got, len("# Привет\n"))
	}
}

func TestUnit_Converter_CustomHandlerAndRule(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := converter.RegisterHandler(markdown.TokenText, func(_ *markdown.RenderContext, writer io.Writer, token markdown.Token) error {
		_, err := io.WriteString(writer, "[["+token.Literal+"]]")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	const customID markdown.TokenID = "mention"
	if err := converter.RegisterRule(markdown.Rule{
		ID:       customID,
		Phase:    markdown.RulePhaseInline,
		Priority: 100,
		Parse: func(_ *markdown.ParseContext, cursor markdown.Cursor) (markdown.Token, int, bool, error) {
			if !strings.HasPrefix(cursor.Remaining(), "@@") {
				return markdown.Token{}, 0, false, nil
			}
			return markdown.Token{ID: customID, Literal: "mention"}, 2, true, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := converter.RegisterHandler(customID, func(_ *markdown.RenderContext, writer io.Writer, token markdown.Token) error {
		_, err := io.WriteString(writer, `<mark>`+token.Literal+`</mark>`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	result, err := converter.Convert("hello @@ world")
	if err != nil {
		t.Fatal(err)
	}
	if result.HTML != `<p>[[hello ]]<mark>mention</mark>[[ world]]</p>` {
		t.Errorf("custom HTML = %q", result.HTML)
	}
}

func TestUnit_Converter_CustomBlockRule(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	const callout markdown.TokenID = "callout"
	if err := converter.RegisterRule(markdown.Rule{
		ID:       callout,
		Phase:    markdown.RulePhaseBlock,
		Priority: 100,
		Parse: func(_ *markdown.ParseContext, cursor markdown.Cursor) (markdown.Token, int, bool, error) {
			remaining := cursor.Remaining()
			if !strings.HasPrefix(remaining, "!!! ") {
				return markdown.Token{}, 0, false, nil
			}
			lineEnd := strings.IndexByte(remaining, '\n')
			consumed := len(remaining)
			literalEnd := len(remaining)
			if lineEnd >= 0 {
				consumed = lineEnd + 1
				literalEnd = lineEnd
			}
			return markdown.Token{Literal: strings.TrimSpace(remaining[:literalEnd])}, consumed, true, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := converter.RegisterHandler(callout, func(ctx *markdown.RenderContext, writer io.Writer, token markdown.Token) error {
		if _, err := io.WriteString(writer, "<aside>"); err != nil {
			return err
		}
		if err := ctx.WriteText(writer, token.Literal); err != nil {
			return err
		}
		_, err := io.WriteString(writer, "</aside>")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert("!!! note\n\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if result.HTML != "<aside>!!! note</aside><p>body</p>" {
		t.Errorf("custom block HTML = %q", result.HTML)
	}
}

func TestUnit_Converter_CustomRulePriorityAndProgress(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	const tokenID markdown.TokenID = "priority"
	for _, value := range []string{"first", "second"} {
		value := value
		if err := converter.RegisterRule(markdown.Rule{
			ID:       tokenID,
			Phase:    markdown.RulePhaseInline,
			Priority: 10,
			Parse: func(_ *markdown.ParseContext, cursor markdown.Cursor) (markdown.Token, int, bool, error) {
				if !strings.HasPrefix(cursor.Remaining(), "@@") {
					return markdown.Token{}, 0, false, nil
				}
				return markdown.Token{Literal: value}, 2, true, nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := converter.RegisterHandler(tokenID, func(_ *markdown.RenderContext, writer io.Writer, token markdown.Token) error {
		_, err := io.WriteString(writer, token.Literal)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert("@@")
	if err != nil {
		t.Fatal(err)
	}
	if result.HTML != `<p>second</p>` {
		t.Errorf("priority HTML = %q, want second rule", result.HTML)
	}

	broken, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := broken.RegisterRule(markdown.Rule{
		ID:       "broken",
		Phase:    markdown.RulePhaseInline,
		Priority: 100,
		Parse: func(_ *markdown.ParseContext, _ markdown.Cursor) (markdown.Token, int, bool, error) {
			return markdown.Token{}, 0, true, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.Convert("text"); !errors.Is(err, markdown.ErrRuleProgress) {
		t.Errorf("invalid rule error = %v, want ErrRuleProgress", err)
	}
}

func TestUnit_Converter_HandlerOverrideLastRegistrationWins(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first", "second"} {
		value := value
		if err := converter.RegisterHandler(markdown.TokenText, func(_ *markdown.RenderContext, writer io.Writer, _ markdown.Token) error {
			_, err := io.WriteString(writer, value)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := converter.Convert("text")
	if err != nil {
		t.Fatal(err)
	}
	if result.HTML != `<p>second</p>` {
		t.Errorf("HTML = %q, want last handler output", result.HTML)
	}
	if err := converter.RegisterHandler(markdown.TokenText, func(_ *markdown.RenderContext, _ io.Writer, _ markdown.Token) error { return nil }); !errors.Is(err, markdown.ErrFrozen) {
		t.Errorf("register after use error = %v, want ErrFrozen", err)
	}
}

func TestUnit_Converter_SecurityDefaults(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert(`<script>alert(1)</script>\n\n[x](javascript:alert(1)) &lt;b&gt;`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.HTML, "<script>") || strings.Contains(result.HTML, `href="javascript:`) {
		t.Errorf("unsafe HTML escaped/URL rejected: %q", result.HTML)
	}
	if !strings.Contains(result.HTML, "&lt;script&gt;") || !strings.Contains(result.HTML, "&lt;b&gt;") {
		t.Errorf("HTML escaping missing: %q", result.HTML)
	}
	for _, scheme := range []string{"javascript", "data", "vbscript"} {
		result, err := converter.Convert("[blocked](" + scheme + ":alert(1))")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(result.HTML, `href="`+scheme+`:`) {
			t.Errorf("unsafe %s URL reached HTML: %q", scheme, result.HTML)
		}
	}
	encoded, err := converter.Convert(`[blocked](java&#x73;cript:alert(1))`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded.HTML, `href="javascript:`) {
		t.Errorf("encoded unsafe URL reached HTML: %q", encoded.HTML)
	}

	unsafe, err := markdown.New(markdown.WithUnsafeRawHTML())
	if err != nil {
		t.Fatal(err)
	}
	unsafeResult, err := unsafe.Convert("<b>trusted</b>")
	if err != nil {
		t.Fatal(err)
	}
	if unsafeResult.HTML != "<b>trusted</b>" {
		t.Errorf("unsafe HTML = %q", unsafeResult.HTML)
	}
	disallowedResult, err := unsafe.Convert("<SCRIPT>alert(1)</SCRIPT> <scripture>allowed</scripture>")
	if err != nil {
		t.Fatal(err)
	}
	expectedUnsafeHTML := "&lt;SCRIPT>alert(1)&lt;/SCRIPT> <scripture>allowed</scripture>"
	if disallowedResult.HTML != expectedUnsafeHTML {
		t.Errorf("unsafe HTML filter = %q, want %q", disallowedResult.HTML, expectedUnsafeHTML)
	}
	inlineDisallowed, err := unsafe.Convert("before <script>alert(1)</script> after")
	if err != nil {
		t.Fatal(err)
	}
	expectedInlineHTML := "<p>before &lt;script>alert(1)&lt;/script> after</p>"
	if inlineDisallowed.HTML != expectedInlineHTML {
		t.Errorf("unsafe inline HTML filter = %q, want %q", inlineDisallowed.HTML, expectedInlineHTML)
	}
	invalidEmail, err := unsafe.Convert("<foo@mail+xyz.example>")
	if err != nil {
		t.Fatal(err)
	}
	if invalidEmail.HTML != "<p>&lt;foo@mail+xyz.example&gt;</p>" {
		t.Errorf("invalid angle email = %q, want escaped text", invalidEmail.HTML)
	}
}

func TestUnit_Converter_BestEffortWarnings(t *testing.T) {
	converter, err := markdown.New(markdown.WithErrorMode(markdown.ErrorModeBestEffort))
	if err != nil {
		t.Fatal(err)
	}
	handlerErr := errors.New("handler failed")
	if err := converter.RegisterHandler(markdown.TokenText, func(_ *markdown.RenderContext, _ io.Writer, _ markdown.Token) error {
		return handlerErr
	}); err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert("safe text")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !errors.Is(result.Warnings[0], handlerErr) {
		t.Errorf("warnings = %v, want handler error", result.Warnings)
	}
	if result.HTML != `<p>safe text</p>` {
		t.Errorf("best effort HTML = %q", result.HTML)
	}

	limited, err := markdown.New(
		markdown.WithErrorMode(markdown.ErrorModeBestEffort),
		markdown.WithMaxNestingDepth(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	limitedResult, err := limited.Convert("[deep](https://example.com)")
	if err != nil {
		t.Fatal(err)
	}
	if len(limitedResult.Warnings) != 1 || !errors.Is(limitedResult.Warnings[0], markdown.ErrLimitExceeded) {
		t.Errorf("parser warnings = %v, want ErrLimitExceeded", limitedResult.Warnings)
	}
	if limitedResult.HTML != "[deep](https://example.com)" {
		t.Errorf("best effort parser fallback = %q", limitedResult.HTML)
	}
}

func TestUnit_Converter_InputAndLimits(t *testing.T) {
	if _, err := markdown.New(markdown.WithMaxInputBytes(1)); err != nil {
		t.Fatal(err)
	}
	converter, err := markdown.New(markdown.WithMaxInputBytes(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := converter.Convert("too long"); !errors.Is(err, markdown.ErrLimitExceeded) {
		t.Errorf("limit error = %v, want ErrLimitExceeded", err)
	}
	invalid := string([]byte{0xff})
	if _, err := converter.Convert(invalid); !errors.Is(err, markdown.ErrInvalidUTF8) {
		t.Errorf("utf-8 error = %v, want ErrInvalidUTF8", err)
	}
}

func TestUnit_Converter_PrettyAndShortWriter(t *testing.T) {
	converter, err := markdown.New(markdown.WithHTMLFormat(markdown.HTMLFormatPretty))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := converter.Tokenize("# title")
	if err != nil {
		t.Fatal(err)
	}
	writer := &shortWriter{limit: 2}
	if _, err := converter.Render(writer, tokens.Tokens); !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("Render() error = %v, want io.ErrShortWrite", err)
	}
	result, err := converter.Convert("# title\n\ntext")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.HTML, "\n") || !strings.Contains(result.HTML, "<h1>") {
		t.Errorf("pretty HTML = %q", result.HTML)
	}
}

func TestUnit_Converter_ConcurrentUse(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := converter.Convert("freeze"); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			result, err := converter.Convert("**parallel**")
			if err != nil || result.HTML != `<p><strong>parallel</strong></p>` {
				t.Errorf("concurrent Convert() = %q, error = %v", result.HTML, err)
			}
		}()
	}
	group.Wait()
}

type shortWriter struct {
	limit int
}

func (w *shortWriter) Write(value []byte) (int, error) {
	if len(value) > w.limit {
		return w.limit, nil
	}
	return len(value), nil
}
