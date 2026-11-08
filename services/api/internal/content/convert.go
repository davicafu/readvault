// Package content turns an article page into Markdown and keeps its text
// and images.
package content

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ImageRef is an image of the article body, with its absolute URL.
type ImageRef struct {
	URL string
	Alt string
}

// Extracted is the article text found in a page.
type Extracted struct {
	Markdown string
	// Images lists each distinct body image once, in reading order.
	Images []ImageRef
	Words  int
	// Paywalled means the page says it shows only part of a paid article.
	Paywalled bool
}

// minImageSide skips avatars, icons and tracking pixels: an image whose
// declared width or height is smaller is not part of the article.
const minImageSide = 64

// memberOnlyMarker is the label Medium puts on the preview of a paid story.
const memberOnlyMarker = "Member-only story"

// FromHTML extracts the article body of an HTML page as Markdown. Navigation,
// scripts, forms and images outside the body are left out. base resolves
// relative links and images.
func FromHTML(r io.Reader, base *url.URL) (Extracted, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return Extracted{}, fmt.Errorf("parsing html: %w", err)
	}
	root := articleRoot(doc)
	c := &converter{base: base, seen: map[string]bool{}}
	blocks := c.blocks(root)

	var text strings.Builder
	plainText(root, &text)
	md := ""
	if len(blocks) > 0 {
		md = strings.Join(blocks, "\n\n") + "\n"
	}
	return Extracted{
		Markdown:  md,
		Images:    c.images,
		Words:     countWords(text.String()),
		Paywalled: declaresNotFree(doc) || strings.Contains(text.String(), memberOnlyMarker),
	}, nil
}

var markdownImage = regexp.MustCompile(`!\[([^\]]*)\]\(\s*([^)\s]+)(?:\s+"[^"]*")?\s*\)`)

// FromMarkdown keeps uploaded Markdown as is and finds its images.
func FromMarkdown(md string, base *url.URL) Extracted {
	var images []ImageRef
	seen := map[string]bool{}
	for _, m := range markdownImage.FindAllStringSubmatch(md, -1) {
		if u, ok := absoluteWebURL(base, m[2]); ok && !seen[u] {
			seen[u] = true
			images = append(images, ImageRef{URL: u, Alt: m[1]})
		}
	}
	return Extracted{Markdown: md, Images: images, Words: countWords(markdownImage.ReplaceAllString(md, " "))}
}

// countWords counts the whitespace-separated tokens that hold a letter or a
// digit, so Markdown symbols such as "#" or "-" are not words.
func countWords(s string) int {
	n := 0
	for _, f := range strings.Fields(s) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			n++
		}
	}
	return n
}

// skipped elements never hold article text.
func skipped(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Iframe, atom.Object, atom.Embed,
		atom.Form, atom.Button, atom.Input, atom.Select, atom.Textarea, atom.Nav, atom.Footer, atom.Aside,
		atom.Svg, atom.Canvas, atom.Head:
		return true
	}
	return false
}

// articleRoot returns the element that holds the article: the <article>
// with the most text, otherwise the element whose paragraphs hold the most
// text, otherwise <body>.
func articleRoot(doc *html.Node) *html.Node {
	var best *html.Node
	bestLen := -1
	walk(doc, func(n *html.Node) {
		if n.DataAtom == atom.Article {
			if l := textLen(n); l > bestLen {
				best, bestLen = n, l
			}
		}
	})
	if best != nil {
		return best
	}

	scores := map[*html.Node]int{}
	walk(doc, func(n *html.Node) {
		if n.DataAtom != atom.P || n.Parent == nil {
			return
		}
		l := textLen(n)
		scores[n.Parent] += l
		if n.Parent.Parent != nil {
			scores[n.Parent.Parent] += l / 2
		}
	})
	for n, s := range scores {
		if s > bestLen {
			best, bestLen = n, s
		}
	}
	if best != nil {
		return best
	}
	var body *html.Node
	walk(doc, func(n *html.Node) {
		if n.DataAtom == atom.Body && body == nil {
			body = n
		}
	})
	if body != nil {
		return body
	}
	return doc
}

// walk visits the element nodes under n, not entering skipped elements.
func walk(n *html.Node, visit func(*html.Node)) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skipped(c) {
			continue
		}
		visit(c)
		walk(c, visit)
	}
}

func textLen(n *html.Node) int {
	var b strings.Builder
	plainText(n, &b)
	return len(strings.TrimSpace(b.String()))
}

// plainText writes the text under n, leaving out skipped elements.
func plainText(n *html.Node, b *strings.Builder) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			b.WriteString(c.Data)
			b.WriteByte(' ')
		case c.Type == html.ElementNode && !skipped(c):
			plainText(c, b)
		}
	}
}

// rawText is the text under n exactly as written, for code.
func rawText(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			} else if c.Type == html.ElementNode {
				if c.DataAtom == atom.Br {
					b.WriteByte('\n')
				}
				rec(c)
			}
		}
	}
	rec(n)
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

