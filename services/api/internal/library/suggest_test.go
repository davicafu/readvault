package library_test

import (
	"slices"
	"testing"

	"readvault/api/internal/library"
	"readvault/api/internal/tag"
)

var (
	streaming = library.CollectionRef{ID: "c-streaming", Name: "Streaming", Slug: "streaming"}
	backend   = library.CollectionRef{ID: "c-backend", Name: "Backend", Slug: "backend"}
)

func tags(names ...string) []tag.Tag {
	out := make([]tag.Tag, len(names))
	for i, n := range names {
		t, err := tag.Normalize(n)
		if err != nil {
			panic(err)
		}
		out[i] = t
	}
	return out
}

func peer(id, title string, c *library.CollectionRef, tagNames ...string) library.SuggestPeer {
	p := library.SuggestPeer{ID: id, Title: title, Tags: tags(tagNames...)}
	if c != nil {
		p.CollectionID = &c.ID
	}
	return p
}

// library is a small shelf: Kafka articles in Streaming, Go ones in Backend.
func shelf() library.SuggestInput {
	return library.SuggestInput{
		Peers: []library.SuggestPeer{
			peer("1", "Kafka consumer groups and rebalancing | Medium", &streaming, "kafka"),
			peer("2", "Exactly-once delivery in Kafka | Medium", &streaming, "kafka", "kafka-streams"),
			peer("3", "Kafka Streams windowing explained | Medium", &streaming, "kafka", "kafka-streams"),
			peer("4", "Go concurrency patterns with channels | Medium", &backend, "go"),
			peer("5", "Error handling in Go services | Medium", &backend, "go"),
			peer("6", "Unsorted reading about cooking", nil),
		},
		Tags:        tags("kafka", "kafka-streams", "go", "iceberg", "postgres", "r"),
		Collections: []library.CollectionRef{streaming, backend},
	}
}

func collectionSlug(s library.Suggestions) string {
	if s.Collection == nil {
		return ""
	}
	return s.Collection.Slug
}

func tagSlugs(s library.Suggestions) []string {
	out := []string{}
	for _, t := range s.Tags {
		out = append(out, t.Slug)
	}
	return out
}

func TestSuggest_CollectionOfTheMostSimilarArticles(t *testing.T) {
	t.Parallel()
	in := shelf()
	in.Target = library.SuggestTarget{ID: "new", Title: "How Kafka consumer offsets work", URL: "https://example.com/post"}

	got := library.Suggest(in)

	if collectionSlug(got) != "streaming" {
		t.Errorf("collection = %q, want streaming", collectionSlug(got))
	}
}

func TestSuggest_RareWordsWeighMoreThanCommonOnes(t *testing.T) {
	t.Parallel()
	in := shelf()
	// "medium" is in every title; "channels" and "concurrency" only in Backend.
	in.Target = library.SuggestTarget{ID: "new", Title: "Concurrency with channels | Medium", URL: "https://example.com/a"}

	if got := library.Suggest(in); collectionSlug(got) != "backend" {
		t.Errorf("collection = %q, want backend", collectionSlug(got))
	}
}

func TestSuggest_NoCollectionWhenNothingIsSimilar(t *testing.T) {
	t.Parallel()
	in := shelf()
	in.Target = library.SuggestTarget{ID: "new", Title: "Baking sourdough bread", URL: "https://example.com/bread"}

	got := library.Suggest(in)

	if got.Collection != nil {
		t.Errorf("collection = %q, want none", got.Collection.Slug)
	}
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none", tagSlugs(got))
	}
}

func TestSuggest_NoCollectionWhenOnlyOneArticleIsSimilar(t *testing.T) {
	t.Parallel()
	in := shelf()
	in.Target = library.SuggestTarget{ID: "new", Title: "Rebalancing a portfolio", URL: "https://example.com/money"}

	if got := library.Suggest(in); got.Collection != nil {
		t.Errorf("collection = %q, want none from a single similar article", got.Collection.Slug)
	}
}

func TestSuggest_NoCollectionWhenTwoCollectionsTie(t *testing.T) {
	t.Parallel()
	in := library.SuggestInput{
		Peers: []library.SuggestPeer{
			peer("1", "Postgres replication", &streaming),
			peer("2", "Postgres replication slots", &streaming),
			peer("3", "Postgres replication lag", &backend),
			peer("4", "Postgres replication setup", &backend),
		},
		Collections: []library.CollectionRef{streaming, backend},
		Target:      library.SuggestTarget{ID: "new", Title: "Postgres replication", URL: "https://example.com/x"},
	}

	if got := library.Suggest(in); got.Collection != nil {
		t.Errorf("collection = %q, want none on a tie", got.Collection.Slug)
	}
}

