"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import {
  ApiError,
  readJSON,
  type Candidate,
  type CandidateStatus,
  type Collection,
  type ImportChoices,
  type ImportOutcome,
  type ImportPreview,
  type ImportReport,
} from "@/lib/api";

import { FilePicker } from "./FilePicker";

// Big exports can hold thousands of bookmarks; the table shows a slice at a time.
const VISIBLE_ROWS = 200;

// Collection names and tags follow the same rule on the server.
const MAX_NAME = 64;

/** What the person decided for a suggested collection, by its slug. */
type GroupChoice = { use: boolean; name: string };

/** What the person changed on one bookmark: raw input, parsed on import. */
type BookmarkEdit = { collection?: string; tags?: string };

const importable = (c: Candidate) => c.status === "valid" || c.status === "existing";

function splitTags(text: string): string[] {
  return text
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

type Step =
  | { kind: "choose" }
  | { kind: "previewing" }
  | { kind: "preview"; preview: ImportPreview }
  | { kind: "importing"; preview: ImportPreview }
  | { kind: "done"; report: ImportReport };

const statusLabels: Record<CandidateStatus, string> = {
  valid: "New",
  existing: "Already saved",
  duplicate_in_file: "Repeated in file",
  invalid: "Cannot import",
};

const outcomeLabels: Record<ImportOutcome, string> = {
  inserted: "Added",
  updated: "Updated",
  skipped: "Unchanged",
  failed: "Not imported",
};

async function upload<T>(path: string, file: File, choices?: ImportChoices): Promise<T> {
  const body = new FormData();
  // The server streams the file, so the choices must come first.
  if (choices) body.append("choices", JSON.stringify(choices));
  body.append("file", file);
  return readJSON<T>(await fetch(path, { method: "POST", body }));
}

function describe(err: unknown): string {
  if (!(err instanceof ApiError)) {
    return "The ReadVault API could not be reached. Check that it is running, then try again.";
  }
  switch (err.code) {
    case "file_too_large": {
      const max = (err.details as { maxBytes?: number } | undefined)?.maxBytes;
      return max ? `The file is larger than ${Math.round(max / 1024 / 1024)} MB. Export a smaller set of bookmarks.` : err.message;
    }
    case "too_many_records": {
      const max = (err.details as { maxRecords?: number } | undefined)?.maxRecords;
      return max ? `The file holds more than ${max.toLocaleString()} bookmarks. Split it and import each part.` : err.message;
    }
    case "invalid_upload":
    case "missing_file":
      return "The file could not be read. Choose the HTML file your browser exported.";
    case "invalid_file":
      return "This JSON file is not a ReadVault export. Choose the file from Export JSON, or your browser's bookmarks file.";
    case "invalid_choices":
      return `Your changes could not be applied: ${err.message}. Fix them and import again.`;
    default:
      return "The server could not finish. Try again in a moment.";
  }
}

export function ImportFlow() {
  const [file, setFile] = useState<File | null>(null);
  const [step, setStep] = useState<Step>({ kind: "choose" });
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setFile(null);
    setError(null);
    setStep({ kind: "choose" });
  };

  const preview = async () => {
    if (!file) return;
    setError(null);
    setStep({ kind: "previewing" });
    try {
      setStep({ kind: "preview", preview: await upload<ImportPreview>("/api/imports/preview", file) });
    } catch (err) {
      setError(describe(err));
      setStep({ kind: "choose" });
    }
  };

  // The file is uploaded again on confirm, with the person's changes; the
  // server keeps nothing between steps.
  const confirm = async (current: ImportPreview, choices: ImportChoices) => {
    if (!file) return;
    setError(null);
    setStep({ kind: "importing", preview: current });
    try {
      setStep({ kind: "done", report: await upload<ImportReport>("/api/imports", file, choices) });
    } catch (err) {
      setError(describe(err));
      setStep({ kind: "preview", preview: current });
    }
  };

  return (
    <div className="steps">
      {error ? (
        <p className="notice notice-error" role="alert">
          {error}
        </p>
      ) : null}

      {step.kind === "choose" || step.kind === "previewing" ? (
        <div className="dropzone">
          <FilePicker
            label="Your bookmarks file (HTML) or a ReadVault export (JSON)"
            accept=".html,.htm,text/html,.json,application/json"
            file={file}
            onFile={setFile}
            disabled={step.kind === "previewing"}
          />
          <p className="fine-print">In Chrome: Bookmarks, Bookmark manager, the menu with three dots, Export bookmarks.</p>
          <p className="fine-print">
            To restore from a backup, choose the file from <strong>Export JSON</strong>: articles missing from the library
            come back, deleted collections are created again and their articles go back into them.
          </p>
          <div className="actions">
            <button className="button" type="button" disabled={!file || step.kind === "previewing"} onClick={preview}>
              {step.kind === "previewing" ? "Checking the file…" : "Check the file"}
            </button>
          </div>
        </div>
      ) : null}

      {step.kind === "preview" || step.kind === "importing" ? (
        <PreviewStep
          preview={step.preview}
          fileName={file?.name ?? ""}
          importing={step.kind === "importing"}
          onConfirm={(choices) => confirm(step.preview, choices)}
          onCancel={reset}
        />
      ) : null}

      {step.kind === "done" ? <ReportStep report={step.report} onAgain={reset} /> : null}
    </div>
  );
}

