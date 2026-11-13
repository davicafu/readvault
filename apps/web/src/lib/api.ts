// Types and helpers for the ReadVault API. They mirror the Go handlers in
// services/api/internal/adapter/http.

export type Tag = { name: string; slug: string };

/** A tag with the number of articles that carry it (GET /api/tags). */
export type TagCount = Tag & { articles: number };

/** The collection an article is in. An article is in at most one. */
export type CollectionRef = { id: string; name: string; slug: string };

/** A collection with the number of its articles (GET /api/collections). */
export type Collection = CollectionRef & { articles: number };

// Suggestions for one article, from its content and the user's library.
export type Suggestions = { collection: CollectionRef | null; tags: Tag[] };

/** How many articles wait for, lack, or failed to get their text (GET /api/content-downloads). */
export type DownloadSummary = { queued: number; missing: number; failed: number; incomplete: number };

export type Article = {
  id: string;
  originalUrl: string;
  canonicalUrl: string;
  title: string;
  titleEditedAt: string | null;
  tagsEditedAt: string | null;
  contentStatus: ContentStatus | null;
  folder: string[];
  bookmarkedAt: string | null;
  tags: Tag[];
  collection: CollectionRef | null;
  createdAt: string;
  updatedAt: string;
};

export type ContentStatus = "complete" | "incomplete" | "failed";

export type ArticleContent = {
  articleId: string;
  status: ContentStatus;
  source: "fetched" | "uploaded_html" | "uploaded_markdown";
  markdown: string;
  words: number;
  problem: string | null;
  images: { id: string; sourceUrl: string }[];
  missingImages: number;
  savedAt: string;
};

// Why a download failed, for people. Keys are the API's problem codes.
export const downloadProblems: Record<string, string> = {
  blocked_address: "its address is not a public web address",
  unreachable: "the site could not be reached",
  timeout: "the site took too long to answer",
  http_status: "the site refused to send it to a server",
  not_html: "the address is not a web page",
  too_large: "the page is too large",
  no_text: "the page holds no article text",
  no_copy: "no other copy of the article was found",
};

export type ArticlePage = { items: Article[]; nextCursor: string | null };

export type Sort = "created_desc" | "created_asc" | "title_asc" | "title_desc";

export type Problem = { code: string; message: string };

export type FieldProblem = { field: string; code: string; message: string };

export type CandidateStatus = "valid" | "invalid" | "duplicate_in_file" | "existing";

export type Candidate = {
  position: number;
  status: CandidateStatus;
  originalUrl: string;
  canonicalUrl?: string;
  title: string;
  folder: string[];
  /** The suggested collection, or null for none. */
  collection: string | null;
  tags: string[];
  errors: Problem[];
  warnings: Problem[];
};

/** A collection the bookmarks of a file suggest. */
export type SuggestedCollection = { name: string; slug: string; bookmarks: number; existing: boolean };

export type ImportPreview = {
  totals: { detected: number; valid: number; invalid: number; duplicateInFile: number; existing: number };
  collections: SuggestedCollection[];
  candidates: Candidate[];
};

/**
 * The person's decisions on a preview, sent with the import. A suggested
 * collection maps to a new name or null (dropped); a bookmark's collection
 * is a name or null (none), and its tags replace the file's.
 */
export type ImportChoices = {
  collections?: Record<string, string | null>;
  bookmarks?: Record<number, { collection?: string | null; tags?: string[] }>;
};

export type ImportOutcome = "inserted" | "updated" | "skipped" | "failed";

export type ImportReport = {
  importId: string;
  totals: { inserted: number; updated: number; skipped: number; failed: number };
  items: { position: number; originalUrl: string; outcome: ImportOutcome; problems?: Problem[] }[];
};

/** An error response from the API: {"error": {"code", "message", "details"}}. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }

  /** Field problems sent with validation errors, when there are any. */
  get fieldProblems(): FieldProblem[] {
    return Array.isArray(this.details) ? (this.details as FieldProblem[]) : [];
  }
}

/** Returns the JSON body of a successful response or throws ApiError. */
export async function readJSON<T>(res: Response): Promise<T> {
  const body: unknown = await res.json().catch(() => null);
  if (res.ok) {
    return body as T;
  }
  const err = (body as { error?: { code?: string; message?: string; details?: unknown } } | null)?.error;
  throw new ApiError(
    res.status,
    err?.code ?? "unexpected_response",
    err?.message ?? `The server answered with status ${res.status}.`,
    err?.details,
  );
}

/** Only http and https URLs become links; anything else is shown as text. */
export function isWebUrl(url: string): boolean {
  return /^https?:\/\//i.test(url);
}

export function hostname(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}
