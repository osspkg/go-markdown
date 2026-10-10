package markdown_test

import (
	"errors"
	"testing"

	"go.osspkg.com/markdown"
)

func TestConverter_ConvertHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "inline elements and entities",
			input:    `<h1>Guide</h1><p>Hello <strong>world</strong>, <em>Go</em> &amp; <del>old</del>.</p>`,
			expected: "# Guide\n\nHello **world**, *Go* & ~~old~~\\.",
		},
		{
			name:     "links images and breaks",
			input:    `<p><a href="https://example.com/a b" title="A &quot;quote&quot;">docs</a><br><img src="image.png" alt="diagram" title="Diagram"></p>`,
			expected: "[docs](<https://example.com/a%20b> \"A &quot;quote&quot;\")  \n![diagram](<image.png> \"Diagram\")",
		},
		{
			name:     "frontmatter from full document",
			input:    `<!doctype html><html><head><title>Guide &amp; API</title><meta name="description" content="A description"><script>ignore head script</script></head><body><p>Body</p></body></html>`,
			expected: "---\ntitle: \"Guide & API\"\ndescription: \"A description\"\n---\n\nBody",
		},
		{
			name:     "frontmatter escapes quotes and newlines",
			input:    `<head><title>A &quot;title&quot;</title><meta name="description" content="first&#10;second"></head><p>Text</p>`,
			expected: "---\ntitle: \"A \\\"title\\\"\"\ndescription: \"first\\nsecond\"\n---\n\nText",
		},
		{
			name:     "duplicate metadata uses first value",
			input:    `<head><title>First</title><title>Second</title><meta name="description" content="One"><meta name="description" content="Two"></head><p>Text</p>`,
			expected: "---\ntitle: \"First\"\ndescription: \"One\"\n---\n\nText",
		},
		{
			name:     "nested task lists",
			input:    `<ul><li><input type="checkbox" checked>done</li><li><input type="checkbox">todo<ul><li>nested</li></ul></li></ul>`,
			expected: "- [x] done\n- [ ] todo\n\n      - nested",
		},
		{
			name:     "blockquote ordered list and thematic break",
			input:    `<blockquote><p>quote</p><ul><li>one</li><li>two</li></ul></blockquote><ol start="3"><li>three</li><li>four</li></ol><hr>`,
			expected: "> quote\n> \n> - one\n> - two\n\n3. three\n4. four\n\n---",
		},
		{
			name:     "table alignment and pipe escaping",
			input:    `<table><thead><tr><th align="left">Name</th><th style="text-align: center">Age</th></tr></thead><tbody><tr><td>A|B</td><td><code>36</code></td></tr></tbody></table>`,
			expected: "| Name | Age |\n| :--- | :---: |\n| A\\|B | `36` |",
		},
		{
			name:     "code block fence and language",
			input:    "<pre><code class=\"language-go\">fmt.Println(\"hello\")\n</code></pre>",
			expected: "```go\nfmt.Println(\"hello\")\n```",
		},
		{
			name:     "unknown tags keep content and active content is skipped",
			input:    `<custom>visible <b>bold</b><script>alert(1)</script><style>.secret{}</style></custom><!-- comment -->`,
			expected: "visible **bold**",
		},
		{
			name:     "unsafe link and image URLs become plain text",
			input:    `<p><a href="javascript:alert(1)">unsafe</a> <img src="data:image/png;base64,abc" alt="image"></p>`,
			expected: "unsafe image",
		},
		{
			name:     "HTML5 parser repairs unclosed paragraphs",
			input:    `<p>first<p>second`,
			expected: "first\n\nsecond",
		},
		{
			name:     "empty head does not emit frontmatter",
			input:    `<html><head><meta name="keywords" content="x"></head><body><p>Text</p></body></html>`,
			expected: "Text",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			converter, err := markdown.New()
			if err != nil {
				t.Fatal(err)
			}
			got, err := converter.ConvertHTML(test.input)
			if err != nil {
				t.Fatalf("ConvertHTML() error = %v", err)
			}
			if got != test.expected {
				t.Errorf("ConvertHTML() = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestConverter_ConvertHTMLLimits(t *testing.T) {
	t.Run("invalid UTF-8", func(t *testing.T) {
		converter, err := markdown.New()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := converter.ConvertHTML(string([]byte{0xff})); !errors.Is(err, markdown.ErrInvalidUTF8) {
			t.Errorf("ConvertHTML() error = %v, want ErrInvalidUTF8", err)
		}
	})

	t.Run("input bytes", func(t *testing.T) {
		converter, err := markdown.New(markdown.WithMaxInputBytes(3))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := converter.ConvertHTML("<p>x</p>"); !errors.Is(err, markdown.ErrLimitExceeded) {
			t.Errorf("ConvertHTML() error = %v, want ErrLimitExceeded", err)
		}
	})

	t.Run("nesting depth", func(t *testing.T) {
		converter, err := markdown.New(markdown.WithMaxNestingDepth(2))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := converter.ConvertHTML("<p>x</p>"); !errors.Is(err, markdown.ErrLimitExceeded) {
			t.Errorf("ConvertHTML() error = %v, want ErrLimitExceeded", err)
		}
	})

	t.Run("node count", func(t *testing.T) {
		converter, err := markdown.New(markdown.WithMaxTokens(3))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := converter.ConvertHTML("<p>x</p>"); !errors.Is(err, markdown.ErrLimitExceeded) {
			t.Errorf("ConvertHTML() error = %v, want ErrLimitExceeded", err)
		}
	})
}

func TestConverter_ConvertHTMLMarkdownRoundTrip(t *testing.T) {
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	markdownText, err := converter.ConvertHTML(`<p>Hello <strong>world</strong>! <a href="https://example.com/?a=1&amp;b=2" title="A &quot;quote&quot;">docs</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := converter.Convert(markdownText)
	if err != nil {
		t.Fatalf("Convert() round trip error = %v; markdown = %q", err, markdownText)
	}
	expected := `<p>Hello <strong>world</strong>! <a href="https://example.com/?a=1&amp;b=2" title="A &#34;quote&#34;">docs</a></p>`
	if result.HTML != expected {
		t.Errorf("Convert(ConvertHTML()) = %q, want %q; markdown = %q", result.HTML, expected, markdownText)
	}
}
