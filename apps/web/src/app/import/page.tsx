import type { Metadata } from "next";

import { ImportFlow } from "./ImportFlow";

export const metadata: Metadata = { title: "Import bookmarks" };

export default function ImportPage() {
  return (
    <>
      <div className="page-head">
        <h1 className="page-title">Import bookmarks</h1>
        <p className="lede">
          Choose the HTML file your browser exports. You see what will happen before anything is saved, and importing the
          same file again never creates duplicates.
        </p>
      </div>
      <ImportFlow />
    </>
  );
}
