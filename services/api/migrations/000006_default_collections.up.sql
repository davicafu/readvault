-- Every user starts with the same collections, named in English. The
-- collections of the first library, named in Spanish, are renamed in place,
-- keeping their articles; the others are created. Users rename, delete and
-- add collections afterwards. Slugs follow tag.Slugify: lower case, words
-- joined with "-".
CREATE TEMPORARY TABLE default_collections (old_slug text, name text, slug text) ON COMMIT DROP;
INSERT INTO default_collections (old_slug, name, slug) VALUES
    ('ia-y-herramientas',            'AI and tools',                   'ai-and-tools'),
    ('arquitectura-y-system-design', 'Architecture and system design', 'architecture-and-system-design'),
    ('backend',                      'Backend',                        'backend'),
    ('ingeniería-de-datos',          'Data engineering',               'data-engineering'),
    ('bases-de-datos-y-sql',         'Databases and SQL',              'databases-and-sql'),
    ('devops,-cloud-y-linux',        'DevOps, cloud and Linux',        'devops,-cloud-and-linux'),
    ('frontend-y-visualización',     'Frontend and visualization',     'frontend-and-visualization'),
    ('machine-learning-y-análisis',  'Machine learning and analytics', 'machine-learning-and-analytics'),
    ('streaming-y-kafka',            'Streaming and Kafka',            'streaming-and-kafka');

-- A user who already has the English name keeps both: nothing is merged.
UPDATE collections c
SET name = d.name, slug = d.slug, updated_at = now()
FROM default_collections d
WHERE c.slug = d.old_slug
  AND NOT EXISTS (SELECT 1 FROM collections x WHERE x.user_id = c.user_id AND x.slug = d.slug);

INSERT INTO collections (user_id, name, slug)
SELECT u.id, d.name, d.slug
FROM users u CROSS JOIN default_collections d
ON CONFLICT (user_id, slug) DO NOTHING;
