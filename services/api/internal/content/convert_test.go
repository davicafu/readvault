package content_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"readvault/api/internal/content"
)

var pageURL, _ = url.Parse("https://blog.example/posts/queues")

func fromHTML(t *testing.T, page string) content.Extracted {
	t.Helper()
	got, err := content.FromHTML(strings.NewReader(page), pageURL)
	if err != nil {
		t.Fatalf("FromHTML() error: %v", err)
	}
	return got
}

func TestFromHTML_ConvertsTheArticleBodyToMarkdown(t *testing.T) {
	t.Parallel()
	page := `<html><head><title>x</title><style>p{}</style></head><body>
<nav><a href="/">Home</a> <a href="/about">About</a></nav>
<article>
  <h1>Working   with queues</h1>
  <p>Queues <strong>decouple</strong> producers and <em>consumers</em>. See <a href="/docs/intro">the intro</a>.</p>
  <h2>Delivery</h2>
  <ul><li>At most once</li><li>At least once<ul><li>needs idempotency</li></ul></li></ul>
  <ol><li>Publish</li><li>Consume</li></ol>
  <blockquote><p>Retries need backoff.</p></blockquote>
  <pre><code>for msg := range ch {
	handle(msg)
}</code></pre>
  <p>Call <code>Ack()</code> after the work.<br>Then stop.</p>
  <script>track()</script>
  <hr>
  <p>Stars like *this* and [brackets] stay literal.</p>
</article>
<footer>Copyright footer</footer>
</body></html>`

	got := fromHTML(t, page)

	expected := strings.Join([]string{
		"# Working with queues",
		"",
		"Queues **decouple** producers and *consumers*. See [the intro](https://blog.example/docs/intro).",
		"",
		"## Delivery",
		"",
		"- At most once",
		"- At least once",
		"  - needs idempotency",
		"",
		"1. Publish",
		"2. Consume",
		"",
		"> Retries need backoff.",
		"",
		"```",
		"for msg := range ch {",
		"\thandle(msg)",
		"}",
		"```",
		"",
		"Call `Ack()` after the work.  ",
		"Then stop.",
		"",
		"---",
		"",
		`Stars like \*this\* and \[brackets\] stay literal.`,
		"",
	}, "\n")
	if got.Markdown != expected {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got.Markdown, expected)
	}
	for _, noise := range []string{"Home", "Copyright", "track()", "p{}"} {
		if strings.Contains(got.Markdown, noise) {
			t.Errorf("markdown holds %q from outside the article body", noise)
		}
	}
	if got.Words < 30 || got.Words > 60 {
		t.Errorf("words = %d, want the article's word count", got.Words)
	}
}

func TestFromHTML_WithoutAnArticleElementPicksTheMainText(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Backpressure keeps queues bounded under load. ", 20)
	page := `<body>
<div class="sidebar"><p>Subscribe now</p><p>Related posts</p></div>
<div class="post"><div class="body"><p>` + long + `</p><p>Second paragraph.</p></div></div>
<div class="comments"><p>Nice post!</p></div>
</body>`

	got := fromHTML(t, page)

	if !strings.Contains(got.Markdown, "Backpressure keeps queues bounded") || !strings.Contains(got.Markdown, "Second paragraph.") {
		t.Errorf("markdown = %q, want the post body", got.Markdown)
	}
	for _, noise := range []string{"Subscribe", "Nice post"} {
		if strings.Contains(got.Markdown, noise) {
			t.Errorf("markdown holds %q from outside the main text", noise)
		}
	}
}

