// A small Markdown renderer for saved article text. It builds React
// elements and never injects HTML, so text from any page is shown as text.
// Links open only http and https addresses; images are shown only from the
// copies ReadVault stored (resolveImage), never loaded from other sites.
//
// Supported: headings, paragraphs with hard breaks, emphasis, strong, inline
// code, fenced code, block quotes, nested lists, rules, links and images.

import { Fragment, type ReactNode } from "react";

export type ImageResolver = (src: string) => string | null;

// headingIds maps the line of each top-level heading to its anchor id.
type Ctx = { resolveImage: ImageResolver; headingIds: Map<number, string> };

export function Markdown({ source, resolveImage }: { source: string; resolveImage: ImageResolver }) {
  const lines = splitLines(source);
  const headingIds = new Map(scanHeadings(lines).map((h) => [h.line, h.id]));
  return <>{blocks(lines, { resolveImage, headingIds }, "b")}</>;
}

export type OutlineEntry = { id: string; level: number; text: string };

// outline lists the top-level headings of the text down to depth levels,
// with the same anchor ids Markdown gives them, for a table of contents.
export function outline(source: string, depth = 3): OutlineEntry[] {
  return scanHeadings(splitLines(source))
    .filter((h) => h.level <= depth)
    .map(({ id, level, text }) => ({ id, level, text }));
}

const splitLines = (source: string) => source.replace(/\r\n?/g, "\n").split("\n");

// scanHeadings finds the headings blocks renders at the top level: every
// heading line outside fenced code (a heading line always ends a paragraph,
// and lines in a block quote start with ">"). Ids are unique per text.
function scanHeadings(lines: string[]): { line: number; id: string; level: number; text: string }[] {
  const found: { line: number; id: string; level: number; text: string }[] = [];
  const seen = new Map<string, number>();
  for (let i = 0; i < lines.length; i++) {
    const fence = fenceOpen.exec(lines[i]);
    if (fence) {
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith(fence[1])) i++;
      continue;
    }
    const h = heading.exec(lines[i]);
    if (!h) continue;
    const text = plainText(h[2]);
    const base =
      "h-" +
      (text
        .toLowerCase()
        .normalize("NFD")
        .replace(/[\u0300-\u036f]/g, "")
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-|-$/g, "")
        .slice(0, 60) || "section");
    const n = (seen.get(base) ?? 0) + 1;
    seen.set(base, n);
    found.push({ line: i, id: n === 1 ? base : `${base}-${n}`, level: h[1].length, text });
  }
  return found;
}

// plainText drops the inline syntax of a heading: images, link targets,
// emphasis and code marks, and escapes.
function plainText(md: string): string {
  return md
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\\(.)/g, "$1")
    .replace(/[*_`~]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

const fenceOpen = /^ {0,3}(`{3,}|~{3,})/;
const heading = /^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const rule = /^ {0,3}([-*_])(\s*\1){2,}\s*$/;
const listItem = /^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$/;
const quote = /^ {0,3}> ?(.*)$/;

function blocks(lines: string[], ctx: Ctx, key: string): ReactNode[] {
  const out: ReactNode[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    const k = `${key}-${i}`;

    if (line.trim() === "") {
      i++;
      continue;
    }

    const fence = fenceOpen.exec(line);
    if (fence) {
      const marker = fence[1];
      const code: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith(marker)) {
        code.push(lines[i]);
        i++;
      }
      i++;
      out.push(
        <pre key={k}>
          <code>{code.join("\n")}</code>
        </pre>,
      );
      continue;
    }

    const h = heading.exec(line);
    if (h) {
      // The page title is the h1, so article headings start at h2.
      const level = Math.min(h[1].length + 1, 6);
      const Tag = `h${level}` as "h2" | "h3" | "h4" | "h5" | "h6";
      const id = key === "b" ? ctx.headingIds.get(i) : undefined;
      out.push(
        <Tag key={k} id={id}>
          {inline(h[2], ctx, k)}
        </Tag>,
      );
      i++;
      continue;
    }

    if (rule.test(line)) {
      out.push(<hr key={k} />);
      i++;
      continue;
    }

    if (quote.test(line)) {
      const inner: string[] = [];
      while (i < lines.length && lines[i].trim() !== "" && quote.test(lines[i])) {
        inner.push(quote.exec(lines[i])![1]);
        i++;
      }
      out.push(<blockquote key={k}>{blocks(inner, ctx, k)}</blockquote>);
      continue;
    }

    if (listItem.test(line)) {
      const items: { indent: number; ordered: boolean; text: string }[] = [];
      while (i < lines.length && listItem.test(lines[i])) {
        const m = listItem.exec(lines[i])!;
        items.push({ indent: m[1].replace(/\t/g, "    ").length, ordered: /\d/.test(m[2]), text: m[3] });
        i++;
      }
      out.push(list(items, 0, ctx, k).node);
      continue;
    }

    const para: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !fenceOpen.test(lines[i]) &&
      !heading.test(lines[i]) &&
      !rule.test(lines[i]) &&
      !quote.test(lines[i]) &&
      !listItem.test(lines[i])
    ) {
      para.push(lines[i]);
      i++;
    }
    out.push(<p key={k}>{paragraph(para, ctx, k)}</p>);
  }
  return out;
}

