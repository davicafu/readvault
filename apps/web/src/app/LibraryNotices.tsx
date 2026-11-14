import Link from "next/link";

import type { ApiError } from "@/lib/api";

// The notices shared by the front page and the list of articles.

export function EmptyLibrary({ filtered, clearHref }: { filtered: boolean; clearHref: string }) {
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
      <p>Add one or more links, or export your bookmarks from Chrome, Firefox, Safari or Edge as an HTML file and import them.</p>
      <Link className="button" href="/add">
        Add articles
      </Link>
    </section>
  );
}

export function LibraryError({ error, firstPage }: { error: ApiError; firstPage: string }) {
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
