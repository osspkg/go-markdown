# go-markdown

[![Go Reference](https://pkg.go.dev/badge/go.osspkg.com/markdown.svg)](https://pkg.go.dev/go.osspkg.com/markdown)
[![Go Version](https://img.shields.io/github/go-mod/go-version/osspkg/go-markdown)](https://go.dev/)
[![CI](https://github.com/osspkg/go-markdown/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/osspkg/go-markdown/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/osspkg/go-markdown)](LICENSE)

`go.osspkg.com/markdown` is the import path for this library.

## Contents

- [Overview](#overview)
- [Requirements and installation](#requirements-and-installation)
- [Quick start](#quick-start)
- [Features](#features)
- [HTML to Markdown](#html-to-markdown)
- [Customize rendering](#customize-rendering)
- [Add a parser rule](#add-a-parser-rule)
- [Security](#security)
- [Configuration](#configuration)
- [Development](#development)
- [Contributing](#contributing)
- [Contributors](#contributors)
- [License](#license)

## Overview

This Go library parses Markdown into a
hierarchical token tree and renders it as HTML. It includes CommonMark-style
block and inline syntax, selected GitHub Flavored Markdown (GFM) features, and
extension points for custom parser rules and renderers in both directions.

See the [API reference](API.md) for the available methods, token IDs, and
attributes.

## Requirements and installation

Requires Go 1.26 or later.

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

	markdownText, err := converter.ConvertHTML(`<h1>Hello</h1><p><strong>HTML</strong> content</p>`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(markdownText)
	// # Hello
	//
	// **HTML** content
}
```

## Features

- Convert Markdown directly to HTML with `Converter.Convert`.
- Convert HTML documents or fragments to GFM Markdown with
  `Converter.ConvertHTML`, including optional YAML frontmatter from the page
  title and description.
- Parse Markdown into tokens with `Converter.Tokenize`, or render a token tree
  with `Converter.Render`.
- Use headings, paragraphs, blockquotes, ordered and unordered lists, fenced
  and indented code blocks, links, images, emphasis, code spans, and thematic
  breaks.
- Use GFM-style tables, strikethrough, task-list checkboxes, and automatic
  links for URL and email text.
- Add block or inline Markdown parser rules, replace Markdown token handlers,
  and customize HTML-to-Markdown rendering by tag or prioritized rule before
  the converter's first operation.
- Choose compact or deterministic indented HTML output and fail-fast or
  best-effort error handling.
- Set input, nesting-depth, and token-count limits.

This package implements the syntax covered by its parser and tests; it does
not claim complete conformance with every CommonMark or GFM edge case.

## HTML to Markdown

`ConvertHTML` accepts a complete document or an HTML fragment. The HTML5 parser
repairs malformed markup, decodes character references, and converts the body
content to GFM Markdown. For documents, the first `<title>` and the first
`<meta name="description">` supply YAML frontmatter values when non-empty:

```html
<html>
  <head>
    <title>Guide</title>
    <meta name="description" content="A short guide">
  </head>
  <body><h1>Welcome</h1></body>
</html>
```

```markdown
---
title: "Guide"
description: "A short guide"
---

# Welcome
```

Head metadata is omitted when both values are empty. Unsupported tags are
removed while their children are converted; comments, `<script>`, and
`<style>` content are skipped. Links and images use the built-in safe URL
policy. `WithMaxInputBytes`, `WithMaxNestingDepth`, and `WithMaxTokens` also
limit HTML input size, nesting depth, and parsed node count.

Headings, paragraphs, blockquotes, ordered and unordered lists, fenced code,
links, images, line breaks, thematic breaks, tables, task checkboxes, emphasis,
and strikethrough map to their Markdown equivalents.

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
Register handlers and rules before calling `Tokenize`, `Render`, `Convert`, or
`ConvertHTML`; the converter becomes immutable after its first operation and
can then be used concurrently.

HTML-to-Markdown handlers use `HTMLNode` and `HTMLRenderContext`. A tag handler
replaces the built-in conversion for that tag. A custom rule can match an
element by its tag and attributes; higher priorities run first, with the most
recent registration winning ties. `RenderChildren` uses the current block or
inline mode, while `WriteText` escapes literal Markdown punctuation. For
example, map `<mark>` to GFM highlight syntax:

```go
err := converter.RegisterHTMLHandler("mark",
	func(ctx *markdown.HTMLRenderContext, w io.Writer, node markdown.HTMLNode) error {
		if _, err := io.WriteString(w, "=="); err != nil {
			return err
		}
		if err := ctx.RenderChildren(w, node); err != nil {
			return err
		}
		_, err := io.WriteString(w, "==")
		return err
	})
```

Use a rule when the conversion depends on attributes rather than just the tag:

```go
err := converter.RegisterHTMLRule(markdown.HTMLRule{
	ID:       "mention",
	Priority: 100,
	Match: func(node markdown.HTMLNode) bool {
		kind, ok := node.Attribute("data-kind")
		return node.Tag() == "span" && ok && kind == "mention"
	},
	Handler: func(ctx *markdown.HTMLRenderContext, w io.Writer, node markdown.HTMLNode) error {
		if _, err := io.WriteString(w, "@"); err != nil {
			return err
		}
		return ctx.RenderChildren(w, node)
	},
})
```

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
when handling untrusted values. HTML-to-Markdown handlers write Markdown
directly and are trusted code; use `HTMLRenderContext.WriteText` for literal
text from untrusted HTML.

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
returns recoverable Markdown parser and renderer problems in the result's
`Warnings`; `ConvertHTML` reports failures through its returned error.
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

## Contributing

Use Go 1.26 or later. Before opening a pull request, run `make ci` from the
repository root; it installs and configures `goppy`, then runs license, lint,
test, benchmark, fuzz, and build steps. The setup may download tooling.

## Contributors

See the [contributor list](https://github.com/osspkg/go-markdown/graphs/contributors).

## License

This project is licensed under the [BSD 3-Clause License](LICENSE).
