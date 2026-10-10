package markdown_test

import (
	"fmt"
	"io"

	"go.osspkg.com/markdown"
)

func ExampleConverter() {
	converter, err := markdown.New()
	if err != nil {
		panic(err)
	}
	if err := converter.RegisterHandler(markdown.TokenStrong, func(ctx *markdown.RenderContext, writer io.Writer, token markdown.Token) error {
		if _, err := io.WriteString(writer, "<mark>"); err != nil {
			return err
		}
		if err := ctx.RenderChildren(writer, token); err != nil {
			return err
		}
		_, err := io.WriteString(writer, "</mark>")
		return err
	}); err != nil {
		panic(err)
	}

	result, err := converter.Convert("**important**")
	if err != nil {
		panic(err)
	}
	fmt.Println(result.HTML)
	// Output: <p><mark>important</mark></p>
}
