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
| 400 | `invalid_choices` | The `choices` field of an import is not JSON of the documented shape, or is larger than 1 MiB |
| 400 | `invalid_query` | A list parameter is invalid, or the query string is malformed (for example pairs separated by `;`); `details` is a list of field problems |
| 400 | `invalid_json` | The body is not one JSON object with only the documented fields, spelled exactly (`Title` is not `title`) |
| 404 | `not_found` | Unknown route, or an article or collection that does not exist for this user (including malformed ids) |
| 404 | `content_not_found` | No text has been saved for the article yet |
| 413 | `file_too_large` | The upload is larger than `MAX_UPLOAD_BYTES`; `details.maxBytes` |
| 413 | `body_too_large` | A JSON body is larger than 64 KiB; `details.maxBytes` |
| 415 | `unsupported_media_type` | A JSON endpoint received a body whose `Content-Type` is not `application/json` |
| 422 | `too_many_records` | The file has more bookmarks than `MAX_IMPORT_RECORDS`; `details.maxRecords` |
| 422 | `invalid_choices` | An import choice breaks the rules (a name too long or blank, a position or suggestion not in the file); `details.field` names it, such as `bookmarks.12.tags` |
| 422 | `invalid_file` | An imported JSON file is not a ReadVault export this version can read |
| 422 | `no_text` | An uploaded article file holds no text |
| 409 | `collection_exists` | The user already has a collection with that name (compared by slug) |
| 422 | `validation_failed` | An article edit or a collection name is invalid; `details` is a list of field problems |
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

Imports take a browser bookmarks export in the Netscape HTML format (Chrome, Firefox, Safari, Edge), or a ReadVault export (`GET /api/export`) to restore a library. A file whose first non-blank character is `{` is read as an export: each article becomes a bookmark with its original URL, title, folder, bookmark date, tags, read time and its collection as the suggested collection, so the rules below apply as for any import (a missing article is added, a missing collection is created, an article without a collection goes back to it). The read time (`readAt`) is restored only on the articles the import adds; an article already in the library keeps its own read state. A JSON file that is not an export of `schemaVersion` 1 is rejected with `422 invalid_file`. Send it as `multipart/form-data` with the file in the `file` field. The file is streamed and never stored: the preview and the import each upload it.

Limits: `MAX_UPLOAD_BYTES` (default 10 MiB) and `MAX_IMPORT_RECORDS` (default 20,000 bookmarks).

### `POST /api/imports/preview`

Parses and validates the file without writing anything, and suggests a collection and tags for each bookmark.

- **Collection:** the bookmark's first folder below the browser's own root folders (headings marked `PERSONAL_TOOLBAR_FOLDER` or `UNFILED_BOOKMARKS_FOLDER`, such as "Bookmarks bar"). A bookmark directly in a root folder, or in none, gets no suggestion. The name follows the collection rules; a folder name over 64 characters gives the `collection_too_long` warning and no suggestion.
- **Tags:** from the bookmark's `TAGS` attribute.

`collections` lists every suggested collection, by slug, with how many bookmarks to import would go in it and whether the user already has a collection with that slug (`existing`).

```bash
curl -F file=@fixtures/bookmarks.html http://localhost:8080/api/imports/preview
```

