"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";

import { readJSON } from "@/lib/api";
import { ActionIcon } from "../ActionIcon";

// The width of the Delete button once the entry is swiped open.
const ACTION = 96;
// How far a finger moves before the gesture counts as a swipe or a scroll.
const SLOP = 8;
// Opening one entry closes any other one.
const OPENED = "readvault:swipe-open";

type Drag = { x: number; y: number; from: number; axis: "x" | "y" | null };

// SwipeDelete is a list entry (an <li>) that a touch screen can swipe to the
// left to show a Delete button; tapping it deletes the article for good. A mouse or a
// keyboard does not swipe: the article page has Delete for them. A tap on an
// open entry closes it instead of opening the article.
export function SwipeDelete({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  const router = useRouter();
  const [offset, setOffset] = useState(0);
  const [dragging, setDragging] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [gone, setGone] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const drag = useRef<Drag | null>(null);
  // Set when the last touch swiped or closed the entry, so its click does
  // not follow a link.
  const swallowClick = useRef(false);
  const open = offset !== 0;

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
    if (e.pointerType !== "touch" || deleting) return;
    drag.current = { x: e.clientX, y: e.clientY, from: offset, axis: null };
    swallowClick.current = false;
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
    setOffset(Math.min(0, Math.max(-ACTION * 1.4, d.from + dx)));
  };

  const onPointerUp = () => {
    const d = drag.current;
    drag.current = null;
    if (!d) return;
    if (d.axis === "x") {
      setDragging(false);
      swallowClick.current = true;
      settle(offset < -ACTION / 2 ? -ACTION : 0);
    } else if (open) {
      // A tap on an open entry closes it.
      swallowClick.current = true;
      setOffset(0);
    }
  };

  const onPointerCancel = () => {
    drag.current = null;
    setDragging(false);
    setOffset((o) => (o < -ACTION / 2 ? -ACTION : 0));
  };

  const remove = async () => {
    setDeleting(true);
    setFailure(null);
    try {
      const res = await fetch(`/api/articles/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (!res.ok && res.status !== 404) await readJSON(res);
      setGone(true);
      router.refresh();
    } catch {
      setFailure("The article could not be deleted. Try again in a moment.");
      setDeleting(false);
      setOffset(0);
    }
  };

  if (gone) return null;

  return (
    <li className="entry" data-dragging={dragging ? "" : undefined}>
      <button
        type="button"
        className="swipe-delete"
        style={{ width: -offset }}
        hidden={!open}
        tabIndex={open && !dragging ? 0 : -1}
        disabled={deleting}
        onClick={remove}
        aria-label={`Delete “${title || "Untitled bookmark"}”`}
      >
        <ActionIcon name="delete" />
        {deleting ? "Deleting…" : "Delete"}
      </button>
      <div
        className="entry-body"
        style={{ translate: `${offset}px 0` }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerCancel}
        onClickCapture={(e) => {
          if (!swallowClick.current) return;
          swallowClick.current = false;
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
