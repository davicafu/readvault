# ReadVault HTTP API

The Go API listens on `HTTP_ADDR` (default `:8080`). The web app forwards `/api/*` to it, so the browser uses one origin.

Until accounts exist, every request acts for the fixed development owner (`00000000-0000-0000-0000-000000000001`). The server sets it; a client cannot choose another user.

## Errors

Every error has the same shape:

```json
{ "error": { "code": "file_too_large", "message": "the file is larger than an import allows", "details": { "maxBytes": 10485760 } } }
```

`code` is stable and meant for programs; `message` is for people. `details` appears only where listed below. Responses never contain SQL errors, stack traces or other internal details; those go to the server log.

| Status | Code | When |
|---|---|---|
| 400 | `invalid_upload` | The import request is not `multipart/form-data`, or the form cannot be read or holds no part at all |
| 400 | `missing_file` | The form has no `file` field |
| 400 | `invalid_query` | A list parameter is invalid, or the query string is malformed (for example pairs separated by `;`); `details` is a list of field problems |
| 400 | `invalid_json` | The body is not one JSON object with only the documented fields, spelled exactly (`Title` is not `title`) |
| 404 | `not_found` | Unknown route, or an article that does not exist for this user (including malformed ids) |
| 404 | `content_not_found` | No text has been saved for the article yet |
| 413 | `file_too_large` | The upload is larger than `MAX_UPLOAD_BYTES`; `details.maxBytes` |
| 413 | `body_too_large` | A JSON body is larger than 64 KiB; `details.maxBytes` |
| 415 | `unsupported_media_type` | A JSON endpoint received a body whose `Content-Type` is not `application/json` |
| 422 | `too_many_records` | The file has more bookmarks than `MAX_IMPORT_RECORDS`; `details.maxRecords` |
| 422 | `no_text` | An uploaded article file holds no text |
| 422 | `validation_failed` | An article edit is invalid; `details` is a list of field problems |
| 500 | `internal` | Anything unexpected; retry later |
| 502 | `fetch_failed` | The article could not be downloaded from its URL; `details.problem` says why (see Article text) |
| 503 | `timeout` | The request took longer than the server's request timeout |

A field problem is `{ "field": "limit", "code": "out_of_range", "message": "limit must be between 1 and 100" }`.

## Health

| Request | Response |
|---|---|
| `GET /healthz` | `200 {"status":"ok"}` while the process runs |
| `GET /readyz` | `200 {"status":"ok"}` when Postgres answers within 2 s, otherwise `503 {"status":"unavailable"}` |

## Imports

Imports take a browser bookmarks export in the Netscape HTML format (Chrome, Firefox, Safari, Edge). Send it as `multipart/form-data` with the file in the `file` field. The file is streamed and never stored: the preview and the import each upload it.

Limits: `MAX_UPLOAD_BYTES` (default 10 MiB) and `MAX_IMPORT_RECORDS` (default 20,000 bookmarks).

### `POST /api/imports/preview`

Parses and validates the file without writing anything.

```bash
curl -F file=@fixtures/bookmarks.html http://localhost:8080/api/imports/preview
```

```json
{
  "totals": { "detected": 11, "valid": 8, "invalid": 2, "duplicateInFile": 1, "existing": 0 },
  "candidates": [
    {
      "position": 1,
      "status": "valid",
      "originalUrl": "https://medium.com/@example/understanding-go-interfaces-1a2b3c4d5e6f?source=rss----5517fd7b58a6---4&utm_source=newsletter&utm_medium=email",
      "canonicalUrl": "https://medium.com/@example/understanding-go-interfaces-1a2b3c4d5e6f?source=rss----5517fd7b58a6---4",
      "title": "Understanding Go interfaces",
      "folder": ["Bookmarks bar", "Reading", "Go"],
      "tags": ["go", "interfaces"],
      "errors": [],
      "warnings": []
    }
  ]
}
```

`status` is one of:

| Status | Meaning |
|---|---|
| `valid` | New; it will be added |
| `existing` | Already in the library; it will only gain missing details and tags |
| `duplicate_in_file` | Its canonical URL appears earlier in the file; the first one wins |
| `invalid` | It cannot be imported; see `errors` |

