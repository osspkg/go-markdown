/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package main

import (
	"fmt"
	"io"
	"log"
	"strings"

	"go.osspkg.com/markdown"
)

func main() {
	converter, err := markdown.New()
	if err != nil {
		log.Fatal(err)
	}

	const mention markdown.TokenID = "mention"
	err = converter.RegisterRule(markdown.Rule{
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
	if err != nil {
		log.Fatal(err)
	}

	err = converter.RegisterHandler(mention, func(ctx *markdown.RenderContext, w io.Writer, token markdown.Token) error {
		if _, err := io.WriteString(w, "<mark>"); err != nil {
			return err
		}
		if err := ctx.WriteText(w, token.Literal); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</mark>")
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := converter.Convert("hello @@ world")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.HTML)
}
