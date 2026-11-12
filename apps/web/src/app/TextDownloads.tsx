"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { ApiError, readJSON, type DownloadSummary as Summary } from "@/lib/api";

const POLL_MS = 8000;

const articles = (n: number) => `${n.toLocaleString("en-US")} ${n === 1 ? "article" : "articles"}`;

// TextDownloads tells how the library's texts are doing: articles never
// downloaded (with a button to download them all), the ones still waiting
// in the background, and the ones that failed, linked to their list. It
// shows nothing when every article has its text. The page renders it with
// the summary it loaded, so the notice is there from the first paint.
export function TextDownloads({ initial }: { initial: Summary | null }) {
  const [summary, setSummary] = useState<Summary | null>(initial);
  const [queueing, setQueueing] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setSummary(await readJSON<Summary>(await fetch("/api/content-downloads", { cache: "no-store" })));
    } catch {
      // A summary is a convenience: without it the notice stays hidden.
    }
  }, []);

  // While texts download, check again now and then.
  const waiting = (summary?.queued ?? 0) > 0;
  useEffect(() => {
    if (!waiting) return;
    const timer = setInterval(() => void load(), POLL_MS);
    return () => clearInterval(timer);
  }, [waiting, load]);

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

  if (!summary || (summary.queued === 0 && summary.missing === 0 && summary.failed === 0)) return null;

  return (
    <section className="text-downloads" aria-label="Article texts" aria-live="polite">
      {summary.queued > 0 ? (
        <p className="text-downloads-line">
          <span className="text-downloads-pulse" aria-hidden="true" />
          Downloading texts in the background: {articles(summary.queued)} left.
        </p>
      ) : null}
      {summary.missing > 0 ? (
        <p className="text-downloads-line">
          {articles(summary.missing)} {summary.missing === 1 ? "has" : "have"} no text yet.
          <button className="button button-small" type="button" disabled={queueing} onClick={downloadAll}>
            {queueing ? "Queueing…" : "Download all"}
          </button>
        </p>
      ) : null}
      {summary.failed > 0 ? (
        <p className="text-downloads-line text-downloads-bad">
          The text of {articles(summary.failed)} could not be downloaded.
          <Link href="/?text=failed">See them</Link>
        </p>
      ) : null}
      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}
    </section>
  );
}