Error codes: `missing_url`, `url_too_long` (over 2,048 bytes), `unsupported_scheme` (anything but http and https), `invalid_url`. Warning codes: `duplicate_in_file`, `missing_title`, `title_truncated` (over 500 characters), `tag_too_long` (over 64 characters; the tag is skipped), `text_repaired` (bytes that are not valid UTF-8, as in exports saved in a legacy encoding, became U+FFFD, and NUL bytes were dropped).

### `POST /api/imports`

Imports the file and returns a report. It is idempotent: importing the same file again adds nothing.

```json
{
  "importId": "7a037732-8ce1-479f-a0a2-9c3088e72f3a",
  "totals": { "inserted": 8, "updated": 0, "skipped": 1, "failed": 2 },
  "items": [
    { "position": 3, "originalUrl": "https://GO.dev/blog/pipelines#stopping-short", "outcome": "skipped", "problems": [{ "code": "duplicate_in_file", "message": "the same URL appears earlier in the file" }] }
  ]
}
```

`items` lists every bookmark that was not inserted. `outcome` is `inserted`, `updated` (an existing article gained a missing title, folder, bookmark date or tag), `skipped` (nothing to change) or `failed`. A `storage_error` problem means its batch of up to 500 bookmarks could not be saved; the other batches are kept, and importing the file again completes it. `importId` also appears in the server log line `import finished`.

Rules applied to existing articles:

- A title is filled only when it is empty and the user never edited it.
- The folder and bookmark date are filled only when missing.
- Tags are only added, never removed, and never to an article whose tags the user edited (`tagsEditedAt`), so a tag the user removed does not come back.
- The original URL of the first import is kept.

## Articles

An article:

```json
{
  "id": "ad28cff6-c943-4457-ae8c-ea9097845a5c",
  "originalUrl": "https://go.dev/blog/?utm_source=x#top",
  "canonicalUrl": "https://go.dev/blog/",
  "title": "The Go Blog",
  "titleEditedAt": null,
  "tagsEditedAt": null,
  "contentStatus": "complete",
  "folder": ["Bookmarks bar", "Go"],
  "bookmarkedAt": "2023-11-14T22:15:00Z",
  "tags": [{ "name": "go", "slug": "go" }],
  "createdAt": "2026-11-07T23:27:07.5Z",
  "updatedAt": "2026-11-07T23:27:07.5Z"
}
```

Times are RFC 3339 in UTC. `canonicalUrl` identifies the article: the host is lowercased, the fragment and known tracking parameters (`utm_*`, `fbclid`, `gclid` and similar) are removed, and everything else is kept. Tags are sorted by slug; a tag's slug is its lowercase name with spaces turned into hyphens, and it is unique per user.

### `GET /api/articles`

| Parameter | Meaning |
|---|---|
| `q` | Matches words of the title and URL, and any part of the title, URL or a tag name. Up to 200 characters |
| `tag` | Keeps articles that have this tag (name or slug). Repeat it to require several tags, up to 10 |
| `sort` | `created_desc` (default), `created_asc`, `title_asc`, `title_desc` |
| `limit` | 1 to 100; 25 when absent |
| `cursor` | The `nextCursor` of the previous page |

```json
{ "items": [ /* articles */ ], "nextCursor": "eyJzIjoiY3JlYXRlZF9kZXNjIiwiay..." }
```

`nextCursor` is `null` on the last page. A cursor only works with the `sort` it came from. `q` and `tag` must be valid UTF-8 without NUL bytes (`invalid`).

### `GET /api/articles/{id}`

Returns one article, or `404 not_found`.

### `PATCH /api/articles/{id}`

```json
{ "title": "My title", "tags": ["go", "Concurrency"] }
```

Send it as `application/json`. Both fields are optional, but at least one is required. Any other field, such as `originalUrl`, is rejected with `400 invalid_json`, so URLs cannot change here.

- `title` is trimmed and inner spaces collapsed. It must not be empty and can be up to 500 characters. A changed title is marked as edited (`titleEditedAt`), and imports never overwrite it.
- `tags` replaces the whole tag set: send `[]` to remove every tag. When the set changes, the tags are marked as edited (`tagsEditedAt`), and imports add no tags to the article from then on; sending the same set marks nothing. Up to 50 tags of up to 64 characters each. Tags that share a slug are merged, and an existing tag keeps its stored spelling.

