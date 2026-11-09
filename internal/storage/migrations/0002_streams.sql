ALTER TABLE channels ADD COLUMN epg_id TEXT;

ALTER TABLE sources ADD COLUMN last_success_at INTEGER;
ALTER TABLE sources ADD COLUMN item_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE channel_streams (
	id          INTEGER PRIMARY KEY,
	channel_id  INTEGER NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
	provider_id TEXT NOT NULL,
	url         TEXT NOT NULL,
	quality     TEXT,
	is_hls      INTEGER NOT NULL DEFAULT 0,
	is_live     INTEGER NOT NULL DEFAULT 1,
	position    INTEGER NOT NULL DEFAULT 0,
	headers     TEXT,
	UNIQUE (channel_id, provider_id)
);

CREATE INDEX channel_streams_channel ON channel_streams (channel_id, position);