func TestSuggest_UsesTheWordsOfTheURLAndTheText(t *testing.T) {
	t.Parallel()
	in := shelf()
	// Saved from a link: the title is the address itself.
	in.Target = library.SuggestTarget{
		ID:    "new",
		Title: "https://blog.example.com/kafka-consumer-groups",
		URL:   "https://blog.example.com/kafka-consumer-groups",
		Text:  "Consumer groups split partitions. Rebalancing moves them between consumers.",
	}

	got := library.Suggest(in)

	if collectionSlug(got) != "streaming" {
		t.Errorf("collection = %q, want streaming", collectionSlug(got))
	}
	if !slices.Contains(tagSlugs(got), "kafka") {
		t.Errorf("tags = %v, want kafka from the URL", tagSlugs(got))
	}
}

func TestSuggest_TagsNamedInTheContent(t *testing.T) {
	t.Parallel()
	in := library.SuggestInput{
		Tags: tags("kafka", "kafka-streams", "iceberg", "postgres", "go", "r", "spark"),
		Target: library.SuggestTarget{
			ID:    "new",
			Title: "Streaming into Iceberg",
			URL:   "https://example.com/posts/123",
			// postgres twice, Kafka Streams twice, spark once, "go" and "r"
			// many times as ordinary words.
			Text: "We read from Postgres and write with Kafka Streams. Postgres stays the source; " +
				"Kafka Streams does the joins. Spark was too heavy. Let's go, go, go. R r r.",
		},
	}

	got := tagSlugs(library.Suggest(in))

	// Kafka is named inside "Kafka Streams" twice, so it counts too.
	expected := []string{"iceberg", "kafka", "kafka-streams", "postgres"}
	slices.Sort(got)
	if !slices.Equal(got, expected) {
		t.Errorf("tags = %v, want %v", got, expected)
	}
}

func TestSuggest_ShortTagsOnlyFromTheTitle(t *testing.T) {
	t.Parallel()
	in := library.SuggestInput{
		Tags:   tags("go", "r"),
		Target: library.SuggestTarget{ID: "new", Title: "Profiling Go services", URL: "https://example.com/p", Text: "R r r"},
	}

	if got := tagSlugs(library.Suggest(in)); !slices.Equal(got, []string{"go"}) {
		t.Errorf("tags = %v, want [go]", got)
	}
}

func TestSuggest_TagsSharedBySimilarArticles(t *testing.T) {
	t.Parallel()
	in := shelf()
	// Not named in the content, but two of the similar articles carry it.
	in.Target = library.SuggestTarget{ID: "new", Title: "Exactly-once windowing", URL: "https://example.com/eo"}

	if got := tagSlugs(library.Suggest(in)); !slices.Contains(got, "kafka-streams") {
		t.Errorf("tags = %v, want kafka-streams from the similar articles", got)
	}
}

func TestSuggest_NeverATagTheArticleHasOrMoreThanFive(t *testing.T) {
	t.Parallel()
	in := library.SuggestInput{
		Tags: tags("alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf"),
		Target: library.SuggestTarget{
			ID: "new", Title: "alpha bravo charlie delta echo foxtrot golf", URL: "https://example.com/x",
			Tags: tags("Alpha"),
		},
	}

	got := tagSlugs(library.Suggest(in))
	if len(got) != 5 {
		t.Errorf("tags = %v, want 5", got)
	}
	if slices.Contains(got, "alpha") {
		t.Errorf("tags = %v, want no tag the article already has", got)
	}
}

func TestSuggest_IgnoresAddressesInTheText(t *testing.T) {
	t.Parallel()
	in := library.SuggestInput{
		Tags: tags("postgres"),
		Target: library.SuggestTarget{
			ID: "new", Title: "Notes", URL: "https://example.com/n",
			Text: "![diagram](https://img.example.com/postgres/postgres.png) see https://postgres.example.org/postgres",
		},
	}

	if got := tagSlugs(library.Suggest(in)); len(got) != 0 {
		t.Errorf("tags = %v, want none from image and link addresses", got)
	}
}

func TestSuggest_TheArticleIsNotItsOwnNeighbour(t *testing.T) {
	t.Parallel()
	in := shelf()
	// The article is in the library too, in Backend, with a title that
	// matches nothing else.
	in.Peers = append(in.Peers, peer("new", "Sourdough starters", &backend))
	in.Target = library.SuggestTarget{ID: "new", Title: "Sourdough starters", URL: "https://example.com/s"}

	if got := library.Suggest(in); got.Collection != nil {
		t.Errorf("collection = %q, want none: the article itself does not count", got.Collection.Slug)
	}
}
