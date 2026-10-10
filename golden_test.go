package markdown_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.osspkg.com/markdown"
)

func TestUnit_Converter_Golden(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "commonmark/basic"},
		{name: "gfm/basic"},
	}
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", tt.name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join("testdata", tt.name+".html"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := converter.Convert(string(input))
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSuffix(string(expected), "\n")
			if result.HTML != want {
				t.Errorf("HTML = %q, want %q", result.HTML, want)
			}
		})
	}
}

func TestUnit_Converter_GoldenHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "document"},
		{name: "gfm"},
		{name: "fragment-recovery"},
	}
	converter, err := markdown.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", "html-to-markdown", test.name+".html"))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join("testdata", "html-to-markdown", test.name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := converter.ConvertHTML(string(input))
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSuffix(string(expected), "\n")
			if got != want {
				t.Errorf("Markdown = %q, want %q", got, want)
			}
		})
	}
}
