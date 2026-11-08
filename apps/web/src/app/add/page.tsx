import type { Metadata } from "next";

import { AddLink } from "./AddLink";
import { ImportFlow } from "./ImportFlow";

export const metadata: Metadata = { title: "Add articles" };

export default function AddPage() {
  return (
    <>
      <div className="page-head">
        <h1 className="page-title">Add articles</h1>
        <p className="lede">
          Save one link, or bring in every bookmark from your browser at once.
          When a site blocks the download of an article&apos;s text, open the
          article and upload the page you saved from your browser.
        </p>
      </div>

      <section className="add-section" aria-labelledby="add-link-heading">
        <h2 id="add-link-heading">Save a link</h2>
        <AddLink />
      </section>

      <section
        className="add-section"
        id="bookmarks"
        aria-labelledby="bookmarks-heading"
      >
        <h2 id="bookmarks-heading">Import a bookmarks file</h2>
        <p className="add-section-lede">
          Choose the HTML file your browser exports. You see what will happen
          before anything is saved, and importing the same file again never
          creates duplicates.
        </p>
        <ImportFlow />
      </section>
    </>
  );
}
