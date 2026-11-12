// Shown at once while the library loads, in the shape of the collections,
// so the page builds up in place instead of appearing all of a sudden.
export default function Loading() {
  return (
    <div aria-busy="true" aria-live="polite">
      <span className="visually-hidden">Loading your library…</span>
      <span className="skeleton skeleton-title" />
      <ul className="skeleton-grid" aria-hidden="true">
        {Array.from({ length: 6 }, (_, i) => (
          <li key={i} className="skeleton skeleton-card" style={{ animationDelay: `${i * 90}ms` }} />
        ))}
      </ul>
    </div>
  );
}
