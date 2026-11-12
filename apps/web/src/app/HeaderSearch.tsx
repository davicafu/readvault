"use client";

import { usePathname, useSearchParams } from "next/navigation";
import { useEffect, useRef } from "react";

// HeaderSearch is the library's search box, in the page header on every
// page. On the library it keeps the filters in use (collection, tags, text
// state and order), so a search inside a collection stays in it. Press "/"
// anywhere outside a field to jump to it.
export function HeaderSearch() {
  const pathname = usePathname();
  const params = useSearchParams();
  const input = useRef<HTMLInputElement>(null);
  const onLibrary = pathname === "/";
  const q = onLibrary ? (params.get("q") ?? "") : "";
  const collection = onLibrary ? params.get("collection") : null;

  // The box keeps what was searched when the page changes under it.
  useEffect(() => {
    if (input.current && document.activeElement !== input.current) input.current.value = q;
  }, [q]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
      e.preventDefault();
      input.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <form className="header-search" action="/" method="get" role="search">
      <label className="visually-hidden" htmlFor="q">
        {collection ? "Search this collection" : "Search your library"}
      </label>
      <svg className="header-search-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <circle cx="11" cy="11" r="6.5" />
        <path d="M16 16l4.5 4.5" />
      </svg>
      <input
        ref={input}
        className="header-search-input"
        id="q"
        name="q"
        type="search"
        placeholder={collection ? "Search this collection" : "Title, address or tag"}
        defaultValue={q}
        maxLength={200}
        autoComplete="off"
      />
      <kbd className="header-search-key" aria-hidden="true">
        /
      </kbd>
      {onLibrary ? (
        <>
          {collection ? <input type="hidden" name="collection" value={collection} /> : null}
          {params.get("text") ? <input type="hidden" name="text" value={params.get("text") ?? ""} /> : null}
          {params.get("sort") ? <input type="hidden" name="sort" value={params.get("sort") ?? ""} /> : null}
          {params.getAll("tag").map((t) => (
            <input key={t} type="hidden" name="tag" value={t} />
          ))}
        </>
      ) : null}
    </form>
  );
}
