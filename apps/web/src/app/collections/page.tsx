import type { Metadata } from "next";
import Link from "next/link";

import type { Collection } from "@/lib/api";
import { apiGet } from "@/lib/server-api";

import { CollectionsManager } from "./CollectionsManager";

export const metadata: Metadata = { title: "Collections" };

export default async function CollectionsPage() {
  let collections: Collection[] | null = null;
  try {
    collections = (await apiGet<{ collections: Collection[] }>("/api/collections")).collections;
  } catch {
    collections = null;
  }

  return (
    <>
      <div className="page-head">
        <p className="fine-print">
          <Link href="/">Library</Link>
        </p>
        <h1 className="page-title">Collections</h1>
        <p className="lede">
          A collection groups articles. Each article is in one collection at most, and keeps its tags. Choose an
          article&apos;s collection on the article page.
        </p>
      </div>
      {collections ? (
        <CollectionsManager initial={collections} />
      ) : (
        <section className="notice notice-error" role="alert">
          <h2>The collections could not be loaded</h2>
          <p>The ReadVault API is not running or not reachable. Start it with make start and reload this page.</p>
        </section>
      )}
    </>
  );
}
