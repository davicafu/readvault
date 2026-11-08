"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError, readJSON, type ArticlePage } from "@/lib/api";

type Report = {
  totals: { inserted: number; updated: number; skipped: number; failed: number };
  items: { outcome: string; problems: { code: string; message: string }[] }[];
};

const escapeHTML = (s: string) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

// AddLink saves one address. It goes through the same import as a bookmarks
// file, so the same URL rules, duplicate check and limits apply.
export function AddLink() {
  const router = useRouter();
  const [url, setUrl] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const address = url.trim();
    if (!address) return;
    setSaving(true);
    setFailure(null);
    try {
      const file = `<DL><DT><A HREF="${escapeHTML(address)}">${escapeHTML(address)}</A></DL>\n`;
      const form = new FormData();
      form.append("file", new Blob([file], { type: "text/html" }), "link.html");
      const report = await readJSON<Report>(await fetch("/api/imports", { method: "POST", body: form }));
      if (report.totals.failed > 0) {
        setFailure(report.items[0]?.problems[0]?.message ?? "That address cannot be saved.");
        return;
      }
      // Find the saved article, new or already in the library, and open it.
      const page = await readJSON<ArticlePage>(
        await fetch(`/api/articles?${new URLSearchParams({ q: address, limit: "10" })}`),
      );
      const match = page.items.find((a) => a.originalUrl === address) ?? page.items[0];
      setUrl("");
      if (match) {
        router.push(`/articles/${match.id}`);
      } else {
        router.refresh();
      }
    } catch (err) {
      setFailure(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <form className="add-link" onSubmit={submit}>
      <div className="field">
        <label className="field-label" htmlFor="add-url">
          Address
        </label>
        <input
          id="add-url"
          className="input"
          type="url"
          inputMode="url"
          placeholder="https://"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          required
        />
      </div>
      <button className="button" type="submit" disabled={saving}>
        {saving ? "Saving…" : "Save link"}
      </button>
      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}
    </form>
  );
}
