---
name: go-markdown
description: Use when writing Go code with go.osspkg.com/markdown, including Markdown-to-HTML conversion, HTML-to-Markdown conversion, parser extensions, and renderer customization.
---

# go.osspkg.com/markdown

Help implement or review code that uses this library. First identify the input
and output direction, then use the matching converter API:

- Markdown to HTML: `Converter.Convert`, or `Tokenize` followed by `Render` when
  token inspection or reuse is needed.
- HTML to GFM Markdown: `Converter.ConvertHTML`.
- Custom Markdown syntax: `RegisterRule` plus a token handler registered with
  `RegisterHandler`.
- Custom Markdown token output: `RegisterHandler`.
- Custom HTML-to-Markdown output: `RegisterHTMLHandler` by tag or
  `RegisterHTMLRule` by element predicate.

Register all rules and handlers before the converter's first operation. Any
operation freezes configuration; afterward the converter can be used
concurrently, but registrations return `ErrFrozen`.

Keep output safety explicit. Built-in Markdown rendering escapes text and
attributes and checks link destinations. Custom handlers are trusted code:
use `RenderContext` helpers when emitting HTML, and `HTMLRenderContext.WriteText`
when emitting literal Markdown text. Preserve configured input, depth, and
token/node limits.

Use the references relevant to the task:

- [API guide](references/api.md) for method selection, options, and lifecycle.
- [HTML conversion](references/html-conversion.md) for supported mappings,
  metadata, safety, and HTML-specific extension points.
- [Code examples](references/examples.md) for complete snippets and example
  source files.

Check the repository's `API.md`, `README.md`, `AGENTS.md`, and source before
assuming behavior not covered by these references; update examples when the
public API changes.
