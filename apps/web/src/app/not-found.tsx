import Link from "next/link";

export default function NotFound() {
  return (
    <section className="notice">
      <h2>This page does not exist</h2>
      <p>The article may have been removed, or the address is wrong.</p>
      <Link className="button button-quiet" href="/">
        Back to the library
      </Link>
    </section>
  );
}