```json
{
  "totals": { "detected": 11, "valid": 8, "invalid": 2, "duplicateInFile": 1, "existing": 0 },
  "collections": [
    { "name": "Reading", "slug": "reading", "bookmarks": 6, "existing": false }
  ],
  "candidates": [
    {
      "position": 1,
      "status": "valid",
      "originalUrl": "https://medium.com/@example/understanding-go-interfaces-1a2b3c4d5e6f?source=rss----5517fd7b58a6---4&utm_source=newsletter&utm_medium=email",
      "canonicalUrl": "https://medium.com/@example/understanding-go-interfaces-1a2b3c4d5e6f?source=rss----5517fd7b58a6---4",
      "title": "Understanding Go interfaces",
      "folder": ["Bookmarks bar", "Reading", "Go"],
      "collection": "Reading",
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

Error codes: `missing_url`, `url_too_long` (over 2,048 bytes), `unsupported_scheme` (anything but http and https), `invalid_url`. Warning codes: `duplicate_in_file`, `missing_title`, `collection_too_long`, `title_truncated` (over 500 characters), `tag_too_long` (over 64 characters; the tag is skipped), `text_repaired` (bytes that are not valid UTF-8, as in exports saved in a legacy encoding, became U+FFFD, and NUL bytes were dropped).

### `POST /api/imports`

Imports the file with the suggested collections and tags, changed by the optional `choices` field, and returns a report. It is idempotent: importing the same file again adds nothing.

Every imported article, new or already in the library, that has no saved text and no recorded download is queued, and the API downloads its text in the background the same way as `POST /api/articles/{id}/content/fetch`, mirror included. A queued article waits a minute before it is downloaded, so a single link, which the web app downloads right after adding it, is not downloaded twice; `DOWNLOAD_WORKERS` downloads run at once, with a pause between two downloads of each. A failed download is recorded on the article and not retried; a download cut short by an API restart is tried again, 3 times at most. Adding a single link goes through this import too.

`choices` is JSON and must come before `file` in the form, because the file is streamed; parts after the file are not read. Every key is optional:

```json
{
  "collections": { "Reading": "To read", "Misc": null },
  "bookmarks": {
    "12": { "collection": "Backend", "tags": ["go", "http"] },
    "13": { "collection": null }
  }
}
```

- `collections` renames a suggested collection for all its bookmarks (keyed by the suggested name, compared by slug), or drops it with `null`.
- `bookmarks` changes single bookmarks by their `position`: `collection` is a name or `null` for none (absent keeps the suggestion); `tags` replaces the file's tags (absent keeps them).
- A choice about a suggestion or a position that is not imported, a blank or over-long collection name, or a tag over 64 characters rejects the whole import with `invalid_choices` before anything is written.

```bash
curl -F choices='{"collections":{"Misc":null}}' -F file=@fixtures/bookmarks.html http://localhost:8080/api/imports
```

Collections that do not exist are created; a name with the slug of an existing collection reuses it and keeps its name.

```json
{
  "importId": "7a037732-8ce1-479f-a0a2-9c3088e72f3a",
  "totals": { "inserted": 8, "updated": 0, "skipped": 1, "failed": 2 },
  "items": [
    { "position": 3, "originalUrl": "https://GO.dev/blog/pipelines#stopping-short", "outcome": "skipped", "problems": [{ "code": "duplicate_in_file", "message": "the same URL appears earlier in the file" }] }
  ]
}
```

`items` lists every bookmark that was not inserted. `outcome` is `inserted`, `updated` (an existing article gained a missing title, folder, bookmark date, collection or tag), `skipped` (nothing to change) or `failed`. A `storage_error` problem means its batch of up to 500 bookmarks could not be saved; the other batches are kept, and importing the file again completes it. `importId` also appears in the server log line `import finished`.

Rules applied to existing articles:

- A title is filled only when it is empty and the user never edited it.
- The folder, bookmark date and collection are filled only when missing: an article already in a collection stays there.
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
  "collection": { "id": "0b6f4c1e-5b3a-4a52-9a49-3f1d2b7c8e10", "name": "Reading", "slug": "reading" },
  "readAt": null,
  "createdAt": "2026-11-07T23:27:07.5Z",
  "updatedAt": "2026-11-07T23:27:07.5Z"
}
```

Times are RFC 3339 in UTC. `canonicalUrl` identifies the article: the host is lowercased, the fragment and known tracking parameters (`utm_*`, `fbclid`, `gclid` and similar) are removed, and everything else is kept. Tags are sorted by slug; a tag's slug is its lowercase name with spaces turned into hyphens, and it is unique per user. `collection` is the one collection the article is in, or `null`. `readAt` is when the article was read, or `null` while it is unread.

### `GET /api/articles`

| Parameter | Meaning |
|---|---|
| `q` | Matches words of the title and URL, and any part of the title, URL or a tag name. `-word` leaves out articles with a word and `"exact words"` matches a phrase. Up to 200 characters |
| `tag` | Keeps articles that have this tag (name or slug). Repeat it to require several tags, up to 10 |
| `collection` | Keeps the articles of this collection (name or slug); an unknown collection gives an empty list, and a blank value is ignored |
| `text` | Keeps the articles whose saved text is `complete`, `incomplete` (a paid preview), `failed` (the download failed and nothing was saved) or `missing` (never downloaded); any other value is `text/invalid` |
| `read` | `true` keeps the read articles, `false` the unread ones; any other value is `read/invalid` |
| `sort` | `created_desc` (default), `created_asc`, `title_asc`, `title_desc` |
| `limit` | 1 to 100; 25 when absent |
| `cursor` | The `nextCursor` of the previous page |

```json
{ "items": [ /* articles */ ], "nextCursor": "eyJzIjoiY3JlYXRlZF9kZXNjIiwiay..." }
```