type converter struct {
	base   *url.URL
	images []ImageRef
	seen   map[string]bool
}

func isBlock(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Main, atom.Header, atom.H1, atom.H2, atom.H3,
		atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Li, atom.Pre, atom.Blockquote, atom.Hr, atom.Figure,
		atom.Figcaption, atom.Table, atom.Dl, atom.Dt, atom.Dd:
		return true
	}
	return false
}

// blocks converts the children of a container into Markdown blocks. Runs of
// inline content between block children become paragraphs.
func (c *converter) blocks(n *html.Node) []string {
	var out []string
	var inline strings.Builder
	flush := func() {
		if t := cleanInline(inline.String()); t != "" {
			out = append(out, t)
		}
		inline.Reset()
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		switch {
		case ch.Type == html.ElementNode && skipped(ch):
		case isBlock(ch):
			flush()
			out = append(out, c.block(ch)...)
		default:
			inline.WriteString(c.inline(ch))
		}
	}
	flush()
	return out
}

func (c *converter) block(n *html.Node) []string {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		t := cleanInline(c.inlineChildren(n))
		if t == "" {
			return nil
		}
		level := int(n.Data[1] - '0')
		return []string{strings.Repeat("#", level) + " " + strings.ReplaceAll(t, "  \n", " ")}
	case atom.Ul, atom.Ol:
		if l := c.list(n, 0); l != "" {
			return []string{l}
		}
		return nil
	case atom.Pre:
		code := strings.TrimRight(rawText(n), "\n")
		if strings.TrimSpace(code) == "" {
			return nil
		}
		fence := "```"
		for strings.Contains(code, fence) {
			fence += "`"
		}
		return []string{fence + "\n" + code + "\n" + fence}
	case atom.Blockquote:
		inner := c.blocks(n)
		if len(inner) == 0 {
			return nil
		}
		lines := strings.Split(strings.Join(inner, "\n\n"), "\n")
		for i, l := range lines {
			if l == "" {
				lines[i] = ">"
			} else {
				lines[i] = "> " + l
			}
		}
		return []string{strings.Join(lines, "\n")}
	case atom.Hr:
		return []string{"---"}
	case atom.Figcaption:
		if t := cleanInline(c.inlineChildren(n)); t != "" {
			return []string{"*" + t + "*"}
		}
		return nil
	case atom.Table:
		var rows []string
		walk(n, func(r *html.Node) {
			if r.DataAtom != atom.Tr {
				return
			}
			var cells []string
			for cell := r.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.DataAtom == atom.Td || cell.DataAtom == atom.Th {
					cells = append(cells, cleanInline(c.inlineChildren(cell)))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, strings.Join(cells, " | "))
			}
		})
		if len(rows) == 0 {
			return nil
		}
		return []string{strings.Join(rows, "  \n")}
	default:
		return c.blocks(n)
	}
}

// list renders a <ul> or <ol>, nesting inner lists by two spaces per level.
func (c *converter) list(n *html.Node, depth int) string {
	var lines []string
	i := 0
	for li := n.FirstChild; li != nil; li = li.NextSibling {
		if li.DataAtom != atom.Li {
			continue
		}
		i++
		marker := "- "
		if n.DataAtom == atom.Ol {
			marker = strconv.Itoa(i) + ". "
		}
		var text strings.Builder
		var nested []string
		for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
			switch {
			case ch.DataAtom == atom.Ul || ch.DataAtom == atom.Ol:
				if l := c.list(ch, depth+1); l != "" {
					nested = append(nested, l)
				}
			case ch.Type == html.ElementNode && skipped(ch):
			case isBlock(ch):
				text.WriteString(" " + c.inlineChildren(ch) + " ")
			default:
				text.WriteString(c.inline(ch))
			}
		}
		item := strings.ReplaceAll(cleanInline(text.String()), "  \n", " ")
		lines = append(lines, strings.Repeat("  ", depth)+marker+item)
		lines = append(lines, nested...)
	}
	return strings.Join(lines, "\n")
}

func (c *converter) inlineChildren(n *html.Node) string {
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && skipped(ch) {
			continue
		}
		b.WriteString(c.inline(ch))
	}
	return b.String()
}

// brMark stands for a line break until cleanInline renders it.
const brMark = " "

var spaces = regexp.MustCompile(`[ \t\r\n\f]+`)

