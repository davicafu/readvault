"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";

import { readJSON, type Article } from "@/lib/api";
import { ActionIcon } from "../ActionIcon";

// The width of an action button once the entry is swiped open.
const ACTION = 96;
// How far a finger moves before the gesture counts as a swipe or a scroll.
const SLOP = 8;
// Opening one entry closes any other one.
const OPENED = "readvault:swipe-open";

// to is where the move left the entry: state may not have caught up yet
// when the finger lifts after a quick swipe.
type Drag = { x: number; y: number; from: number; to: number; axis: "x" | "y" | null };

// SwipeActions is a list entry (an <li>) that a touch screen can swipe open:
// to the left it shows Delete, which deletes the article for good; to the
// right it shows Mark read, or Mark unread for a read article. A mouse or a
// keyboard does not swipe: the article page has both actions for them. A
// tap on an open entry closes it instead of opening the article.
export function SwipeActions({
  id,
  title,
  readAt,
  children,
}: {
  id: string;
  title: string;
  readAt: string | null;
  children: ReactNode;
}) {
  const router = useRouter();
  const [offset, setOffset] = useState(0);
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState<"delete" | "read" | null>(null);
  const [gone, setGone] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const drag = useRef<Drag | null>(null);
  // Until when a click is ignored: the click that may follow a touch that
  // swiped or closed the entry must not follow a link. It expires, so a later
  // mouse click or Enter on the entry still opens the article.
  const swallowUntil = useRef(0);
  const swallowNextClick = () => {
    swallowUntil.current = Date.now() + 500;
  };
  const open = offset !== 0;
  const name = title || "Untitled bookmark";

  useEffect(() => {
    const onOpened = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== id) setOffset(0);
    };
    window.addEventListener(OPENED, onOpened);
    return () => window.removeEventListener(OPENED, onOpened);
  }, [id]);

  const settle = (next: number) => {
    setOffset(next);
    if (next !== 0) window.dispatchEvent(new CustomEvent(OPENED, { detail: id }));
  };

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    if (e.pointerType !== "touch" || busy) return;
    drag.current = { x: e.clientX, y: e.clientY, from: offset, to: offset, axis: null };
  };

  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    if (d.axis === null) {
      if (Math.abs(dx) < SLOP && Math.abs(dy) < SLOP) return;
      d.axis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      if (d.axis === "y") {
        drag.current = null; // a scroll: leave it to the page
        return;
      }
      try {
        e.currentTarget.setPointerCapture(e.pointerId);
      } catch {
        // the pointer is already gone; the move still slides the entry
      }
      setDragging(true);
    }
    d.to = Math.min(ACTION * 1.4, Math.max(-ACTION * 1.4, d.from + dx));
    setOffset(d.to);
  };

  const onPointerUp = () => {
    const d = drag.current;
    drag.current = null;
    if (!d) return;
    if (d.axis === "x") {
      setDragging(false);
      swallowNextClick();
      settle(d.to < -ACTION / 2 ? -ACTION : d.to > ACTION / 2 ? ACTION : 0);
    } else if (open) {
      // A tap on an open entry closes it.
      swallowNextClick();
      setOffset(0);
    }
  };

  const onPointerCancel = () => {
    drag.current = null;
    setDragging(false);
    setOffset((o) => (o < -ACTION / 2 ? -ACTION : o > ACTION / 2 ? ACTION : 0));
  };

  const remove = async () => {
    setBusy("delete");
    setFailure(null);
    try {
      const res = await fetch(`/api/articles/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (!res.ok && res.status !== 404) await readJSON(res);
      setGone(true);
      router.refresh();
    } catch {
      setFailure("The article could not be deleted. Try again in a moment.");
      setBusy(null);
      setOffset(0);
    }
  };

  // The list shows the new state once the page reloads its data; a list
  // filtered to unread or read articles then leaves this one out.
  const toggleRead = async () => {
    setBusy("read");
    setFailure(null);
    try {
      await readJSON<Article>(
        await fetch(`/api/articles/${encodeURIComponent(id)}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ read: readAt === null }),
        }),
      );
      router.refresh();
    } catch {
      setFailure("The read state could not be saved. Try again in a moment.");
    } finally {
      setBusy(null);
      setOffset(0);
    }
  };

  if (gone) return null;

  return (
    <li className="entry" data-dragging={dragging ? "" : undefined}>
      <button
        type="button"
        className="swipe-action swipe-read"
        style={{ width: Math.max(0, offset) }}
        hidden={offset <= 0}
        tabIndex={offset > 0 && !dragging ? 0 : -1}
        disabled={busy !== null}
        onClick={toggleRead}
        aria-label={`${readAt ? "Mark as unread" : "Mark as read"}: “${name}”`}
      >
        <ActionIcon name={readAt ? "unread" : "read"} />
        {busy === "read" ? "Saving…" : readAt ? "Mark unread" : "Mark read"}
      </button>
      <button
        type="button"
        className="swipe-action swipe-delete"
        style={{ width: Math.max(0, -offset) }}
        hidden={offset >= 0}
        tabIndex={offset < 0 && !dragging ? 0 : -1}
        disabled={busy !== null}
        onClick={remove}
        aria-label={`Delete “${name}”`}
      >
        <ActionIcon name="delete" />
        {busy === "delete" ? "Deleting…" : "Delete"}
      </button>
      <div
        className="entry-body"
        style={{ translate: `${offset}px 0` }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerCancel}
        onClickCapture={(e) => {
          if (Date.now() >= swallowUntil.current) return;
          swallowUntil.current = 0;
          e.preventDefault();
          e.stopPropagation();
        }}
      >
        {children}
      </div>
      {failure ? (
        <p className="field-error" role="alert">
          {failure}
        </p>
      ) : null}
    </li>
  );
}
