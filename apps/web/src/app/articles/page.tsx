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

import { ActionIcon } from "../ActionIcon";
import { CollectionIcon, hueStyle } from "../CollectionIcon";
import { EmptyLibrary, LibraryError } from "../LibraryNotices";
import { TagLogo } from "../TagLogo";
import { SwipeActions } from "./SwipeActions";

const PAGE_SIZE = 25;

const sortLabels: Record<Sort, string> = {
  created_desc: "Newest first",
  created_asc: "Oldest first",
  title_asc: "Title, A to Z",
  title_desc: "Title, Z to A",
};

type TextFilter = "" | "complete" | "incomplete" | "failed" | "missing";

// read is "true" for read articles, "false" for unread ones, as in the API.
type ReadFilter = "" | "true" | "false";

const readLabels: Record<ReadFilter, string> = {
  "": "All",
  "false": "Unread",
  "true": "Read",
};

const textLabels: Record<Exclude<TextFilter, "">, string> = {
  complete: "with their text",
  incomplete: "with only part of their text",
  failed: "whose text could not be downloaded",
  missing: "never downloaded",
};

type Filters = {
  q: string;
  tags: string[];
  collection: string;
  text: TextFilter;
  read: ReadFilter;
  sort: Sort;
  cursor: string;
};

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

function readFilters(params: Record<string, string | string[] | undefined>): Filters {
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";
  const sort = one(params.sort);
  return {
    q: one(params.q).trim(),
    tags: [params.tag ?? []].flat().filter(Boolean),
    collection: one(params.collection).trim(),
    text: one(params.text) in textLabels ? (one(params.text) as TextFilter) : "",
    read: one(params.read) === "true" || one(params.read) === "false" ? (one(params.read) as ReadFilter) : "",
    sort: sort in sortLabels ? (sort as Sort) : "created_desc",
    cursor: one(params.cursor),
  };
}

/** Builds a list URL; overrides replace the current filters. */
function libraryHref(f: Filters, overrides: Partial<Filters> = {}): string {
  const next = { ...f, cursor: "", ...overrides };
  const qs = new URLSearchParams();
  if (next.q) qs.set("q", next.q);
  if (next.collection) qs.set("collection", next.collection);
  for (const t of next.tags) qs.append("tag", t);
  if (next.text) qs.set("text", next.text);
  if (next.read) qs.set("read", next.read);
  if (next.sort !== "created_desc") qs.set("sort", next.sort);
  if (next.cursor) qs.set("cursor", next.cursor);
  const s = qs.toString();
  return s ? `/articles?${s}` : "/articles";
}

// ArticlesPage is the list of articles, at /articles like the API's
// GET /api/articles, with the same filters in its query: q, collection, tag,
// text, sort and cursor. Without filters it lists every article.
export default async function ArticlesPage({ searchParams }: { searchParams: SearchParams }) {
  const filters = readFilters(await searchParams);
  const filtered =
    filters.q !== "" || filters.tags.length > 0 || filters.collection !== "" || filters.text !== "" || filters.read !== "";
  const cleared: Filters = { ...filters, q: "", tags: [], collection: "", text: "", read: "" };

  const qs = new URLSearchParams({ limit: String(PAGE_SIZE), sort: filters.sort });
  if (filters.q) qs.set("q", filters.q);
  if (filters.collection) qs.set("collection", filters.collection);
  for (const t of filters.tags) qs.append("tag", t);
  if (filters.text) qs.set("text", filters.text);
  if (filters.read) qs.set("read", filters.read);
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
      <div className="list-controls">
        <ReadLinks filters={filters} />
        <SortLinks filters={filters} />
      </div>

      {filters.q || filters.tags.length > 0 || filters.text ? (
        <div className="active-filters">
          <span>Showing</span>
          {filters.q ? <span>results for “{filters.q}”</span> : null}
          {filters.text ? (
            <Link className="tag" href={libraryHref(filters, { text: "" })}>
              articles {textLabels[filters.text]}
              <span aria-hidden="true">×</span>
              <span className="visually-hidden">Remove this filter</span>
            </Link>
          ) : null}
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

// SortLinks orders the list; the search box is in the page header.
function SortLinks({ filters }: { filters: Filters }) {
  return (
    <nav className="sort-links" aria-label="Order">
      {(Object.entries(sortLabels) as [Sort, string][]).map(([value, label]) => (
        <Link
          key={value}
          href={libraryHref(filters, { sort: value })}
          aria-current={filters.sort === value ? "true" : undefined}
        >
          {label}
        </Link>
      ))}
    </nav>
  );
}

// ReadLinks shows every article, only the unread ones or only the read ones.
function ReadLinks({ filters }: { filters: Filters }) {
  return (
    <nav className="sort-links" aria-label="Read or unread">
      {(["", "false", "true"] as ReadFilter[]).map((value) => (
        <Link
          key={value || "all"}
          href={libraryHref(filters, { read: value })}
          aria-current={filters.read === value ? "true" : undefined}
        >
          {readLabels[value]}
        </Link>
      ))}
    </nav>
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
    <header className="collection-hero" style={hueStyle(current ?? { name: slug, slug })}>
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
                <Link href={libraryHref({ ...filters, collection: c.slug, q: "" })} style={hueStyle(c)}>
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
                  <span className="side-panel-name">
                    <CollectionIcon name={c.name} className="side-panel-icon" />
                    {c.name}
                  </span>
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
                <span className="side-panel-name">
                  <TagLogo name={t.name} className="side-panel-icon" />
                  {t.name}
                </span>
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

// Entry is one article of a list: its title, a line of facts (collection,
// site, date saved, text problems) and its tags. The browser folder it came
// from is on the article's page. On a touch screen, swiping it to the left
// offers to delete it, and to the right to mark it read or unread.
function Entry({ article: a, filters }: { article: Article; filters: Filters }) {
  const saved = a.bookmarkedAt ?? a.createdAt;
  return (
    <SwipeActions id={a.id} title={a.title} readAt={a.readAt}>
      <Link href={`/articles/${a.id}`} className="entry-title">
        {a.title ? a.title : <span className="entry-untitled">Untitled bookmark</span>}
      </Link>
      <div className="entry-facts">
        {a.collection ? (
          <Link
            className="entry-collection"
            href={libraryHref(filters, { collection: a.collection.slug })}
            style={hueStyle(a.collection)}
          >
            <CollectionIcon name={a.collection.name} />
            {a.collection.name}
          </Link>
        ) : null}
        <span className="entry-source">{hostname(a.canonicalUrl || a.originalUrl)}</span>
        <span>
          Saved <time dateTime={saved}>{formatDate(saved)}</time>
        </span>
        {a.readAt ? (
          <span className="entry-read">
            <ActionIcon name="read" />
            Read
          </span>
        ) : null}
        {a.contentStatus === "incomplete" ? (
          <span className="badge badge-warn">Text incomplete</span>
        ) : a.contentStatus === "failed" ? (
          <span className="badge badge-bad">Text not downloaded</span>
        ) : null}
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
                <TagLogo name={t.name} className="tag-icon" />
                {t.name}
              </Link>
            </li>
          ))}
        </ul>
      ) : null}
    </SwipeActions>
  );
}
