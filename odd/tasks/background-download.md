# Background download

Objective: download the text of every article added by a single link or a bookmarks import, in the background, falling back to a reading mirror (such as Freedium) when the original page fails or is a paid preview. Branch: `feat/readvault-19-background-download` (stacked on `feat/readvault-18-content-suggestions`).
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -tags=integration ./...`; web has no test runner (tsc, smoke through the proxy).

## Specs

- **S1** Mirror fallback (L1): "a ver entonces de si eres capaz de sacar el html de este link https://freedium-mirror.cfd/https://blog.stackademic.com/how-would-you-design-a-webhook-system-for-multiple-events-2aa03481b742" / "quiero que implementes eso que acabas de hacer en readvault".
  - A download first tries the original URL. When it fails, or the page is a paid preview, and a mirror is configured, it downloads the mirror address (the mirror prefix followed by the original URL) the same way.
  - A mirror page with text that is not a paid preview is saved as `complete`; otherwise the original outcome stands (the paid preview saved as `incomplete`, or the original failure recorded).
  - The mirror is configured with `ARTICLE_MIRROR_URL` (for example `https://freedium-mirror.cfd/`); unset means no mirror. Only addresses that failed or were paid previews are sent to it.
- **S2** Download on add and import (L1, L2): "cuando le doy a incorporar desde un link o cuando se encuentra el link en el importador masivo desde marcadores"; the user chose a server-side queue: "Cola en el servidor".
  - Every article of an import (single link or bookmarks file, new or already in the library) that has no saved text and no recorded download is queued in the same transaction as the import.
  - A worker in the API downloads queued articles in the background, a bounded number at a time with a pause between downloads, and keeps working after the browser is closed. A queued article that already has text when its turn comes is skipped.
  - A download that is interrupted (API restart) is retried; an article is tried at most 3 times. A download failure is recorded on the article as today and not retried automatically.
  - `DOWNLOAD_WORKERS` (default 2, `0` turns the worker off).
- **S3** The import result says that the text is downloaded in the background. Docs (API, README, configuration) describe the mirror and the queue.

Assumptions: the queue lives in PostgreSQL (one table) and the worker in the API process; no new service or dependency.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S1 | inline | done (first commit) | Service tests RED (build failure: no WithMirror) then GREEN: mirror used when the original fails and for a paid preview; original outcome kept when the mirror fails or is a paid preview too; mirror not asked when the original works or none is configured. Guard test RED (two requests) then GREEN: an address already on the mirror is not sent again. Config test RED then GREEN: unset, set, invalid values rejected. Real page: the user's Freedium article downloaded through ReadVault gives 608 words, complete, cover and images kept, Freedium chrome left out |
| T2 | S2 | inline | done (second commit) | Risk items 1 (migration) and 4 (concurrent worker). Import tests RED (nothing queued) then GREEN: new article without text queued, article with text or a recorded download not queued, re-import queues nothing twice, existing article without text queued. Worker and queue tests RED (build failure) then GREEN: queued articles downloaded with 2 workers and the queue emptied; an article with a recorded download skipped without a request; a failed download recorded and not retried; a held claim not handed out twice; a claim that ran out handed out again, 3 attempts at most, then dropped; Done removes. Config test RED then GREEN. Full Go suite with `-race` passes; migration up/down/up test passes |
| T3 | S3 | inline | done (third commit) | Import report says texts download in the background; README features and configuration, API docs updated in T1/T2. tsc passes |

## Log

- L1 (2026-11-12, user): "a ver entonces de si eres capaz de sacar el html de este link https://freedium-mirror.cfd/https://blog.stackademic.com/how-would-you-design-a-webhook-system-for-multiple-events-2aa03481b742" then "vale pues quiero que implementes eso que acabas de hacer en readvault cuando le doy a incorporar desde un link o cuando se encuentra el link en el importador masivo desde marcadores"
- L2 (2026-11-12, user, choosing how a bulk import downloads its texts): "Cola en el servidor (Recommended)"
- L3: Evidence: the original URL answers 403 to a plain client (Medium blocks it); the Freedium page answered 200 with the whole article (about 700 words, no "Member-only story" marker, no `isAccessibleForFree`).
- L4: The user's library stores this article under its Freedium address, so an address that already starts with the mirror prefix is never sent to the mirror again.