`nextCursor` is `null` on the last page. A cursor only works with the `sort` it came from. `q`, `tag` and `collection` must be valid UTF-8 without NUL bytes (`invalid`).

### `GET /api/articles/{id}`

Returns one article, or `404 not_found`.

### `PATCH /api/articles/{id}`

```json
{ "title": "My title", "tags": ["go", "Concurrency"], "collectionId": "0b6f4c1e-5b3a-4a52-9a49-3f1d2b7c8e10" }
```

Send it as `application/json`. Every field is optional, but at least one is required. Any other field, such as `originalUrl`, is rejected with `400 invalid_json`, so URLs cannot change here.

- `title` is trimmed and inner spaces collapsed. It must not be empty and can be up to 500 characters. A changed title is marked as edited (`titleEditedAt`), and imports never overwrite it.
- `tags` replaces the whole tag set: send `[]` to remove every tag. When the set changes, the tags are marked as edited (`tagsEditedAt`), and imports add no tags to the article from then on; sending the same set marks nothing. Up to 50 tags of up to 64 characters each. Tags that share a slug are merged, and an existing tag keeps its stored spelling.

- `collectionId` moves the article to that collection; an article is in at most one, so this replaces its current one. Send `null` to take it out of its collection. Tags are not affected.
- `read`: `true` marks the article read and sets `readAt`; marking a read article read again keeps the first `readAt`. `false` marks it unread (`readAt` becomes `null`). Reading is not an edit: `updatedAt` does not change. `null` is `422 validation_failed` with `read/invalid`, and any other value is `400 invalid_json`.

Returns the updated article. Field problem codes: `body/empty` (no field sent), `collectionId/not_found` (no collection with that id for this user), `title/required`, `title/too_long`, `title/invalid`, `tags/empty`, `tags/too_long`, `tags/too_many`, `tags/invalid`, `read/invalid` (`invalid` means the text is not valid UTF-8 or holds a NUL byte).

### `DELETE /api/articles/{id}`

Deletes the article for good, with its tags, saved text, images and queued download. A tag left without articles no longer appears in `GET /api/tags`. Returns `204`; `404 not_found` when the user has no such article (including a malformed id). Importing the same URL again saves it as a new article.

### `GET /api/articles/{id}/suggestions`

Suggests a collection and tags for one article from its content: its title, the words of its URL and its saved text, when it has one (the first 200,000 characters). Nothing is changed; apply a suggestion with `PATCH /api/articles/{id}`.

```json
{ "collection": { "id": "0b6f4c1e-5b3a-4a52-9a49-3f1d2b7c8e10", "name": "Streaming", "slug": "streaming" }, "tags": [{ "name": "kafka", "slug": "kafka" }] }
```

Only the user's own collections and tags are suggested; nothing new is invented.

- `collection` is the collection most common among the 7 library articles most similar to this one, compared by title and tags (rare words weigh more), among the 10,000 most recently added. It is `null` unless at least 2 of them share it and it has at least 1.5 times the votes of the next one.
- `tags` holds up to 5 tags the article does not have yet: tags named in the title or the URL, named at least twice in the text, or carried by at least 2 of the similar articles. A tag of 1 or 2 characters, such as `go`, counts only when the title names it. Words inside links and image addresses in the text are ignored.

Returns `404 not_found` for an unknown article.

### `GET /api/tags`

Returns every tag that has at least one article, with how many articles carry it: most articles first, then by name. Filter the library by a tag with `GET /api/articles?tag=<slug>`.

```json
{ "tags": [{ "name": "Go", "slug": "go", "articles": 12 }] }
```

## Collections

A collection groups articles. An article is in at most one collection; its tags are independent of it. Collections are flat.

```json
{ "id": "0b6f4c1e-5b3a-4a52-9a49-3f1d2b7c8e10", "name": "Reading", "slug": "reading", "articles": 12 }
```

A new library starts with nine collections (AI and tools, Architecture and system design, Backend, Data engineering, Databases and SQL, DevOps, cloud and Linux, Frontend and visualization, Machine learning and analytics, Streaming and Kafka), which can be renamed or deleted like any other. Names follow the tag rules: spaces are collapsed, up to 64 characters, and the slug (lowercase, spaces as hyphens) is unique per user. Field problem codes: `name/required`, `name/too_long`, `name/invalid`.

### `GET /api/collections`

Returns `{ "collections": [...] }`, sorted by name, with the number of articles in each.

### `POST /api/collections`

`{ "name": "Reading" }` as `application/json`. Returns `201` with the collection, or `409 collection_exists`.

### `PATCH /api/collections/{id}`

