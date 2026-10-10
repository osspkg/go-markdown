/*
 *  Copyright (c) 2026 Mikhail Knyazhev <markus621@yandex.com>. All rights reserved.
 *  Use of this source code is governed by a BSD 3-Clause license that can be found in the LICENSE file.
 */

package main

import (
	"fmt"
	"log"

	"go.osspkg.com/markdown"
)

func main() {
	converter, err := markdown.New()
	if err != nil {
		log.Fatal(err)
	}

	result, err := converter.Convert("# Hello\n\nMarkdown **content**")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.HTML)

	markdownText, err := converter.ConvertHTML(`<h1>Hello</h1><p><strong>HTML</strong> content</p>`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(markdownText)
}
