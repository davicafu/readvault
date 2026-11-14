"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, useTransition, type ChangeEvent } from "react";

import { ApiError, downloadProblems, readJSON, type ArticleContent } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { Markdown } from "@/lib/markdown";

import { ActionIcon } from "../../ActionIcon";
import { downloadsChanged } from "../../Notifications";

type Props = { articleId: string; articleUrl: string; content: ArticleContent | null };

const accepted = ".html,.htm,.xhtml,.md,.markdown,.txt,text/html,text/markdown,text/plain";

function problemText(problem: string | null, status?: number): string {
  const reason = (problem && downloadProblems[problem]) || "the download failed";
  return status ? `${reason} (status ${status})` : reason;
}

export function ArticleText({ articleId, articleUrl, content }: Props) {
  const router = useRouter();
  const fileInput = useRef<HTMLInputElement>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState<"download" | "copy" | "upload" | "remove" | null>(null);
  const [, startRefresh] = useTransition();

  const run = async (kind: "download" | "copy" | "upload" | "remove", request: () => Promise<Response>) => {
    setBusy(kind);
    setFailure(null);
    try {
      await readJSON<ArticleContent>(await request());
    } catch (err) {
      if (err instanceof ApiError && err.code === "fetch_failed") {
        const d = err.details as { problem?: string; status?: number } | undefined;
        setFailure(
          d?.problem === "no_copy"
            ? "No other copy of this article was found; the text you had is kept."
            : `The text could not be downloaded: ${problemText(d?.problem ?? null, d?.status)}.`,
        );
      } else if (err instanceof ApiError && err.code === "no_text") {
        setFailure("That file holds no article text. Choose the page you saved from the browser.");
      } else if (err instanceof ApiError && err.code === "file_too_large") {
        setFailure("That file is too large for one article.");
      } else {
        setFailure(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
      }
    } finally {
      setBusy(null);
      downloadsChanged();
      startRefresh(() => router.refresh());
    }
  };

  const download = () =>
    run("download", () => fetch(`/api/articles/${articleId}/content/fetch`, { method: "POST" }));

  // Another copy of the page, for a text that could not be read or came out
  // cut short. Offered quietly: the download already tries one on its own.
  const fromCopy = () =>
    run("copy", () => fetch(`/api/articles/${articleId}/content/fetch?from=copy`, { method: "POST" }));

  const copyLink = (
    <button className="link-quiet" type="button" disabled={busy !== null} onClick={fromCopy}>
      {busy === "copy" ? "Looking for another copy…" : "Try another copy"}
    </button>
  );

  // Frees the space of this one text; the article stays and its text can be
  // downloaded or uploaded again.
  const removeText = () => {
    if (!window.confirm("Delete the saved text of this article, with its images? The article, its tags and collection stay; you can download the text again."))
      return;
    run("remove", () => fetch(`/api/articles/${articleId}/content`, { method: "DELETE" }));
  };

  const upload = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    const form = new FormData();
    form.append("file", file);
    run("upload", () => fetch(`/api/articles/${articleId}/content`, { method: "POST", body: form }));
  };

  const stored = new Map((content?.images ?? []).map((img) => [img.sourceUrl, img.id]));
  const resolveImage = (src: string) => {
    let abs: string;
    try {
      abs = new URL(src, articleUrl).href;
    } catch {
      return null;
    }
    const id = stored.get(abs) ?? stored.get(src);
    return id ? `/api/articles/${articleId}/images/${id}` : null;
  };

  const actions = (
    <div className="actions">
      <button className="button button-with-icon" type="button" disabled={busy !== null} onClick={() => fileInput.current?.click()}>
        <ActionIcon name="upload" />
        {busy === "upload" ? "Uploading…" : "Upload page"}
      </button>
      <button className="button button-quiet button-with-icon" type="button" disabled={busy !== null} onClick={download}>
        <ActionIcon name="download" />
        {busy === "download" ? "Downloading…" : content?.status === "complete" ? "Download again" : "Download text"}
      </button>
      {content ? (
        <button className="button button-danger button-with-icon actions-end" type="button" disabled={busy !== null} onClick={removeText}>
          <ActionIcon name="clearText" />
          {busy === "remove" ? "Deleting…" : "Delete text"}
        </button>
      ) : null}
      <input ref={fileInput} type="file" accept={accepted} hidden onChange={upload} />
    </div>
  );

  const howToUpload =
    "Open the article in your browser, save it as a web page (HTML), and upload that file here. A Markdown file works too.";

  // The latest snapshot of the page on archive.org, for the reader to open,
  // save and upload when neither the site nor the automatic copies gave the
  // text.
  const archived = (
    <p className="fine-print">
      {copyLink} or{" "}
      <a href={`https://web.archive.org/web/2/${articleUrl}`} target="_blank" rel="noreferrer">
        open the page on archive.org
      </a>{" "}
      to save it from there.
    </p>
  );

  return (
    <section className="reading" aria-labelledby="text-title">
      <h2 id="text-title" className="visually-hidden">
        Article text
      </h2>

      {content === null ? (
        <div className="notice">
          <h3>No text saved yet</h3>
          <p>Download it from the article&rsquo;s address, or upload the page if the site does not let servers in.</p>
          {actions}
        </div>
      ) : content.status === "failed" ? (
        <div className="notice notice-error" role="alert">
          <h3>The text could not be downloaded</h3>
          <p>
            The download failed because {problemText(content.problem)}. {howToUpload}
          </p>
          {actions}
          {archived}
        </div>
      ) : content.status === "incomplete" ? (
        <div className="notice notice-warn" role="alert">
          <h3>Only part of this article was saved</h3>
          <p>The page said the full text is for paying members, so this is a preview. {howToUpload}</p>
          {actions}
          {archived}
        </div>
      ) : (
        <div className="reading-head">
          <p className="fine-print">
            {content.words.toLocaleString("en-US")} words, {content.source === "fetched" ? "downloaded" : "uploaded"} on{" "}
            {formatDate(content.savedAt)}
            {content.missingImages > 0
              ? `. ${content.missingImages} ${content.missingImages === 1 ? "image" : "images"} could not be saved`
              : ""}
            {content.problem ? `. The last download failed (${problemText(content.problem)}); this text was kept` : ""}.
          </p>
          {/* The text is here: replacing it is rare, so it stays folded. */}
          <details className="text-options">
            <summary>Replace or delete the text</summary>
            {actions}
          </details>
        </div>
      )}

      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}

      {content && content.markdown ? (
        <div className="prose" id="article-text">
          <Markdown source={content.markdown} resolveImage={resolveImage} />
        </div>
      ) : null}
    </section>
  );
}