function PreviewStep({
  preview,
  fileName,
  importing,
  onConfirm,
  onCancel,
}: {
  preview: ImportPreview;
  fileName: string;
  importing: boolean;
  onConfirm: (choices: ImportChoices) => void;
  onCancel: () => void;
}) {
  const [show, setShow] = useState<CandidateStatus | "all">("all");
  const [groups, setGroups] = useState<Record<string, GroupChoice>>(() =>
    Object.fromEntries(preview.collections.map((c) => [c.slug, { use: true, name: c.name }])),
  );
  const [edits, setEdits] = useState<Record<number, BookmarkEdit>>({});
  const existing = useExistingCollections();

  const rows = useMemo(
    () => (show === "all" ? preview.candidates : preview.candidates.filter((c) => c.status === show)),
    [preview.candidates, show],
  );
  const t = preview.totals;
  const importableCount = t.valid + t.existing;

  // The collection a bookmark gets from its suggestion, after the group choice.
  const suggestedFor = (c: Candidate): string => {
    if (!c.collection) return "";
    const g = groups[slugOf(preview, c.collection)];
    return g && g.use ? g.name : "";
  };

  const problems = validate(groups, edits);
  const names = [...new Set([...existing.map((c) => c.name), ...Object.values(groups).filter((g) => g.use).map((g) => g.name.trim())])]
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b));

  const choices = (): ImportChoices => {
    const collections: Record<string, string | null> = {};
    for (const s of preview.collections) {
      const g = groups[s.slug];
      if (!g.use) collections[s.name] = null;
      else if (g.name.trim() !== s.name) collections[s.name] = g.name.trim();
    }
    const bookmarks: NonNullable<ImportChoices["bookmarks"]> = {};
    for (const [pos, e] of Object.entries(edits)) {
      const b: { collection?: string | null; tags?: string[] } = {};
      if (e.collection !== undefined) b.collection = e.collection.trim() || null;
      if (e.tags !== undefined) b.tags = splitTags(e.tags);
      bookmarks[Number(pos)] = b;
    }
    return { collections, bookmarks };
  };

  const counts: [CandidateStatus | "all", string, number][] = [
    ["all", "All", t.detected],
    ["valid", statusLabels.valid, t.valid],
    ["existing", statusLabels.existing, t.existing],
    ["duplicate_in_file", statusLabels.duplicate_in_file, t.duplicateInFile],
    ["invalid", statusLabels.invalid, t.invalid],
  ];

  return (
    <section aria-labelledby="preview-title" className="steps">
      <div className="page-head">
        <h2 id="preview-title">Check before importing {fileName}</h2>
        <p className="lede">
          New bookmarks are added. Bookmarks already in your library keep your edits and only gain missing details and
          tags (no tags where you edited them). Repeated and invalid entries are skipped.
        </p>
      </div>

      {t.detected === 0 ? (
        <div className="notice notice-error" role="alert">
          <h3>No bookmarks were found in {fileName}</h3>
          <p>
            Choose the HTML file your browser exports from its bookmarks manager (Export bookmarks). Other files, and
            exports with no bookmarks, have nothing to import.
          </p>
        </div>
      ) : null}

      <dl className="summary">
        {counts.map(([key, label, n]) => (
          <div key={key}>
            <dt>{key === "all" ? "Found in the file" : label}</dt>
            <dd>{n.toLocaleString()}</dd>
          </div>
        ))}
      </dl>

      <div className="actions">
        <button
          className="button"
          type="button"
          disabled={importing || importableCount === 0 || problems.length > 0}
          onClick={() => onConfirm(choices())}
        >
          {importing ? "Importing…" : `Import ${importableCount.toLocaleString()} bookmarks`}
        </button>
        <button className="button button-quiet" type="button" disabled={importing} onClick={onCancel}>
          Cancel
        </button>
      </div>

      {problems.length > 0 ? (
        <ul className="notice notice-error problems" role="alert">
          {problems.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
      ) : null}

      {preview.collections.length > 0 ? (
        <SuggestedCollections preview={preview} groups={groups} onChange={setGroups} />
      ) : null}

      <datalist id="collection-names">
        {names.map((n) => (
          <option key={n} value={n} />
        ))}
      </datalist>

      <ul className="filter-tabs" aria-label="Show bookmarks">
        {counts.map(([key, label, n]) => (
          <li key={key}>
            <button type="button" aria-pressed={show === key} onClick={() => setShow(key)}>
              {label} ({n.toLocaleString()})
            </button>
          </li>
        ))}
      </ul>

      <CandidateTable
        key={show}
        rows={rows}
        edits={edits}
        suggestedFor={suggestedFor}
        onEdit={(position, edit) => setEdits((all) => ({ ...all, [position]: { ...all[position], ...edit } }))}
      />
    </section>
  );
}

