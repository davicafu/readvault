import type { Metadata } from "next";
import Link from "next/link";
import type { CSSProperties } from "react";

import { hostname, type Article, type ArticlePage, type Collection } from "@/lib/api";
import { apiGet } from "@/lib/server-api";

import { CollectionIcon, hueStyle } from "../../CollectionIcon";
import "./lab.css";

export const metadata: Metadata = { title: "Card designs" };

// A prototype to compare collection card designs with the real library.
// Not linked from the menu; the chosen design replaces the front page cards.
export default async function ShelvesLab() {
  const { collections } = await apiGet<{ collections: Collection[] }>("/api/collections");
  const shown = collections.slice(0, 6);
  const latest = await Promise.all(
    shown.map((c) =>
      apiGet<ArticlePage>(`/api/articles?${new URLSearchParams({ collection: c.slug, limit: "3" })}`).then(
        (p) => p.items,
        () => [] as Article[],
      ),
    ),
  );

  return (
    <>
      <div className="page-head">
        <p className="fine-print">
          <Link href="/">Library</Link>
        </p>
        <h1 className="page-title">Card designs</h1>
        <p className="lede">Spare designs for a collection card; the folder is on the front page. Hover or focus a card to see it move.</p>
      </div>

      <section className="lab-section">
        <h2>Original</h2>
        <p className="fine-print">The front page card before the folders: a cover in the collection&apos;s colours over its latest articles.</p>
        <ul className="lab-orig-grid">
          {shown.map((c, i) => (
            <li key={c.id} className="orig-card" style={{ ...hueStyle(c), "--i": i } as CSSProperties}>
              <div className="orig-cover">
                <CollectionIcon name={c.name} className="orig-cover-mark" />
                <span className="orig-cover-icon">
                  <CollectionIcon name={c.name} />
                </span>
                <h3>
                  <Link href={`/?collection=${encodeURIComponent(c.slug)}`}>{c.name}</Link>
                </h3>
                <span className="orig-card-count">{c.articles} articles</span>
              </div>
              <ul className="orig-card-list">
                {latest[i].map((a) => (
                  <li key={a.id}>
                    <Link href={`/articles/${a.id}`}>{a.title || "Untitled bookmark"}</Link>
                    <span className="orig-card-source">{hostname(a.canonicalUrl || a.originalUrl)}</span>
                  </li>
                ))}
              </ul>
              <Link className="orig-card-more" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                Browse {c.name}
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="lab-section">
        <h2>A. A book that opens</h2>
        <p className="fine-print">The cover swings open on its spine and shows the first page: the latest articles.</p>
        <ul className="lab-grid">
          {shown.map((c, i) => (
            <li key={c.id} className="lab-book" style={{ ...hueStyle(c), "--i": i } as CSSProperties}>
              <div className="lab-book-page">
                <ol>
                  {latest[i].map((a) => (
                    <li key={a.id}>
                      <Link href={`/articles/${a.id}`}>{a.title || "Untitled bookmark"}</Link>
                      <span>{hostname(a.canonicalUrl || a.originalUrl)}</span>
                    </li>
                  ))}
                </ol>
                <Link className="lab-more" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                  Browse all {c.articles}
                </Link>
              </div>
              <Link className="lab-book-cover" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                <CollectionIcon name={c.name} className="lab-book-mark" />
                <span className="lab-book-icon">
                  <CollectionIcon name={c.name} />
                </span>
                <span className="lab-book-title">{c.name}</span>
                <span className="lab-book-count">{c.articles} articles</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="lab-section">
        <h2>E. A record and its sleeve</h2>
        <p className="fine-print">The collection is a record sleeve with its latest articles as the track list; the record slides out, spinning, with the icon on its label.</p>
        <ul className="lab-grid">
          {shown.map((c, i) => (
            <li key={c.id} className="lab-vinyl" style={{ ...hueStyle(c), "--i": i } as CSSProperties}>
              <span className="lab-vinyl-disc" aria-hidden="true">
                <span className="lab-vinyl-label">
                  <CollectionIcon name={c.name} />
                </span>
              </span>
              <Link className="lab-vinyl-sleeve" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                <span className="lab-folder-title">{c.name}</span>
                <ol className="lab-vinyl-tracks">
                  {latest[i].map((a, j) => (
                    <li key={a.id}>
                      <span>{String(j + 1).padStart(2, "0")}</span>
                      <span>{a.title || "Untitled bookmark"}</span>
                    </li>
                  ))}
                </ol>
                <span className="lab-folder-count">{c.articles} articles</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="lab-section">
        <h2>F. Holographic glass</h2>
        <p className="fine-print">A frosted card with an iridescent rim that keeps turning; it tilts in 3D and the icon floats up when you reach for it.</p>
        <ul className="lab-grid">
          {shown.map((c, i) => (
            <li key={c.id} className="lab-holo-wrap" style={{ ...hueStyle(c), "--i": i } as CSSProperties}>
              <Link className="lab-holo" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                <span className="lab-holo-icon" aria-hidden="true">
                  <CollectionIcon name={c.name} />
                </span>
                <span>
                  <span className="lab-folder-title">{c.name}</span>
                  <br />
                  <span className="lab-folder-count">{c.articles} articles</span>
                </span>
                <ul>
                  {latest[i].map((a) => (
                    <li key={a.id}>{a.title || "Untitled bookmark"}</li>
                  ))}
                </ul>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}
