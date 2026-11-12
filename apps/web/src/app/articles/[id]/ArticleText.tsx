"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, useTransition, type ChangeEvent } from "react";

import { ApiError, downloadProblems, readJSON, type ArticleContent } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { Markdown } from "@/lib/markdown";

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
  const [busy, setBusy] = useState<"download" | "upload" | null>(null);
  const [, startRefresh] = useTransition();

  const run = async (kind: "download" | "upload", request: () => Promise<Response>) => {
    setBusy(kind);
    setFailure(null);
    try {
      await readJSON<ArticleContent>(await request());
    } catch (err) {
      if (err instanceof ApiError && err.code === "fetch_failed") {
        const d = err.details as { problem?: string; status?: number } | undefined;
        setFailure(`The text could not be downloaded: ${problemText(d?.problem ?? null, d?.status)}.`);
      } else if (err instanceof ApiError && err.code === "no_text") {
        setFailure("That file holds no article text. Choose the page you saved from the browser.");
      } else if (err instanceof ApiError && err.code === "file_too_large") {
        setFailure("That file is too large for one article.");
      } else {
        setFailure(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
      }
    } finally {
      setBusy(null);
      startRefresh(() => router.refresh());
    }
  };

  const download = () =>
    run("download", () => fetch(`/api/articles/${articleId}/content/fetch`, { method: "POST" }));

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
      <button className="button" type="button" disabled={busy !== null} onClick={() => fileInput.current?.click()}>
        {busy === "upload" ? "Uploading…" : "Upload page"}
      </button>
      <button className="button button-quiet" type="button" disabled={busy !== null} onClick={download}>
        {busy === "download" ? "Downloading…" : content?.status === "complete" ? "Download again" : "Download text"}
      </button>
      <input ref={fileInput} type="file" accept={accepted} hidden onChange={upload} />
    </div>
  );

  const howToUpload =
    "Open the article in your browser, save it as a web page (HTML), and upload that file here. A Markdown file works too.";

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
        </div>
      ) : content.status === "incomplete" ? (
        <div className="notice notice-warn" role="alert">
          <h3>Only part of this article was saved</h3>
          <p>The page said the full text is for paying members, so this is a preview. {howToUpload}</p>
          {actions}
        </div>
      ) : (
        <div className="reading-head">
          <p className="fine-print">
            {content.words.toLocaleString()} words, {content.source === "fetched" ? "downloaded" : "uploaded"} on{" "}
            {formatDate(content.savedAt)}
            {content.missingImages > 0
              ? `. ${content.missingImages} ${content.missingImages === 1 ? "image" : "images"} could not be saved`
              : ""}
            {content.problem ? `. The last download failed (${problemText(content.problem)}); this text was kept` : ""}.
          </p>
          {actions}
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
