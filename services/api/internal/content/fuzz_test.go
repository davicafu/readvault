package content_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"readvault/api/internal/content"
)

func FuzzFromHTML(f *testing.F) {
	f.Add(`<article><h1>t</h1><p>a <b>b</b> <a href="/x">c</a></p><ul><li>x<ol><li>y</li></ol></li></ul><pre>z</pre></article>`)
	f.Add(`<div><p>` + strings.Repeat("w ", 50) + `</p><img src="i.png" srcset="a 1w, b 2w"></div>`)
	f.Add(`<script type="application/ld+json">{"isAccessibleForFree":[false,{"a":"False"}]}</script>`)
	f.Fuzz(func(t *testing.T, page string) {
		got, err := content.FromHTML(strings.NewReader(page), pageURL)
		if err != nil {
			return
		}
		if utf8.ValidString(page) && !utf8.ValidString(got.Markdown) {
			t.Errorf("valid input gave invalid UTF-8 markdown")
		}
		for _, img := range got.Images {
			if !strings.HasPrefix(img.URL, "http://") && !strings.HasPrefix(img.URL, "https://") {
				t.Errorf("image %q is not a web address", img.URL)
			}
		}
	})
}
