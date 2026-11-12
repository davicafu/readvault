# Library care

Objective: free the space of saved texts (all, or one collection's), start with English default collections that get an icon, and make the front page and import feel modern (search in the header, file picker, livelier backdrop, smooth first load). Branch: `feat/readvault-20-reading-ui`.
Delivery strategy: `ask-on-risk`, chain strategy `stacked-to-main`. Test runner: `go test ./...` and `go test -race -tags=integration ./...`; web has no test runner (tsc, screenshots).

## Specs

- **S1** Text cleanup (L1): "me gustaria que hubiera tambien una funcion de limpieza tanto global como por colecciones de los textos guardos, no los articulo, solo lo que se descarga, peude ser util cuando tengas poca memoria en el dispositivo o no quieras almacenar tanto".
  - `DELETE /api/contents` removes the saved text (and its images and recorded failure) of every article of the user; `DELETE /api/collections/{id}/contents` only of that collection's articles. Articles, tags and collections stay. Queued background downloads in the scope are dropped too. Answers `{"removed": n}`; an unknown collection is 404.
  - The Collections page offers both, each behind a confirmation.
- **S2** Default collections in English (L1): "cambia los titulos de la colecciones a ingles y quiero qu eesas aparezcan por defecto, y que despues el suuario las modifique o cree mas si quiere, cuando se creen nuevas si puede ser molaria que se le asginara tambien un incono".
  - A migration renames the user's nine collections to English and creates the ones missing, for every user; the user can still rename, delete and create collections.
  - A new collection gets an icon picked from its name (English and Spanish words); the Collections page shows each collection's icon, including while typing a new name.
- **S3** Modern file picker on Add articles (L1): "en import files hazme favor y ponme uns selector de ficheros moderno, no que parezca de windwos 98".
- **S4** Livelier backdrop (L1): "el fondo sigue siendo demasiado sobrio, quiero que sea algo mucho mas bonito y atractivo".
- **S5** Smooth first load (L1): "la carga inicial parece que va a trompicones o lento, lo que busco es dar sensacion suave y de carga armoniosa y con gusto".
- **S6** Search in the header (L2): "la barra de seach no me gusta ahi, prefioro que sea mas moderna en la parte superior la lado de library y todo eso con, que ya tenga una lupa con la caja de texto y el placeholder qu etiene ahora".

Already in place, no change (L1 questions): suggestions are computed each time the article page loads and again right after a download (the page refreshes); a failed download already tells the user to save the page as HTML and upload it.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S1 (API) | inline | pending | |
| T2 | S2 (migration) | inline | pending | |
| T3 | S1, S2 (web) | inline | pending | |
| T4 | S3-S6 (web) | inline | pending | |

## Log

- L1 (2026-11-12, user): "-en import files hazme favor y ponme uns selector de ficheros moderno, no que parezca de windwos 98 - las recomendaciones las hace despues de descargar el texto o cuando? porque si no no me tiene sentido las recomendacions de texto, si es asi es mejor que despues de cargar el texto se recargue esa funcion - cuando falla varias veces se muestra al usuario la posibilidad de que descargue el html completo y lo cargue en la web? - el fondo sigue siendo demasiado sobrio, quiero que sea algo mucho mas bonito y atractivo - la carga inicial parece que va a trompicones o lento, lo que busco es dar sensacion suave y de carga armoniosa y con gusto, si no parece que va lenta la carga cuando no es asi, - me gustaria que hubiera tambien una funcion de limpieza tanto global como por colecciones de los textos guardos, no los articulo, solo lo que se descarga, peude ser util cuando tengas poca memoria en el dispositivo o no quieras almacenar tanto - cambia los titulos de la colecciones a ingles y quiero qu eesas aparezcan por defecto, y que despues el suuario las modifique o cree mas si quiere, cuando se creen nuevas si puede ser molaria que se le asginara tambien un incono"
- L2 (2026-11-12, user): "la barra de seach no me gusta ahi, prefioro que sea mas moderna en la parte superior la lado de library y todo eso con, que ya tenga una lupa con la caja de texto y el placeholder qu etiene ahora"
- L3: Assumptions: the cleanup removes uploaded texts too (they are saved texts); after a cleanup the front page offers "Download all" again. The icon is picked automatically from the name, not chosen by hand.
