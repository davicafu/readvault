import Link from "next/link";

import {
  ApiError,
  hostname,
  type Article,
  type ArticlePage,
  type Collection,
  type Sort,
  type TagCount,
} from "@/lib/api";
import { formatDate } from "@/lib/format";
import { apiGet } from "@/lib/server-api";

import { BrowseHome, hueStyle, type CollectionShelf } from "./BrowseHome";

const PAGE_SIZE = 25;
// The front page previews the latest articles of this many collections.
const PREVIEWED_COLLECTIONS = 24;
const PREVIEW_SIZE = 3;
const RECENT_SIZE = 6;

const sortLabels: Record<Sort, string> = {
  created_desc: "Newest first",
  created_asc: "Oldest first",
  title_asc: "Title, A to Z",
  title_desc: "Title, Z to A",
};

// all is set when the list was asked for without filters (view=all or an
// explicit order); without it and without filters the front page shows.
type Filters = { q: string; tags: string[]; collection: string; sort: Sort; cursor: string; all: boolean };

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

function readFilters(params: Record<string, string | string[] | undefined>): Filters {
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";
  const sort = one(params.sort);
  return {
    q: one(params.q).trim(),
    tags: [params.tag ?? []].flat().filter(Boolean),
    collection: one(params.collection).trim(),
    sort: sort in sortLabels ? (sort as Sort) : "created_desc",
    cursor: one(params.cursor),
    all: one(params.view) === "all" || one(params.sort) !== "",
  };
}

/** Builds a library URL; overrides replace the current filters. */
function libraryHref(f: Filters, overrides: Partial<Filters> = {}): string {
  const next = { ...f, cursor: "", ...overrides };
  const qs = new URLSearchParams();
  if (next.q) qs.set("q", next.q);
  if (next.collection) qs.set("collection", next.collection);
  for (const t of next.tags) qs.append("tag", t);
  if (next.sort !== "created_desc") qs.set("sort", next.sort);
  if (next.cursor) qs.set("cursor", next.cursor);
  if (next.all && qs.size === 0) qs.set("view", "all");
  const s = qs.toString();
  return s ? `/?${s}` : "/";
}