func (c *converter) inline(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return escapeText(spaces.ReplaceAllString(strings.ReplaceAll(n.Data, brMark, " "), " "))
	case html.ElementNode:
	default:
		return ""
	}
	switch n.DataAtom {
	case atom.Br:
		return brMark
	case atom.Strong, atom.B:
		return wrap(c.inlineChildren(n), "**")
	case atom.Em, atom.I:
		return wrap(c.inlineChildren(n), "*")
	case atom.Code, atom.Kbd, atom.Samp:
		code := spaces.ReplaceAllString(rawText(n), " ")
		if strings.TrimSpace(code) == "" {
			return ""
		}
		tick := "`"
		for strings.Contains(code, tick) {
			tick += "`"
		}
		return tick + code + tick
	case atom.A:
		inner := c.inlineChildren(n)
		if strings.TrimSpace(inner) == "" {
			return inner
		}
		u, ok := absoluteWebURL(c.base, attr(n, "href"))
		if !ok {
			return inner
		}
		return wrapKeepingSpace(inner, "[", "]("+markdownURL(u)+")")
	case atom.Img:
		return c.image(n, "")
	case atom.Picture:
		// The <img> may have no address of its own (Medium puts it only in
		// the <source> elements); the widest source is the fallback.
		var sources []string
		walk(n, func(ch *html.Node) {
			if ch.DataAtom == atom.Source {
				sources = append(sources, attr(ch, "srcset"))
			}
		})
		fallback := largestFromSrcset(strings.Join(sources, ","))
		var img string
		walk(n, func(ch *html.Node) {
			if ch.DataAtom == atom.Img && img == "" {
				img = c.image(ch, fallback)
			}
		})
		return img
	default:
		return c.inlineChildren(n)
	}
}

// image renders an article image and records it. Images without a usable
// web address, data URIs and small images (icons, avatars, tracking pixels)
// are left out. fallback is used when the element has no address.
func (c *converter) image(n *html.Node, fallback string) string {
	if tooSmall(attr(n, "width")) || tooSmall(attr(n, "height")) {
		return ""
	}
	src := attr(n, "src")
	if src == "" || strings.HasPrefix(src, "data:") {
		src = attr(n, "data-src")
	}
	if src == "" {
		src = largestFromSrcset(attr(n, "srcset"))
	}
	if src == "" {
		src = fallback
	}
	u, ok := absoluteWebURL(c.base, src)
	if !ok {
		return ""
	}
	alt := strings.TrimSpace(spaces.ReplaceAllString(attr(n, "alt"), " "))
	if !c.seen[u] {
		c.seen[u] = true
		c.images = append(c.images, ImageRef{URL: u, Alt: alt})
	}
	return "![" + escapeText(alt) + "](" + markdownURL(u) + ")"
}

func tooSmall(dim string) bool {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(dim), "px"))
	return err == nil && n < minImageSide
}

// largestFromSrcset picks the widest candidate of a srcset attribute.
func largestFromSrcset(srcset string) string {
	best, bestW := "", -1
	for _, cand := range strings.Split(srcset, ",") {
		f := strings.Fields(cand)
		if len(f) == 0 {
			continue
		}
		w := 0
		if len(f) > 1 {
			w, _ = strconv.Atoi(strings.TrimSuffix(f[1], "w"))
		}
		if w > bestW {
			best, bestW = f[0], w
		}
	}
	return best
}

// absoluteWebURL resolves ref against base and accepts only http and https.
func absoluteWebURL(base *url.URL, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "data:") {
		return "", false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

// markdownURL escapes the characters that would end a Markdown link target.
func markdownURL(u string) string {
	return strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(u)
}

var textEscaper = strings.NewReplacer(`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`)

func escapeText(s string) string { return textEscaper.Replace(s) }

// wrap puts a marker around text, keeping its outer spaces outside.
func wrap(s, marker string) string { return wrapKeepingSpace(s, marker, marker) }

func wrapKeepingSpace(s, open, closing string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return s
	}
	lead, trail := "", ""
	if strings.HasPrefix(s, " ") {
		lead = " "
	}
	if strings.HasSuffix(s, " ") {
		trail = " "
	}
	return lead + open + t + closing + trail
}

// cleanInline collapses spaces, trims the text and renders line breaks.
func cleanInline(s string) string {
	parts := strings.Split(spaces.ReplaceAllString(s, " "), brMark)
	var lines []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			lines = append(lines, p)
		}
	}
	return strings.Join(lines, "  \n")
}

// declaresNotFree reports whether the page's schema.org data marks it as not
// free to read, the markup paywalled sites publish for search engines.
func declaresNotFree(doc *html.Node) bool {
	found := false
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil && !found; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			if c.DataAtom == atom.Script && strings.EqualFold(attr(c, "type"), "application/ld+json") {
				var data any
				if json.Unmarshal([]byte(rawText(c)), &data) == nil && notFree(data) {
					found = true
				}
				continue
			}
			visit(c)
		}
	}
	visit(doc)
	return found
}

func notFree(v any) bool {
	switch v := v.(type) {
	case map[string]any:
		for k, val := range v {
			if k == "isAccessibleForFree" {
				if b, ok := val.(bool); ok && !b {
					return true
				}
				if s, ok := val.(string); ok && strings.EqualFold(s, "false") {
					return true
				}
			}
			if notFree(val) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if notFree(item) {
				return true
			}
		}
	}
	return false
}
