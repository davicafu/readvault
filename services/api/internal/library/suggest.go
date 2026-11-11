package library

import (
	"cmp"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"readvault/api/internal/tag"
)

// Suggestion limits. neighbours is how many similar articles vote;
// a collection needs minVoters of them and must beat the runner-up by
// standOut; a tag needs minTextMentions in the text or minSharers among
// the neighbours.
const (
	neighbours      = 7
	minVoters       = 2
	standOut        = 1.5
	maxTags         = 5
	minTextMentions = 2
	minSharers      = 2
	// shortTag is the longest tag, in runes, that is taken only from the
	// title: "go" or "r" are ordinary words in a text or an address.
	shortTag = 2
)

// SuggestTarget is the article to suggest for.
type SuggestTarget struct {
	ID    string
	Title string
	URL   string
	// Text is the saved text as Markdown, or "" when there is none.
	Text string
	Tags []tag.Tag
}

// SuggestPeer is another article of the library, as compared against.
type SuggestPeer struct {
	ID           string
	Title        string
	CollectionID *string
	Tags         []tag.Tag
}

// SuggestInput is everything a suggestion is made from.
type SuggestInput struct {
	Target      SuggestTarget
	Peers       []SuggestPeer
	Tags        []tag.Tag
	Collections []CollectionRef
}

// Suggestions are a collection, or none, and up to five tags for one
// article, all taken from the user's own collections and tags.
type Suggestions struct {
	Collection *CollectionRef `json:"collection"`
	Tags       []tag.Tag      `json:"tags"`
}

// Suggest picks a collection and tags for an article from its content: its
// title, the words of its URL and its text.
//
// The collection is the one most similar library articles share: articles
// are compared by the words of their titles and tags, rarer words weighing
// more. No collection is suggested when fewer than two similar articles
// agree or when no collection clearly beats the others.
//
// Tags are the user's tags named in the title or URL, named several times
// in the text, or carried by several of the similar articles.
func Suggest(in SuggestInput) Suggestions {
	t := readTarget(in.Target)
	near := nearest(t, in.Target.ID, in.Peers)
	return Suggestions{
		Collection: pickCollection(near, in.Collections),
		Tags:       pickTags(t, near, in.Tags, in.Target.Tags),
	}
}

// target is the content of the article, split into words.
type target struct {
	title, url, text []string
	// weight of each term: the title counts most, then the URL, then how
	// often the text uses it (up to three times).
	weight map[string]float64
}

var (
	markdownLink = regexp.MustCompile(`\]\([^)]*\)`)
	bareURL      = regexp.MustCompile(`https?://\S+`)
)

func readTarget(in SuggestTarget) target {
	text := bareURL.ReplaceAllString(markdownLink.ReplaceAllString(in.Text, "]"), " ")
	t := target{
		title:  words(bareURL.ReplaceAllString(in.Title, " ")),
		url:    urlWords(in.URL),
		text:   words(text),
		weight: map[string]float64{},
	}
	// A title that is only the address, as when a link is saved, reads as
	// the address.
	if len(t.title) == 0 {
		t.title = urlWords(in.Title)
	}
	for _, w := range terms(t.title) {
		t.weight[w] = max(t.weight[w], 3)
	}
	for _, w := range terms(t.url) {
		t.weight[w] = max(t.weight[w], 2)
	}
	inText := map[string]float64{}
	for _, w := range terms(t.text) {
		inText[w] = min(inText[w]+1, 3)
	}
	for w, n := range inText {
		t.weight[w] += n
	}
	return t
}

type neighbour struct {
	peer  SuggestPeer
	score float64
}

// nearest returns the most similar peers, best first. A peer's score adds
// the target's weight of every term they share, times how rare the term is
// in the library, and is divided by the root of the peer's length so long
// titles do not win by size alone.
func nearest(t target, targetID string, peers []SuggestPeer) []neighbour {
	sets := make([]map[string]bool, len(peers))
	df := map[string]int{}
	for i, p := range peers {
		set := map[string]bool{}
		for _, w := range terms(words(p.Title)) {
			set[w] = true
		}
		for _, tg := range p.Tags {
			for _, w := range terms(words(tg.Name)) {
				set[w] = true
			}
		}
		sets[i] = set
		for w := range set {
			df[w]++
		}
	}

	n := float64(len(peers))
	var near []neighbour
	for i, p := range peers {
		if p.ID == targetID || len(sets[i]) == 0 {
			continue
		}
		var score float64
		for w := range sets[i] {
			if weight, ok := t.weight[w]; ok {
				score += weight * math.Log(1+n/float64(df[w]))
			}
		}
		if score > 0 {
			near = append(near, neighbour{p, score / math.Sqrt(float64(len(sets[i])))})
		}
	}
	slices.SortStableFunc(near, func(a, b neighbour) int { return cmp.Compare(b.score, a.score) })
	return near[:min(len(near), neighbours)]
}

