import Link from "next/link";
import { Suspense, type CSSProperties } from "react";

import { hostname, type Article, type Collection, type TagCount } from "@/lib/api";

import { CollectionIcon, hueStyle, TagIcon } from "./CollectionIcon";

// latest resolves to the collection's newest articles, or null when they
// were not loaded.
export type CollectionShelf = { collection: Collection; latest: Promise<Article[] | null> };


// BrowseHome is the library's front page when nothing is searched: the
// collections as cards to explore, the latest saved articles and the tags.
export function BrowseHome({
  shelves,
  recent,
  tags,
}: {
  // null when the collections could not be loaded.
  shelves: CollectionShelf[] | null;
  recent: Article[];
  tags: TagCount[];
}) {
  return (
    <>
      <section className="browse-section" aria-labelledby="collections-heading">
        <div className="browse-head">
          <h2 id="collections-heading">Collections</h2>
        </div>
        {shelves === null ? (
          <p className="fine-print">The collections could not be loaded. Reload the page to try again.</p>
        ) : shelves.length === 0 ? (
          <div className="shelf-card shelf-card-empty">
            <h3>Group your articles</h3>
            <p>Collections keep related reading together. Each article lives in one collection and keeps its tags.</p>
            <Link className="button" href="/collections">
              Create a collection
            </Link>
          </div>
        ) : (
          <ul className="shelf-grid">
            {shelves.map((s, i) => (
              <ShelfCard key={s.collection.id} shelf={s} index={i} />
            ))}
          </ul>
        )}
      </section>

      {recent.length > 0 ? (
        <section className="browse-section" aria-labelledby="recent-heading">
          <div className="browse-head">
            <h2 id="recent-heading">Recently added</h2>
            <Link href="/?view=all">See all</Link>
          </div>
          <ul className="recent-grid">
            {recent.map((a) => (
              <li key={a.id} className="recent-item">
                <span className="recent-source">{hostname(a.canonicalUrl || a.originalUrl)}</span>
                <Link href={`/articles/${a.id}`} className="recent-title">
                  {a.title || "Untitled bookmark"}
                </Link>
                {a.collection ? (
                  <Link
                    className="recent-collection"
                    href={`/?collection=${encodeURIComponent(a.collection.slug)}`}
                    style={hueStyle(a.collection)}
                  >
                    {a.collection.name}
                  </Link>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {tags.length > 0 ? (
        <section className="browse-section" aria-labelledby="tags-heading">
          <div className="browse-head">
            <h2 id="tags-heading">Tags</h2>
          </div>
          <ul className="tag-cloud">
            {tags.map((t) => (
              <li key={t.slug}>
                <Link className="tag" href={`/?tag=${encodeURIComponent(t.slug)}`}>
                  <TagIcon name={t.name} className="tag-icon" />
                  {t.name}
                  <span className="tag-count">
                    {t.articles}
                    <span className="visually-hidden"> {t.articles === 1 ? "article" : "articles"}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </>
  );
}

function ShelfCard({ shelf: { collection: c, latest }, index }: { shelf: CollectionShelf; index: number }) {
  const href = `/?collection=${encodeURIComponent(c.slug)}`;
  return (
    <li className="shelf-card" style={{ ...hueStyle(c), "--i": index } as CSSProperties}>
      <div className="shelf-cover">
        <CollectionIcon name={c.name} className="shelf-cover-mark" />
        <span className="shelf-cover-icon">
          <CollectionIcon name={c.name} />
        </span>
        <h3>
          <Link href={href}>{c.name}</Link>
        </h3>
        <span className="shelf-card-count">
          {c.articles} {c.articles === 1 ? "article" : "articles"}
        </span>
      </div>
      <Suspense fallback={<ShelfLatestPlaceholder />}>
        <ShelfLatest collection={c} latest={latest} />
      </Suspense>
      {c.articles > 0 ? (
        <Link className="shelf-card-more" href={href}>
          Browse {c.name}
        </Link>
      ) : null}
    </li>
  );
}

async function ShelfLatest({ collection: c, latest }: { collection: Collection; latest: Promise<Article[] | null> }) {
  const articles = await latest;
  if (!articles || articles.length === 0) {
    return (
      <p className="shelf-card-note">
        {c.articles === 0 ? "No articles yet. Choose this collection on an article's page." : "Open it to see its articles."}
      </p>
    );
  }
  return (
    <ul className="shelf-card-list" aria-label={`Latest in ${c.name}`}>
      {articles.map((a, i) => (
        <li key={a.id} style={{ "--j": i } as CSSProperties}>
          <Link href={`/articles/${a.id}`}>{a.title || "Untitled bookmark"}</Link>
          <span className="shelf-card-source">{hostname(a.canonicalUrl || a.originalUrl)}</span>
        </li>
      ))}
    </ul>
  );
}

// Holds the list's place while it loads, so the card keeps its height.
function ShelfLatestPlaceholder() {
  return (
    <ul className="shelf-card-list shelf-card-list-loading" aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <li key={i}>
          <span className="skeleton skeleton-line" />
          <span className="skeleton skeleton-line skeleton-short" />
        </li>
      ))}
    </ul>
  );
}
