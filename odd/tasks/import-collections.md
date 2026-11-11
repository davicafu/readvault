# Import with collections

Objective: a bookmarks import suggests a collection and tags for each bookmark, the person adjusts them in the preview, and the import creates the collections and assigns them. Branch: `feat/readvault-17-import-collections` (stacked on `feat/readvault-16-library-organization`).
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -tags=integration ./...`; web has no test runner (tsc, smoke through the proxy).

## Specs

- **S1** Suggestions (L1): "seria interesante que se integrase ya con las colecciones o al menos como las etiquetas, que sean sugeridas y tu decidas si quitar, añadir mas o lo que sea".
  - Each bookmark gets a suggested collection: its first folder that is not a browser root folder (a heading marked `PERSONAL_TOOLBAR_FOLDER` or `UNFILED_BOOKMARKS_FOLDER`, such as "Bookmarks bar"). No such folder means no suggestion.
  - The suggested collection name follows the collection name rules; a name that breaks them gives a warning and no suggestion.
  - Tags are suggested from the file's `TAGS` attribute, as today.
  - The preview lists the suggested collections with how many bookmarks each gets and whether the person already has a collection with that name.
- **S2** The person decides (L1): in the preview the person can rename or drop a suggested collection for all its bookmarks, and change the collection and the tags of a single bookmark (another collection, none, add or remove tags).
- **S3** Import applies the decisions:
  - Missing collections are created; names that match an existing collection (same slug) reuse it.
  - New articles get their collection and tags.
  - Existing articles get a collection only when they have none (same rule as folder and bookmark date) and gain tags under the existing tag rules.
  - Choices that break the name rules reject the whole import with 422 and change nothing.
  - Without choices, the import behaves as the suggestions say. Importing the same file twice stays idempotent.
- **S4** Then import the user's cleaned bookmarks (`bookmarks_11_11_26.collections.html`) into the local ReadVault through this flow (user's "si" to importing).

Assumptions: choices travel in a multipart field `choices` sent before `file` (the server streams the file and keeps nothing between preview and import). Per-bookmark edits address bookmarks by their position in the file.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S1, S3 | inline | done (99f9712) | Parser test RED (build failure: no Record.Collection) then GREEN. Service integration tests RED (build failure) then GREEN: suggestion per bookmark below root folders, collections listed with counts and `existing`, `collection_too_long` warning, collections created and existing ones reused (name kept), idempotent re-import, choices (rename, drop, per-bookmark collection, none, tags replaced), existing article gains a collection only without one, invalid choices (too long, blank, tag too long, unknown position, unknown suggestion) rejected with nothing written. Handler tests RED then GREEN: choices passed through, malformed/oversized JSON 400 `invalid_choices` without calling the service, ChoiceError 422 with `details.field`. Full Go suite with `-race` passes. Native assess (3dcacd6..99f9712, untracked bookmark files excluded): medium, `under_budget` |
| T2 | S2 | inline | done (see Log L4) | Server fix found while writing the UI: candidates now carry their group's name (folders sharing a slug); test RED (`go` vs `Go`) then GREEN, after a second RED caused by pointers into the sorted slice. tsc passes (no lint script exists). Smoke on `make dev` through the proxy: preview of the user's file lists 9 collections and every candidate's collection; `/add` 200. The editing controls were not clicked in a browser: visual review by the user pending |
| T3 | S4 | inline | done (see Log L5) | Dev DB backed up first (pg_dump, scratchpad). Import through the proxy with empty choices: inserted 519, updated 3, failed 0; 9 collections with 130/84/78/64/51/39/39/23/14 articles; 2 pre-existing articles not in the file keep no collection; 39 tags. Re-preview: 522 existing, every collection existing. `/`, `/?collection=backend`, `/?tag=iceberg`, `/collections` 200 |

## Log

- L1 (2026-11-11, user, answering whether to import the classified bookmarks and assign collections): "si, serian interesante que se integrase ya con las colecciones o al menos como las etiquetas, que sean sugeridas y tu decidas si quitar, añadir mas o lo que sea"
- L2: Design decided by the parent (the user delegated it): suggestion from the first non-root folder, edits in the preview per suggested collection and per bookmark, collections filled only when missing on existing articles. Import of the user's file waits for the flow so it is tried for real.
- L3: T1 committed. A first commit accidentally carried the user's staged PRODUCT_SPEC.md (git commit takes the whole index); it was redone at once with `git commit -- <paths>` before anything was pushed, leaving PRODUCT_SPEC.md staged as before.
- L4: Preview editing: Collections panel (use checkbox, name, bookmark count, new or already yours), collection and tags inputs per importable bookmark with a datalist of existing and suggested names, "Show more" past 200 rows, client checks for blank or over-long names before enabling Import; choices are sent before the file, only with what changed.
- L5: The user's cleaned bookmarks (`bookmarks_11_11_26.collections.html`, 522 links) imported into the local ReadVault through the new flow.
