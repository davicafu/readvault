"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError, readJSON, type Collection } from "@/lib/api";

import { CollectionIcon, hueStyle } from "../CollectionIcon";

function message(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "collection_exists") return "You already have a collection with this name.";
    if (err.code === "validation_failed") return err.fieldProblems[0]?.message ?? err.message;
    if (err.status === 404) return "This collection no longer exists. Reload the page.";
  }
  return "The change could not be saved. Try again in a moment.";
}

async function send(method: string, path: string, body?: unknown): Promise<Response> {
  return fetch(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

// removeTexts deletes the saved texts of every article, or of one
// collection's, and returns how many were deleted.
async function removeTexts(path: string): Promise<number> {
  return (await readJSON<{ removed: number }>(await send("DELETE", path))).removed;
}

const removedMessage = (n: number) =>
  n === 0 ? "There were no saved texts to delete." : `Deleted ${n} saved ${n === 1 ? "text" : "texts"}.`;

// CollectionsManager creates, renames and deletes collections, and deletes
// saved texts to free space. Deleting a collection keeps its articles,
// without a collection; deleting texts keeps the articles.
export function CollectionsManager({ initial }: { initial: Collection[] }) {
  const router = useRouter();
  const [collections, setCollections] = useState(initial);
  const [name, setName] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const sorted = (cs: Collection[]) => [...cs].sort((a, b) => a.name.localeCompare(b.name));

  const create = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    setFailure(null);
    try {
      const created = await readJSON<Collection>(await send("POST", "/api/collections", { name }));
      setCollections(sorted([...collections, created]));
      setName("");
      router.refresh();
    } catch (err) {
      setFailure(message(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <form className="collection-form" onSubmit={create}>
        <label className="field-label" htmlFor="new-collection">
          New collection
        </label>
        <div className="collection-form-row">
          <span
            className="collection-chip"
            style={hueStyle({ name, slug: name.trim().toLowerCase().split(/\s+/).join("-") || "new" })}
            aria-hidden="true"
          >
            <CollectionIcon name={name} />
          </span>
          <input
            id="new-collection"
            className="input"
            value={name}
            maxLength={64}
            onChange={(e) => setName(e.target.value)}
            aria-describedby="new-collection-hint"
            required
          />
          <button className="button" type="submit" disabled={busy}>
            {busy ? "Creating…" : "Create collection"}
          </button>
        </div>
        <p className="fine-print" id="new-collection-hint">
          The icon is picked from the name.
        </p>
        {failure ? (
          <p className="field-error" role="alert">
            {failure}
          </p>
        ) : null}
      </form>

      {collections.length === 0 ? (
        <p className="fine-print">No collections yet.</p>
      ) : (
        <ul className="collection-list">
          {collections.map((c) => (
            <CollectionRow
              key={c.id}
              collection={c}
              onRenamed={(updated) => {
                setCollections(sorted(collections.map((x) => (x.id === updated.id ? updated : x))));
                router.refresh();
              }}
              onDeleted={() => {
                setCollections(collections.filter((x) => x.id !== c.id));
                router.refresh();
              }}
            />
          ))}
        </ul>
      )}

      <SavedTexts />
    </>
  );
}

// SavedTexts deletes the saved text of every article to free space.
function SavedTexts() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<{ ok: boolean; text: string } | null>(null);

  const removeAll = async () => {
    const question =
      "Delete the saved text of every article? The articles, their tags and collections stay; you can download the texts again later. Texts you uploaded are deleted too.";
    if (!window.confirm(question)) return;
    setBusy(true);
    setOutcome(null);
    try {
      setOutcome({ ok: true, text: removedMessage(await removeTexts("/api/contents")) });
      router.refresh();
    } catch {
      setOutcome({ ok: false, text: "The texts could not be deleted. Try again in a moment." });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="saved-texts" aria-labelledby="saved-texts-title">
      <h2 id="saved-texts-title">Saved texts</h2>
      <p>
        Each article can keep its text and images for reading later. Delete them to free space; the articles stay in
        your library. To clear one collection, use <strong>Delete texts</strong> on its row.
      </p>
      <button className="button button-danger" type="button" onClick={removeAll} disabled={busy}>
        {busy ? "Deleting…" : "Delete all saved texts"}
      </button>
      {outcome ? (
        <p className={outcome.ok ? "fine-print" : "field-error"} role={outcome.ok ? "status" : "alert"}>
          {outcome.text}
        </p>
      ) : null}
    </section>
  );
}

function CollectionRow({
  collection: c,
  onRenamed,
  onDeleted,
}: {
  collection: Collection;
  onRenamed: (c: Collection) => void;
  onDeleted: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(c.name);
  const [failure, setFailure] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const clearTexts = async () => {
    const question = `Delete the saved texts of the articles in “${c.name}”? The articles stay in the collection; you can download the texts again later.`;
    if (!window.confirm(question)) return;
    setBusy(true);
    setFailure(null);
    setDone(null);
    try {
      setDone(removedMessage(await removeTexts(`/api/collections/${encodeURIComponent(c.id)}/contents`)));
    } catch (err) {
      setFailure(err instanceof ApiError && err.status === 404 ? message(err) : "The texts could not be deleted. Try again in a moment.");
    } finally {
      setBusy(false);
    }
  };

  const rename = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setFailure(null);
    try {
      const res = await send("PATCH", `/api/collections/${encodeURIComponent(c.id)}`, { name });
      onRenamed(await readJSON<Collection>(res));
      setEditing(false);
    } catch (err) {
      setFailure(message(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    const articles = c.articles === 1 ? "Its article stays" : `Its ${c.articles} articles stay`;
    const question = `Delete the collection “${c.name}”? ${articles} in your library, without a collection.`;
    if (!window.confirm(question)) return;
    setBusy(true);
    setFailure(null);
    try {
      const res = await send("DELETE", `/api/collections/${encodeURIComponent(c.id)}`);
      if (!res.ok) await readJSON(res);
      onDeleted();
    } catch (err) {
      setFailure(message(err));
      setBusy(false);
    }
  };

  return (
    <li className="collection-row" style={hueStyle(c)}>
      <span className="collection-chip" aria-hidden="true">
        <CollectionIcon name={editing ? name : c.name} />
      </span>
      {editing ? (
        <form className="collection-rename" onSubmit={rename}>
          <label className="visually-hidden" htmlFor={`rename-${c.id}`}>
            New name for {c.name}
          </label>
          <input
            id={`rename-${c.id}`}
            className="input"
            value={name}
            maxLength={64}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />
          <button className="button" type="submit" disabled={busy}>
            Save name
          </button>
          <button
            className="button button-quiet"
            type="button"
            onClick={() => {
              setEditing(false);
              setName(c.name);
              setFailure(null);
            }}
          >
            Cancel
          </button>
        </form>
      ) : (
        <>
          <span className="collection-title">
            <Link href={`/?collection=${encodeURIComponent(c.slug)}`} className="collection-name">
              {c.name}
            </Link>
            <span className="fine-print">
              {c.articles} {c.articles === 1 ? "article" : "articles"}
            </span>
          </span>
          <span className="collection-actions">
            <button className="button button-quiet" type="button" onClick={() => setEditing(true)} disabled={busy}>
              Rename
            </button>
            <button className="button button-quiet" type="button" onClick={clearTexts} disabled={busy}>
              Delete texts
            </button>
            <button className="button button-quiet" type="button" onClick={remove} disabled={busy}>
              Delete
            </button>
          </span>
        </>
      )}
      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}
      {done ? (
        <p className="fine-print collection-done" role="status">
          {done}
        </p>
      ) : null}
    </li>
  );
}
