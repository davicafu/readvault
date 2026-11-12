// An icon for a collection, picked from the words of its name, so a reader
// recognises "Bases de datos" or "Streaming y Kafka" at a glance. Names are
// the user's own, in any language: unknown ones get a book.

type Kind =
  | "stream"
  | "learning"
  | "ai"
  | "pipeline"
  | "database"
  | "cloud"
  | "interface"
  | "architecture"
  | "server"
  | "book";

// Checked in order: the first rule with a matching word wins, so the more
// specific subjects come first ("ingeniería de datos" is a pipeline, not a
// database).
const rules: [Kind, RegExp][] = [
  ["stream", /^(streaming|stream|streams|kafka|eventos|events|event|flink|pulsar|kinesis)$/],
  ["learning", /^(machine|learning|ml|analisis|analysis|analytics|estadistica|statistics)$/],
  ["ai", /^(ia|ai|llm|llms|gpt|agentes|agents|prompts?)$/],
  ["pipeline", /^(ingenieria|engineering|pipelines?|etl|elt|airflow|dbt|spark|lakehouse)$/],
  ["database", /^(bases|database|databases|sql|postgres|postgresql|mysql|duckdb|iceberg|clickhouse)$/],
  ["cloud", /^(devops|cloud|linux|kubernetes|k8s|docker|aws|gcp|azure|infra|infraestructura)$/],
  ["interface", /^(frontend|visualizacion|visualization|ui|ux|css|react|design-systems?)$/],
  ["architecture", /^(arquitectura|architecture|system|sistemas|patrones|patterns|ddd)$/],
  ["server", /^(backend|api|apis|go|golang|java|rust|python|microservicios|microservices)$/],
];

export function collectionKind(name: string): Kind {
  const words = name
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .split(/[^a-z0-9]+/)
    .filter(Boolean);
  for (const [kind, pattern] of rules) {
    if (words.some((w) => pattern.test(w))) return kind;
  }
  return "book";
}

// 24×24 line drawings, stroked with the current colour.
const paths: Record<Kind, string[]> = {
  stream: ["M3 8c3-3 6 3 9 0s6 3 9 0", "M3 13c3-3 6 3 9 0s6 3 9 0", "M3 18c3-3 6 3 9 0s6 3 9 0"],
  learning: ["M4 20V4", "M4 20h16", "M7 15l4-4 3 3 5-6", "M17 8h2v2"],
  ai: ["M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z", "M19 15l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z"],
  pipeline: ["M3 7h5v4H3z", "M16 13h5v4h-5z", "M8 9h3a2 2 0 0 1 2 2v2a2 2 0 0 0 2 2h1", "M10 4l-2 3 2 3"],
  database: [
    "M5 6c0-1.7 3.1-3 7-3s7 1.3 7 3-3.1 3-7 3-7-1.3-7-3z",
    "M5 6v12c0 1.7 3.1 3 7 3s7-1.3 7-3V6",
    "M5 12c0 1.7 3.1 3 7 3s7-1.3 7-3",
  ],
  cloud: ["M7 18h10a4 4 0 0 0 .5-8 6 6 0 0 0-11.4 1.5A3.3 3.3 0 0 0 7 18z", "M10 21v-3", "M14 21v-3"],
  interface: ["M3 5h18v14H3z", "M3 9h18", "M7 13h4v3H7z", "M13 13h4", "M13 16h3"],
  architecture: ["M4 4h6v6H4z", "M14 4h6v6h-6z", "M9 14h6v6H9z", "M7 10v2h10v-2", "M12 12v2"],
  server: ["M4 4h16v6H4z", "M4 14h16v6H4z", "M8 7h.01", "M8 17h.01", "M12 7h5", "M12 17h5"],
  book: ["M5 4h9a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3z", "M5 17a3 3 0 0 1 3-3h9", "M9 8h5"],
};

export function CollectionIcon({ name, className }: { name: string; className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {paths[collectionKind(name)].map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}
