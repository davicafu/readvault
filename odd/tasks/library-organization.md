# Library organization

Objective: make the library page a search page, move adding articles to a secondary page, and add ways to organize articles (tag panel, collections). Branch: `feat/readvault-16-library-organization` (stacked on `feat/readvault-15-article-content`).
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -tags=integration ./...`; web has no test runner (tsc, `next build`, smoke through the proxy).

## Specs

- **S1** Home page is for searching: "la pagina inicial deberia ser para buscar articulos".
- **S2** Adding is secondary: "la parte de añadir un link, o mediante html o boorkmars, deberia ser secundario". Adding a link and importing a bookmarks file move to their own page, linked from the main navigation.
- **S3** Tag panel ("1", answering the question about other ways to organize): next to the results, every tag with how many articles it has; selecting one filters the list. Needs `GET /api/tags`.
- **S4** Collections ("3"): group articles into named collections, separate from tags. Rule (L4): "un articulo solo puede estar en una coleccion pero puede tener varias etiquetas que hagan referencia a otras colecciones".
  - An article is in at most one collection; tags stay as they are and are independent of collections.
  - Collections are flat (assumption: no nesting; the user did not ask for it).
  - The user creates, renames and deletes collections; deleting one leaves its articles without a collection, never deletes them.
  - The library can be filtered by collection, alongside search and tags.
  - Assumption: "etiquetas que hagan referencia a otras colecciones" needs nothing extra; a tag may share a collection's name. To confirm with the user.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S3 | inline | done (93ed9bf) | `GET /api/tags`. Service tests RED (build failure: no Tags) then GREEN: counts per tag, most articles first then by name, tags without articles and other users' tags left out, empty library gives `[]`. Handler RED (404) then GREEN, internal errors hidden. Full Go suite with `-race` passes |
| T2 | S1, S2, S3 | inline | done (b7a8b51; visual review by the user pending) | No web test runner: tsc passes (the web app has no lint script; an earlier claim that lint passed was wrong). Smoke on `make dev`: home shows the search first and no add form; `/add` 200 with "Save a link" and "Import a bookmarks file"; `/import` 308 to `/add#bookmarks`; tag panel hidden while the library has no tags |
| T3a | S4 | inline (Risk 1: migration 000004; Risk 3: article JSON and PATCH gain a field) | done (138588c; review fixes ad097e1) | Integration tests RED (build failure) then GREEN: create normalizes the name and rejects duplicates per user; invalid names; list by name with counts, other users' hidden; one collection per article (assign, move, remove with null, title-only edit keeps it); another user's, unknown and malformed ids give `collectionId/not_found` with no change; rename and delete scoped to the user, delete keeps the article; filter by collection, combined with a tag. Handler tests RED (404) then GREEN. Migration up/down/up test passes. Full Go suite with `-race` passes. Native review approved and acknowledged (lineage review-b8dfce015093dc9a); its warnings fixed in ad097e1: rename checks the id first (RED then GREEN), blank collection filter ignored and documented, export carries the collection (test), PostgreSQL 15+ noted in the README and the migration |
| T3b | S4 | inline | done (02c4331, verifier blocker fixed in ec071f9; visual review by the user pending) | tsc passes (no lint script exists). Smoke on `make dev` with an empty test collection, deleted afterwards (no article touched): migration applied; create 201 with the name normalized, duplicate 409; home shows the Collections panel with the collection and Manage; `/collections` lists it; `?collection=` shows the filter chip; the article page offers it in the picker; rename 200; delete through the proxy failed with 405 because the proxy did not forward DELETE: fixed, then 204 and 404 on a second delete. Native assess of ad097e1..02c4331: medium, `under_budget` (review pending in the slice) |

## Log

- L1 (2026-11-08, user): "vale ahora si, pero la pagina inicial deberia ser para buscar articulos, y me gustaria alguna otra forma de organizarlo, la parte de añadir un link,  o mediante html o boorkmars, deberia ser secundario"
- L2 (2026-11-08, user, choosing among: tag panel, reading status, collections, page only): "la 1 y la 3 serían las que me interesarían"
- L3: T1 and T2 start now; collection rules (one or many collections per article, nesting) are asked after them.
- L4 (2026-11-09, user): "no, un articulo solo puede estar en una coleccion pero puede tener varias etiquetas que hagan referencia a otras colecciones"
- L5: Collections follow L4 as specified: one per article, flat, tags independent. Assumption recorded in S4: a tag may share a collection's name, with nothing linking them; to confirm with the user.
- L6: Independent verification of the whole feature (Risk 1 in T3a) started against baseline cae55ab.
- L7: Independent verifier (baseline cae55ab, code at 02c4331): S1, S2, S3 pass; S4 failed on one blocker, the search form dropped the active collection filter. Fixed in ec071f9 (hidden `collection` input; the rendered form carries it). Probes passed: migration up/down/up on a seeded database with row hashes unchanged, FK rejects another user's collection, deleting a collection keeps its articles; two APIs side by side showed no change against the baseline except the new `collection` field and the `body/empty` message; rejected PATCHes change no data; ownership on every collection endpoint. Advisories left as they are: an edit with a collection deleted in another tab is rejected whole (422); `?tag=Go` written as a name filters but does not highlight in the panel. Scratch removed (verifier dropped its databases and worktree; the parent removed its temp directory).
