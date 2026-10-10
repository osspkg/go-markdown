package markdown_test

import (
	"strings"
	"testing"

	"go.osspkg.com/markdown"
)

func BenchmarkConvert(b *testing.B) {
	inputs := map[string]string{
		"small":  "# title\n\ntext with **emphasis** and [link](https://example.com)",
		"medium": "# title\n\n" + repeatedMarkdown(64),
		"large":  "# title\n\n" + repeatedMarkdown(1024),
	}
	converter, err := markdown.New()
	if err != nil {
		b.Fatal(err)
	}
	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				result, err := converter.Convert(input)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.HTML) == 0 {
					b.Fatal("Convert() returned empty HTML")
				}
			}
		})
	}
}

func repeatedMarkdown(lines int) string {
	value := "- item with `code` and ~~strike~~\n"
	var builder strings.Builder
	builder.Grow(len(value) * lines)
	for range lines {
		builder.WriteString(value)
	}
	return builder.String()
}
