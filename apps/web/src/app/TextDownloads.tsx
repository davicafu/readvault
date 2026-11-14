"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { readJSON, type DownloadSummary as Summary } from "@/lib/api";

const POLL_MS = 8000;

const articles = (n: number) => `${n.toLocaleString("en-US")} ${n === 1 ? "article" : "articles"}`;

// TextDownloads tells, on the front page, how many texts could not be
// downloaded, linked to their list. Texts still waiting are told only by the
// bell in the menu (Notifications). While texts download, it checks again
// now and then, since a failure can appear at any moment. The page renders
// it with the summary it loaded, so the notice is there from the first paint.
export function TextDownloads({ initial }: { initial: Summary | null }) {
  const [summary, setSummary] = useState<Summary | null>(initial);

  const load = useCallback(async () => {
    try {
      setSummary(await readJSON<Summary>(await fetch("/api/content-downloads", { cache: "no-store" })));
    } catch {
      // A summary is a convenience: without it the notice stays as it was.
    }
  }, []);

  const waiting = (summary?.queued ?? 0) > 0;
  useEffect(() => {
    if (!waiting) return;
    const timer = setInterval(() => void load(), POLL_MS);
    return () => clearInterval(timer);
  }, [waiting, load]);

  if (!summary || summary.failed === 0) return null;

  return (
    <section className="text-downloads" aria-label="Article texts" aria-live="polite">
      <p className="text-downloads-line text-downloads-bad">
        The text of {articles(summary.failed)} could not be downloaded.
        <Link href="/?text=failed">See them</Link>
      </p>
    </section>
  );
}
