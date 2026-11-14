"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

import { ApiError, readJSON, type DownloadSummary as Summary } from "@/lib/api";
import { ActionIcon } from "./ActionIcon";

const POLL_MS = 8000;

/** Event a page sends after it queued downloads, so the bell looks again. */
export const DOWNLOADS_CHANGED = "readvault:downloads-changed";

export const downloadsChanged = () => window.dispatchEvent(new Event(DOWNLOADS_CHANGED));

const articles = (n: number) => `${n.toLocaleString("en-US")} ${n === 1 ? "article" : "articles"}`;

// Notifications is the bell in the menu: texts still downloading in the
// background, articles without a text yet (with Download all) and failed
// downloads. They are told only here. The bell checks again on every page
// change, every few seconds while texts download, and when a page says it
// queued some.
export function Notifications() {
  const pathname = usePathname();
  const [summary, setSummary] = useState<Summary | null>(null);
  const [open, setOpen] = useState(false);
  const [queueing, setQueueing] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const root = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    try {
      setSummary(await readJSON<Summary>(await fetch("/api/content-downloads", { cache: "no-store" })));
    } catch {
      // The bell is a convenience: without a summary it stays quiet.
    }
  }, []);

  useEffect(() => {
    void load();
    setOpen(false);
  }, [pathname, load]);

  useEffect(() => {
    const again = () => void load();
    window.addEventListener(DOWNLOADS_CHANGED, again);
    return () => window.removeEventListener(DOWNLOADS_CHANGED, again);
  }, [load]);

  const waiting = (summary?.queued ?? 0) > 0;
  useEffect(() => {
    if (!waiting) return;
    const timer = setInterval(() => void load(), POLL_MS);
    return () => clearInterval(timer);
  }, [waiting, load]);

  // Close on a click outside or Escape.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const downloadAll = async () => {
    setQueueing(true);
    setFailure(null);
    try {
      await readJSON<{ queued: number }>(await fetch("/api/content-downloads", { method: "POST" }));
      await load();
    } catch (err) {
      setFailure(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
    } finally {
      setQueueing(false);
    }
  };

  const queued = summary?.queued ?? 0;
  const missing = summary?.missing ?? 0;
  const failed = summary?.failed ?? 0;
  const pending = queued + missing;
  const label =
    failed > 0 || pending > 0
      ? `Notifications: ${[pending > 0 ? `${articles(pending)} waiting for a text` : "", failed > 0 ? `${articles(failed)} failed` : ""]
          .filter(Boolean)
          .join(", ")}`
      : "Notifications: nothing new";

  return (
    <div className="bell" ref={root}>
      <button
        type="button"
        className="bell-button"
        aria-label={label}
        aria-expanded={open}
        aria-controls="bell-panel"
        onClick={() => setOpen((o) => !o)}
      >
        <ActionIcon name="bell" className="bell-icon" />
        {failed > 0 ? (
          <span className="bell-badge" aria-hidden="true">
            {failed > 99 ? "99+" : failed}
          </span>
        ) : pending > 0 ? (
          <span className={`bell-dot${waiting ? " bell-dot-busy" : ""}`} aria-hidden="true" />
        ) : null}
      </button>
      {open ? (
        <div id="bell-panel" className="bell-panel" role="region" aria-label="Notifications" aria-live="polite">
          {queued > 0 ? (
            <p className="bell-item">
              <span className="pulse-dot" aria-hidden="true" />
              Downloading texts in the background: {articles(queued)} left.
            </p>
          ) : null}
          {missing > 0 ? (
            <p className="bell-item">
              {articles(missing)} {missing === 1 ? "has" : "have"} no text yet.
              <button className="button button-small" type="button" disabled={queueing} onClick={downloadAll}>
                {queueing ? "Queueing…" : "Download all"}
              </button>
            </p>
          ) : null}
          {failed > 0 ? (
            <p className="bell-item bell-item-bad">
              The text of {articles(failed)} could not be downloaded.
              <Link href="/articles?text=failed" onClick={() => setOpen(false)}>
                See them
              </Link>
            </p>
          ) : null}
          {queued === 0 && missing === 0 && failed === 0 ? (
            <p className="bell-item bell-empty">Nothing new: every article has its text.</p>
          ) : null}
          {failure ? (
            <p className="field-error" role="alert">
              {failure}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
