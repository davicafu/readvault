# ReadVault — Especificación del producto

Versión: 0.2
Estado: MVP implementado (ver «Estado del MVP» en la sección 2)
Última actualización: 14 de noviembre de 2026
Plataforma inicial: Web
Usuarios iniciales: Un usuario, con diseño preparado para soportar múltiples usuarios en el futuro.

## 1. Visión del producto

ReadVault transforma una colección de enlaces guardados en una biblioteca personal de conocimiento técnico, organizada y fácil de consultar.

El caso de uso inicial consiste en importar marcadores del navegador, especialmente enlaces a artículos de Medium, conservar sus metadatos, organizarlos mediante etiquetas y colecciones y recuperarlos fácilmente.

A largo plazo, el producto podrá incorporar resúmenes generados mediante IA (no prioritario), búsqueda semántica, descubrimiento de artículos relacionados y generación de documentación técnica a partir de una o varias fuentes.

### Principios del producto

- Preservar las URL originales y los metadatos disponibles.
- No perder información durante las importaciones.
- Hacer que las importaciones sean idempotentes.
- Separar el contenido original de las notas del usuario y del contenido generado por IA.
- Mantener una arquitectura sencilla, observable y mantenible.
- Permitir exportar los datos sin depender de la aplicación.
- Preparar el modelo de datos para múltiples usuarios sin introducir complejidad prematura.

## 2. Objetivos y exclusiones

### Objetivos del MVP

- Importar marcadores HTML exportados desde navegadores.
- Importar exportaciones JSON de ReadVault para restaurar la biblioteca.
- Mostrar una vista previa antes de confirmar la importación.
- Validar registros y detectar duplicados.
- Normalizar URL de forma conservadora.
- Consultar, buscar, filtrar y editar artículos.
- Exportar los datos a JSON.
- Conservar las carpetas originales de los marcadores cuando existan.
- Ejecutar la aplicación localmente mediante Docker Compose.
- Proporcionar pruebas automatizadas y documentación.

### Estado del MVP (14 de noviembre de 2026)

Implementado:

- Importación de marcadores HTML con vista previa, validación, duplicados, normalización de URL e idempotencia, conservando las carpetas.
- Añadir uno o varios enlaces a la vez desde la web.
- Biblioteca paginada con búsqueda de texto completo, filtros, ordenación y edición de títulos, etiquetas y colección.
- Colecciones (RF-08), con un artículo en una colección como máximo.
- Exportación JSON y restauración importando ese mismo archivo.
- Texto de los artículos (RF-12), sugerencias sin IA (RF-13) y notificaciones (RF-14).
- Ejecución local con Docker Compose (Colima o Docker Desktop) o con un PostgreSQL propio.

### Fuera del alcance inicial

- Iniciar sesión en Medium o usar credenciales de Medium. La descarga del texto de los artículos sí está implementada (RF-12), sin credenciales.
- Enriquecimiento mediante IA.
- Embeddings y búsqueda semántica.
- Extensiones de navegador y aplicaciones móviles.
- Edición colaborativa.
- Suscripciones, pagos y facturación.
- Microservicios, Kubernetes, Kafka o un broker dedicado.

## 3. Usuarios objetivo

### Usuario inicial

Un desarrollador con una colección grande de enlaces técnicos que necesita organizarla y recuperar información rápidamente.

### Usuarios futuros

Desarrolladores, ingenieros y lectores técnicos que buscan una biblioteca personal con clasificación automática, búsqueda avanzada y síntesis de conocimiento asistida por IA.

## 4. Requisitos funcionales

### RF-01: Importación HTML

- Aceptar archivos HTML exportados desde navegadores.
- Interpretar los formatos habituales de exportación de marcadores.
- Conservar las rutas de carpetas anidadas cuando existan.
- Extraer títulos, URL y metadatos disponibles.
- Tolerar títulos ausentes y registros malformados.
- Rechazar esquemas no admitidos, incluidos javascript: y file:.
- No ejecutar ni renderizar el HTML importado como contenido confiable.

### RF-02: Importación de exportaciones JSON