func TestFromHTML_KeepsOnlyTheArticleImages(t *testing.T) {
	t.Parallel()
	page := `<body>
<img src="/logo.png" alt="site logo">
<article>
  <img src="/avatar.jpg" alt="Author" width="44" height="44">
  <p>Intro.</p>
  <figure><picture>
    <source srcset="https://cdn.example/fig1-400.webp 400w, https://cdn.example/fig1-1400.webp 1400w">
    <img src="https://cdn.example/fig1-700.png" alt="Queue diagram" width="700" height="400">
  </picture><figcaption>The queue.</figcaption></figure>
  <img data-src="img/lazy.jpg" alt="">
  <img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" alt="pixel">
  <img src="https://track.example/p.gif" width="1" height="1">
  <img src="https://cdn.example/fig1-700.png" alt="Queue diagram again">
</article>
</body>`

	got := fromHTML(t, page)

	expected := []content.ImageRef{
		{URL: "https://cdn.example/fig1-700.png", Alt: "Queue diagram"},
		{URL: "https://blog.example/posts/img/lazy.jpg", Alt: ""},
	}
	if !slices.Equal(got.Images, expected) {
		t.Errorf("images = %+v, want %+v", got.Images, expected)
	}
	for _, md := range []string{"![Queue diagram](https://cdn.example/fig1-700.png)", "*The queue.*", "![](https://blog.example/posts/img/lazy.jpg)"} {
		if !strings.Contains(got.Markdown, md) {
			t.Errorf("markdown lacks %q:\n%s", md, got.Markdown)
		}
	}
	for _, skipped := range []string{"logo", "avatar", "data:", "track.example"} {
		if strings.Contains(got.Markdown, skipped) {
			t.Errorf("markdown holds the skipped image %q", skipped)
		}
	}
}

func TestFromHTML_UnsafeLinksBecomeText(t *testing.T) {
	t.Parallel()
	got := fromHTML(t, `<article><p><a href="javascript:alert(1)">click</a> and <a href="mailto:a@b.example">mail</a></p></article>`)

	if got.Markdown != "click and mail\n" {
		t.Errorf("markdown = %q, want link text only", got.Markdown)
	}
}

func TestFromHTML_DetectsAPaywalledPage(t *testing.T) {
	t.Parallel()
	body := `<article><p>Only the start of the story.</p></article>`
	tests := []struct {
		name     string
		head     string
		body     string
		expected bool
	}{
		{"free page", `<script type="application/ld+json">{"@type":"Article","isAccessibleForFree":true}</script>`, body, false},
		{"no marker", ``, body, false},
		{"schema.org says not free", `<script type="application/ld+json">{"@type":"NewsArticle","isAccessibleForFree":"False"}</script>`, body, true},
		{"schema.org in a graph", `<script type="application/ld+json">{"@graph":[{"@type":"WebPage"},{"@type":"Article","isAccessibleForFree":false}]}</script>`, body, true},
		{"member-only marker", ``, `<article><p>Member-only story</p><p>Only the start.</p></article>`, true},
	}
	for _, tt := range tests {
		got := fromHTML(t, `<html><head>`+tt.head+`</head><body>`+tt.body+`</body></html>`)
		if got.Paywalled != tt.expected {
			t.Errorf("%s: paywalled = %v, want %v", tt.name, got.Paywalled, tt.expected)
		}
	}
}

func TestFromMarkdown_FindsImagesAndWords(t *testing.T) {
	t.Parallel()
	md := "# Title\n\nSome words here.\n\n![Diagram](https://cdn.example/a.png) and ![](img/b.jpg \"caption\")\n\n![x](javascript:alert(1))\n"

	got := content.FromMarkdown(md, pageURL)

	expected := []content.ImageRef{
		{URL: "https://cdn.example/a.png", Alt: "Diagram"},
		{URL: "https://blog.example/posts/img/b.jpg", Alt: ""},
	}
	if !slices.Equal(got.Images, expected) {
		t.Errorf("images = %+v, want %+v", got.Images, expected)
	}
	if got.Markdown != md {
		t.Error("markdown must be kept as uploaded")
	}
	if got.Words != 5 {
		t.Errorf("words = %d, want 5 (Title, Some, words, here., and)", got.Words)
	}
}
