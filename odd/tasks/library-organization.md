# Library organization

Objective: make the library page a search page, move adding articles to a secondary page, and add ways to organize articles (tag panel, collections). Branch: `feat/readvault-16-library-organization` (stacked on `feat/readvault-15-article-content`).
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -tags=integration ./...`; web has no test runner (tsc, `next build`, smoke through the proxy).

## Specs

- **S1** Home page is for searching: "la pagina inicial deberia ser para buscar articulos".
- **S2** Adding is secondary: "la parte de añadir un link, o mediante html o boorkmars, deberia ser secundario". Adding a link and importing a bookmarks file move to their own page, linked from the main navigation.
- **S3** Tag panel ("1", answering the question about other ways to organize): next to the results, every tag with how many articles it has; selecting one filters the list. Needs `GET /api/tags`.
- **S4** Collections ("3"): group articles into named collections, separate from tags. Rules still to decide with the user (see Log).

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S3 | inline | done (93ed9bf) | `GET /api/tags`. Service tests RED (build failure: no Tags) then GREEN: counts per tag, most articles first then by name, tags without articles and other users' tags left out, empty library gives `[]`. Handler RED (404) then GREEN, internal errors hidden. Full Go suite with `-race` passes |
| T2 | S1, S2, S3 | inline | done (b7a8b51; visual review by the user pending) | No web test runner: tsc and lint pass. Smoke on `make dev`: home shows the search first and no add form; `/add` 200 with "Save a link" and "Import a bookmarks file"; `/import` 308 to `/add#bookmarks`; tag panel hidden while the library has no tags |
| T3 | S4 | inline | blocked: rules to decide | collections |

## Log

- L1 (2026-11-08, user): "vale ahora si, pero la pagina inicial deberia ser para buscar articulos, y me gustaria alguna otra forma de organizarlo, la parte de añadir un link,  o mediante html o boorkmars, deberia ser secundario"
- L2 (2026-11-08, user, choosing among: tag panel, reading status, collections, page only): "la 1 y la 3 serían las que me interesarían"
- L3: T1 and T2 start now; collection rules (one or many collections per article, nesting) are asked after them.