func pickCollection(near []neighbour, collections []CollectionRef) *CollectionRef {
	votes := map[string]float64{}
	voters := map[string]int{}
	for _, nb := range near {
		if id := nb.peer.CollectionID; id != nil {
			votes[*id] += nb.score
			voters[*id]++
		}
	}
	var best, second float64
	var bestID string
	for id, v := range votes {
		switch {
		case v > best:
			best, second, bestID = v, best, id
		case v > second:
			second = v
		}
	}
	if bestID == "" || voters[bestID] < minVoters || best < standOut*second {
		return nil
	}
	for _, c := range collections {
		if c.ID == bestID {
			return &c
		}
	}
	return nil
}

func pickTags(t target, near []neighbour, all, has []tag.Tag) []tag.Tag {
	type scored struct {
		tag   tag.Tag
		score float64
	}
	sharers := map[string]int{}
	for _, nb := range near {
		for _, tg := range nb.peer.Tags {
			sharers[tg.Slug]++
		}
	}

	var picked []scored
	for _, tg := range all {
		if slices.ContainsFunc(has, func(h tag.Tag) bool { return h.Slug == tg.Slug }) {
			continue
		}
		phrase := words(tg.Name)
		if len(phrase) == 0 {
			continue
		}
		short := utf8.RuneCountInString(tg.Name) <= shortTag
		inTitle := count(t.title, phrase) > 0
		inURL := !short && count(t.url, phrase) > 0
		inText := 0
		if !short {
			inText = count(t.text, phrase)
		}
		shared := sharers[tg.Slug]

		if !inTitle && !inURL && inText < minTextMentions && shared < minSharers {
			continue
		}
		score := float64(min(inText, 5)) + 2*float64(shared)
		if inTitle {
			score += 3
		}
		if inURL {
			score += 2
		}
		picked = append(picked, scored{tg, score})
	}
	slices.SortStableFunc(picked, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.tag.Slug, b.tag.Slug))
	})

	out := []tag.Tag{}
	for _, p := range picked[:min(len(picked), maxTags)] {
		out = append(out, p.tag)
	}
	return out
}

// count returns how many times phrase appears in seq as consecutive words.
func count(seq, phrase []string) int {
	n := 0
	for i := 0; i+len(phrase) <= len(seq); i++ {
		if slices.Equal(seq[i:i+len(phrase)], phrase) {
			n++
		}
	}
	return n
}

// words lowercases s and splits it on anything that is not a letter or a
// digit, keeping every word in order.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// urlWords returns the words of an address's host and path, without the
// parts every address has.
func urlWords(raw string) []string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil
	}
	var out []string
	for _, w := range words(u.Hostname() + " " + u.Path) {
		if !urlNoise[w] {
			out = append(out, w)
		}
	}
	return out
}

// terms keeps the words that tell articles apart: three or more runes, not
// a number, not a stop word.
func terms(ws []string) []string {
	var out []string
	for _, w := range ws {
		if utf8.RuneCountInString(w) < 3 || stopWords[w] || strings.IndexFunc(w, unicode.IsLetter) < 0 {
			continue
		}
		out = append(out, w)
	}
	return out
}

var urlNoise = setOf("www", "com", "org", "net", "io", "dev", "html", "htm", "php", "amp")

// stopWords are frequent English and Spanish words that say nothing about a
// topic.
var stopWords = setOf(
	"the", "and", "for", "with", "you", "your", "are", "was", "were", "this", "that", "these", "those",
	"from", "into", "about", "what", "when", "where", "which", "who", "why", "how", "not", "but", "all",
	"any", "can", "will", "just", "more", "most", "than", "then", "there", "their", "they", "them",
	"has", "have", "had", "its", "our", "out", "one", "use", "using", "used", "every", "need", "should",
	"would", "could", "also", "only", "very", "here", "over", "after", "before", "like", "make", "get",
	"del", "las", "los", "una", "uno", "por", "para", "con", "sin", "que", "como", "más", "sus", "este",
	"esta", "estos", "entre", "sobre", "desde", "hasta", "pero", "cuando", "donde", "también",
)

func setOf(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}
