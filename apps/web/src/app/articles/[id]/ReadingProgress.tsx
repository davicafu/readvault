"use client";

import { useEffect, useRef } from "react";

// ReadingProgress is a thin bar at the top of the window that fills as the
// article text (the element with targetId) is read. It is decoration for
// sighted readers; the table of contents carries the same information.
export function ReadingProgress({ targetId }: { targetId: string }) {
  const bar = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      const el = document.getElementById(targetId);
      if (!el || !bar.current) return;
      const rect = el.getBoundingClientRect();
      const span = rect.height - window.innerHeight;
      const done = span <= 0 ? (rect.bottom <= window.innerHeight ? 1 : 0) : -rect.top / span;
      bar.current.style.transform = `scaleX(${Math.min(1, Math.max(0, done))})`;
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      if (frame) cancelAnimationFrame(frame);
    };
  }, [targetId]);

  return (
    <div className="reading-progress" aria-hidden="true">
      <div ref={bar} className="reading-progress-bar" />
    </div>
  );
}