- Aceptar el archivo de la exportación JSON de ReadVault (RF-09) en la misma página y el mismo flujo que los marcadores HTML.
- Recuperar los artículos que falten, volver a crear las colecciones borradas con sus artículos y añadir etiquetas.
- No duplicar nada ni sobrescribir lo que el usuario haya cambiado después.
- Rechazar un JSON que no sea una exportación de una versión compatible.
- Aplicar los mismos límites de tamaño de archivo y número de registros.

### RF-03: Vista previa

Antes de confirmar la importación, mostrar:

- Total de registros detectados.
- Registros válidos e inválidos.
- Duplicados dentro del archivo.
- Artículos que ya existen en la biblioteca.
- Advertencias y errores por registro.
- Título, URL y carpeta de cada candidato.

El usuario debe poder confirmar o cancelar la operación.

### RF-04: Ejecución de la importación

- Persistir los registros válidos mediante operaciones transaccionales cuando proceda.
- Utilizar restricciones de unicidad en la base de datos.
- Hacer que las importaciones repetidas sean idempotentes.
- No sobrescribir silenciosamente títulos ni etiquetas editados por el usuario.
- Conservar las URL originales.
- Informar de registros insertados, omitidos, actualizados y fallidos.
- No conservar indefinidamente los archivos subidos.
- Definir la limpieza de los datos temporales de importación.

### RF-05: Normalización de URL

- Normalizar las mayúsculas del host.
- Eliminar fragmentos de la identidad canónica.
- Eliminar únicamente parámetros de seguimiento reconocidos.
- Conservar parámetros que puedan afectar a la identidad o al contenido.
- Mantener la URL original para mostrarla y navegar.
- No asumir que todas las URL de un mismo dominio representan el mismo artículo.
- Cubrir la normalización con pruebas representativas.

### RF-06: Biblioteca de artículos

- Mostrar artículos en una lista paginada.
- Permitir búsquedas por título, etiquetas y URL.
- Filtrar por etiquetas y estado de importación cuando corresponda.
- Ordenar por título y fecha de creación.
- Mostrar URL original, título, etiquetas y procedencia.
- Conservar las carpetas originales de los marcadores.
- Mostrar estados de carga, vacío y error.

### RF-07: Edición de artículos

- Permitir editar títulos y etiquetas.
- Añadir y eliminar etiquetas.
- Validar los datos en el servidor.
- Preservar las URL originales salvo que una operación explícita permita cambiarlas.
- Mantener las fechas de creación y actualización.

### RF-08: Colecciones

Implementado.

- Crear, renombrar y eliminar colecciones.
- Un artículo pertenece a una colección como máximo; las etiquetas cubren la clasificación múltiple. Decisión tomada al implementar, en lugar de asociar un artículo a varias colecciones.
- Eliminar una colección sin borrar sus artículos.
- La biblioteca empieza con nueve colecciones. Cada colección tiene un icono y un color derivados de su nombre.
- La importación sugiere la colección a partir de la carpeta del marcador.

### RF-09: Exportación

- Exportar los artículos del usuario actual a JSON.
- Conservar URL originales y canónicas, títulos, etiquetas y metadatos.
- Documentar el esquema de exportación y versionarlo cuando proceda.
- Evitar que la exportación incluya datos de otros usuarios.
- Documentar los procedimientos de copia de seguridad y restauración.

### RF-10: Enriquecimiento mediante IA, fase futura

- Generar resúmenes y puntos clave a partir de contenido cuyo procesamiento esté autorizado.
- Sugerir categorías y etiquetas. Ya existe una versión sin IA (RF-13); la IA podría mejorarla.
- Registrar las versiones del modelo y de los prompts.
- Separar los metadatos originales del contenido generado.
- Permitir editar los resultados.
- Registrar el estado y los errores del procesamiento.
- Aplicar límites de tiempo, tokens y coste.
- No implementar esta funcionalidad en el primer hito.

### RF-11: Evolución de la búsqueda

