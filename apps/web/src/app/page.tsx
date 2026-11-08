import Link from "next/link";

import { ApiError, hostname, type Article, type ArticlePage, type Sort, type TagCount } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { apiGet } from "@/lib/server-api";

const PAGE_SIZE = 25;

const sortLabels: Record<Sort, string> = {
  created_desc: "Newest first",
  created_asc: "Oldest first",
  title_asc: "Title, A to Z",
  title_desc: "Title, Z to A",
};

type Filters = { q: string; tags: string[]; sort: Sort; cursor: string };

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

function readFilters(params: Record<string, string | string[] | undefined>): Filters {
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";
  const sort = one(params.sort);
  return {
    q: one(params.q).trim(),
    tags: [params.tag ?? []].flat().filter(Boolean),
    sort: sort in sortLabels ? (sort as Sort) : "created_desc",
    cursor: one(params.cursor),
  };
}

/** Builds a library URL; overrides replace the current filters. */
function libraryHref(f: Filters, overrides: Partial<Filters> = {}): string {
  const next = { ...f, cursor: "", ...overrides };
  const qs = new URLSearchParams();
  if (next.q) qs.set("q", next.q);
  for (const t of next.tags) qs.append("tag", t);
  if (next.sort !== "created_desc") qs.set("sort", next.sort);
  if (next.cursor) qs.set("cursor", next.cursor);
  const s = qs.toString();
  return s ? `/?${s}` : "/";
}

