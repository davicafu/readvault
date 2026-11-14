"use client";

import { ActionIcon } from "./ActionIcon";

export default function ErrorPage({ reset }: { error: Error; reset: () => void }) {
  return (
    <section className="notice notice-error" role="alert">
      <h2>Something went wrong while showing this page</h2>
      <p>Try again. If it keeps failing, check that the API is running.</p>
      <button className="button button-quiet button-with-icon" type="button" onClick={reset}>
        <ActionIcon name="retry" />
        Try again
      </button>
    </section>
  );
}
