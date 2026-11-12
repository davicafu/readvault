# ReadVault

ReadVault is a private library for the articles you save to read. Import your browser bookmarks, find articles by title, address or tag, edit titles and tags, and export everything as JSON.

It runs locally: a Go API, PostgreSQL in Docker and a Next.js web app. It needs no AI keys and no Medium credentials.

## Quick start

You need:

- Go 1.27 or later
- Docker with Compose (Docker Desktop on macOS; the `docker-compose` command)
- Node.js 22 or later, with npm

Compose runs PostgreSQL 17. To use your own server instead, it must be PostgreSQL 15 or later; set `DATABASE_URL` to point at it.

Then:

```bash
make dev                    # Postgres, migrations, API on :8080, web app on :3000
```

Open http://localhost:3000, choose **Add articles** and import a bookmarks file. To try it without your own export, import `fixtures/bookmarks.html`.

The first `make` run creates `.env` with a random database password; edit it to change ports or limits (`.env.example` lists every variable). `make dev` checks that Node.js 22 or later comes first on `PATH`, installs the web app's dependencies the first time and stops everything with Ctrl+C. Postgres keeps running in Docker; stop it with `make db-down`. The data stays in the `db-data` volume.

## Commands

| Command | What it does |
|---|---|
| `make dev` | Starts Postgres, applies migrations, runs the API and the web app |
| `make migrate` | Applies pending migrations |
| `make migrate-down` | Rolls back the last migration |
| `make test` | Runs the unit tests (no database needed) |
| `make test-integration` | Runs the integration tests against the Compose Postgres |
| `make lint` | Checks Go formatting, runs `go vet` (and golangci-lint when installed), typechecks the web app |
| `make build` | Builds `services/api/bin/api`, `services/api/bin/migrate` and the production web app |
| `make db-up` / `make db-down` | Starts or stops Postgres |
| `make web-install` | Reinstalls the web app's dependencies from the lockfile |

## Configuration

Everything is configured with environment variables. `make` reads them from `.env` (or the file named by `ENV_FILE`); `.env` is not committed.

| Variable | Used by | Default | Meaning |
|---|---|---|---|
| `POSTGRES_USER` | Compose | `readvault` | Database user |
| `POSTGRES_PASSWORD` | Compose | none, required | Database password |
| `POSTGRES_DB` | Compose | `readvault` | Database name |
| `POSTGRES_PORT` | Compose | `5432` | Port on 127.0.0.1 where Postgres is published |
| `DATABASE_URL` | API, migrate | none, required | For example `postgres://readvault:<password>@localhost:5432/readvault?sslmode=disable` |
| `HTTP_ADDR` | API | `:8080` | Address the API listens on |
| `LOG_LEVEL` | API | `info` | `debug`, `info`, `warn` or `error`; logs are JSON on stdout |
| `MAX_UPLOAD_BYTES` | API | `10485760` | Largest bookmarks file accepted (10 MiB) |
| `MAX_IMPORT_RECORDS` | API | `20000` | Most bookmarks accepted in one file |
| `MAX_ARTICLE_BYTES` | API | `5242880` | Largest article page or Markdown file uploaded or downloaded (5 MiB) |
| `ARTICLE_MIRROR_URL` | API | none | Reading mirror tried when an article page fails or is a paid preview, as a prefix put before the article's address, for example `https://freedium-mirror.cfd/`. Only those addresses are sent to it. Unset: no mirror |
| `API_ORIGIN` | Web app | `http://127.0.0.1:8080` | Where the web app's server reaches the API. Read at runtime. Set it when the API is not on port 8080 |
| `TEST_DATABASE_URL` | Integration tests | `DATABASE_URL` | A server where the tests may create and drop throwaway databases |

The defaults are for local development. In production, pass the variables from your platform's secret store instead of a `.env` file, use a dedicated database user and enable TLS on the database connection (`sslmode=require` or stronger). The API has no authentication yet, so it must not be exposed beyond your machine.

## How it fits together

```text
browser ──> apps/web (Next.js, :3000) ──/api/*──> services/api (Go, :8080) ──> Postgres (Docker)
```

- `services/api`: the Go API. Code is grouped by domain (`internal/importing`, `internal/library`, `internal/urlnorm`, `internal/tag`), with adapters in `internal/adapter/http` and `internal/adapter/postgres`. Migrations are in `services/api/migrations` and are embedded in the binaries.
- `apps/web`: the web app. Server components read the API directly. The browser goes through the `/api/*` route handler, so there is a single origin. Brand settings are in `apps/web/src/config/brand.ts`.
- `deploy/compose.yaml`: Postgres for local development.
- `fixtures/bookmarks.html`: a sample export with nested folders, tags, tracking parameters, a duplicate, Unicode titles and entries that cannot be imported.
- `docs/api.md`: every endpoint, its request and response, and the export format.

