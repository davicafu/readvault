# Content suggestions

Objective: suggest a collection and tags for a single article from its content, and offer them on the article page. Branch: `feat/readvault-18-content-suggestions` (stacked on `feat/readvault-17-import-collections`).
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -tags=integration ./...`; web has no test runner (tsc, smoke through the proxy).

## Specs

- **S1** Suggestions from content (L1, L2): "La Sugerencia de colección y etiquetas funciona también cuando solo cargo un enlace no?" / "quiero que sea a partir del contenido si".
  - `GET /api/articles/{id}/suggestions` returns a suggested collection (or none) and suggested tags for one article.
  - The content is the article's title, the words of its URL and its saved text when there is one.
  - Suggestions use only the user's own collections and tags; nothing new is invented.
  - Collection: the collection most common among the library articles most similar to this one (by title and tags, rare words weigh more); none when no collection stands out.
  - Tags: existing tags named in the title, the URL or several times in the text, and tags shared by several similar articles; at most 5, never one the article already has.
- **S2** Offer them (L2): the article page shows the suggested collection and tags; one click puts each into the editor, and saving stays the person's decision.
- **S3** Adding one link downloads its text right away, so its suggestions come from the content; a failed download still saves the link and the suggestions use the title and URL.

Assumptions: the library compared against is bounded to the 10,000 most recently added articles; the text read is bounded to its first 200,000 characters.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S1 | inline | done (f958754) | Unit tests of `Suggest` RED (build failure) then GREEN: collection from the most similar articles, rare words weigh more, none when nothing or only one article is similar or two collections tie, URL and text words, tags named in the content, short tags only from the title, tags shared by similar articles, never an existing tag or more than 5, link addresses ignored, the article is not its own neighbour. Integration tests GREEN: saved text and only the user's own library; 404 for another user's, unknown and malformed ids. Handler tests RED (404 route not found) then GREEN. Full Go suite with `-race` passes |
| T2 | S2, S3 | inline | done (see Log L4) | tsc passes (no lint or test runner). Smoke on `make dev` (restarted) through the proxy: suggestions for 5 Kafka articles all give their own collection "Streaming y Kafka"; the article page renders 200 with "Add suggested tag" buttons and hides the collection suggestion when the article is already in it; `/add` 200. Not clicked in a browser, and the add-link download was not run end to end to keep the user's library unchanged: visual review by the user pending |

## Log

- L1 (2026-11-11, user): "La Sugerencia de colección y etiquetas funciona también cuando solo cargo un enlace no?"
- L2 (2026-11-11, user, choosing between library-based and content-based suggestions): "quiero que sea a partir del contenido si"
- L3: Design decided by the parent: only the user's collections and tags are suggested; the collection comes from the most similar library articles, since only 2 of 524 articles have saved text, so the library side is titles and tags. Adding a link downloads its text so there is content to read.
- L4: The article editor shows suggested tags ("+ name") and the suggested collection under their fields; a click puts them in the form and saving stays the person's. Suggestions load with the page and are left out when they fail. Adding a link downloads its text only when the link is new (`inserted > 0`), because a download replaces saved text; a failed download is ignored and the article page still offers it. Observed on the dev library: tags shared by similar articles are sometimes loose (for example `machine-learning` for a Kafka Streams/KSQL article, `spring` for a Kafka transactions one).
