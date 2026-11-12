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
        <h2>C. A drawer that slides out</h2>
        <p className="fine-print">The front of a filing drawer; it slides out to show the index cards inside.</p>
        <ul className="lab-grid">
          {shown.map((c, i) => (
            <li key={c.id} className="lab-drawer" style={{ ...hueStyle(c), "--i": i } as CSSProperties}>
              <div className="lab-drawer-box">
                {latest[i].map((a, j) => (
                  <Link
                    key={a.id}
                    href={`/articles/${a.id}`}
                    className="lab-drawer-card"
                    style={{ "--j": j } as CSSProperties}
                  >
                    {a.title || "Untitled bookmark"}
                  </Link>
                ))}
              </div>
              <Link className="lab-drawer-front" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                <span className="lab-drawer-label">
                  <CollectionIcon name={c.name} />
                  {c.name}
                </span>
                <span className="lab-drawer-handle" aria-hidden="true" />
                <span className="lab-drawer-count">{c.articles} articles</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="lab-section">
        <h2>D. A deck of cards</h2>
        <p className="fine-print">The collection on top of its articles, stacked like cards; they fan out like a hand.</p>
        <ul className="lab-grid">
          {shown.map((c, i) => (
            <li key={c.id} className="lab-deck" style={{ ...hueStyle(c), "--i": i } as CSSProperties}>
              {latest[i].map((a, j) => (
                <Link
                  key={a.id}
                  href={`/articles/${a.id}`}
                  className="lab-deck-card"
                  style={{ "--j": j, "--n": latest[i].length } as CSSProperties}
                >
                  <span>{a.title || "Untitled bookmark"}</span>
                  <small>{hostname(a.canonicalUrl || a.originalUrl)}</small>
                </Link>
              ))}
              <Link className="lab-deck-top" href={`/?collection=${encodeURIComponent(c.slug)}`}>
                <span className="lab-book-icon">
                  <CollectionIcon name={c.name} />
                </span>
                <span className="lab-folder-title">{c.name}</span>
                <span className="lab-folder-count">{c.articles} articles</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}
