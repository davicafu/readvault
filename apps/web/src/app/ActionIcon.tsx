// Icons of the buttons that change things, drawn like the menu icons:
// 24-unit strokes in the text colour.
const paths = {
  // A pencil: renaming.
  rename: ["M14.5 5.5l4 4", "M4.5 19.5l1-4.5L16 4.5a1.4 1.4 0 0 1 2 0l1.5 1.5a1.4 1.4 0 0 1 0 2L9 18.5z"],
  // A page with a cross: deleting a saved text.
  clearText: [
    "M13.5 3.5H7a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V9z",
    "M13.5 3.5V9H19",
    "M9.5 12.5l5 5",
    "M14.5 12.5l-5 5",
  ],
  // A bin: deleting for good.
  delete: ["M4.5 6.5h15", "M9.5 6.5V4.5h5v2", "M6.5 6.5l1 13h9l1-13", "M10.5 10.5v6", "M13.5 10.5v6"],
  // An arrow out of a tray: uploading a file.
  upload: ["M12 15V4", "M7.5 8.5L12 4l4.5 4.5", "M4.5 16.5v2a1.5 1.5 0 0 0 1.5 1.5h12a1.5 1.5 0 0 0 1.5-1.5v-2"],
  // An arrow into a tray: downloading.
  download: ["M12 4v11", "M7.5 10.5L12 15l4.5-4.5", "M4.5 16.5v2a1.5 1.5 0 0 0 1.5 1.5h12a1.5 1.5 0 0 0 1.5-1.5v-2"],
  // A tick: saving changes.
  save: ["M5 12.5l4.5 4.5L19 7.5"],
  // A plus in a circle: adding links.
  add: ["M12 3.5a8.5 8.5 0 1 0 0 17a8.5 8.5 0 1 0 0-17z", "M12 8.5v7", "M8.5 12h7"],
  // An open book: marking an article read.
  read: ["M12 6.5c-2-1.5-4.5-2-7.5-2v13c3 0 5.5.5 7.5 2c2-1.5 4.5-2 7.5-2v-13c-3 0-5.5.5-7.5 2z", "M12 6.5v13"],
  // A closed book: marking an article unread again.
  unread: ["M6.5 4.5h11v15h-11a1.5 1.5 0 0 1-1.5-1.5V6a1.5 1.5 0 0 1 1.5-1.5z", "M5 16.5a1.5 1.5 0 0 1 1.5-1.5h11"],
  // A circling arrow: trying again.
  retry: ["M19.5 12a7.5 7.5 0 1 1-2.2-5.3", "M19.5 4.5v4h-4"],
  // A bell: notifications.
  bell: [
    "M6 16.5V11a6 6 0 0 1 12 0v5.5l1.5 2h-15z",
    "M10 20.5a2 2 0 0 0 4 0",
  ],
} as const;

export type ActionIconName = keyof typeof paths;

export function ActionIcon({ name, className = "action-icon" }: { name: ActionIconName; className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
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
