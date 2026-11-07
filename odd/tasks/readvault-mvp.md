# ReadVault MVP (milestone 1)

Objective: local, runnable ReadVault MVP as defined in `PRODUCT_SPEC.md` (sections 4–15). Branch: `feat/readvault-mvp`.
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main` (one branch per slice, each based on the previous one). Test runner: `go test ./...` (unit) and `go test -tags=integration ./...` (Postgres via Docker); frontend runner TBD (Node 22 at `~/.local/node/bin`).

## Specs

The source of truth is `PRODUCT_SPEC.md`. Each spec quotes the product spec verbatim and points to its section.

- **S1 (§12, §13)** Repo and local dev: "Configuración de Docker Compose.", "Archivo .env.example.", "Makefile con objetivos dev, test, migrate y lint.", "Fixtures HTML y CSV representativos.", "README con instrucciones de instalación y resolución de problemas."
- **S2 (RF-05)** URL normalization: "Normalizar las mayúsculas del host.", "Eliminar fragmentos de la identidad canónica.", "Eliminar únicamente parámetros de seguimiento reconocidos.", "Conservar parámetros que puedan afectar a la identidad o al contenido.", "Mantener la URL original para mostrarla y navegar."
- **S3 (RF-01)** HTML import: "Conservar las rutas de carpetas anidadas cuando existan.", "Tolerar títulos ausentes y registros malformados.", "Rechazar esquemas no admitidos, incluidos javascript: y file:.", "No ejecutar ni renderizar el HTML importado como contenido confiable."
- **S4 (RF-02)** CSV import: removed by the user (L16).
- **S5 (§7, RNF-01)** Schema: users, articles, tags, article_tags; "URL canónica única por usuario."; "Si el MVP utiliza un propietario fijo de desarrollo, debe representarse explícitamente."; "No crear tablas que no se utilicen".
- **S6 (RF-03, RF-04, §9)** Preview and import execution: "Reimportar el mismo archivo no debe generar registros duplicados.", "No sobrescribir un título editado por el usuario.", "No eliminar etiquetas existentes.", "Informar de registros insertados, omitidos, actualizados y fallidos."
- **S7 (§8)** HTTP API: the endpoints listed in §8; "Devolver estructuras de error consistentes.", "Admitir paginación limitada.", "No exponer trazas de ejecución ni detalles internos de la base de datos."
- **S8 (RF-06, RF-07)** Library: "Permitir búsquedas por título, etiquetas y URL.", "Ordenar por título y fecha de creación.", "Permitir editar títulos y etiquetas."
- **S9 (RF-09)** Export: "Exportar los artículos del usuario actual a JSON.", "Conservar URL originales y canónicas, títulos, etiquetas y metadatos."
- **S10 (§10)** Frontend screens: Library, Import, Article detail, visual design.
- **S11 (spec change, L19-L21)** Article content: store each article's full Markdown, fetched through a link the user will provide. User: "como tenemos la URL vamos a guardarla para quitar las imágenes de ahí y no almacenarlas, siempre las podríamos generar después". Images are stripped and not stored; the article URL is kept so content and images can be fetched again later. The authorization clause ("contenido cuyo procesamiento esté autorizado") is waived by the user for private use (L21). Scheduled after T11 (assumption: the user did not pick MVP vs. after; the recommended option was taken). Fetch link pending from the user.

Decisions (L2–L4): screaming-by-domain packages with light hexagonal naming (`ports.go` per domain, `internal/adapter/{http,postgres}`); manual constructor DI; test seams 1–4 (see L4).

Assumptions:
- A1: `go.mod` lives in `services/api` with the module path `readvault/api`, because there is no remote yet. Rename it when a repository URL exists.
- A2: migrations live in `services/api/migrations` (instead of the root `migrations/`) so they can be embedded with `//go:embed`. This is the spec's "Adaptar la estructura".
- A3: `deploy/compose.yaml` is run with `docker-compose -f` (the local Compose is v1.27.4).
- A4: until authentication exists, a fixed development owner is seeded explicitly by migration.
- A5: no `articles.source` column (user decision L14, overriding §7).
- A6: schema additions not listed in §7, each needed by a quoted spec: `title_edited_at` (S6 "No sobrescribir un título editado por el usuario"), `bookmarked_at` (S9 metadata), `search` tsvector (§6 full-text search), `article_tags.user_id` with composite FKs (RNF-03 cross-user access). IDs are UUIDs.
- A8: the import is stateless. `POST /api/imports` re-uploads the same file confirmed in the preview, so nothing uploaded is stored (RF-04 "No conservar indefinidamente los archivos subidos"). `GET /api/imports/{id}` is dropped (§8 "Adaptar los endpoints"), because `import_jobs` is a future table. The import id is returned in the report and logged.
- A9: a title is completed only when it is empty and was never edited. Folder and bookmark date are completed only when absent. Tags are only added. The original URL of the first import is kept.
- A7: the agent cannot write `.env` (deny rule); the user copies `.env.example`. Agent checks pass the variables to the process instead.

