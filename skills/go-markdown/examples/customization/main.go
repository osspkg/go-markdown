/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package main

import (
	"fmt"
	"io"
	"log"

	"go.osspkg.com/markdown"
)

func main() {
	converter, err := markdown.New()
	if err != nil {
		log.Fatal(err)
	}

	err = converter.RegisterHandler(markdown.TokenStrong, func(ctx *markdown.RenderContext, w io.Writer, token markdown.Token) error {
		if _, err := io.WriteString(w, "<mark>"); err != nil {
			return err
		}
		if err := ctx.RenderChildren(w, token); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</mark>")
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	err = converter.RegisterHTMLHandler("mark", func(ctx *markdown.HTMLRenderContext, w io.Writer, node markdown.HTMLNode) error {
		if _, err := io.WriteString(w, "=="); err != nil {
			return err
		}
		if err := ctx.RenderChildren(w, node); err != nil {
			return err
		}
		_, err := io.WriteString(w, "==")
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	err = converter.RegisterHTMLRule(markdown.HTMLRule{
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
	if err != nil {
		log.Fatal(err)
	}

	markdownResult, err := converter.Convert("**important**")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(markdownResult.HTML)

	text, err := converter.ConvertHTML(`<p><span data-kind="mention">ada</span> <mark>important</mark></p>`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)
}