Returns the updated article. Field problem codes: `body/empty`, `title/required`, `title/too_long`, `title/invalid`, `tags/empty`, `tags/too_long`, `tags/too_many`, `tags/invalid` (`invalid` means the text is not valid UTF-8 or holds a NUL byte).

## Article text

An article can keep its text as Markdown, with the images of its body. The text comes from the article's own URL or from a file the user uploads. `contentStatus` on an article is `complete`, `incomplete` (the page said it is a paid preview), `failed` (the download failed and no text was saved yet) or `null` (nothing tried yet). The web app warns about `incomplete` and `failed` so the user can upload the page.

### `GET /api/articles/{id}/content`

```json
{
  "articleId": "ad28cff6-c943-4457-ae8c-ea9097845a5c",
  "status": "complete",
  "source": "uploaded_html",
  "markdown": "# Pipelines\n\nGo's concurrency primitives ...\n\n![Diagram](https://cdn.example/diagram.png)\n",
  "words": 3005,
  "problem": null,
  "images": [{ "id": "5b0b0c43-3a43-4f3e-9d0a-8f2b8f6f2f10", "sourceUrl": "https://cdn.example/diagram.png" }],
  "missingImages": 0,
  "savedAt": "2026-11-08T13:12:46Z"
}
```

`source` is `fetched`, `uploaded_html` or `uploaded_markdown`. The Markdown keeps the original image addresses; `images` maps each one to a stored copy. `missingImages` counts body images that could not be stored. `problem` is the code of the last failed download, also when older text was kept. `404 content_not_found` when nothing was saved.

### `POST /api/articles/{id}/content`

Saves the text of a file the user chose, sent as `multipart/form-data` in the `file` field: an HTML page (for example one saved from the browser) or a Markdown file (`.md`, `.markdown`, `.txt`). HTML is reduced to the article body (the `<article>` element, or the block with the most paragraph text) and converted to Markdown; navigation, scripts, forms and small images (avatars, icons, tracking pixels) are left out. Markdown is kept as is. The text counts as `complete` and replaces any saved text and images. Up to `MAX_ARTICLE_BYTES` (default 5 MiB): `413 file_too_large`; no text: `422 no_text`.

### `POST /api/articles/{id}/content/fetch`

Downloads the article from its original URL and saves its text the same way. A page whose schema.org data says `isAccessibleForFree: false`, or that shows Medium's "Member-only story" label, is saved as `incomplete`. On failure the answer is `502 fetch_failed` with `details.problem`:

| Problem | Meaning |
|---|---|
| `blocked_address` | The address is not a public `http` or `https` address on port 80 or 443 (private, local and metadata addresses are refused, also after redirects) |
| `unreachable` | DNS or connection failure, more than 5 redirects, or a redirect to another scheme |
| `timeout` | No complete answer within 10 s |
| `http_status` | The site answered with another status; `details.status` holds it (Medium answers 403 to servers) |
| `not_html` | The answer is not an HTML page |
| `too_large` | The page is larger than `MAX_ARTICLE_BYTES` |
| `no_text` | The page holds no article text |

A failed download keeps any saved text. Requests identify themselves as `ReadVault/0.1`.

Images: up to 30 per article, 5 MiB each and 30 MiB in total, downloaded with the same address rules. Only PNG, JPEG, GIF and WebP are kept, recognized from their bytes; SVG and anything else are counted in `missingImages`.

### `GET /api/articles/{id}/images/{imageId}`

Returns a stored image with its type, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; sandbox` and `Cache-Control: private, max-age=86400`.

## Export

### `GET /api/export`

Downloads every article of the user, oldest first, as `readvault-export-<date>-<time>.json`:

```json
{
  "schemaVersion": 1,
  "exportedAt": "2026-11-08T02:53:55Z",
  "articles": [ /* articles, in the format above */ ]
}
```

`schemaVersion` changes when a field is removed or changes meaning. A new optional field does not change it. The export holds only the current user's articles. Saved article text and images are not part of it; the database backup keeps them.
