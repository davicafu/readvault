"use client";

import { useRouter } from "next/navigation";
import { useLayoutEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";

import { ApiError, readJSON, type ArticlePage } from "@/lib/api";
import { downloadsChanged } from "../Notifications";

type Report = {
  totals: { inserted: number; updated: number; skipped: number; failed: number };
  items: { originalUrl: string; outcome: string; problems: { code: string; message: string }[] }[];
};

const escapeHTML = (s: string) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

/** The distinct addresses in the box, in order: separated by spaces or new lines. */
export function linksIn(text: string): string[] {
  return [...new Set(text.split(/\s+/).filter(Boolean))];
}

const isWebAddress = (s: string) => {
  try {
    const u = new URL(s);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
};

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// AddLink saves one address or several at once. They go through the same
// import as a bookmarks file, so the same URL rules, duplicate check and
// limits apply. One link opens its article with its text downloaded; several
// download their texts in the background (the bell in the menu shows them).
export function AddLink() {
  const router = useRouter();
  const box = useRef<HTMLTextAreaElement>(null);
  const [text, setText] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [stage, setStage] = useState("Adding…");

  const links = linksIn(text);
  const several = links.length > 1;

  // The box grows with what is in it, a line per pasted link.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight + (el.offsetHeight - el.clientHeight)}px`;
  }, [text]);

  const importLinks = async (addresses: string[]) => {
    const rows = addresses.map((a) => `<DT><A HREF="${escapeHTML(a)}">${escapeHTML(a)}</A>`).join("\n");
    const form = new FormData();
    form.append("file", new Blob([`<DL>\n${rows}\n</DL>\n`], { type: "text/html" }), "links.html");
    return readJSON<Report>(await fetch("/api/imports", { method: "POST", body: form }));
  };

  const addOne = async (address: string) => {
    const report = await importLinks([address]);
    if (report.totals.failed > 0) {
      setFailure(report.items[0]?.problems[0]?.message ?? "That address cannot be saved.");
      return;
    }
    // Find the saved article, new or already in the library, and open it.
    const page = await readJSON<ArticlePage>(
      await fetch(`/api/articles?${new URLSearchParams({ q: address, limit: "10" })}`),
    );
    const match = page.items.find((a) => a.originalUrl === address) ?? page.items[0];
    setText("");
    // A new link gets its text right away, so its suggestions come from the
    // content. A failed download still keeps the link; the article page
    // offers the download again. Existing articles keep their saved text.
    if (match && report.totals.inserted > 0) {
      setStage("Downloading the text…");
      await fetch(`/api/articles/${encodeURIComponent(match.id)}/content/fetch`, { method: "POST" }).catch(() => null);
    }
    if (match) {
      router.push(`/articles/${match.id}`);
    } else {
      router.refresh();
    }
  };

  const addSeveral = async (addresses: string[]) => {
    const report = await importLinks(addresses);
    const failed = report.items.filter((i) => i.outcome === "failed");
    const known = report.totals.updated + report.totals.skipped;
    const parts = [`Added ${plural(report.totals.inserted, "link", "links")}.`];
    if (known > 0) parts.push(`${plural(known, "was", "were")} already in your library.`);
    if (report.totals.inserted > 0) parts.push("Their texts download in the background; the bell in the menu shows how many are left.");
    setDone(parts.join(" "));
    if (failed.length > 0) {
      setFailure(
        `${plural(failed.length, "link", "links")} could not be added and ${failed.length === 1 ? "stays" : "stay"} in the box: ` +
          failed.map((i) => `${i.originalUrl} (${i.problems[0]?.message ?? "not valid"})`).join("; "),
      );
    }
    // Keep only what failed, to fix and try again.
    setText(failed.map((i) => i.originalUrl).join("\n"));
    downloadsChanged();
    router.refresh();
  };

  const submit = async (e?: FormEvent) => {
    e?.preventDefault();
    if (links.length === 0 || saving) return;
    const invalid = links.filter((l) => !isWebAddress(l));
    if (invalid.length > 0) {
      setDone(null);
      setFailure(`Not a web address (it must start with http:// or https://): ${invalid.join(", ")}`);
      return;
    }
    setSaving(true);
    setStage("Adding…");
    setFailure(null);
    setDone(null);
    try {
      await (several ? addSeveral(links) : addOne(links[0]));
    } catch (err) {
      setFailure(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
    } finally {
      setSaving(false);
    }
  };

  // Enter adds, as in a one-line field; Shift+Enter starts a new line.
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  };

  return (
    <form className="add-link" onSubmit={submit}>
      <div className="field">
        <label className="field-label" htmlFor="add-url">
          {several ? "Addresses" : "Address"}
        </label>
        <textarea
          ref={box}
          id="add-url"
          className="input add-link-box"
          rows={1}
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          aria-describedby="add-url-help"
          required
        />
        <p id="add-url-help" className="fine-print">
          {several
            ? `${links.length} links. Each is added once; links already in your library are kept as they are.`
            : "Paste one link, or several separated by spaces or new lines (Shift+Enter starts a new line)."}
        </p>
      </div>
      <button className="button" type="submit" disabled={saving}>
        {saving ? stage : several ? "Add links" : "Add link"}
      </button>
      {done ? (
        <p className="fine-print" role="status">
          {done}
        </p>
      ) : null}
      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}
    </form>
  );
}
