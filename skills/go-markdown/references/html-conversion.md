# HTML to GFM Markdown

Use `Converter.ConvertHTML(source)` for a complete HTML document or fragment.
It validates UTF-8 and applies the converter's input-byte, nesting-depth, and
HTML-node limits. The HTML5 parser repairs malformed markup and decodes
character references.

For a full document, content is rendered from `<body>`. The first `<title>`
and first `<meta name="description">` in `<head>` supply YAML frontmatter when
their values are non-empty. Duplicate metadata uses the first element even if
its value is empty. Frontmatter is omitted when both selected values are empty.

Built-in mappings include:

- Headings, paragraphs, blockquotes, ordered/unordered lists, and thematic
  breaks.
- Fenced code blocks and inline code spans.
- Links, images, and hard line breaks.
- GFM tables with alignment, task checkboxes, emphasis, and strikethrough.

Unsupported elements are removed while their children are rendered. Comments,
`<script>`, and `<style>` nodes and their contents are skipped. Checkbox
`<input>` elements are represented in task-list items; other input elements
are discarded by default. Link and image destinations use the package's
safe-URL policy; an unsafe URL is omitted and its text or image alternative
text is retained.

## Customize output

Register custom behavior before the converter is used:

- `RegisterHTMLHandler(tag, handler)` overrides the built-in conversion for
  that element name. Names are normalized to lowercase and surrounding spaces
  are trimmed.
- `RegisterHTMLRule(HTMLRule{ID, Priority, Match, Handler})` matches elements
  using `HTMLNode`. Greater priority runs first; ties use reverse registration
  order. A matching rule runs before a tag handler.

The handler receives a read-only `HTMLNode` view. Use `Tag`, `Text`, and
`Attribute(name)` to inspect it. `HTMLRenderContext.RenderChildren` keeps the
current block or inline mode and applies registered handlers recursively.
`HTMLRenderContext.WriteText` escapes Markdown punctuation in literal text.
Handlers write Markdown directly, so they are trusted callbacks; escape
untrusted literal text and validate any generated destinations.

HTML handlers and rules do not run for `<script>` or `<style>`; these nodes
are always omitted.

See [complete examples](examples.md) and the [API guide](api.md) for the
customization method signatures and converter lifecycle.