Comenzar con búsqueda de texto completo de PostgreSQL e índices adecuados. Implementado: busca en título, URL y etiquetas, con `-palabra` para excluir, `"frase exacta"` y `:colección` desde el buscador.

Evaluar pgvector para búsqueda semántica únicamente cuando exista un caso de uso validado.

El descubrimiento de artículos relacionados y la generación de documentos a partir de varias fuentes deben conservar referencias a los artículos utilizados.

### RF-12: Texto de los artículos

- Descargar el texto de cada artículo como Markdown, con las imágenes de su cuerpo.
- Tras una importación o al añadir varios enlaces, descargar en segundo plano, con pocos trabajos a la vez y reintentos limitados.
- Si la página original falla, no tiene texto o es una vista previa de pago, probar una copia: un mirror (Freedium) para Medium y para algunos medios de noticias, y después archive.org. Una página de error del mirror no cuenta como copia.
- Permitir subir la página guardada desde el navegador (HTML) o un archivo Markdown cuando la descarga falle.
- Ofrecer «Try another copy» y un enlace a archive.org solo en textos fallidos o incompletos.
- Leer el texto con índice y barra de progreso.
- Borrar el texto de un artículo, de una colección o de todos, sin borrar los artículos.
- Descargar solo de direcciones públicas en los puertos 80 y 443.

### RF-13: Sugerencias

- Sugerir en la página del artículo una colección y hasta cinco etiquetas, solo de las del propio usuario, a partir del título, la dirección y el texto guardado. Sin IA.

### RF-14: Notificaciones

- Una campana en la cabecera muestra los textos que se están descargando, los artículos sin texto (con «Download all») y las descargas fallidas, con un enlace a la lista filtrada. Los avisos solo aparecen ahí.

## 5. Requisitos no funcionales

### RNF-01: Fiabilidad

- Las importaciones deben ser idempotentes.
- Las restricciones de la base de datos deben garantizar las invariantes principales.
- Los fallos parciales deben ser visibles.
- Las transacciones deben proteger las escrituras relacionadas.
- Los errores deben poder recuperarse cuando sea posible.

### RNF-02: Rendimiento

- Paginar las consultas de artículos.
- Limitar el tamaño de las solicitudes y los parámetros de consulta.
- Evitar resultados sin límites.
- Crear índices para los filtros y las búsquedas frecuentes.
- Definir objetivos de rendimiento después de probar con un conjunto representativo de marcadores.

### RNF-03: Seguridad

- Validar y limitar los archivos subidos.
- Utilizar consultas SQL parametrizadas.
- No incluir credenciales en el repositorio.
- Guardar secretos en variables de entorno o en un gestor apropiado.
- Aplicar límites de recursos y tiempos de espera.
- Tratar HTML y contenido importado como datos no confiables.
- Impedir el acceso cruzado entre usuarios cuando se habilite el modo multiusuario.
- No confiar en un identificador de usuario enviado por el cliente como mecanismo de autorización.
- Evitar filtrar errores internos o información sensible en las respuestas.

### RNF-04: Observabilidad

- Utilizar logs estructurados.
- Registrar identificadores de importación y resultados.
- Evitar registrar innecesariamente el contenido completo de los artículos.
- Proporcionar comprobaciones de salud.
- Hacer observables los errores y los recuentos de importación.

### RNF-05: Mantenibilidad

- Separar controladores HTTP, lógica de negocio, persistencia y parsers.
- Mantener los parsers de marcadores HTML y de exportaciones JSON probables de forma independiente.
- Utilizar migraciones versionadas.
- Documentar las decisiones arquitectónicas.
- Evitar abstracciones innecesarias y optimizaciones prematuras.

### RNF-06: Portabilidad

- Proporcionar exportación JSON.
- Documentar copias de seguridad y restauración.
- Evitar dependencias innecesarias de un proveedor de IA o de una plataforma de alojamiento.

## 6. Arquitectura propuesta

### Frontend

- Next.js.
- TypeScript.
- App Router.
- Interfaz responsive y accesible.

### Backend