type Item = { indent: number; ordered: boolean; text: string };

// list renders items from start while their indent is at least the first
// item's; deeper items become nested lists.
function list(items: Item[], start: number, ctx: Ctx, key: string): { node: ReactNode; next: number } {
  const base = items[start].indent;
  const ordered = items[start].ordered;
  const children: ReactNode[] = [];
  let i = start;
  while (i < items.length && items[i].indent >= base) {
    if (items[i].indent > base) {
      i++;
      continue;
    }
    const item = items[i];
    const k = `${key}-${i}`;
    i++;
    let nested: ReactNode = null;
    if (i < items.length && items[i].indent > base) {
      const sub = list(items, i, ctx, k);
      nested = sub.node;
      i = sub.next;
    }
    children.push(
      <li key={k}>
        {inline(item.text, ctx, k)}
        {nested}
      </li>,
    );
  }
  const node = ordered ? <ol key={key}>{children}</ol> : <ul key={key}>{children}</ul>;
  return { node, next: i };
}

function paragraph(lines: string[], ctx: Ctx, key: string): ReactNode[] {
  const out: ReactNode[] = [];
  lines.forEach((line, i) => {
    const hardBreak = / {2,}$/.test(line) || line.endsWith("\\");
    const text = line.replace(/( {2,}|\\)$/, "").trim();
    out.push(<Fragment key={`${key}-${i}`}>{inline(text, ctx, `${key}-${i}`)}</Fragment>);
    if (i < lines.length - 1) out.push(hardBreak ? <br key={`${key}-${i}-br`} /> : " ");
  });
  return out;
}

const escapable = "\\`*_[]()#+-.!>~|{}";

// webURL accepts only absolute http and https addresses.
export function webURL(href: string): string | null {
  try {
    const u = new URL(href);
    return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
  } catch {
    return null;
  }
}

// bracketed reads "[text](target)" starting at the "[" at i. It returns the
// text, the target without an optional title, and the index after ")".
function bracketed(s: string, i: number): { text: string; target: string; end: number } | null {
  let depth = 0;
  let j = i;
  for (; j < s.length; j++) {
    if (s[j] === "\\") {
      j++;
    } else if (s[j] === "[") {
      depth++;
    } else if (s[j] === "]") {
      depth--;
      if (depth === 0) break;
    }
  }
  if (depth !== 0 || s[j + 1] !== "(") return null;
  const close = s.indexOf(")", j + 2);
  if (close < 0) return null;
  const target = s
    .slice(j + 2, close)
    .trim()
    .replace(/\s+"[^"]*"$/, "")
    .replace(/^<(.*)>$/, "$1");
  return { text: s.slice(i + 1, j), target, end: close + 1 };
}

function inline(s: string, ctx: Ctx, key: string): ReactNode[] {
  const out: ReactNode[] = [];
  let text = "";
  const flush = () => {
    if (text) out.push(text);
    text = "";
  };
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    const k = `${key}-${i}`;

    if (c === "\\" && i + 1 < s.length && escapable.includes(s[i + 1])) {
      text += s[i + 1];
      i += 2;
      continue;
    }

    if (c === "`") {
      const run = /^`+/.exec(s.slice(i))![0];
      const close = s.indexOf(run, i + run.length);
      if (close > 0) {
        flush();
        out.push(<code key={k}>{s.slice(i + run.length, close).trim()}</code>);
        i = close + run.length;
        continue;
      }
    }

    if (c === "!" && s[i + 1] === "[") {
      const b = bracketed(s, i + 1);
      if (b) {
        flush();
        const alt = b.text.replace(/\\(.)/g, "$1");
        const src = ctx.resolveImage(b.target);
        out.push(
          src ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img key={k} src={src} alt={alt} loading="lazy" />
          ) : (
            <span key={k} className="missing-image">
              Image not saved{alt ? `: ${alt}` : ""}
            </span>
          ),
        );
        i = b.end;
        continue;
      }
    }

    if (c === "[") {
      const b = bracketed(s, i);
      if (b) {
        flush();
        const href = webURL(b.target);
        const label = inline(b.text, ctx, k);
        out.push(
          href ? (
            <a key={k} href={href} target="_blank" rel="noopener noreferrer nofollow">
              {label}
            </a>
          ) : (
            <Fragment key={k}>{label}</Fragment>
          ),
        );
        i = b.end;
        continue;
      }
    }

    if ((c === "*" || c === "_") && s[i + 1] === c) {
      const close = s.indexOf(c + c, i + 2);
      if (close > i + 2) {
        flush();
        out.push(<strong key={k}>{inline(s.slice(i + 2, close), ctx, k)}</strong>);
        i = close + 2;
        continue;
      }
    }

    if ((c === "*" || c === "_") && s[i + 1] !== " " && s[i + 1] !== undefined) {
      let close = i + 1;
      while ((close = s.indexOf(c, close)) > 0 && (s[close - 1] === "\\" || s[close + 1] === c)) close += 2;
      if (close > i + 1) {
        flush();
        out.push(<em key={k}>{inline(s.slice(i + 1, close), ctx, k)}</em>);
        i = close + 1;
        continue;
      }
    }

    text += c;
    i++;
  }
  flush();
  return out;
}
