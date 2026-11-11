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
| T1 | S1, S3 | inline | pending | API: parser, preview suggestions, choices, repository |
| T2 | S2 | inline | pending | Web preview editing |
| T3 | S4 | inline | pending | Import the user's file |

## Log

- L1 (2026-11-11, user, answering whether to import the classified bookmarks and assign collections): "si, serian interesante que se integrase ya con las colecciones o al menos como las etiquetas, que sean sugeridas y tu decidas si quitar, añadir mas o lo que sea"
- L2: Design decided by the parent (the user delegated it): suggestion from the first non-root folder, edits in the preview per suggested collection and per bookmark, collections filled only when missing on existing articles. Import of the user's file waits for the flow so it is tried for real.