`{ "name": "To read" }`. Returns the renamed collection, `404 not_found` or `409 collection_exists`.

### `DELETE /api/collections/{id}`

Returns `204`. The collection's articles stay in the library, without a collection. `404 not_found` when it does not exist.

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

Downloads the article from its original URL and saves its text the same way. A page whose schema.org data says `isAccessibleForFree: false`, or that shows Medium's "Member-only story" label, is saved as `incomplete`. When an article the mirror reads (a Medium article on medium.com, a subdomain, or its own domain served by Medium, recognised by Medium's addresses in DNS; or an article of nytimes.com, theathletic.com, washingtonpost.com, economist.com, bloomberg.com, reuters.com or quora.com, subdomains included) answers without the article (an HTTP error such as 403, or a page without text) or with a paid preview, the mirror's copy (`ARTICLE_MIRROR_URL`, Freedium by default, `off` to turn it off; the prefix followed by the article's URL) is downloaded the same way; if it has text, is not a paid preview and is not the mirror's error page (Freedium answers 200 with an error in its page data when it cannot read an article), it is saved as `complete`, otherwise the original outcome stands. Any article, Medium or not, that answers the same way and that the mirror did not save is then looked up in the archive (`ARTICLE_ARCHIVE_URL`, the Wayback Machine by default, `off` to turn it off; the prefix followed by the article's URL), whose relative image addresses resolve against the original site. An article whose URL already starts with either prefix is not sent to a copy again, and an address that did not resolve or answer, or that the API refuses to download (`blocked_address`), is never sent to one, since it may be a private or intranet host.

`?from=copy` skips the original and asks only the copies (mirror, then archive); the web app offers it as **Try another copy** when a download failed or saved a paid preview. A copy with text replaces the saved text; when no copy has text the answer is `502 fetch_failed` with problem `no_copy`, and the saved text and recorded download are left as they were. Any other `from` value is `400 invalid_query`.

On failure the answer is `502 fetch_failed` with `details.problem`:

| Problem | Meaning |
|---|---|
| `blocked_address` | The address is not a public `http` or `https` address on port 80 or 443 (private, local and metadata addresses are refused, also after redirects) |
| `unreachable` | DNS or connection failure, more than 5 redirects, or a redirect to another scheme |
| `timeout` | No complete answer within 10 s |
| `http_status` | The site answered with another status; `details.status` holds it (Medium answers 403 to servers) |
| `not_html` | The answer is not an HTML page |
| `too_large` | The page is larger than `MAX_ARTICLE_BYTES` |
| `no_text` | The page holds no article text |
| `no_copy` | `?from=copy` only: neither the mirror nor the archive has the article's text |

A failed download keeps any saved text. Requests identify themselves as `ReadVault/0.1`.

Images: up to 30 per article, 5 MiB each and 30 MiB in total, downloaded with the same address rules. Only PNG, JPEG, GIF and WebP are kept, recognized from their bytes; SVG and anything else are counted in `missingImages`.

### `GET /api/content-downloads`

Counts the articles by the state of their text: `queued` wait for the background download, `missing` were never downloaded and are not queued, `failed` have a recorded download failure and no text (and are not queued), `incomplete` have only a paid preview.

```json
{ "queued": 312, "missing": 0, "failed": 7, "incomplete": 2 }
```

### `POST /api/content-downloads`

Queues the background download of every article never downloaded (the ones counted as `missing`) and answers `202` with how many were queued, `{ "queued": 518 }`. Failed and incomplete articles are not queued again; download them from their page. Calling it again queues nothing twice.

### `DELETE /api/contents` and `DELETE /api/collections/{id}/contents`

Deletes the saved texts, with their images and recorded download failures, of every article of the user, or of the articles of one collection, to free space. The articles, their tags and collections stay, and their queued background downloads are dropped. Uploaded texts are deleted too. Answers `200` with how many saved entries were deleted, `{ "removed": 84 }`; an unknown collection is `404` `not_found`. A download already running may still save its text afterwards.

### `DELETE /api/articles/{id}/content`

Deletes one article's saved text, with its images and recorded download failure, and drops its queued download. The article, its tags and collection stay; its text can be downloaded or uploaded again. Answers `200` with `{ "removed": 1 }` (`0` when it had nothing saved); `404 not_found` when the user has no such article (including a malformed id).

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

`schemaVersion` changes when a field is removed or changes meaning. A new optional field does not change it. The export holds only the current user's articles. Each article carries its `collection`; collections without articles are not part of it. Saved article text and images are not part of it; the database backup keeps them. Import the file (`POST /api/imports`) to restore it.
