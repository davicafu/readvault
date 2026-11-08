// Types and helpers for the ReadVault API. They mirror the Go handlers in
// services/api/internal/adapter/http.

export type Tag = { name: string; slug: string };

export type Article = {
  id: string;
  originalUrl: string;
  canonicalUrl: string;
  title: string;
  titleEditedAt: string | null;
  folder: string[];
  bookmarkedAt: string | null;
  tags: Tag[];
  createdAt: string;
  updatedAt: string;
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
  tags: string[];
  errors: Problem[];
  warnings: Problem[];
};

export type ImportPreview = {
  totals: { detected: number; valid: number; invalid: number; duplicateInFile: number; existing: number };
  candidates: Candidate[];
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
