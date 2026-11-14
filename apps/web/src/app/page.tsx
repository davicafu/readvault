import { permanentRedirect } from "next/navigation";

import { ApiError, type ArticlePage, type Collection, type TagCount } from "@/lib/api";
import { apiGet } from "@/lib/server-api";

import { BrowseHome, type CollectionShelf } from "./BrowseHome";
import { EmptyLibrary, LibraryError } from "./LibraryNotices";

// The front page previews the latest articles of this many collections.
const PREVIEWED_COLLECTIONS = 24;
const PREVIEW_SIZE = 3;
const RECENT_SIZE = 6;

// The query of the list of articles, which used to live at /.
const LIST_PARAMS = ["q", "collection", "tag", "text", "sort", "cursor"];

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

/**
 * Where an old list address (/?view=all, /?collection=…) now lives, or null
 * when the address is the front page.
 */
function movedList(params: Record<string, string | string[] | undefined>): string | null {
  const qs = new URLSearchParams();
  for (const key of LIST_PARAMS) {
    for (const v of [params[key] ?? []].flat()) qs.append(key, v);
  }
  if (qs.size === 0 && params.view !== "all") return null;
  const s = qs.toString();
  return s ? `/articles?${s}` : "/articles";
}

export default async function HomePage({ searchParams }: { searchParams: SearchParams }) {
  const moved = movedList(await searchParams);
  if (moved) permanentRedirect(moved);
  return <FrontPage />;
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

  // The cards render as soon as the collections are known; each card's
  // latest articles stream in when their request answers.
  let shelves: CollectionShelf[] | null = null;
  if (collectionList.status === "fulfilled") {
    shelves = collectionList.value.collections.map((collection, i) => ({
      collection,
      latest:
        i >= PREVIEWED_COLLECTIONS
          ? Promise.resolve(null)
          : collection.articles === 0
            ? Promise.resolve([])
            : apiGet<ArticlePage>(
                `/api/articles?${new URLSearchParams({ collection: collection.slug, limit: String(PREVIEW_SIZE) })}`,
              ).then(
                (p) => p.items,
                () => null,
              ),
    }));
  }

  return (
    <>
      <h1 className="visually-hidden">Library</h1>
      <BrowseHome shelves={shelves} recent={recent} tags={tagList.status === "fulfilled" ? tagList.value.tags : []} />
    </>
  );
}

