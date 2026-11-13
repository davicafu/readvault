# Backup and restore

Objective: clean the kept bookmarks file, restore a library (collections included) from the JSON export, put icons in the header, and decide the reading fallback for sites that cannot be read. Branch: `feat/readvault-22-backup-restore`.
Delivery strategy: `ask-on-risk`. Test runner: `go test ./...`, `go test -race -tags=integration ./...`; web: tsc and screenshots.

## Specs

- **S1** (L1) "elimna los archive.ph" from the kept bookmarks file.
- **S2** (L1) "los que tengan freedium quitale la parte de freedium asi queda la fuente original y si no se puede leer salta a freedium".
- **S3** (L1) "añade freedium como fallback para cualquier link que no se puede leer, o en caso de que no se pueda leer completamente, ofrece la posibilidad de decargarlo por ahi pero sutilmente, sin nombrar freediu". Finding: Freedium answers any non-Medium address with its own home page (HTTP 200, title "Freedium - Freedium"), so it cannot read other sites; Medium (custom domains included) already falls back automatically. Open: which fallback to use for other sites.
- **S4** (L1) "el export debe funcionar como un backup, me parece bien que guarde todo eso pero deberia ser una forma tambien para que exista un import y se pierda la coleccion que sea recuperable desde ahi". The import accepts the export file: missing articles come back, collections are recreated and articles without a collection go back to theirs; tags are added. The export keeps its current content (no saved texts).
- **S5** (L1) "añade al lado de los textos de arriba algun icono que cuadre con el texto que tiene".

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S1, S2 | inline | done (not committed: the file is gitignored personal data) | 33 archive.ph links removed, 60 Freedium links unwrapped (freedium-mirror.cfd 59, freedium.cfd 1), " - Freedium" dropped from 58 titles; 489 links left, no duplicates |
| T2 | S5 | inline | done (`22401aa`) | Library, Collections, Add articles, Export JSON with icons; screenshots at 1300 and 760 px |
| T3 | S4 | inline | done (this commit) | Parser tests RED (Parse and ErrInvalidFile undefined) then GREEN: export read into records, bookmarks files unchanged, limit enforced, malformed, other versions or trailing data are ErrInvalidFile. HTTP test RED then GREEN: 422 invalid_file. Integration: import an export, delete its collection and one article, preview (1 new, 1 existing, Backend to create) and import again: article back, collection recreated with its article, tags kept, nothing duplicated (written before the code, not run RED). Full suites with -race pass. Live preview of the real export: 524 existing, 9 collections existing; bad JSON 422; fixture HTML unchanged |
| T4 | S3 | - | needs decision | |

## Log

- L1 (2026-11-13, user): "vale los articulos quiero que hagas una limpieza en el bookmark final: - elimna los archive.ph - los que tengan freedium quitale la parte de freedium asi queda la fuente original y si no se puede leer salta a freedium - añade freedium como fallback para cualquier link que no se puede leer, o en caso de que no se pueda leer completamente, ofrece la posibilidad de decargarlo por ahi pero sutilmente, sin nombrar freediu, - el export debe funcionar como un backup, me parece bien que guarde todo eso pero deberia ser una forma tambien para que exista un import y se pierda la coleccion que sea recuperable desde ahi - añade al lado de los textos de arriba algun icono que cuadre con el texto que tiene"
