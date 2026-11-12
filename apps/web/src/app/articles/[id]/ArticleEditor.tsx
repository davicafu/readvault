"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition, type FormEvent, type KeyboardEvent } from "react";

import Link from "next/link";

import { ApiError, readJSON, type Article, type Collection, type FieldProblem, type Suggestions } from "@/lib/api";

type Props = {
  id: string;
  title: string;
  tags: string[];
  collectionId: string | null;
  // null when the list could not be loaded.
  collections: Collection[] | null;
  // null when there are none to offer or they could not be loaded.
  suggestions: Suggestions | null;
  // Folded away once the article has its text; the person opens it to edit.
  folded: boolean;
};

export function ArticleEditor({
  id,
  title: initialTitle,
  tags: initialTags,
  collectionId: initialCollection,
  collections,
  suggestions,
  folded,
}: Props) {
  const [open, setOpen] = useState(!folded);
  const router = useRouter();
  const [title, setTitle] = useState(initialTitle);
  const [tags, setTags] = useState(initialTags);
  const [collectionId, setCollectionId] = useState(initialCollection);
  const [draft, setDraft] = useState("");
  const [problems, setProblems] = useState<FieldProblem[]>([]);
  const [failure, setFailure] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [saving, startSaving] = useTransition();

  const problemFor = (field: string) => problems.find((p) => p.field === field)?.message;

  const addDraft = () => {
    const name = draft.trim().replace(/\s+/g, " ");
    if (name && !tags.some((t) => t.toLowerCase() === name.toLowerCase())) {
      setTags([...tags, name]);
    }
    setDraft("");
  };

  const onTagKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addDraft();
    } else if (e.key === "Backspace" && draft === "" && tags.length > 0) {
      setTags(tags.slice(0, -1));
    }
  };

  const save = (e: FormEvent) => {
    e.preventDefault();
    const pending = draft.trim() ? [...tags, draft.trim()] : tags;
    setDraft("");
    setTags(pending);
    setProblems([]);
    setFailure(null);
    setSaved(false);
    startSaving(async () => {
      try {
        const res = await fetch(`/api/articles/${encodeURIComponent(id)}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          // Without the list the picker cannot change the collection, so
          // the edit leaves it as it is.
          body: JSON.stringify(collections ? { title, tags: pending, collectionId } : { title, tags: pending }),
        });
        const updated = await readJSON<Article>(res);
        setTitle(updated.title);
        setTags(updated.tags.map((t) => t.name));
        setCollectionId(updated.collection?.id ?? null);
        setSaved(true);
        router.refresh();
      } catch (err) {
        if (err instanceof ApiError && err.code === "validation_failed") {
          setProblems(err.fieldProblems);
        } else if (err instanceof ApiError && err.status === 404) {
          setFailure("This article no longer exists.");
        } else {
          setFailure("The changes could not be saved. Try again in a moment.");
        }
      }
    });
  };

  // A suggestion is offered until it is in the form; nothing is saved
  // until the person saves.
  const suggestedTags = (suggestions?.tags ?? []).filter(
    (s) => !tags.some((t) => t.toLowerCase() === s.name.toLowerCase()),
  );
  const suggestedCollection =
    collections && suggestions?.collection && suggestions.collection.id !== collectionId
      ? collections.find((c) => c.id === suggestions.collection?.id)
      : undefined;

  const titleError = problemFor("title");
  const tagsError = problemFor("tags");
  const collectionError = problemFor("collectionId");

  return (
    <details className="editor-panel" open={open} onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary>
        <h2>Edit title, tags and collection</h2>
        {!open && suggestedTags.length + (suggestedCollection ? 1 : 0) > 0 ? (
          <span className="editor-hint">
            {suggestedTags.length + (suggestedCollection ? 1 : 0)} suggested
          </span>
        ) : null}
      </summary>
      <form className="editor" onSubmit={save} noValidate>
        <div className="field">
          <label className="field-label" htmlFor="title">
            Title
          </label>
          <input
            id="title"
            className="input"
            value={title}
            maxLength={500}
            onChange={(e) => setTitle(e.target.value)}
            aria-invalid={titleError ? true : undefined}
            aria-describedby={titleError ? "title-error" : undefined}
          />
          {titleError ? (
            <p id="title-error" className="field-error">
              {titleError}
            </p>
          ) : null}
        </div>

        <div className="field">
          <label className="field-label" htmlFor="tag-draft">
            Tags
          </label>
          <div className="tag-input">
            {tags.map((t) => (
              <span key={t.toLowerCase()} className="tag">
                {t}
                <button type="button" onClick={() => setTags(tags.filter((x) => x !== t))} aria-label={`Remove tag ${t}`}>
                  ×
                </button>
              </span>
            ))}
            <input
              id="tag-draft"
              value={draft}
              maxLength={64}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onTagKey}
              onBlur={addDraft}
              aria-describedby="tag-help"
            />
          </div>
          <p id="tag-help" className="fine-print">
            Press Enter or type a comma to add a tag.
          </p>
          {suggestedTags.length > 0 ? (
            <div className="suggestions">
              <span className="fine-print">Suggested:</span>
              {suggestedTags.map((s) => (
                <button
                  key={s.slug}
                  type="button"
                  className="suggestion"
                  onClick={() => setTags([...tags, s.name])}
                  aria-label={`Add suggested tag ${s.name}`}
                >
                  + {s.name}
                </button>
              ))}
            </div>
          ) : null}
          {tagsError ? <p className="field-error">{tagsError}</p> : null}
        </div>

        <div className="field">
          <label className="field-label" htmlFor="collection">
            Collection
          </label>
          {collections ? (
            <select
              id="collection"
              className="select"
              value={collectionId ?? ""}
              onChange={(e) => setCollectionId(e.target.value || null)}
              aria-describedby="collection-help"
            >
              <option value="">No collection</option>
              {collections.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          ) : (
            <p className="field-error">The collections could not be loaded. Reload the page to change the collection.</p>
          )}
          {suggestedCollection ? (
            <div className="suggestions">
              <span className="fine-print">Suggested:</span>
              <button
                type="button"
                className="suggestion"
                onClick={() => setCollectionId(suggestedCollection.id)}
                aria-label={`Use suggested collection ${suggestedCollection.name}`}
              >
                {suggestedCollection.name}
              </button>
            </div>
          ) : null}
          <p id="collection-help" className="fine-print">
            An article is in one collection at most. <Link href="/collections">Manage collections</Link>
          </p>
          {collectionError ? <p className="field-error">{collectionError}</p> : null}
        </div>

        <div className="actions">
          <button className="button" type="submit" disabled={saving}>
            {saving ? "Saving…" : "Save changes"}
          </button>
          <span role="status" aria-live="polite">
            {saved ? <span className="saved">Changes saved</span> : null}
          </span>
        </div>
        {failure ? (
          <p className="field-error" role="alert">
            {failure}
          </p>
        ) : null}
      </form>
    </details>
  );
}
