import Link from "next/link";
import { Suspense, type CSSProperties } from "react";

import { hostname, type Article, type Collection, type TagCount } from "@/lib/api";

import { CollectionIcon, hueStyle } from "./CollectionIcon";
import { PixelFilters } from "./PixelFilters";
import { TagLogo } from "./TagLogo";

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
          <div className="shelf-card-empty">
            <h3>Group your articles</h3>
            <p>Collections keep related reading together. Each article lives in one collection and keeps its tags.</p>
            <Link className="button" href="/collections">
              Create a collection
            </Link>
          </div>
        ) : (
          <>
            <PixelFilters />
            <ul className="shelf-grid">
              {shelves.map((s, i) => (
                <FolderCard key={s.collection.id} shelf={s} index={i} />
              ))}
            </ul>
          </>
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
                  <TagLogo name={t.name} className="tag-icon" />
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

// FolderCard is a collection as a folder: its latest articles stick out of
// it as sheets (streamed in when they load) and the front opens the
// collection.
function FolderCard({ shelf: { collection: c, latest }, index }: { shelf: CollectionShelf; index: number }) {
  return (
    <li className="folder" style={{ ...hueStyle(c), "--i": index } as CSSProperties}>
      <span className="folder-glow" aria-hidden="true" />
      <span className="folder-back" aria-hidden="true" />
      <Suspense fallback={null}>
        <FolderSheets collection={c} latest={latest} />
      </Suspense>
      <Link className="folder-front" href={`/?collection=${encodeURIComponent(c.slug)}`}>
        <CollectionIcon name={c.name} className="folder-mark" />
        <span className="folder-icon" aria-hidden="true">
          <CollectionIcon name={c.name} />
        </span>
        <span className="folder-title">{c.name}</span>
        <span className="folder-count">
          {c.articles} {c.articles === 1 ? "article" : "articles"}
        </span>
      </Link>
    </li>
  );
}

async function FolderSheets({ collection: c, latest }: { collection: Collection; latest: Promise<Article[] | null> }) {
  const articles = await latest;
  if (!articles || articles.length === 0) return null;
  return (
    <ul className="folder-sheets" aria-label={`Latest in ${c.name}`}>
      {articles.map((a, j) => (
        <li key={a.id} style={{ "--j": j } as CSSProperties}>
          <Link className="folder-sheet" href={`/articles/${a.id}`}>
            {a.title || "Untitled bookmark"}
          </Link>
        </li>
      ))}
    </ul>
  );
}
