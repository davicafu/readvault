"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition, type FormEvent, type KeyboardEvent } from "react";

import { ApiError, readJSON, type Article, type FieldProblem } from "@/lib/api";

type Props = { id: string; title: string; tags: string[] };

export function ArticleEditor({ id, title: initialTitle, tags: initialTags }: Props) {
  const router = useRouter();
  const [title, setTitle] = useState(initialTitle);
  const [tags, setTags] = useState(initialTags);
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
          body: JSON.stringify({ title, tags: pending }),
        });
        const updated = await readJSON<Article>(res);
        setTitle(updated.title);
        setTags(updated.tags.map((t) => t.name));
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

  const titleError = problemFor("title");
  const tagsError = problemFor("tags");

  return (
    <form className="editor" onSubmit={save} noValidate>
      <h2>Edit</h2>
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
        {tagsError ? <p className="field-error">{tagsError}</p> : null}
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
  );
}
