import {
  siApacheairflow,
  siApachecassandra,
  siApacheflink,
  siApachehadoop,
  siApachekafka,
  siApachenifi,
  siApachespark,
  siClaude,
  siClickhouse,
  siD3,
  siDatabricks,
  siDeno,
  siDocker,
  siDuckdb,
  siElasticsearch,
  siFigma,
  siGit,
  siGithub,
  siGnubash,
  siGo,
  siGrafana,
  siGraphql,
  siJavascript,
  siKubernetes,
  siLinux,
  siMongodb,
  siMysql,
  siNeo4j,
  siNeovim,
  siNextdotjs,
  siNginx,
  siNodedotjs,
  siNumpy,
  siOpenjdk,
  siPandas,
  siPostgresql,
  siPrometheus,
  siPython,
  siPytorch,
  siR,
  siRabbitmq,
  siReact,
  siRedis,
  siRust,
  siScala,
  siSnowflake,
  siSpring,
  siSqlite,
  siSvelte,
  siTailwindcss,
  siTensorflow,
  siTerraform,
  siTrino,
  siTypescript,
  siVim,
  siVuedotjs,
  type SimpleIcon,
} from "simple-icons";

import { TagIcon } from "./CollectionIcon";

// Brand logos from Simple Icons (CC0 data; the marks belong to their
// owners), for tags that name a technology. Each tag is matched by its
// words, so "kafka-streams" gets Kafka's logo. Only the logos listed here
// are bundled.
const logos: Record<string, SimpleIcon> = {
  airflow: siApacheairflow,
  bash: siGnubash,
  cassandra: siApachecassandra,
  claude: siClaude,
  clickhouse: siClickhouse,
  d3: siD3,
  databricks: siDatabricks,
  deno: siDeno,
  docker: siDocker,
  duckdb: siDuckdb,
  elasticsearch: siElasticsearch,
  figma: siFigma,
  flink: siApacheflink,
  git: siGit,
  github: siGithub,
  go: siGo,
  golang: siGo,
  grafana: siGrafana,
  graphql: siGraphql,
  hadoop: siApachehadoop,
  java: siOpenjdk,
  javascript: siJavascript,
  js: siJavascript,
  k8s: siKubernetes,
  kafka: siApachekafka,
  kubernetes: siKubernetes,
  linux: siLinux,
  mongodb: siMongodb,
  mysql: siMysql,
  neo4j: siNeo4j,
  neovim: siNeovim,
  nextjs: siNextdotjs,
  nginx: siNginx,
  nifi: siApachenifi,
  node: siNodedotjs,
  nodejs: siNodedotjs,
  numpy: siNumpy,
  pandas: siPandas,
  postgres: siPostgresql,
  postgresql: siPostgresql,
  prometheus: siPrometheus,
  python: siPython,
  pytorch: siPytorch,
  r: siR,
  rabbitmq: siRabbitmq,
  react: siReact,
  redis: siRedis,
  rust: siRust,
  scala: siScala,
  snowflake: siSnowflake,
  spark: siApachespark,
  spring: siSpring,
  sqlite: siSqlite,
  svelte: siSvelte,
  tailwind: siTailwindcss,
  tailwindcss: siTailwindcss,
  tensorflow: siTensorflow,
  terminal: siGnubash,
  terraform: siTerraform,
  trino: siTrino,
  typescript: siTypescript,
  ts: siTypescript,
  vim: siVim,
  vue: siVuedotjs,
};

/** The brand logo a tag names, if any: the whole name first, then each word. */
export function tagLogo(name: string): SimpleIcon | undefined {
  const lower = name.toLowerCase();
  const whole = lower.replace(/[^a-z0-9]/g, "");
  return logos[whole] ?? lower.split(/[^a-z0-9]+/).map((w) => logos[w]).find(Boolean);
}

// Brand colours too dark or too light to read on the pill are left to the
// text colour.
function readable(hex: string): boolean {
  const n = parseInt(hex, 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const v = c / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  return luminance > 0.06 && luminance < 0.75;
}

// TagLogo is a tag's mark: the technology's logo in its colour when the tag
// names one, the icon of its subject otherwise.
export function TagLogo({ name, className }: { name: string; className?: string }) {
  const logo = tagLogo(name);
  if (!logo) return <TagIcon name={name} className={className} />;
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      aria-hidden="true"
      focusable="false"
      style={readable(logo.hex) ? { color: `#${logo.hex}` } : undefined}
    >
      <path d={logo.path} fill="currentColor" />
    </svg>
  );
}