Until accounts exist, everything belongs to a fixed development owner created by the first migration.

## How imports behave

- The file is checked first. The preview shows what is new, what is already saved, what repeats inside the file and what cannot be imported, and nothing is written yet.
- The preview suggests a collection for each bookmark, its first folder below the browser's own folders (such as "Bookmarks bar"), and the tags saved in the file. Rename or drop a suggested collection, or change the collection and tags of a single bookmark, before importing. Missing collections are created; a name you already use reuses that collection.
- Uploaded files are never stored. Confirming the import uploads the file again, with your changes.
- Two bookmarks are the same article when their canonical URL matches: same address, ignoring the host's case, the `#fragment` and known tracking parameters such as `utm_*` and `fbclid`.
- Importing the same file again creates nothing new. Titles you edited are kept. Missing titles, folders, dates and collections are filled in; an article already in a collection stays there. Tags are only ever added, and never to an article whose tags you edited.
- Only `http` and `https` addresses are imported. `javascript:`, `file:` and others are reported and skipped.

## Organizing the library

The front page starts with the search box, then shows your collections as cards with their latest articles, the most recently added articles and your tags, so you can browse and discover what you saved. Open a collection to see its articles and move to the others from its header; **All articles** lists everything. Searching shows the results with **Collections** and **Tags** panels beside them. A collection groups articles; each article is in one collection at most and keeps its tags. Create, rename and delete collections on the **Collections** page, and pick an article's collection on its page. Deleting a collection keeps its articles. Adding articles (one link, or a bookmarks file) is on the **Add articles** page. A single link has its text downloaded right after it is saved; if the download fails, the link is still saved.

An article's page suggests a collection and up to five tags from its content: its title, the words of its address and its saved text. Only your own collections and tags are suggested. The collection comes from the most similar articles in your library, and the tags are ones named in the article or carried by similar articles. Click a suggestion to put it in the form; nothing changes until you save.

## Article text

Each article can keep its text as Markdown, with the images of its body, for reading later. On the article page, **Download text** fetches it from the article's address; **Upload page** takes an HTML page you saved from your browser, or a Markdown file. When a download fails (Medium refuses servers) or saves only a paid preview, the library and the article page show a warning so you can upload the page instead. Downloads only reach public addresses on ports 80 and 443. See `docs/api.md` for the details.

## Backup and restore

The database is the source of truth. Back it up with `pg_dump` from the Compose service:

```bash
docker-compose -f deploy/compose.yaml --env-file .env exec -T db \
  sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom' > readvault-$(date +%Y%m%d).dump
```

Restore it into an empty database. This replaces what is there, so take a backup first:

```bash
make db-up
docker-compose -f deploy/compose.yaml --env-file .env exec -T db \
  sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner' < readvault-20261108.dump
make migrate   # applies any migration newer than the backup
```

Check the restore by opening the library, or with `curl http://localhost:8080/readyz`.

The JSON export (**Export JSON** in the web app, or `GET /api/export`) is a readable copy of your articles, tags and metadata for keeping or moving elsewhere. It cannot be imported back; use the database backup to restore.

## Troubleshooting

| Problem | Fix |
|---|---|
| `set POSTGRES_PASSWORD in .env` | Add `POSTGRES_PASSWORD` to `.env`, or delete `.env` so `make` creates a new one. Compose refuses to start Postgres without a password |
| `Node.js 22 or later is required` | An older `node` comes first on `PATH`. Put Node 22 first, for example `export PATH="$HOME/.local/node/bin:$PATH"` in your shell profile |
| `make db-up` waits forever | Start Docker Desktop. Then check `docker-compose -f deploy/compose.yaml --env-file .env logs db` |
| `port is already allocated` on 5432 | Another Postgres is running. Set `POSTGRES_PORT=55432` in `.env` and use the same port in `DATABASE_URL` |
| `address already in use` on 8080 | Set `HTTP_ADDR=:8081` and `API_ORIGIN=http://127.0.0.1:8081` in `.env` |
| The library says the API is not reachable | The API is not running or `API_ORIGIN` points elsewhere. Check `curl http://localhost:8080/readyz` |
| `/readyz` answers `unavailable` | The API cannot reach Postgres. Check `DATABASE_URL` and `make db-up` |
| `password authentication failed` | The volume was created with another password. Use that password, or remove the volume with `docker-compose -f deploy/compose.yaml --env-file .env down -v`. This deletes all data |
| `npm: command not found` | Install Node.js 22 or later and make sure it is on `PATH` |
| The import says the file is too large | Raise `MAX_UPLOAD_BYTES`, or export a smaller bookmarks folder |
| Integration tests fail with `TEST_DATABASE_URL (or DATABASE_URL) must be set` | Run them with `make test-integration`, which reads `.env`, or set `TEST_DATABASE_URL` to a database whose user may create databases |
