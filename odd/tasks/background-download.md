# Background download

Objective: download the text of every article added by a single link or a bookmarks import, in the background, falling back to a reading mirror (such as Freedium) when the original page fails or is a paid preview. Branch: `feat/readvault-19-background-download` (stacked on `feat/readvault-18-content-suggestions`).
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -tags=integration ./...`; web has no test runner (tsc, smoke through the proxy).

## Specs

- **S1** Mirror fallback (L1): "a ver entonces de si eres capaz de sacar el html de este link https://freedium-mirror.cfd/https://blog.stackademic.com/how-would-you-design-a-webhook-system-for-multiple-events-2aa03481b742" / "quiero que implementes eso que acabas de hacer en readvault".
  - A download first tries the original URL. When the site answers without the article (an HTTP error such as 403, or a page without text) or with a paid preview, and a mirror is configured, it downloads the mirror address (the mirror prefix followed by the original URL) the same way. Addresses that did not resolve or answer, or that the fetcher refuses, are never sent (they may be intranet hosts).
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
- L5: Independent verifier (after T1-T3): S1, S2, S3 pass; blocker B1: a single link is downloaded twice, by the web app's immediate download and by a worker that claims the queued article meanwhile (reproduced: 2 page requests). Advisories: blocked addresses sent to the mirror, a mirror prefix without `/` runs into the host, no committed test of a download cut short, the mirror asked after cancellation.
- L6: Correction (one batch): the queue hands out an article only after it has waited a minute (`NewDownloadQueue(pool, time.Minute)`); a direct download is bounded by its timeouts (about 35 s) and always leaves text or a recorded failure, so the worker skips it. Test RED (2 requests with no wait, confirmed) then GREEN (1 request). Also: blocked addresses never go to the mirror, the mirror is not asked once cancelled, a prefix without a path gets its `/`, and a committed test shows a download cut short stays queued, records nothing and is retried. Full Go suite with `-race` passes.
- L7: Native review of feature 19 (f137462..28ce077, lenses risk, resilience, reliability; the first attempt over 3dcacd6..HEAD stopped with `lens_context_budget_exceeded`, so features 17-18 stay unreviewed): approved and acknowledged (authority burned). Advisories: R1-001 an unresolvable intranet host failed as `unreachable` would go to the mirror, contradicting the README; R1-002 one import can queue thousands of outbound downloads (cap per user once accounts exist); R3/R4 the one-minute delay may not cover original plus mirror downloads, refuted by the timeouts (page 10 s + mirror page 10 s + images 15 s, images downloaded once); R3 no worker test with a mirror.
- L8: R1-001 fixed after the review: only `http_status` and `no_text` failures or a paid preview go to the mirror. Test RED (unreachable, timeout, not_html, too_large sent to the mirror) then GREEN.
