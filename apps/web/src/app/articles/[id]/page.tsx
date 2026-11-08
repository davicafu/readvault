import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { cache } from "react";

import { brand } from "@/config/brand";
import { ApiError, isWebUrl, type Article } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { apiGet } from "@/lib/server-api";

import { ArticleEditor } from "./ArticleEditor";

type Params = Promise<{ id: string }>;

// Metadata and the page share one request per render.
const loadArticle = cache(async (id: string): Promise<Article> => {
  try {
    return await apiGet<Article>(`/api/articles/${encodeURIComponent(id)}`);
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) notFound();
    throw err;
  }
});

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const article = await loadArticle((await params).id);
  return { title: article.title || "Untitled bookmark" };
}

export default async function ArticlePage({ params }: { params: Params }) {
  const a = await loadArticle((await params).id);

  return (
    <article className="article">
      <div>
        <p className="fine-print">
          <Link href="/">Library</Link>
        </p>
        <h1 className="article-title">{a.title || "Untitled bookmark"}</h1>
        {isWebUrl(a.originalUrl) ? (
          <a className="article-link" href={a.originalUrl} target="_blank" rel="noopener noreferrer">
            Open the original article
          </a>
        ) : null}
        <ArticleEditor id={a.id} title={a.title} tags={a.tags.map((t) => t.name)} />
      </div>

      <dl className="facts">
        <div>
          <dt>Address</dt>
          <dd>{a.originalUrl}</dd>
        </div>
        {a.canonicalUrl !== a.originalUrl ? (
          <div>
            <dt>Address used to spot duplicates</dt>
            <dd>{a.canonicalUrl}</dd>
          </div>
        ) : null}
        <div>
          <dt>Original folder</dt>
          <dd>{a.folder.length > 0 ? a.folder.join(" / ") : "None"}</dd>
        </div>
        <div>
          <dt>Bookmarked in the browser</dt>
          <dd>{a.bookmarkedAt ? formatDate(a.bookmarkedAt) : "Unknown"}</dd>
        </div>
        <div>
          <dt>Added to {brand.name}</dt>
          <dd>{formatDate(a.createdAt)}</dd>
        </div>
        <div>
          <dt>Last changed</dt>
          <dd>{formatDate(a.updatedAt)}</dd>
        </div>
        {a.titleEditedAt ? (
          <div>
            <dt>Title</dt>
            <dd>Edited by you on {formatDate(a.titleEditedAt)}; imports keep it</dd>
          </div>
        ) : null}
      </dl>
    </article>
  );
}
