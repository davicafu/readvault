"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useId, useRef, useState, type FormEvent, type KeyboardEvent } from "react";

import type { Collection } from "@/lib/api";

import { CollectionIcon, hueStyle } from "./CollectionIcon";

type Picked = { slug: string; name: string };

// The ":word" being typed at the caret, if any: start index and the word.
function collectionToken(text: string, caret: number): { start: number; word: string } | null {
  const m = /(^|\s):([^\s:]*)$/.exec(text.slice(0, caret));
  return m ? { start: caret - m[2].length - 1, word: m[2] } : null;
}

const fold = (s: string) =>
  s
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "");

function matching(collections: Collection[], word: string): Collection[] {
  const w = fold(word);
  if (!w) return collections;
  const starts = collections.filter((c) => fold(c.name).startsWith(w) || c.slug.startsWith(w));
  const contains = collections.filter((c) => !starts.includes(c) && fold(c.name).includes(w));
  return [...starts, ...contains];
}

// HeaderSearch is the library's search box, in the page header on every
// page. Type ":" and a collection's name to search in that collection;
// "-word" leaves out articles with a word and "exact words" in quotes match
// a phrase (the API's search does both). On the library it keeps the
// filters in use. Press "/" anywhere outside a field to jump to it.
export function HeaderSearch() {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const onLibrary = pathname === "/articles";
  const listId = useId();
  const input = useRef<HTMLInputElement>(null);

  const [text, setText] = useState(onLibrary ? (params.get("q") ?? "") : "");
  const [picked, setPicked] = useState<Picked | null>(null);
  const [collections, setCollections] = useState<Collection[] | null>(null);
  const [token, setToken] = useState<{ start: number; word: string } | null>(null);
  const [active, setActive] = useState(0);
  const [focused, setFocused] = useState(false);

  const urlCollection = onLibrary ? params.get("collection") : null;

  // Follow the page: a new search or collection in the address resets the box.
  useEffect(() => {
    setText(onLibrary ? (params.get("q") ?? "") : "");
  }, [onLibrary, params]);

  // The collection of the page shows as a chip once its name is known.
  useEffect(() => {
    if (!urlCollection) {
      setPicked(null);
      return;
    }
    setPicked({ slug: urlCollection, name: collections?.find((c) => c.slug === urlCollection)?.name ?? urlCollection });
  }, [urlCollection, collections]);

  const loadCollections = () => {
    if (collections) return;
    fetch("/api/collections")
      .then((r) => (r.ok ? r.json() : Promise.reject(r)))
      .then((body: { collections: Collection[] }) => setCollections(body.collections))
      .catch(() => setCollections([]));
  };

  useEffect(() => {
    if (urlCollection) loadCollections();
  }, [urlCollection]);

  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
      e.preventDefault();
      input.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const options = token && collections ? matching(collections, token.word).slice(0, 8) : [];
  const open = token !== null && options.length > 0;

  const update = (value: string, caret: number) => {
    setText(value);
    const t = collectionToken(value, caret);
    setToken(t);
    setActive(0);
    if (t) loadCollections();
  };

  const pick = (c: Collection) => {
    if (!token) return;
    const rest = (text.slice(0, token.start) + text.slice(token.start + token.word.length + 1)).replace(/\s{2,}/g, " ");
    setPicked({ slug: c.slug, name: c.name });
    setText(rest.trimStart());
    setToken(null);
    input.current?.focus();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (open) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setActive((i) => (i + (e.key === "ArrowDown" ? 1 : options.length - 1)) % options.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        pick(options[active]);
        return;
      }
      if (e.key === "Escape") {
        setToken(null);
        return;
      }
    }
    if (e.key === "Backspace" && picked && input.current?.selectionStart === 0 && input.current.selectionEnd === 0) {
      setPicked(null);
    }
  };

  // Searching keeps the library's other filters. A ":word" left in the box
  // picks the collection that matches it best.
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    let q = text;
    let collection = picked?.slug ?? "";
    const leftover = /(^|\s):([^\s:]+)/.exec(q);
    if (leftover && collections) {
      const best = matching(collections, leftover[2])[0];
      if (best) {
        collection = best.slug;
        q = q.replace(leftover[0], leftover[1]);
      }
    }
    const next = new URLSearchParams();
    if (q.trim()) next.set("q", q.trim());
    if (collection) next.set("collection", collection);
    if (onLibrary) {
      for (const key of ["text", "sort"]) {
        const v = params.get(key);
        if (v) next.set(key, v);
      }
      for (const t of params.getAll("tag")) next.append("tag", t);
    }
    setToken(null);
    input.current?.blur();
    router.push(next.size > 0 ? `/articles?${next}` : "/articles");
  };

  const showHelp = focused && !open && text === "" && !picked;

  return (
    <form className="header-search" action="/articles" method="get" role="search" onSubmit={onSubmit}>
      <label className="visually-hidden" htmlFor="q">
        {picked ? `Search ${picked.name}` : "Search your library"}
      </label>
      <svg className="header-search-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <circle cx="11" cy="11" r="6.5" />
        <path d="M16 16l4.5 4.5" />
      </svg>
      {picked ? (
        <span className="header-search-chip" style={hueStyle(picked)}>
          <CollectionIcon name={picked.name} />
          {picked.name}
          <button type="button" onClick={() => setPicked(null)} aria-label={`Search every collection, not only ${picked.name}`}>
            ×
          </button>
        </span>
      ) : null}
      <input
        ref={input}
        className="header-search-input"
        id="q"
        name="q"
        type="search"
        placeholder={picked ? "Search this collection" : "Title, address or tag"}
        value={text}
        maxLength={200}
        autoComplete="off"
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={open ? `${listId}-${active}` : undefined}
        onChange={(e) => update(e.target.value, e.target.selectionStart ?? e.target.value.length)}
        onKeyDown={onKeyDown}
        onFocus={() => setFocused(true)}
        onBlur={() => {
          setFocused(false);
          setToken(null);
        }}
      />
      <kbd className="header-search-key" aria-hidden="true">
        /
      </kbd>

      <ul id={listId} role="listbox" className="header-search-menu" hidden={!open} aria-label="Collections">
        {options.map((c, i) => (
          <li
            key={c.id}
            id={`${listId}-${i}`}
            role="option"
            aria-selected={i === active}
            style={hueStyle(c)}
            // mousedown, not click: picking must happen before the input blurs.
            onMouseDown={(e) => {
              e.preventDefault();
              pick(c);
            }}
            onMouseEnter={() => setActive(i)}
          >
            <span className="header-search-menu-icon">
              <CollectionIcon name={c.name} />
            </span>
            <span>{c.name}</span>
            <span className="header-search-menu-count">{c.articles}</span>
          </li>
        ))}
      </ul>

      {showHelp ? (
        <div className="header-search-help" aria-hidden="true">
          <p>
            <kbd>:name</kbd> search in a collection
          </p>
          <p>
            <kbd>-word</kbd> leave out articles with a word
          </p>
          <p>
            <kbd>&quot;exact words&quot;</kbd> match a phrase
          </p>
        </div>
      ) : null}

      {/* Without JavaScript the form still searches, in the page's collection. */}
      {picked ? <input type="hidden" name="collection" value={picked.slug} /> : null}
    </form>
  );
}
