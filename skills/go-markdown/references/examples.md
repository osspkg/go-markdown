# Examples

The runnable Go files in [`examples/`](../examples/) use the local module
import path and can be read or copied as starting points:

- [`basic/main.go`](../examples/basic/main.go): create a converter and convert in both
  directions.
- [`parser-rule/main.go`](../examples/parser-rule/main.go): add inline syntax
  using a parser rule and a token handler.
- [`customization/main.go`](../examples/customization/main.go): register a Markdown
  token handler, an HTML tag handler, and an attribute-matching HTML rule
  before first use.

## Minimal conversion

```go
converter, err := markdown.New()
if err != nil {
	return err
}

result, err := converter.Convert("# Hello\n\n**world**")
if err != nil {
	return err
}
fmt.Println(result.HTML)

markdownText, err := converter.ConvertHTML(`<h1>Hello</h1><p><strong>world</strong></p>`)
if err != nil {
	return err
}
fmt.Println(markdownText)
```

See the linked source files for complete `main` functions and customization
callbacks.
