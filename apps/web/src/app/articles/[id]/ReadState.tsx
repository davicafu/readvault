"use client";

import { useEffect, useRef, useState } from "react";

import { ApiError, readJSON, type Article } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { ActionIcon } from "../../ActionIcon";

// The share of the text that counts as read when scrolled past.
const END = 0.98;

// ReadState shows whether the article was read, with a button to mark it
// read or unread. Scrolling to the end of the saved text (the element with
// targetId) marks it read on its own, once per visit, unless the reader
// marked it unread during this visit. A text that fits on the screen is
// never marked on its own: the button does it.
export function ReadState({ id, readAt, targetId }: { id: string; readAt: string | null; targetId: string }) {
  const [read, setRead] = useState(readAt);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  // Set once the reader marks it unread here, or once it was marked read on
  // its own: either way, scrolling does not mark it again on this visit.
  const settled = useRef(readAt !== null);

  const save = async (next: boolean) => {
    setBusy(true);
    setFailure(null);
    try {
      const a = await readJSON<Article>(
        await fetch(`/api/articles/${encodeURIComponent(id)}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ read: next }),
        }),
      );
      setRead(a.readAt);
    } catch (err) {
      setFailure(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    if (settled.current) return;
    let frame = 0;
    const check = () => {
      frame = 0;
      const el = document.getElementById(targetId);
      if (!el || settled.current) return;
      const rect = el.getBoundingClientRect();
      const span = rect.height - window.innerHeight;
      if (span > 0 && -rect.top / span >= END) {
        settled.current = true;
        void save(true);
      }
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(check);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.removeEventListener("scroll", onScroll);
      if (frame) cancelAnimationFrame(frame);
    };
    // save depends only on id, which restarts this effect when it changes.
  }, [id, targetId]);

  const toggle = () => {
    settled.current = true;
    void save(read === null);
  };

  return (
    <div className="read-state">
      {read ? (
        <span className="read-state-note">
          <ActionIcon name="read" />
          Read on <time dateTime={read}>{formatDate(read)}</time>
        </span>
      ) : null}
      <button className="button button-quiet button-small button-with-icon" type="button" onClick={toggle} disabled={busy}>
        <ActionIcon name={read ? "unread" : "read"} />
        {read ? "Mark as unread" : "Mark as read"}
      </button>
      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}
    </div>
  );
}