/** The person's collections, so a bookmark can be sent to one the file does not suggest. */
function useExistingCollections(): Collection[] {
  const [collections, setCollections] = useState<Collection[]>([]);
  useEffect(() => {
    let live = true;
    fetch("/api/collections")
      .then((res) => readJSON<{ collections: Collection[] }>(res))
      .then((body) => live && setCollections(body.collections))
      // Without the list, names can still be typed.
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);
  return collections;
}

function slugOf(preview: ImportPreview, name: string): string {
  return preview.collections.find((c) => c.name === name)?.slug ?? "";
}

function validate(groups: Record<string, GroupChoice>, edits: Record<number, BookmarkEdit>): string[] {
  const problems: string[] = [];
  for (const g of Object.values(groups)) {
    if (g.use && !g.name.trim()) problems.push("A collection you keep needs a name. Untick it to import its bookmarks without one.");
    if (g.use && g.name.trim().length > MAX_NAME) problems.push(`The collection name "${g.name.trim().slice(0, 20)}…" is longer than ${MAX_NAME} characters.`);
  }
  for (const [pos, e] of Object.entries(edits)) {
    if (e.collection && e.collection.trim().length > MAX_NAME) problems.push(`Line ${pos}: the collection name is longer than ${MAX_NAME} characters.`);
    if (e.tags && splitTags(e.tags).some((t) => t.length > MAX_NAME)) problems.push(`Line ${pos}: a tag is longer than ${MAX_NAME} characters.`);
  }
  return [...new Set(problems)];
}

function SuggestedCollections({
  preview,
  groups,
  onChange,
}: {
  preview: ImportPreview;
  groups: Record<string, GroupChoice>;
  onChange: (update: (all: Record<string, GroupChoice>) => Record<string, GroupChoice>) => void;
}) {
  const set = (slug: string, change: Partial<GroupChoice>) => onChange((all) => ({ ...all, [slug]: { ...all[slug], ...change } }));
  return (
    <section aria-labelledby="suggested-title" className="suggested">
      <h3 id="suggested-title">Collections</h3>
      <p className="fine-print">
        Each bookmark goes into the collection named after its folder. Rename a collection, or untick it to import its
        bookmarks without one. A name you already use adds them to that collection. Change single bookmarks in the
        list below.
      </p>
      <ul className="suggested-list">
        {preview.collections.map((c) => {
          const g = groups[c.slug];
          return (
            <li key={c.slug} className={g.use ? undefined : "suggested-off"}>
              <label className="suggested-use">
                <input type="checkbox" checked={g.use} onChange={(e) => set(c.slug, { use: e.target.checked })} />
                <span className="visually-hidden">Use the collection {c.name}</span>
              </label>
              <input
                className="input"
                aria-label={`Name for the collection suggested by the folder ${c.name}`}
                value={g.name}
                disabled={!g.use}
                onChange={(e) => set(c.slug, { name: e.target.value })}
              />
              <span className="suggested-count">
                {c.bookmarks.toLocaleString()} {c.bookmarks === 1 ? "bookmark" : "bookmarks"}
                {c.existing ? " · already yours" : " · new"}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function CandidateTable({
  rows,
  edits,
  suggestedFor,
  onEdit,
}: {
  rows: Candidate[];
  edits: Record<number, BookmarkEdit>;
  suggestedFor: (c: Candidate) => string;
  onEdit: (position: number, edit: BookmarkEdit) => void;
}) {
  const [limit, setLimit] = useState(VISIBLE_ROWS);
  const visible = rows.slice(0, limit);
  return (
    <>
      <div className="table-wrap">
        <table className="records">
          <thead>
            <tr>
              <th scope="col">Line</th>
              <th scope="col">Bookmark</th>
              <th scope="col">Folder</th>
              <th scope="col">Collection</th>
              <th scope="col">Tags</th>
              <th scope="col">Result</th>
              <th scope="col">Notes</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((c) => (
              <tr key={c.position}>
                <td className="num">{c.position}</td>
                <td>
                  <span className="record-title">{c.title || "Untitled"}</span>
                  <span className="record-url">{c.originalUrl || "No address"}</span>
                </td>
                <td>{c.folder.join(" / ")}</td>
                {importable(c) ? (
                  <>
                    <td>
                      <input
                        className="input input-cell"
                        list="collection-names"
                        aria-label={`Collection for line ${c.position}`}
                        placeholder="No collection"
                        value={edits[c.position]?.collection ?? suggestedFor(c)}
                        onChange={(e) => onEdit(c.position, { collection: e.target.value })}
                      />
                    </td>
                    <td>
                      <input
                        className="input input-cell"
                        aria-label={`Tags for line ${c.position}, separated by commas`}
                        placeholder="No tags"
                        value={edits[c.position]?.tags ?? c.tags.join(", ")}
                        onChange={(e) => onEdit(c.position, { tags: e.target.value })}
                      />
                    </td>
                  </>
                ) : (
                  <>
                    <td />
                    <td />
                  </>
                )}
                <td>
                  <span className={`status status-${c.status}`}>{statusLabels[c.status]}</span>
                </td>
                <td>
                  {c.errors.length + c.warnings.length > 0 ? (
                    <ul className="problems">
                      {[...c.errors, ...c.warnings].map((p) => (
                        <li key={p.code}>{p.message}</li>
                      ))}
                    </ul>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {rows.length > limit ? (
        <div className="actions">
          <p className="fine-print">
            Showing the first {limit.toLocaleString()} of {rows.length.toLocaleString()}. All of them are imported.
          </p>
          <button className="button button-quiet" type="button" onClick={() => setLimit((n) => n + VISIBLE_ROWS)}>
            Show more
          </button>
        </div>
      ) : null}
    </>
  );
}

function ReportStep({ report, onAgain }: { report: ImportReport; onAgain: () => void }) {
  const t = report.totals;
  const listed = report.items.filter((i) => i.outcome !== "skipped" || i.problems?.length);
  return (
    <section aria-labelledby="report-title" className="steps" role="status">
      <div className="page-head">
        <h2 id="report-title">Import finished</h2>
      </div>
      <dl className="summary">
        {(Object.keys(outcomeLabels) as ImportOutcome[]).map((o) => (
          <div key={o}>
            <dt>{outcomeLabels[o]}</dt>
            <dd>{t[o].toLocaleString()}</dd>
          </div>
        ))}
      </dl>
      <p>
        The text of the imported articles that have none yet is downloaded in the background, a few at a time, even
        if you close this page. It appears on each article's page as it arrives.
      </p>
      <div className="actions">
        <Link className="button" href="/">
          Go to the library
        </Link>
        <button className="button button-quiet" type="button" onClick={onAgain}>
          Import another file
        </button>
      </div>

      {listed.length > 0 ? (
        <div className="table-wrap">
          <table className="records">
            <thead>
              <tr>
                <th scope="col">Line</th>
                <th scope="col">Address</th>
                <th scope="col">Result</th>
                <th scope="col">Why</th>
              </tr>
            </thead>
            <tbody>
              {listed.slice(0, VISIBLE_ROWS).map((i) => (
                <tr key={i.position}>
                  <td className="num">{i.position}</td>
                  <td>
                    <span className="record-url">{i.originalUrl || "No address"}</span>
                  </td>
                  <td>
                    <span className={`status status-${i.outcome}`}>{outcomeLabels[i.outcome]}</span>
                  </td>
                  <td>{i.problems?.map((p) => p.message).join(" ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <p className="fine-print">Import ID {report.importId}. Include it if you report a problem with this import.</p>
    </section>
  );
}
