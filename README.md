# go.osspkg.com/markdown

`go.osspkg.com/markdown` is a Go library that parses Markdown into a
hierarchical token tree and renders it as HTML. It includes CommonMark-style
block and inline syntax, selected GitHub Flavored Markdown (GFM) features, and
extension points for custom parser rules and token handlers.

See the [API reference](API.md) for the available methods, token IDs, and
attributes.

## Requirements

- Go 1.26 or later

## Installation

```sh
go get go.osspkg.com/markdown
```

## Quick start

```go
package main

import (
	"fmt"
	"log"

	"go.osspkg.com/markdown"
)

func main() {
	converter, err := markdown.New()
	if err != nil {
		log.Fatal(err)
	}

	result, err := converter.Convert("# Hello\n\nMarkdown **content**")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.HTML)
	// <h1>Hello</h1><p>Markdown <strong>content</strong></p>
}
```

## Features

- Convert Markdown directly to HTML with `Converter.Convert`.
- Parse Markdown into tokens with `Converter.Tokenize`, or render a token tree
  with `Converter.Render`.
- Use headings, paragraphs, blockquotes, ordered and unordered lists, fenced
  and indented code blocks, links, images, emphasis, code spans, and thematic
  breaks.
- Use GFM-style tables, strikethrough, task-list checkboxes, and automatic
  links for URL and email text.
- Add block or inline parser rules and replace rendering handlers before the
  converter's first operation.
- Choose compact or deterministic indented HTML output and fail-fast or
  best-effort error handling.
- Set input, nesting-depth, and token-count limits.

This package implements the syntax covered by its parser and tests; it does
not claim complete conformance with every CommonMark or GFM edge case.

## Customize rendering

Handlers are registered by `TokenID`. A handler receives a `RenderContext`, an
`io.Writer`, and the token being rendered. Use the context helpers to render
children and escape text and attribute values. The example assumes `converter`
was created as in the quick start and that `io` is imported:

```go
err := converter.RegisterHandler(markdown.TokenStrong,
	func(ctx *markdown.RenderContext, w io.Writer, token markdown.Token) error {
		if _, err := io.WriteString(w, "<mark>"); err != nil {
			return err
		}
		if err := ctx.RenderChildren(w, token); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</mark>")
		return err
	})
```

The latest handler registered for a token ID replaces the previous handler.
Register handlers and rules before calling `Tokenize`, `Render`, or `Convert`;
the converter becomes immutable after its first operation and can then be used
concurrently.

## Add a parser rule

Rules run in either the block or inline phase. Higher priorities run first;
rules with equal priorities run in reverse registration order. A matched rule
must consume a positive number of UTF-8 bytes. Block rules must consume whole
lines. This example assumes `converter` was created as in the quick start and
that `strings` is imported:

```go
const mention markdown.TokenID = "mention"

err := converter.RegisterRule(markdown.Rule{
	ID:       mention,
	Phase:    markdown.RulePhaseInline,
	Priority: 100,
	Parse: func(_ *markdown.ParseContext, cursor markdown.Cursor) (markdown.Token, int, bool, error) {
		if !strings.HasPrefix(cursor.Remaining(), "@@") {
			return markdown.Token{}, 0, false, nil
		}
		return markdown.Token{ID: mention, Literal: "mention"}, 2, true, nil
	},
})
```

Custom token IDs also need a registered handler before the converter is used.

## Security

The built-in renderer escapes text and HTML attributes. Raw HTML input is
escaped by default; `markdown.WithUnsafeRawHTML()` allows raw HTML through,
while filtering GFM-disallowed tags such as `<script>` and `<style>`.
Built-in links and images accept only `http`, `https`, `mailto`, `tel`, `xmpp`,
relative, and fragment URLs. Custom handlers write HTML directly and are
trusted code; use `RenderContext.WriteText`, `WriteAttribute`, and `SafeURL`
when handling untrusted values.

By default, input is limited to 16 MiB, nesting depth to 128, and token count
to 1,000,000. Configure these limits with `WithMaxInputBytes`,
`WithMaxNestingDepth`, and `WithMaxTokens`. A non-positive input-byte limit
disables that limit; nesting and token limits must be positive.

## Configuration

```go
converter, err := markdown.New(
	markdown.WithHTMLFormat(markdown.HTMLFormatPretty),
	markdown.WithErrorMode(markdown.ErrorModeBestEffort),
	markdown.WithMaxInputBytes(4<<20),
)
```

`ErrorModeFailFast` and `HTMLFormatCompact` are the defaults. Best-effort mode
returns recoverable parser and renderer problems in the result's `Warnings`.
`Convert` returns a `ConversionResult`; `Tokenize` returns a
`TokenizationResult`; `Render` returns warnings and an error.

## Development

Run these commands from the repository root:

```sh
go test ./...       # unit, example, and golden tests
make lint           # goppy lint
make tests          # tests, benchmarks, and two 10-second fuzzing runs
make build          # goppy build --arch=amd64
make ci             # install/setup, license, lint, tests, and build
```

`make install` installs `goppy` at `latest` and runs `goppy setup-lib`.
`make ci` includes this setup step and may download tooling. The test target
also runs benchmarks and fuzzers, so `go test ./...` is the focused check for
ordinary code changes.

The CI workflow runs `make ci` with Go 1.26. See
[`.github/workflows/ci.yml`](.github/workflows/ci.yml) and the
[Makefile](Makefile) for the maintained command definitions.