## Tasks

| ID | Specs | Route | Status | Evidence / commit |
|---|---|---|---|---|
| T1 | S1 | inline | done | RED: 3 health tests failed on empty mux; GREEN; smoke: /healthz 200, /readyz 200→503 after stopping db, 404 JSON, SIGINT graceful shutdown; commit f5e31f3 |
| T2 | S2 | inline | done | RED: tests failed against stub; GREEN: `go test ./internal/urlnorm/` ok; fuzz 20s clean; commit 197161f |
| T3 | S3 | inline | done | RED: 4 tests failed against stub; GREEN: `go test ./internal/importing/` ok; fuzz 20s clean; commit 0d77c0b |
| T4 | S4 | — | dropped (L16) | commit 5b8737b removed from the stack |
| T5 | S5 | inline (Risk 1: schema, needs human review) | done (schema approved by user, L14) | RED: users relation missing; GREEN: up/down/up, dev owner, 6 invariants (`-tags=integration`); commit 8431f13 |
| T6 | S6 | inline | done | RED: 4 service tests failed on stubs; GREEN: preview classification, idempotent re-import, edited title kept, missing title filled, tags only added, record limit, 4 concurrent imports with no duplicates (`-count=3`); branch feat/readvault-05-import-service |
| T7 | S7 (import endpoints) | inline | done | RED: 2 handler tests failed with 404 before the routes existed; GREEN: owner from server not client, file at limit ok, over limit 413 `file_too_large`, non-multipart 400 `invalid_upload`, no file 400 `missing_file`, too many records 422, internal 500 without SQL details; integration preview→import→re-import→preview via HTTP; curl smoke on the real binary; branch feat/readvault-06-import-http |
| T8 | S8 | inline | pending | |
| T9 | S9 | inline | pending | |
| T10 | S10 | inline | pending (Node 22 at ~/.local/node/bin) | |
| T11 | S1 | inline | pending | |
| T12 | S11 | inline (Risk 2: server-side fetch, SSRF allowlist) | pending, after T11; needs the fetch link | |

## Slices (stacked to main)

| PR | Branch | Commits | Tasks |
|---|---|---|---|
| 1 | `feat/readvault-01-scaffold` | f5e31f3 | T1 |
| 2 | `feat/readvault-02-urlnorm` | 197161f | T2 |
| 3 | `feat/readvault-03-html-import` | 0d77c0b | T3 |
| 4 | `feat/readvault-04-schema` | 8431f13 | T5 |
| 5 | `feat/readvault-05-import-service` | 446ed5f | T6 |
| 6 | `feat/readvault-06-import-http` | (this commit) | T7 |

`main` has no commits yet. Before opening PR 1, `main` needs a base commit (for example, the product spec); that is the user's decision.

## Log

