// Icons of the header menu, drawn like the collection icons: 24-unit
// strokes in the text colour.
const paths = {
  // Books on a shelf: every article.
  library: ["M4 4.5v15", "M8.5 4.5v15", "M13 6.5l4.2-1.4 3.3 13.6-4.2 1.3z", "M3 19.5h10"],
  // A folder: the collections.
  collections: ["M3.5 7.5a2 2 0 0 1 2-2h4l2 2.2h7a2 2 0 0 1 2 2v7.8a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"],
  // A plus in a circle: adding articles.
  add: ["M12 3.5a8.5 8.5 0 1 0 0 17a8.5 8.5 0 1 0 0-17z", "M12 8.5v7", "M8.5 12h7"],
  // An arrow into a tray: downloading the export.
  export: ["M12 4v11", "M7.5 10.5L12 15l4.5-4.5", "M4.5 16.5v2a1.5 1.5 0 0 0 1.5 1.5h12a1.5 1.5 0 0 0 1.5-1.5v-2"],
} as const;

export type NavIconName = keyof typeof paths;

export function NavIcon({ name }: { name: NavIconName }) {
  return (
    <svg
      className="nav-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {paths[name].map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}