- API HTTP en Go.
- Monolito modular con arquitectura hexagonal. Cada módulo (`importing`, `library`, `content`, `tag`) contiene su dominio y sus puertos, y debajo sus propios adapters: `http`, `postgres` y, en `content`, `fetch`. El dominio no importa ningún adapter. Lo compartido está en `internal/platform` (router, middleware, errores JSON, conexión y migraciones). Todos los módulos comparten un único esquema de base de datos.
- Separación explícita entre controladores, servicios y repositorios.
- Propagación de contextos y tiempos de espera.
- Respuestas de error consistentes.

### Persistencia

- PostgreSQL.
- Migraciones versionadas.
- Claves foráneas, restricciones de unicidad e índices.
- Búsqueda de texto completo de PostgreSQL.

### Desarrollo local

- Docker Compose; en macOS, Colima como motor ligero. También vale un PostgreSQL 15 o superior propio con `make dev DB=external`.
- Configuración mediante variables de entorno.
- Makefile con comandos habituales.
- Sin credenciales obligatorias de IA.

### Infraestructura aplazada

No introducir Redis, Kafka, Kubernetes, pgvector ni una base de datos vectorial independiente durante el primer hito sin una necesidad demostrada.

## 7. Modelo de datos

### users

- id
- email, nullable hasta implementar autenticación
- created_at
- updated_at

### articles

- id
- user_id
- original_url
- canonical_url
- title
- title_edited_at y tags_edited_at, para no sobrescribir ediciones al reimportar
- original_folder
- bookmarked_at
- collection_id, nullable
- search, columna de búsqueda de texto completo
- created_at
- updated_at

Restricciones:
- URL canónica única por usuario.
- Clave foránea al usuario cuando se habilite la propiedad.
- Índices adecuados para búsquedas y paginación.

Si el MVP utiliza un propietario fijo de desarrollo, debe representarse explícitamente. No dejar ambiguo el modelo de propiedad.

### tags

- id
- user_id
- name
- slug
- created_at

### article_tags

- article_id
- tag_id
- created_at

Restricciones:
- Clave primaria compuesta por article_id y tag_id.
- Claves foráneas y política de borrado adecuadas.

### collections

- id
- user_id
- name
- slug, único por usuario
- created_at
- updated_at

### article_contents, article_images y content_downloads

- article_contents: el texto en Markdown de un artículo, su estado (completo, incompleto o fallido), su origen, el número de palabras, el problema de la última descarga y las imágenes que faltan.
- article_images: las imágenes del texto, guardadas en la base de datos.
- content_downloads: la cola de descargas en segundo plano, con intentos y reserva temporal.

### Tablas futuras

- article_enrichments
- import_jobs, si las importaciones llegan a guardarse

No crear tablas que no se utilicen solo porque aparecen en la hoja de ruta futura.

## 8. Requisitos de la API

Endpoints implementados (detalle en `docs/api.md`):

- POST /api/imports/preview y POST /api/imports
- GET /api/articles, GET, PATCH y DELETE /api/articles/{id}
- GET /api/articles/{id}/suggestions
- GET, POST y DELETE /api/articles/{id}/content; POST /api/articles/{id}/content/fetch
- GET /api/articles/{id}/images/{imageId}
- GET y POST /api/content-downloads
- DELETE /api/contents y DELETE /api/collections/{id}/contents
- GET y POST /api/collections; PATCH y DELETE /api/collections/{id}
- GET /api/tags
- GET /api/export
- GET /healthz y GET /readyz

GET /api/imports/{id} no existe: las importaciones no se guardan y el resultado se devuelve en la respuesta de POST /api/imports.

Requisitos:

- Validar todas las entradas.
- Utilizar códigos HTTP apropiados.
- Devolver estructuras de error consistentes.
- Admitir paginación limitada.
- Aplicar límites de tamaño de archivos.
- Utilizar transacciones cuando proceda.
- No exponer trazas de ejecución ni detalles internos de la base de datos.
- Documentar esquemas de solicitudes y respuestas.

Adaptar los endpoints si es necesario para conseguir un flujo más sencillo y seguro.

## 9. Idempotencia y comportamiento ante duplicados

