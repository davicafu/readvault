"use client";

import { useEffect, useState, type CSSProperties } from "react";

import type { OutlineEntry } from "@/lib/markdown";

// ArticleToc lists the article's headings and marks the section being read.
// On narrow screens it folds behind a button above the text.
export function ArticleToc({ items }: { items: OutlineEntry[] }) {
  const [current, setCurrent] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const top = Math.min(...items.map((i) => i.level));

  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      // The current section is the last heading that has scrolled past
      // the upper fifth of the window.
      const line = window.innerHeight * 0.2;
      let found: string | null = null;
      for (const item of items) {
        const el = document.getElementById(item.id);
        if (el && el.getBoundingClientRect().top <= line) found = item.id;
      }
      setCurrent(found);
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
  }, [items]);

  return (
    <nav className="toc" aria-labelledby="toc-title" data-open={open}>
      <h2 id="toc-title" className="toc-title">
        <button type="button" className="toc-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
          Contents
        </button>
        <span className="toc-heading">Contents</span>
      </h2>
      <ol className="toc-list">
        {items.map((item) => (
          <li key={item.id} style={{ "--depth": item.level - top } as CSSProperties}>
            <a
              href={`#${item.id}`}
              aria-current={current === item.id ? "location" : undefined}
              onClick={() => setOpen(false)}
            >
              {item.text || "Untitled section"}
            </a>
          </li>
        ))}
      </ol>
    </nav>
  );
}