- L1 (2026-11-07, user): "revisa el product_spec.md y mira las skills que necesitas y dime las que vas a cargar" / "vamos a dejarlo un poco mas, mientras puedes ir empezando con el desaroollo de la app? si necesitas un par de enlaces de ejemplos avisame"
- L2 (2026-11-07, user): "me gusta la opcion de screaming mas hexagonal ligera pero metele los nombres de la hexagonal donde toque"
- L3 (2026-11-07, user): DI = "Constructores manuales".
- L4 (2026-11-07, user): approved test seams: (1) `urlnorm.Canonicalize`, pure table tests; (2) HTML/CSV parsers, pure, with fixtures and fuzz; (3) importing/library services plus the postgres adapter, integration tests with `-tags=integration`; (4) the http adapter via httptest. No in-memory fake repository.
- L5: environment: Go 1.27.3; Docker 20.10.2 with the daemon not running; docker-compose 1.27.4; no golangci-lint; Node v12.13.0 (`brew install node` is building LLVM from source).
- L7: urlnorm keeps path case, trailing slashes and unknown query params; it removes `utm_*` and known click IDs (fbclid, gclid, dclid, msclkid, mc_cid, mc_eid, igshid, _hsenc, _hsmi). Medium's `source` param is NOT removed (conservative). The CSV folder column maps to a single-level folder path. The CSV parser uses LazyQuotes to tolerate stray quotes.
- L8: next is T1 (compose, Makefile, main) and T5 (schema). Both need the Docker daemon, which is not running.
- L6: forecast is well over 400 authored lines, so a chain strategy is needed before the first commit over budget.
- L9 (2026-11-07): `brew install node` failed compiling Node on macOS 12 (Tier 3). Installed the official Node v22.23.4 x64 binary (SHA-256 verified) at `~/.local/node`. Adding it to PATH in `~/.zprofile` was blocked for the agent, so the user must do it. Frontend skills installed: frontend-design, vercel-react-best-practices, web-design-guidelines, vercel-composition-patterns.
- L10 (2026-11-07, user): "no, comproueba primero orbstack y si no tiramos con docker desktop". OrbStack needs macOS 14.0+ (docs.orbstack.dev/faq), so we use Docker Desktop; started it.
- L11: T1 and T5 done. Migrations use golang-migrate v4.20.1 (iofs + pgx5 driver); pgx v5.12.0. Integration tests create a throwaway database per test from TEST_DATABASE_URL. Authored lines so far: about 2,000, over budget, so the chain strategy is asked before the first commit. Next: T6 (import service).
- L12 (2026-11-07, user): chain strategy = "Stacked a main". Each slice was built and verified alone in a worktree (`go mod tidy`, gofmt, vet, `go test -race`).
- L13: RDD is on. Schema commit 8431f13 assessed `high` (item 1 escalation and a Makefile config change), so review was due. Native review was not started: the STATUS preflight asks for an intended-untracked selection (the user's untracked `.atl/`), and the CLI publishes no schema for that input. Slices 1-4 were not assessed, because an orphan root has no base ref. Recorded as unavailable proof. The human SQL review of `services/api/migrations/000001_init.up.sql` is pending.
- L14 (2026-11-07, user): "lo de source no hace falta, siempre será html, lo demas doy por bueno y puedes hacer commit". Removed `articles.source`; the rest of the schema is approved. The CSV import (RF-02, T4) is kept; the user was told it also exists. Amended the unpublished schema commit.
- L15: an amend accidentally included the user's staged `PRODUCT_SPEC.md`. It was removed from the commit and its staged blob (9fba4bc) restored. The schema commit is now 8431f13. Next: T6.
- L16 (2026-11-07, user): "si ,quita la importacion csv porque no tengo fuentes con ese tipo. dale a t6, si necesitas enlaces de prueba avisame". Dropped the CSV slice from the unpublished stack (the schema commit was replayed onto the HTML slice as 8431f13; the branch was renamed to feat/readvault-04-schema). S4 is out of scope. CSV column mapping in the import screen (§10) is out of scope too.
- L17: T6 done. One test input was wrong (`https://go.dev/blog` without a trailing slash is a different canonical URL by design, L7); the test was fixed, not the code. Saves run in transactions of 500 articles with pgx batches. A failing batch is reported as `storage_error` while the other batches are kept.
- L18: T6 commit 446ed5f (972 changed lines; a cohesive unit, since the service tests need the adapter at seam 3, so it is over the 400 budget: recommend `size:exception` for PR 5). gentle-ai assess: medium, review not due (`under_budget`). Risk item 4 (row locks and concurrent imports): an independent verifier is planned at the feature end, covering all inline deliveries. Next: T7 HTTP import endpoints.
- L19 (2026-11-08, user): "con respecto a la regla de no almancenar archivos subidos, aunque sea el markdown, cuanto ocuparia una biblioteca con 100 articulos de markdown estandar de meddium?" Estimated about 1-2 MB of text for 100 articles (images would add 30-300 MB). Rule §6 "No conservar indefinidamente los archivos subidos" is about the uploaded import file, not article content.
- L20 (2026-11-08, user): "para el muro de pago te pasara un enlace con el que podras sacar el md entero, por eso igual cunde guardarlo y si tal se cambia la spec"
- L21 (2026-11-08, user): "Una vez se tuviera el acceso el repo será de forma privada así que vamos a saltarnos eso, como tenemos la URL vamos a guardarla para quitar las imágenes de ahí y no almacenarlas, siempre las podríamos generar después". Added S11 and T12, after T11. Next: T7.
- L22: T7 done. The upload is streamed from the multipart reader (no `ParseMultipartForm`, so nothing is written to a temp file), capped at MAX_UPLOAD_BYTES plus 64 KiB of multipart overhead. Upload read errors are told apart from service errors by the reader wrapper, not by string matching. The rest of S7 (library, export endpoints) lands with T8 and T9. Next: T8.
