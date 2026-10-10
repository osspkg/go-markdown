package markdown_test

import (
	"errors"
	"strings"
	"testing"

	"go.osspkg.com/markdown"
)

func FuzzConverterDoesNotPanic(f *testing.F) {
	f.Add("# title\n\n**text**")
	f.Add("[link](javascript:alert(1)) <b>x</b>")
	f.Add("| a | b |\n| --- | --- |\n| c | d |")
	f.Add("```go\nfunc main() {}\n```")

	converter, err := markdown.New()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source string) {
		result, err := converter.Convert(source)
		if err != nil && !errors.Is(err, markdown.ErrInvalidUTF8) && !errors.Is(err, markdown.ErrLimitExceeded) {
			t.Fatalf("Convert() unexpected error = %v", err)
		}
		if strings.Contains(result.HTML, "<script>") {
			t.Fatalf("unsafe script reached HTML output: %q", result.HTML)
		}
	})
}

func FuzzCustomRuleProgressIsBounded(f *testing.F) {
	f.Add("text")
	f.Add("@@")

	converter, err := markdown.New()
	if err != nil {
		f.Fatal(err)
	}
	if err := converter.RegisterRule(markdown.Rule{
		ID:       "broken",
		Phase:    markdown.RulePhaseInline,
		Priority: 100,
		Parse: func(_ *markdown.ParseContext, _ markdown.Cursor) (markdown.Token, int, bool, error) {
			return markdown.Token{ID: "broken"}, 0, true, nil
		},
	}); err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source string) {
		_, err := converter.Convert("x\n\n" + source)
		if errors.Is(err, markdown.ErrInvalidUTF8) || errors.Is(err, markdown.ErrLimitExceeded) {
			return
		}
		if !errors.Is(err, markdown.ErrRuleProgress) {
			t.Fatalf("Convert() error = %v, want ErrRuleProgress", err)
		}
	})
}