Las restricciones de unicidad de la base de datos son la fuente de verdad para prevenir duplicados.

Cuando una URL importada ya exista:

- No crear otro artículo.
- No sobrescribir un título editado por el usuario.
- No eliminar etiquetas existentes.
- Conservar la URL original salvo actualización explícita.
- Definir cómo completar metadatos ausentes sin sobrescribir ediciones manuales.
- Informar del duplicado en el resultado de la importación.

Reimportar el mismo archivo no debe generar registros duplicados.

## 10. Pantallas del frontend

### Portada (`/`)

- Las colecciones como carpetas con sus últimos artículos, los artículos añadidos más recientemente y las etiquetas.

### Biblioteca (`/articles`)

- Las direcciones siguen a la API: `/articles?collection=…&q=…&tag=…&text=…&sort=…`. Las antiguas `/?view=all` y `/?collection=…` redirigen aquí.

- Lista paginada de artículos.
- Campo de búsqueda.
- Filtros por etiquetas.
- Ordenación.
- Acción de importar.
- Estados vacío y error.

### Importación

- Subida de archivos.
- Resumen de validación y vista previa.
- Detección de duplicados.
- Acciones de confirmar y cancelar.
- Informe final de importación.

### Colecciones (`/collections`)

- Crear, renombrar y eliminar colecciones, y borrar textos guardados, con botones de icono.

### Añadir artículos (`/add`)

- Uno o varios enlaces a la vez, o un archivo de marcadores o una exportación JSON.

### Detalle del artículo

- Título y URL.
- Carpeta original y metadatos de procedencia.
- Etiquetas y colección editables, con sugerencias.
- Fechas de creación y actualización.
- Texto guardado con índice, y las acciones para descargarlo, subirlo, borrarlo o probar otra copia.

### Diseño visual

- Nombre del producto: ReadVault.
- Estética editorial, moderna y orientada a desarrolladores.
- Tipografía clara y colores sobrios.
- Diseño responsive y controles accesibles.
- Centralizar la configuración de marca.

## 11. Requisitos de pruebas

### Pruebas unitarias

- Parsing de marcadores HTML.
- Parsing de carpetas anidadas.
- Lectura de exportaciones JSON.
- Validación y canonicalización de URL.
- Detección de duplicados.
- Gestión de títulos ausentes.
- Caracteres Unicode.
- Registros malformados.

### Pruebas de integración

- Migraciones desde una base de datos vacía.
- Transacciones de importación.
- Restricciones de unicidad.
- Importaciones repetidas.
- Conservación de ediciones del usuario.
- Listado y paginación.
- Edición de etiquetas.
- Exportación JSON.

### Pruebas de seguridad y robustez

- Archivos demasiado grandes.
- HTML y JSON malformados.
- Esquemas de URL no permitidos.
- Intentos de inyección SQL.
- Identificadores y parámetros inválidos.
- Aislamiento entre usuarios cuando exista propiedad multiusuario.

### Escenarios de aceptación

1. Se importa correctamente un archivo HTML válido.
2. Se conservan las carpetas anidadas.
3. Se restaura la biblioteca importando una exportación JSON.
4. Los registros inválidos se notifican sin perder los válidos.
5. Los duplicados se gestionan de forma determinista.
6. Reimportar el mismo archivo no genera duplicados.
7. Las ediciones manuales de títulos y etiquetas sobreviven a la reimportación.
8. Funcionan la búsqueda, la paginación y la edición de etiquetas.
9. La exportación JSON conserva los metadatos esperados.
10. Las migraciones funcionan desde una base de datos vacía.
11. La aplicación arranca con los comandos documentados.
12. La suite de pruebas y las comprobaciones de compilación terminan correctamente.

## 12. Estructura del repositorio

Estructura actual:

    ReadVault/
    ├── apps/
    │   └── web/                  Next.js
    ├── services/
    │   └── api/
    │       ├── cmd/              api y migrate
    │       ├── internal/         un paquete por módulo, con sus adapters, y platform/
    │       └── migrations/       embebidas en los binarios
    ├── deploy/
    │   └── compose.yaml
    ├── docs/
    │   └── api.md
    ├── fixtures/
    │   └── bookmarks.html
    ├── odd/tasks/                documentos de las features
    ├── Makefile
    ├── .env.example
    └── README.md

