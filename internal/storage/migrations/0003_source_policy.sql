-- Per-source stream policy. Built-in sources keep the strict defaults; only a
-- user source may store a consent, and only the catalog layer may widen the
-- scheme allowlist.
ALTER TABLE sources ADD COLUMN allow_private INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sources ADD COLUMN allowed_schemes TEXT NOT NULL DEFAULT 'http,https';
