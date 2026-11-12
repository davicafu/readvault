// Shown at once while an article loads, in the shape of its page.
export default function Loading() {
  const widths = ["100%", "96%", "88%", "100%", "92%", "70%", "100%", "94%", "60%"];
  return (
    <div className="article-loading" aria-busy="true" aria-live="polite">
      <span className="visually-hidden">Loading the article…</span>
      <span className="skeleton skeleton-heading" aria-hidden="true" />
      {widths.map((w, i) => (
        <span key={i} className="skeleton skeleton-line" style={{ width: w }} aria-hidden="true" />
      ))}
    </div>
  );
}