No hay `packages/contracts`: el contrato de la API está documentado en `docs/api.md`.

Adaptar la estructura si el repositorio ya tiene convenciones establecidas.

No hay un worker independiente: las descargas en segundo plano se ejecutan dentro de la API, con una cola en PostgreSQL.

## 13. Experiencia de desarrollo

Proporcionar:

- Configuración de Docker Compose.
- Migraciones de base de datos.
- Archivo .env.example.
- Makefile con objetivos dev, test, migrate y lint.
- Fixture HTML representativo.
- README con instrucciones de instalación y resolución de problemas.
- Comportamiento documentado de la API.
- Separación clara entre valores predeterminados de desarrollo y configuración de producción.

El proyecto debe funcionar sin claves de IA ni credenciales de Medium.

## 14. Fases de implementación

### Fase 1: Importador de marcadores (hecha)

- Configuración del repositorio y entorno local.
- Esquema PostgreSQL y migraciones.
- Parsers de marcadores HTML y de exportaciones JSON.
- Vista previa y validación.
- Deduplicación.
- Persistencia y visualización de artículos.
- Pruebas básicas.

### Fase 2: Biblioteca (hecha)

- Paginación.
- Búsqueda y filtros.
- Edición de etiquetas.
- Detalle de artículos.
- Exportación JSON.

### Fase 3: Enriquecimiento con IA

- Ingesta autorizada de contenido (adelantada: el texto de los artículos ya se descarga, RF-12).
- Resúmenes y puntos clave.
- Sugerencias de etiquetas.
- Procesamiento asíncrono y reintentos.
- Control de costes y versiones de resultados.

### Fase 4: Descubrimiento avanzado

- Búsqueda semántica.
- Artículos relacionados.
- Documentos técnicos generados a partir de varias fuentes con referencias.

### Fase 5: Preparación comercial

- Autenticación.
- Aislamiento multiusuario.
- Despliegue de producción.
- Monitorización, copias de seguridad y pruebas de restauración.
- Límites de uso y controles de privacidad.
- Facturación solo después de validar la demanda.

## 15. Definición de terminado del primer hito

El hito estará completo cuando pueda:

1. Arrancar la aplicación localmente.
2. Subir una exportación de marcadores HTML o una exportación JSON de ReadVault.
3. Previsualizar registros válidos, inválidos y duplicados.
4. Confirmar la importación.
5. Navegar por la biblioteca.
6. Buscar por título y URL.
7. Editar títulos y etiquetas.
8. Reimportar el mismo archivo sin duplicados ni pérdida de cambios.
9. Exportar los artículos como JSON.
10. Ejecutar correctamente la suite de pruebas documentada.

## 16. Instrucciones de implementación para Claude Code

1. Inspecciona el repositorio y las herramientas disponibles.
2. Resume brevemente el plan de implementación y las suposiciones.
3. Implementa una solución funcional de extremo a extremo, no solo un plan o una maqueta.
4. Añade pruebas junto con el código.
5. Ejecuta formateo, pruebas y comprobaciones de compilación.
6. Corrige los fallos cuando sea posible e informa de las limitaciones pendientes.
7. Documenta cómo arrancar, probar y diagnosticar la aplicación.

Si el repositorio está vacío, inicialízalo. Si ya contiene código, conserva las convenciones existentes salvo que haya una razón sólida para cambiarlas.

No afirmes que una operación o prueba ha tenido éxito si no se ha ejecutado y verificado realmente.

Ante requisitos ambiguos, elige la interpretación segura más sencilla, documéntala y continúa, salvo que sea imprescindible pedir una aclaración.

**Objetivo final:** un MVP de ReadVault ejecutable localmente que importe marcadores de navegador de forma fiable, evite duplicados, ofrezca una biblioteca consultable, permita editar etiquetas y exporte los datos sin necesitar servicios externos de IA.
