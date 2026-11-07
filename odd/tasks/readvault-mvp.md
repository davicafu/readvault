# ReadVault MVP (milestone 1)

Objective: local, runnable ReadVault MVP as defined in `PRODUCT_SPEC.md` (sections 4–15). Branch: `feat/readvault-mvp`.
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main` (one branch per slice, each based on the previous one). Test runner: `go test ./...` (unit) and `go test -tags=integration ./...` (Postgres via Docker); frontend runner TBD (Node 22 at `~/.local/node/bin`).

## Specs

The source of truth is `PRODUCT_SPEC.md`. Each spec quotes the product spec verbatim and points to its section.

- **S1 (§12, §13)** Repo and local dev: "Configuración de Docker Compose.", "Archivo .env.example.", "Makefile con objetivos dev, test, migrate y lint.", "Fixtures HTML y CSV representativos.", "README con instrucciones de instalación y resolución de problemas."
- **S2 (RF-05)** URL normalization: "Normalizar las mayúsculas del host.", "Eliminar fragmentos de la identidad canónica.", "Eliminar únicamente parámetros de seguimiento reconocidos.", "Conservar parámetros que puedan afectar a la identidad o al contenido.", "Mantener la URL original para mostrarla y navegar."
- **S3 (RF-01)** HTML import: "Conservar las rutas de carpetas anidadas cuando existan.", "Tolerar títulos ausentes y registros malformados.", "Rechazar esquemas no admitidos, incluidos javascript: y file:.", "No ejecutar ni renderizar el HTML importado como contenido confiable."
- **S4 (RF-02)** CSV import: "Detectar nombres habituales de columnas cuando sea posible.", "Permitir asignación manual de columnas si la detección es ambigua.", "Admitir UTF-8, comillas y campos con saltos de línea.", "Informar de los registros inválidos sin descartar los válidos.", "Aplicar límites de tamaño de archivo y número de registros."
- **S5 (§7, RNF-01)** Schema: users, articles, tags, article_tags; "URL canónica única por usuario."; "Si el MVP utiliza un propietario fijo de desarrollo, debe representarse explícitamente."; "No crear tablas que no se utilicen".
- **S6 (RF-03, RF-04, §9)** Preview and import execution: "Reimportar el mismo archivo no debe generar registros duplicados.", "No sobrescribir un título editado por el usuario.", "No eliminar etiquetas existentes.", "Informar de registros insertados, omitidos, actualizados y fallidos."
- **S7 (§8)** HTTP API: the endpoints listed in §8; "Devolver estructuras de error consistentes.", "Admitir paginación limitada.", "No exponer trazas de ejecución ni detalles internos de la base de datos."
- **S8 (RF-06, RF-07)** Library: "Permitir búsquedas por título, etiquetas y URL.", "Ordenar por título y fecha de creación.", "Permitir editar títulos y etiquetas."
- **S9 (RF-09)** Export: "Exportar los artículos del usuario actual a JSON.", "Conservar URL originales y canónicas, títulos, etiquetas y metadatos."
- **S10 (§10)** Frontend screens: Library, Import, Article detail, visual design.

Decisions (L2–L4): screaming-by-domain packages with light hexagonal naming (`ports.go` per domain, `internal/adapter/{http,postgres}`); manual constructor DI; test seams 1–4 (see L4).

Assumptions:
- A1: `go.mod` lives in `services/api` with the module path `readvault/api`, because there is no remote yet. Rename it when a repository URL exists.
- A2: migrations live in `services/api/migrations` (instead of the root `migrations/`) so they can be embedded with `//go:embed`. This is the spec's "Adaptar la estructura".
- A3: `deploy/compose.yaml` is run with `docker-compose -f` (the local Compose is v1.27.4).
- A4: until authentication exists, a fixed development owner is seeded explicitly by migration.
- A5: no `articles.source` column (user decision L14, overriding §7).
- A6: schema additions not listed in §7, each needed by a quoted spec: `title_edited_at` (S6 "No sobrescribir un título editado por el usuario"), `bookmarked_at` (S9 metadata), `search` tsvector (§6 full-text search), `article_tags.user_id` with composite FKs (RNF-03 cross-user access). IDs are UUIDs.
- A7: the agent cannot write `.env` (deny rule); the user copies `.env.example`. Agent checks pass the variables to the process instead.

## Tasks

| ID | Specs | Route | Status | Evidence / commit |
|---|---|---|---|---|
| T1 | S1 | inline | done | RED: 3 health tests failed on empty mux; GREEN; smoke: /healthz 200, /readyz 200→503 after stopping db, 404 JSON, SIGINT graceful shutdown; commit f5e31f3 |
| T2 | S2 | inline | done | RED: tests failed against stub; GREEN: `go test ./internal/urlnorm/` ok; fuzz 20s clean; commit 197161f |
| T3 | S3 | inline | done | RED: 4 tests failed against stub; GREEN: `go test ./internal/importing/` ok; fuzz 20s clean; commit 0d77c0b |
| T4 | S4 | inline | done | RED: 4 tests failed against stub; GREEN: ok; fuzz 20s clean; `go vet` clean; commit 5b8737b |
| T5 | S5 | inline (Risk 1: schema, needs human review) | done (awaiting user SQL review) | RED: users relation missing; GREEN: up/down/up, dev owner, 6 invariants (`-tags=integration`); commit 27143a4 |
| T6 | S6 | inline | pending | |
| T7 | S7 | inline | pending | |
| T8 | S8 | inline | pending | |
| T9 | S9 | inline | pending | |
| T10 | S10 | inline | pending (Node 22 at ~/.local/node/bin) | |
| T11 | S1 | inline | pending | |

## Slices (stacked to main)

| PR | Branch | Commits | Tasks |
|---|---|---|---|
| 1 | `feat/readvault-01-scaffold` | f5e31f3 | T1 |
| 2 | `feat/readvault-02-urlnorm` | 197161f | T2 |
| 3 | `feat/readvault-03-html-import` | 0d77c0b | T3 |
| 4 | `feat/readvault-04-csv-import` | 5b8737b | T4 |
| 5 | `feat/readvault-05-schema` | 27143a4 | T5 |

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
- L13: RDD is on. Schema commit 27143a4 assessed `high` (item 1 escalation and a Makefile config change), so review was due. Native review was not started: the STATUS preflight asks for an intended-untracked selection (the user's untracked `.atl/`), and the CLI publishes no schema for that input. Slices 1-4 were not assessed, because an orphan root has no base ref. Recorded as unavailable proof. The human SQL review of `services/api/migrations/000001_init.up.sql` is pending.
- L14 (2026-11-07, user): "lo de source no hace falta, siempre será html, lo demas doy por bueno y puedes hacer commit". Removed `articles.source`; the rest of the schema is approved. The CSV import (RF-02, T4) is kept; the user was told it also exists. Amended the unpublished schema commit.
