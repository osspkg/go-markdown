# API guide

Import the package as `markdown`:

```go
import "go.osspkg.com/markdown"
```

## Choose a conversion path

| Need | API | Result |
| --- | --- | --- |
| Convert Markdown directly to HTML | `converter.Convert(source)` | `ConversionResult`, with rendered `HTML` and any `Warnings` |
| Inspect parsed Markdown tokens | `converter.Tokenize(source)` | `TokenizationResult` with root `Tokens` and `Warnings` |
| Render a token tree | `converter.Render(w, tokens)` | Warnings and an error |
| Convert HTML document or fragment to GFM Markdown | `converter.ConvertHTML(source)` | Markdown string and an error |

Create the converter with `markdown.New(opts...)`; it returns `(*Converter,
error)`. Check that error before using the converter.

## Configuration

Pass options to `New`:

- `WithErrorMode(ErrorModeFailFast)` is the default. `ErrorModeBestEffort`
  collects recoverable Markdown parser and renderer errors in `Warnings`.
- `WithHTMLFormat(HTMLFormatCompact)` is the default. Use
  `HTMLFormatPretty` for indented HTML output.
- `WithMaxInputBytes(n)` limits normalized input bytes. The default is 16 MiB;
  `0` disables this limit.
- `WithMaxNestingDepth(n)` limits recursive parse/render depth. The default is
  128, and the value must be positive.
- `WithMaxTokens(n)` limits Markdown tokens or parsed HTML nodes. The default
  is 1,000,000, and the value must be positive.
- `WithUnsafeRawHTML()` allows raw HTML from Markdown input through, while the
  built-in renderer still filters GFM-disallowed tags. Use it only for trusted
  Markdown.

`ConvertHTML` reports errors directly; `ErrorModeBestEffort` does not turn
those errors into a warning result.

## Extending Markdown parsing and rendering

`RegisterRule` adds a block or inline Markdown parser rule. A rule has an ID,
phase, priority, and parser callback. On no match it returns `matched=false`
and `consumed=0`. On a match it must consume a positive number of bytes, end on
a UTF-8 boundary, and produce a token with a registered handler. Block rules
must consume whole lines. Higher priority runs first; ties use reverse
registration order.

`RegisterHandler` installs or replaces the handler for a `TokenID`. Use
`RenderContext.RenderChildren` for nested tokens and `WriteText`,
`WriteAttribute`, and `SafeURL` for safely producing HTML. The latest
registration for an ID wins.

Built-in IDs and attributes are defined in the root `API.md`. Common IDs
include `TokenText`, `TokenParagraph`, `TokenStrong`, `TokenLink`, and
`TokenCodeBlock`; use the `Attribute...` constants rather than spelling
attribute keys when available.

## Lifecycle

Configure the converter and register all extensions before calling any of
`Convert`, `ConvertHTML`, `Tokenize`, or `Render`. The first operation freezes
configuration. Later registration attempts return `ErrFrozen`. A configured
converter can be reused concurrently after that point.

See [examples](examples.md) for complete usage, and use the API guide in this
skill as the reference for the public methods and lifecycle.