export default async function LibraryPage({ searchParams }: { searchParams: SearchParams }) {
  const filters = readFilters(await searchParams);
  const filtered = filters.q !== "" || filters.tags.length > 0 || filters.collection !== "";
  const cleared = { ...filters, q: "", tags: [], collection: "", all: false };

  if (!filtered && !filters.all && !filters.cursor) {
    return <FrontPage />;
  }

  const qs = new URLSearchParams({ limit: String(PAGE_SIZE), sort: filters.sort });
  if (filters.q) qs.set("q", filters.q);
  if (filters.collection) qs.set("collection", filters.collection);
  for (const t of filters.tags) qs.append("tag", t);
  if (filters.cursor) qs.set("cursor", filters.cursor);

  // The side panels are optional: the list still shows when they fail to load.
  const [listed, tagList, collectionList] = await Promise.allSettled([
    apiGet<ArticlePage>(`/api/articles?${qs}`),
    apiGet<{ tags: TagCount[] }>("/api/tags"),
    apiGet<{ collections: Collection[] }>("/api/collections"),
  ]);
  const page = listed.status === "fulfilled" ? listed.value : null;
  const failure =
    listed.status === "fulfilled"
      ? null
      : listed.reason instanceof ApiError
        ? listed.reason
        : new ApiError(0, "unreachable", "The ReadVault API is not reachable.");
  const tags = tagList.status === "fulfilled" ? tagList.value.tags : [];
  const collections = collectionList.status === "fulfilled" ? collectionList.value.collections : null;
  const activeCollection = collections?.find((c) => c.slug === filters.collection);

  return (
    <>
      {filters.collection ? (
        <CollectionHeader
          slug={filters.collection}
          current={activeCollection}
          collections={collections ?? []}
          filters={filters}
        />
      ) : (
        <h1 className="visually-hidden">Library</h1>
      )}
      <SearchForm filters={filters} withOrder />

      {filters.q || filters.tags.length > 0 ? (
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
          <Link href={libraryHref(cleared)}>Clear filters</Link>
        </div>
      ) : null}

      <div className="library">
        <div className="library-results">
          {failure ? (
            <LibraryError error={failure} firstPage={libraryHref(filters)} />
          ) : page && page.items.length === 0 ? (
            <EmptyLibrary filtered={filtered} clearHref={libraryHref(cleared)} />
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
        <aside className="library-side" aria-label="Organize">
          {filters.collection ? null : <CollectionPanel collections={collections} filters={filters} />}
          {tags.length > 0 ? <TagPanel tags={tags} filters={filters} /> : null}
        </aside>
      </div>
    </>
  );
}

// FrontPage is the library's home: the search box, then the collections to
// explore, the latest articles and the tags. Each part is optional, so a
// failed request hides that part only.
async function FrontPage() {
  const [collectionList, tagList, recentList] = await Promise.allSettled([
    apiGet<{ collections: Collection[] }>("/api/collections"),
    apiGet<{ tags: TagCount[] }>("/api/tags"),
    apiGet<ArticlePage>(`/api/articles?limit=${RECENT_SIZE}`),
  ]);
  if (recentList.status === "rejected") {
    const err = recentList.reason;
    return (
      <>
        <h1 className="visually-hidden">Library</h1>
        <SearchForm />
        <LibraryError
          error={err instanceof ApiError ? err : new ApiError(0, "unreachable", "The ReadVault API is not reachable.")}
          firstPage="/"
        />
      </>
    );
  }
  const recent = recentList.value.items;
  if (recent.length === 0) {
    return (
      <>
        <h1 className="visually-hidden">Library</h1>
        <EmptyLibrary filtered={false} clearHref="/" />
      </>
    );
  }

  let shelves: CollectionShelf[] | null = null;
  if (collectionList.status === "fulfilled") {
    const collections = collectionList.value.collections;
    const latest = await Promise.allSettled(
      collections.slice(0, PREVIEWED_COLLECTIONS).map((c) =>
        c.articles === 0
          ? Promise.resolve<ArticlePage>({ items: [], nextCursor: null })
          : apiGet<ArticlePage>(
              `/api/articles?${new URLSearchParams({ collection: c.slug, limit: String(PREVIEW_SIZE) })}`,
            ),
      ),
    );
    shelves = collections.map((collection, i) => {
      const r = latest[i];
      return { collection, latest: r?.status === "fulfilled" ? r.value.items : null };
    });
  }

  return (
    <>
      <h1 className="visually-hidden">Library</h1>
      <SearchForm />
      <BrowseHome shelves={shelves} recent={recent} tags={tagList.status === "fulfilled" ? tagList.value.tags : []} />
    </>
  );
}

function SearchForm({ filters, withOrder = false }: { filters?: Filters; withOrder?: boolean }) {
  return (
    <form className="finder" action="/" method="get" role="search">
      <div className="field">
        <label className="field-label" htmlFor="q">
          {filters?.collection ? "Search this collection" : "Search your library"}
        </label>
        <input
          className="input input-search"
          id="q"
          name="q"
          type="search"
          placeholder="Title, address or tag"
          defaultValue={filters?.q}
          maxLength={200}
          autoFocus={!filters}
        />
      </div>
      {withOrder ? (
        <div className="field">
          <label className="field-label" htmlFor="sort">
            Order
          </label>
          <select className="select" id="sort" name="sort" defaultValue={filters?.sort}>
            {Object.entries(sortLabels).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </div>
      ) : null}
      {filters?.collection ? <input type="hidden" name="collection" value={filters.collection} /> : null}
      {filters?.tags.map((t) => (
        <input key={t} type="hidden" name="tag" value={t} />
      ))}
      <button className="button" type="submit">
        Search
      </button>
    </form>
  );
}

// CollectionHeader names the open collection and lists the others, so the
// reader can move between collections without going back.
function CollectionHeader({
  slug,
  current,
  collections,
  filters,
}: {
  slug: string;
  current: Collection | undefined;
  collections: Collection[];
  filters: Filters;
}) {
  const others = collections.filter((c) => c.slug !== slug);
  return (
    <header className="collection-hero" style={hueStyle(current?.slug ?? slug)}>
      <p className="fine-print">
        <Link href="/">All collections</Link>
      </p>
      <h1 className="page-title">{current?.name ?? slug}</h1>
      <p className="collection-hero-count">
        {current
          ? `${current.articles} ${current.articles === 1 ? "article" : "articles"}`
          : "No collection has this name."}
      </p>
      {others.length > 0 ? (
        <nav aria-label="Other collections">
          <ul className="collection-hop">
            {others.map((c) => (
              <li key={c.id}>
                <Link href={libraryHref({ ...filters, collection: c.slug, q: "" })} style={hueStyle(c.slug)}>
                  {c.name}
                  <span className="tag-count">{c.articles}</span>
                </Link>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}
    </header>
  );
}

// CollectionPanel lists the collections with their number of articles.
// Selecting one shows only its articles; selecting it again shows them all.
function CollectionPanel({ collections, filters }: { collections: Collection[] | null; filters: Filters }) {
  return (
    <nav className="side-panel" aria-labelledby="collection-panel-heading">
      <div className="side-panel-head">
        <h2 id="collection-panel-heading">Collections</h2>
        <Link href="/collections">Manage</Link>
      </div>
      {collections === null ? (
        <p className="fine-print">The collections could not be loaded.</p>
      ) : collections.length === 0 ? (
        <p className="fine-print">
          <Link href="/collections">Create a collection</Link> to group articles.
        </p>
      ) : (
        <ul>
          {collections.map((c) => {
            const selected = filters.collection === c.slug;
            return (
              <li key={c.id}>
                <Link
                  href={libraryHref(filters, { collection: selected ? "" : c.slug })}
                  aria-current={selected ? "true" : undefined}
                >
                  <span className="side-panel-name">{c.name}</span>
                  <span className="side-panel-count">
                    {c.articles}
                    <span className="visually-hidden"> {c.articles === 1 ? "article" : "articles"}</span>
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </nav>
  );
}

// TagPanel lists every tag with its number of articles. Selecting a tag adds
// it to the filters; selecting a selected tag removes it.
function TagPanel({ tags, filters }: { tags: TagCount[]; filters: Filters }) {
  return (
    <nav className="side-panel" aria-labelledby="tag-panel-heading">
      <div className="side-panel-head">
        <h2 id="tag-panel-heading">Tags</h2>
      </div>
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
                <span className="side-panel-name">{t.name}</span>
                <span className="side-panel-count">
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
        {a.collection ? (
          <Link className="entry-collection" href={libraryHref(filters, { collection: a.collection.slug })}>
            {a.collection.name}
          </Link>
        ) : null}
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
