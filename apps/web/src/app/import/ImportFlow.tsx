"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import {
  ApiError,
  readJSON,
  type Candidate,
  type CandidateStatus,
  type ImportOutcome,
  type ImportPreview,
  type ImportReport,
} from "@/lib/api";

// Big exports can hold thousands of bookmarks; the table shows a slice.
const VISIBLE_ROWS = 200;

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

async function upload<T>(path: string, file: File): Promise<T> {
  const body = new FormData();
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

  // The file is uploaded again on confirm; the server keeps nothing between steps.
  const confirm = async (current: ImportPreview) => {
    if (!file) return;
    setError(null);
    setStep({ kind: "importing", preview: current });
    try {
      setStep({ kind: "done", report: await upload<ImportReport>("/api/imports", file) });
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
          <div className="field">
            <label className="field-label" htmlFor="file">
              Bookmarks file
            </label>
            <input
              id="file"
              className="input"
              type="file"
              accept=".html,.htm,text/html"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </div>
          <p className="fine-print">In Chrome: Bookmarks, Bookmark manager, the menu with three dots, Export bookmarks.</p>
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
          onConfirm={() => confirm(step.preview)}
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
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const [show, setShow] = useState<CandidateStatus | "all">("all");
  const rows = useMemo(
    () => (show === "all" ? preview.candidates : preview.candidates.filter((c) => c.status === show)),
    [preview.candidates, show],
  );
  const t = preview.totals;
  const importable = t.valid + t.existing;

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
          tags. Repeated and invalid entries are skipped.
        </p>
      </div>

      <dl className="summary">
        {counts.map(([key, label, n]) => (
          <div key={key}>
            <dt>{key === "all" ? "Found in the file" : label}</dt>
            <dd>{n.toLocaleString()}</dd>
          </div>
        ))}
      </dl>

      <div className="actions">
        <button className="button" type="button" disabled={importing || importable === 0} onClick={onConfirm}>
          {importing ? "Importing…" : `Import ${importable.toLocaleString()} bookmarks`}
        </button>
        <button className="button button-quiet" type="button" disabled={importing} onClick={onCancel}>
          Cancel
        </button>
      </div>

      <ul className="filter-tabs" aria-label="Show bookmarks">
        {counts.map(([key, label, n]) => (
          <li key={key}>
            <button type="button" aria-pressed={show === key} onClick={() => setShow(key)}>
              {label} ({n.toLocaleString()})
            </button>
          </li>
        ))}
      </ul>

      <CandidateTable rows={rows} />
    </section>
  );
}

function CandidateTable({ rows }: { rows: Candidate[] }) {
  const visible = rows.slice(0, VISIBLE_ROWS);
  return (
    <>
      <div className="table-wrap">
        <table className="records">
          <thead>
            <tr>
              <th scope="col">Line</th>
              <th scope="col">Bookmark</th>
              <th scope="col">Folder</th>
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
      {rows.length > VISIBLE_ROWS ? (
        <p className="fine-print">
          Showing the first {VISIBLE_ROWS} of {rows.length.toLocaleString()}. All of them are imported.
        </p>
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
