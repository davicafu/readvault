# ReadVault

ReadVault is a private library for the articles you save to read. Import your browser bookmarks, keep each article's text to read later, organise articles in collections and tags, find them again, and export everything as JSON.

It runs locally: a Go API, PostgreSQL in Docker and a Next.js web app. It needs no AI keys and no Medium credentials.

## Features

- **Import bookmarks** from a browser export (Chrome, Firefox, Safari, Edge) with a preview first, or add a single link. Collections and tags are suggested from the bookmark folders and can be changed before importing. Importing the same file again adds nothing twice.
- **Saved text**: each article's text is downloaded as Markdown with its images, in the background after an import or one by one. Medium articles that Medium refuses are read from a mirror. When a download fails, upload the page saved from your browser (HTML) or a Markdown file instead.
- **Reading**: an article page shows the saved text with a table of contents and a reading progress bar.
- **Collections**: nine to start with, which you can rename, delete or add to. Each collection gets an icon and a colour from its name. An article is in one collection at most.
- **Tags**: any number per article, shown with the logo of the technology they name or an icon for their subject.
- **Suggestions**: an article page suggests a collection and tags from its content, among your own.
- **Search** from the header on every page (press `/`): by title, address or tag, `:name` to search one collection, `-word` to leave a word out, `"exact words"` for a phrase. Results can be filtered by collection, tag and saved text, and ordered.
- **Front page**: your collections as folders with their latest articles, the articles added last and your tags.
- **Clean up**: delete one article for good, or free space by deleting the saved texts of every article or of one collection (the articles stay).
- **Backup and restore**: export everything as JSON and import that file to bring back lost articles and collections; back up the whole database, saved texts included, with `pg_dump`.

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
| `ARTICLE_MIRROR_URL` | API | `https://freedium-mirror.cfd/` | Reading mirror for Medium articles, tried when Medium answers with an HTTP error (such as 403), a page without text or a paid preview, as a prefix put before the article's address (a missing final `/` after the host is added). Only Medium articles go to it: medium.com, its subdomains and publications on their own domain that Medium serves (recognised by Medium's addresses in DNS). `off` turns it off |
| `DOWNLOAD_WORKERS` | API | `2` | How many article texts are downloaded at once in the background after an import (0 to 16); `0` turns background downloads off |
| `API_ORIGIN` | Web app | `http://127.0.0.1:8080` | Where the web app's server reaches the API. Read at runtime. Set it when the API is not on port 8080 |
| `TEST_DATABASE_URL` | Integration tests | `DATABASE_URL` | A server where the tests may create and drop throwaway databases |

The defaults are for local development. In production, pass the variables from your platform's secret store instead of a `.env` file, use a dedicated database user and enable TLS on the database connection (`sslmode=require` or stronger). The API has no authentication yet, so it must not be exposed beyond your machine.

## How it fits together

```text
browser ──> apps/web (Next.js, :3000) ──/api/*──> services/api (Go, :8080) ──> Postgres (Docker)
```

- `services/api`: the Go API. Code is grouped by domain (`internal/importing`, `internal/library`, `internal/content` for saved texts and downloads, `internal/urlnorm`, `internal/tag`), with adapters in `internal/adapter/http`, `internal/adapter/postgres` and `internal/adapter/fetch` (article downloads). Migrations are in `services/api/migrations` and are embedded in the binaries.
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

- **Front page**: the ReadVault name in the header goes back to it. It shows your collections as folders with their latest articles, the most recently added articles and your tags. The folders gather in from a veil of sparks as the page opens, and entries fade in as you scroll (both are off when your system asks for reduced motion).
- **Library**: **Library** in the header lists every article. Opening a collection lists its articles, with the other collections in its header. Search results can be ordered and narrowed with the **Collections** and **Tags** panels beside them.
- **Search**: the search box is in the page header on every page; press `/` to jump to it. On a collection it searches that collection. Type `:` and a collection's name to search in it (pick it from the list with the arrow keys and Enter), `-word` to leave out articles with a word, and `"exact words"` to match a phrase.
- **Collections**: a collection groups articles; each article is in one collection at most and keeps its tags. The library starts with nine collections: AI and tools, Architecture and system design, Backend, Data engineering, Databases and SQL, DevOps, cloud and Linux, Frontend and visualization, Machine learning and analytics, Streaming and Kafka. Create, rename and delete collections on the **Collections** page, and pick an article's collection on its page. Deleting a collection keeps its articles.
- **Icons and colours**: each collection gets an icon and a colour picked from the words of its name. Each tag shows the logo of the technology it names (from [Simple Icons](https://simpleicons.org), CC0 data; the marks belong to their owners), or else an icon for its subject.
- **Deleting an article**: **Delete article** on its page deletes it for good, with its tags and saved text, after a confirmation.
- **Adding articles**: one link, or a bookmarks file, on the **Add articles** page. A single link has its text downloaded right after it is saved. The articles of a bookmarks file that have no text yet are downloaded in the background, a few at a time, and keep downloading if you close the page. If a download fails, the link is still saved and its page offers to try again. A Medium article that Medium refuses (such as a 403) or shows only as a paid preview is read from Freedium instead; set `ARTICLE_MIRROR_URL` to change the mirror or to `off`.
- **Article page**: the saved text with a table of contents built from its headings (two or more) and a thin bar at the top that fills as you read. It also suggests a collection and up to five tags from its content: its title, the words of its address and its saved text. Only your own collections and tags are suggested; the collection comes from the most similar articles in your library, and the tags are ones named in the article or carried by similar articles. Click a suggestion to put it in the form; nothing changes until you save.
- **Card designs**: alternative designs for the collection cards, to compare, are at `/lab/shelves`.

## Article text

Each article can keep its text as Markdown, with the images of its body, for reading later. On the article page, **Download text** fetches it from the article's address; **Upload page** takes an HTML page you saved from your browser, or a Markdown file. When a download fails (Medium refuses servers) or saves only a paid preview, the library and the article page show a warning so you can upload the page instead. Once an article has its whole text, its page folds the editor (title, tags and collection) and the controls that replace the text; open them to change anything. When articles in the library have no text yet, the front page offers **Download all**, which queues them for the background download and shows how many are left; articles whose download failed are listed under **See them** (the library's `text=failed` filter). To free space, the **Collections** page deletes the saved texts of every article (**Delete all saved texts**) or of one collection (**Delete texts**); the articles stay, and their texts can be downloaded again. Downloads only reach public addresses on ports 80 and 443. See `docs/api.md` for the details.

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

The JSON export (**Export JSON** in the web app, or `GET /api/export`) is a readable backup of your articles: their addresses, titles, tags, collections and bookmark dates. To restore from it, import the file on the **Add articles** page like a bookmarks file. Articles missing from the library come back, a deleted collection is created again and its articles go back into it, and tags are added; nothing is duplicated and nothing you changed since is overwritten. Restored articles download their text again in the background. The export does not hold the saved texts and images, nor collections without articles; the database backup above keeps everything.

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

## License

ReadVault is source-available under the [PolyForm Noncommercial License 1.0.0](LICENSE.md): you may use, change and share it for any noncommercial purpose, such as personal use, study, research or a hobby, and charities, schools and public bodies may use it too. Selling it or using it to make money is not allowed without the author's permission. Brand logos on tags come from [Simple Icons](https://simpleicons.org) (CC0 data); the marks belong to their owners.
