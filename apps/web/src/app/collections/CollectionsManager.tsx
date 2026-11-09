"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError, readJSON, type Collection } from "@/lib/api";

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

// CollectionsManager creates, renames and deletes collections. Deleting a
// collection keeps its articles, without a collection.
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
      <form className="add-link" onSubmit={create}>
        <div className="field">
          <label className="field-label" htmlFor="new-collection">
            New collection
          </label>
          <input
            id="new-collection"
            className="input"
            value={name}
            maxLength={64}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>
        <button className="button" type="submit" disabled={busy}>
          {busy ? "Creating…" : "Create collection"}
        </button>
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
    </>
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
  const [busy, setBusy] = useState(false);

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
    <li className="collection-row">
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
          <Link href={`/?collection=${encodeURIComponent(c.slug)}`} className="collection-name">
            {c.name}
          </Link>
          <span className="fine-print">
            {c.articles} {c.articles === 1 ? "article" : "articles"}
          </span>
          <span className="collection-actions">
            <button className="button button-quiet" type="button" onClick={() => setEditing(true)} disabled={busy}>
              Rename
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
    </li>
  );
}