export default async function LibraryPage({ searchParams }: { searchParams: SearchParams }) {
  const filters = readFilters(await searchParams);
  const filtered = filters.q !== "" || filters.tags.length > 0;

  const qs = new URLSearchParams({ limit: String(PAGE_SIZE), sort: filters.sort });
  if (filters.q) qs.set("q", filters.q);
  for (const t of filters.tags) qs.append("tag", t);
  if (filters.cursor) qs.set("cursor", filters.cursor);

  // The tag panel is optional: the list still shows when tags fail to load.
  const [listed, tagList] = await Promise.allSettled([
    apiGet<ArticlePage>(`/api/articles?${qs}`),
    apiGet<{ tags: TagCount[] }>("/api/tags"),
  ]);
  const page = listed.status === "fulfilled" ? listed.value : null;
  const failure =
    listed.status === "fulfilled"
      ? null
      : listed.reason instanceof ApiError
        ? listed.reason
        : new ApiError(0, "unreachable", "The ReadVault API is not reachable.");
  const tags = tagList.status === "fulfilled" ? tagList.value.tags : [];

  return (
    <>
      <h1 className="visually-hidden">Library</h1>
      <form className="finder" action="/" method="get" role="search">
        <div className="field">
          <label className="field-label" htmlFor="q">
            Search your library
          </label>
          <input
            className="input input-search"
            id="q"
            name="q"
            type="search"
            placeholder="Title, address or tag"
            defaultValue={filters.q}
            maxLength={200}
            autoFocus={!filtered}
          />
        </div>
        <div className="field">
          <label className="field-label" htmlFor="sort">
            Order
          </label>
          <select className="select" id="sort" name="sort" defaultValue={filters.sort}>
            {Object.entries(sortLabels).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </div>
        {filters.tags.map((t) => (
          <input key={t} type="hidden" name="tag" value={t} />
        ))}
        <button className="button" type="submit">
          Search
        </button>
      </form>

      {filtered ? (
        <div className="active-filters">
          <span>Showing</span>
          {filters.q ? <span>results for “{filters.q}”</span> : null}
          {filters.tags.map((t) => (
            <Link key={t} className="tag" href={libraryHref(filters, { tags: filters.tags.filter((x) => x !== t) })}>
              {t}
              <span aria-hidden="true">×</span>
              <span className="visually-hidden">Remove the {t} filter</span>
            </Link>
          ))}
          <Link href={libraryHref({ ...filters, q: "", tags: [] })}>Clear filters</Link>
        </div>
      ) : null}

      <div className="library">
        <div className="library-results">
          {failure ? (
            <LibraryError error={failure} firstPage={libraryHref(filters)} />
          ) : page && page.items.length === 0 ? (
            <EmptyLibrary filtered={filtered} clearHref={libraryHref({ ...filters, q: "", tags: [] })} />
          ) : page ? (
            <>
              <ol className="shelf">
                {page.items.map((a) => (
                  <Entry key={a.id} article={a} filters={filters} />
                ))}
              </ol>
              <nav className="pager" aria-label="Pages">
                {filters.cursor ? (
                  <Link className="button button-quiet" href={libraryHref(filters)}>
                    First page
                  </Link>
                ) : null}
                {page.nextCursor ? (
                  <Link className="button button-quiet" href={libraryHref(filters, { cursor: page.nextCursor })}>
                    Next page
                  </Link>
                ) : null}
              </nav>
            </>
          ) : null}
        </div>
        {tags.length > 0 ? <TagPanel tags={tags} filters={filters} /> : null}
      </div>
    </>
  );
}

// TagPanel lists every tag with its number of articles. Selecting a tag adds
// it to the filters; selecting a selected tag removes it.
function TagPanel({ tags, filters }: { tags: TagCount[]; filters: Filters }) {
  return (
    <nav className="tag-panel" aria-labelledby="tag-panel-heading">
      <h2 id="tag-panel-heading">Tags</h2>
      <ul>
        {tags.map((t) => {
          const selected = filters.tags.includes(t.slug);
          return (
            <li key={t.slug}>
              <Link
                href={libraryHref(filters, {
                  tags: selected ? filters.tags.filter((x) => x !== t.slug) : [...filters.tags, t.slug],
                })}
                aria-current={selected ? "true" : undefined}
              >
                <span className="tag-panel-name">{t.name}</span>
                <span className="tag-panel-count">
                  {t.articles}
                  <span className="visually-hidden"> {t.articles === 1 ? "article" : "articles"}</span>
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function Entry({ article: a, filters }: { article: Article; filters: Filters }) {
  return (
    <li className="entry">
      <span className="entry-source">{hostname(a.canonicalUrl || a.originalUrl)}</span>
      <div>
        <Link href={`/articles/${a.id}`} className="entry-title">
          {a.title ? a.title : <span className="entry-untitled">Untitled bookmark</span>}
        </Link>
        {a.contentStatus === "incomplete" ? (
          <span className="badge badge-warn">Text incomplete</span>
        ) : a.contentStatus === "failed" ? (
          <span className="badge badge-bad">Text not downloaded</span>
        ) : null}
        <div className="entry-facts">
          {a.folder.length > 0 ? (
            <ul className="folder-path" aria-label="Original folder">
              {a.folder.map((f, i) => (
                <li key={`${i}-${f}`}>{f}</li>
              ))}
            </ul>
          ) : null}
          <span>
            Saved <time dateTime={a.bookmarkedAt ?? a.createdAt}>{formatDate(a.bookmarkedAt ?? a.createdAt)}</time>
          </span>
        </div>
        {a.tags.length > 0 ? (
          <ul className="tags" aria-label="Tags">
            {a.tags.map((t) => (
              <li key={t.slug}>
                <Link
                  className="tag"
                  href={libraryHref(filters, {
                    tags: filters.tags.includes(t.slug) ? filters.tags : [...filters.tags, t.slug],
                  })}
                >
                  {t.name}
                </Link>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </li>
  );
}

function EmptyLibrary({ filtered, clearHref }: { filtered: boolean; clearHref: string }) {
  if (filtered) {
    return (
      <section className="notice">
        <h2>No articles match these filters</h2>
        <p>Try fewer words or remove a tag.</p>
        <Link className="button button-quiet" href={clearHref}>
          Clear filters
        </Link>
      </section>
    );
  }
  return (
    <section className="notice">
      <h2>Your library is empty</h2>
      <p>Save a link, or export your bookmarks from Chrome, Firefox, Safari or Edge as an HTML file and import them.</p>
      <Link className="button" href="/add">
        Add articles
      </Link>
    </section>
  );
}

function LibraryError({ error, firstPage }: { error: ApiError; firstPage: string }) {
  const invalidQuery = error.code === "invalid_query";
  return (
    <section className="notice notice-error" role="alert">
      <h2>{invalidQuery ? "This list address is not valid" : "The library could not be loaded"}</h2>
      <p>
        {invalidQuery
          ? (error.fieldProblems[0]?.message ?? error.message)
          : error.code === "unreachable"
            ? "The ReadVault API is not running or not reachable. Start it with make dev and reload this page."
            : "The server could not answer this time. Reload the page to try again."}
      </p>
      <Link className="button button-quiet" href={invalidQuery ? firstPage : "/"}>
        {invalidQuery ? "Go to the first page" : "Reload the library"}
      </Link>
    </section>
  );
}
