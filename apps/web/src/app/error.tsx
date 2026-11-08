"use client";

export default function ErrorPage({ reset }: { error: Error; reset: () => void }) {
  return (
    <section className="notice notice-error" role="alert">
      <h2>Something went wrong while showing this page</h2>
      <p>Try again. If it keeps failing, check that the API is running.</p>
      <button className="button button-quiet" type="button" onClick={reset}>
        Try again
      </button>
    </section>
  );
}
