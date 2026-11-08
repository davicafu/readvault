import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { cache } from "react";

import { brand } from "@/config/brand";
import { ApiError, isWebUrl, type Article, type ArticleContent } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { apiGet } from "@/lib/server-api";

import { ArticleEditor } from "./ArticleEditor";
import { ArticleText } from "./ArticleText";

type Params = Promise<{ id: string }>;

type Loaded = { article: Article; error?: never } | { article?: never; error: ApiError };

// Metadata and the page share one request per render. A missing article is a
// 404 page; any other failure is shown on the page instead of a server error.
const loadArticle = cache(async (id: string): Promise<Loaded> => {
  try {
    return { article: await apiGet<Article>(`/api/articles/${encodeURIComponent(id)}`) };
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) notFound();
    return {
      error: err instanceof ApiError ? err : new ApiError(0, "unreachable", "The ReadVault API is not reachable."),
    };
  }
});

// loadContent returns the saved text, or null when none was saved. Other
// failures leave the text out; the article itself still shows.
async function loadContent(id: string): Promise<ArticleContent | null | undefined> {
  try {
    return await apiGet<ArticleContent>(`/api/articles/${encodeURIComponent(id)}/content`);
  } catch (err) {
    if (err instanceof ApiError && err.code === "content_not_found") return null;
    return undefined;
  }
}

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const { article } = await loadArticle((await params).id);
  return { title: article ? article.title || "Untitled bookmark" : "Article unavailable" };
}

export default async function ArticlePage({ params }: { params: Params }) {
  const { id } = await params;
  const [{ article: a, error }, content] = await Promise.all([loadArticle(id), loadContent(id)]);
  if (!a) return <ArticleError error={error} />;

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
        {content === undefined ? (
          <p className="field-error" role="alert">
            The saved text could not be loaded. Reload the page to try again.
          </p>
        ) : (
          <ArticleText articleId={a.id} articleUrl={a.originalUrl} content={content} />
        )}
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
        {a.tagsEditedAt ? (
          <div>
            <dt>Tags</dt>
            <dd>Edited by you on {formatDate(a.tagsEditedAt)}; imports add none</dd>
          </div>
        ) : null}
      </dl>
    </article>
  );
}

function ArticleError({ error }: { error: ApiError }) {
  return (
    <section className="notice notice-error" role="alert">
      <h2>The article could not be loaded</h2>
      <p>
        {error.code === "unreachable"
          ? `The ${brand.name} API is not running or not reachable. Start it with make dev and reload this page.`
          : "The server could not answer this time. Reload the page to try again."}
      </p>
      <Link className="button button-quiet" href="/">
        Back to the library
      </Link>
    </section>
  );
}
